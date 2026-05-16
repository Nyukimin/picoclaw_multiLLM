# Phase7 MessageOrchestrator 分割実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase7 として、MessageOrchestrator を段階的に分割するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase7: MessageOrchestrator 分割設計

目的:
  - Phase6 で固定した route chain 契約を守ったまま、`MessageOrchestrator` の責務を段階的に薄くする。
  - `ProcessMessage` に集中している session、event、pre-routing command、route decision、TTS、autonomous execution、response assembly の責務境界を整理する。
  - いきなり大規模分割せず、責務ごとに小 Phase 化して、安全に移行できる実装仕様を作る。
  - Chat / Worker / Coder の責務境界を維持し、将来 `internal/application` 内の coordinator / workflow / adapter 境界へ移しやすくする。
  - モジュール化と疎結合を最重要方針として、単なる helper 化や巨大 manager 化を避ける。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase2_route_chain明確化実装仕様.md
  9. docs/refactor/Phase2_完了判定.md
  10. docs/refactor/Phase3_worker_execution境界整理実装仕様.md
  11. docs/refactor/Phase3_完了判定.md
  12. docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md
  13. docs/refactor/Phase4_完了判定.md
  14. docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md
  15. docs/refactor/Phase5_完了判定.md
  16. docs/refactor/Phase6_route_chain統合確認実装仕様.md
  17. docs/refactor/Phase6_完了判定.md
  18. docs/codebase-map/アーキテクチャ総合.md
  19. docs/codebase-map/結合ポイントマップ.md
  20. docs/codebase-map/ユースケース逆引き.md
  21. docs/codebase-map/modules/application.md
  22. docs/codebase-map/modules/domain.md
  23. docs/codebase-map/modules/潜在バグ一覧.md
  24. internal/application/orchestrator/message_orchestrator.go
  25. internal/application/orchestrator/message_orchestrator_*test.go
  26. internal/application/orchestrator/code_executor*.go
  27. internal/application/orchestrator/code_executor_test.go
  28. internal/application/service/worker_execution_service.go
  29. internal/application/service/worker_execution_service_test.go
  30. internal/domain/routing/
  31. internal/domain/task/
  32. internal/domain/session/

docs/codebase-map/ の使い方:
  - 一次解析資料として使う。
  - MessageOrchestrator の周辺責務、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

作成する文書:
  - docs/refactor/Phase7_MessageOrchestrator分割実装仕様.md

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
  - fallback を正常系として扱わない。
  - Viewer 表示、音声、口パク、ログを混同しない。
  - repo example と live runtime config を混同しない。
  - 巨大な service / manager / helper / util を新設しない。
  - 「便利だから共有する」「似ているからまとめる」だけの共通化をしない。

文書に必ず含める内容:

  1. Phase7 の目的
     - `MessageOrchestrator` を段階的に薄くすること。
     - `ProcessMessage` の責務を棚卸しし、分割順を決めること。
     - Phase6 の route chain 契約を維持すること。
     - 挙動変更ではなく、責務分離と移行計画を目的にすること。

  2. 対象範囲
     - `MessageOrchestrator.ProcessMessage`
     - session load / create / save
     - `message.received` event
     - pre-routing chat command
     - task / job / TTS session ID assembly
     - route decision
     - TTS session start / end
     - idle notifier busy state
     - `executeTask`
     - `executeAutonomousTask`
     - route-specific execution entrypoint
     - response assembly
     - event emission helper
     - current tests around ProcessMessage and route chain

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

  4. 現在の ProcessMessage 責務棚卸し
     必ず以下を書く:
     - session load / create の責務。
     - message received event の責務。
     - pre-routing chat command の責務。
     - task / job / TTS session ID assembly の責務。
     - Mio route decision の責務。
     - TTS session start / end の責務。
     - idle notifier busy state の責務。
     - route execution への委譲責務。
     - autonomous executor retry / verify flow との境界。
     - response assembly の責務。
     - session save の責務。
     - error wrapping の責務。

  5. 分割候補
     必ず以下の候補を比較し、Phase7 で扱う順序を書く:
     - session lifecycle helper
     - event emission helper
     - pre-routing command handler
     - task context builder
     - route decision coordinator
     - TTS lifecycle helper
     - idle busy guard
     - autonomous execution coordinator
     - response assembler
     - route dispatch file / coordinator

  6. 残すもの / 移すもの
     `MessageOrchestrator` に残してよいもの:
     - top-level orchestration
     - 主要 dependency の保持
     - route chain の大枠
     - 分割した collaborator の呼び出し

     `MessageOrchestrator` から減らしたいもの:
     - 長い session lifecycle 詳細
     - 長い TTS lifecycle 詳細
     - autonomous executor の request 組み立て詳細
     - route-specific execution の細部
     - response assembly の細部
     - error / event / log の局所的な重複

  7. 分割単位ごとの契約
     各分割候補について以下を書く:
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 変更してはいけない既存挙動
     - Phase6 契約との関係

  8. 小 Phase 案
     Phase7 を以下の小 Phase に分けること。

     - Phase7-0: `ProcessMessage` 現在責務棚卸しと baseline 固定
     - Phase7-1: session lifecycle / response assembly 境界整理
     - Phase7-2: pre-routing command / route decision 境界整理
     - Phase7-3: TTS lifecycle / idle busy guard 境界整理
     - Phase7-4: autonomous execution coordinator 境界整理
     - Phase7-5: route dispatch entrypoint 境界整理
     - Phase7-6: 完了判定と Phase8 判断

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
     - live health は原則不要。ただし server startup / runtime config に触った場合のみ:
       ```bash
       curl -fsS http://127.0.0.1:18790/health
       ```
     - Viewer / IdleChat / STT / TTS の handler 中身を変えた場合は実ブラウザまたは同等の E2E 確認が必要。ただし Phase7 では原則触らない。

  10. リスク
      必ず以下を含める:
      - `ProcessMessage` 分割で route chain の順序を変えるリスク。
      - session save / event emission / response assembly の順序を変えるリスク。
      - TTS session start / end の条件を崩すリスク。
      - idle notifier busy state の解除漏れ。
      - autonomous executor の retry / verify flow と route-specific execution を混同するリスク。
      - CodeExecutor handoff を崩すリスク。
      - Worker error / Generate error を success として扱うリスク。
      - `Handled` を final success 判定に使ってしまうリスク。
      - Viewer event / execution log / response text を混同するリスク。
      - helper 化だけで責務境界が曖昧になるリスク。
      - 巨大な coordinator / manager を作るリスク。

  11. 完了条件
      - `docs/refactor/Phase7_MessageOrchestrator分割実装仕様.md` が作成されている。
      - `ProcessMessage` の現在責務が棚卸しされている。
      - 分割候補と順序が明記されている。
      - 各分割候補の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
      - Phase6 契約を維持する方針が明記されている。
      - 小 Phase 案が書かれている。
      - 検証手順が書かれている。
      - コード変更は行っていない。
      - ユーザーが次に「Phase7 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. `internal/application/orchestrator/message_orchestrator.go` と関連テストを確認する。
  3. Phase6 で固定した契約を確認する。
  4. `ProcessMessage` の責務を棚卸しする。
  5. 分割候補と小 Phase の順序を決める。
  6. `docs/refactor/Phase7_MessageOrchestrator分割実装仕様.md` を作成する。
  7. コード変更は行わない。
  8. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
