//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

func seedTestChannelInstance(t *testing.T, tenantID, agentID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	db := testDB(t)
	id := uuid.New()
	_, err := db.Exec(
		`INSERT INTO channel_instances (id, name, display_name, channel_type, agent_id, enabled, tenant_id)
		 VALUES ($1, $2, $2, 'telegram', $3, true, $4)`,
		id, name, agentID, tenantID)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM channel_thread_schedules WHERE channel_instance_id = $1", id)
		db.Exec("DELETE FROM channel_instances WHERE id = $1", id)
	})
	return id
}

func TestChannelScheduleStore_InstanceRoundTrip(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTestChannelInstance(t, tenantID, agentID, "telegram-rt-"+uuid.NewString()[:8])
	s := pg.NewPGChannelScheduleStore(db)

	got, err := s.GetInstanceSchedule(ctx, instID.String())
	if err != nil {
		t.Fatalf("Get nil: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil schedule on fresh row, got %+v", got)
	}

	sc := &schedule.Schedule{
		DefaultMode: schedule.ModeActive,
		Windows: []schedule.Window{{
			Mode: schedule.ModeStandby, TZ: "Asia/Saigon",
			Weekday: "mon-fri", Start: "09:00", End: "17:00",
		}},
	}
	if err := s.SetInstanceSchedule(ctx, instID.String(), sc); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = s.GetInstanceSchedule(ctx, instID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.DefaultMode != schedule.ModeActive || len(got.Windows) != 1 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	if err := s.DeleteInstanceSchedule(ctx, instID.String()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = s.GetInstanceSchedule(ctx, instID.String())
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

func TestChannelScheduleStore_ThreadRoundTrip(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTestChannelInstance(t, tenantID, agentID, "telegram-thr-"+uuid.NewString()[:8])
	s := pg.NewPGChannelScheduleStore(db)

	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	thread := store.ThreadSchedule{
		ChannelInstanceID: instID.String(),
		ThreadKey:         "direct:peer42",
		Schedule: &schedule.Schedule{DefaultMode: schedule.ModeStandby, Windows: []schedule.Window{
			{Mode: schedule.ModeStandby, From: ptrTime(time.Now()), Until: ptrTime(expires)},
		}},
		ExpiresAt: &expires,
		Reason:    "lunch",
		CreatedBy: "agent",
	}
	if err := s.SetThreadSchedule(ctx, thread); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.GetThreadSchedule(ctx, instID.String(), "direct:peer42")
	if err != nil || got == nil {
		t.Fatalf("Get: %v %+v", err, got)
	}
	if got.Reason != "lunch" || got.Schedule.DefaultMode != schedule.ModeStandby {
		t.Fatalf("mismatch: %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("expires_at: %v vs %v", got.ExpiresAt, expires)
	}

	list, err := s.ListThreadSchedules(ctx, instID.String())
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}

	if err := s.DeleteThreadSchedule(ctx, instID.String(), "direct:peer42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = s.GetThreadSchedule(ctx, instID.String(), "direct:peer42")
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

func TestChannelScheduleStore_PurgeExpired(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	instID := seedTestChannelInstance(t, tenantID, agentID, "telegram-purge-"+uuid.NewString()[:8])
	s := pg.NewPGChannelScheduleStore(db)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	mustSet := func(key string, exp *time.Time) {
		if err := s.SetThreadSchedule(ctx, store.ThreadSchedule{
			ChannelInstanceID: instID.String(), ThreadKey: key,
			Schedule:  &schedule.Schedule{DefaultMode: schedule.ModeStandby},
			ExpiresAt: exp,
		}); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	mustSet("expired", &past)
	mustSet("future", &future)
	mustSet("permanent", nil)

	n, err := s.PurgeExpiredThreadSchedules(ctx, time.Now())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row purged, got %d", n)
	}
	list, _ := s.ListThreadSchedules(ctx, instID.String())
	if len(list) != 2 {
		t.Fatalf("expected 2 remaining (future, permanent), got %d", len(list))
	}
}

func TestChannelScheduleStore_ResolveInstanceIDByName(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	tenant2ID, _ := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	name := "tg-resolve-" + uuid.NewString()[:8]
	instID := seedTestChannelInstance(t, tenantID, agentID, name)
	s := pg.NewPGChannelScheduleStore(db)

	id, err := s.ResolveInstanceIDByName(ctx, tenantID.String(), name)
	if err != nil {
		t.Fatalf("happy: %v", err)
	}
	if id != instID.String() {
		t.Fatalf("expected %s, got %s", instID, id)
	}

	id2, err := s.ResolveInstanceIDByName(ctx, tenantID.String(), "doesnotexist")
	if err != nil || id2 != "" {
		t.Fatalf("missing: want (\"\",nil), got (%q,%v)", id2, err)
	}

	id3, err := s.ResolveInstanceIDByName(ctx, tenant2ID.String(), name)
	if err != nil || id3 != "" {
		t.Fatalf("cross-tenant leak: want (\"\",nil), got (%q,%v)", id3, err)
	}
}

func TestChannelScheduleStore_FKCascade(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	name := "tg-fk-" + uuid.NewString()[:8]
	instID := seedTestChannelInstance(t, tenantID, agentID, name)
	s := pg.NewPGChannelScheduleStore(db)

	if err := s.SetThreadSchedule(ctx, store.ThreadSchedule{
		ChannelInstanceID: instID.String(),
		ThreadKey:         "group:42",
		Schedule:          &schedule.Schedule{DefaultMode: schedule.ModeStandby},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := db.Exec("DELETE FROM channel_instances WHERE id = $1", instID); err != nil {
		t.Fatalf("delete instance: %v", err)
	}
	got, _ := s.GetThreadSchedule(ctx, instID.String(), "group:42")
	if got != nil {
		t.Fatalf("FK cascade failed: thread schedule survived: %+v", got)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
