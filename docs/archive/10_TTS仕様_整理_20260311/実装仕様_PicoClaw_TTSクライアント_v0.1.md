# 実装仕様: RenCrow TTSクライアント v0.1

**作成日**: 2026-03-10  
**対象ブランチ**: `integration/openclaw-parity`  
**正本参照**: `docs/TTS仕様/RenCrow_TTSクライアント仕様.md`

---

## 1. 目的

本仕様は、RenCrow 側 TTS クライアント連携の **実装済みスコープ** を定義する。  
対象は「仕様 v0.1 に対して今回コードへ反映した内容」であり、未実装項目を明確化する。

---

## 2. 実装範囲（今回）

### 2.1 TTS Bridge（オーケストレータ統合）

- `internal/application/orchestrator/tts_bridge.go` に `TTSBridge` を定義
- `MessageOrchestrator` / `DistributedOrchestrator` に注入ポイント追加
  - `SetTTSBridge(...)`
- 各 `ProcessMessage` の 1 応答を 1 TTS session として扱う
  - start: ルーティング決定後
  - push: ストリームtoken / 最終応答テキスト
  - end: 応答完了後

### 2.2 TTS Client（infrastructure）

- `internal/infrastructure/tts/client_bridge.go`
  - `/health/ready` 事前確認
  - `WS /sessions` 接続
  - 送信:
    - `session_start`
    - `text_delta`（`seq` 連番）
    - `session_end`
  - 受信:
    - `audio_chunk_ready`
    - `session_completed`
    - `error`

### 2.3 Audio Chunk Reorder / Audio Sink

- `internal/infrastructure/tts/reorder_buffer.go`
  - `chunk_index` 順序制御
  - 欠番待機
  - gap timeout 超過時のスキップ
  - `session_completed` 時の強制ドレイン
- `internal/infrastructure/tts/audio_sink.go`
  - `audio_path` を既存 `CommandPlayer` で再生
  - 再生失敗時はログ記録（継続可能）

### 2.4 アプリ配線（main）

- `cmd/picoclaw/tts_client_bridge.go`
  - `tts.enabled=true` の場合に Bridge 構築
  - `tts.playback_commands` 必須
- `cmd/picoclaw/main.go`
  - local/distributed 両オーケストレータに Bridge 注入

---

## 3. 設定仕様（実装済み）

`tts` セクションに以下を追加。

- `http_base_url`（default: `http://127.0.0.1:8765`）
- `ws_url`（default: `ws://127.0.0.1:8765/sessions`）
- `connect_timeout_ms`（default: `3000`）
- `receive_timeout_ms`（default: `15000`）
- `chunk_gap_timeout_ms`（default: `3000`）
- `voice_id`（default: `female_01`）
- `speech_mode`（default: `conversational`）

既存の `tts.enabled` を有効化フラグとして使用する。

---

## 4. TDD（追加テスト）

### 4.1 Orchestrator

- `internal/application/orchestrator/message_orchestrator_test.go`
  - TTS bridge start/push/end 呼び出し
  - start失敗時の degraded 継続
- `internal/application/orchestrator/distributed_orchestrator_test.go`
  - distributed 経路での start/end 呼び出し

### 4.2 Infrastructure

- `internal/infrastructure/tts/reorder_buffer_test.go`
  - 順序排出
  - 欠番timeoutスキップ
  - 強制ドレイン

### 4.3 main / config

- `cmd/picoclaw/tts_client_bridge_test.go`
  - bridge 生成条件（enabled on/off）
- `internal/adapter/config/config_test.go`
  - 新規設定項目の読み込み検証

---

## 5. 失敗時挙動（実装済み方針）

- health/WS/start/push/end の失敗は **degraded mode** として扱う
- 失敗時も `ProcessMessage` 本体は継続（TTS のみ部分停止）
- ログイベント:
  - `tts_health_check_start`
  - `tts_health_check_ready`
  - `tts_session_start_sent`
  - `tts_text_delta_sent`
  - `tts_audio_chunk_received`
  - `tts_session_end_sent`
  - `tts_session_completed_received`
  - `tts_error_received`
  - `tts_session_abort`

---

## 6. 未実装 / 今後

- `session_started` を使った厳密な開始ACK待ち
- `pause_after` を再生制御へ反映
- マルチ voice 選択ロジック（route/context ベース）
- `Audio Sink` の保存モード/転送モード分離
- 実機 runbook に基づく E2E 証跡化

---

## 7. 完了判定（今回）

次を満たしたため、本実装仕様 v0.1 を「実装済み」とする。

1. 全経路で TTS bridge が注入される
2. `start -> delta -> end` の一連送信が機能する
3. chunk reorder + playback の基礎挙動が実装される
4. 追加テストを含め `go test ./...` が通過する

