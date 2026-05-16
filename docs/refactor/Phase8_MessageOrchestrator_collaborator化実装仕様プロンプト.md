# Phase8 MessageOrchestrator collaborator 化実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase8 として、Phase7 でファイル分離した MessageOrchestrator 周辺責務を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase8: MessageOrchestrator collaborator 化

目的:
  - Phase7 で分離したファイル責務を、必要な範囲で collaborator / small coordinator として明確化する。
  - `MessageOrchestrator` が全ての helper method を直接抱え続ける状態から、差し替え可能な境界を持つ構造へ段階的に寄せる。
  - ただし巨大な service / manager / helper / util を作らず、意味のある責務境界だけを collaborator 化する。
  - Phase6 で固定した Chat / Worker / Coder route chain 契約と、Phase7 で整理したファイル責務を壊さない。
  - モジュール化と疎結合を最重要方針として、将来 `internal/application` 内の workflow / coordinator / adapter 境界へ移しやすくする。

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
  12. docs/codebase-map/アーキテクチャ総合.md
  13. docs/codebase-map/結合ポイントマップ.md
  14. docs/codebase-map/ユースケース逆引き.md
  15. docs/codebase-map/modules/application.md
  16. docs/codebase-map/modules/domain.md
  17. docs/codebase-map/modules/潜在バグ一覧.md
  18. internal/application/orchestrator/message_orchestrator.go
  19. internal/application/orchestrator/message_orchestrator_events.go
  20. internal/application/orchestrator/message_orchestrator_session.go
  21. internal/application/orchestrator/message_orchestrator_response.go
  22. internal/application/orchestrator/message_orchestrator_commands.go
  23. internal/application/orchestrator/message_orchestrator_task.go
  24. internal/application/orchestrator/message_orchestrator_routing.go
  25. internal/application/orchestrator/message_orchestrator_idle.go
  26. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  27. internal/application/orchestrator/message_orchestrator_autonomous.go
  28. internal/application/orchestrator/message_orchestrator_routes.go
  29. internal/application/orchestrator/message_orchestrator_*test.go
  30. internal/application/orchestrator/code_executor*.go
  31. internal/application/orchestrator/code_executor_test.go
  32. internal/application/service/worker_execution_service.go
  33. internal/application/service/worker_execution_service_test.go
  34. internal/domain/routing/
  35. internal/domain/task/
  36. internal/domain/session/

docs/codebase-map/ の使い方:
  - 一次解析資料として使う。
  - MessageOrchestrator の周辺責務、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

作成する文書:
  - docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat 契約、STT/TTS provider、LLM provider、runtime config の挙動は変更しない前提にする。
  - Phase6 で固定した route chain 契約を変更しない。
  - Phase7 で分離したファイル責務を崩さない。
  - fallback を正常系として扱わない。
  - Viewer 表示、音声、口パク、ログを混同しない。
  - repo example と live runtime config を混同しない。
  - 巨大な service / manager / helper / util を新設しない。
  - 「便利だから共有する」「似ているからまとめる」だけの共通化をしない。
  - collaborator 化は、入力、出力、副作用、永続化、ログ、エラー契約を明記できるものだけに限定する。

