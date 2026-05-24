package consolidation

import (
	"strings"
	"testing"
)

func TestRenderJudgePrompt_Basic(t *testing.T) {
	p := RenderJudgePrompt(JudgeInput{
		CustomerMessage: "where is my order",
		TeamReply:       "shipped today",
	})
	if !strings.Contains(p, "where is my order") || !strings.Contains(p, "shipped today") {
		t.Fatalf("missing inputs in prompt: %s", p)
	}
	if !strings.Contains(p, "valid JSON") {
		t.Fatal("prompt should ask for JSON-only response")
	}
}

func TestRenderJudgePrompt_EmptyCustomer(t *testing.T) {
	p := RenderJudgePrompt(JudgeInput{TeamReply: "yes"})
	if !strings.Contains(p, "(no preceding customer message captured)") {
		t.Fatal("expected placeholder for empty customer message")
	}
}

func TestParseJudgeResponse_BareJSON(t *testing.T) {
	raw := `{"hypothesized_bot_reply":"hi","diff_score":0.7,"diff_reasoning":"close"}`
	v, err := ParseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.HypothesizedBotReply != "hi" || v.DiffScore != 0.7 || v.DiffReasoning != "close" {
		t.Fatalf("verdict mismatch: %+v", v)
	}
}

func TestParseJudgeResponse_CodeFence(t *testing.T) {
	raw := "```json\n{\"hypothesized_bot_reply\":\"x\",\"diff_score\":0.3,\"diff_reasoning\":\"y\"}\n```"
	v, err := ParseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("parse fenced: %v", err)
	}
	if v.DiffScore != 0.3 {
		t.Fatalf("score: %f", v.DiffScore)
	}
}

func TestParseJudgeResponse_LeadingProse(t *testing.T) {
	raw := "Sure, here it is: {\"hypothesized_bot_reply\":\"z\",\"diff_score\":0.5,\"diff_reasoning\":\"q\"}"
	v, err := ParseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("parse prose: %v", err)
	}
	if v.HypothesizedBotReply != "z" {
		t.Fatalf("hypo: %s", v.HypothesizedBotReply)
	}
}

func TestParseJudgeResponse_ScoreClamped(t *testing.T) {
	raw := `{"hypothesized_bot_reply":"a","diff_score":1.7,"diff_reasoning":"b"}`
	v, _ := ParseJudgeResponse(raw)
	if v.DiffScore != 1.0 {
		t.Fatalf("expected clamped to 1.0, got %f", v.DiffScore)
	}
	raw2 := `{"hypothesized_bot_reply":"a","diff_score":-0.4,"diff_reasoning":"b"}`
	v2, _ := ParseJudgeResponse(raw2)
	if v2.DiffScore != 0 {
		t.Fatalf("expected clamped to 0, got %f", v2.DiffScore)
	}
}

func TestParseJudgeResponse_EmptyHypoErrors(t *testing.T) {
	raw := `{"hypothesized_bot_reply":"  ","diff_score":0.5,"diff_reasoning":"x"}`
	if _, err := ParseJudgeResponse(raw); err == nil {
		t.Fatal("expected error for empty hypothesized_bot_reply")
	}
}

func TestParseJudgeResponse_Garbage(t *testing.T) {
	if _, err := ParseJudgeResponse(""); err == nil {
		t.Fatal("expected error for empty")
	}
	if _, err := ParseJudgeResponse("not json"); err == nil {
		t.Fatal("expected error for non-json")
	}
}
