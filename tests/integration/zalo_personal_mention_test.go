//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// TestZaloPersonalMention_WireCaptureFixtureByteEquality asserts byte-equality
// between the Go-side computed mentionInfo JSON and the zca-js-captured
// fixture. The fixture is captured one-time via a Node + zca-js script that
// decrypts a real outbound send packet; see plans/260524-1044-zalo-personal-
// mentions/spike-utf16-encoding-result.md for the Path B status.
//
// When the fixture is absent, the test SKIPS — Path B's runtime dogfood gate
// in Phase 5 step 5 remains the load-bearing verification until the fixture
// lands.
func TestZaloPersonalMention_WireCaptureFixtureByteEquality(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "zalo_personal_mention_wire_capture.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("zalo_personal_mention_wire_capture.json fixture not captured yet — see spike-utf16-encoding-result.md (Path B). Skipping byte-equality gate; Phase 5 dogfood is the active verification.")
	}
	// Once the fixture is captured, expand this test:
	//   1. Load fixture JSON: { input, resolverOutputs, expectedMentionInfo }
	//   2. Run pkg/protocol + internal/channels/mentions parser with resolverOutputs.
	//   3. Marshal mentions to wire shape (matches send.go::wireMention).
	//   4. assert bytes.Equal(actual, []byte(fixture.expectedMentionInfo)).
	t.Skip("expand once fixture lands")
}
