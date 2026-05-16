# Phase25 組み合わせテスト設計プロンプト

## Goal

RenCrow のリファクタリング後品質保証として、全機能の組み合わせテスト設計を `docs/refactor/` に作成してください。

## Repository

- `/home/nyukimi/picoclaw_multiLLM`

## 作成する文書

- `docs/refactor/Phase25_組み合わせテスト設計.md`

## 目的

- リファクタリング後の RenCrow について、どの組み合わせを unit / integration / e2e / live e2e で検証するかを明確にする。
- Chat / Worker / Coder、Viewer、IdleChat、STT、TTS、LLM provider、transport、runtime config、Memory / Source Registry の組み合わせを整理する。
- 「すべてを総当たりで実行する」のではなく、責務境界と障害リスクに基づいて、必須ケース、代表ケース、外部依存ケースを分類する。
- 今後の TDD / E2E 実装に使える検証設計書にする。

## 必ず参照するもの

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/01_正本仕様/実装仕様.md`
4. `docs/refactor/リファクタリング指針.md`
5. `docs/refactor/検証方針.md`
6. `docs/refactor/Phase24_最終完了判定.md`
7. `docs/codebase-map/アーキテクチャ総合.md`
8. `docs/codebase-map/結合ポイントマップ.md`
9. `docs/codebase-map/ユースケース逆引き.md`
10. `test/e2e/*.go`
11. `internal/application/orchestrator/*_test.go`
12. `cmd/picoclaw/*_test.go`

## 制約

- この作業ではテスト実装はしない。
- `docs/refactor/` 配下の Markdown 追加だけにする。
- ファイル名は日本語にする。
- archive 文書を一次参照にしない。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- 外部 API / Ollama / STT / TTS / ブラウザ依存ケースは、環境前提を明記する。
- 未確定の仮置き項目は残さない。

## 文書に必ず含める内容

### 1. 目的

- 組み合わせテスト設計の役割
- Phase24 完了判定との関係
- 今回は実装ではなく設計であること

### 2. テスト分類

- unit test
- integration test
- httptest e2e
- live e2e
- browser e2e
- manual observation
- external dependency e2e

### 3. 機能軸

- Chat / Worker / Coder
- route: CHAT / PLAN / ANALYZE / OPS / RESEARCH / CODE / CODE1 / CODE2 / CODE3
- LLM provider: local_openai / ollama / deepseek / openai / claude / gemini
- transport: local / ssh / mailbox / direct / distributed
- Viewer
- IdleChat
- STT
- TTS
- Audio router
- VTuber / lipsync
- runtime config
- Memory / Source Registry
- ToolRunner / PolicyEngine
- WorkerExecutionService
- CodeExecutor
- DistributedOrchestrator

### 4. 組み合わせマトリクス

以下を表で整理する。

- 対象機能
- 入力
- 経路
- 依存モジュール
- 期待結果
- 検証種別
- 必須度
- 外部依存
- 既存テスト
- 不足テスト
- 優先度

### 5. 必須ケース

最低限、以下を含める。

- Chat route が Worker / Coder 実行責務を持たない
- Worker route が file / shell / git / policy / log contract を守る
- Coder route が plan / patch 生成に留まる
- CODE / CODE1 / CODE2 / CODE3 の route 差分
- fallback が成功扱いされない
- nil provider / missing key / timeout / empty response
- route event / response / session / report の整合
- Viewer send / runtime config / llm ops
- IdleChat raw / view / audio contract
- STT input が通常 chat のみに流れる
- TTS chunk が表示本文の唯一根拠にならない
- audio / lipsync / log が混同されない
- local runtime config と repo example が混同されない
- distributed local / ssh / mailbox / direct route
- Memory / Source Registry の observed / candidate / validated / promoted 境界

### 6. 代表ケース

- 全総当たりを避ける理由
- 境界ごとに代表ケースを選ぶ基準
- 変更頻度、障害影響、外部依存、既存テスト有無による優先順位

### 7. 外部依存ケース

- API key が必要なもの
- Ollama / local_openai が必要なもの
- STT server が必要なもの
- TTS server が必要なもの
- browser permission / HTTPS が必要なもの
- skip 可能条件
- skip を成功扱いしない記録方法

### 8. Browser / Viewer 検証方針

- DOM 存在だけで成功扱いしない
- 1 session を開始から終了まで追う
- 表示本文、event log、audio trigger、lipsync trigger、終了状態を分けて確認する
- IdleChat は raw / view / audio を分けて確認する

### 9. TDD 実装順

- まず unit / contract test
- 次に integration test
- 次に httptest e2e
- 最後に browser / live e2e
- 既存の Phase test を壊さない
- 1回の Phase で複数の責務を混ぜない

### 10. 完了条件

- `Phase25_組み合わせテスト設計.md` が作成されている
- 組み合わせマトリクスがある
- 必須ケースが列挙されている
- 外部依存ケースと skip 条件が明記されている
- Browser / Viewer 検証方針が明記されている
- 今後の TDD 実装順が明記されている
- コード変更は行っていない

## 実行手順

1. 参照文書を読む。
2. 既存テストを確認する。
3. 機能軸を整理する。
4. 組み合わせマトリクスを作る。
5. 必須ケース、代表ケース、外部依存ケースを分類する。
6. `docs/refactor/Phase25_組み合わせテスト設計.md` を作成する。
7. コード変更は行わない。
8. 最後に、作成ファイル、設計の要点、不足テストの大分類を報告する。
