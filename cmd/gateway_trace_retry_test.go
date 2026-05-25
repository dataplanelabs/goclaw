package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// fakeRetryAgent is the minimum agent.Agent surface dispatchRetryOutbound
// actually touches — just enough to compile + return identifiable values.
type fakeRetryAgent struct {
	uuid        uuid.UUID
	otherConfig json.RawMessage
}

func (f *fakeRetryAgent) ID() string                                                { return "fake" }
func (f *fakeRetryAgent) UUID() uuid.UUID                                           { return f.uuid }
func (f *fakeRetryAgent) OtherConfig() json.RawMessage                              { return f.otherConfig }
func (f *fakeRetryAgent) Run(context.Context, agent.RunRequest) (*agent.RunResult, error) { return nil, nil }
func (f *fakeRetryAgent) IsRunning() bool                                           { return false }
func (f *fakeRetryAgent) Model() string                                             { return "" }
func (f *fakeRetryAgent) ProviderName() string                                      { return "" }
func (f *fakeRetryAgent) Provider() providers.Provider                              { return nil }

func pollOutbound(t *testing.T, mb *bus.MessageBus) (bus.OutboundMessage, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	return mb.SubscribeOutbound(ctx)
}

func TestDispatchRetryOutbound_PublishesText(t *testing.T) {
	mb := bus.New()
	agentUUID := uuid.New()
	ag := &fakeRetryAgent{uuid: agentUUID, otherConfig: json.RawMessage(`{}`)}
	req := &agent.RunRequest{Channel: "zalo-shtp", ChatID: "chat-1"}
	traceID := uuid.New()
	result := &agent.RunResult{Content: "hello", TraceID: traceID}

	dispatchRetryOutbound(context.Background(), mb, req, result, ag, uuid.New())

	got, ok := pollOutbound(t, mb)
	if !ok {
		t.Fatal("expected outbound to be published")
	}
	if got.Channel != "zalo-shtp" || got.ChatID != "chat-1" {
		t.Errorf("routing wrong: channel=%q chat=%q", got.Channel, got.ChatID)
	}
	if got.Content != "hello" {
		t.Errorf("content = %q, want %q", got.Content, "hello")
	}
	if got.TraceID != traceID {
		t.Errorf("trace_id propagation failed")
	}
	if got.AgentID != agentUUID {
		t.Errorf("agent_id propagation failed")
	}
}

func TestDispatchRetryOutbound_AttachesMedia(t *testing.T) {
	mb := bus.New()
	ag := &fakeRetryAgent{uuid: uuid.New()}
	req := &agent.RunRequest{Channel: "zalo-shtp", ChatID: "chat-1"}
	result := &agent.RunResult{
		Content: "here is the file",
		Media:   []agent.MediaResult{{Path: "/tmp/foo.pdf", ContentType: "application/pdf"}},
	}

	dispatchRetryOutbound(context.Background(), mb, req, result, ag, uuid.New())

	got, ok := pollOutbound(t, mb)
	if !ok {
		t.Fatal("expected outbound with media")
	}
	if len(got.Media) != 1 || got.Media[0].URL != "/tmp/foo.pdf" {
		t.Errorf("media not attached: %+v", got.Media)
	}
}

func TestDispatchRetryOutbound_SkipsWhenSilent(t *testing.T) {
	mb := bus.New()
	ag := &fakeRetryAgent{uuid: uuid.New()}
	req := &agent.RunRequest{Channel: "zalo-shtp", ChatID: "chat-1"}

	for _, content := range []string{"", "NO_REPLY"} {
		result := &agent.RunResult{Content: content}
		dispatchRetryOutbound(context.Background(), mb, req, result, ag, uuid.New())
		if _, ok := pollOutbound(t, mb); ok {
			t.Errorf("must not publish for silent content %q", content)
		}
	}
}

func TestDispatchRetryOutbound_SkipsWhenNoChannel(t *testing.T) {
	mb := bus.New()
	ag := &fakeRetryAgent{uuid: uuid.New()}
	req := &agent.RunRequest{Channel: "", ChatID: "chat-1"}
	result := &agent.RunResult{Content: "hello"}

	dispatchRetryOutbound(context.Background(), mb, req, result, ag, uuid.New())
	if _, ok := pollOutbound(t, mb); ok {
		t.Fatal("must not publish when Channel is empty")
	}
}

func TestDispatchRetryOutbound_StampsGroupMetadata(t *testing.T) {
	mb := bus.New()
	ag := &fakeRetryAgent{uuid: uuid.New()}
	req := &agent.RunRequest{Channel: "zalo-shtp", ChatID: "group-1", PeerKind: "group"}
	result := &agent.RunResult{Content: "hi"}

	dispatchRetryOutbound(context.Background(), mb, req, result, ag, uuid.New())

	got, ok := pollOutbound(t, mb)
	if !ok {
		t.Fatal("expected outbound")
	}
	if got.Metadata["group_id"] != "group-1" {
		t.Errorf("group_id metadata = %q, want group-1 (so channel.Send routes via group API)", got.Metadata["group_id"])
	}
}
