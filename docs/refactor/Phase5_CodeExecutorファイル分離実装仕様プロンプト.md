# Phase5 CodeExecutor ファイル分離実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase5 として、CodeExecutor 周辺ファイル分離を徹底的に行うための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase5: CodeExecutor 周辺ファイル分離

目的:
  - Phase4 で整理した CodeExecutor の責務境界を、ファイル構成としても明確にする。
  - `internal/application/orchestrator/code_executor.go` に残っている selection / proposal path / Generate path / event / response helper を、責務単位で別ファイルへ分離する。
  - 挙動変更は行わず、構造整理だけを行う。
  - モジュール化と疎結合を最重要方針として、将来 selection / proposal execution / event emission を差し替えやすい構造にする。
  - 単なるファイル分割ではなく、責務境界が説明できる分割にする。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md
  9. docs/refactor/Phase4-0_CodeExecutor現在契約固定.md
  10. docs/refactor/Phase4-1_Coder_selection境界整理.md
  11. docs/refactor/Phase4-2_proposal_path境界整理.md
  12. docs/refactor/Phase4-3_Generate_path_event境界整理.md
  13. docs/refactor/Phase4-4_CodeExecutionResponse契約整理.md
  14. docs/refactor/Phase4_完了判定.md
  15. docs/codebase-map/アーキテクチャ総合.md
  16. docs/codebase-map/結合ポイントマップ.md
  17. docs/codebase-map/ユースケース逆引き.md
  18. docs/codebase-map/modules/*.md
  19. docs/codebase-map/modules/潜在バグ一覧.md
  20. internal/application/orchestrator/code_executor.go
  21. internal/application/orchestrator/code_executor_test.go
  22. internal/application/orchestrator/*code*_test.go
  23. internal/application/orchestrator/coder_status.go
  24. internal/domain/capability/
  25. internal/domain/proposal/
  26. internal/domain/patch/
  27. internal/application/service/worker_execution_service.go

docs/codebase-map/ の使い方:
  - 一次解析資料として使う。
  - CodeExecutor 周辺の結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 判断が矛盾する場合は docs/01_正本仕様/実装仕様.md と現在コードを優先する。
  - docs/archive/ は一次参照にしない。

作成する文書:
  - docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md

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

文書に必ず含める内容:

  1. Phase5 の目的
     - CodeExecutor 周辺を責務単位でファイル分離すること。
     - Phase4 で整理した境界をファイル構成にも反映すること。
     - 挙動変更をしないこと。
     - 将来差し替え可能な構造へ寄せること。

  2. 対象範囲
     - internal/application/orchestrator/code_executor.go
     - CodeExecutionRequest / CodeExecutionResponse
     - DefaultCodeExecutor
     - codeTarget
     - selection helper
     - proposal path helper
     - Generate path helper
     - event emission helper
     - response assembly helper
     - explicitCodeRouteTarget
     - systemPromptForRoute
     - coderByName

  3. 対象外
     - MessageOrchestrator の route chain 変更
     - WorkerExecutionService 内部
     - ToolRunner / PolicyEngine
     - Coder provider
     - proposal / patch format の意味変更
     - handler / DTO / SSE event
     - Viewer JS / CSS
     - IdleChat
     - STT / TTS
     - runtime config
     - 未追跡の tests/

  4. 現在の code_executor.go の責務棚卸し
     - CodeExecutor interface
     - request / response DTO
     - DefaultCodeExecutor struct / constructor
     - ExecuteCode orchestration
     - Coder selection
     - proposal path
     - Generate path
     - event emission
     - response assembly
     - route prompt / coder lookup helper

  5. 提案するファイル分割
     必ず以下の候補を検討し、採用 / 不採用理由を書く。

     - internal/application/orchestrator/code_executor.go
       - interface、request / response、DefaultCodeExecutor、constructor、ExecuteCode のみに絞る。

     - internal/application/orchestrator/code_executor_selection.go
       - selectCoderForRoute
       - selectDynamicCoderForRoute
       - selectExplicitCoderForRoute
       - selectAvailableCoderForGenericRoute
       - explicitCodeRouteTarget
       - systemPromptForRoute
       - coderByName
       - codeTarget を置くかどうかも判断する。

     - internal/application/orchestrator/code_executor_proposal.go
       - shouldUseProposalPath
       - tryExecuteProposalPath
       - proposalCoderForTarget
       - generateProposalForTarget
       - validateGeneratedProposal
       - emitProposalPlan
       - executeProposalWithWorker
       - emitProposalExecutionResult

     - internal/application/orchestrator/code_executor_generate.go
       - executeCoderGeneratePath
       - emitCoderGenerateError
       - emitCoderGenerateResponse

     - internal/application/orchestrator/code_executor_events.go
       - emit
       - SetEventEmitter
       - emitDegradedRouteNotice
       - emitCodeHandoffStart

     - internal/application/orchestrator/code_executor_response.go
       - buildProposalHandledResponse
       - buildCoderGenerateResponse
       - CodeExecutionResponse をここへ移すかどうかも判断する。

  6. 分割単位ごとの契約
     各ファイルについて以下を書く:
     - 責務
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 置いてはいけない責務
     - 将来差し替える場合の境界

  7. 分割方針
     - package は `orchestrator` のままにする。
     - exported API は原則増やさない。
     - テストから必要な既存 package-private helper は維持してよい。
     - import のためだけに責務を広げない。
     - ファイル名は責務を表す。
     - 巨大な `helper` / `util` / `manager` ファイルを作らない。
     - 「便利だから共有する」「似ているからまとめる」だけの共通化は禁止。
     - code_executor.go を薄くするが、composition の流れは追えるようにする。

  8. 小 Phase 案
     Phase5 を一度に大きく動かさず、以下のような小 Phase に分けること。

     - Phase5-0: 現在ファイル構成と baseline test 記録
     - Phase5-1: selection helper を code_executor_selection.go へ移動
     - Phase5-2: proposal path helper を code_executor_proposal.go へ移動
     - Phase5-3: Generate path helper を code_executor_generate.go へ移動
     - Phase5-4: event helper を code_executor_events.go へ移動
     - Phase5-5: response helper を code_executor_response.go へ移動
     - Phase5-6: 完了判定

     各小 Phase について:
     - 目的
     - 対象範囲
     - 対象外
     - 実装手順
     - 検証手順
     - 完了条件
     を書く。

  9. 実装手順
     - baseline test を実行する。
     - 1 回の commit では 1 種類の責務だけを移動する。
     - 関数本体は原則そのまま移動する。
     - 関数名、error message、log message、event content を変えない。
     - import を最小化する。
     - gofmt を実行する。
     - after test を実行する。
     - 各小 Phase ごとに docs / 実装を commit / push する前提にする。

  10. 検証手順
      baseline:
      ```bash
      GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
      ```

      after:
      ```bash
      GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
      git diff --check
      git diff --stat
      ```

      ファイル移動確認:
      ```bash
      rg "func \\(e \\*DefaultCodeExecutor\\) select|func shouldUseProposalPath|func \\(e \\*DefaultCodeExecutor\\) tryExecuteProposalPath|func \\(e \\*DefaultCodeExecutor\\) executeCoderGeneratePath|func \\(e \\*DefaultCodeExecutor\\) emit|func buildProposalHandledResponse" internal/application/orchestrator
      ```

  11. リスク
      - 関数移動時に import を壊す。
      - package-private helper の配置を誤る。
      - event type / from / to / content / route を変える。
      - error message を変える。
      - CoderStatus release 契約を壊す。
      - invalid proposal が Worker に渡る。
      - `Handled` の意味を変える。
      - code_executor.go を薄くしすぎて ExecuteCode の流れが読めなくなる。
      - 分割ファイルが新しい巨大 helper になる。
      - ファイル分割だけで責務境界を説明できない状態になる。

  12. 完了条件
      - Phase5 実装仕様書が docs/refactor/ に作成されている。
      - 提案ファイル分割が明記されている。
      - 各ファイルの責務、入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
      - 小 Phase の順序が書かれている。
      - 検証手順が書かれている。
      - コード変更は行っていない。
      - ユーザーが次に「Phase5 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. `internal/application/orchestrator/code_executor.go` の現在構造を確認する。
  3. Phase4 で追加された helper の責務を棚卸しする。
  4. ファイル分割案を作る。
  5. 各ファイルの契約を書く。
  6. 小 Phase の順序を書く。
  7. `docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md` を作成する。
  8. コード変更は行わない。
  9. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
