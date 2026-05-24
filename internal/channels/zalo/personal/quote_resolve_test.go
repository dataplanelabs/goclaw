package personal

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func TestQuoteResolve_Precedence(t *testing.T) {
	t.Parallel()
	ptr := func(b bool) *bool { return &b }

	cases := []struct {
		name        string
		old         *bool
		newGroup    *bool
		newDM       *bool
		wantGroup   bool
		wantDM      bool
	}{
		{"all nil → defaults (group=true, dm=false)", nil, nil, nil, true, false},
		{"legacy on → both true (shim preserves)", ptr(true), nil, nil, true, true},
		{"legacy off → both false (shim preserves)", ptr(false), nil, nil, false, false},
		{"group override wins over legacy", ptr(true), ptr(false), nil, false, true},
		{"dm override wins over legacy", ptr(false), nil, ptr(true), false, true},
		{"both new set, legacy ignored", ptr(false), ptr(true), ptr(true), true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Channel{config: config.ZaloPersonalConfig{
				QuoteUserMessage:        tc.old,
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
