package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// pgAutoInjector implements AutoInjector backed by EpisodicStore + FTS search.
type pgAutoInjector struct {
	episodicStore store.EpisodicStore
	metricsStore  store.EvolutionMetricsStore // nil = metrics disabled
}

// NewAutoInjector creates an AutoInjector backed by episodic store search.
func NewAutoInjector(es store.EpisodicStore, ms store.EvolutionMetricsStore) AutoInjector {
	return &pgAutoInjector{episodicStore: es, metricsStore: ms}
}

// Inject builds the per-turn auto-inject section. Reaction feedback fires
// regardless of message triviality — even on "ok" we want the agent to see
// that the user reacted angrily on the previous reply. The semantic memory
// recall (FTS+vector) keeps its triviality gate because greetings don't carry
// search intent for past episodes.
func (a *pgAutoInjector) Inject(ctx context.Context, params InjectParams) (*InjectResult, error) {
	if a.episodicStore == nil {
		return &InjectResult{}, nil
	}

	feedbackSection := a.reactionFeedbackSection(ctx, params)

	if isTrivialMessage(params.UserMessage) {
		if feedbackSection == "" {
			return &InjectResult{}, nil
		}
		return &InjectResult{Section: feedbackSection}, nil
	}

	maxEntries := params.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 5
	}
	threshold := params.Threshold
	if threshold <= 0 {
		threshold = 0.3
	}

	searchQuery := buildRecallQuery(params.UserMessage, params.RecentContext)

	results, err := a.episodicStore.Search(ctx, searchQuery, params.AgentID, params.UserID,
		store.EpisodicSearchOptions{
			MaxResults:   maxEntries * 2,
			MinScore:     threshold,
			VectorWeight: 0.3,
			TextWeight:   0.7,
		})
	if err != nil {
		return nil, fmt.Errorf("auto-inject search: %w", err)
	}

	var sb strings.Builder
	injected := 0
	var topScore float64

	if len(results) > 0 {
		sb.WriteString("## Memory Context\n\nRelevant memories from past sessions (use memory_search for details):\n")
		for _, r := range results {
			if injected >= maxEntries {
				break
			}
			if r.L0Abstract == "" {
				continue
			}
			sb.WriteString("- ")
			sb.WriteString(r.L0Abstract)
			sb.WriteString("\n")
			injected++
			if r.Score > topScore {
				topScore = r.Score
			}
		}
	}

	if feedbackSection != "" {
		if injected > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(feedbackSection)
	}

	if sb.Len() == 0 {
		return &InjectResult{MatchCount: len(results)}, nil
	}

	result := &InjectResult{
		Section:    sb.String(),
		MatchCount: len(results),
		Injected:   injected,
		TopScore:   topScore,
	}
	a.recordRetrievalMetric(params, result)
	return result, nil
}

const (
	reactionFeedbackLookback   = 24 * time.Hour
	reactionFeedbackFetchLimit = 100
	reactionFeedbackTopReplies = 5
	reactionFeedbackPreviewMax = 80
)

func (a *pgAutoInjector) reactionFeedbackSection(ctx context.Context, params InjectParams) string {
	rows, err := a.episodicStore.ListBySourceType(ctx, params.AgentID, params.UserID, "reaction_feedback", time.Now().Add(-reactionFeedbackLookback), reactionFeedbackFetchLimit)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return buildReactionFeedbackSection(rows, time.Now(), params)
}

type reactionMsgAgg struct {
	msgID    string
	preview  string
	emojis   map[string]int
	reactors map[string]struct{}
	latest   time.Time
}

