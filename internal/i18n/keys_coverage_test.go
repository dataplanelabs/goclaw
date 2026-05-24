package i18n

import (
	"reflect"
	"testing"
)

// TestStandbyKeysCoverage asserts the new standby keys appear in every locale
// catalog. Missing key = silent runtime fallback (returns key string itself).
func TestStandbyKeysCoverage(t *testing.T) {
	keys := []string{
		StandbyToolDescription,
		StandbyToolParamDuration,
		StandbyToolParamReason,
		StandbyErrorInvalidDuration,
		StandbyErrorNoChannelCtx,
		StandbyEntered,
		StandbyRPCInvalidSchedule,
		StandbyRPCNoPermission,
		TeamCaptureRPCNoPermission,
		TeamCaptureRPCInvalidConfig,
		TeamEvalNotFound,
		TeamEvalJudgeError,
	}
	locales := []string{"en", "vi", "zh"}
	for _, loc := range locales {
		for _, k := range keys {
			got := T(loc, k)
			if got == k {
				t.Errorf("locale %q: key %q falls through to raw key string (translation missing)", loc, k)
			}
			if reflect.TypeOf(got).Kind() != reflect.String {
				t.Errorf("locale %q: key %q returned non-string %T", loc, k, got)
			}
		}
	}
}
