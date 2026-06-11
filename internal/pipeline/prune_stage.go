package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// PruneStage runs every iteration. 2-phase pruning:
//   - Phase 1 (70% budget): soft trim via PruneMessages callback
//   - Phase 2 (100% budget): memory flush + LLM compaction
//
// Implements StageWithResult. NEVER returns AbortRun for the over-budget case —
// it recovers (compaction → hard-truncate → proceed) so the run is never silenced.
type PruneStage struct {
	deps        *PipelineDeps
	memoryFlush *MemoryFlushStage
	result      StageResult
}

// NewPruneStage creates a PruneStage with inline memory flush.
func NewPruneStage(deps *PipelineDeps, memFlush *MemoryFlushStage) *PruneStage {
	return &PruneStage{deps: deps, memoryFlush: memFlush, result: Continue}
}

func (s *PruneStage) Name() string       { return "prune" }
func (s *PruneStage) Result() StageResult { return s.result }

// defaultCachePruneTTL is used when cfg.TTL is empty or invalid.
const defaultCachePruneTTL = 5 * time.Minute

// parseTTL parses a Go duration string (e.g. "5m"). Falls back to 5m default on
// empty/invalid input with a warning log. Negative durations treated as invalid.
func parseTTL(s string) time.Duration {
	if s == "" {
		return defaultCachePruneTTL
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		slog.Warn("context_pruning: invalid ttl, using 5m default", "ttl", s, "err", err)
		return defaultCachePruneTTL
	}
	return d
}

// Execute checks history tokens against budget, prunes/compacts as needed.
func (s *PruneStage) Execute(ctx context.Context, state *RunState) error {
	s.result = Continue

	if state.Think.LastResponse != nil && len(state.Think.LastResponse.ToolCalls) == 0 {
		return nil
	}

	// Compute budget using the effective context window for this run's model.
	// ContextStage resolves EffectiveContextWindow once per run via ModelRegistry;
	// if zero (unknown model, registry not wired) fall back to the pipeline-wide
	// Config.ContextWindow for backward compatibility.
	//
	// ReserveTokens (optional, default 0) carves out a safety buffer so compaction
	// fires slightly before the hard limit — protects against provider over-delivery
	// and token-counter drift on streaming responses.
	contextWindow := state.Context.EffectiveContextWindow
	if contextWindow == 0 {
		contextWindow = s.deps.Config.ContextWindow
	}
	budget := contextWindow - state.Context.OverheadTokens - s.deps.Config.MaxTokens - s.deps.Config.ReserveTokens
	if budget <= 0 {
		return nil // no history budget, nothing to prune
	}
	state.Prune.HistoryBudget = budget

	// Count current history tokens
	historyTokens := s.countHistory(state)
	state.Prune.HistoryTokens = historyTokens
	tokensBefore := historyTokens

	softThreshold := budget * 70 / 100
	if historyTokens <= softThreshold {
		return nil // under budget, no action needed
	}

	sessionKey := state.Input.SessionKey
	skipSoftPrune := false

	// Cache-TTL gate (provider-aware, per-session).
	// Only activates when all 4 callbacks are wired and mode == "cache-ttl".
	if s.deps.GetPruningConfig != nil && s.deps.GetProviderCaps != nil && s.deps.GetCacheTouch != nil {
		cfg := s.deps.GetPruningConfig()
		if cfg != nil && cfg.Mode == "cache-ttl" {
			caps := s.deps.GetProviderCaps()
			if caps.CacheControl {
				ttl := parseTTL(cfg.TTL)
				touch := s.deps.GetCacheTouch(sessionKey)
				if !touch.IsZero() && time.Since(touch) < ttl {
					if historyTokens <= budget {
						return nil // cache still live, below hard budget — preserve prefix
					}
					// Over hard budget despite live cache — safety valve: skip soft prune,
					// fall through to compaction only.
					slog.Info("context.cache_ttl_overridden",
						"session_key", sessionKey,
						"tokens", historyTokens,
						"budget", budget,
					)
					skipSoftPrune = true
				}
			}
		}
	}

	// Phase 1: soft prune at 70% budget (unless TTL gate routed to compact-only).
	var pruneStats PruneStats
	if !skipSoftPrune && s.deps.PruneMessages != nil {
		var pruned []providers.Message
		pruned, pruneStats = s.deps.PruneMessages(state.Messages.History(), budget)
		mutated := pruneStats.ResultsTrimmed > 0 || pruneStats.ResultsCleared > 0
		if mutated {
			// Sanitize after prune to fix broken tool_use/tool_result pairs.
			// Applied in-memory only — prune-induced orphans are ephemeral.
			if s.deps.SanitizeHistory != nil {
				pruned, _ = s.deps.SanitizeHistory(pruned)
			}
			state.Messages.SetHistory(pruned)
			historyTokens = s.countHistory(state)
			state.Prune.HistoryTokens = historyTokens
			// Match TS semantics: touch recorded ONLY after prune mutates messages.
			if s.deps.MarkCacheTouched != nil && sessionKey != "" {
				s.deps.MarkCacheTouched(sessionKey)
			}
		}
	}

	// Emit observability event + slog when pruning actually mutated messages.
	if pruneStats.ResultsTrimmed > 0 || pruneStats.ResultsCleared > 0 || pruneStats.Compacted {
		trigger := "soft"
		if pruneStats.Compacted {
			trigger = "compact"
		} else if pruneStats.ResultsCleared > 0 {
			trigger = "hard"
		}
		if s.deps.EventBus != nil {
			s.deps.EventBus.Publish(eventbus.DomainEvent{
				Type:     eventbus.EventContextPruned,
				SourceID: sessionKey,
				Payload: &eventbus.ContextPrunedPayload{
					SessionKey:     sessionKey,
					TokensBefore:   tokensBefore,
					TokensAfter:    historyTokens,
					Budget:         budget,
					ResultsTrimmed: pruneStats.ResultsTrimmed,
					ResultsCleared: pruneStats.ResultsCleared,
					Compacted:      pruneStats.Compacted,
					Trigger:        trigger,
				},
			})
		}
		slog.Info("context.pruned",
			"session_key", sessionKey,
			"tokens_before", tokensBefore,
			"tokens_after", historyTokens,
			"trimmed", pruneStats.ResultsTrimmed,
			"cleared", pruneStats.ResultsCleared,
			"compacted", pruneStats.Compacted,
			"trigger", trigger,
		)
	}

	if historyTokens <= budget {
		return nil // under budget after soft prune
	}

	// Phase 2: compaction — flush memories first, then compact
	if !state.Compact.MemoryFlushedThisCycle && s.memoryFlush != nil {
		if err := s.memoryFlush.Execute(ctx, state); err != nil {
			slog.Warn("prune: memory flush error", "err", err)
		}
		state.Compact.MemoryFlushedThisCycle = true
	}

	if s.deps.CompactMessages == nil {
		return nil // no compaction available
	}

	compacted, err := s.deps.CompactMessages(ctx, state.Messages.History(), state.Model)
	if err != nil {
		// Never abort the run on a compaction failure — recover by hard-truncating
		// oldest history. A silent NO_REPLY is worse than a truncated context.
		slog.Warn("compaction failed — hard-truncating to recover", "err", err, "budget", budget)
	} else {
		state.Messages.ReplaceHistory(compacted)
		state.Prune.MidLoopCompacted = true
		state.Compact.CompactionCount++
		state.Compact.MemoryFlushedThisCycle = false // reset for next cycle

		// Recount after compaction
		historyTokens = s.countHistory(state)
		state.Prune.HistoryTokens = historyTokens
	}

	if historyTokens > budget {
		s.hardTruncateToFit(state, budget)
		historyTokens = s.countHistory(state)
		state.Prune.HistoryTokens = historyTokens
		if historyTokens > budget {
			// Pathological single huge message — proceed anyway. The model's hard
			// context window exceeds this soft budget, so the next LLM call usually
			// succeeds; a genuine failure surfaces as a real provider error, not silence.
			slog.Warn("still over budget after hard truncation — proceeding",
				"tokens", historyTokens, "budget", budget)
		}
	}

	return nil
}

