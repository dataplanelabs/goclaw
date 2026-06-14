package methods

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ---- stub WorkstationStore ----

type stubWsStore struct {
	workstations map[uuid.UUID]*store.Workstation
	updateErr    error
	// captureUpdates records the updates map passed to Update for inspection.
	captureUpdates map[string]any
}

func newStubWsStore() *stubWsStore {
	return &stubWsStore{workstations: make(map[uuid.UUID]*store.Workstation)}
}

func (s *stubWsStore) Create(_ context.Context, ws *store.Workstation) error {
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	s.workstations[ws.ID] = ws
	return nil
}

func (s *stubWsStore) GetByID(_ context.Context, id uuid.UUID) (*store.Workstation, error) {
	ws, ok := s.workstations[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return ws, nil
}

func (s *stubWsStore) GetByKey(_ context.Context, key string) (*store.Workstation, error) {
	for _, ws := range s.workstations {
		if ws.WorkstationKey == key {
			return ws, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *stubWsStore) List(_ context.Context) ([]store.Workstation, error) {
	result := make([]store.Workstation, 0, len(s.workstations))
	for _, ws := range s.workstations {
		result = append(result, *ws)
	}
	return result, nil
}

func (s *stubWsStore) Update(_ context.Context, _ uuid.UUID, updates map[string]any) error {
	s.captureUpdates = updates
	return s.updateErr
}

func (s *stubWsStore) SetActive(_ context.Context, _ uuid.UUID, _ bool) error { return nil }

func (s *stubWsStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }

// ---- stub AgentWorkstationLinkStore ----

type stubWsLinkStore struct{}

func (s *stubWsLinkStore) Link(_ context.Context, _ *store.AgentWorkstationLink) error { return nil }
func (s *stubWsLinkStore) Unlink(_ context.Context, _, _ uuid.UUID) error              { return nil }
func (s *stubWsLinkStore) SetDefault(_ context.Context, _, _ uuid.UUID) error          { return nil }
func (s *stubWsLinkStore) ListForAgent(_ context.Context, _ uuid.UUID) ([]store.AgentWorkstationLink, error) {
	return nil, nil
}
func (s *stubWsLinkStore) ListForWorkstation(_ context.Context, _ uuid.UUID) ([]store.AgentWorkstationLink, error) {
	return nil, nil
}

// ---- helpers ----

func buildWorkstationMethods(t *testing.T) (*WorkstationsMethods, *stubWsStore) {
	t.Helper()
	ws := newStubWsStore()
	return NewWorkstationsMethods(ws, &stubWsLinkStore{}), ws
}

func wsReqFrame(t *testing.T, method string, params map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &protocol.RequestFrame{
		Type:   protocol.FrameTypeRequest,
		ID:     "ws-req-1",
		Method: method,
		Params: raw,
	}
}

// readResponse drains one frame from the capturing channel and unmarshals it.
func readResponse(t *testing.T, ch <-chan []byte) *protocol.ResponseFrame {
	t.Helper()
	select {
	case raw := <-ch:
		var f protocol.ResponseFrame
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		return &f
	default:
		return nil
	}
}

// addSSHWorkstation seeds an SSH workstation into the stub store and returns its ID.
func addSSHWorkstation(t *testing.T, ws *stubWsStore, privateKey string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	meta, _ := json.Marshal(map[string]any{
		"host":       "192.168.1.1",
		"port":       22,
		"user":       "deploy",
		"privateKey": privateKey,
	})
	ws.workstations[id] = &store.Workstation{
		ID:             id,
		WorkstationKey: "test-ws",
		BackendType:    store.BackendSSH,
		Metadata:       meta,
		DefaultEnv:     []byte("{}"),
		Active:         true,
	}
	return id
}

// ---- Tests: handleUpdate — security: no privateKey in error response ----

// TestWorkstationsUpdate_StoreError_PrivateKeyNotInResponse verifies that when
// the store Update returns an error, the SSH privateKey from params is NOT
// included in the WS error response (security regression: gcplane logged it).
func TestWorkstationsUpdate_StoreError_PrivateKeyNotInResponse(t *testing.T) {
	m, stub := buildWorkstationMethods(t)

	const sensitiveKey = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA0Z3VS5JJcds"
	wsID := addSSHWorkstation(t, stub, sensitiveKey)

	// Make Update() return an error so the handler reaches the error response path.
	stub.updateErr = fmt.Errorf("unable to encode map[string]interface{}{\"host\":\"192.168.1.1\",\"privateKey\":%q} into bytea", sensitiveKey)

	tenantID := uuid.New()
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "admin-user")

	req := wsReqFrame(t, protocol.MethodWorkstationsUpdate, map[string]any{
		"id": wsID.String(),
		"updates": map[string]any{
			"metadata": map[string]any{
				"host":       "192.168.1.1",
				"port":       22,
				"user":       "deploy",
				"privateKey": sensitiveKey,
			},
		},
	})

	m.handleUpdate(context.Background(), client, req)

	resp := readResponse(t, ch)
	if resp == nil {
		t.Fatal("expected a response frame, got none")
	}

	// Marshal the full response to check its JSON representation.
	respJSON, _ := json.Marshal(resp)
	if strings.Contains(string(respJSON), sensitiveKey) {
		t.Errorf("privateKey found in error response: %s", string(respJSON))
	}
	if strings.Contains(string(respJSON), "BEGIN RSA") {
		t.Errorf("PEM key material found in error response: %s", string(respJSON))
	}
}

// ---- Tests: handleUpdate — correctness: metadata converted to []byte before store ----

// TestWorkstationsUpdate_MetadataConvertedToBytes verifies that when metadata is
// passed as a JSON object in the updates map, handleUpdate converts it to []byte
// before calling the store — so the store can encrypt it as a bytea column.
func TestWorkstationsUpdate_MetadataConvertedToBytes(t *testing.T) {
	m, stub := buildWorkstationMethods(t)

	wsID := addSSHWorkstation(t, stub, "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK")

	tenantID := uuid.New()
	client, _ := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "admin-user")

	req := wsReqFrame(t, protocol.MethodWorkstationsUpdate, map[string]any{
		"id": wsID.String(),
		"updates": map[string]any{
			"metadata": map[string]any{
				"host":       "10.0.0.1",
				"port":       2222,
				"user":       "ubuntu",
				"privateKey": "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK",
			},
		},
	})

	m.handleUpdate(context.Background(), client, req)

	if stub.captureUpdates == nil {
		t.Fatal("Update was not called on the store")
	}
	metaVal, ok := stub.captureUpdates["metadata"]
	if !ok {
		t.Fatal("metadata key missing from updates passed to store")
	}
	switch metaVal.(type) {
	case []byte:
		// correct
	default:
		t.Errorf("metadata passed to store is %T, want []byte", metaVal)
	}
}

// ---- Tests: handleUpdate — defaultEnv validation ----

// TestWorkstationsUpdate_DefaultEnv covers the three cases for the defaultEnv
// validation block: nested object rejected, non-string value rejected, and a
// valid flat map[string]string that reaches the store as default_env []byte.
func TestWorkstationsUpdate_DefaultEnv(t *testing.T) {
	cases := []struct {
		name        string
		defaultEnv  any
		wantErrCode string // non-empty → expect an error response with this code
		wantKey     string // non-empty → expect this key in store updates
	}{
		{
			name:        "nested object rejected",
			defaultEnv:  map[string]any{"a": map[string]any{"b": 1}},
			wantErrCode: protocol.ErrInvalidRequest,
		},
		{
			name:        "non-string value rejected",
			defaultEnv:  map[string]any{"a": 1},
			wantErrCode: protocol.ErrInvalidRequest,
		},
		{
			name:       "valid flat map reaches store as default_env bytes",
			defaultEnv: map[string]any{"GH_TOKEN": "x"},
			wantKey:    "default_env",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, stub := buildWorkstationMethods(t)
			wsID := addSSHWorkstation(t, stub, "synthetic-key-material")
			tenantID := uuid.New()
			client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "admin-user")

			req := wsReqFrame(t, protocol.MethodWorkstationsUpdate, map[string]any{
				"id": wsID.String(),
				"updates": map[string]any{
					"defaultEnv": tc.defaultEnv,
				},
			})

			m.handleUpdate(context.Background(), client, req)

			resp := readResponse(t, ch)
			if resp == nil {
				t.Fatal("expected response frame, got none")
			}

			if tc.wantErrCode != "" {
				if resp.Error == nil {
					t.Fatalf("expected error %q, got success", tc.wantErrCode)
				}
				if resp.Error.Code != tc.wantErrCode {
					t.Errorf("error code = %q, want %q", resp.Error.Code, tc.wantErrCode)
				}
				return
			}

			// Happy path: store must have been called with default_env key as []byte.
			if stub.captureUpdates == nil {
				t.Fatal("store Update was not called")
			}
			if _, hasOld := stub.captureUpdates["defaultEnv"]; hasOld {
				t.Error("store received camelCase 'defaultEnv' key; expected snake_case 'default_env'")
			}
			val, ok := stub.captureUpdates[tc.wantKey]
			if !ok {
				t.Fatalf("store updates missing key %q; got %v", tc.wantKey, stub.captureUpdates)
			}
			b, isBytes := val.([]byte)
			if !isBytes {
				t.Fatalf("store updates[%q] is %T, want []byte", tc.wantKey, val)
			}
			var decoded map[string]string
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatalf("store default_env bytes not valid JSON: %v", err)
			}
			if decoded["GH_TOKEN"] != "x" {
				t.Errorf("decoded GH_TOKEN = %q, want %q", decoded["GH_TOKEN"], "x")
			}
		})
	}
}