文書に必ず含める内容:

  1. Phase8 の目的
     - Phase7 のファイル分離から、意味のある collaborator 境界へ進めること。
     - `MessageOrchestrator` の top-level orchestration を保ったまま、過剰な method 集中を減らすこと。
     - 差し替え可能性と責務境界を強めること。
     - 挙動変更ではなく、構造整理と契約固定を目的にすること。

  2. 対象範囲
     - `MessageOrchestrator.ProcessMessage`
     - `message_orchestrator_session.go`
     - `message_orchestrator_response.go`
     - `message_orchestrator_commands.go`
     - `message_orchestrator_task.go`
     - `message_orchestrator_routing.go`
     - `message_orchestrator_idle.go`
     - `message_orchestrator_tts_lifecycle.go`
     - `message_orchestrator_autonomous.go`
     - `message_orchestrator_routes.go`
     - `message_orchestrator_events.go`
     - ProcessMessage / route chain / TTS / session tests

  3. 対象外
     - Phase6 で固定した route chain 契約の変更。
     - CodeExecutor の再分割。
     - WorkerExecutionService 内部の再分割。
     - ToolRunner / PolicyEngine。
     - handler / DTO / SSE event。
     - Viewer JS / CSS。
     - IdleChat。
     - STT / TTS provider。
     - LLM provider。
     - runtime config。
     - distributed orchestrator の大規模変更。
     - 未追跡の tests/。

  4. Phase7 後の現在構造
     必ず以下を書く:
     - `message_orchestrator.go` に残った責務。
     - `message_orchestrator_session.go` の責務。
     - `message_orchestrator_response.go` の責務。
     - `message_orchestrator_commands.go` の責務。
     - `message_orchestrator_task.go` の責務。
     - `message_orchestrator_routing.go` の責務。
     - `message_orchestrator_idle.go` の責務。
     - `message_orchestrator_tts_lifecycle.go` の責務。
     - `message_orchestrator_autonomous.go` の責務。
     - `message_orchestrator_routes.go` の責務。
     - `message_orchestrator_events.go` の責務。
     - まだ `MessageOrchestrator` method として残る理由。

  5. collaborator 化する候補
     必ず以下を比較する:
     - session lifecycle collaborator
     - response assembler
     - pre-routing command handler
     - task context builder
     - route decision coordinator
     - idle busy guard
     - TTS lifecycle collaborator
     - autonomous execution coordinator
     - route dispatcher
     - event emitter

     各候補について:
     - collaborator 化する価値
     - collaborator 化しない場合の理由
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 差し替え可能性
     - Phase6 / Phase7 契約との関係
     を書く。

  6. Phase8 で collaborator 化してよいもの / まだ method のまま残すもの
     Phase8 では必ず保守的に判断する。

     collaborator 化してよい候補例:
     - response assembler
     - session lifecycle
     - route decision coordinator
     - idle busy guard
     - autonomous execution coordinator

     まだ method のまま残す候補例:
     - route dispatcher
     - TTS lifecycle
     - event emitter

     ただし、現在コードを確認したうえで、実際の提案を明記すること。

  7. 提案する collaborator 構成
     必ず具体名を書く:
     - `messageSessionLifecycle`
     - `messageResponseAssembler`
     - `preRoutingCommandHandler`
     - `routeDecisionCoordinator`
     - `idleBusyGuardFactory`
     - `autonomousExecutionCoordinator`

     各 collaborator について:
     - struct / interface にするか。
     - private にするか。
     - `MessageOrchestrator` に field として持たせるか。
     - constructor で組み立てるか。
     - 既存 dependency をどう渡すか。
     - test double が必要か。

  8. 小 Phase 案
     Phase8 を以下の小 Phase に分けること。

     - Phase8-0: collaborator 化対象の現在契約固定
     - Phase8-1: response assembler collaborator 化
     - Phase8-2: session lifecycle collaborator 化
     - Phase8-3: route decision coordinator / pre-routing command handler collaborator 化
     - Phase8-4: idle busy guard collaborator 化
     - Phase8-5: autonomous execution coordinator collaborator 化
     - Phase8-6: 完了判定と Phase9 判断

     各小 Phase について:
     - 目的
     - 対象範囲
     - 対象外
     - 実装手順
     - 検証手順
     - 完了条件
     を書く。

  9. テスト方針
     必ず以下を書く:
     - baseline:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
       ```
     - after:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
       git diff --check
       git diff --stat
       ```
     - route chain に触った場合:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
       ```
     - session / response に触った場合:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
       ```
     - TTS lifecycle に触った場合:
       ```bash
       GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
       ```
     - live health は原則不要。ただし server startup / runtime config に触った場合のみ:
       ```bash
       curl -fsS http://127.0.0.1:18790/health
       ```

  10. リスク
      必ず以下を含める:
      - collaborator 化で依存注入が増えすぎるリスク。
      - 小さい collaborator が多すぎて逆に読みにくくなるリスク。
      - collaborator 名が抽象的すぎて責務が曖昧になるリスク。
      - `MessageOrchestrator` constructor が巨大化するリスク。
      - Phase6 route chain event order を壊すリスク。
      - session save / response assembly / route decision の順序を変えるリスク。
      - TTS degraded を success と混同するリスク。
      - idle busy guard の defer 解除漏れ。
      - autonomous executor の retry / verify flow と route dispatch を混同するリスク。
      - Worker error / Generate error を success として扱うリスク。
      - Viewer event / execution log / response text を混同するリスク。
      - distributed orchestrator へ同時に広げて Phase8 の範囲を超えるリスク。

  11. 完了条件
      - `docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md` が作成されている。
      - Phase7 後の現在構造が棚卸しされている。
      - collaborator 化候補が比較されている。
      - collaborator 化するもの / まだ method のまま残すものが明記されている。
      - 各 collaborator の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
      - 小 Phase 案が書かれている。
      - 検証手順が書かれている。
      - コード変更は行っていない。
      - ユーザーが次に「Phase8 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. Phase6 / Phase7 の完了判定を確認する。
  3. Phase7 後の `message_orchestrator*.go` を確認する。
  4. collaborator 化候補を比較する。
  5. collaborator 化するもの / 残すものを決める。
  6. `docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md` を作成する。
  7. コード変更は行わない。
  8. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
