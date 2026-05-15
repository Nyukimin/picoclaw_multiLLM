# Phase4-1 Coder selection 境界整理

## 目的

Phase4-1 は、`DefaultCodeExecutor` の Coder selection を proposal path / Generate path / Worker handoff から分けて読める状態にする。

挙動変更は行わない。`selectCoderForRoute` に集中している dynamic selection、explicit route selection、generic `CODE` selection を最小限の helper へ分け、入力、出力、副作用、エラー契約を関数単位で追えるようにする。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `selectCoderForRoute`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`
- `codeTarget`
- `CoderStatus` acquire / release 接続
- `capability.SelectCoder` 接続

## 対象外

- proposal path。
- Generate path。
- Worker handoff。
- `capability.SelectCoder` の選択ロジック。
- `CoderStatus` の内部実装。
- Coder provider。
- handler、DTO、SSE event、Viewer、IdleChat、STT / TTS、runtime config。
- 未追跡の `tests/`。

## 現在の責務

`selectCoderForRoute` は現在、次の責務を同じ関数内で扱っている。

- `coderCaps` がある場合に dynamic selection を行う。
- explicit route の `CODE1` / `CODE2` / `CODE3` / `CODE4` を Coder slot に対応付ける。
- generic `CODE` の fallback chain を処理する。
- `CoderStatus` がある場合に acquire し、release 関数を `codeTarget` に保持する。
- selected / skipped / degraded の log を出す。
- missing / unavailable / busy の error を返す。

## 提案する分離単位

- `selectDynamicCoderForRoute(route routing.Route) (codeTarget, error)`
  - `capability.SelectCoder` との接続を持つ。
  - degraded route を `codeTarget` に保持する。

- `selectExplicitCoderForRoute(route routing.Route, name, prompt string) (codeTarget, error)`
  - explicit route と Coder slot の対応を扱う。
  - Coder missing は既存 error を返す。

- `selectAvailableCoderForGenericRoute(route routing.Route) (codeTarget, error)`
  - generic `CODE` の fallback chain と `CoderStatus` acquire を扱う。
  - acquire 成功時に release 関数を `codeTarget` に保持する。

## 入力

- `routing.Route`
- `DefaultCodeExecutor` の Coder slots
- `DefaultCodeExecutor.coderCaps`
- `DefaultCodeExecutor.coderStatus`

## 出力

- `codeTarget`
- error

## 副作用

- dynamic / explicit / generic selection の log 出力。
- generic `CODE` で `CoderStatus` がある場合の acquire。
- acquire 成功時の release 関数生成。

## 永続化

なし。

## ログ

- dynamic selection: `mode=dynamic`
- explicit selection: `mode=explicit`
- generic selection: `mode=auto`
- unavailable / busy skip

ログ文字列の意味は変えない。

## エラー契約

- dynamic selection の `capability.SelectCoder` error は `%s route: %w` で wrap する。
- dynamic selection の selected Coder missing は `%s route: selected coder %s is not initialized` を返す。
- explicit route の Coder missing は `%s route requested but no %s available` を返す。
- generic `CODE` で `CoderStatus` ありの場合、全 busy / unavailable は `CODE route requested but all coders are busy or unavailable` を返す。
- generic `CODE` で `CoderStatus` なしの場合、全 unavailable は `CODE route requested but all coders are unavailable` を返す。
- unknown route は `unknown code route: %s` を返す。

## 変更してはいけない既存挙動

- `CODE1` / `CODE2` / `CODE3` / `CODE4` と Coder slot の対応。
- generic `CODE` の fallback order。
- `coderCaps != nil` のとき dynamic selection を優先すること。
- `systemPromptForRoute` の返す prompt。
- `CoderStatus` acquire / release の条件。
- proposal path / Generate path への入力になる `codeTarget` の意味。

## 実装手順

1. baseline test を実行する。
2. `selectCoderForRoute` を dispatcher として薄くする。
3. dynamic / explicit / generic selection helper を追加する。
4. proposal path / Generate path には触れない。
5. error message と log message を維持する。
6. gofmt を実行する。
7. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/capability ./cmd/picoclaw
git diff --check
```

## リスク

- dynamic selection より explicit selection を先に見てしまう。
- generic `CODE` の fallback order を変えてしまう。
- `CoderStatus` acquire 後の release 関数設定を落とす。
- error message を変えて既存テストや運用ログの読み方を壊す。
- selection helper が proposal path の責務まで持つ。

## 完了条件

- selection の dynamic / explicit / generic 境界が関数名で追える。
- proposal path / Generate path / Worker handoff に触れていない。
- CoderStatus release 契約テストが成功している。
- 対象パッケージのテストが成功している。
- `git diff --check` が成功している。
