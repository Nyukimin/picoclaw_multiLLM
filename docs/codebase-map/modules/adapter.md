---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 2
step: "10"
profile: rencrow-core-map
artifact: module
module_group_id: adapter
---

# Adapter 層

## 概要

`internal/adapter/` は Viewer、LINE、Slack/Discord/Telegram、health、entry、config、Chrome bridge など外部入口と表示面を扱う。  
Application usecase を HTTP/チャネル/UI の形に変換し、Viewer は実行状態・記憶・LLM ops・IdleChat・音声周辺の観測面になる。

## 関連ドキュメント

- `internal/adapter/viewer/handler.go`
- `internal/adapter/viewer/source_registry_handler.go`
- `internal/adapter/viewer/assets/js/`
- `internal/adapter/line/handler.go`
- `internal/adapter/entry/handler.go`
- `internal/adapter/health/handler.go`
- `internal/adapter/config/config.go`
- `docs/09_Viewer/Viewer仕様.md`

## 役割

- `viewer`: HTML/assets、SSE hub、send API、event log、memory panels、Source Registry、LLM ops、glossary、recall trace、audio router、STT capture。
- `line`: LINE webhook の署名検証、イベント正規化、reply/send、media download。
- `channels`: Slack/Discord/Telegram adapter と共通 type。
- `entry`: platform/channel/user/session/message を正規化し、Application processor へ渡す簡易入口。
- `health`: `/health` と `/ready` 系 handler。
- `config`: config load/default/validation と prompt bundle。
- `chrome`: Chrome bridge の HTTP handler。

## 構造マップ

```text
HTTP / channel event
  ├─ line/channels/entry handlers
  │   └─ Application Orchestrator / Processor
  ├─ viewer handlers
  │   ├─ page/assets
  │   ├─ /viewer/send
  │   ├─ SSE hub + event logs
  │   ├─ memory/source registry/recall/ops
  │   └─ audio/STT endpoints
  └─ health/config/chrome handlers
```

## 外部依存・被依存

- `cmd/picoclaw` が handler を配線する。
- Viewer は Application events と Infrastructure stores の状態を表示するため、表示用 state と raw log の境界を意識する必要がある。
- LINE/channels は外部 platform event を `ProcessMessageRequest` 相当に変換する境界。

## 落とし穴・注意点

- Viewer は DOM の存在だけでは完了確認にならない。最低 1 セッションを追い、表示本文、イベントログ、終了状態を照合する。
- `viewerSendAliasSpec` など Viewer 側 alias と runtime LLM config がずれると、見た目の選択肢と実際の provider が一致しない。
- Memory/Source Registry handler は official DB 相当の store に触れるため、保存と promote の違いを混同しない。
- LINE webhook は署名検証と media download が絡むため、テスト token なしでの部分確認に限界がある。
- ※Phase 2 で追加: Viewer assets は `internal/adapter/viewer/viewer.html` と `internal/adapter/viewer/assets/...` が served path で、draft/redesign 系と取り違えない。

## 読むべき場面

- Viewer 表示、SSE、IdleChat UI、memory panel、LLM ops panel を直すとき。
- チャネル入力から Orchestrator に渡る session/channel/user 情報を追うとき。
- Source Registry や memory action の HTTP contract を確認するとき。
- health/ready の公開状態を変更するとき。
