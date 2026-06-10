//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

func TestSystemPromptTimezone_ChannelOverride(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	ciStore := pg.NewPGChannelInstanceStore(db, "")
	ciID := uuid.New()
	name := "tz-test-" + ciID.String()[:8]
	_, err := db.ExecContext(ctx,
		`INSERT INTO channel_instances (id, tenant_id, name, channel_type, agent_id, config, enabled)
		 VALUES ($1, $2, $3, 'telegram', $4, '{"timezone":"Asia/Ho_Chi_Minh"}'::jsonb, true)`,
		ciID, tenantID, name, agentID)
	if err != nil {
		t.Fatalf("insert channel_instance: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM channel_instances WHERE id = $1", ciID)
	})

	inst, err := ciStore.GetByName(ctx, name)
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	channelTZ := channels.TimezoneFromConfig(inst.Config)
	if channelTZ != "Asia/Ho_Chi_Minh" {
		t.Fatalf("TimezoneFromConfig = %q, want Asia/Ho_Chi_Minh", channelTZ)
	}

	resolved := agent.ResolveUserTimezone(channelTZ, "UTC")
	prompt := agent.BuildSystemPrompt(agent.SystemPromptConfig{
		Mode:         agent.PromptFull,
		UserTimezone: resolved,
	})
	if !strings.Contains(prompt, "(Asia/Ho_Chi_Minh)") {
		t.Errorf("prompt missing (Asia/Ho_Chi_Minh); got time section in:\n%s", excerpt(prompt, "Current date:"))
	}
	if strings.Contains(prompt, "(UTC)") {
		t.Errorf("prompt should not contain (UTC) when channel TZ is set; got:\n%s", excerpt(prompt, "Current date:"))
	}
}

func TestSystemPromptTimezone_WorkspaceFallback(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	ciStore := pg.NewPGChannelInstanceStore(db, "")
	ciID := uuid.New()
	name := "tz-fallback-" + ciID.String()[:8]
	_, err := db.ExecContext(ctx,
		`INSERT INTO channel_instances (id, tenant_id, name, channel_type, agent_id, config, enabled)
		 VALUES ($1, $2, $3, 'telegram', $4, '{}'::jsonb, true)`,
		ciID, tenantID, name, agentID)
	if err != nil {
		t.Fatalf("insert channel_instance: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM channel_instances WHERE id = $1", ciID)
	})

	inst, err := ciStore.GetByName(ctx, name)
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	channelTZ := channels.TimezoneFromConfig(inst.Config)
	if channelTZ != "" {
		t.Fatalf("expected empty channel TZ when config has no timezone, got %q", channelTZ)
	}

	resolved := agent.ResolveUserTimezone(channelTZ, "America/New_York")
	prompt := agent.BuildSystemPrompt(agent.SystemPromptConfig{
		Mode:         agent.PromptFull,
		UserTimezone: resolved,
	})
	if !strings.Contains(prompt, "(America/New_York)") {
		t.Errorf("prompt missing (America/New_York) workspace fallback; got:\n%s", excerpt(prompt, "Current date:"))
	}
}

func TestSystemPromptTimezone_BothEmptyFallsToUTC(t *testing.T) {
	prompt := agent.BuildSystemPrompt(agent.SystemPromptConfig{
		Mode:         agent.PromptFull,
		UserTimezone: agent.ResolveUserTimezone("", ""),
	})
	if !strings.Contains(prompt, "(UTC)") {
		t.Errorf("expected (UTC) when no TZ available; got:\n%s", excerpt(prompt, "Current date:"))
	}
}

func excerpt(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return "<marker not found>"
	}
	end := idx + 80
	if end > len(s) {
		end = len(s)
	}
	return s[idx:end]
}
