# Phase12 task context builder 境界整理実装仕様作成プロンプト

```md
Goal:
  RenCrow のリファクタリング Phase12 として、MessageOrchestrator に残っている task context builder を collaborator 境界へ整理するための実装仕様書を作成してください。

Repository:
  - /home/nyukimi/picoclaw_multiLLM

Phase:
  - Phase12: task context builder 境界整理

目的:
  - `buildTaskForRequest` の task / job ID / attachment event / TTS session ID 生成責務を整理する。
  - attachment event は Viewer event であり、response text / execution log / audio chunk と混同しない。
  - TTS session ID 生成は TTS lifecycle provider 挙動ではなく、request context assembly として扱う。
  - Phase8-11 の collaborator 契約を維持する。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/01_正本仕様/実装仕様.md
  4. docs/refactor/リファクタリング指針.md
  5. docs/refactor/フォルダ構成方針.md
  6. docs/refactor/段階移行計画.md
  7. docs/refactor/検証方針.md
  8. docs/refactor/Phase10_完了判定.md
  9. docs/refactor/Phase11_完了判定.md
  10. docs/codebase-map/アーキテクチャ総合.md
  11. docs/codebase-map/modules/application.md
  12. docs/codebase-map/modules/潜在バグ一覧.md
  13. internal/application/orchestrator/message_orchestrator.go
  14. internal/application/orchestrator/message_orchestrator_task.go
  15. internal/application/orchestrator/message_orchestrator_events.go
  16. internal/application/orchestrator/message_orchestrator_tts_lifecycle.go
  17. internal/application/orchestrator/message_orchestrator_*test.go

作成する文書:
  - docs/refactor/Phase12_task_context_builder境界整理実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - TODO / TBD の仮置きは残さない。
  - 未追跡の tests/ は対象外として触らない。
  - handler、DTO、SSE event、Viewer JS / CSS、IdleChat、STT/TTS provider、LLM provider、runtime config の挙動は変更しない。
  - event payload の意味を変えない。

文書に必ず含める内容:
  1. Phase12 の目的
  2. 対象範囲
     - `message_orchestrator_task.go`
     - `buildTaskForRequest`
     - `MessageOrchestrator` constructor / field
     - attachment event
     - TTS session ID generation
  3. 対象外
  4. 現在の task context builder 構造
  5. 提案する collaborator
     - `messageTaskContextBuilder`
     - private struct
     - 初期段階では interface 化しない
     - `MessageOrchestrator` field として持つ
     - event port の emit function を受け取る
     - TTS enabled 判定 function を受け取る
  6. `messageTaskContextBuilder` の契約
     - 入力
     - 出力
     - 副作用
     - 永続化
     - ログ
     - エラー契約
     - 変更してはいけない既存挙動
  7. 実装手順
  8. テスト方針
  9. リスク
  10. 完了条件

実行手順:
  1. 参照文書を読む。
  2. task context builder 周辺コードを確認する。
  3. collaborator 境界と契約を棚卸しする。
  4. `docs/refactor/Phase12_task_context_builder境界整理実装仕様.md` を作成する。
  5. コード変更は行わない。
  6. 最後に、作成ファイル、仕様の要点、次に確認すべきことを報告する。
```
