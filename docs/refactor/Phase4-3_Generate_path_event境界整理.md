# Phase4-3 Generate path / event 境界整理

## 目的

Phase4-3 は、Generate path と CodeExecutor event emission の責務を明確にする。

proposal path に進まない場合の通常 Generate 実行と、degraded route notice / start / response event を helper へ分ける。Viewer-facing event の type、from、to、route、content の意味は変えない。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `ExecuteCode`
- `executeCoderGeneratePath`
- `emit`
- degraded route notice
- `agent.start`
- `agent.response`

## 対象外

- proposal path の条件や Worker handoff。
- Coder selection。
- Viewer JS / CSS。
- SSE event schema。
- MessageOrchestrator route chain。
- handler、DTO、IdleChat、STT / TTS、runtime config。
- 未追跡の `tests/`。

## 現在の責務

`ExecuteCode` と `executeCoderGeneratePath` は現在、次を直接行っている。

- degraded route notice event を emit する。
- degraded route log を出す。
- mio -> shiro の start event を emit する。
- shiro -> Coder の start event を emit する。
- Generate error event を emit する。
- Generate success の Coder response event を emit する。
- Generate success の Shiro response event を emit する。

## 提案する分離単位

- `emitDegradedRouteNotice(req CodeExecutionRequest, target codeTarget)`
  - degraded route notice と log を扱う。

- `emitCodeHandoffStart(req CodeExecutionRequest, target codeTarget)`
  - `agent.start` の開始 event を扱う。

- `emitCoderGenerateError(req CodeExecutionRequest, target codeTarget, err error)`
  - Generate error event を扱う。

- `emitCoderGenerateResponse(req CodeExecutionRequest, target codeTarget, response string)`
  - Generate success event を扱う。

## 入力

- `CodeExecutionRequest`
- `codeTarget`
- generated response
- Generate error

## 出力

- `CodeExecutionResponse`
- error
- CodeExecutor event

## 副作用

- Coder `Generate` 呼び出し。
- event emission。
- degraded route log。

## 永続化

なし。

## ログ

- degraded route log を維持する。
- selected / skipped log は selection 側に置く。

## エラー契約

- Generate error は event と error にする。
- Generate error は fallback success にしない。
- Generate path response は `Handled: false` とする。

## 変更してはいけない既存挙動

- degraded route notice の event type / from / to / route / content。
- `agent.start` の from / to / route / content。
- Generate error event の from / to / content。
- Generate success event の from / to / content。
- response truncation の条件。
- Generate path の `Handled: false`。

## 実装手順

1. baseline test を実行する。
2. event helper を必要最小限で追加する。
3. event type、from、to、content、route を変えない。
4. proposal path と selection path には触れない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

## リスク

- degraded route notice を success と混同する。
- event の from / to を変えて Viewer 側の観測を壊す。
- Generate error を隠して fallback success にする。
- response truncation の範囲を変える。
- proposal path の event と Generate path の event を混同する。

## 完了条件

- Generate path と event emission の境界が関数名で追える。
- Viewer-facing event 契約が維持されている。
- Generate error が error として返る。
- 対象パッケージのテストが成功している。
- `git diff --check` が成功している。
