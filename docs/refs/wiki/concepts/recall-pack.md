---
type: concept
status: active
owner: core
canonical_source: docs/01_正本仕様/18_Memory_Lifecycle_Recall_Context.md
source:
  - docs/01_正本仕様/18_Memory_Lifecycle_Recall_Context.md
  - docs/10_新仕様/09_Memory_SourceRegistry仕様.md
  - internal/domain/conversation/recall_pack.go
related:
  - docs/wiki/concepts/memory-lifecycle.md
  - docs/wiki/concepts/source-registry.md
updated: 2026-06-25
---

# RecallPack

RecallPack は Mio / Worker / Coder に渡す文脈を選別済みの形にした prompt 注入用フォーマットである。

DB 全体や docs 全体を読むのではなく、ターンごとに必要な会話文脈、UserMemory、Knowledge、SearchCache、Wiki snippet を選んで渡す。

## 現行 layer

- L0: 現在会話、short context、rolling summary
- L1: Search Cache、hot store、Wiki index
- L2: thread summary
- L3: long facts、Knowledge DB、Vector KB
- L4: Markdown Wiki / docs map

## 重要ルール

- chat role では `FilterForRole("chat")` を通す。
- token budget を守る。
- 採用 / 不採用を recall trace に残す。
- prompt text を保存正本にしない。
- Wiki snippet は source と path を失わない。

## Knowledge Wiki との関係

Knowledge Wiki は RecallPack の `WikiSnippets` として入る。
Mio は Markdown page 全文ではなく、SQL index から選ばれた短い summary と source link だけを読む。
