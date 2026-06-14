package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// Math-delimiter normalization → GFM ($..$ inline, $$..$$ block), done at the
// source so any agent renders without per-agent prompting. Handles the three
// ways agents emit math: \[..\]/\(..\) in prose, ```latex/```math fenced blocks,
// and `$..$`/`\(..\)`/`\[..\]`-wrapped inline code. Real (non-math) code is left
// untouched.
const mathBacktick = "`"

var (
	mathFenceRe      = regexp.MustCompile("(?is)```(?:latex|math)[ \\t]*\\r?\\n(.*?)\\r?\\n?```")
	inlineCodeMathRe = regexp.MustCompile(mathBacktick + `(\\\[.*?\\\]|\\\(.*?\\\)|\$[^` + mathBacktick + `\n]+\$)` + mathBacktick)
	codeSpanRe       = regexp.MustCompile("(?s)```.*?```|`[^`]*`")
	blockMathRe      = regexp.MustCompile(`(?s)\\\[(.*?)\\\]`)
	inlineMathRe     = regexp.MustCompile(`(?s)\\\((.*?)\\\)`)
)

// telegramAPIBase returns the Bot API base URL (official or self-hosted).
func (c *Channel) telegramAPIBase() string {
	if c.config.APIServer != "" {
		return strings.TrimRight(c.config.APIServer, "/")
	}
	return "https://api.telegram.org"
}

// inputRichMessage mirrors Bot API 10.1 InputRichMessage (markdown variant only for v1).
type inputRichMessage struct {
	Markdown string `json:"markdown"`
}

type richReplyParameters struct {
	MessageID                int  `json:"message_id"`
	AllowSendingWithoutReply bool `json:"allow_sending_without_reply,omitempty"`
}

type sendRichMessageParams struct {
	ChatID          int64                `json:"chat_id"`
	RichMessage     inputRichMessage     `json:"rich_message"`
	MessageThreadID int                  `json:"message_thread_id,omitempty"`
	ReplyParameters *richReplyParameters `json:"reply_parameters,omitempty"`
}

type sendRichMessageDraftParams struct {
	ChatID          int64            `json:"chat_id"`
	DraftID         int              `json:"draft_id"`
	RichMessage     inputRichMessage `json:"rich_message"`
	MessageThreadID int              `json:"message_thread_id,omitempty"`
}

// telegramAPIResponse is the standard {ok, result, description, error_code} envelope.
type telegramAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Channel) doRawPost(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/bot%s/%s", c.telegramAPIBase(), c.config.Token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response (%d): %w", method, resp.StatusCode, err)
	}

	var env telegramAPIResponse
	if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
		return nil, fmt.Errorf("%s: bad response (%d): %s", method, resp.StatusCode, string(raw))
	}
	if !env.OK {
		// Surface description so existing parseErrRe/messageTooLongRe/draftFallbackRe regexes match.
		return nil, fmt.Errorf("%s failed (code %d): %s", method, env.ErrorCode, env.Description)
	}
	return env.Result, nil
}

// sendRichMessage POSTs a Rich Markdown message. Returns the new message_id.
func (c *Channel) sendRichMessage(ctx context.Context, p sendRichMessageParams) (int, error) {
	result, err := c.doRawPost(ctx, "sendRichMessage", p)
	if err != nil {
		return 0, err
	}
	var msg struct {
		MessageID int `json:"message_id"`
	}
	_ = json.Unmarshal(result, &msg)
	return msg.MessageID, nil
}

// sendRichMessageDraft POSTs an ephemeral Rich Markdown draft preview.
func (c *Channel) sendRichMessageDraft(ctx context.Context, p sendRichMessageDraftParams) error {
	_, err := c.doRawPost(ctx, "sendRichMessageDraft", p)
	return err
}

// prepareRichMarkdown is near-identity: trim only. The agent already emits GFM;
// Rich Markdown accepts it verbatim. Do NOT run markdownToTelegramHTML (lossy).
func (c *Channel) prepareRichMarkdown(text string) string {
	return normalizeMathDelimiters(strings.TrimSpace(text))
}

// normalizeMathDelimiters rewrites the math forms agents emit into GFM math.
func normalizeMathDelimiters(text string) string {
	// 1. ```latex / ```math fences. Mixed content (already has $ / \[ / \() is a
	//    prose+multi-formula block — strip the fence and let step 3 convert each
	//    region (wrapping the whole thing in $$ would render prose as math). Only a
	//    single bare formula (no delimiters) is wrapped as one $$ block.
	text = mathFenceRe.ReplaceAllStringFunc(text, func(m string) string {
		inner := strings.TrimSpace(mathFenceRe.FindStringSubmatch(m)[1])
		if strings.ContainsAny(inner, "$") || strings.Contains(inner, `\[`) || strings.Contains(inner, `\(`) {
			return inner
		}
		return "$$\n" + inner + "\n$$"
	})
	// 2. Unwrap inline code that is purely math: `$x$` / `\(x\)` / `\[x\]` → the math.
	text = inlineCodeMathRe.ReplaceAllString(text, "${1}")
	// 3. Prose \[..\] → $$..$$ and \(..\) → $..$, skipping remaining (real) code spans.
	if !strings.Contains(text, `\[`) && !strings.Contains(text, `\(`) {
		return text
	}
	var b strings.Builder
	last := 0
	for _, loc := range codeSpanRe.FindAllStringIndex(text, -1) {
		b.WriteString(convertMath(text[last:loc[0]])) // prose before the code span
		b.WriteString(text[loc[0]:loc[1]])            // code span verbatim
		last = loc[1]
	}
	b.WriteString(convertMath(text[last:]))
	return b.String()
}

func convertMath(s string) string {
	s = blockMathRe.ReplaceAllStringFunc(s, func(m string) string {
		return "$$" + blockMathRe.FindStringSubmatch(m)[1] + "$$"
	})
	s = inlineMathRe.ReplaceAllStringFunc(s, func(m string) string {
		return "$" + inlineMathRe.FindStringSubmatch(m)[1] + "$"
	})
	return s
}

// trySendRich attempts the Rich Markdown path. Returns true if delivered.
// On any error returns false so the caller falls through to the HTML path.
func (c *Channel) trySendRich(ctx context.Context, chatID int64, content string, replyTo, threadID int, localKey string) bool {
	md := c.prepareRichMarkdown(content)

	// Delete any stream placeholder: rich send creates a fresh bubble and
	// we cannot edit a plain message into a rich one in v1.
	if pID, ok := c.placeholders.LoadAndDelete(localKey); ok {
		if id, ok := pID.(int); ok && id > 0 {
			_ = c.deleteMessage(ctx, chatID, id)
		}
	}

	p := sendRichMessageParams{
		ChatID:          chatID,
		RichMessage:     inputRichMessage{Markdown: md},
		MessageThreadID: resolveThreadIDForSend(threadID),
	}
	if replyTo > 0 {
		p.ReplyParameters = &richReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
	}

	rerr := c.retrySend(ctx, "sendRichMessage", nil, func(ctx context.Context) error {
		_, e := c.sendRichMessage(ctx, p)
		return e
	})
	if rerr == nil {
		return true
	}
	// Thread-not-found: retry once without thread (parity with sendHTML Case 3).
	if p.MessageThreadID != 0 && threadNotFoundRe.MatchString(rerr.Error()) {
		p.MessageThreadID = 0
		if _, e := c.sendRichMessage(ctx, p); e == nil {
			return true
		}
	}
	slog.Warn("telegram: sendRichMessage failed, falling back to HTML", "chat_id", chatID, "error", rerr)
	return false
}
