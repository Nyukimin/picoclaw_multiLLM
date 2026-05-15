---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 2
step: "10"
profile: rencrow-core-map
artifact: module
module_group_id: domain
---

# Domain 層

## 概要

`internal/domain/` は Agent、Route、Task、PatchCommand、Tool、Conversation、Memory、Session などの中核契約を持つ。  
実行や永続化の詳細は含めず、Application/Infrastructure が使う型とルールの境界を定義する層である。

## 関連ドキュメント

- `internal/domain/agent/`
- `internal/domain/routing/route.go`
- `internal/domain/patch/command.go`
- `internal/domain/tool/`
- `internal/domain/conversation/`
- `internal/domain/execution/`
- `docs/01_正本仕様/03_エージェント定義.md`
- `docs/01_正本仕様/04_ルーティング.md`

## 役割

- `agent`: Mio/Shiro/Coder/Wild などのエージェント抽象と実装。
- `routing`: `CHAT`, `PLAN`, `ANALYZE`, `OPS`, `RESEARCH`, `CODE`, `CODE1..4`, `WILD` などの route と decision。
- `patch`: Worker が実行する `PatchCommand` の種類と action を表現する。
- `tool`: tool runner が扱う manifest、registry、validation、response。
- `conversation`: thread、message、persona、recall pack、profile extraction、summarizer の domain 契約。
- `execution`: 実行 record/report の domain model。
- `security`, `session`, `task`, `attachment`, `transport`, `llm`: 各境界の値と interface。

## 構造マップ

```text
internal/domain
  ├─ agent
  │   ├─ MioAgent / ShiroAgent / CoderAgent / WildAgent
  │   └─ persona, attachments, light memory, chat commands
  ├─ routing
  │   └─ Route + Decision
  ├─ patch
  │   └─ PatchCommand(Type, Action, Target, Content, Metadata)
  ├─ tool
  │   └─ manifest, registry, runner contract, validation
  ├─ conversation
  │   └─ message, thread, recall, persona, profile, engine interface
  └─ execution/session/security/task/transport/llm
```

## 外部依存・被依存

- Application 層の `MessageOrchestrator` は route、agent、proposal、patch、session の domain 契約を組み合わせる。
- Infrastructure 層の tools/security/persistence は domain の tool/execution/session/conversation 契約に実装を与える。
- Adapter 層は domain を直接深く扱うより、Application 経由で domain 結果を受け取るのが基本。

## 落とし穴・注意点

- `PatchCommand` は file edit、shell command、git operation を表現できるため、Domain で表現可能だからといって実行が許可されるわけではない。実行前に Application/Infrastructure 側の guard が必要。
- Route と Coder slot の対応を変えると、Orchestrator、Viewer 表示、分散実行、prompts の整合性に影響する。
- `conversation` は raw log、view data、prompt injection data の分離方針と衝突しやすい領域。表示・音声・記憶の境界を混ぜない。
- ※Phase 2 で追加: domain package は細かく分かれているが、命名が似た `memory`, `conversation`, `execution`, `session` は状態の所有者が異なる。変更前に主たる真実がどこかを確認する。

## 読むべき場面

- route や agent 役割を変えるとき。
- Worker が実行できる command/action を増やすとき。
- Tool manifest/validation を変更するとき。
- 会話記憶・recall・session の型を変えるとき。
