# Phase2-4 Viewer event / report / TTS hook 契約確認

## 目的

Phase 2-4 では、Phase 2 の route chain 分割後も Viewer event、execution report、TTS / VTuber hook の契約を混同せず維持できているかを確認する。

この Phase は契約確認を目的とし、Viewer JS / CSS、TTS provider、VTuber provider、report 保存形式は変更しない。

## 対象範囲

- `MessageOrchestrator` の event emission。
- autonomous execution の `ReportStore` 受け渡し。
- `startTTSSessionForRoute` / `endTTSSession`。
- `withStreamHooks` / `pushTTS`。
- route-specific function から呼ばれる TTS / VTuber hook。

## 対象外

- Viewer 表示ロジック。
- Viewer assets。
- TTS / STT provider 実装。
- VTuber provider 実装。
- execution report の保存 schema。
- WorkerExecutionService の内部実行。
- IdleChat raw/view/audio 契約。

## 守る契約

- Viewer event は `OrchestratorEvent` として `EventListener` へ渡す。
- `message.received` は routing 前のユーザー入力受信を表す。
- `routing.decision` は route 決定を表す。
- `agent.start` は各 agent への処理開始を表す。
- `agent.thinking` は stream token の進行表示であり、最終本文の唯一の根拠にしない。
- `agent.response` は route 実行結果の応答 event を表す。
- `entry.stage` は autonomous executor の stage を表す。
- execution report は autonomous executor の `ReportStore` に委譲し、Viewer event と混同しない。
- TTS / VTuber hook は表示ログではなく音声・口パク用の副作用として扱う。
- TTS start / push / end の失敗は degraded log として扱い、route 実行成功へすり替えない。

## 現在の確認結果

- `emit` は `EventListener` がある場合だけ `NewEvent` を渡す。
- `ProcessMessage` は `message.received`、`routing.decision`、`agent.start`、`agent.response` の順序を契約テストで固定している。
- autonomous execution は `ReportStore: o.reporter` を `autonomousapp.RunExecutor` に渡している。
- route-specific function は Viewer event の発火と TTS hook を分けて呼んでいる。
- stream hook は `agent.thinking` と TTS / VTuber stream forwarder を同じ token から起動するが、最終本文 assembly には使っていない。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

追加で確認した観点:

```bash
rg -n "\"(message.received|routing.decision|agent.start|agent.thinking|agent.response|entry.stage)\"" internal/application/orchestrator
rg -n "ReportStore|SetReportStore|startTTSSessionForRoute|endTTSSession|withStreamHooks|pushTTS" internal/application/orchestrator
```

## 完了条件

- Viewer event、execution report、TTS / VTuber hook の責務が文書上で分離されている。
- Phase 2 の関数分割で event 名、route 名、from/to の意味を変更していない。
- TTS / VTuber hook を Viewer 表示契約として扱っていない。
- 対象パッケージのテストが成功している。