func buildReactionFeedbackSection(rows []store.EpisodicSummary, now time.Time, params InjectParams) string {
	byMsg := map[string]*reactionMsgAgg{}
	var posC, negC, surC, unkC int
	allReactors := map[string]struct{}{}

	for _, r := range rows {
		msgID := parseReactionSourceID(r.SourceID)
		if msgID == "" {
			continue
		}
		switch extractSentiment(r.Summary) {
		case "positive":
			posC++
		case "negative":
			negC++
		case "surprise":
			surC++
		default:
			unkC++
		}
		agg, ok := byMsg[msgID]
		if !ok {
			agg = &reactionMsgAgg{
				msgID:    msgID,
				preview:  extractReactionPreview(r.Summary),
				emojis:   map[string]int{},
				reactors: map[string]struct{}{},
			}
			byMsg[msgID] = agg
		}
		if icon := extractReactionIcon(r.Summary); icon != "" {
			agg.emojis[icon]++
		}
		if reactor := extractReactionReactor(r.Summary); reactor != "" {
			agg.reactors[reactor] = struct{}{}
			allReactors[reactor] = struct{}{}
		}
		if r.CreatedAt.After(agg.latest) {
			agg.latest = r.CreatedAt
		}
	}

	if len(byMsg) == 0 {
		return ""
	}

	sortedMsgs := make([]*reactionMsgAgg, 0, len(byMsg))
	for _, m := range byMsg {
		sortedMsgs = append(sortedMsgs, m)
	}
	sort.Slice(sortedMsgs, func(i, j int) bool {
		return sortedMsgs[i].latest.After(sortedMsgs[j].latest)
	})
	if len(sortedMsgs) > reactionFeedbackTopReplies {
		sortedMsgs = sortedMsgs[:reactionFeedbackTopReplies]
	}

	var sb strings.Builder
	sb.WriteString("## Recent User Reactions (last 24h)\n")
	fmt.Fprintf(&sb, "Stats: %d positive · %d negative · %d surprise · across %d replies (%d reactors)\n\n",
		posC, negC, surC, len(byMsg), len(allReactors))
	sb.WriteString("Most reacted-to replies (most recent first):\n")
	for _, m := range sortedMsgs {
		sb.WriteString("- ")
		sb.WriteString(relTimeAgo(now.Sub(m.latest)))
		sb.WriteString(" · ")
		sb.WriteString(formatEmojiCluster(m.emojis))
		if m.preview != "" {
			sb.WriteString(" on ")
			sb.WriteString(strconv.Quote(truncateRunes(m.preview, reactionFeedbackPreviewMax)))
		} else {
			sb.WriteString(" on message ")
			sb.WriteString(m.msgID)
		}
		sb.WriteString("\n")
	}

	slog.Info("memory.reaction_feedback.injected",
		"agent_id", params.AgentID,
		"user_id", params.UserID,
		"rows_total", len(rows),
		"unique_replies", len(byMsg),
		"reactors", len(allReactors),
	)
	return sb.String()
}

var (
	reactionSentimentRe = regexp.MustCompile(`\(([a-z]+)\)`)
	reactionIconRe      = regexp.MustCompile(`reacted (\S+?) \(`)
	reactionReactorRe   = regexp.MustCompile(`^(.+?) reacted `)
)

func parseReactionSourceID(sourceID string) string {
	const prefix = "react:"
	if !strings.HasPrefix(sourceID, prefix) {
		return ""
	}
	rest := sourceID[len(prefix):]
	if i := strings.IndexByte(rest, ':'); i > 0 {
		return rest[:i]
	}
	return rest
}

func extractSentiment(summary string) string {
	if m := reactionSentimentRe.FindStringSubmatch(summary); len(m) == 2 {
		return m[1]
	}
	return ""
}

func extractReactionIcon(summary string) string {
	if m := reactionIconRe.FindStringSubmatch(summary); len(m) == 2 {
		return m[1]
	}
	return ""
}

func extractReactionReactor(summary string) string {
	if m := reactionReactorRe.FindStringSubmatch(summary); len(m) == 2 {
		return m[1]
	}
	return ""
}

func extractReactionPreview(summary string) string {
	const sep = `on your reply: "`
	idx := strings.Index(summary, sep)
	if idx < 0 {
		return ""
	}
	rest := summary[idx+len(sep):]
	if end := strings.LastIndex(rest, `"`); end >= 0 {
		return rest[:end]
	}
	return rest
}

func formatEmojiCluster(emojis map[string]int) string {
	type kv struct {
		k string
		v int
	}
	items := make([]kv, 0, len(emojis))
	for k, v := range emojis {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v != items[j].v {
			return items[i].v > items[j].v
		}
		return items[i].k < items[j].k
	})
	var sb strings.Builder
	for i, it := range items {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(it.k)
		if it.v > 1 {
			fmt.Fprintf(&sb, "×%d", it.v)
		}
	}
	return sb.String()
}

func relTimeAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	rs := []rune(s)
	return string(rs[:max]) + "…"
}

// recordRetrievalMetric records an auto-inject retrieval metric in a background goroutine.
func (a *pgAutoInjector) recordRetrievalMetric(params InjectParams, result *InjectResult) {
	if a.metricsStore == nil || params.TenantID == "" {
		return
	}
	tenantID, err := uuid.Parse(params.TenantID)
	if err != nil {
		return
	}
	agentID, err := uuid.Parse(params.AgentID)
	if err != nil {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(store.WithTenantID(context.Background(), tenantID), 5*time.Second)
		defer cancel()
		value, _ := json.Marshal(map[string]any{
			"result_count":  result.MatchCount,
			"injected":      result.Injected,
			"top_score":     result.TopScore,
			"used_in_reply": result.Injected > 0,
		})
		if err := a.metricsStore.RecordMetric(bgCtx, store.EvolutionMetric{
			ID:         uuid.New(),
			TenantID:   tenantID,
			AgentID:    agentID,
			MetricType: store.MetricRetrieval,
			MetricKey:  "auto_inject",
			Value:      value,
		}); err != nil {
			slog.Debug("evolution.metric.auto_inject_failed", "error", err)
		}
	}()
}
