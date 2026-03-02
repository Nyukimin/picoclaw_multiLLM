# 出力例（sourceノート）

---
type: source
date: 2026-02-20
published: 2026-02-18
title: "Example: New evaluation method for long-context LLMs"
publisher: "arXiv"
url: "https://arxiv.org/abs/xxxx.xxxxx"
tags:
  - topics/llms
  - topics/evaluations
  - artifacts/papers
  - domains/researches
  - signals/benchmarks
  - publishers/arxivs
rating: 5
links:
  - "[[Topics - LLMs]]"
  - "[[Topics - Evaluations]]"
  - "[[Projects - OpenClaws]]"
  - "[[Weekly - 2026-W08]]"
summary: "長文コンテキストの評価を改善する新手法を提案し、既存ベンチマークとの比較結果を示す。"
---

# Example: New evaluation method for long-context LLMs

[[Weekly - 2026-W08]] / [[Topics - LLMs]] / [[Topics - Evaluations]] / [[Projects - OpenClaws]]

## 要点
- 長文コンテキスト性能の測定における弱点を整理し、新しい評価プロトコルを提案している
- 既存ベンチマークとの比較で、モデル間の差がより明確に出るケースを示している
- 再現可能性のための設定・手順が提示されている（付録・実験条件）

## 重要な事実
- 公開日：2026-02-18
- 収集日：2026-02-20
- 種別：論文（arXiv）
- 主張：長文評価の指標設計と測定手順を改良

## 引用（必要最小）
> 提案手法は長文条件での性能差をより安定して観測できる、という趣旨の記述。

## メモ（作業仮説）
- 仮説：既存の長文ベンチが「運が良いと通る」系のノイズを含んでいて、改善提案はその除去に寄っている可能性
- 根拠：測定条件の固定化と、差分が出る状況の説明に比重がある
- 再検証ポイント：同手法を自分の評価パイプラインに載せたとき、モデル順位がどれくらい入れ替わるか

## 次の一手
- 週次ToDoへ：[[Weekly - 2026-W08]]
- まとめ候補：[[Topics - Evaluations]] に「長文評価の観測ノイズ」節を追加
- 追加で調べるなら：同テーマの既存ベンチ（比較対象）と、その限界指摘の先行研究

---

# 出力例（weeklyノート：1週間に1つ）

---
type: weekly
date: 2026-02-16
week: 2026-W08
tags:
  - weeks/2026s
  - projects/openclaws
---

# Weekly - 2026-W08

## ToDo
- [ ] AI収集のタグ辞書をOpenClawに組み込む（topics/domains/methods/artifacts/signals/publishers）
- [ ] sourceノート出力で published（公開日）抽出が安定するか確認
- [ ] [[Topics - LLMs]] と [[Topics - Agents]] のハブノートを最低限整備

## Incoming（今週流入したsource）
- [[2026-02-20__source__openai-model-update]]
- [[2026-02-20__source__new-evaluation-method-long-context-llms]]

