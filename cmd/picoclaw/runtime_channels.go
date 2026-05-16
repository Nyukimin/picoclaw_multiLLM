package main

import (
	"strings"

	discordadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/discord"
	slackadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/slack"
	telegramadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/telegram"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
)

func buildChannelRuntimeHandlers(cfg *config.Config, deps *Dependencies, proc messageProcessor) {
	deps.lineHandler = line.NewHandler(proc, cfg.Line.ChannelSecret, cfg.Line.AccessToken)
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		tg := telegramadapter.NewAdapter(cfg.Telegram.BotToken, proc)
		tg.SetWebhookSecret(cfg.Telegram.WebhookSecret)
		deps.telegramHandler = tg
	}
	if strings.TrimSpace(cfg.Discord.BotToken) != "" {
		dc := discordadapter.NewAdapter(cfg.Discord.BotToken, proc)
		dc.SetPublicKeyHex(cfg.Discord.PublicKey)
		deps.discordHandler = dc
	}
	if strings.TrimSpace(cfg.Slack.BotToken) != "" {
		deps.slackHandler = slackadapter.NewAdapter(cfg.Slack.BotToken, cfg.Slack.SigningSecret, proc)
	}
}
