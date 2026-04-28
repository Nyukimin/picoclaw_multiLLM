# STT Client-Serverリクエスト実装仕様（AUDIO_Client仕様）

## 1. 目的
本書は、`Client(Viewer/Browser)` から `Server(RenCrow STT Gateway)` へ送るリクエスト仕様を実装レベルで定義する。  
Server 内部の Provider 実装差分は本書の対象外とし、Client から見える契約のみを扱う。

## 2. スコープ
- 対象:
  - Client -> Server の WebSocket 接続
  - Client -> Server の制御メッセージ
  - 音声バイナリ送信
  - 受信イベント（`speech_start`/`draft`/`final`/`error`）の扱い
- 非対象:
  - Server -> Provider 実装詳細
  - STT モデル選定/学習
  - UIデザイン

## 3. 責務分離（必須）
- Client の責務:
  - マイク開始/停止（ユーザー操作にのみ連動）
  - 音声データ送信
  - Server 返信イベントの表示
- Server の責務:
  - 無音判定/発話区切り判定
  - `draft` / `final` の確定
  - timeout/retry/fail-open の制御

不変条件:
- Client は無音判定ロジックを持たない。
- Client は STT 接続断を契機にマイクを自動停止しない（再接続は通信層で吸収）。

## 4. 接続仕様

### 4.1 Endpoint
- URL: `wss://<chat-host>/stt-ws`（HTTPS環境）
- ローカル検証: `ws://127.0.0.1:18790/stt-ws`

### 4.1.1 マイク利用の前提（Secure Context）
- Browser の `getUserMedia` は **Secure Context** でのみ利用可能。
- 実運用では `https://<chat-host>:18790/viewer` + `wss://<chat-host>:18790/stt-ws` を必須とする。
- 例外としてローカル開発の `http://localhost` / `http://127.0.0.1` は許容される。
- `http://192.168.x.x` や `http://100.x.x.x`（Tailnet IP含む）はページ表示できても、マイク入力はブラウザ制約で利用不可とする。

### 4.2 接続ライフサイクル
1. User がマイク開始
2. Client が `/stt-ws` へ接続
3. `config` 送信
4. 音声バイナリを周期送信
5. User 停止時に `final_pending` 送信（任意だが推奨）
6. 接続クローズ

## 5. Client -> Server リクエスト契約

### 5.1 `config`（JSON）
```json
{ "type": "config", "mimeType": "audio/wav" }
```

要件:
- 接続直後に1回送る
- 互換のため `mimeType` は必須

### 5.2 音声バイナリ（binary）
- 推奨形式: WAV (mono, 16kHz, PCM16)
- 送信間隔目安: 300ms〜700ms
- 最小送信サイズは Server 側設定に従う

### 5.3 `final_pending`（JSON）
```json
{ "type": "final_pending" }
```

用途:
- ユーザー停止時に最終確定を促すための明示トリガー

## 6. Server -> Client 応答イベント契約

### 6.1 `speech_start`
```json
{ "type": "speech_start" }
```

### 6.2 `draft`
```json
{ "type": "draft", "text": "..." }
```

### 6.3 `final`
```json
{ "type": "final", "text": "..." }
```

### 6.4 `error`
```json
{ "type": "error", "error": "..." }
```

### 6.5 `status`（任意運用）
```json
{ "type": "status", "text": "stt provider timeout (retrying)" }
```

## 7. エラー時の Client 挙動（必須）
- `error` 受信時:
  - セッション継続（即停止しない）
  - ユーザーに非破壊通知
- `status` 受信時:
  - 進行状態として表示
  - エラー扱いにしない
- WebSocket切断時:
  - 録音状態は維持
  - 接続のみ再試行（短いバックオフ）

## 8. 実装チェックリスト（Client側）
- [ ] マイク開始/停止がユーザー操作にのみ連動する
- [ ] マイク利用時のUI接続先が Secure Context（HTTPS または localhost）である
- [ ] `/stt-ws` 接続直後に `config` を送る
- [ ] 音声バイナリを定周期で送信する
- [ ] `speech_start`/`draft`/`final`/`error` を表示できる
- [ ] WS切断時に録音を止めず再接続する
- [ ] 停止時 `final_pending` を送る

## 9. 検証コマンド（最小）
```bash
# 接続確認（Server側）
curl -i -sS https://<chat-host>/stt-ws

# WebSocket疎通
wscat -c wss://<chat-host>/stt-ws --no-check
# -> {"type":"config","mimeType":"audio/wav"} を送信

# Server準備状態
curl -sS https://<chat-host>/ready
```

補足:
- マイクを使う検証は `https://...`（または `http://localhost`）で実施すること。
- `http://<LAN or Tailnet IP>:18790/viewer` はページ疎通確認用途に限定し、音声入力検証には使用しないこと。

## 10. 補足
- 本書は「Client -> Server リクエスト契約」の正本である。
- Server -> Provider の詳細は `AUDIO_Server仕様/STT` 側文書を参照する。

## 11. 本書の実装に必要な情報（作業の意味と実態リンク）

### 11.1 この作業の意味
- Client 側実装者が「何を送るべきか」「何を受け取るべきか」を固定し、実装ごとの解釈差をなくすため。
- Server 側変更時に、Client 互換性が壊れていないかを確認する基準点にするため。
- 仕様・実装・検証（DoD）の三点を同じ用語でつなぎ、保守コストを下げるため。

### 11.2 まず確認すべき正本（仕様）
- STT API 契約: `STT_API.md`
- STT 要求仕様: `STT仕様.md`
- STT 実装仕様: `STT実装仕様.md`
- 現状サマリ: `STT_Server側_現状サマリ.md`
- DoD 記録: `STT_API_DOD_2026-04-10.md`

### 11.3 各項目と実態の対応

| 本書の論点 | 作業の意味 | 実態 |
|---|---|---|
| 接続エンドポイント (`/stt-ws`) | Client が接続失敗しないための入口固定 | voice-bridge の `WebSocketServer` path 設定 |
| `config` 送信 (`mimeType` 必須) | 音声フォーマット解釈を一致させるため | `msg.type === 'config'` 分岐 |
| binary 音声送信 | STT 推論の入力品質を担保するため | WAV 判定・VAD 処理 |
| `final_pending` | 停止操作時の最終確定制御を明示するため | final 待ち処理 |
| `speech_start`/`draft`/`final` | Client 表示更新タイミングを統一するため | `send({ type: ... })` |
| `error`/`status` | 障害時も UX を壊さず継続運用するため | エラー送信・ログ出力 |

### 11.4 現状差分（仕様 vs 実装）と対応優先度

| 優先度 | 論点 | 仕様（本書） | 現状 | 対応方針 |
|---|---|---|---|---|
| 高 | WS エンドポイント | `/stt-ws` | `/ws` | `/stt-ws` を正規入口として実装、`/ws` は後方互換に残す |
| 高 | `final_pending` の意味 | 最終確定を促す明示トリガー | 実装差あり | 主系実装を1つの契約に統一する |
| 中 | `error` ペイロード | `{ "type": "error", "error": "..." }` | `{ "type": "error", "text": "..." }` | キー仕様を統一する |
| 中 | `status` イベント | 任意だが契約例あり | 未送信 | timeout/retry/fail-open 状態の可視化 |
| 低 | ローカル検証値 | `ws://127.0.0.1:18790/stt-ws` | 既定ポート 8090 系 | 運用ポート方針を仕様に合わせて明文化 |
