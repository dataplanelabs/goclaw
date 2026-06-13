package personal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const defaultTurnGrace = 2 * time.Second

// Channel connects to Zalo Personal Chat via the internal protocol port (from zcago, MIT).
// WARNING: Zalo Personal is an unofficial, reverse-engineered integration. Account may be locked/banned.
type Channel struct {
	*channels.BaseChannel
	config      config.ZaloPersonalConfig
	typingCtrls sync.Map // threadID → *typing.Controller

	mu       sync.RWMutex // protects sess and listener
	sess     *protocol.Session
	listener *protocol.Listener

	reactionCoalescer *reactionCoalescer
	episodicStore     store.EpisodicStore

	reactions      sync.Map
	reactionWG     sync.WaitGroup
	reactionCtx    context.Context
	reactionCancel context.CancelFunc
	lastReplyChars sync.Map

	preloadedCreds *protocol.Credentials

	groups             *groupCache
	lastGroupBootstrap atomic.Int64

	memberCache        *MemberCache
	memberFetchLimiter *MemberFetchLimiter
	memberFetcher      func(ctx context.Context, sess *protocol.Session, groupID string) ([]protocol.GroupMember, error)

	enableNativeStyles bool
	turnCoalescer      *channels.TurnCoalescer[inboundTurn]

	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a new Zalo Personal channel from config.
func New(cfg config.ZaloPersonalConfig, msgBus *bus.MessageBus, pairingSvc store.PairingStore, pendingStore store.PendingMessageStore) (*Channel, error) {
	base := channels.NewBaseChannel(channels.TypeZaloPersonal, msgBus, cfg.AllowFrom)

	if cfg.DMPolicy == "" {
		cfg.DMPolicy = "allowlist"
	}
	if cfg.GroupPolicy == "" {
		cfg.GroupPolicy = "allowlist"
	}
	base.ValidatePolicy(cfg.DMPolicy, cfg.GroupPolicy)

	historyLimit := cfg.HistoryLimit
	if historyLimit == 0 {
		historyLimit = channels.DefaultGroupHistoryLimit
	}

	requireMention := true
	if cfg.RequireMention != nil {
		requireMention = *cfg.RequireMention
	}

	// zalo_personal operator defaults: native markdown styling + full reactions.
	enableNativeStyles := true
	if cfg.EnableNativeStyles != nil {
		enableNativeStyles = *cfg.EnableNativeStyles
	}
	if cfg.ReactionLevel == "" {
		cfg.ReactionLevel = "full"
	}
	turnGrace := defaultTurnGrace
	switch {
	case cfg.TurnGraceMs < 0:
		turnGrace = 0
	case cfg.TurnGraceMs > 0:
		turnGrace = time.Duration(cfg.TurnGraceMs) * time.Millisecond
	}

	ch := &Channel{
		BaseChannel:        base,
		config:             cfg,
		groups:             newGroupCache(),
		stopCh:             make(chan struct{}),
		memberCache:        NewMemberCache(),
		memberFetchLimiter: NewMemberFetchLimiter(60 * time.Second),
		memberFetcher:      protocol.FetchGroupMembers,
		enableNativeStyles: enableNativeStyles,
	}
	ch.turnCoalescer = channels.NewTurnCoalescer[inboundTurn](turnGrace, mergeInboundTurns, ch.dispatchInboundTurn)
	ch.SetPairingService(pairingSvc)
	ch.SetGroupHistory(channels.MakeHistory(channels.TypeZaloPersonal, pendingStore, base.TenantID()))
	ch.SetHistoryLimit(historyLimit)
	ch.SetRequireMention(requireMention)
	ch.reactionCoalescer = newReactionCoalescer(reactionCoalesceWindow, ch.emitCoalescedReaction)
	ch.reactionCtx, ch.reactionCancel = context.WithCancel(context.Background())
	return ch, nil
}

// BlockReplyEnabled returns the per-channel block_reply override (nil = inherit gateway default).
func (c *Channel) BlockReplyEnabled() *bool { return c.config.BlockReply }

func (c *Channel) QuoteInboundOnDM() bool { return c.quoteInDM() }

// quoteInGroup defaults true (groups need disambiguation).
func (c *Channel) quoteInGroup() bool {
	if c.config.QuoteUserMessageInGroup != nil {
		return *c.config.QuoteUserMessageInGroup
	}
	return true
}

// quoteInDM defaults false (DM has no ambiguity).
func (c *Channel) quoteInDM() bool {
	if c.config.QuoteUserMessageInDM != nil {
		return *c.config.QuoteUserMessageInDM
	}
	return false
}

// session returns the current session snapshot (thread-safe).
func (c *Channel) session() *protocol.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sess
}

