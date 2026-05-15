---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 2
step: "10"
profile: rencrow-core-map
artifact: module
module_group_id: application
---

# Application 層

## 概要

`internal/application/` は usecase と orchestration の層で、Chat/Worker/Coder の責務分離を実際の処理フローへ落とす。  
主要な入口は `MessageOrchestrator`、実行系は `WorkerExecutionService`、長時間会話は `idlechat`、安全な tool 実行要求は `execution.Service` が受け持つ。

## 関連ドキュメント

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/service/worker_execution_service.go`
- `internal/application/idlechat/orchestrator.go`
- `internal/application/execution/service.go`
- `internal/application/autonomous/service.go`
- `docs/01_正本仕様/実装仕様.md`
- `docs/07_IdleChat仕様/IdleChat仕様.md`

## 役割

- `orchestrator`: ユーザー入力を受け、Mio の route 判断、Shiro/Worker/Coder/Wild/Heavy への委譲、TTS/VTuber/Idle notifier/event emission を統合する。
- `service/worker_execution_service.go`: Coder proposal の patch command を順次/並列実行し、file edit/shell/git operation の実行分岐と保護ファイル判定を行う。
- `idlechat`: Mio/Shiro の自律会話、話題生成、要約、品質レビュー、TimelineEvent、TTS待機、ループ検知を管理する。
- `execution`: policy evaluator と tool executor を組み合わせ、tool 実行要求を record 化する。
- `autonomous`, `toolloop`, `heartbeat`, `health`, `knowledge`, `sourcefetcher`, `subagent`, `tts`, `channel`, `attachment`: 個別 usecase の調整役。

## 構造マップ

```text
MessageOrchestrator.ProcessMessage
  ├─ session load/create
  ├─ MioAgent.DecideAction
  ├─ route dispatch
  │   ├─ CHAT: Mio.Chat
  │   ├─ PLAN/ANALYZE/OPS/RESEARCH: Shiro / Worker path
  │   ├─ CODE*: Coder proposal + WorkerExecutionService
  │   ├─ WILD/HEAVY: specialized LLM
  │   └─ autonomous route: autonomous service path
  ├─ event/report hooks
  ├─ TTS / VTuber hooks
  └─ response assembly

WorkerExecutionService.ExecuteProposal
  ├─ execution summary
  ├─ sequential or parallel execution
  ├─ file edit / shell / git operation dispatch
  ├─ protected pattern checks
  └─ report result

IdleChatOrchestrator
  ├─ monitor loop / manual/forecast/story modes
  ├─ topic resolution
  ├─ generateResponse + sanitize + retry
  ├─ timeline event emission
  └─ summary / quality review / topic store
```

## 外部依存・被依存

- Adapter 層の LINE/Viewer/channel handler は Application usecase を呼ぶ。
- Infrastructure 層の LLM provider、tool runner、persistence、security、TTS/STT は Application から interface 経由で利用される。
- Domain 層の route、patch、agent contract に強く依存する。

## 落とし穴・注意点

- `MessageOrchestrator` は多機能で、route 追加や event 追加の影響範囲が広い。
- Worker execution は Coder の plan/patch を実行する場所なので、Coder 役割に実行責務を戻さない。
- `IdleChat` は LLM応答、表示イベント、TTS、口パク、要約、履歴保存が絡むため、音声 chunk を本文表示の唯一根拠にしない。
- fallback 応答は成功ではなくエラー経路の可視化として扱う必要がある。
- ※Phase 2 で追加: `WorkerExecutionService` に file edit/shell/git が集中しているため、ここを変更すると安全性と監査ログに直結する。

## 読むべき場面

- ルーティング結果と実際の agent 実行先がずれるとき。
- Coder proposal が Worker でどう適用されるか追うとき。
- IdleChat の表示・要約・TTS・ループ検知を直すとき。
- tool 実行の policy と record を確認するとき。
