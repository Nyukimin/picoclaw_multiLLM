# Phase22 distributed coder selection 境界整理実装仕様作成プロンプト

Goal:
  RenCrow のリファクタリング Phase22 として、DistributedOrchestrator の coder selection / node capability selection を distributed coder selection 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase22: distributed coder selection 境界整理

目的:
  - `routeToCoder` / `routeToCoderForMessage` / `isCoderConnected` / node capability selection を分散 Coder 選択専用 collaborator へ分離する。
  - explicit CODE1-4、CODE fallback chain、capability selection、connection 判定、既存ログを維持する。
  - Code execution、transport executor、node selector 実装本体には踏み込まない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/Phase21_完了判定.md
  6. docs/codebase-map/アーキテクチャ総合.md
  7. docs/codebase-map/結合ポイントマップ.md
  8. docs/codebase-map/ユースケース逆引き.md
  9. docs/codebase-map/modules/*.md
  10. docs/codebase-map/modules/潜在バグ一覧.md
  11. internal/application/orchestrator/distributed_orchestrator.go
  12. internal/application/orchestrator/node_selector.go
  13. internal/application/orchestrator/distributed_orchestrator_code.go
  14. internal/application/orchestrator/distributed_orchestrator_test.go

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - ファイル名は日本語を含める。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
  - docs/refactor/Phase22_distributed_coder_selection境界整理実装仕様.md

文書に必ず含める内容:
  1. Phase22 の目的
  2. 対象範囲
  3. 対象外
  4. 現在の coder selection 構造
  5. 提案する `distributedCoderSelection`
  6. collaborator 契約
  7. 実装手順
  8. 検証手順
  9. リスク
  10. 完了条件

必ず明記する既存契約:
  - CODE は coder1-4 の接続済み fallback chain を使う。
  - CODE1-4 は明示 coder が接続済みの場合だけ選ぶ。
  - CODE route かつ node selector / capability がある場合だけ capability selection を使う。
  - selected が空の場合は fallback chain に戻す。
  - SSH transport または router registered agent を接続済みとする。
  - coder selected / skip / capability fallback のログを維持する。

検証手順:
  - baseline / after:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw`
  - coder selection focused:
    `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase22|TestDistributedOrchestrator_.*(CODE|Retry)|TestNodeSelector'`
  - diff:
    `git diff --check`
    `git diff --stat`

実行手順:
  1. 参照文書と対象コードを読む。
  2. coder selection の explicit route、fallback chain、capability selection、connection 判定を棚卸しする。
  3. `distributedCoderSelection` の契約を書く。
  4. docs/refactor/Phase22_distributed_coder_selection境界整理実装仕様.md を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
