package workstation

import (
	"context"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const (
	sessionBufferMaxSessions = 20
	sessionBufferMaxLines    = 5000
)

// SessionStatus represents the run state of a buffered exec session.
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusDone    SessionStatus = "done"
	SessionStatusFailed  SessionStatus = "failed"
)

// SessionLine is a single buffered output line.
type SessionLine struct {
	Stream string `json:"stream"`
	Seq    int64  `json:"seq"`
	Data   string `json:"data"`
}

// SessionSummary is the metadata for a buffered session (no lines).
type SessionSummary struct {
	SessionKey string        `json:"sessionKey"`
	AgentID    string        `json:"agentId"`
	Command    string        `json:"command"`
	StartedAt  time.Time     `json:"startedAt"`
	EndedAt    *time.Time    `json:"endedAt,omitempty"`
	ExitCode   *int          `json:"exitCode,omitempty"`
	Status     SessionStatus `json:"status"`
	LineCount  int           `json:"lineCount"`
}

// SessionOutput is the full output for a buffered session.
type SessionOutput struct {
	Lines    []SessionLine `json:"lines"`
	Status   SessionStatus `json:"status"`
	ExitCode *int          `json:"exitCode,omitempty"`
}

type sessionEntry struct {
	summary SessionSummary
	lines   []SessionLine
}

// SessionBuffer maintains per-workstation in-memory session output for replay.
// Keyed by workstation_id (string). Thread-safe.
type SessionBuffer struct {
	mu       sync.RWMutex
	sessions map[string][]*sessionEntry // workstationID → ordered sessions (newest last)
}

// NewSessionBuffer creates a ready-to-use SessionBuffer.
func NewSessionBuffer() *SessionBuffer {
	return &SessionBuffer{
		sessions: make(map[string][]*sessionEntry),
	}
}

// WireSessionBuffer subscribes to workstation exec domain events and populates the buffer.
func WireSessionBuffer(bus eventbus.DomainEventBus, buf *SessionBuffer) {
	if bus == nil || buf == nil {
		return
	}

	bus.Subscribe(eventbus.EventType(protocol.EventWorkstationExecStart), func(_ context.Context, ev eventbus.DomainEvent) error {
		p, _ := ev.Payload.(map[string]any)
		if p == nil {
			return nil
		}
		wsID, _ := p["workstation_id"].(string)
		sessionKey, _ := p["session_key"].(string)
		agentID, _ := p["agent_id"].(string)
		command, _ := p["command"].(string)
		if wsID == "" || sessionKey == "" {
			return nil
		}
		buf.onStart(wsID, sessionKey, agentID, command)
		return nil
	})

	bus.Subscribe(eventbus.EventType(protocol.EventWorkstationExecChunk), func(_ context.Context, ev eventbus.DomainEvent) error {
		p, _ := ev.Payload.(map[string]any)
		if p == nil {
			return nil
		}
		wsID, _ := p["workstation_id"].(string)
		sessionKey, _ := p["session_key"].(string)
		stream, _ := p["stream"].(string)
		data, _ := p["data"].(string)
		var seq int64
		switch v := p["seq"].(type) {
		case float64:
			seq = int64(v)
		case int64:
			seq = v
		}
		if wsID == "" || sessionKey == "" {
			return nil
		}
		buf.onChunk(wsID, sessionKey, stream, seq, data)
		return nil
	})

	bus.Subscribe(eventbus.EventType(protocol.EventWorkstationExecDone), func(_ context.Context, ev eventbus.DomainEvent) error {
		p, _ := ev.Payload.(map[string]any)
		if p == nil {
			return nil
		}
		wsID, _ := p["workstation_id"].(string)
		sessionKey, _ := p["session_key"].(string)
		var exitCode int
		switch v := p["exit_code"].(type) {
		case float64:
			exitCode = int(v)
		case int:
			exitCode = v
		}
		if wsID == "" || sessionKey == "" {
			return nil
		}
		buf.onDone(wsID, sessionKey, exitCode)
		return nil
	})
}

func (b *SessionBuffer) onStart(wsID, sessionKey, agentID, command string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.sessions[wsID]
	// If session already exists (e.g. duplicate start), skip.
	for _, e := range list {
		if e.summary.SessionKey == sessionKey {
			return
		}
	}
	entry := &sessionEntry{
		summary: SessionSummary{
			SessionKey: sessionKey,
			AgentID:    agentID,
			Command:    command,
			StartedAt:  time.Now().UTC(),
			Status:     SessionStatusRunning,
		},
	}
	list = append(list, entry)
	// Keep only the last N sessions.
	if len(list) > sessionBufferMaxSessions {
		list = list[len(list)-sessionBufferMaxSessions:]
	}
	b.sessions[wsID] = list
}

func (b *SessionBuffer) onChunk(wsID, sessionKey, stream string, seq int64, data string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := b.findEntry(wsID, sessionKey)
	if entry == nil {
		return
	}
	entry.lines = append(entry.lines, SessionLine{Stream: stream, Seq: seq, Data: data})
	if len(entry.lines) > sessionBufferMaxLines {
		entry.lines = entry.lines[len(entry.lines)-sessionBufferMaxLines:]
	}
	entry.summary.LineCount = len(entry.lines)
}

func (b *SessionBuffer) onDone(wsID, sessionKey string, exitCode int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := b.findEntry(wsID, sessionKey)
	if entry == nil {
		return
	}
	now := time.Now().UTC()
	entry.summary.EndedAt = &now
	entry.summary.ExitCode = &exitCode
	if exitCode == 0 {
		entry.summary.Status = SessionStatusDone
	} else {
		entry.summary.Status = SessionStatusFailed
	}
}

// ListSessions returns session summaries for a workstation, newest first.
func (b *SessionBuffer) ListSessions(wsID string) []SessionSummary {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := b.sessions[wsID]
	out := make([]SessionSummary, len(list))
	for i, e := range list {
		out[i] = e.summary
	}
	// Reverse so newest is first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// GetOutput returns buffered output for a specific session.
func (b *SessionBuffer) GetOutput(wsID, sessionKey string) (SessionOutput, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entry := b.findEntry(wsID, sessionKey)
	if entry == nil {
		return SessionOutput{}, false
	}
	lines := make([]SessionLine, len(entry.lines))
	copy(lines, entry.lines)
	return SessionOutput{
		Lines:    lines,
		Status:   entry.summary.Status,
		ExitCode: entry.summary.ExitCode,
	}, true
}

// findEntry returns the entry for wsID+sessionKey; must be called with b.mu held.
func (b *SessionBuffer) findEntry(wsID, sessionKey string) *sessionEntry {
	for _, e := range b.sessions[wsID] {
		if e.summary.SessionKey == sessionKey {
			return e
		}
	}
	return nil
}
