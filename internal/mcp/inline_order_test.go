package mcp

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// newManagerWithTools registers tools for each server in a random order, the way
// map iteration and connection timing can vary between starts.
func newManagerWithTools(t *testing.T, rng *rand.Rand, perServer map[string][]string) *Manager {
	t.Helper()
	reg := tools.NewRegistry()
	m := NewManager(reg)

	servers := make([]string, 0, len(perServer))
	for s := range perServer {
		servers = append(servers, s)
	}
	rng.Shuffle(len(servers), func(i, j int) { servers[i], servers[j] = servers[j], servers[i] })

	for _, server := range servers {
		origs := slices.Clone(perServer[server])
		rng.Shuffle(len(origs), func(i, j int) { origs[i], origs[j] = origs[j], origs[i] })
		ss := &serverState{name: server}
		for _, orig := range origs {
			bt := makeBridgeTool(server, orig)
			reg.Register(bt)
			ss.toolNames = append(ss.toolNames, bt.Name())
		}
		m.servers[server] = ss
	}
	return m
}

func syntheticTools(servers []string, perServer int) map[string][]string {
	out := make(map[string][]string, len(servers))
	for _, s := range servers {
		for i := 0; i < perServer; i++ {
			out[s] = append(out[s], fmt.Sprintf("tool_%02d", i))
		}
	}
	return out
}

func inlineAndDeferred(m *Manager) (inline, deferred []string) {
	inline = m.ToolNames()
	for _, bt := range m.DeferredToolInfos() {
		deferred = append(deferred, bt.Name())
	}
	sort.Strings(deferred)
	return inline, deferred
}

func TestMaybeEnterSearchMode_InlineSetIsStableAcrossRestarts(t *testing.T) {
	perServer := syntheticTools([]string{"alpha", "bravo", "charlie"}, 20)
	rng := rand.New(rand.NewSource(1))

	var wantInline, wantDeferred []string
	for i := 0; i < 60; i++ {
		m := newManagerWithTools(t, rng, perServer)
		m.maybeEnterSearchMode()
		if !m.IsSearchMode() {
			t.Fatal("60 tools must enter search mode")
		}
		inline, deferred := inlineAndDeferred(m)
		if len(inline) != mcpToolInlineMaxCount || len(deferred) != 20 {
			t.Fatalf("run %d: inline=%d deferred=%d, want 40 and 20", i, len(inline), len(deferred))
		}
		if i == 0 {
			wantInline, wantDeferred = inline, deferred
			continue
		}
		if !slices.Equal(inline, wantInline) || !slices.Equal(deferred, wantDeferred) {
			t.Fatalf("run %d: inline/deferred split differs from run 0", i)
		}
	}

	for _, name := range wantInline {
		if strings.HasPrefix(name, "mcp_charlie__") {
			t.Errorf("charlie sorts last, %s must be deferred", name)
		}
	}
}

func TestMaybeEnterSearchMode_UnderThresholdIsUnchanged(t *testing.T) {
	m := newManagerWithTools(t, rand.New(rand.NewSource(2)), syntheticTools([]string{"alpha", "bravo"}, 20))
	m.maybeEnterSearchMode()
	if m.IsSearchMode() || len(m.DeferredToolInfos()) != 0 || len(m.ToolNames()) != 40 {
		t.Errorf("40 tools or fewer must stay fully inline")
	}
}
