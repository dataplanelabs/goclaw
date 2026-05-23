package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

func TestReact_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"target_msg_id":     "100",
		"target_cli_msg_id": "200",
		"reaction":          "heart",
		"thread_type":       "group",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.reactCall.reaction != "heart" || fake.reactCall.hint != "group" {
		t.Errorf("call mismatch: %+v", fake.reactCall)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.ForLLM), &out)
	if out["status"] != "added" {
		t.Errorf("status=%v, want added", out["status"])
	}
}

func TestReact_EmptyReactionMeansRemove(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"target_msg_id":     "100",
		"target_cli_msg_id": "200",
		"reaction":          "",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.ForLLM), &out)
	if out["status"] != "removed" {
		t.Errorf("status=%v, want removed", out["status"])
	}
}

func TestReact_MissingReactionKeyIsError(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"target_msg_id":     "100",
		"target_cli_msg_id": "200",
	})
	if !res.IsError {
		t.Errorf("missing reaction key must error (distinct from empty string)")
	}
}

func TestReact_MissingTargetID(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"reaction": "heart",
	})
	if !res.IsError {
		t.Errorf("missing target_msg_id must error")
	}
}

func TestReact_WrongChannelType(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	ctx := context.Background()
	ctx = WithToolChannelType(ctx, "discord")
	ctx = WithToolChannel(ctx, "dc")
	ctx = WithToolChatID(ctx, "chat")
	res := tool.Execute(ctx, map[string]any{
		"target_msg_id": "1", "target_cli_msg_id": "2", "reaction": "heart",
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "zalo_personal") {
		t.Errorf("want channel-type error: %+v", res)
	}
}

func TestReact_PropagatesActionError(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{reactErr: errors.New("boom")}
	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"target_msg_id": "1", "target_cli_msg_id": "2", "reaction": "heart",
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "boom") {
		t.Errorf("want propagated error: %+v", res)
	}
}

func TestReact_OnlyOnZaloPersonal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = WithToolChannelType(ctx, channels.TypeZaloPersonal)
	ctx = WithToolChannel(ctx, "zp")
	ctx = WithToolChatID(ctx, "u1")

	tool := NewZaloPersonalReactTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(ctx, map[string]any{
		"target_msg_id": "1", "target_cli_msg_id": "2", "reaction": "heart",
	})
	if res.IsError {
		t.Errorf("should succeed on zalo_personal channel: %s", res.ForLLM)
	}
}
