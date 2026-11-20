package mcp

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"
)

func pinTools(m *Manager, server string, names ...string) {
	if m.inlinePins == nil {
		m.inlinePins = map[string]map[string]struct{}{}
	}
	m.inlinePins[server] = toSet(names)
}

func TestMaybeEnterSearchMode_PinnedToolStaysInline(t *testing.T) {
	perServer := syntheticTools([]string{"alpha", "bravo", "charlie"}, 20)
	perServer["charlie"] = append(perServer["charlie"][:19], "orders_create") // sorts past position 40
	rng := rand.New(rand.NewSource(3))

	for i := 0; i < 30; i++ {
		m := newManagerWithTools(t, rng, perServer)
		pinTools(m, "charlie", "orders_create")
		m.maybeEnterSearchMode()

		inline, deferred := inlineAndDeferred(m)
		if len(inline) != mcpToolInlineMaxCount || len(deferred) != 20 {
			t.Fatalf("pins count toward the threshold: inline=%d deferred=%d, want 40 and 20", len(inline), len(deferred))
		}
		if !slices.Contains(inline, "mcp_charlie__orders_create") {
			t.Fatalf("pinned tool must be inline, inline=%v", inline)
		}
		if slices.Contains(deferred, "mcp_charlie__orders_create") {
			t.Fatal("pinned tool must not be deferred")
		}
		for _, bt := range m.DeferredToolInfos() {
			if bt.OriginalName() == "orders_create" {
				t.Fatal("pinned tool must not reach the search index")
			}
		}
		if _, ok := m.registry.Get("mcp_charlie__orders_create"); !ok {
			t.Fatal("pinned tool must stay registered")
		}
	}
}

func TestMaybeEnterSearchMode_PinsBeyondThresholdAllStay(t *testing.T) {
	perServer := syntheticTools([]string{"alpha", "bravo"}, 30)
	m := newManagerWithTools(t, rand.New(rand.NewSource(4)), perServer)

	pinTools(m, "bravo", perServer["bravo"]...)
	pinTools(m, "alpha", perServer["alpha"][:15]...)
	m.maybeEnterSearchMode()

	inline, deferred := inlineAndDeferred(m)
	if len(inline) != 45 || len(deferred) != 15 {
		t.Fatalf("45 pins: inline=%d deferred=%d, want 45 and 15", len(inline), len(deferred))
	}
	for _, name := range deferred {
		if name == "mcp_bravo__tool_00" || name == "mcp_alpha__tool_00" {
			t.Errorf("pinned %s must stay inline", name)
		}
	}
}

func TestMaybeEnterSearchMode_UnknownPinIsIgnored(t *testing.T) {
	perServer := syntheticTools([]string{"alpha", "bravo", "charlie"}, 20)
	m := newManagerWithTools(t, rand.New(rand.NewSource(5)), perServer)
	pinTools(m, "charlie", "no_such_tool")
	pinTools(m, "ghost", "tool_00")
	m.maybeEnterSearchMode()

	inline, deferred := inlineAndDeferred(m)
	if len(inline) != mcpToolInlineMaxCount || len(deferred) != 20 {
		t.Fatalf("unknown pins must change nothing: inline=%d deferred=%d", len(inline), len(deferred))
	}
}

func TestInlineTools_ParsesAndLeavesOAuthSettingsAlone(t *testing.T) {
	settings := json.RawMessage(`{"inline_tools":["crm_sales_orders_create","crm_customers_search"],"require_user_credentials":true,"oauth":{"auth_type":"oauth"}}`)
	if got := InlineTools(settings); !slices.Equal(got, []string{"crm_sales_orders_create", "crm_customers_search"}) {
		t.Errorf("InlineTools = %v", got)
	}
	if !IsOAuthActive(settings) || !requireUserCreds(settings) {
		t.Error("inline_tools must not disturb the OAuth and credential settings")
	}

	plain := json.RawMessage(`{"inline_tools":["x"]}`)
	if IsOAuthActive(plain) || requireUserCreds(plain) {
		t.Error("a settings object with only inline_tools is neither OAuth nor per-user")
	}

	for _, bad := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`not json`), json.RawMessage(`{"inline_tools":"x"}`)} {
		if got := InlineTools(bad); got != nil {
			t.Errorf("InlineTools(%s) = %v, want nil", bad, got)
		}
	}
}
