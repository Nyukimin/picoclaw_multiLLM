# Phase9 route dispatcher 境界整理実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase9 として、Phase8 で method のまま残した MessageOrchestrator の route dispatcher 境界を整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase9: MessageOrchestrator route dispatcher 境界整理

目的:
  - `MessageOrchestrator` に残っている route-specific execution を、意味のある private collaborator 境界へ整理する。
  - Chat / Worker / Coder route chain を壊さず、route dispatch の入力、出力、副作用、ログ、エラー契約を明確にする。
  - Phase6 で固定した Shiro relay / CodeExecutor / WorkerExecutionService 契約を維持する。
  - Phase8 で collaborator 化した autonomous execution coordinator との接続を明確にする。
  - TTS lifecycle、event emitter、task context builder は Phase9 では原則 method のまま残し、同時に混ぜない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase6_route_chain統合確認実装仕様.md
  9. docs/refactor/Phase6_完了判定.md
  10. docs/refactor/Phase7_MessageOrchestrator分割実装仕様.md
  11. docs/refactor/Phase7_完了判定.md
  12. docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md
  13. docs/refactor/Phase8_完了判定.md
  14. docs/codebase-map/アーキテクチャ総合.md
  15. docs/codebase-map/結合ポイントマップ.md
  16. docs/codebase-map/ユースケース逆引き.md
  17. docs/codebase-map/modules/application.md
  18. docs/codebase-map/modules/domain.md
  19. docs/codebase-map/modules/潜在バグ一覧.md
  20. internal/application/orchestrator/message_orchestrator.go
  21. internal/application/orchestrator/message_orchestrator_routes.go
  22. internal/application/orchestrator/message_orchestrator_autonomous.go
  23. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  24. internal/application/orchestrator/message_orchestrator_events.go
  25. internal/application/orchestrator/code_executor*.go
  26. internal/application/orchestrator/message_orchestrator_*test.go

docs/codebase-map/ の使い方:
  - route dispatch 周辺の責務、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

作成する文書:
  - docs/refactor/Phase9_route_dispatcher境界整理実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat 契約、STT/TTS provider、LLM provider、runtime config の挙動は変更しない前提にする。
  - Phase6 route chain 契約を変更しない。
  - Phase8 collaborator 境界を崩さない。
  - fallback / degraded / error / handled success を混同しない。
  - Viewer 表示、音声、口パク、ログを混同しない。
  - 巨大な service / manager / helper / util を新設しない。
  - route dispatcher と TTS lifecycle / event emitter / task context builder を同時に collaborator 化しない。

文書に必ず含める内容:

  1. Phase9 の目的
     - route-specific execution の境界整理。
     - `MessageOrchestrator.ProcessMessage` の top-level orchestration 維持。
     - Phase6 / Phase8 契約維持。
     - 挙動変更ではなく構造整理であること。

  2. 対象範囲
     - `message_orchestrator_routes.go`
     - `executeTask`
     - `executeChatRoute`
     - `executeRouteDirect`
     - `executeOPSRoute`
     - `executeCodeRoute`
     - `executeWildRoute`
     - `executePlanRoute`
     - `executeAnalyzeRoute`
     - `executeResearchRoute`
     - `executeCodeViaShiro`
     - `MessageOrchestrator` constructor / field
     - route chain / TTS / Phase8 collaborator tests

  3. 対象外
     - TTS lifecycle collaborator 化。
     - event emitter collaborator 化。
     - task context builder collaborator 化。
     - CodeExecutor 再分割。
     - WorkerExecutionService 内部。
     - handler / DTO / SSE event。
     - Viewer JS / CSS。
     - IdleChat。
     - STT / TTS provider。
     - LLM provider。
     - runtime config。
     - distributed orchestrator。
     - 未追跡の tests/。

  4. 現在の route dispatcher 構造
     - `executeTask` の CHAT / autonomous route 分岐。
     - `executeChatRoute` の Mio chat / stream hook / event / TTS finalize。
     - `executeRouteDirect` の route switch。
     - OPS / CODE / WILD / PLAN / ANALYZE / RESEARCH 各 route の実行責務。
     - `executeCodeViaShiro` の CodeExecutor handoff。
     - TTS lifecycle と event emitter に依存している点。

  5. 提案する collaborator
     - `messageRouteDispatcher`
     - private struct とする。
     - 初期段階では interface 化しない。
     - `MessageOrchestrator` field として持つ。
     - constructor で組み立てる。
     - route direct executor として autonomous coordinator へ渡す。
     - TTS lifecycle と event emitter は function / method dependency として渡す。
     - agents と CodeExecutor は dependency として渡す。

  6. `messageRouteDispatcher` の契約
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 差し替え可能性
     - 変更してはいけない既存挙動

  7. 実装手順
     - baseline test を実行する。
     - `messageRouteDispatcher` を追加する。
     - `executeTask` / `executeRouteDirect` / route-specific execution を collaborator method へ移す。
     - `MessageOrchestrator` は thin wrapper または direct collaborator 呼び出しにする。
     - autonomous coordinator に渡す route direct executor を dispatcher 経由にする。
     - route path、event order、TTS push/finalize、CodeExecutor handoff を変更しない。
     - gofmt を実行する。
     - focused test と全体 test を実行する。

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
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
       ```
     - Phase8:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase8'
       ```
     - `git diff --check`

  9. リスク
     - route chain event order を壊す。
     - CODE route が CodeExecutor を経由しなくなる。
     - Worker error / Generate error を success に変換する。
     - TTS push/finalize のタイミングを変える。
     - WILD / HEAVY nil fallback の挙動を変える。
     - autonomous coordinator の route direct executor と dispatcher が循環する。
     - route dispatcher が巨大 manager になる。

  10. 完了条件
      - 実装仕様書が作成されている。
      - route dispatcher の現在構造が棚卸しされている。
      - `messageRouteDispatcher` の契約が書かれている。
      - 実装手順と検証手順が書かれている。
      - コード変更は行っていない。
      - ユーザーが次に Phase9 実装可否を判断できる。

実行手順:
  1. 参照文書を読む。
  2. Phase8 後の `message_orchestrator_routes.go` と周辺コードを確認する。
  3. route dispatcher の責務と結合点を棚卸しする。
  4. `docs/refactor/Phase9_route_dispatcher境界整理実装仕様.md` を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
