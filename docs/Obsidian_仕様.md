# OpenClaw × Obsidian（AI情報収集特化）ストア仕様書 v1.1

## 1. 目的とスコープ
本仕様書は、OpenClawで収集したAI関連情報をObsidianの単一Vaultへ蓄積し、検索性と再利用性を最大化するための保存形式・タグ設計・運用ルールを定義する。  
対象はAI関連の外部情報（論文、ブログ、リポジトリ、リリース、ベンチマーク、政策/規制等）とする。

## 2. 基本運用ルール（Obsidian運用方針）
- ノートを複数のVaultに分割しない（Vaultは1つのみ）
- フォルダ分けによる整理を避ける（Vault直下運用を前提）
- 標準的ではない独自のMarkdown記法を使わない（標準Markdown＋YAML frontmatter）
- 内部リンクを多用する（Obsidianの [[内部リンク]] を使用）
- 日付は常に「YYYY-MM-DD」形式で統一する
- 評価は7段階評価（1〜7の整数）を用いる
- ToDoリストは1週間につき1つの週次ノートに集約する
- カテゴリー名とタグ名は常に複数形にする（タグの名前空間を複数形で統一）

## 3. ノート種別（type）
ノートはYAML frontmatterの `type` により種別を識別する。
- source：外部情報の1件クリップ（OpenClawが自動生成する主対象）
- weekly：週次ToDoおよび当週の流入整理（手動または半自動）
（必要になったら追加：note / person / glossary 等。ただし最初は増やさない）

## 4. ファイル命名規則（フォルダなし運用）
フォルダを使用しないため、ファイル名で最低限の並びと衝突回避を行う。

- 形式：`YYYY-MM-DD__source__<slug>.md`
- YYYY-MM-DD：収集日（`date` と一致させる）
- slug：タイトルから生成した短い識別子（安全な英数字とハイフンを推奨）

例：
- `2026-02-20__source__openai-model-update.md`

## 5. Frontmatter（必須メタ）
### 5.1 sourceノート（必須）
必須：
- type: "source"
- date: "YYYY-MM-DD"（収集日）
- title: 記事/論文/リリースのタイトル
- url: 出典URL
- tags: タグ配列（本仕様のタグ設計に従う）
- rating: 1〜7 の整数（評価軸は「有用度」で固定）

推奨（ただし可能な限り付与する）：
- published: "YYYY-MM-DD"（公開日。取得できない場合は省略可）
- publisher: 媒体名（例：arXiv / OpenAI / Anthropic / Google / 日経 等）
- links: 内部リンク配列（ハブノートへのリンク文字列）
- summary: 1〜2文の極短い要約（後段本文に詳細がある前提）

ルール：
- `date` と `published` は常に `YYYY-MM-DD`
- 公開日が取得できない場合は `published` を省略する（`published: ""` は非推奨）

## 6. TAG設計（AI特化・複数形統一）
タグは「名前空間（prefix）」で統一し、名前空間自体を複数形とする。  
これにより、表記ゆれを最小化し、検索と自動付与を安定させる。

### 6.1 名前空間（固定）
- topics/*：何の話題か
- domains/*：文脈（研究/プロダクト/政策など）
- methods/*：手法
- artifacts/*：情報の種類（論文/リリース等）
- signals/*：性質・重要度（速報/回帰/再現等）
- publishers/*：媒体・発信元（検索・フィルタのため）

### 6.2 最小コア辞書（推奨）
topics：
- topics/llms
- topics/agents
- topics/rags
- topics/safeties

artifacts：
- artifacts/papers
- artifacts/releases
- artifacts/repos
- artifacts/blogs

signals：
- signals/breakings
- signals/benchmarks
- signals/regressions

publishers（例。運用しながら増やす）：
- publishers/arxivs
- publishers/openais
- publishers/anthropics
- publishers/googles
- publishers/microsofts
- publishers/metas
- publishers/nikkeis

（必要に応じて拡張候補）
topics：topics/multimodals, topics/finetunings, topics/inferences, topics/optimizations, topics/hardwares, topics/governances, topics/open-sources, topics/evaluations
domains：domains/researches, domains/products, domains/ops, domains/policies, domains/legalities, domains/educations
methods：methods/transformers, methods/diffusions, methods/rlhfs, methods/dpos, methods/rerankers, methods/vector-searches, methods/tool-callings
artifacts：artifacts/benchmarks, artifacts/datasets, artifacts/models
signals：signals/replications, signals/controversies

### 6.3 タグ付与ルール（ノートあたり3〜6個）
- 必ず1個：topics/*
- できれば1個：artifacts/*
- 必要に応じて：domains/* と methods/*
- 速報性や重要度がある場合のみ：signals/*
- 可能なら1個：publishers/*（発信元での検索性を上げる）

例：
- 新しいLLM論文：
  tags: [topics/llms, artifacts/papers, domains/researches, signals/benchmarks, publishers/arxivs]
- Agentフレームワークの更新：
  tags: [topics/agents, artifacts/repos, domains/products, signals/releases, publishers/githubs]（※githubsを使うなら追加定義）
- 政策・規制の動き：
  tags: [topics/governances, domains/policies, artifacts/blogs, signals/breakings, publishers/governments]（※governmentsを使うなら追加定義）

### 6.4 「複数形縛り」例外処理（不可算名詞の扱い）
英語として複数形にしにくい概念（例：security 等）は、複合語で複数形に逃がす。
例：
- topics/security-risks
- topics/security-controls
- topics/security-incidents

## 7. 内部リンク設計（フォルダの代替）
フォルダを使わない代わりに、ハブノートへ必ずリンクし、ネットワークで整理する。

sourceノートが必ずリンクすべきハブ：
- 週次ノート：[[Weekly - YYYY-Www]]
- 話題ハブ：[[Topics - <TopicName>]]（例：[[Topics - LLMs]]）
- プロジェクトハブ：[[Projects - OpenClaws]]

備考：
- ハブノートの作成は手動でもよいが、運用が安定したらOpenClawが存在チェックして自動生成してもよい。

## 8. 週次ToDo（1週間1ノート）
ToDoは必ず週次ノートに集約し、sourceノート側には「週次へのリンク」のみを置く。

週次ノート名：
- `Weekly - YYYY-Www`

週次ノートに含める最低要素：
- ToDo（チェックリスト）
- Incoming（その週に流入したsourceリンク一覧）

## 9. sourceノート本文テンプレ（標準Markdown）
以下は推奨テンプレであり、OpenClawの出力形式として固定する。

- 冒頭にハブへの内部リンクを並べる
- 要点→事実→引用→仮説→次の一手 の順で統一する

## 10. 評価（rating 1〜7）の定義
ratingは「有用度」を表し、整数1〜7で固定する。
- 1：ほぼ不要（ノイズ）
- 2：参考程度
- 3：軽い示唆
- 4：標準（読む価値あり）
- 5：重要（プロジェクトに効く）
- 6：かなり重要（意思決定に影響）
- 7：必読（方針変更/重大インパクト）

## 11. OpenClaw実装上の注意（保存の原子性）
Obsidianは書き込み途中のファイルを読みうるため、保存は原子性を確保する。
- 一時ファイルに書き出し → rename/move で確定（同一ファイルシステム内）
