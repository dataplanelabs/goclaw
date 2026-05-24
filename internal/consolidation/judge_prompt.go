package consolidation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JudgeInput is what the judge agent sees per evaluation.
type JudgeInput struct {
	CustomerMessage string
	TeamReply       string
}

// JudgeVerdict is the structured judge response.
type JudgeVerdict struct {
	HypothesizedBotReply string  `json:"hypothesized_bot_reply"`
	DiffScore            float64 `json:"diff_score"`
	DiffReasoning        string  `json:"diff_reasoning"`
}

// judgePromptTemplate asks the judge to compose a hypothesized bot reply
// and grade similarity to the team-typed reply. JSON-only response keeps
// parsing deterministic.
const judgePromptTemplate = `You are evaluating a customer-support reply written by a human team member, compared to what an AI assistant WOULD have written.

CUSTOMER MESSAGE:
%s

TEAM REPLY (what the human actually said):
%s

Your task:
1. Compose what YOU (an expert assistant) would have replied to this customer message, ignoring the team reply for now. Be concise and helpful.
2. Compare your reply to the team reply. Score similarity on a 0.0–1.0 scale:
   - 1.0 = essentially the same content + tone + actionability
   - 0.5 = same intent but different wording/structure
   - 0.0 = fundamentally different approach
3. In 1-2 sentences, explain the key difference (style, content, missed information, etc.).

Respond ONLY with valid JSON:
{"hypothesized_bot_reply":"...","diff_score":0.0,"diff_reasoning":"..."}`

// RenderJudgePrompt formats the canonical judge prompt.
func RenderJudgePrompt(in JudgeInput) string {
	customer := strings.TrimSpace(in.CustomerMessage)
	if customer == "" {
		customer = "(no preceding customer message captured)"
	}
	return fmt.Sprintf(judgePromptTemplate, customer, strings.TrimSpace(in.TeamReply))
}

// ParseJudgeResponse extracts a verdict from raw LLM output. Tolerates
// common wrappers (```json fences, leading prose). Score clamped to [0,1].
func ParseJudgeResponse(raw string) (*JudgeVerdict, error) {
	body := strings.TrimSpace(stripCodeFence(raw))
	if body == "" {
		return nil, fmt.Errorf("judge: empty response")
	}
	// Sometimes the LLM wraps with "Sure, here it is:" prose; isolate the
	// first balanced JSON object.
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
	var v JudgeVerdict
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, fmt.Errorf("judge: unmarshal: %w", err)
	}
	if strings.TrimSpace(v.HypothesizedBotReply) == "" {
		return nil, fmt.Errorf("judge: empty hypothesized_bot_reply")
	}
	if v.DiffScore < 0 {
		v.DiffScore = 0
	}
	if v.DiffScore > 1 {
		v.DiffScore = 1
	}
	return &v, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// drop opening fence (with optional language tag)
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		// drop closing fence
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}
