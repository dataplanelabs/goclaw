package http

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TracesHandler handles LLM trace listing, detail, and retry endpoints.
type TracesHandler struct {
	tracing    store.TracingStore
	agents     store.AgentStore
	replay     store.ReplayPayloadStore
	retryLocks store.RetryLockStore
	tenants    store.TenantStore
	router     RetryAgentRunner
	channels   store.ChannelInstanceStore
	contacts   store.ContactStore
	cron       store.CronStore
}

func (h *TracesHandler) SetEnrichmentDeps(channels store.ChannelInstanceStore, contacts store.ContactStore) {
	h.channels = channels
	h.contacts = contacts
}

func (h *TracesHandler) SetCronStore(cron store.CronStore) {
	h.cron = cron
}

// RetryAgentRunner is the subset of agent.Router needed to invoke a retry run.
type RetryAgentRunner interface {
	GetAgent(ctx context.Context, agentID string) (RetryAgent, error)
}

// RetryAgent is the subset of agent.Agent the retry handler depends on.
type RetryAgent interface {
	UUID() uuid.UUID
	ProviderName() string
}

// NewTracesHandler creates a handler for trace management endpoints.
func NewTracesHandler(tracing store.TracingStore) *TracesHandler {
	return &TracesHandler{tracing: tracing}
}

// SetRetryDeps wires the dependencies required by POST /v1/traces/{id}/retry.
// Retry route is registered only when all are non-nil.
func (h *TracesHandler) SetRetryDeps(agents store.AgentStore, replay store.ReplayPayloadStore, retryLocks store.RetryLockStore, tenants store.TenantStore, router RetryAgentRunner) {
	h.agents = agents
	h.replay = replay
	h.retryLocks = retryLocks
	h.tenants = tenants
	h.router = router
}

// RegisterRoutes registers trace routes on the given mux.
func (h *TracesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/traces", h.authMiddleware(h.handleList))
	mux.HandleFunc("GET /v1/traces/recipients", h.authMiddleware(h.handleRecipients))
	mux.HandleFunc("GET /v1/traces/{traceID}/export", h.authMiddleware(h.handleExport))
	mux.HandleFunc("GET /v1/traces/{traceID}", h.authMiddleware(h.handleGet))
	mux.HandleFunc("GET /v1/costs/summary", h.authMiddleware(h.handleCostSummary))
	if h.canRetry() {
		mux.HandleFunc("POST /v1/traces/{traceID}/retry", h.authMiddleware(h.handleRetry))
	}
}

func (h *TracesHandler) canRetry() bool {
	return h.agents != nil && h.replay != nil && h.retryLocks != nil && h.tenants != nil && h.router != nil
}

func (h *TracesHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth("", next)
}

