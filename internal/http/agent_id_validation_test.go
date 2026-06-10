package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAgentUUIDRejectsRouteSentinels(t *testing.T) {
	for _, agentID := range []string{"", "undefined", "null", "not-a-uuid"} {
		t.Run(agentID, func(t *testing.T) {
			w := httptest.NewRecorder()
			if requireAgentUUID(w, agentID) {
				t.Fatalf("expected %q to be rejected", agentID)
			}
			if w.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, stdhttp.StatusBadRequest)
			}
		})
	}
}

func TestRequireAgentUUIDAllowsUUID(t *testing.T) {
	w := httptest.NewRecorder()
	if !requireAgentUUID(w, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatal("expected valid UUID to pass")
	}
	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want implicit %d", w.Code, stdhttp.StatusOK)
	}
}
