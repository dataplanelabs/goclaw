package tools

import "testing"

func TestBuildAddPatch(t *testing.T) {
	t.Run("empty input yields no patch", func(t *testing.T) {
		_, dirty := buildAddPatch(map[string]any{"name": "x"})
		if dirty {
			t.Fatal("expected dirty=false for no patchable fields")
		}
	})

	t.Run("injectTargetHistory=false overrides the true default", func(t *testing.T) {
		// JSON numbers decode to float64 in map[string]any.
		p, dirty := buildAddPatch(map[string]any{
			"injectTargetHistory":      false,
			"injectTargetHistoryLimit": float64(20),
			"stateless":                true,
		})
		if !dirty {
			t.Fatal("expected dirty=true")
		}
		if p.InjectTargetHistory == nil || *p.InjectTargetHistory != false {
			t.Errorf("InjectTargetHistory: want false, got %v", p.InjectTargetHistory)
		}
		if p.InjectTargetHistoryLimit == nil || *p.InjectTargetHistoryLimit != 20 {
			t.Errorf("InjectTargetHistoryLimit: want 20, got %v", p.InjectTargetHistoryLimit)
		}
		if p.Stateless == nil || *p.Stateless != true {
			t.Errorf("Stateless: want true, got %v", p.Stateless)
		}
	})

	t.Run("wake_heartbeat only patches when true", func(t *testing.T) {
		if _, dirty := buildAddPatch(map[string]any{"wake_heartbeat": false}); dirty {
			t.Error("wake_heartbeat=false should not produce a patch")
		}
		p, dirty := buildAddPatch(map[string]any{"wake_heartbeat": true})
		if !dirty || p.WakeHeartbeat == nil || *p.WakeHeartbeat != true {
			t.Errorf("wake_heartbeat=true: want patch with true, got dirty=%v %v", dirty, p.WakeHeartbeat)
		}
	})
}
