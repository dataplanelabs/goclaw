package agent

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

type captionExecutor struct{ stubExecutor }

func (e *captionExecutor) ExecuteWithContext(_ context.Context, _ string, _ map[string]any, _, _, _, _ string, _ tools.AsyncCallback) *tools.Result {
	return &tools.Result{
		ForLLM: "weekly report",
		Media: []bus.MediaFile{{
			Path:     "/workspace/report.pdf",
			MimeType: "application/pdf",
			Caption:  "weekly report",
		}},
	}
}

func TestPipelineMediaCaptionRoundTrip(t *testing.T) {
	state := &pipeline.RunState{}
	bridge := &runState{}
	loop := newTestLoopForToolCallbacks(func(AgentEvent) {})
	loop.tools = &captionExecutor{}

	_, err := loop.makeExecuteToolCall(&RunRequest{RunID: "run-caption"}, bridge)(
		context.Background(),
		state,
		providers.ToolCall{ID: "tool-caption", Name: "send_file"},
	)
	if err != nil {
		t.Fatalf("makeExecuteToolCall() error: %v", err)
	}
	if got := state.Tool.MediaResults[0].Caption; got != "weekly report" {
		t.Fatalf("pipeline caption = %q, want weekly report", got)
	}

	result := convertRunResult(&pipeline.RunResult{MediaResults: state.Tool.MediaResults})
	if got := result.Media[0].Caption; got != "weekly report" {
		t.Fatalf("agent caption = %q, want weekly report", got)
	}
}
