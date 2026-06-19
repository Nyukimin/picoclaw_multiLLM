---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: channels
---

# channels モジュール解析

## 概要

`pkg/channels/` 以下に各プラットフォーム向けのチャネルハンドラーが実装されている。
全チャネルは共通インターフェース（Bot インターフェース相当）を通じて `pkg/bus/` の MessageBus に接続し、
inbound メッセージを publish / outbound を subscribe して送信する。

## パッケージ一覧

| パッケージ | 対応サービス | 認証方式 |
|-----------|------------|---------|
| `pkg/channels/line` | LINE Messaging API | Channel Secret + Access Token |
| `pkg/channels/slack` | Slack Events API | Bot Token + Signing Secret |
| `pkg/channels/telegram` | Telegram Bot API | Bot Token (telego) |
| `pkg/channels/discord` | Discord Gateway | Bot Token (discordgo) |
| `pkg/channels/whatsapp` | WhatsApp Business API | — |
| `pkg/channels/dingtalk` | DingTalk (釘釘) | — |
| `pkg/channels/feishu` | Feishu (Lark) | — |
| `pkg/channels/qq` | QQ (OneBot) | — |
| `pkg/channels/maixcam` | MaixCam デバイス | — |
| `pkg/channels/onebot` | OneBot v11 Protocol | — |

## 主要構造体 / インターフェース

### Bot インターフェース（推定）

```go
type Bot interface {
    Start(ctx context.Context) error
    Stop() error
    SendMessage(chatID string, text string) error
}
```

各チャネルの実装が Bus と接続するパターン:

```go
// inbound: webhook/event → Bus
bus.PublishInbound(bus.InboundMessage{
    Channel:    "line",
    SenderID:   userID,
    ChatID:     chatID,
    Content:    text,
    SessionKey: sessionKey,
})

// outbound: Bus → send
msg := <-bus.SubscribeOutbound()
sendToChannel(msg.ChatID, msg.Content)
```

## LINE チャネル（最重要）

- main.go で HTTP POST `/webhook` に登録
- `LineSDK.ParseRequest()` で署名検証後メッセージ種別を振り分け
- テキストメッセージのみ AgentLoop に渡す
- **重要**: LINE チャネルは RoutingDecision を無視し、常に CHAT ルートが強制される（`loop.go` 側の制御）
- 返信は `LineClient.ReplyMessage()` または `PushMessage()` を使用

## Slack チャネル

- Events API (HTTP) + Socket Mode の両方をサポート
- `challenge` ハンドシェイクを自動返答
- `app_mention` イベントのみ AgentLoop へ渡す

## Telegram チャネル

- `mymmrac/telego` ライブラリ使用
- Long Polling または Webhook の切り替えが設定で可能

## Discord チャネル

- `bwmarrin/discordgo` ライブラリ使用
- `MessageCreate` イベントを受信
- Bot 自身のメッセージを無視する guard が必要

## 注意事項

- `pkg/channels/` の実装は `pkg/bus/` に依存するが、`internal/adapter/channels/` へ移行が計画中
- main.go の import は `internal/adapter/channels` を参照しており、現時点では乖離している（コンパイル不能）
- LINE 以外のチャネルのルーティング強制は未実装（全チャネルが CHAT になる可能性あり）

## 関連

- [bus_state_heartbeat.md](bus_state_heartbeat.md) — MessageBus 詳細
- [アーキテクチャ総合.md](../アーキテクチャ総合.md)