func (h *TracesHandler) handleList(w http.ResponseWriter, r *http.Request) {
	opts := store.TraceListOpts{
		Limit:  50,
		Offset: 0,
	}

	if v := r.URL.Query().Get("agent_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			opts.AgentID = &id
		}
	}
	if v := r.URL.Query().Get("user_id"); v != "" {
		opts.UserID = v
	}
	if v := r.URL.Query().Get("session_key"); v != "" {
		opts.SessionKey = v
	}
	switch r.URL.Query().Get("source_type") {
	case "cron", "group", "direct", "team", "ws":
		opts.SourceType = r.URL.Query().Get("source_type")
	}
	if v := r.URL.Query().Get("status"); v != "" {
		opts.Status = v
	}
	if v := r.URL.Query().Get("channel"); v != "" {
		opts.Channel = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	// Non-admin callers may only see their own traces.
	auth := resolveAuth(r)
	if !permissions.HasMinRole(auth.Role, permissions.RoleAdmin) {
		callerID := store.UserIDFromContext(r.Context())
		opts.UserID = callerID
	}

	traces, err := h.tracing.ListTraces(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.enrichChatTitles(r.Context(), traces)

	total, _ := h.tracing.CountTraces(r.Context(), opts)

	writeJSON(w, http.StatusOK, map[string]any{
		"traces": traces,
		"total":  total,
		"limit":  opts.Limit,
		"offset": opts.Offset,
	})
}

// traceRecipientOut is one entry in the tenant-wide recipient filter list.
type traceRecipientOut struct {
	UserID string `json:"user_id"`
	Label  string `json:"label"`
}

// handleRecipients returns the distinct set of trace recipients tenant-wide so
// the "Delivered to" filter lists every recipient, not just the current page.
func (h *TracesHandler) handleRecipients(w http.ResponseWriter, r *http.Request) {
	tenantID := store.TenantIDFromContext(r.Context())

	// Non-admin callers only ever filter their own traces; expose just themselves.
	auth := resolveAuth(r)
	if !permissions.HasMinRole(auth.Role, permissions.RoleAdmin) {
		callerID := store.UserIDFromContext(r.Context())
		out := []traceRecipientOut{}
		if callerID != "" {
			out = append(out, traceRecipientOut{UserID: callerID})
		}
		writeJSON(w, http.StatusOK, map[string]any{"recipients": out})
		return
	}

	recipients, err := h.tracing.ListTraceRecipients(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	pairs := make([]channelSession, len(recipients))
	for i, rec := range recipients {
		pairs[i] = channelSession{channel: rec.Channel, sessionKey: rec.SessionKey, runID: rec.RunID}
	}
	titles := h.resolveChatTitles(r.Context(), pairs, nil)

	out := make([]traceRecipientOut, 0, len(recipients))
	for i, rec := range recipients {
		out = append(out, traceRecipientOut{UserID: rec.UserID, Label: titles[i]})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Label == out[b].Label {
			return out[a].UserID < out[b].UserID
		}
		return out[a].Label < out[b].Label
	})

	writeJSON(w, http.StatusOK, map[string]any{"recipients": out})
}

// enrichChatTitles fills TraceData.ChatTitle for the given page by joining
// (trace.channel → channel_instance.channel_type) × (sender_id from session_key)
// against channel_contacts.display_name. Best-effort: silent on any lookup miss.
func (h *TracesHandler) enrichChatTitles(ctx context.Context, traces []store.TraceData) {
	if len(traces) == 0 {
		return
	}
	cronJobs := cronJobCache(nil)
	if h.cron != nil {
		cronJobs = make(cronJobCache)
	}
	h.enrichCronInputPreviews(ctx, traces, cronJobs)
	if h.channels == nil || h.contacts == nil {
		return
	}
	pairs := make([]channelSession, len(traces))
	for i := range traces {
		pairs[i] = channelSession{channel: traces[i].Channel, sessionKey: traces[i].SessionKey, runID: traces[i].RunID}
	}
	titles := h.resolveChatTitles(ctx, pairs, cronJobs)
	for i := range traces {
		if title, ok := titles[i]; ok {
			traces[i].ChatTitle = title
		}
	}
}

// channelSession pairs a trace's channel name with its session_key for title resolution.
type channelSession struct {
	channel    string
	sessionKey string
	runID      string
}

// resolveChatTitles resolves group/DM display names for the given (channel,
// session_key) pairs, returning a map of index → display name for resolved rows.
// Shared by enrichChatTitles and the recipients endpoint so labels match the rows.
func (h *TracesHandler) resolveChatTitles(ctx context.Context, pairs []channelSession, cronJobs cronJobCache) map[int]string {
	out := make(map[int]string, len(pairs))
	if len(pairs) == 0 || h.channels == nil || h.contacts == nil {
		return out
	}
	if cronJobs == nil && h.cron != nil {
		cronJobs = make(cronJobCache)
	}
	channelTypes := make(map[string]string, 4)
	senderIDs := make([]string, 0, len(pairs))
	sids := make([]string, len(pairs))
	for i, p := range pairs {
		sid := h.chatIDForTraceTitle(ctx, p, cronJobs)
		sids[i] = sid
		if sid == "" || p.channel == "" {
			continue
		}
		if _, ok := channelTypes[p.channel]; !ok {
			inst, err := h.channels.GetByName(ctx, p.channel)
			if err != nil || inst == nil {
				channelTypes[p.channel] = ""
				continue
			}
			channelTypes[p.channel] = inst.ChannelType
		}
		if channelTypes[p.channel] == "" {
			continue
		}
		senderIDs = append(senderIDs, sid)
	}
	if len(senderIDs) == 0 {
		return out
	}
	byID, err := h.contacts.GetContactsBySenderIDs(ctx, senderIDs)
	if err != nil {
		return out
	}
	for i, p := range pairs {
		sid := sids[i]
		if sid == "" {
			continue
		}
		c, ok := byID[sid]
		if !ok || c.ChannelType != channelTypes[p.channel] {
			continue
		}
		if c.DisplayName != nil && *c.DisplayName != "" {
			out[i] = *c.DisplayName
		}
	}
	return out
}

func (h *TracesHandler) chatIDForTraceTitle(ctx context.Context, p channelSession, cronJobs cronJobCache) string {
	if job := h.cronJobForTrace(ctx, p.runID, p.sessionKey, cronJobs); job != nil && job.DeliverTo != "" {
		return job.DeliverTo
	}
	return chatIDFromSessionKey(p.sessionKey)
}

func (h *TracesHandler) enrichCronInputPreviews(ctx context.Context, traces []store.TraceData, cronJobs cronJobCache) {
	if h.cron == nil {
		return
	}
	for i := range traces {
		job := h.cronJobForTrace(ctx, traces[i].RunID, traces[i].SessionKey, cronJobs)
		if job == nil || job.Schedule.TZ == "" {
			continue
		}
		traces[i].InputPreview = rewriteLeadingTraceStamp(traces[i].InputPreview, traces[i].StartTime, job.Schedule.TZ)
	}
}

type cronJobCache map[string]*store.CronJob

func (h *TracesHandler) cronJobForTrace(ctx context.Context, runID, sessionKey string, cronJobs cronJobCache) *store.CronJob {
	if h.cron == nil {
		return nil
	}
	jobID := cronJobIDFromTrace(runID, sessionKey)
	if jobID == "" {
		return nil
	}
	if cronJobs != nil {
		if job, ok := cronJobs[jobID]; ok {
			return job
		}
	}
	job, ok := h.cron.GetJob(ctx, jobID)
	if !ok {
		if cronJobs != nil {
			cronJobs[jobID] = nil
		}
		return nil
	}
	if cronJobs != nil {
		cronJobs[jobID] = job
	}
	return job
}

func rewriteLeadingTraceStamp(input string, start time.Time, tz string) string {
	if input == "" || start.IsZero() || tz == "" || !strings.HasPrefix(input, "[") {
		return input
	}
	end := strings.IndexByte(input, ']')
	if end <= 1 {
		return input
	}
	if _, err := time.Parse("2006-01-02 15:04 -07", input[1:end]); err != nil {
		return input
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return input
	}
	stamp := start.In(loc).Format("2006-01-02 15:04 -07")
	return "[" + stamp + "]" + input[end+1:]
}

func cronJobIDFromTrace(runID, sessionKey string) string {
	if strings.HasPrefix(runID, "cron:") {
		if jobID := strings.TrimPrefix(runID, "cron:"); jobID != "" {
			return jobID
		}
	}
	if !strings.Contains(sessionKey, ":cron:") {
		return ""
	}
	return chatIDFromSessionKey(sessionKey)
}

// chatIDFromSessionKey returns the last colon-separated segment, the typical
// shape being "agent:<agent>:<channel>:<direct|group>:<chat_id>".
func chatIDFromSessionKey(key string) string {
	if key == "" {
		return ""
	}
	idx := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	return key[idx+1:]
}

func (h *TracesHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	traceIDStr := r.PathValue("traceID")
	traceID, err := uuid.Parse(traceIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidID, "trace")})
		return
	}

	trace, err := h.tracing.GetTrace(r.Context(), traceID)
	if err != nil {
		slog.Warn("traces.get_trace_failed", "trace_id", traceIDStr, "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceIDStr)})
		return
	}

	// Non-admin callers may only access their own traces.
	auth := resolveAuth(r)
	if !permissions.HasMinRole(auth.Role, permissions.RoleAdmin) {
		callerID := store.UserIDFromContext(r.Context())
		if trace.UserID != callerID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceIDStr)})
			return
		}
	}

	enriched := []store.TraceData{*trace}
	h.enrichChatTitles(r.Context(), enriched)
	*trace = enriched[0]

	spans, err := h.tracing.GetTraceSpans(r.Context(), traceID)
	if err != nil {
		slog.Error("traces.get_spans_failed", "trace_id", traceIDStr, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trace": trace,
		"spans": spans,
	})
}

