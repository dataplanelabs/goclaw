package bootstrap

import (
	"strings"
	"testing"
)

func TestChannelAddendum_ZaloPersonal_Returns(t *testing.T) {
	addendum, ok := ChannelAddendum("zalo_personal")
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
	if _, ok := ChannelAddendum("telegram"); ok {
		t.Error("unknown channel should return ok=false")
	}
}

func TestChannelAddendum_EmptyString_NotFound(t *testing.T) {
	if _, ok := ChannelAddendum(""); ok {
		t.Error("empty channelType should return ok=false")
	}
}
