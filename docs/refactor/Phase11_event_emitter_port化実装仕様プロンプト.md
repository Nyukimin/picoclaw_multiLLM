# Phase11 event emitter port 化実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase11 として、MessageOrchestrator に残っている event emitter を小さな private port / collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase11: event emitter port 化

目的:
  - `MessageOrchestrator.emit` / `emitMessageReceived` の責務を、Viewer event 用の小さな port 境界として整理する。
  - Viewer event と execution log / response text / audio chunk / lipsync を混同しない。
  - nil listener の skipped log と既存 event order を維持する。
  - Phase6 route chain、Phase8 collaborator、Phase9 route dispatcher、Phase10 TTS lifecycle 契約を維持する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase8_完了判定.md
  9. docs/refactor/Phase9_完了判定.md
  10. docs/refactor/Phase10_完了判定.md
  11. docs/codebase-map/アーキテクチャ総合.md
  12. docs/codebase-map/結合ポイントマップ.md
  13. docs/codebase-map/ユースケース逆引き.md
  14. docs/codebase-map/modules/application.md
  15. docs/codebase-map/modules/潜在バグ一覧.md
  16. internal/application/orchestrator/event.go
  17. internal/application/orchestrator/message_orchestrator_events.go
  18. internal/application/orchestrator/message_orchestrator.go
  19. internal/application/orchestrator/message_orchestrator_routes.go
  20. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  21. internal/application/orchestrator/message_orchestrator_*test.go
  22. internal/application/orchestrator/code_executor*.go

作成する文書:
  - docs/refactor/Phase11_event_emitter_port化実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat、STT/TTS provider、LLM provider、runtime config の挙動は変更しない。
  - task context builder は同時に collaborator 化しない。
  - distributed orchestrator へ広げない。

文書に必ず含める内容:

  1. Phase11 の目的
  2. 対象範囲
     - `message_orchestrator_events.go`
     - `event.go`
     - `MessageOrchestrator` constructor / SetEventListener
     - collaborator に渡している `messageEventEmitter`
     - CodeExecutor event emitter 接続
  3. 対象外
  4. 現在の event emitter 構造
     - nil listener skipped log
     - `NewEvent`
     - `emitMessageReceived`
     - collaborator へ function として渡っている点
  5. 提案する collaborator
     - `messageEventPort`
     - private struct
     - 初期段階では interface 化しない
     - `MessageOrchestrator` field として持つ
     - `SetEventListener` 後に最新 listener を使う
     - `Emit` / `EmitMessageReceived` method を持つ
     - function adapter として collaborator / CodeExecutor に渡せる
  6. `messageEventPort` の契約
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 差し替え可能性
     - 変更してはいけない既存挙動
  7. 実装手順
  8. テスト方針
     - baseline / after:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
       ```
     - route chain:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
       ```
     - TTS:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder'
       ```
     - event:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase11|TestMessageOrchestrator_RouteChainContract_'
       ```
     - `git diff --check`
  9. リスク
     - route chain event order を壊す。
     - nil listener を panic に変える。
     - Viewer event と execution log を混同する。
     - raw content / response text / audio chunk を同じ扱いにする。
     - CodeExecutor への event emitter 接続を落とす。
  10. 完了条件

実行手順:
  1. 参照文書を読む。
  2. event emitter 周辺コードを確認する。
  3. port 境界と契約を棚卸しする。
  4. `docs/refactor/Phase11_event_emitter_port化実装仕様.md` を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
