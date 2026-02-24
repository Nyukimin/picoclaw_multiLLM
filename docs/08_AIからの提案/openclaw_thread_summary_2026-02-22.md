# OpenClawスレッドまとめ（2026-02-22）

## 0. まとめ（結論）
- OpenClawは「チャット入口（LINE/Slack等）→ AI → ツール実行（コマンド/ファイル/ブラウザ/定期実行/WebHook）」を**常駐型のGateway**に統合したOSS。
- 強みは、**マルチエージェント分離（Chat/Worker/Coderの役割分担）**と、**“会話のまま作業が走る”**体験を土台から作れる点。
- 評判の論点は二極化しやすい：  
  - 価値：自前で常駐AI＋作業実行ができる  
  - 代償：権限を渡しやすく、**プロンプトインジェクション/拡張（スキル）経由の事故**が現実に問題化している  
- れんの要件（Chat/Worker/Coder、Spawn禁止）は、OpenClawの設計に**かなり素直に載る**。鍵は「入口でrouting」「spawnをdeny」「send中心」。

---

## 1. れんの依頼（このスレッドのゴール）
- 「OpenClawって何ができるのか」を、世間の評判込みでしっかり調査。
- そのうえで、れんの運用（Chat/Worker/Coder、Subagentは使うがSpawn禁止）に落として「実現手段」を提示。
- LINE/Slackなど“入口”前提で、満足できる粒度の回答が欲しい。

---

## 2. OpenClawで“できること”（回答の要点）
### 2-1. 入口（チャネル）統合
- WhatsApp/Telegram/Slack/Discord/Teams/WebChat等の“入口”をGatewayへ接続し、同じアシスタントにつなぐ。
- LINEはプラグイン扱いで、LINE Messaging APIのWebhookでGatewayに入れる。

### 2-2. ツール実行（手足）
- コマンド実行（bash/process系）
- ファイル操作（read/write/edit系）
- ブラウザ操作（browser系）
- 定期実行/ゲートウェイ運用（cron/gateway系）
- エージェント間連携（sessions系）

### 2-3. マルチエージェント（役割分担）
- “エージェント”単位で、ワークスペース・ルール・履歴などを分離して運用できる。
- 入口やアカウントごとに、どのAgentへ流すか（bindings/routing）を決められる。

### 2-4. ローカルLLM（Ollama）も射程内
- Ollamaをプロバイダとして統合して運用できる（ローカル常駐＋無料寄り運用に繋がる）。

---

## 3. 世間の評判（このスレッドで触れた論点）
### 3-1. ポジティブ
- “チャットにいるのに、裏で作業できる常駐AI”という体験が強い。
- Gatewayにチャネル・ツール・スケジュール・WebUI等が集約され、全体構造が把握しやすい、という評価が出やすい。

### 3-2. ネガティブ（構造的リスク）
- プロンプトインジェクションや、拡張（スキル）経由の攻撃/事故が実際に問題化しており、警鐘・制限・禁止の動きが出ることがある。
- 便利さ（権限）とリスクが同時に増えるタイプのプロダクト。

---

## 4. れん構成（Chat/Worker/Coder、Spawn禁止）への落とし込み
### 4-1. 推奨の骨格：Single Gateway / 3 Agents
- chat：LINE/Slackの入口、軽作業（会話・画像評価など）
- worker：重い作業（調査・整形・実行系）。外部から直接触れさせない運用が基本
- coder：コード生成（外部LLMを使うならここに寄せる）

### 4-2. Spawn禁止の実装観点
- `sessions_spawn` を使わせない（deny）  
- 連携は `sessions_send`（既存の相手に投げる）中心  
- 役割分担（routing）は“入口で決める”＝bindingsで固定化

#### 概念スケッチ（例：実際のキーはOpenClawの設定仕様に合わせて調整）
```json
{
  "agents": {
    "list": [
      { "id": "chat", "default": true, "workspace": "~/.openclaw/ws-chat" },
      { "id": "worker", "workspace": "~/.openclaw/ws-worker" },
      { "id": "coder", "workspace": "~/.openclaw/ws-coder" }
    ]
  },
  "bindings": [
    { "agentId": "chat", "match": { "channel": "line", "accountId": "home" } },
    { "agentId": "chat", "match": { "channel": "slack", "accountId": "home" } },
    { "agentId": "worker", "match": { "channel": "slack", "accountId": "internal-worker" } },
    { "agentId": "coder",  "match": { "channel": "slack", "accountId": "internal-coder" } }
  ],
  "tools": {
    "deny": ["sessions_spawn"]
  }
}
```

---

## 5. LINE接続の確定事項（このスレッドで整理した点）
- LINEはWebhookでGatewayに入る。
- Webhook URLは、LINE Developers（Messaging API設定）のWebhook URL欄に設定する。
- `webhookPath` を変えた場合は、URLもそれに合わせる。

---

## 6. 最低限のセキュリティ柵（“評判の事故”を踏まえた現実ライン）
- Gatewayを外に晒す前提なら、まずは露出面積を小さくする（例：loopback/内向き運用→段階的に公開）。
- グループ入力や外部入力が混じるセッションは、サンドボックス化（Docker等）で爆心地を小さくする。
- “自動スキル取得/外部拡張”は、初期はオフ寄りで設計し、必要最小限から段階導入。

---

## 7. 次のステップ（決めるべきことリスト）
- 入口：LINEのみか、Slackも同時に入口にするか（運用の主入口を固定）
- bindings：どのチャネル/アカウントを chat に流すか（固定）
- tools：worker/coderに許可するツール範囲（特に `exec`/`browser`/`write` などの強権限）
- spawn禁止の徹底：deny対象のツール確認（sessions_spawn以外も必要なら追加）
- 露出：Tailscale/公開URL/リバプロなど、どこまで外に出すか（段階設計）

---

## 8. 参照（このスレッドで提示した主な情報源）
※URLはそのまま貼ると流用時に事故りやすいので、コードブロックに隔離。

```text
OpenClaw GitHub
https://github.com/openclaw/openclaw

OpenClaw Docs（例）
https://docs.openclaw.ai/channels/line
https://docs.openclaw.ai/tools
https://docs.openclaw.ai/gateway/configuration
https://docs.openclaw.ai/providers/ollama
https://docs.openclaw.ai/gateway/security

LINE Messaging API（Webhook）
https://developers.line.biz/ja/reference/messaging-api/

（評判・論点として参照したニュース/記事）
https://www.reuters.com/...
https://www.theverge.com/...
https://www.wired.com/...
https://www.techradar.com/...
https://www.businessinsider.com/...
```

---

### 備考
- このまとめは「このスレッド内で扱った要点」に限定している（OpenClaw全機能の網羅ではない）。
- 実装に入る段階では、OpenClawの`config`（agents/bindings/tools/providers）を、れんの現行ネットワーク構成（Tailscale/Ubuntu gateway）に合わせて確定させるのが最短。
