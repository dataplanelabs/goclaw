package bot

import (
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
)

// Zalo expires the indicator after ~5s; re-fire under that.
// 5-min safety net for stuck runs — covers heavy pipelines while staying
// below Zalo's anti-abuse threshold for continuous typing pings.
const (
	typingKeepalive = 4 * time.Second
	typingMaxTTL    = 5 * time.Minute
)

func (c *Channel) startTyping(chatID string) {
	if !c.IsRunning() {
		return
	}
	ctrl := typing.New(typing.Options{
		MaxDuration:       typingMaxTTL,
		KeepaliveInterval: typingKeepalive,
		StartFn: func() error {
			return c.sendChatAction(chatID, "typing")
		},
	})
	if prev, ok := c.typingCtrls.Load(chatID); ok {
		prev.(*typing.Controller).Stop()
	}
	c.typingCtrls.Store(chatID, ctrl)
	// If Stop's Range happened before our Store, ctrl would leak past shutdown.
	if !c.IsRunning() {
		c.typingCtrls.Delete(chatID)
		ctrl.Stop()
		return
	}
	ctrl.Start()
}
