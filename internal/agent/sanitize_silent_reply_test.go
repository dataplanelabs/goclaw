package agent

import "testing"

func TestIsSilentReply(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		// Exact matches.
		{"exact", "NO_REPLY", true},
		{"with whitespace", "  NO_REPLY  ", true},
		{"with newlines", "\nNO_REPLY\n", true},
		// Decorative variants — the bug report.
		{"trailing underscore", "NO_REPLY_", true},
		{"double trailing underscore", "NO_REPLY__", true},
		{"leading underscore", "_NO_REPLY", true},
		{"both underscores", "_NO_REPLY_", true},
		{"trailing dot", "NO_REPLY.", true},
		{"trailing bang", "NO_REPLY!", true},
		{"double-quoted", `"NO_REPLY"`, true},
		{"single-quoted", "'NO_REPLY'", true},
		{"backticked", "`NO_REPLY`", true},
		{"markdown bold", "**NO_REPLY**", true},
		{"parenthesized", "(NO_REPLY)", true},
		// Case-insensitive.
		{"lowercase", "no_reply", true},
		{"mixed case", "No_Reply", true},
		// Silent — token + explanation (user intent: prefix-match, divergent from upstream).
		{"prefix + space + content", "NO_REPLY hello", true},
		{"prefix + colon + content", "NO_REPLY: offline", true},
		{"prefix + because", "NO_REPLY because user is away", true},
		{"reason then terminal sentinel", "The scheduled reminder was cancelled.\n\nNO_REPLY", true},
		{"suffix with whitespace", "No reminder is needed. NO_REPLY", true},
		{"token mid-sentence", "Hello NO_REPLY world", false},
		{"token mentioned as instruction", "Use NO_REPLY when no response is needed.", false},
		// Leading <br> an LLM may prepend before the token.
		{"leading br self-close", "<br/>NO_REPLY", true},
		{"leading br spaced", "<br/>\n\nNO_REPLY (task done)", true},
		{"leading br open tag", "<br>NO_REPLY", true},
		{"leading br with space", "<br />  NO_REPLY", true},
		{"multiple leading br", "<br/><br/>NO_REPLY", true},
		// NOT silent - token glued to another word.
		{"embedded word", "NO_REPLYING", false},
		{"glued prefix", "XNO_REPLY", false},
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"unrelated text", "no reply needed", false},
		// NOT silent — <br> but no token (the prod trace: freeform note, no sentinel).
		{"br then freeform note", "<br/>\n\n(Không gửi - chị đã xong)", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSilentReply(c.in); got != c.want {
				t.Errorf("IsSilentReply(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
