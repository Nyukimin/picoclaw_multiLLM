package main

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
)

func TestBuildChannelRegistry(t *testing.T) {
	cfg := &config.Config{
		Line: config.LineConfig{
			ChannelSecret: "secret",
			AccessToken:   "token",
		},
		Telegram: config.TelegramConfig{BotToken: "tg-token"},
		Discord:  config.DiscordConfig{BotToken: "dc-token"},
		Slack:    config.SlackConfig{BotToken: "sl-token", SigningSecret: "sl-secret"},
	}

	r := buildChannelRegistry(cfg)
	names := r.List()
	if len(names) != 4 {
		t.Fatalf("expected 4 channels, got %d (%v)", len(names), names)
	}
}
