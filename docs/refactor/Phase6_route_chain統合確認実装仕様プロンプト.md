# Phase6 route chain 統合確認実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase6 として、Chat / Worker / Coder route chain 全体の統合確認を行うための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase6: Chat / Worker / Coder route chain 統合確認

目的:
  - Phase2〜Phase5 で分離した責務境界が、全体フローとして矛盾していないか確認する。
  - `MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の流れを仕様として固定する。
  - Chat / Worker / Coder の責務分離を、docs と契約テストで再確認する。
  - 次の大きな `MessageOrchestrator` 分割に入る前に、route chain の現在契約を安全に固定する。
  - 挙動変更ではなく、現在契約の確認と不足テストの明文化を目的にする。

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
  16. docs/codebase-map/アーキテクチャ総合.md
  17. docs/codebase-map/結合ポイントマップ.md
  18. docs/codebase-map/ユースケース逆引き.md
  19. docs/codebase-map/modules/application.md
  20. docs/codebase-map/modules/domain.md
  21. docs/codebase-map/modules/潜在バグ一覧.md
  22. internal/application/orchestrator/message_orchestrator.go
  23. internal/application/orchestrator/message_orchestrator_*test.go
  24. internal/application/orchestrator/code_executor.go
  25. internal/application/orchestrator/code_executor_*.go
  26. internal/application/orchestrator/code_executor_test.go
  27. internal/application/service/worker_execution_service.go
  28. internal/application/service/worker_execution_service_test.go
  29. internal/domain/routing/
  30. internal/domain/task/
  31. internal/domain/proposal/
  32. internal/domain/patch/

docs/codebase-map/ の使い方:
  - 一次解析資料として使う。
  - route chain、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

作成する文書:
  - docs/refactor/Phase6_route_chain統合確認実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat 契約、STT/TTS provider、LLM provider、runtime config の挙動は変更しない前提にする。
  - fallback を正常系として扱わない。
  - Viewer 表示、音声、口パク、ログを混同しない。
  - repo example と live runtime config を混同しない。
  - Phase6 では大規模な MessageOrchestrator 分割を開始しない。

文書に必ず含める内容:

  1. Phase6 の目的
     - Phase2〜Phase5 で整理した境界の統合確認。
     - `MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の契約固定。
     - Chat / Worker / Coder の責務分離確認。
     - 次の MessageOrchestrator 分割前の安全確認。
     - 挙動変更ではなく、現在契約確認と不足テストの計画であること。

  2. 対象範囲
     - `MessageOrchestrator.ProcessMessage`
     - route decision / route dispatch
     - CODE / CODE1 / CODE2 / CODE3 / CODE4 path
     - `executeCodeViaShiro`
     - `CodeExecutor.ExecuteCode`
     - Coder selection
     - proposal path
     - Generate path
     - WorkerExecutionService handoff
     - response assembly
     - event emission
     - existing route chain tests

  3. 対象外
     - MessageOrchestrator の大規模分割
     - WorkerExecutionService 内部の再分割
     - ToolRunner / PolicyEngine
     - Coder provider
     - proposal / patch format の意味変更
     - handler / DTO / SSE event
     - Viewer JS / CSS
     - IdleChat
     - STT / TTS
     - runtime config
     - 未追跡の tests/

  4. 現在の route chain 棚卸し
     必ず以下を書く:
     - ユーザー入力から `MessageOrchestrator.ProcessMessage` へ入る流れ
     - Mio / routing decision の責務
     - route dispatch の責務
     - CODE 系 route が Shiro 経由で CodeExecutor に入る流れ
     - CodeExecutor が Coder を選ぶ責務
     - Coder が proposal / Generate を担当する責務
     - WorkerExecutionService が valid proposal を実行する責務
     - response が Mio / user 向けに戻る流れ
     - event が Viewer / log と混同されないこと

  5. Chat / Worker / Coder 責務境界
     - Chat:
       - ユーザー対話
       - route 判断
       - 結果返却
       - 実行詳細や破壊的操作を持たない
     - Worker:
       - 実行主体
       - file edit / shell / git / test execution
       - Coder proposal の適用
       - 実行ログと evidence
     - Coder:
       - plan / patch / proposal / Generate
       - 破壊的操作を直接実行しない
       - WorkerExecutionService の実行責務を持たない

  6. 統合確認する契約
     必ず以下を含める:
     - CODE 系 route は CodeExecutor へ委譲される。
     - CODE 系 route は Shiro 経由 event を維持する。
     - explicit CODE1 / CODE2 / CODE3 / CODE4 の Coder slot 対応を維持する。
     - generic CODE fallback order を維持する。
     - dynamic capability selection が route chain を壊さない。
     - proposal interface 非対応 Coder は Generate path に戻る。
     - nil / invalid proposal は Worker に渡らない。
     - valid proposal だけ WorkerExecutionService に渡る。
     - WorkerExecutionService に渡す jobID は `req.Task.JobID()`。
     - Worker error は success に変換しない。
     - Generate error は fallback success にしない。
     - `CodeExecutionResponse.Handled` は success/failure ではなく proposal path 処理有無。
     - degraded route notice は fallback success ではない。
     - Viewer event / execution log / response text を混同しない。
     - route chain の event order を壊さない。

  7. 既存テストの棚卸し
     既存テストを確認して、何を保証しているかを書く:
     - `message_orchestrator_code_path_test.go`
     - `message_orchestrator_route_chain_contract_test.go`
     - `message_orchestrator_code3_test.go`
     - `code_executor_test.go`
     - `worker_execution_service_test.go`
     - domain routing / proposal / patch tests

  8. 不足している契約テスト案
     実装仕様書として、次に追加すべきテスト案を書く。
     例:
     - CODE 系 route がすべて Shiro 経由 event を維持すること。
     - invalid proposal が WorkerExecutionService に到達しないこと。
     - Worker execution error が response success に変換されないこと。
     - `Handled` が final success 判定に使われていないこと。
     - degraded route notice が success event として扱われないこと。
     - Generate path error が fallback success に変換されないこと。
     - route chain の response assembly が CODE / WORKER / CHAT を混同しないこと。

  9. 小 Phase 案
     Phase6 を以下の小 Phase に分けること。

     - Phase6-0: 現在 route chain と既存テストの棚卸し
     - Phase6-1: CODE route chain 契約テスト追加
     - Phase6-2: proposal handoff / invalid proposal 契約テスト追加
     - Phase6-3: error / degraded / Handled 契約テスト追加
     - Phase6-4: route chain 統合確認文書と完了判定

     各小 Phase について:
     - 目的
     - 対象範囲
     - 対象外
     - 実装手順
     - 検証手順
     - 完了条件
     を書く。

  10. 実装手順
      - baseline test を実行する。
      - まず既存テストの保証範囲を文書化する。
      - 不足契約テストを小さく追加する。
      - 原則として production code は変更しない。
      - production code 変更が必要な場合は、Phase6 の範囲を超える可能性として停止し報告する。
      - gofmt を実行する。
      - after test を実行する。
      - 各小 Phase ごとに docs / test diff を commit / push する。

  11. 検証手順
      baseline:
      ```bash
      GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
      ```

      after:
      ```bash
      GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
      git diff --check
      git diff --stat
      ```

      route chain 確認:
      ```bash
      rg "executeCodeViaShiro|ExecuteCode|WorkerExecutionService|ExecuteProposal|Handled|agent.start|agent.response|agent.notice" internal/application/orchestrator internal/application/service
      ```

  12. リスク
      - 統合確認の名目で MessageOrchestrator の大規模分割を始める。
      - route chain の挙動変更とテスト追加を混ぜる。
      - WorkerExecutionService の内部責務へ踏み込む。
      - Coder に実行責務を戻す。
      - fallback / degraded を success として扱う。
      - `Handled` を success / failure と混同する。
      - Viewer event と execution log と response text を混同する。
      - 既存テストの保証範囲を過大評価する。
      - test double が本物の契約を覆い隠す。

  13. 完了条件
      - Phase6 実装仕様書が docs/refactor/ に作成されている。
      - route chain の現在契約が文書化されている。
      - Chat / Worker / Coder の責務境界が文書化されている。
      - 既存テストの保証範囲が棚卸しされている。
      - 不足している契約テスト案が書かれている。
      - 小 Phase の順序が書かれている。
      - 検証手順が書かれている。
      - コード変更は行っていない。
      - ユーザーが次に「Phase6 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. `MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の現在コードを確認する。
  3. Phase2〜Phase5 の完了判定を確認する。
  4. 既存 route chain テストを棚卸しする。
  5. 不足している契約テスト案を整理する。
  6. 小 Phase の順序を書く。
  7. `docs/refactor/Phase6_route_chain統合確認実装仕様.md` を作成する。
  8. コード変更は行わない。
  9. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
