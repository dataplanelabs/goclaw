package channels

import "testing"

func TestCopyRoutingMeta_PreservesPancakeCommentRouting(t *testing.T) {
	src := map[string]string{
		"pancake_mode":        "comment",
		"reply_to_comment_id": "msg-123",
		"sender_id":           "user-123",
	}

	got := copyRoutingMeta(src)

	for key, want := range map[string]string{
		"pancake_mode":        "comment",
		"reply_to_comment_id": "msg-123",
		"sender_id":           "user-123",
	} {
		if got[key] != want {
			t.Fatalf("copyRoutingMeta()[%q] = %q, want %q", key, got[key], want)
		}
	}
}

func TestCopyFinalRoutingMeta_PreservesPlaceholderAndPancakeMode(t *testing.T) {
	src := map[string]string{
		"placeholder_key": "placeholder-123",
		"pancake_mode":    "comment",
	}

	got := CopyFinalRoutingMeta(src)

	if got["placeholder_key"] != "placeholder-123" {
		t.Fatalf("CopyFinalRoutingMeta()[%q] = %q, want %q", "placeholder_key", got["placeholder_key"], "placeholder-123")
	}
	if got["pancake_mode"] != "comment" {
		t.Fatalf("CopyFinalRoutingMeta()[%q] = %q, want %q", "pancake_mode", got["pancake_mode"], "comment")
	}
}

// TestCopyRoutingMeta_PreservesZaloPersonalQuotePayload verifies that the
// zalo_personal quote-payload metadata key survives both the intermediate
// block-reply path and the final-reply path. Without this the Phase 4 outbound
// Send() would receive an empty payload and the entire feature would silently
// degrade to non-quoted sends.
func TestCopyRoutingMeta_PreservesZaloPersonalQuotePayload(t *testing.T) {
	src := map[string]string{
		"reply_to_message_id":    "9876543210",
		"reply_to_quote_payload": `{"ownerId":"111","msg":"hello"}`,
	}

	got := copyRoutingMeta(src)
	if got["reply_to_quote_payload"] != src["reply_to_quote_payload"] {
		t.Errorf("copyRoutingMeta lost reply_to_quote_payload: got %q", got["reply_to_quote_payload"])
	}

	final := CopyFinalRoutingMeta(src)
	if final["reply_to_quote_payload"] != src["reply_to_quote_payload"] {
		t.Errorf("CopyFinalRoutingMeta lost reply_to_quote_payload: got %q", final["reply_to_quote_payload"])
	}
}

// TestCopyRoutingMeta_PreservesPancakePrivateReplyKeys verifies the metadata
// keys used by the private_reply DM (post_id, display_name, sender_id)
// survive inbound→outbound copy.
func TestCopyRoutingMeta_PreservesPancakePrivateReplyKeys(t *testing.T) {
	src := map[string]string{
		"post_id":      "post-42",
		"display_name": "Tuấn",
		"sender_id":    "user-1",
	}

	got := copyRoutingMeta(src)
	for k, want := range src {
		if got[k] != want {
			t.Fatalf("copyRoutingMeta()[%q] = %q, want %q", k, got[k], want)
		}
	}

	final := CopyFinalRoutingMeta(src)
	for k, want := range src {
		if final[k] != want {
			t.Fatalf("CopyFinalRoutingMeta()[%q] = %q, want %q", k, final[k], want)
		}
	}
}
