# Kuro人格復旧 Runbook

## 目的
LINEでKuro人格が崩れたときに、最短で復旧する。

## 想定する症状
- 「くろいますか？」への返答が、汎用ヘルプデスク調になる
- Kuro自身を認識しない文面が返る
- 直前まで正常だったのに急に人格が弱くなる

## 主な原因
1. 実運用モデルが `kuro-v2` ではなく `kuro-v1` になっている
2. LINEセッション履歴に崩れた応答が残り、会話文脈を汚染している
3. サービスが古い `~/.local/bin/picoclaw` バイナリで動いている

## 復旧手順
### 1) 現在の設定を確認
`~/.picoclaw/config.json` を確認し、以下を `ollama/kuro-v2:latest` にする。
- `agents.defaults.model`
- `routing.llm.chat_model`

### 2) 汚染セッションを削除
対象ユーザーのセッションファイルを削除する。
- 例: `~/.picoclaw/workspace/sessions/line_<user_id>.json`

### 3) バイナリを最新化
リポジトリルートで実行:
```bash
go build -o "/home/nyukimi/.local/bin/picoclaw" ./cmd/picoclaw
```

### 4) ゲートウェイ再起動
```bash
systemctl --user restart picoclaw-gateway.service
```

### 5) ヘルス確認
```bash
curl -sS http://127.0.0.1:18790/health
curl -sS http://127.0.0.1:18790/ready
```
どちらも `status: ok/ready` を返せば正常。

## エミュレート検証（実機不要）
LINE webhookを署名付きでローカル送信し、ログを確認する。

期待値:
- `initial_route=CHAT`
- 返答がKuro人格（例: 「れんさん！...Kuroです。」）

## 追加チェック
- `~/.picoclaw/workspace/CHAT_PERSONA.md` が存在し、最新ルールになっている
- `~/.config/systemd/user/picoclaw-gateway.service` の `ExecStart` が `~/.local/bin/picoclaw gateway` を参照している

## 再発防止
- モデル更新時は `~/.picoclaw/config.json` と `.env` の両方を確認
- コード修正後は再ビルドを必須化
- 人格崩れ時はセッションリセットを第一候補にする

## 人格変更時の標準コマンド（コピペ用）
人格関連ファイルを更新したら、次を順に実行する。

```bash
# 1) バイナリ更新
go build -o "/home/nyukimi/.local/bin/picoclaw" ./cmd/picoclaw

# 2) サービス再起動
systemctl --user restart picoclaw-gateway.service

# 3) ヘルス確認
curl -sS http://127.0.0.1:18790/health
curl -sS http://127.0.0.1:18790/ready

# 4) ルーティング確認（LINEはCHAT強制）
rg "line_forced_chat" pkg/agent/loop.go
```

## 対象ファイル（人格変更）
- `~/.picoclaw/workspace/CHAT_PERSONA.md`
- `docs/05_LLM運用プロンプト設計/Kuro_SYSTEM_prompt_v1.txt`
- `docs/05_LLM運用プロンプト設計/Kuro_キャラクター設定_v1.md`
