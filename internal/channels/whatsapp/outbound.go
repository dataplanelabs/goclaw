package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// Send delivers an outbound message to WhatsApp via whatsmeow.
func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("whatsapp not connected")
	}

	chatJID, err := types.ParseJID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid whatsapp JID %q: %w", msg.ChatID, err)
	}

	// TTS auto-apply: convert [[tts]] tagged responses to voice
	if c.audioMgr != nil && msg.Content != "" {
		isVoiceInbound := msg.Metadata["is_voice_inbound"] == "true"
		ttsResult, ttsErr := c.audioMgr.AutoApplyToText(ctx, msg.Content, "whatsapp", isVoiceInbound, "")
		if ttsErr != nil {
			slog.Debug("whatsapp: tts auto-apply error", "error", ttsErr)
		}
		if ttsResult != nil && ttsResult.AudioPath != "" {
			// Send audio as voice message
			audioData, readErr := os.ReadFile(ttsResult.AudioPath)
			if readErr == nil {
				waMsg, buildErr := c.buildMediaMessage(audioData, ttsResult.AudioMime, "")
				if buildErr == nil {
					// Mark as voice message (PTT) for WhatsApp
					if waMsg.AudioMessage != nil {
						waMsg.AudioMessage.PTT = new(true)
					}
					resp, sendErr := c.client.SendMessage(c.ctx, chatJID, waMsg)
					if sendErr != nil {
						slog.Warn("whatsapp: tts auto-apply voice send failed, falling back to text", "error", sendErr)
					} else {
						c.recordMessagePreview(ctx, string(resp.ID), mediaAttachmentPreview("voice", ttsResult.AudioPath, ""), resp.Timestamp)
						// Voice sent successfully, stop typing and return
						if cancel, ok := c.typingCancel.LoadAndDelete(msg.ChatID); ok {
							if fn, ok := cancel.(context.CancelFunc); ok {
								fn()
							}
						}
						go c.sendPresence(chatJID, types.ChatPresencePaused)
						return nil
					}
				}
			}
		}
		// Update content with directives stripped (even if TTS not applied)
		if ttsResult != nil {
			msg.Content = ttsResult.Text
		}
	}

	// Send media attachments first.
	if len(msg.Media) > 0 {
		for i, m := range msg.Media {
			caption := m.Caption
			if caption == "" && i == 0 && msg.Content != "" {
				caption = markdownToWhatsApp(msg.Content)
			}

			data, readErr := os.ReadFile(m.URL)
			if readErr != nil {
				return fmt.Errorf("read media file: %w", readErr)
			}

			waMsg, buildErr := c.buildMediaMessage(data, m.ContentType, caption)
			if buildErr != nil {
				return fmt.Errorf("build media message: %w", buildErr)
			}

			resp, sendErr := c.client.SendMessage(c.ctx, chatJID, waMsg)
			if sendErr != nil {
				return fmt.Errorf("send whatsapp media: %w", sendErr)
			}
			c.recordMessagePreview(ctx, string(resp.ID), mediaAttachmentPreview(mediaKindFromMIME(m.ContentType), m.URL, caption), resp.Timestamp)
		}
		msg.Content = remainingContentAfterMediaCaption(msg.Content, msg.Media[0].Caption, msg.Media[0].ContentType)
	}

	// Send text (chunked if exceeding limit).
	if msg.Content != "" {
		formatted := markdownToWhatsApp(msg.Content)
		chunks := chunkText(formatted, maxMessageLen)
		for _, chunk := range chunks {
			waMsg := &waE2E.Message{
				Conversation: new(chunk),
			}
			resp, err := c.client.SendMessage(c.ctx, chatJID, waMsg)
			if err != nil {
				return fmt.Errorf("send whatsapp message: %w", err)
			}
			c.recordMessagePreview(ctx, string(resp.ID), chunk, resp.Timestamp)
		}
	}

	// Stop typing indicator.
	if cancel, ok := c.typingCancel.LoadAndDelete(msg.ChatID); ok {
		if fn, ok := cancel.(context.CancelFunc); ok {
			fn()
		}
	}
	go c.sendPresence(chatJID, types.ChatPresencePaused)

	return nil
}

func remainingContentAfterMediaCaption(content, caption, contentType string) string {
	if strings.HasPrefix(strings.ToLower(contentType), "audio/") {
		return content
	}
	if caption == "" || strings.TrimSpace(content) == strings.TrimSpace(caption) {
		return ""
	}
	return content
}

// buildMediaMessage uploads media to WhatsApp and returns the message proto.
func (c *Channel) buildMediaMessage(data []byte, mime, caption string) (*waE2E.Message, error) {
	switch {
	case strings.HasPrefix(mime, "image/"):
		uploaded, err := c.client.Upload(c.ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return nil, err
		}
		return &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:       new(caption),
				Mimetype:      new(mime),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(data))),
			},
		}, nil

	case strings.HasPrefix(mime, "video/"):
		uploaded, err := c.client.Upload(c.ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return nil, err
		}
		return &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				Caption:       new(caption),
				Mimetype:      new(mime),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(data))),
			},
		}, nil

	case strings.HasPrefix(mime, "audio/"):
		uploaded, err := c.client.Upload(c.ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return nil, err
		}
		return &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				Mimetype:      new(mime),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(data))),
			},
		}, nil

	default: // document
		uploaded, err := c.client.Upload(c.ctx, data, whatsmeow.MediaDocument)
		if err != nil {
			return nil, err
		}
		return &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				Caption:       new(caption),
				Mimetype:      new(mime),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(data))),
			},
		}, nil
	}
}

// keepTyping sends "composing" presence repeatedly until ctx is cancelled.
func (c *Channel) keepTyping(ctx context.Context, chatJID types.JID) {
	c.sendPresence(chatJID, types.ChatPresenceComposing)
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendPresence(chatJID, types.ChatPresenceComposing)
		}
	}
}

// sendPresence sends a WhatsApp chat presence update.
func (c *Channel) sendPresence(to types.JID, state types.ChatPresence) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}
	if err := c.client.SendChatPresence(c.ctx, to, state, ""); err != nil {
		slog.Debug("whatsapp: presence update failed", "state", state, "error", err)
	}
}
