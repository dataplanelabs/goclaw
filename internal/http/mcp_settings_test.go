package http

import (
	"encoding/json"
	"testing"
)

// The update handler purges OAuth tokens when the OAuth fingerprint changes, so
// adding or editing inline_tools must not change it.
func TestExtractOAuthFingerprint_IgnoresInlineTools(t *testing.T) {
	base := json.RawMessage(`{"oauth":{"auth_type":"oauth","client_id":"c1","token_endpoint":"https://idp.example/token","grant_type":"client_credentials","scope":"read"}}`)
	withPins := json.RawMessage(`{"inline_tools":["orders_create","customers_search"],"oauth":{"auth_type":"oauth","client_id":"c1","token_endpoint":"https://idp.example/token","grant_type":"client_credentials","scope":"read"}}`)

	if extractOAuthFingerprint(base) != extractOAuthFingerprint(withPins) {
		t.Error("inline_tools must not change the OAuth fingerprint")
	}
	if extractOAuthFingerprint(json.RawMessage(`{"inline_tools":["orders_create"]}`)) != (oauthFingerprint{}) {
		t.Error("a settings object with only inline_tools has an empty fingerprint")
	}
}
