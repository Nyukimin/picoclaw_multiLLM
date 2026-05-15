# Phase2-1 ProcessMessage 内部ステップ命名

## 目的

`MessageOrchestrator.ProcessMessage` の処理順を private helper として命名し、route chain の読みやすさを上げる。

Phase 2-1 では route dispatch の中身を変更しない。`ProcessMessage` が何を順に行っているかをコード上で追えるようにする。

## 対象範囲

- `MessageOrchestrator.ProcessMessage`
- session load / create
- chat command handling
- task creation
- route decision
- TTS session start / end
- session save

## 対象外

- `executeTask`
- `executeAutonomousTask`
- `executeRouteDirect`
- `CodeExecutor`
- `WorkerExecutionService`
- Viewer JS / CSS
- STT / TTS provider

## 守る契約

- `HandleChatCommand` は routing 前に呼ぶ。
- chat command handled は `RouteCHAT`、confidence `1.0` を返す。
- `message.received` は route 判断前に emit する。
- attachment がある場合は `viewer.attachment.received` を emit する。
- route decision 後に `routing.decision` を emit する。
- TTS session ID は `sessionID-jobID` のままにする。
- CHAT 以外では worker busy を true にし、終了時に false に戻す。
- task execution 後に session へ task を追加し、保存する。
- error wrapping の文言を変更しない。

## 実装方針

- `ProcessMessage` は処理順を示す薄い関数にする。
- helper は `message_orchestrator.go` 内の private function とする。
- package 外 API は増やさない。
- event、log、error、response の内容を変更しない。
- route dispatch は Phase 2-2 で扱う。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
git diff --stat
```

## 完了条件

- `ProcessMessage` の主要ステップが helper 名で追える。
- route dispatch の挙動が変わっていない。
- Phase 2-0 の契約テストが成功している。
- `git diff --check` が成功している。