// hardTruncateToFit drops the OLDEST history messages until under budget, keeping
// the last keepCount turns. Preserves tool_use→tool_result pairing exactly like
// compactMessagesInPlace's boundary walk (never starts the kept window on an
// orphaned tool result). Never AbortRun — recovery, not failure.
func (s *PruneStage) hardTruncateToFit(state *RunState, budget int) {
	history := state.Messages.History()
	if len(history) == 0 {
		return
	}

	keepCount := 6
	if s.deps.Config.Compaction != nil && s.deps.Config.Compaction.KeepLastMessages > 0 {
		keepCount = s.deps.Config.Compaction.KeepLastMessages
	}
	if keepCount >= len(history) {
		return // nothing older to drop
	}

	// Drop from the front, growing the kept window until under budget or exhausted.
	for keepCount < len(history) {
		splitIdx := len(history) - keepCount
		// Walk backward to a clean boundary so we never start the kept window on a
		// tool result whose tool_use was dropped.
		for splitIdx > 0 {
			m := history[splitIdx]
			if m.Role == "tool" || (m.Role == "assistant" && len(m.ToolCalls) > 0) {
				splitIdx--
				continue
			}
			break
		}
		if splitIdx <= 0 {
			return // can't drop further without orphaning — keep everything
		}

		truncated := history[splitIdx:]
		state.Messages.ReplaceHistory(truncated)
		state.Prune.MidLoopCompacted = true

		tokens := s.countHistory(state)
		state.Prune.HistoryTokens = tokens
		slog.Warn("hard-truncated history to recover",
			"dropped", splitIdx, "kept", len(truncated), "tokens", tokens, "budget", budget)
		if tokens <= budget {
			return
		}
		// Still over — keep more of the tail dropped (grow keepCount target relative
		// to the now-shorter history) by retrying against the truncated slice.
		history = truncated
		keepCount = max(keepCount-1, 1)
		if keepCount >= len(history) {
			return
		}
	}
}

// countHistory counts history + pending tokens via TokenCounter.
func (s *PruneStage) countHistory(state *RunState) int {
	h := state.Messages.History()
	p := state.Messages.Pending()
	if s.deps.TokenCounter == nil || (len(h) == 0 && len(p) == 0) {
		return 0
	}
	// Explicit copy to avoid aliasing the history slice.
	msgs := make([]providers.Message, 0, len(h)+len(p))
	msgs = append(msgs, h...)
	msgs = append(msgs, p...)
	return s.deps.TokenCounter.CountMessages(state.Model, msgs)
}
