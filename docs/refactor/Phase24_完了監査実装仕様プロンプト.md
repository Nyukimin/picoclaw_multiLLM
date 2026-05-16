# Phase24 完了監査実装仕様プロンプト

## Goal

RenCrow の段階的リファクタリング Phase 1 から Phase 23 までの成果を監査し、リファクタリング完了判定、検証結果、README 反映、残リスクを文書化してください。

## Repository

- `/home/nyukimi/picoclaw_multiLLM`

## Phase

- Phase 24: リファクタリング完了監査

## 目的

- これまでの Phase が、プロンプト作成、実装仕様作成、実装、検証、Push のサイクルで完了していることを確認する。
- `cmd/picoclaw`、`MessageOrchestrator`、`CodeExecutor`、`DistributedOrchestrator` の責務分割が、モジュール化と疎結合の方針に沿っていることを確認する。
- 通常テストと E2E テストの実行条件を明記し、現在の実装に合わせて README を更新する。
- 追加の大規模リファクタを開始せず、完了判定に必要な最小修正だけを行う。

## 必ず参照するもの

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/01_正本仕様/実装仕様.md`
4. `docs/refactor/リファクタリング指針.md`
5. `docs/refactor/フォルダ構成方針.md`
6. `docs/refactor/段階移行計画.md`
7. `docs/refactor/検証方針.md`
8. `docs/refactor/Phase*_*.md`
9. `docs/codebase-map/アーキテクチャ総合.md`
10. `docs/codebase-map/結合ポイントマップ.md`
11. `docs/codebase-map/ユースケース逆引き.md`
12. `docs/codebase-map/modules/*.md`
13. `cmd/picoclaw/main.go`
14. `cmd/picoclaw/routes.go`
15. `internal/application/orchestrator/*.go`
16. `test/e2e/*.go`
17. `README.md`

## 制約

- 仕様変更を混ぜない。
- handler 本体、DTO、SSE event、Viewer 表示契約、IdleChat 契約、STT/TTS provider、LLM provider の挙動を変更しない。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- `docs/archive/` を一次参照にしない。
- 未追跡の `tests/` は触らない。
- 未確定の仮置き項目は残さない。

## 実施内容

1. `git status --short --branch` を確認する。
2. Phase 1 から Phase 23 までの prompt / 実装仕様 / 完了判定文書を確認する。
3. 現在の主要ファイル構成を確認する。
4. 通常テストを実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./...`
5. E2E テストを実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e`
6. E2E が環境依存で失敗する場合は、失敗理由が外部依存か実装不具合かを切り分ける。
7. 必要なら E2E 実行条件だけを最小修正する。
8. `README.md` に現在のリファクタリング後の構成と検証コマンドを反映する。
9. `docs/refactor/Phase24_完了監査実装仕様.md` と `docs/refactor/Phase24_最終完了判定.md` を作成する。
10. `git diff --check` を実行する。
11. 変更を commit / Push する。

## 完了条件

- Phase 24 の実装仕様と最終完了判定が `docs/refactor/` に作成されている。
- README が現在の構成と検証コマンドに合わせて更新されている。
- 通常テストが成功している。
- E2E テストが成功している。
- E2E の外部依存 skip は、実装成功としてではなく環境未準備として明記されている。
- `git diff --check` が成功している。
- 未追跡の `tests/` を触っていない。
- 変更が Push 済みである。
