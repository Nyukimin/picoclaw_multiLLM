package main

import (
	"context"
	"log"
	"strings"

	adapterchannels "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/heartbeat"
)

// channelNotificationSender sends Heartbeat notifications through the configured channel adapter.
type channelNotificationSender struct {
	registry *adapterchannels.Registry
	channel  string
	chatID   string
}

func buildHeartbeatNotificationSender(cfg *config.Config) heartbeat.NotificationSender {
	channel := strings.ToLower(strings.TrimSpace(cfg.Heartbeat.Channel))
	chatID := strings.TrimSpace(cfg.Heartbeat.ChatID)
	if channel == "" && chatID != "" {
		channel = "line"
	}
	if channel == "" && chatID == "" {
		return nil
	}
	return &channelNotificationSender{
		registry: buildOutboundChannelRegistry(cfg),
		channel:  channel,
		chatID:   chatID,
	}
}

func (s *channelNotificationSender) SendNotification(ctx context.Context, message string) error {
	if s.channel == "" {
		log.Printf("[Heartbeat] notification skipped: heartbeat.channel not set")
		return nil
	}
	if s.chatID == "" {
		log.Printf("[Heartbeat] notification skipped: heartbeat.chat_id not set (channel=%s)", s.channel)
		return nil
	}
	adapter, ok := s.registry.Get(s.channel)
	if !ok {
		log.Printf("[Heartbeat] notification skipped: channel adapter not configured (channel=%s)", s.channel)
		return nil
	}
	return adapter.Send(ctx, s.chatID, message)
}
