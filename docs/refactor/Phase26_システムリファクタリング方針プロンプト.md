# Phase26 システムリファクタリング方針プロンプト

```text
Goal:
  RenCrow の新仕様と実装構造を照合し、「仕様と実装箇所が 1 対 1 に説明できるシステム」へ近づけるための、仕様・コード両面のリファクタリング方針を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase26: システムリファクタリング方針

目的:
  - 仕様追加の積み重ねにより未分化になっている責務を整理する。
  - 仕様変更時に触る実装箇所を明確にする。
  - 仕様と実装箇所が 1 対 1 に説明できない箇所を、システム設計上の改善対象として扱う。
  - ただし、コードの構造が仕様より良い場合は、コードを無理に変更せず、仕様側を更新することも許容する。
  - 「仕様をコードへ合わせる」「コードを仕様へ合わせる」のどちらを選ぶかを、根拠付きで判断できる状態にする。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/10_新仕様/README.md
  5. docs/10_新仕様/新仕様_概要.md
  6. docs/10_新仕様/モジュール構成仕様.md
  7. docs/10_新仕様/検証仕様.md
  8. docs/refactor/リファクタリング指針.md
  9. docs/refactor/フォルダ構成方針.md
  10. docs/refactor/段階移行計画.md
  11. docs/refactor/検証方針.md
  12. docs/refactor/Phase25_組み合わせテスト設計.md
  13. docs/codebase-map/アーキテクチャ総合.md
  14. docs/codebase-map/結合ポイントマップ.md
  15. docs/codebase-map/ユースケース逆引き.md
  16. docs/codebase-map/modules/*.md
  17. 現在の実装コード

docs/codebase-map/ の使い方:
  - 実装前の一次解析資料として使う。
  - 対象ファイルの周辺責務、結合点、ユースケース、潜在バグを確認する。
  - ただし正本仕様ではない。
  - 実装判断で矛盾がある場合は docs/01_正本仕様/実装仕様.md を優先する。
  - codebase-map の記述と現在コードが違う場合は、現在コードを確認し、docs/refactor/ に差分リスクとして記録する。
  - docs/archive/ は一次参照にしない。

重要方針:
  - モジュール化と疎結合を最重要原則とする。
  - 単にファイルを分けるだけではモジュール化とみなさない。
  - 仕様変更時に主に触る実装箇所を説明できる状態を目指す。
  - 仕様と実装箇所が 1 対 1 に説明できない場合は、必ず理由を分類する。
  - 理由分類は以下とする:
    1. 仕様が粗すぎる
    2. 仕様が古い
    3. 実装が未分化
    4. 実装が仕様より良い構造になっている
    5. 複数仕様が意図せず 1 モジュールに混ざっている
    6. 仕様上は 1 つだが、実行時には adapter / application / infrastructure に分かれるべきもの
  - コードが良い構造であれば、コードを残し、仕様を変更してよい。
  - コードが悪い構造であれば、仕様に合わせて段階的に分割する。
  - 仕様とコードのどちらを変更する場合でも、判断理由、影響範囲、検証条件を文書化する。

特に確認する集中点:
  - cmd/picoclaw/runtime_dependencies.go
  - cmd/picoclaw/health_runtime.go
  - internal/application/service/worker_execution_service.go
  - internal/application/orchestrator/distributed_orchestrator.go
  - docs/10_新仕様/モジュール構成仕様.md に記載された runtime_options.go 相当の扱い
  - Viewer / IdleChat / STT / TTS / LLM provider / Memory / Source Registry の仕様と実装対応
  - Chat / Worker / Coder の責務境界
  - route chain、Worker execution、CodeExecutor、DistributedOrchestrator の境界

作成してほしい文書:
  - docs/refactor/Phase26_システムリファクタリング方針.md

文書に必ず含める内容:

  1. Phase26 の目的
     - 仕様と実装箇所の対応を明確にすること
     - 仕様追加で未分化になった責務を整理すること
     - コードが良い場合は仕様を更新することも許容すること

  2. 判断原則
     - 仕様を優先する場合
     - コードを優先し、仕様を更新する場合
     - 仕様とコードの両方を分割する場合
     - いったん変更せず、リスクとして記録する場合

  3. 仕様と実装箇所の 1 対 1 判定基準
     各仕様について以下を確認する:
     - 主担当モジュール
     - 主担当ファイル
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 差し替え境界
     - 検証方法

  4. 1 対 1 で説明できない場合の分類
     - 仕様が粗すぎる
     - 仕様が古い
     - 実装が未分化
     - 実装が仕様より良い
     - 複数仕様が混在している
     - 層分離が不足している

  5. 修正方針
     各分類について、仕様を直すのか、コードを直すのか、両方を直すのかを書く。

  6. 現在確認済みの要注意箇所
     - runtime_dependencies.go
     - health_runtime.go
     - worker_execution_service.go
     - distributed_orchestrator.go
     - runtime_options.go 記載と実ファイル不一致
     それぞれについて:
     - 現在の状態
     - 1 対 1 で説明しにくい理由
     - 仕様を変更すべき可能性
     - コードを分割すべき可能性
     - 推奨する次アクション

  7. 段階移行計画
     - Phase A: 仕様・実装対応表の確定
     - Phase B: 仕様が古い箇所の更新
     - Phase C: 実装が未分化な箇所の分割
     - Phase D: 仕様と実装の再照合
     - Phase E: 組み合わせテストと E2E で確認

  8. 禁止事項
     - 仕様変更とコード変更を同じ根拠なしに混ぜない
     - 巨大な service / manager / helper / util を新設しない
     - 「便利だから共有する」「似ているからまとめる」だけの共通化をしない
     - fallback を正常系として扱わない
     - Viewer 表示、音声、口パク、ログを混同しない
     - repo example と live runtime config を混同しない
     - archive 文書を一次参照にしない

  9. 検証方針
     - 仕様だけ変更した場合の確認
     - コードだけ変更した場合の確認
     - 仕様とコードを両方変更した場合の確認
     - Viewer / IdleChat / STT / TTS / LLM provider / Memory / Source Registry に関する確認
     - 1 対 1 対応が改善したことをどう確認するか

  10. 完了条件
     - docs/refactor/Phase26_システムリファクタリング方針.md が作成されている
     - 仕様優先、コード優先、両方修正の判断基準が書かれている
     - 現在の要注意箇所が分類されている
     - 次に実装へ進むべき単位が明確になっている
     - コード変更は行っていない

制約:
  - この作業では文書作成だけを行う。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
  - docs/10_新仕様/ は補助仕様として扱う。
  - 未確定の見出しや空欄を残さない。
  - ファイル名は日本語にする。

実行手順:
  1. 参照文書を読む。
  2. docs/10_新仕様/ の仕様と現在コードを照合する。
  3. 仕様と実装箇所が 1 対 1 に説明しにくい箇所を分類する。
  4. コードを直すべきか、仕様を直すべきか、両方直すべきかを判断する基準を書く。
  5. docs/refactor/Phase26_システムリファクタリング方針.md を作成する。
  6. コード変更は行わない。
  7. 最後に、作成ファイル、判断基準の要点、次に進むべき作業を報告する。
```
