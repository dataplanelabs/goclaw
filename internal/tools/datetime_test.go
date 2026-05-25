package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDateTimeTool_IncludesWeekdayAndHuman(t *testing.T) {
	tool := NewDateTimeTool()
	res := tool.Execute(context.Background(), map[string]any{"timezone": "Asia/Ho_Chi_Minh"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.ForLLM), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"utc", "unix_ms", "local", "timezone", "weekday", "human"} {
		if _, ok := out[k]; !ok {
			t.Errorf("missing key %q in %v", k, out)
		}
	}
	weekday := out["weekday"].(string)
	if !isValidWeekday(weekday) {
		t.Errorf("weekday %q not a valid Go weekday name", weekday)
	}
	human := out["human"].(string)
	if !strings.Contains(human, weekday) {
		t.Errorf("human %q should contain weekday %q", human, weekday)
	}
}

func TestDateTimeTool_UTCFallbackHasWeekday(t *testing.T) {
	tool := NewDateTimeTool()
	res := tool.Execute(context.Background(), map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.ForLLM), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := out["weekday"]; !ok {
		t.Error("UTC-only response should still include weekday")
	}
	if _, ok := out["human"]; !ok {
		t.Error("UTC-only response should still include human")
	}
}

func TestDateTimeTool_InvalidTimezone(t *testing.T) {
	tool := NewDateTimeTool()
	res := tool.Execute(context.Background(), map[string]any{"timezone": "Bogus/Invalid"})
	if !res.IsError {
		t.Error("expected error for invalid timezone")
	}
}

func isValidWeekday(s string) bool {
	switch s {
	case "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday":
		return true
	}
	return false
}
