# Phase3 worker execution 境界整理実装仕様プロンプト

```text
Goal:
  RenCrow のリファクタリング Phase 3 として、WorkerExecutionService / ToolRunner / PolicyEngine の境界整理に入る前に、実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase 3: Worker execution chain の責務境界整理

目的:
  - Phase 2 で明確化した Chat / Worker / Coder route chain の次段階として、Worker 側の実行責務を整理する。
  - WorkerExecutionService、ToolRunner、PolicyEngine、patch/proposal 実行、ログ、検証結果の責務境界を明確にする。
  - モジュール化と疎結合を最重要方針として、将来差し替え可能な Worker execution 境界を作る。
  - ただし、この作業では実装仕様書のみを作成し、コード変更は行わない。

作成する文書:
  - docs/refactor/Phase3_worker_execution境界整理実装仕様.md

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
  10. docs/codebase-map/アーキテクチャ総合.md
  11. docs/codebase-map/結合ポイントマップ.md
  12. docs/codebase-map/ユースケース逆引き.md
  13. docs/codebase-map/modules/*.md
  14. docs/codebase-map/modules/潜在バグ一覧.md
  15. internal/application/orchestrator/code_executor.go
  16. internal/application/service/worker_execution_service.go
  17. ToolRunner / PolicyEngine 関連ファイル
  18. 関連する *_test.go

docs/codebase-map/ の使い方:
  - 実装前の一次解析資料として使う。
  - 対象ファイルの周辺責務、結合点、ユースケース、潜在バグを確認する。
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
  - Phase 2 で整理した route chain の契約を変更しない。
  - handler、DTO、SSE event、Viewer JS/CSS、IdleChat 契約、STT/TTS provider、LLM provider の挙動は変更しない前提にする。
  - fallback を正常系として扱わない。
  - Viewer 表示、音声、口パク、ログを混同しない。

文書に必ず含める内容:

1. Phase 3 の目的
   - Worker execution chain の責務境界を整理すること。
   - WorkerExecutionService / ToolRunner / PolicyEngine の役割を明確にすること。
   - Coder proposal を Worker が実行する境界を壊さないこと。

2. 対象範囲
   - WorkerExecutionService
   - ToolRunner
   - PolicyEngine
   - proposal / patch execution
   - 実行ログ
   - 実行結果
   - error contract
   - CodeExecutor から WorkerExecutionService への接続点

3. 対象外
   - MessageOrchestrator の route chain 変更
   - Coder の proposal 生成ロジック変更
   - Tool の実処理内容変更
   - Policy の意味変更
   - Viewer JS / CSS
   - STT / TTS provider
   - LLM provider
   - runtime config の意味変更

4. 現在の責務整理
   - CodeExecutor の責務
   - WorkerExecutionService の責務
   - ToolRunner の責務
   - PolicyEngine の責務
   - patch/proposal domain object の責務
   - ログと実行結果の責務

5. 提案する Phase 3 の小 Phase
   - Phase 3-0: 現在契約の固定
   - Phase 3-1: WorkerExecutionService の入力・出力・副作用整理
   - Phase 3-2: ToolRunner 境界整理
   - Phase 3-3: PolicyEngine 境界整理
   - Phase 3-4: execution result / error contract 整理
   - Phase 3-5: 完了判定

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
   - interface / contract / DTO / adapter の境界を明確にすること。
   - WorkerExecutionService が巨大 manager にならないこと。
   - ToolRunner が PolicyEngine の判断を抱え込まないこと。
   - PolicyEngine が tool 実行の副作用を持たないこと。
   - 差し替え時に他層へ影響が広がらない構造を目指すこと。

8. 検証方針
   - baseline:
     GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
   - after:
     GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
   - git diff --check
   - 可能なら WorkerExecutionService / ToolRunner / PolicyEngine に近い unit test を優先する。
   - 実ブラウザ確認は原則不要。ただし Viewer event や runtime route に触る場合は追加確認する。

9. リスク
   - Coder が直接実行する構造に戻るリスク。
   - WorkerExecutionService が巨大 service になるリスク。
   - ToolRunner と PolicyEngine の責務が混ざるリスク。
   - policy 判定を bypass するリスク。
   - patch 実行結果とログを混同するリスク。
   - fallback を成功扱いするリスク。
   - Phase 2 の route chain 契約を壊すリスク。

10. 完了条件
   - docs/refactor/Phase3_worker_execution境界整理実装仕様.md が作成されている。
   - Phase 3 の目的、対象、対象外が明記されている。
   - WorkerExecutionService / ToolRunner / PolicyEngine の境界が説明されている。
   - 小 Phase の移行順が明記されている。
   - 各小 Phase の検証条件が書かれている。
   - コード変更は行っていない。
   - ユーザーが次に「Phase 3 を実装してよいか」を判断できる。

実行手順:
  1. 参照文書を読む。
  2. docs/codebase-map/ で Worker execution 周辺の結合点を確認する。
  3. 現在コードで CodeExecutor / WorkerExecutionService / ToolRunner / PolicyEngine の接続点を確認する。
  4. Phase 3 の実装仕様を docs/refactor/Phase3_worker_execution境界整理実装仕様.md に作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
