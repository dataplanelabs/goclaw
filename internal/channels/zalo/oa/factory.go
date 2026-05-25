package oa

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Factory returns a channels.ChannelFactory closure capturing the store.
// Webhook-mode channels register with common.SharedRouter() at Start().
func Factory(ciStore store.ChannelInstanceStore) channels.ChannelFactory {
	return func(name string, credsRaw json.RawMessage, cfgRaw json.RawMessage,
		msgBus *bus.MessageBus, pairingSvc store.PairingStore) (channels.Channel, error) {

		if ciStore == nil {
			return nil, errors.New("zalo_oa: nil ChannelInstanceStore")
		}

		creds, err := LoadCreds(credsRaw)
		if err != nil {
			return nil, fmt.Errorf("zalo_oa: decode credentials: %w", err)
		}

		var cfg config.ZaloOAConfig
		if len(cfgRaw) > 0 {
			if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
				return nil, fmt.Errorf("zalo_oa: decode config: %w", err)
			}
		}

		ch, err := New(name, cfg, creds, ciStore, msgBus, pairingSvc)
		if err != nil {
			return nil, err
		}
		// Seed cursor from persisted channel_instances.config.poll_cursor.
		if seeded := parseCursorFromConfig(cfgRaw); len(seeded) > 0 {
			ch.cursor.loadFromMap(seeded)
		}
		return ch, nil
	}
}

// FactoryWithDeps mirrors Factory but threads team-reply-capture deps into
// the channel so Phase 4's polling worker can be wired up at Start().
// Tenant resolution: ciStore lookup by instanceID is set by the loader via
// SetInstanceID; the worker reads tenantID lazily from credentials/loader.
func FactoryWithDeps(ciStore store.ChannelInstanceStore, domainBus eventbus.DomainEventBus,
	sessions store.SessionStore, evals store.TeamReplyEvalStore, atomic store.AtomicTeamReplyWriter,
	judgeResolver JudgeAgentResolver) channels.ChannelFactory {

	base := Factory(ciStore)
	return func(name string, credsRaw json.RawMessage, cfgRaw json.RawMessage,
		msgBus *bus.MessageBus, pairingSvc store.PairingStore) (channels.Channel, error) {

		c, err := base(name, credsRaw, cfgRaw, msgBus, pairingSvc)
		if err != nil {
			return nil, err
		}
		oaCh, ok := c.(*Channel)
		if !ok {
			return c, nil
		}
		oaCh.SetTeamReplyDeps(domainBus, sessions, evals, atomic, "", judgeResolver)
		return oaCh, nil
	}
}
