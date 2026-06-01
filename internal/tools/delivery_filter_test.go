package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsScratchDeliveryPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "tmp prefix file", path: "_tmp_render_report.py", want: true},
		{name: "tmp dash file", path: "tmp-report.json", want: true},
		{name: "staging segment", path: "generated/staging/report.png", want: true},
		{name: "tmp segment in workspace", path: "reports/tmp/chart.png", want: true},
		{name: "final generated artifact", path: "generated/2026-06-01/report.png", want: false},
		{name: "source file explicitly named", path: "scripts/report.py", want: false},
		{name: "final artifact under system temp", path: filepath.Join(os.TempDir(), "report.png"), want: false},
		{name: "scratch artifact under system temp", path: filepath.Join(os.TempDir(), "_tmp_report.py"), want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsScratchDeliveryPath(tc.path); got != tc.want {
				t.Fatalf("IsScratchDeliveryPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
