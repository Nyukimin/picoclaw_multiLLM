# Phase4 CodeExecutor 境界整理実装仕様プロンプト

```text
Goal:
  RenCrow のリファクタリング Phase 4 として、CodeExecutor / Coder selection / proposal path / Worker handoff の境界整理に入る前に、実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase 4: CodeExecutor / Coder selection 境界整理

目的:
  - Phase 2 で MessageOrchestrator の route chain を明確化し、Phase 3 で WorkerExecutionService / ToolRunner / PolicyEngine の境界を整理した。
  - 次段階として、CODE 系 route の中核である CodeExecutor の責務を整理する。
  - Coder selection、proposal path、Generate path、Worker handoff、event emission、fallback / degraded route notice の責務境界を明確にする。
  - モジュール化と疎結合を最重要方針として、Coder selection や proposal execution を将来差し替えやすい構造にする。
  - ただし、この作業では実装仕様書のみを作成し、コード変更は行わない。

作成する文書:
  - docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md

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
  12. docs/codebase-map/アーキテクチャ総合.md
  13. docs/codebase-map/結合ポイントマップ.md
  14. docs/codebase-map/ユースケース逆引き.md
  15. docs/codebase-map/modules/application.md
  16. docs/codebase-map/modules/domain.md
  17. docs/codebase-map/modules/潜在バグ一覧.md
  18. internal/application/orchestrator/code_executor.go
  19. internal/application/orchestrator/code_helpers.go
  20. internal/application/orchestrator/code_executor_test.go
  21. internal/application/orchestrator/message_orchestrator.go
  22. internal/application/orchestrator/message_orchestrator_code_path_test.go
  23. internal/application/orchestrator/message_orchestrator_route_chain_contract_test.go
  24. internal/application/service/worker_execution_service.go
  25. internal/domain/proposal/
  26. internal/domain/patch/
  27. internal/domain/capability/

docs/codebase-map/ の使い方:
  - 実装前の一次解析資料として使う。
  - CodeExecutor 周辺の責務、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md を優先する。
  - docs/archive/ は一次参照にしない。

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語にする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。
  - Phase 2 の route chain 契約を変更しない。
  - Phase 3 の WorkerExecutionService / ToolRunner / PolicyEngine 境界を変更しない。
  - handler、DTO、SSE event、Viewer JS/CSS、IdleChat 契約、STT/TTS provider、LLM provider の挙動は変更しない前提にする。
  - fallback を正常系として扱わない。
  - Viewer 表示、音声、口パク、ログを混同しない。

文書に必ず含める内容:

1. Phase 4 の目的
   - CodeExecutor の責務境界を整理すること。
   - Coder selection、proposal path、Generate path、Worker handoff を分けて追えるようにすること。
   - Coder が実行責務を持たず、Worker が実行主体である境界を維持すること。

2. 対象範囲
   - DefaultCodeExecutor
   - CodeExecutionRequest / CodeExecutionResponse
   - codeTarget
   - selectCoderForRoute
   - shouldUseProposalPath
   - tryExecuteProposalPath
   - executeCoderGeneratePath
   - explicitCodeRouteTarget
   - systemPromptForRoute
   - coderByName
   - CodeExecutor event emission
   - CoderStatus acquire / release
   - capability.SelectCoder との接続点
   - WorkerExecutionService への handoff

3. 対象外
   - MessageOrchestrator の route chain 変更
   - WorkerExecutionService の内部実行方式変更
   - ToolRunner / PolicyEngine の変更
   - CoderAgent の LLM provider 挙動変更
   - proposal / patch format の意味変更
   - Viewer JS / CSS
   - STT / TTS provider
   - runtime config の意味変更

4. 現在の責務整理
   - DefaultCodeExecutor の責務
   - Coder selection の責務
   - proposal path の責務
   - Generate path の責務
   - Worker handoff の責務
   - event emission の責務
   - fallback / degraded route notice の責務
   - CodeExecutionResponse.Handled の意味

5. 提案する Phase 4 の小 Phase
   - Phase 4-0: 現在契約の固定
   - Phase 4-1: Coder selection 境界整理
   - Phase 4-2: proposal path 境界整理
   - Phase 4-3: Generate path / event emission 境界整理
   - Phase 4-4: CodeExecutionResponse / result contract 整理
   - Phase 4-5: 完了判定

6. 各小 Phase の契約
   各 Phase について以下を書く:
   - 目的
   - 対象範囲
   - 対象外
   - 入力
   - 出力
   - 副作用
   - 永続化
   - ログ
   - エラー契約
   - 変更してはいけない既存挙動
   - 検証手順
   - 完了条件

7. モジュール化・疎結合の方針
   - 単にファイルを分けるだけではモジュール化ではないこと。
   - Coder selection と proposal execution を混同しないこと。
   - proposal validation と Worker handoff を明確に分けること。
   - CodeExecutor が巨大 manager にならないこと。
   - WorkerExecutionService の実行責務を CodeExecutor に戻さないこと。
   - CoderStatus の acquire / release が selection の副作用であることを明記すること。
   - degraded route notice は Coder selection の結果通知であり、fallback success ではないことを明記すること。

8. 検証方針
   - baseline:
     GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
   - after:
     GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
   - git diff --check
   - CodeExecutor に近い unit test を優先する。
   - WorkerExecutionService の内部へ踏み込んだ場合は Phase 3 の対象テストも実行する。
   - 実ブラウザ確認は原則不要。ただし Viewer event や runtime route に触る場合は追加確認する。

9. リスク
   - Coder selection と proposal path を混ぜるリスク。
   - Coder が直接実行する構造に戻るリスク。
   - invalid proposal を Worker に渡すリスク。
   - CODE route の fallback chain を成功保証のように扱うリスク。
   - degraded route notice を fallback success と誤解するリスク。
   - CoderStatus release 漏れで busy state が残るリスク。
   - event emission の順序を壊すリスク。
   - Phase 2 の route chain 契約を壊すリスク。
   - Phase 3 の Worker handoff 契約を壊すリスク。

10. 完了条件
   - docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md が作成されている。
   - Phase 4 の目的、対象、対象外が明記されている。
   - CodeExecutor / Coder selection / proposal path / Worker handoff の境界が説明されている。
   - 小 Phase の移行順が明記されている。
   - 各小 Phase の検証条件が書かれている。
   - コード変更は行っていない。
   - ユーザーが次に「Phase 4 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. docs/codebase-map/ で CodeExecutor 周辺の結合点を確認する。
  3. 現在コードで CodeExecutor / Coder selection / proposal path / Worker handoff の接続点を確認する。
  4. Phase 4 の実装仕様を docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md に作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
