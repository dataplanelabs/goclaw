package consolidation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const batchJudgePromptTemplate = `You are evaluating customer-support replies written by human team members, compared to what an AI assistant WOULD have written. You will grade %d pairs in this single call.

For EACH pair (indexed 0 to %d):
1. Compose what YOU (an expert assistant) would have replied to the customer message, ignoring the team reply.
2. Compare your reply to the team reply. Score similarity on a 0.0–1.0 scale:
   - 1.0 = essentially the same content + tone + actionability
   - 0.5 = same intent but different wording/structure
   - 0.0 = fundamentally different approach
3. In 1-2 sentences, explain the key difference.

PAIRS TO GRADE:
%s

Respond with ONLY valid JSON in this exact shape (no markdown, no prose):
{"verdicts":[{"idx":0,"hypothesized_bot_reply":"...","diff_score":0.0,"diff_reasoning":"..."}, ...]}

The verdicts array MUST have exactly %d entries, one per pair, with idx matching the pair index. Do not skip pairs.`

type BatchJudgeInput struct {
	EvaluationID    string
	CustomerMessage string
	TeamReply       string
}

type batchVerdictWire struct {
	Idx                  int     `json:"idx"`
	HypothesizedBotReply string  `json:"hypothesized_bot_reply"`
	DiffScore            float64 `json:"diff_score"`
	DiffReasoning        string  `json:"diff_reasoning"`
}

type batchResponseWire struct {
	Verdicts []batchVerdictWire `json:"verdicts"`
}

func RenderBatchJudgePrompt(rows []BatchJudgeInput) string {
	if len(rows) == 0 {
		return ""
	}
	var pairs strings.Builder
	for i, r := range rows {
		customer := strings.TrimSpace(r.CustomerMessage)
		if customer == "" {
			customer = "(no preceding customer message captured)"
		}
		pairs.WriteString("--- PAIR ")
		pairs.WriteString(strconv.Itoa(i))
		pairs.WriteString(" ---\nCUSTOMER MESSAGE:\n")
		pairs.WriteString(customer)
		pairs.WriteString("\nTEAM REPLY:\n")
		pairs.WriteString(strings.TrimSpace(r.TeamReply))
		pairs.WriteString("\n\n")
	}
	n := len(rows)
	return fmt.Sprintf(batchJudgePromptTemplate, n, n-1, pairs.String(), n)
}

// ParseBatchJudgeResponse returns (verdicts, true) on full success;
// (nil, false) on any failure so caller can fall back to per-row grading.
func ParseBatchJudgeResponse(raw string, expectedN int) ([]JudgeVerdict, bool) {
	body := strings.TrimSpace(stripCodeFence(raw))
	if body == "" {
		return nil, false
	}
	if !strings.HasPrefix(body, "{") {
		if idx := strings.Index(body, "{"); idx >= 0 {
			body = body[idx:]
		}
	}
	if !strings.HasSuffix(body, "}") {
		if idx := strings.LastIndex(body, "}"); idx >= 0 {
			body = body[:idx+1]
		}
	}
	var wire batchResponseWire
	if err := json.Unmarshal([]byte(body), &wire); err != nil {
		return nil, false
	}
	if len(wire.Verdicts) != expectedN {
		return nil, false
	}
	out := make([]JudgeVerdict, expectedN)
	seen := make(map[int]bool, expectedN)
	for _, v := range wire.Verdicts {
		if v.Idx < 0 || v.Idx >= expectedN || seen[v.Idx] {
			return nil, false
		}
		if strings.TrimSpace(v.HypothesizedBotReply) == "" {
			return nil, false
		}
		seen[v.Idx] = true
		score := v.DiffScore
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		out[v.Idx] = JudgeVerdict{
			HypothesizedBotReply: v.HypothesizedBotReply,
			DiffScore:            score,
			DiffReasoning:        v.DiffReasoning,
		}
	}
	return out, true
}
