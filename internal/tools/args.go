package tools

import (
	"encoding/json"
	"strconv"
	"strings"
)

// argBool reads a tool arg as a boolean. Missing or wrong-type returns false.
func argBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// argInt64 reads a tool arg as int64. LLMs commonly send IDs as JSON strings,
// so a string branch parses those (returns 0 on parse failure).
func argInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	}
	return 0
}

// argStringSlice reads a tool arg as []string. Non-string entries are skipped.
func argStringSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// argInt64Slice reads a tool arg as []int64. Accepts int/int64/float64 plus
// string-encoded numerics (LLMs often serialize ID arrays as ["1","2"]).
func argInt64Slice(m map[string]any, key string) []int64 {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(raw))
	for _, v := range raw {
		switch n := v.(type) {
		case int64:
			out = append(out, n)
		case int:
			out = append(out, int64(n))
		case float64:
			out = append(out, int64(n))
		case json.Number:
			if i, err := n.Int64(); err == nil {
				out = append(out, i)
			}
		case string:
			if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
				out = append(out, i)
			}
		}
	}
	return out
}
