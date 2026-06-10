package store

import "testing"

func TestDeriveSkillManagedBy(t *testing.T) {
	tests := []struct {
		name string
		info SkillInfo
		want SkillManagedBy
	}{
		{
			name: "gcplane source wins",
			info: SkillInfo{Source: "gcplane", OwnerID: "vanducng"},
			want: SkillManagedBy{Type: "service", ID: "gcplane", Label: "gcplane reconciliation", Source: "gcplane"},
		},
		{
			name: "system skill",
			info: SkillInfo{IsSystem: true, Source: "unknown"},
			want: SkillManagedBy{Type: "system", ID: "system", Label: "System", Source: "unknown"},
		},
		{
			name: "creator agent",
			info: SkillInfo{Source: "cli", CreatorAgent: &SkillAgentRef{ID: "agent-id", AgentKey: "van-anh", DisplayName: "Vân Anh"}},
			want: SkillManagedBy{Type: "agent", ID: "agent-id", Label: "Vân Anh", Source: "cli"},
		},
		{
			name: "owner fallback",
			info: SkillInfo{Source: "unknown", OwnerID: "gcplane"},
			want: SkillManagedBy{Type: "user", ID: "gcplane", Label: "gcplane", Source: "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveSkillManagedBy(tt.info)
			if got == nil {
				t.Fatal("got nil")
			}
			if *got != tt.want {
				t.Fatalf("managed_by mismatch:\n got: %#v\nwant: %#v", *got, tt.want)
			}
		})
	}
}
