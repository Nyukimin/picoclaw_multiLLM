# Phase2-3 response assembly 分離

## 目的

Phase 2-3 では、`MessageOrchestrator` 内の `ProcessMessageResponse` 組み立てを小さな private function に分ける。

route dispatch、session 保存、Viewer event、TTS hook と response assembly を混ぜず、最終返却値の契約を関数名で追える状態にする。

## 対象範囲

- `ProcessMessage` の最後に返す `ProcessMessageResponse` 組み立て。
- pre-routing chat command が返す `ProcessMessageResponse` 組み立て。
- response assembly 用 private function。

## 対象外

- route decision の意味変更。
- dispatch 先の変更。
- handler / DTO / SSE event の変更。
- Viewer JS / CSS。
- IdleChat 契約。
- STT / TTS provider。
- WorkerExecutionService。

## 守る契約

- 通常 route の `Response` は route 実行結果をそのまま返す。
- 通常 route の `Route` は `routing.Decision.Route` をそのまま返す。
- 通常 route の `Confidence` は `routing.Decision.Confidence` をそのまま返す。
- 通常 route の `JobID` は `task.JobID` を文字列化した値を返す。
- pre-routing chat command は `RouteCHAT`、`Confidence 1.0` を維持する。
- pre-routing chat command の `Response` は command result の response をそのまま返す。
- response assembly helper は Viewer event、TTS、session 保存、LLM 呼び出しを行わない。

## 実装方針

- `buildProcessMessageResponse` を追加し、通常 route の返却値を組み立てる。
- `buildChatCommandResponse` を追加し、pre-routing chat command の返却値を組み立てる。
- helper は入力値から構造体を作るだけに限定する。
- route dispatch や TTS hook の共通化は行わない。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## 完了条件

- `ProcessMessageResponse` の組み立て箇所が private function 名で追える。
- response assembly helper に副作用がない。
- pre-routing chat command の契約が維持されている。
- Phase 2-0 の契約テストが成功している。
