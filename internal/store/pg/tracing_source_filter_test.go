package pg

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestBuildTraceWhere_SourceTypeFilter(t *testing.T) {
	ctx := store.WithCrossTenant(context.Background()) // skip tenant clause

	cases := []struct {
		srcType  string
		wantSQL  string
		wantArgs []any
	}{
		{"cron", "session_key LIKE $1", []any{"%:cron:%"}},
		{"group", "session_key LIKE $1", []any{"%:group:%"}},
		{"team", "session_key LIKE $1", []any{"%:team:%"}},
		{"direct", "(session_key LIKE $1 AND session_key NOT LIKE $2)", []any{"%:direct:%", "%:ws:%"}},
		{"ws", "(channel = $1 OR session_key LIKE $2)", []any{"ws", "%:ws:%"}},
		{"", "", nil},
		{"bogus", "", nil},
	}

	for _, tc := range cases {
		t.Run(tc.srcType, func(t *testing.T) {
			where, args := buildTraceWhere(ctx, store.TraceListOpts{SourceType: tc.srcType})
			if tc.wantSQL == "" {
				if strings.Contains(where, "session_key") || strings.Contains(where, "channel") {
					t.Fatalf("expected no source filter, got %q", where)
				}
				return
			}
			if !strings.Contains(where, tc.wantSQL) {
				t.Fatalf("where = %q, want fragment %q", where, tc.wantSQL)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i := range tc.wantArgs {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("arg[%d] = %v, want %v", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}
