package consolidation

import (
	"strings"
	"testing"
)

func TestRenderBatchJudgePrompt_NEntries(t *testing.T) {
	rows := []BatchJudgeInput{
		{EvaluationID: "e1", CustomerMessage: "What's the price?", TeamReply: "It's 850k VND"},
		{EvaluationID: "e2", CustomerMessage: "Size?", TeamReply: "Size 40"},
	}
	prompt := RenderBatchJudgePrompt(rows)
	if !strings.Contains(prompt, "grade 2 pairs") {
		t.Fatalf("missing N=2 in instruction:\n%s", prompt)
	}
	if !strings.Contains(prompt, "PAIR 0") || !strings.Contains(prompt, "PAIR 1") {
		t.Fatalf("missing PAIR markers:\n%s", prompt)
	}
	if !strings.Contains(prompt, "850k VND") || !strings.Contains(prompt, "Size 40") {
		t.Fatalf("missing reply content:\n%s", prompt)
	}
}

func TestRenderBatchJudgePrompt_EmptyCustomerFallback(t *testing.T) {
	rows := []BatchJudgeInput{
		{EvaluationID: "e1", CustomerMessage: "", TeamReply: "Dạ"},
	}
	prompt := RenderBatchJudgePrompt(rows)
	if !strings.Contains(prompt, "no preceding customer message captured") {
		t.Fatalf("empty-customer fallback missing")
	}
}

func TestParseBatchJudgeResponse_HappyPath(t *testing.T) {
	raw := `{"verdicts":[
		{"idx":0,"hypothesized_bot_reply":"A","diff_score":0.7,"diff_reasoning":"close"},
		{"idx":1,"hypothesized_bot_reply":"B","diff_score":0.3,"diff_reasoning":"far"}
	]}`
	out, ok := ParseBatchJudgeResponse(raw, 2)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if out[0].DiffScore != 0.7 || out[1].DiffScore != 0.3 {
		t.Fatalf("scores wrong: %v", out)
	}
}

func TestParseBatchJudgeResponse_WrongLength(t *testing.T) {
	raw := `{"verdicts":[{"idx":0,"hypothesized_bot_reply":"A","diff_score":0.5,"diff_reasoning":"x"}]}`
	if _, ok := ParseBatchJudgeResponse(raw, 2); ok {
		t.Fatalf("expected ok=false on length mismatch")
	}
}

func TestParseBatchJudgeResponse_DuplicateIdx(t *testing.T) {
	raw := `{"verdicts":[
		{"idx":0,"hypothesized_bot_reply":"A","diff_score":0.5,"diff_reasoning":"x"},
		{"idx":0,"hypothesized_bot_reply":"B","diff_score":0.6,"diff_reasoning":"y"}
	]}`
	if _, ok := ParseBatchJudgeResponse(raw, 2); ok {
		t.Fatalf("expected ok=false on duplicate idx")
	}
}

func TestParseBatchJudgeResponse_MissingHypothesizedReply(t *testing.T) {
	raw := `{"verdicts":[
		{"idx":0,"hypothesized_bot_reply":"","diff_score":0.5,"diff_reasoning":"x"},
		{"idx":1,"hypothesized_bot_reply":"B","diff_score":0.6,"diff_reasoning":"y"}
	]}`
	if _, ok := ParseBatchJudgeResponse(raw, 2); ok {
		t.Fatalf("expected ok=false on empty hypothesized_bot_reply")
	}
}

func TestParseBatchJudgeResponse_ClampsScore(t *testing.T) {
	raw := `{"verdicts":[
		{"idx":0,"hypothesized_bot_reply":"A","diff_score":1.7,"diff_reasoning":"x"},
		{"idx":1,"hypothesized_bot_reply":"B","diff_score":-0.3,"diff_reasoning":"y"}
	]}`
	out, ok := ParseBatchJudgeResponse(raw, 2)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if out[0].DiffScore != 1.0 || out[1].DiffScore != 0.0 {
		t.Fatalf("clamp failed: %+v", out)
	}
}

func TestParseBatchJudgeResponse_HandlesCodeFence(t *testing.T) {
	raw := "```json\n{\"verdicts\":[{\"idx\":0,\"hypothesized_bot_reply\":\"A\",\"diff_score\":0.5,\"diff_reasoning\":\"x\"}]}\n```"
	if _, ok := ParseBatchJudgeResponse(raw, 1); !ok {
		t.Fatalf("expected ok=true with code fence")
	}
}

func TestParseBatchJudgeResponse_MalformedJSON(t *testing.T) {
	if _, ok := ParseBatchJudgeResponse("not json at all", 2); ok {
		t.Fatalf("expected ok=false on malformed JSON")
	}
}
