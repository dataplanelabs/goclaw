//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

func TestTeamReplyAtomicWriter_NewWriteAppendsToSession(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-atomic-"+uuid.NewString()[:8])
	sessions := pg.NewPGSessionStore(db)
	w := pg.NewPGTeamReplyAtomicWriter(db, sessions)

	sessionKey := "zalo_oa:peer-atomic-1"
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (session_key, tenant_id, messages, agent_id)
		VALUES ($1, $2, '[]'::jsonb, $3)`, sessionKey, tenantID, agentID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM sessions WHERE session_key = $1", sessionKey)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	e := store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:atomic-1",
		SessionKey:        sessionKey,
		TeamMsgID:         "atomic-msg-1",
		CapturedAt:        now,
		TeamReply:         "hi from team",
	}
	msg := providers.Message{
		Role:    "assistant",
		Content: "hi from team",
		Metadata: map[string]any{"source": providers.MessageSourceTeam, "team_msg_id": "atomic-msg-1"},
	}
	id, wasNew, err := w.WriteTeamReplyAtomic(ctx, e, sessionKey, msg)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if id == "" || !wasNew {
		t.Fatalf("expected new write, got id=%s wasNew=%v", id, wasNew)
	}

	var msgCount int
	if err := db.QueryRowContext(ctx, `SELECT jsonb_array_length(messages) FROM sessions WHERE session_key = $1`, sessionKey).Scan(&msgCount); err != nil {
		t.Fatalf("count session messages: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("session message count = %d, want 1", msgCount)
	}
}

func TestTeamReplyAtomicWriter_RetrySkipsSessionAppend(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-retry-"+uuid.NewString()[:8])
	sessions := pg.NewPGSessionStore(db)
	w := pg.NewPGTeamReplyAtomicWriter(db, sessions)

	sessionKey := "zalo_oa:peer-retry-1"
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (session_key, tenant_id, messages, agent_id)
		VALUES ($1, $2, '[]'::jsonb, $3)`, sessionKey, tenantID, agentID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM sessions WHERE session_key = $1", sessionKey)
	})

	e := store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:retry-1",
		SessionKey:        sessionKey,
		TeamMsgID:         "retry-msg-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "first attempt",
	}
	msg := providers.Message{Role: "assistant", Content: "first attempt"}

	id1, wasNew1, err := w.WriteTeamReplyAtomic(ctx, e, sessionKey, msg)
	if err != nil || !wasNew1 {
		t.Fatalf("first write: id=%s new=%v err=%v", id1, wasNew1, err)
	}
	id2, wasNew2, err := w.WriteTeamReplyAtomic(ctx, e, sessionKey, msg)
	if err != nil {
		t.Fatalf("retry write: %v", err)
	}
	if wasNew2 {
		t.Fatal("retry should NOT report wasNew=true; would re-emit event + duplicate downstream work")
	}
	if id1 != id2 {
		t.Fatalf("retry returned different id: %s vs %s", id1, id2)
	}

	var msgCount int
	if err := db.QueryRowContext(ctx, `SELECT jsonb_array_length(messages) FROM sessions WHERE session_key = $1`, sessionKey).Scan(&msgCount); err != nil {
		t.Fatalf("count session messages: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("retry duplicated session message: count = %d, want 1", msgCount)
	}
}