func (h *TracesHandler) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	opts := store.CostSummaryOpts{}

	if v := r.URL.Query().Get("agent_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			opts.AgentID = &id
		}
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.From = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.To = &t
		}
	}

	rows, err := h.tracing.GetCostSummary(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// traceExportEntry is a trace with its spans and recursive sub-traces.
type traceExportEntry struct {
	Trace     store.TraceData    `json:"trace"`
	Spans     []store.SpanData   `json:"spans"`
	SubTraces []traceExportEntry `json:"sub_traces,omitempty"`
}

func (h *TracesHandler) handleExport(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	traceID, err := uuid.Parse(r.PathValue("traceID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidID, "trace")})
		return
	}

	// Verify ownership before export.
	rootTrace, err := h.tracing.GetTrace(r.Context(), traceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceID.String())})
		return
	}
	authExport := resolveAuth(r)
	if !permissions.HasMinRole(authExport.Role, permissions.RoleAdmin) {
		callerID := store.UserIDFromContext(r.Context())
		if rootTrace.UserID != callerID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceID.String())})
			return
		}
	}

	entry, err := h.collectTraceTree(r.Context(), traceID, 0)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceID.String())})
		return
	}

	payload := struct {
		ExportedAt time.Time `json:"exported_at"`
		traceExportEntry
	}{
		ExportedAt:       time.Now().UTC(),
		traceExportEntry: *entry,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("trace-%s-%s.json.gz", traceID.String()[:8], time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	gz := gzip.NewWriter(w)
	defer gz.Close()
	gz.Write(data)
}

// collectTraceTree recursively collects a trace, its spans, and child traces.
func (h *TracesHandler) collectTraceTree(ctx context.Context, traceID uuid.UUID, depth int) (*traceExportEntry, error) {
	const maxDepth = 10
	trace, err := h.tracing.GetTrace(ctx, traceID)
	if err != nil {
		return nil, err
	}

	spans, _ := h.tracing.GetTraceSpans(ctx, traceID)

	entry := &traceExportEntry{Trace: *trace, Spans: spans}

	if depth >= maxDepth {
		return entry, nil
	}

	children, _ := h.tracing.ListChildTraces(ctx, traceID)
	for _, child := range children {
		sub, err := h.collectTraceTree(ctx, child.ID, depth+1)
		if err != nil {
			continue
		}
		entry.SubTraces = append(entry.SubTraces, *sub)
	}

	return entry, nil
}
