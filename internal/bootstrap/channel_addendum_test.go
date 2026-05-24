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

func TestChannelAddendum_NativeStylesOn_IncludesFormattingBlock(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: true})
	if !ok {
		t.Fatal("addendum not found")
	}
	if !strings.Contains(addendum, "## Formatting") {
		t.Error("native ON should include Formatting section")
	}
	if !strings.Contains(addendum, "**bold**") {
		t.Error("native ON should mention **bold** primitive")
	}
	if strings.Contains(addendum, "BEGIN_NATIVE_STYLES") || strings.Contains(addendum, "END_NATIVE_STYLES") {
		t.Error("sentinel markers must be stripped from output")
	}
}

func TestChannelAddendum_NativeStylesOff_StripsFormattingBlock(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal", AddendumOpts{EnableNativeStyles: false})
	if !ok {
		t.Fatal("addendum not found")
	}
	if strings.Contains(addendum, "## Formatting") {
		t.Error("native OFF should NOT include Formatting section")
	}
	if strings.Contains(addendum, "**bold**") {
		t.Error("native OFF should NOT mention **bold** (legacy strip path removes it anyway)")
	}
	if strings.Contains(addendum, "BEGIN_NATIVE_STYLES") || strings.Contains(addendum, "END_NATIVE_STYLES") {
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
