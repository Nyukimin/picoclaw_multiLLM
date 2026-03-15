# 詳細実装仕様 07: App/Platform導線の差

**作成日**: 2026-03-09  
**ステータス**: In Progress  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 0. OpenClaw原典の理解

- Platforms: <https://docs.openclaw.ai/platforms>
- Channels: <https://docs.openclaw.ai/channels>
- Gateway: <https://docs.openclaw.ai/gateway>

OpenClawは利用者導線（Chat UI/CLI/連携アプリ）から同一実行基盤へ収束させる。

---

## 1. 現状差分

- RenCrow現状: LINE + Viewer + CLIが個別導線で、開始/終了/証跡のユーザー体験が分断。
- 差分の本質: プラットフォーム横断のセッション導線と状態提示不足。

---

## 2. 実装対象

1. Unified Entry API（platform非依存の入力窓口）
2. セッション導線統一（開始/進行/完了イベント）
3. 実行状況カード（進捗、承認待ち、完了証跡）の標準化
4. Claude in Chrome / Canvas導線の状態同期

---

## 3. 契約仕様

### 3.1 Unified Entry

```json
{
  "platform": "line|viewer|cli|chrome",
  "channel": "line|telegram|discord|slack|local",
  "user_id": "...",
  "session_id": "optional",
  "message": "TTS実装して"
}
```

### 3.2 進行イベント

```json
{
  "session_id": "...",
  "stage": "received|planning|applying|verifying|completed|failed",
  "summary": "...",
  "evidence_ref": "execution_report:job-123"
}
```

---

## 4. 配置

- `internal/adapter/http/unified_entry_handler.go`
- `internal/application/session/journey_service.go`
- `internal/adapter/viewer/progress_publisher.go`
- `internal/adapter/chrome/bridge_handler.go`

---

## 5. TDD計画

1. platform別入力正規化テスト
2. session_id引き継ぎテスト
3. 進行イベント順序テスト
4. 完了時にevidence参照が必ず付くテスト
5. UI未接続時のフォールバック通知テスト

受け入れ基準:
- どの入口でも同一ステージ遷移を観測可能
- 完了通知から証跡参照まで1ホップで到達可能

---

## 6. 未決事項

1. Chrome bridgeをWebSocket常時接続にするかSSEにするか
2. Platformごとの通知粒度（全ステージ通知/要点のみ）をどう統一するか

---

## 7. 実装進捗（2026-03-09）

- 実装済み
  - `/entry` の統一導線（platform/channel/user/session/message）
  - `platform/channel` 正規化（`cli/chrome -> local`、不正値フォールバック）
  - `received/planning/applying/verifying/completed/failed` ステージ通知
  - `entry.stage` を EventHub へ発火（session単位の進行同期）
  - Chrome bridge:
    - `POST /chrome/bridge`（Unified Entry経由で実行、`request_id`/`accepted_at` ACK返却）
    - `GET /chrome/bridge/status?session_id=...`（最新ステージ参照）
    - `GET /chrome/bridge/events?session_id=...`（SSEでセッションイベントをpush受信）
  - SSE再接続境界:
    - EventHubイベントに `seq` を付与
    - `Last-Event-ID` を用いた履歴再送フィルタ（viewer/chrome両方）
  - `TTS` 指示時の Autonomous Executor 適用
  - `execution_report.jsonl` への証跡保存
  - `/viewer/evidence/recent` で証跡一覧取得（Viewer UI表示）
  - `/viewer/evidence/detail?job_id=...` で証跡詳細参照（Viewerから選択表示）
  - `/viewer/evidence/summary` で status/error_kind 集計を取得（Viewer summaryカード表示）
  - Viewerで証跡詳細を整形表示（steps/verification/error）

- 次段
  - Platform別の通知粒度設定（全段通知 or 要点通知）
  - Chrome bridge ACKの一意性保証（重複 request_id の冪等化）
