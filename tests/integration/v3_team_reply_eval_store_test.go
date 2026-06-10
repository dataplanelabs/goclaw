//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

func seedTeamEvalInstance(t *testing.T, tenantID, agentID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	db := testDB(t)
	id := uuid.New()
	_, err := db.Exec(
		`INSERT INTO channel_instances (id, name, display_name, channel_type, agent_id, enabled, tenant_id)
		 VALUES ($1, $2, $2, 'zalo_oa', $3, true, $4)`,
		id, name, agentID, tenantID)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM team_reply_evaluations WHERE channel_instance_id = $1", id)
		db.Exec("DELETE FROM channel_instances WHERE id = $1", id)
	})
	return id
}

func TestTeamReplyEvalStore_InsertRoundTrip(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-eval-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	now := time.Now().UTC().Truncate(time.Millisecond)
	e := store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:peer1",
		SessionKey:        "zalo_oa:peer1",
		TeamMsgID:         "msg-1001",
		CapturedAt:        now,
		CustomerMessage:   "Hi, where is my order?",
		TeamReply:         "Hi! Order #42 ships today.",
	}
	id, err := s.Insert(ctx, e)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id == "" {
		t.Fatal("expected id")
	}

	got, err := s.GetByMessageID(ctx, instID.String(), "msg-1001")
	if err != nil || got == nil {
		t.Fatalf("GetByMessageID: %v %+v", err, got)
	}
	if got.TeamReply != e.TeamReply || got.CustomerMessage != e.CustomerMessage {
		t.Fatalf("mismatch: %+v", got)
	}
	if got.JudgeCompletedAt != nil {
		t.Fatalf("expected pending judge, got completed: %+v", got)
	}
}

func TestTeamReplyEvalStore_InsertIdempotent(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-idem-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	e := store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:peer2",
		SessionKey:        "zalo_oa:peer2",
		TeamMsgID:         "msg-dup-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "first",
	}
	id1, err := s.Insert(ctx, e)
	if err != nil {
		t.Fatalf("Insert 1: %v", err)
	}
	e.TeamReply = "second-should-not-overwrite"
	id2, err := s.Insert(ctx, e)
	if err != nil {
		t.Fatalf("Insert 2: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("idempotency broken: %s != %s", id1, id2)
	}
	got, _ := s.GetByMessageID(ctx, instID.String(), "msg-dup-1")
	if got.TeamReply != "first" {
		t.Fatalf("Insert 2 overwrote: %q", got.TeamReply)
	}
}

func TestTeamReplyEvalStore_UpdateJudgeVerdict(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-jv-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	id, err := s.Insert(ctx, store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:peer3",
		SessionKey:        "zalo_oa:peer3",
		TeamMsgID:         "msg-jv-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "yes sir",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := s.UpdateJudgeVerdict(ctx, id, "Sure thing!", 0.42, "tone diverges", "claude-sonnet-4-6", "anthropic", "judge-v1", 1234); err != nil {
		t.Fatalf("UpdateJudgeVerdict: %v", err)
	}
	got, _ := s.GetByMessageID(ctx, instID.String(), "msg-jv-1")
	if got.JudgeCompletedAt == nil {
		t.Fatal("judge_completed_at still NULL")
	}
	if got.DiffScore == nil || *got.DiffScore < 0.41 || *got.DiffScore > 0.43 {
		t.Fatalf("diff_score = %+v", got.DiffScore)
	}
	if got.JudgeModel == nil || *got.JudgeModel != "claude-sonnet-4-6" {
		t.Fatalf("judge_model = %+v", got.JudgeModel)
	}
	// Second UpdateJudgeVerdict on already-completed row must fail (idempotency guard).
	if err := s.UpdateJudgeVerdict(ctx, id, "x", 0.1, "x", "x", "x", "x", 1); err == nil {
		t.Fatal("expected error on re-update completed row")
	}
}

func TestTeamReplyEvalStore_MarkJudgeError(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-je-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	id, _ := s.Insert(ctx, store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:peer4",
		SessionKey:        "zalo_oa:peer4",
		TeamMsgID:         "msg-err-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "test",
	})
	if err := s.MarkJudgeError(ctx, id, "judge_agent_unavailable"); err != nil {
		t.Fatalf("MarkJudgeError: %v", err)
	}
	got, _ := s.GetByMessageID(ctx, instID.String(), "msg-err-1")
	if got.JudgeError == nil || *got.JudgeError != "judge_agent_unavailable" {
		t.Fatalf("judge_error = %+v", got.JudgeError)
	}
	if got.JudgeCompletedAt != nil {
		t.Fatal("judge_completed_at must remain NULL on error")
	}
}

