package bootstrap

import (
	"strings"
	"testing"
)

func TestChannelAddendum_ZaloPersonal_Returns(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: true})
	if !ok {
		t.Fatal("expected zalo_personal addendum to be registered")
	}
	if !strings.Contains(addendum, "@[<uid>]") {
		t.Errorf("addendum missing marker syntax example; got:\n%s", addendum)
	}
	if !strings.Contains(addendum, "@[all]") {
		t.Errorf("addendum missing @[all] guidance")
	}
}

func TestChannelAddendum_UnknownChannel_NotFound(t *testing.T) {
	if _, ok := ChannelAddendum("telegram", AddendumOpts{}); ok {
		t.Error("unknown channel should return ok=false")
	}
}

func TestChannelAddendum_EmptyString_NotFound(t *testing.T) {
	if _, ok := ChannelAddendum("", AddendumOpts{}); ok {
		t.Error("empty channelType should return ok=false")
	}
}

func TestChannelAddendum_NativeStylesOn_IncludesNativeBlockOnly(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: true})
	if !ok {
		t.Fatal("addendum not found")
	}
	if !strings.Contains(addendum, "Formatting (native-styles mode)") {
		t.Error("native ON should include native-styles Formatting section")
	}
	if !strings.Contains(addendum, "**bold**") {
		t.Error("native ON should mention **bold** primitive")
	}
	if strings.Contains(addendum, "Formatting (plain-text mode)") {
		t.Error("native ON must NOT include plain-text guidance")
	}
	if strings.Contains(addendum, "BEGIN_NATIVE_STYLES") || strings.Contains(addendum, "END_NATIVE_STYLES") {
		t.Error("sentinel markers must be stripped from output")
	}
	if strings.Contains(addendum, "BEGIN_PLAIN_TEXT") {
		t.Error("plain-text sentinels must be stripped along with body")
	}
}

func TestChannelAddendum_NativeStylesOff_IncludesPlainTextBlockOnly(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: false})
	if !ok {
		t.Fatal("addendum not found")
	}
	if !strings.Contains(addendum, "Formatting (plain-text mode)") {
		t.Error("native OFF should include plain-text guidance so LLM stops emitting markdown")
	}
	if !strings.Contains(addendum, "STRIPPED") {
		t.Error("plain-text block should warn that markdown is stripped")
	}
	if strings.Contains(addendum, "Formatting (native-styles mode)") {
		t.Error("native OFF must NOT include native-styles guidance")
	}
	if strings.Contains(addendum, "BEGIN_NATIVE_STYLES") || strings.Contains(addendum, "BEGIN_PLAIN_TEXT") {
		t.Error("sentinel markers must be stripped")
	}
	// Other guidance (mentions, reminders) must survive the strip.
	if !strings.Contains(addendum, "@[<uid>]") {
		t.Error("native OFF still needs mention guidance")
	}
	if !strings.Contains(addendum, "zalo_personal_create_reminder") {
		t.Error("native OFF still needs reminder tool guidance")
	}
}

func TestChannelAddendum_NoLeakAcrossModes(t *testing.T) {
	on, _ := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: true})
	off, _ := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: false})
	if strings.Contains(on, "plain-text mode") {
		t.Error("native ON leaked plain-text content")
	}
	if strings.Contains(off, "native-styles mode") {
		t.Error("native OFF leaked native-styles content")
	}
}
