---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 2
step: "10"
profile: rencrow-core-map
artifact: module
module_group_id: entrypoints_config_docs
---

# エントリポイント・設定・仕様文書

## 概要

RenCrow の起動と運用設定は `cmd/picoclaw/main.go` を中心にまとまっており、Viewer、チャネル、LLM、IdleChat、TTS/STT、Source Registry などの配線をここで組み立てる。  
仕様判断の一次参照は `docs/01_正本仕様/実装仕様.md`、作業ルールは `AGENTS.md` と `rules/`、プロンプト実体は `prompts/` と `config/prompts/` に分離されている。

## 関連ドキュメント

- `cmd/picoclaw/main.go`
- `internal/adapter/config/config.go`
- `docs/01_正本仕様/実装仕様.md`
- `docs/LLM運用/README.md`
- `rules/PROJECT_AGENT.md`
- `prompts/README.md`

## 役割

- `cmd/picoclaw/main.go` は CLI サブコマンド、HTTP サーバ、Viewer handler、分散実行、IdleChat、LLM provider、TTS/STT runtime を結線する実行入口。
- `internal/adapter/config/config.go` は `Config` 配下に Server、LLM、Worker、Distributed、IdleChat、Conversation、Security、TTS、STT、VTuber、Coder などの設定構造を集約する。
- `prompts/` は Mio/Shiro/Coder/Worker/Classifier/skills などのプロンプト外部化先。
- `docs/` と `rules/` は実装判断の正本・運用ルールで、コードより上位の判断資料として扱う。

## 構造マップ

```text
cmd/picoclaw/main.go
  ├─ config load / defaults / validation
  ├─ provider build
  │   ├─ Chat / Worker / Heavy / Wild
  │   └─ conversation embedder / text provider
  ├─ dependencies build
  │   ├─ adapters: LINE, channel, Viewer, health
  │   ├─ application: orchestrator, idlechat, heartbeat
  │   └─ infrastructure: transports, stores, tool registry
  ├─ CLI commands
  │   ├─ health/status/doctor/logs/evidence
  │   ├─ source-registry/knowledge
  │   └─ ollama/gateway/channels
  └─ runtime service
      ├─ HTTP routes
      ├─ Viewer assets
      └─ shutdown hooks
```

## 外部依存・被依存

- `cmd/picoclaw` は Adapter/Application/Infrastructure/Domain の全層に依存する最外層。
- `internal/adapter/config.Config` は多数の runtime factory に渡されるため、設定項目追加時の影響範囲が広い。
- LLM運用 docs と runtime config のズレは Viewer の LLM Ops 表示や実際の provider 接続に直結する。

## 落とし穴・注意点

- `main.go` は責務が大きく、起動系の小変更でも Viewer、IdleChat、LLM、チャネルに波及しやすい。
- live runtime は `~/.picoclaw/config.yaml` で、repo 内 example だけを変えても実サービスには反映されない。
- prompt は Go ソースへ埋め込まず、`prompts/` または runtime-local の prompt bundle として扱う方針。
- `docs/archive/` は一次参照にしない。
- ※Phase 2 で追加: `cmd/picoclaw/main.go` に CLI と service wiring が集中しており、機能追加時は「設定追加」「provider追加」「handler追加」が同時に必要かをチェックする。

## 読むべき場面

- サービス起動・再起動・health・Viewer route の調査。
- LLM endpoint やモデル名、Chat/Worker/Wild/Heavy の配線を確認するとき。
- prompt や rules の参照先を変えるとき。
- 新しい CLI サブコマンドや runtime config を足すとき。
