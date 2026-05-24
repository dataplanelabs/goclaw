package personal

import (
	"encoding/json"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// zaloCreds maps the credentials JSON from the channel_instances table.
type zaloCreds struct {
	IMEI      string               `json:"imei"`
	Cookie    *protocol.CookieUnion `json:"cookie"`
	UserAgent string               `json:"userAgent"`
	Language  *string              `json:"language,omitempty"`
}

// zaloInstanceConfig maps the config JSONB from the channel_instances table.
type zaloInstanceConfig struct {
	DMPolicy            string   `json:"dm_policy,omitempty"`
	GroupPolicy         string   `json:"group_policy,omitempty"`
	RequireMention      *bool    `json:"require_mention,omitempty"`
	HistoryLimit        int      `json:"history_limit,omitempty"`
	AllowFrom           []string `json:"allow_from,omitempty"`
	BlockReply              *bool `json:"block_reply,omitempty"`
	QuoteUserMessageInGroup *bool `json:"quote_user_message_in_group,omitempty"`
	QuoteUserMessageInDM    *bool `json:"quote_user_message_in_dm,omitempty"`
	DisablePolls        bool     `json:"disable_polls,omitempty"`
	DisableReactions    bool     `json:"disable_reactions,omitempty"`
	ListenSelfReactions bool     `json:"listen_self_reactions,omitempty"`
	ReactionsMode       string   `json:"reactions_mode,omitempty"`

	ReactionLevel              string `json:"reaction_level,omitempty"`
	ReactionTerminalDelayMinMs int    `json:"reaction_terminal_delay_min_ms,omitempty"`
	ReactionTerminalDelayMaxMs int    `json:"reaction_terminal_delay_max_ms,omitempty"`
}

// Factory creates a Zalo Personal channel from DB instance data.
// Does NOT trigger QR login — credentials must be provided.
func Factory(name string, creds json.RawMessage, cfg json.RawMessage,
	msgBus *bus.MessageBus, pairingSvc store.PairingStore) (channels.Channel, error) {

	var c zaloCreds
	if len(creds) > 0 {
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("decode zalo_personal credentials: %w", err)
		}
	}

	// No credentials yet — return nil,nil to signal "not ready" to instanceLoader.
	// The channel will be created via Reload() after QR login saves creds.
	if c.IMEI == "" || c.Cookie == nil {
		return nil, nil
	}

	var ic zaloInstanceConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &ic); err != nil {
			return nil, fmt.Errorf("decode zalo_personal config: %w", err)
		}
	}

	zaloCfg := config.ZaloPersonalConfig{
		Enabled:             true,
		AllowFrom:           ic.AllowFrom,
		DMPolicy:            ic.DMPolicy,
		GroupPolicy:         ic.GroupPolicy,
		RequireMention:      ic.RequireMention,
		HistoryLimit:        ic.HistoryLimit,
		BlockReply:              ic.BlockReply,
		QuoteUserMessageInGroup: ic.QuoteUserMessageInGroup,
		QuoteUserMessageInDM:    ic.QuoteUserMessageInDM,
		DisablePolls:        ic.DisablePolls,
		DisableReactions:    ic.DisableReactions,
		ListenSelfReactions: ic.ListenSelfReactions,
		ReactionsMode:       ic.ReactionsMode,

		ReactionLevel:              ic.ReactionLevel,
		ReactionTerminalDelayMinMs: ic.ReactionTerminalDelayMinMs,
		ReactionTerminalDelayMaxMs: ic.ReactionTerminalDelayMaxMs,
	}

	ch, err := New(zaloCfg, msgBus, pairingSvc, nil)
	if err != nil {
		return nil, err
	}

	protoCred := &protocol.Credentials{
		IMEI:      c.IMEI,
		Cookie:    c.Cookie,
		UserAgent: c.UserAgent,
		Language:  c.Language,
	}
	ch.SetPreloadedCredentials(protoCred)
	ch.SetName(name)

	return ch, nil
}

// FactoryWithPendingStore returns a ChannelFactory with persistent history support.
// episodicStore is optional — when set, inbound reactions persist as
// reaction_feedback rows that the agent reads next turn.
func FactoryWithPendingStore(pendingStore store.PendingMessageStore, episodicStore store.EpisodicStore) channels.ChannelFactory {
	return func(name string, creds json.RawMessage, cfg json.RawMessage,
		msgBus *bus.MessageBus, pairingSvc store.PairingStore) (channels.Channel, error) {

		var c zaloCreds
		if len(creds) > 0 {
			if err := json.Unmarshal(creds, &c); err != nil {
				return nil, fmt.Errorf("decode zalo_personal credentials: %w", err)
			}
		}

		if c.IMEI == "" || c.Cookie == nil {
			return nil, nil
		}

		var ic zaloInstanceConfig
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &ic); err != nil {
				return nil, fmt.Errorf("decode zalo_personal config: %w", err)
			}
		}

		zaloCfg := config.ZaloPersonalConfig{
			Enabled:                    true,
			AllowFrom:                  ic.AllowFrom,
			DMPolicy:                   ic.DMPolicy,
			GroupPolicy:                ic.GroupPolicy,
			RequireMention:             ic.RequireMention,
			HistoryLimit:               ic.HistoryLimit,
			BlockReply:                 ic.BlockReply,
			QuoteUserMessageInGroup:    ic.QuoteUserMessageInGroup,
			QuoteUserMessageInDM:       ic.QuoteUserMessageInDM,
			DisablePolls:               ic.DisablePolls,
			DisableReactions:           ic.DisableReactions,
			ListenSelfReactions:        ic.ListenSelfReactions,
			ReactionsMode:              ic.ReactionsMode,
			ReactionLevel:              ic.ReactionLevel,
			ReactionTerminalDelayMinMs: ic.ReactionTerminalDelayMinMs,
			ReactionTerminalDelayMaxMs: ic.ReactionTerminalDelayMaxMs,
		}

		ch, err := New(zaloCfg, msgBus, pairingSvc, pendingStore)
		if err != nil {
			return nil, err
		}
		if episodicStore != nil {
			ch.SetEpisodicStore(episodicStore)
		}

		protoCred := &protocol.Credentials{
			IMEI:      c.IMEI,
			Cookie:    c.Cookie,
			UserAgent: c.UserAgent,
			Language:  c.Language,
		}
		ch.SetPreloadedCredentials(protoCred)
		ch.SetName(name)

		return ch, nil
	}
}