func TestTeamReplyEvalStore_ListPendingJudge(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-pl-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	for i, msg := range []string{"p-1", "p-2", "p-3"} {
		_, err := s.Insert(ctx, store.TeamReplyEvaluation{
			ChannelInstanceID: instID.String(),
			TenantID:          tenantID.String(),
			ThreadKey:         "direct:peerX",
			SessionKey:        "zalo_oa:peerX",
			TeamMsgID:         msg,
			CapturedAt:        time.Now().UTC().Add(time.Duration(i) * time.Second),
			TeamReply:         msg,
		})
		if err != nil {
			t.Fatalf("Insert %s: %v", msg, err)
		}
	}
	// Mark one completed + one errored — only the third should appear in pending.
	got1, _ := s.GetByMessageID(ctx, instID.String(), "p-1")
	got2, _ := s.GetByMessageID(ctx, instID.String(), "p-2")
	_ = s.UpdateJudgeVerdict(ctx, got1.ID, "x", 0.5, "x", "m", "p", "k", 100)
	_ = s.MarkJudgeError(ctx, got2.ID, "err")

	pending, err := s.ListPendingJudge(ctx, 50)
	if err != nil {
		t.Fatalf("ListPendingJudge: %v", err)
	}
	// Cross-test seeds may leak; just assert "p-3" is included and the others aren't.
	var sawP3, sawP1, sawP2 bool
	for _, p := range pending {
		switch p.TeamMsgID {
		case "p-1":
			sawP1 = true
		case "p-2":
			sawP2 = true
		case "p-3":
			sawP3 = true
		}
	}
	if !sawP3 || sawP1 || sawP2 {
		t.Fatalf("pending visibility wrong: p1=%v p2=%v p3=%v", sawP1, sawP2, sawP3)
	}
}

func TestTeamReplyEvalStore_ListFilterMaxDiff(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-md-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	for i, sc := range []float64{0.1, 0.4, 0.8} {
		id, _ := s.Insert(ctx, store.TeamReplyEvaluation{
			ChannelInstanceID: instID.String(),
			TenantID:          tenantID.String(),
			ThreadKey:         "direct:peerM",
			SessionKey:        "zalo_oa:peerM",
			TeamMsgID:         "md-" + uuid.NewString()[:6],
			CapturedAt:        time.Now().UTC().Add(time.Duration(i) * time.Second),
			TeamReply:         "x",
		})
		_ = s.UpdateJudgeVerdict(ctx, id, "y", sc, "z", "m", "p", "k", 50)
	}
	thr := 0.5
	rows, err := s.List(ctx, tenantID.String(), store.TeamReplyEvalFilter{
		ChannelInstanceID: instID.String(),
		MaxDiffScore:      &thr,
		Limit:             50,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows ≤ 0.5, got %d", len(rows))
	}
	for _, r := range rows {
		if r.DiffScore == nil || *r.DiffScore > 0.5 {
			t.Fatalf("filter leak: %+v", r.DiffScore)
		}
	}
}

func TestTeamReplyEvalStore_CrossTenantIsolation(t *testing.T) {
	db := testDB(t)
	tenantA, agentA := seedTenantAgent(t, db)
	tenantB, _ := seedTenantAgent(t, db)
	ctxA := tenantCtx(tenantA)
	instA := seedTeamEvalInstance(t, tenantA, agentA, "zalo-oa-iso-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	_, err := s.Insert(ctxA, store.TeamReplyEvaluation{
		ChannelInstanceID: instA.String(),
		TenantID:          tenantA.String(),
		ThreadKey:         "direct:peerZ",
		SessionKey:        "zalo_oa:peerZ",
		TeamMsgID:         "iso-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "tenant A only",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	rowsB, err := s.List(tenantCtx(tenantB), tenantB.String(), store.TeamReplyEvalFilter{Limit: 50})
	if err != nil {
		t.Fatalf("List B: %v", err)
	}
	for _, r := range rowsB {
		if r.ChannelInstanceID == instA.String() {
			t.Fatalf("cross-tenant leak: %+v", r)
		}
	}
}

func TestTeamReplyEvalStore_FKCascade(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTeamEvalInstance(t, tenantID, agentID, "zalo-oa-fk-"+uuid.NewString()[:8])
	s := pg.NewPGTeamReplyEvalStore(db)

	id, err := s.Insert(ctx, store.TeamReplyEvaluation{
		ChannelInstanceID: instID.String(),
		TenantID:          tenantID.String(),
		ThreadKey:         "direct:peerF",
		SessionKey:        "zalo_oa:peerF",
		TeamMsgID:         "fk-1",
		CapturedAt:        time.Now().UTC(),
		TeamReply:         "x",
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := db.Exec("DELETE FROM channel_instances WHERE id = $1", instID); err != nil {
		t.Fatalf("delete instance: %v", err)
	}
	got, _ := s.GetByMessageID(ctx, instID.String(), "fk-1")
	if got != nil {
		t.Fatalf("expected cascade delete, got row id=%s", id)
	}
}
