package channels

import "encoding/json"

// TimezoneFromConfig extracts the optional "timezone" string from a
// channel-instance config JSONB blob. Returns "" if absent or unparsable.
func TimezoneFromConfig(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var c struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return ""
	}
	return c.Timezone
}
