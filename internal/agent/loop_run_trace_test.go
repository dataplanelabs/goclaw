package agent

import (
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestTraceCompletionForRunResult_LoopKilledIsError(t *testing.T) {
	result := &RunResult{
		Content:    "loop detector fallback",
		LoopKilled: true,
	}

	status, errMsg, outputPreview := traceCompletionForRunResult(result, 100)

	if status != store.TraceStatusError {
		t.Fatalf("status = %q, want %q", status, store.TraceStatusError)
	}
	if errMsg != loopKilledErrorMessage {
		t.Fatalf("errMsg = %q, want %q", errMsg, loopKilledErrorMessage)
	}
	if outputPreview != result.Content {
		t.Fatalf("outputPreview = %q, want %q", outputPreview, result.Content)
	}
}

func TestTraceCompletionForRunResult_NormalRunCompleted(t *testing.T) {
	result := &RunResult{Content: "ok"}

	status, errMsg, outputPreview := traceCompletionForRunResult(result, 100)

	if status != store.TraceStatusCompleted {
		t.Fatalf("status = %q, want %q", status, store.TraceStatusCompleted)
	}
	if errMsg != "" {
		t.Fatalf("errMsg = %q, want empty", errMsg)
	}
	if outputPreview != result.Content {
		t.Fatalf("outputPreview = %q, want %q", outputPreview, result.Content)
	}
}

func TestRunCompletedPayload_LoopKilledMetadata(t *testing.T) {
	payload := runCompletedPayload(&RunResult{
		Content:    "loop detector fallback",
		Iterations: 5,
		ToolCalls:  7,
		LoopKilled: true,
	}, 12*time.Millisecond)

	if payload["loop_killed"] != true {
		t.Fatalf("loop_killed = %v, want true", payload["loop_killed"])
	}
	if payload["failure_class"] != loopKilledFailureClass {
		t.Fatalf("failure_class = %v, want %q", payload["failure_class"], loopKilledFailureClass)
	}
	if payload["content"] != "loop detector fallback" {
		t.Fatalf("content = %v, want fallback content", payload["content"])
	}
}
