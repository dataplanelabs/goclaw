package tools

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

type ZaloPersonalReactTool struct {
	actionFn ZaloPersonalActionFn
}

func NewZaloPersonalReactTool() *ZaloPersonalReactTool { return &ZaloPersonalReactTool{} }

func (t *ZaloPersonalReactTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) { t.actionFn = fn }

func (t *ZaloPersonalReactTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}

func (t *ZaloPersonalReactTool) Name() string { return "zalo_personal_react" }

func (t *ZaloPersonalReactTool) Description() string {
	return "Add or remove a reaction on a Zalo Personal message. " +
		"reaction accepts unicode emoji (❤️), English name (heart), or raw Zalo code; pass empty string to remove. " +
		"target_msg_id and target_cli_msg_id come from inbound metadata keys 'message_id' and 'cli_msg_id'."
}

func (t *ZaloPersonalReactTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_msg_id": map[string]any{
				"type":        "string",
				"description": "Global message ID. Pass inbound metadata key 'message_id'.",
			},
			"target_cli_msg_id": map[string]any{
				"type":        "string",
				"description": "Client message ID. Pass inbound metadata key 'cli_msg_id'.",
			},
			"reaction": map[string]any{
				"type":        "string",
				"description": "Emoji, English name, or raw Zalo code. Empty string removes any prior reaction.",
			},
			"thread_type": map[string]any{
				"type":        "string",
				"enum":        []string{"user", "group"},
				"description": "Optional explicit thread type. Read from prior inbound metadata.peer_kind. Defaults to auto-detect.",
			},
		},
		"required": []string{"target_msg_id", "target_cli_msg_id", "reaction"},
	}
}

func (t *ZaloPersonalReactTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	chatID := ToolChatIDFromCtx(ctx)
	if chatID == "" {
		return ErrorResult("missing chat ID in context")
	}

	msgID := argString(args, "target_msg_id")
	cliMsgID := argString(args, "target_cli_msg_id")
	if msgID == "" || cliMsgID == "" {
		return ErrorResult("target_msg_id and target_cli_msg_id are required (find them in inbound metadata keys 'message_id' and 'cli_msg_id')")
	}
	// Distinguish missing key from explicit empty string (which means remove).
	if _, present := args["reaction"]; !present {
		return ErrorResult("reaction parameter is required (pass empty string '' to remove a reaction)")
	}
	reaction := argString(args, "reaction")

	if err := handle.React(ctx, chatID, msgID, cliMsgID, reaction, argString(args, "thread_type")); err != nil {
		return ErrorResult(fmt.Sprintf("react: %v", err))
	}
	status := "added"
	if reaction == "" {
		status = "removed"
	}
	return jsonResult(map[string]any{
		"status":   status,
		"msg_id":   msgID,
		"reaction": reaction,
	})
}