// getListener returns the current listener snapshot (thread-safe).
func (c *Channel) getListener() *protocol.Listener {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.listener
}

// Start authenticates and begins listening for Zalo messages.
func (c *Channel) Start(ctx context.Context) error {
	if gh := c.GroupHistory(); gh != nil {
		gh.StartFlusher()
	}
	slog.Warn("security.unofficial_api",
		"channel", "zalo_personal",
		"msg", "Zalo Personal is unofficial and reverse-engineered. Account may be locked/banned. Use at own risk.",
	)

	sess, err := c.authenticate(ctx)
	if err != nil {
		return fmt.Errorf("zalo_personal auth: %w", err)
	}

	ln, err := protocol.NewListener(sess)
	if err != nil {
		return fmt.Errorf("zalo_personal listener: %w", err)
	}
	// Retry listener start on transient errors — primarily CoreDNS cold-miss
	// SERVFAIL for ws*-msg.chat.zalo.me at pod boot, where DNS hasn't yet
	// resolved the Zalo WS host. By attempt 2-3, DNS has warmed.
	if _, err := providers.RetryDo(ctx, providers.RetryConfig{
		Attempts: 5,
		MinDelay: 500 * time.Millisecond,
		MaxDelay: 10 * time.Second,
		Jitter:   0.1,
	}, func() (struct{}, error) {
		return struct{}{}, ln.Start(ctx)
	}); err != nil {
		return fmt.Errorf("zalo_personal listener start: %w", err)
	}

	c.mu.Lock()
	c.sess = sess
	c.listener = ln
	c.mu.Unlock()

	slog.Info("zalo_personal connected", "uid", sess.UID)

	c.SetRunning(true)
	go c.listenLoop(ctx)

	if cc := c.ContactCollector(); cc != nil && shouldBootstrap(&c.lastGroupBootstrap) {
		go bootstrapGroups(ctx, sess, cc, c.groups, c.Type(), c.Name())
	}

	slog.Info("zalo_personal listener loop started")
	return nil
}

// SetPendingCompaction configures LLM-based auto-compaction for pending messages.
func (c *Channel) SetPendingCompaction(cfg *channels.CompactionConfig) {
	if gh := c.GroupHistory(); gh != nil {
		gh.SetCompactionConfig(cfg)
	}
}

func (c *Channel) SetEpisodicStore(s store.EpisodicStore) { c.episodicStore = s }

func (c *Channel) SetPendingHistoryTenantID(id uuid.UUID) {
	if gh := c.GroupHistory(); gh != nil {
		gh.SetTenantID(id)
	}
}

// Stop gracefully shuts down the Zalo Personal channel.
func (c *Channel) Stop(_ context.Context) error {
	c.flushPendingInboundTurns()
	if gh := c.GroupHistory(); gh != nil {
		gh.StopFlusher()
	}
	slog.Info("stopping zalo_personal channel")
	c.stopOnce.Do(func() { close(c.stopCh) })
	c.typingCtrls.Range(func(key, val any) bool {
		if ctrl, ok := val.(*typing.Controller); ok {
			ctrl.Stop()
		}
		c.typingCtrls.Delete(key)
		return true
	})
	if ln := c.getListener(); ln != nil {
		ln.Stop()
	}
	if c.reactionCoalescer != nil {
		c.reactionCoalescer.Cancel()
	}
	if c.reactionCancel != nil {
		c.reactionCancel()
	}
	c.reactionWG.Wait()
	c.SetRunning(false)
	return nil
}
