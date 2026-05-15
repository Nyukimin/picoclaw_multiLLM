# Phase3-2 ToolRunner 境界整理

## 目的

Phase 3-2 では、ToolRunner を tool execution adapter として整理する。

WorkerExecutionService は Coder proposal の patch 実行主体であり、ToolRunner は tool request の実行 adapter である。この 2 つを統合せず、ToolRunner 内の tool registration、metadata、execution middleware の境界を読みやすくする。

## 対象範囲

- `internal/infrastructure/tools/runner.go`
- `ToolRunnerConfig`
- `registerTools`
- V1 / V2 tool registration
- tool metadata registration

## 対象外

- tool の外部仕様変更。
- tool name の変更。
- ToolResponse error code の変更。
- ToolRegistry の永続化仕様変更。
- WorkerExecutionService への統合。
- PolicyEngine の policy 意味変更。
- web_search cache の挙動変更。

## 現在の責務

ToolRunner は現在、次を担っている。

- shell / file_read / file_write / file_list / web_search / subagent / register_tool の V1 登録。
- V2 tool execution wrapper の登録。
- tool metadata の登録。
- validation / timeout / retry middleware の接続。
- ToolRegistry / web_search cache / subagent など adapter 詳細の保持。

## 提案する分離単位

- `registerCoreTools`
- `registerOptionalTools`
- `registerToolMetadata`

Phase 3-2 では挙動変更を避けるため、tool 実行関数の中身は移動しない。

## 入力

- `ToolRunnerConfig`
- tool name
- `map[string]interface{}`
- `context.Context`

## 出力

- V1: string / error
- V2: `*tool.ToolResponse` / error
- `[]tool.ToolMetadata`

## 副作用

- shell 実行。
- file read / write / list。
- web_search。
- subagent execution。
- register_tool による ToolRegistry 登録。

## 永続化

- file_write 対象ファイル。
- web_search cache。
- ToolRegistry。

## ログ

- 既存の tool execution ログがある場合は維持する。
- Phase 3-2 では新しい監査ログ形式を導入しない。

## エラー契約

- unknown tool は unknown tool error。
- V2 は `ToolResponse` の error code を維持する。
- validation / timeout / not found / permission denied の分類を維持する。
- web_search disabled 時は web_search を登録しない。

## 変更してはいけない既存挙動

- registered tool name。
- `DisableWebSearch` の意味。
- `AllowedShellCommands` / `AllowedWritePaths` の意味。
- `Subagents` がある場合だけ subagent を登録する条件。
- `ToolRegistry` がある場合だけ register_tool を登録する条件。
- V1 / V2 の互換契約。

## 実装手順

1. baseline test を実行する。
2. `registerTools` から core tools / optional tools / metadata registration を小関数に分ける。
3. tool 実行関数の中身は変更しない。
4. tool name、metadata、条件分岐を変更しない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tools ./internal/infrastructure/security ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

## リスク

- web_search / subagent / register_tool の条件付き登録を崩す。
- V1 と V2 の登録対応をずらす。
- metadata だけ登録され、実行関数がない tool を作る。
- ToolRunner が policy 判断を抱え込む。
- WorkerExecutionService と ToolRunner の責務を混同する。

## 完了条件

- ToolRunner の registration 境界が関数名で追える。
- tool execution adapter としての責務が維持されている。
- Worker patch execution へ統合していない。
- 対象テストが成功している。
