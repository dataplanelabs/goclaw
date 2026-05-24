package personal

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func TestQuoteResolve_Precedence(t *testing.T) {
	t.Parallel()
	ptr := func(b bool) *bool { return &b }

	cases := []struct {
		name      string
		newGroup  *bool
		newDM     *bool
		wantGroup bool
		wantDM    bool
	}{
		{"both nil → defaults (group=true, dm=false)", nil, nil, true, false},
		{"group=false explicit", ptr(false), nil, false, false},
		{"dm=true explicit", nil, ptr(true), true, true},
		{"both explicit", ptr(true), ptr(true), true, true},
		{"both false explicit", ptr(false), ptr(false), false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Channel{config: config.ZaloPersonalConfig{
				QuoteUserMessageInGroup: tc.newGroup,
				QuoteUserMessageInDM:    tc.newDM,
			}}
			if got := c.quoteInGroup(); got != tc.wantGroup {
				t.Errorf("quoteInGroup = %v, want %v", got, tc.wantGroup)
			}
			if got := c.quoteInDM(); got != tc.wantDM {
				t.Errorf("quoteInDM = %v, want %v", got, tc.wantDM)
			}
		})
	}
}

func TestQuoteResolve_QuoteInboundOnDMRoutesToDM(t *testing.T) {
	t.Parallel()
	ptr := func(b bool) *bool { return &b }
	c := &Channel{config: config.ZaloPersonalConfig{QuoteUserMessageInDM: ptr(true)}}
	if !c.QuoteInboundOnDM() {
		t.Fatal("QuoteInboundOnDM must reflect quoteInDM result")
	}
}
