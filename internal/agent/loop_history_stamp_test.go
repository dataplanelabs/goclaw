package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// 08:59 UTC is 15:59 in Asia/Ho_Chi_Minh (+07).
var stampUTC = time.Date(2026, 1, 2, 8, 59, 0, 0, time.UTC)

func ctxWithTurn(tz string, at time.Time) context.Context {
	return store.WithRunContext(context.Background(), &store.RunContext{
		UserTimezone:  tz,
		TurnStartedAt: at,
	})
}

func TestStampCurrentMessageTime_Group(t *testing.T) {
	ctx := ctxWithTurn("Asia/Ho_Chi_Minh", stampUTC)
	msg := "[Chat messages since your last reply - for context]\n  Writer One [15:50]: hi\n\n" +
		channels.CurrentMessageMarker + "\n[From: Writer Two]\nquestion"
	out := stampCurrentMessageTime(ctx, msg)
	if !strings.Contains(out, channels.CurrentMessageMarker+" [15:59]\n") {
		t.Fatalf("expected marker stamped with [15:59], got:\n%s", out)
	}
	if strings.Contains(out, "Writer One [15:50]: hi [15:59]") {
		t.Fatalf("must not stamp buffer lines, got:\n%s", out)
	}
}

func TestStampCurrentMessageTime_DM(t *testing.T) {
	ctx := ctxWithTurn("Asia/Ho_Chi_Minh", stampUTC)
	out := stampCurrentMessageTime(ctx, "[From: Writer Two]\nhi")
	if !strings.HasPrefix(out, "[15:59]\n") {
		t.Fatalf("expected DM prefix [15:59], got:\n%s", out)
	}
}

func TestStampCurrentMessageTime_UTCFallback(t *testing.T) {
	ctx := ctxWithTurn("", stampUTC)
	out := stampCurrentMessageTime(ctx, "hi")
	if !strings.HasPrefix(out, "[08:59]\n") {
		t.Fatalf("expected UTC fallback [08:59], got:\n%s", out)
	}
}

func TestStampCurrentMessageTime_NoRunContext(t *testing.T) {
	if out := stampCurrentMessageTime(context.Background(), "hi"); out != "hi" {
		t.Fatalf("expected no-op without RunContext, got: %q", out)
	}
}
