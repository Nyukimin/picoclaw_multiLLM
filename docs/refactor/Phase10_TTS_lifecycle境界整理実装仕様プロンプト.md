# Phase10 TTS lifecycle 境界整理実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase10 として、MessageOrchestrator に残っている TTS / VTuber lifecycle 境界を整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase10: TTS lifecycle 境界整理

目的:
  - `MessageOrchestrator` に残っている TTS session start / end / stream hook / push の責務を、意味のある private collaborator 境界へ整理する。
  - route dispatcher から呼ばれる TTS / VTuber push と stream finalize の契約を明確にする。
  - 音声、口パク、Viewer 表示本文、event log、execution log を混同しない。
  - TTS degraded を success として扱わない。
  - Phase6 route chain、Phase8 collaborator、Phase9 route dispatcher 契約を維持する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md
  9. docs/refactor/Phase8_完了判定.md
  10. docs/refactor/Phase9_route_dispatcher境界整理実装仕様.md
  11. docs/refactor/Phase9_完了判定.md
  12. docs/codebase-map/アーキテクチャ総合.md
  13. docs/codebase-map/結合ポイントマップ.md
  14. docs/codebase-map/ユースケース逆引き.md
  15. docs/codebase-map/modules/application.md
  16. docs/codebase-map/modules/潜在バグ一覧.md
  17. internal/application/orchestrator/message_orchestrator.go
  18. internal/application/orchestrator/message_orchestrator_routes.go
  19. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  20. internal/application/orchestrator/tts_support.go
  21. internal/application/orchestrator/vtuber_stream.go
  22. internal/application/orchestrator/stream_bundle.go
  23. internal/application/orchestrator/message_orchestrator_*test.go

作成する文書:
  - docs/refactor/Phase10_TTS_lifecycle境界整理実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat、STT/TTS provider、LLM provider、runtime config の挙動は変更しない。
  - route dispatcher、event emitter、task context builder を同時に collaborator 化しない。

文書に必ず含める内容:

  1. Phase10 の目的
  2. 対象範囲
     - `message_orchestrator_tts_lifecycle.go`
     - `startTTSSessionForRoute`
     - `endTTSSession`
     - `withStreamHooks`
     - `pushTTS`
     - `speechModeForRoute`
     - `tts_support.go`
     - `vtuber_stream.go`
     - `stream_bundle.go`
     - `MessageOrchestrator` constructor / setter / route dispatcher dependency
  3. 対象外
     - event emitter port 化
     - task context builder
     - route dispatcher 追加分割
     - provider 挙動変更
     - Viewer / IdleChat / STT / runtime config
  4. 現在の TTS lifecycle 構造
  5. 提案する collaborator
     - `messageTTSLifecycle`
     - private struct
     - 初期段階では interface 化しない
     - `MessageOrchestrator` field として持つ
     - `SetTTSBridge` / `SetVTuberBridge` 後に最新 bridge を使う
     - route dispatcher へ stream hook / push function として渡す
  6. `messageTTSLifecycle` の契約
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
     - TTS:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
       ```
     - route chain:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase9|TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
       ```
     - `git diff --check`
  9. リスク
     - TTS degraded を success と混同する。
     - stream callback の前段 callback を落とす。
     - Viewer 表示本文を音声 chunk に依存させる。
     - 口パクと音声 push の境界を混同する。
     - route dispatcher の event order を壊す。
     - provider 挙動変更に踏み込む。
  10. 完了条件

実行手順:
  1. 参照文書を読む。
  2. TTS lifecycle 周辺コードを確認する。
  3. collaborator 境界と契約を棚卸しする。
  4. `docs/refactor/Phase10_TTS_lifecycle境界整理実装仕様.md` を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
