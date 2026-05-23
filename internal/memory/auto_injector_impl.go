package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

const reactionFeedbackLookback = 24 * time.Hour
const reactionFeedbackLimit = 5

func (a *pgAutoInjector) reactionFeedbackSection(ctx context.Context, params InjectParams) string {
	rows, err := a.episodicStore.ListBySourceType(ctx, params.AgentID, params.UserID, "reaction_feedback", time.Now().Add(-reactionFeedbackLookback), reactionFeedbackLimit)
	if err != nil || len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Recent User Reactions\n\nReactions on your prior replies (use to calibrate tone — don't reply directly to reactions):\n")
	for _, r := range rows {
		sb.WriteString("- ")
		sb.WriteString(r.Summary)
		sb.WriteString("\n")
	}
	return sb.String()
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
