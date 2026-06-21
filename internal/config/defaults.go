package config

// Default agent configuration values.
// These are the single source of truth — all fallback/default logic should reference these
// instead of hardcoding numeric literals.
const (
	DefaultContextWindow       = 200000
	DefaultMaxTokens           = 8192
	DefaultMaxMessageChars     = 32000
	DefaultMaxIterations       = 30
	DefaultTemperature         = 0.7
	DefaultHistoryShare        = 0.85
	DefaultReplayRetentionDays = 7
)

var DefaultCronNoReplyKeywords = []string{
	"send nothing",
	"don't send",
	"do not send",
	"no reply needed",
	"skip reply",
	"no action needed",
	"nothing to do",
	"không gửi gì",
	"không cần gửi",
	"không cần nhắc",
	"không cần làm gì",
	"không còn gì cần làm",
	"khỏi gửi",
	"đừng gửi",
}
