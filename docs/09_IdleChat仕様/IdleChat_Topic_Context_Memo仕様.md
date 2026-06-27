# IdleChat Topic Context Memo 仕様

**作成日**: 2026-06-28
**対象**: IdleChat の `single` / `double` / `external` / `movie` / `news` / `forecast` お題キャッシュ、会話前コンテキスト、関連語句・語句説明
**親仕様**: `IdleChat仕様.md`, `IdleChat前準備仕様.md`, `IdleChat_Topic_Generator_Judge詳細仕様.md`, `未来展望セッション仕様.md`

## 1. 目的

IdleChat の topic は、単なるお題文字列だけでは会話が薄くなりやすい。
そのため、対象カテゴリでは topic cache に「お題」に加えて、関連語句、語句の意味、話題との関係を保持する。

この補助情報を、本仕様では topic context memo と呼ぶ。
実装上は `TopicGenerationResult.ContextTerms` として保持する。

目的は次の通り。

- Mio / Shiro が、題材の用語・背景・見方を自然に使って会話できるようにする。
- Viewer / TTS には説明カードをそのまま出さず、会話生成の内部補助としてだけ使う。
- `single` / `double` / `external` / `movie` / `news` / `forecast` のカテゴリ境界を保ち、別カテゴリへすり替えない。
- お題キャッシュ10件の中に、会話開始時に必要な最低限の補助文脈を同梱する。

## 2. 対象カテゴリ

本仕様の対象は次の6カテゴリとする。

| category | 対象 |
|---|---|
| `single` | 1ジャンル・1題材を深掘りする話題 |
| `double` | 2ジャンルの掛け合わせから共通構造を探す話題 |
| `external` | Wikipedia Random 等の外部刺激とジャンルを接続する話題 |
| `movie` | 架空映画のお題に対して、自前 Movie DB 由来の映画・俳優・監督等の関連メモを使う話題 |
| `news` | ニュース見出し1件を起点にした話題 |
| `forecast` | トレンド・ニュース・ドメインから作る未来展望話題 |

`movie` の詳細は `IdleChat_Movie_DB_Context_Memo仕様.md` に従う。
`story-simple` は生成物の性質が異なるため、本仕様では原則として `ContextTerms` の対象外とする。
将来追加する場合は、物語設定メモとして別途必要性を確認する。

## 3. データモデル

`TopicGenerationResult` に、任意フィールドとして `ContextTerms` を追加する。

```go
type TopicContextTerm struct {
    Term      string `json:"term"`
    Meaning   string `json:"meaning"`
    Relevance string `json:"relevance"`
    Source    string `json:"source,omitempty"`
}

type TopicGenerationResult struct {
    Topic    string        `json:"topic"`
    Category TopicCategory `json:"category"`
    Strategy string        `json:"strategy"`

    InterestingnessAxis string `json:"interestingness_axis"`
    OpeningHook         string `json:"opening_hook"`
    Avoid               string `json:"avoid"`

    Seed         TopicSeed          `json:"seed"`
    ContextTerms []TopicContextTerm `json:"context_terms,omitempty"`
    Candidates   []TopicCandidate   `json:"candidates,omitempty"`
    Judge        *TopicJudgeResult  `json:"judge,omitempty"`
    Provider     string             `json:"provider"`
}
```

### 3.1 TopicContextTerm

| field | 必須 | 内容 |
|---|---:|---|
| `term` | yes | 関連語句。固有名詞、制度名、技術語、ジャンル固有語、背景理解に必要な語句など |
| `meaning` | yes | 語句の短い意味。1〜2文。専門用語を避ける |
| `relevance` | yes | その語句がお題にどう関係するか |
| `source` | no | `genre`, `wikipedia`, `movie_catalog:movie`, `movie_catalog:person`, `news`, `trend`, `google_news`, `nhk`, `generated` など |

### 3.2 件数

- 1 topic あたり `3〜8` 件を目安とする。
- 最低 `3` 件を目標とする。
- ただし enrichment に失敗した場合でも、topic 自体が有効ならキャッシュしてよい。

## 4. 生成タイミング

関連語句・情報メモの生成は、ユーザーが待つ session 開始時ではなく、原則として topic cache 補充時に行う。

```text
topic cache refill
  -> seed 取得
  -> topic 生成
  -> context terms 生成
  -> TopicGenerationResult としてキャッシュ
```

session 開始時は、キャッシュ済みの `TopicGenerationResult` を取り出し、`ContextTerms` を `sessionContext` に変換して会話生成へ渡す。

## 5. single の ContextTerms

### 5.1 入力

single の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- 選択された genre
- genre pool 上の分類や seed
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 5.2 語句候補

single では次の種類を優先して抽出・生成する。

- 題材を理解するための基本語句
- 題材の細部観察に使える部位名・場面名・道具名
- 題材の歴史、用途、慣習、文化背景に関係する語句
- 会話を具体化するための見どころや比較軸

### 5.3 禁止事項

- 単なる百科事典的な説明だけにしない。
- topic と関係の薄い周辺知識を増やしすぎない。
- `single` を `external` や `news` として扱わない。
- 語句説明をそのまま発話させる前提の長文解説にしない。

## 6. double の ContextTerms

### 6.1 入力

double の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- 選択された2つの genre
- 2ジャンルの接続理由
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 6.2 語句候補

double では次の種類を優先して抽出・生成する。

- 1つ目のジャンルを理解するための基本語句
- 2つ目のジャンルを理解するための基本語句
- 2ジャンルをつなぐ共通構造、比喩、役割、動き
- 違いが際立つ対比軸

### 6.3 禁止事項

- 片方のジャンルだけに寄せない。
- 2ジャンルをただ並べるだけにしない。
- 無理なこじつけを事実のように扱わない。
- `double` を `single` 2件分として扱わない。

## 7. external の ContextTerms

### 7.1 入力

external の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- 外部素材の title / summary / provider / url
- 組み合わせる genre
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 7.2 語句候補

external では次の種類を優先して抽出・生成する。

- 外部素材を理解するための基本語句
- 外部素材と genre を自然につなぐ語句
- provider 由来の固有名詞の短い説明
- 偶然の素材を会話内で意味化するための視点

### 7.3 禁止事項

- provider 名、取得経路、URL を発話本文へ漏らさない。
- 外部素材を読んでいない範囲まで断定しない。
- ニュース見出しそのものを深掘りする場合は `news` を使う。
- `external` を `news` の代替カテゴリとして扱わない。

## 8. movie の ContextTerms

movie の enrichment は、自前 Movie DB を参照して行う。
詳細仕様は `IdleChat_Movie_DB_Context_Memo仕様.md` を参照する。

基本方針:

- `TopicGenerationResult.Topic` と `TopicGenerationResult.Seed.Genre1` から検索語を作る。
- `movies / people / movie_people` から、映画・俳優・監督等の関連情報を引く。
- `ContextTerms` には、映画タイトル、人物名、短い意味、topicとの関係、sourceを入れる。
- 架空映画 topic を実在作品として断定しない。
- DBが無い、検索できない、結果が無い場合でも topic 自体は有効とする。

## 9. news の ContextTerms

### 9.1 入力

news の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- `TopicGenerationResult.Seed.News.Title`
- `TopicGenerationResult.Seed.News.Category`
- `TopicGenerationResult.Seed.News.Source`
- `TopicGenerationResult.Seed.News.URL`
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 9.2 語句候補

news では次の種類を優先して抽出・生成する。

- ニュース見出しに含まれる制度名・組織名・地名・技術名
- 背景を理解するために必要な社会制度や業界用語
- 判断が割れやすい論点を表す語句
- 影響を受ける主体を理解するための語句

### 9.3 禁止事項

- ニュース本文を取得していないのに、本文を読んだように説明しない。
- 見出しから確定できない因果関係を断定しない。
- `news` を `external` として扱わない。
- 説明をそのまま発話させる前提の長文解説にしない。

## 10. forecast の ContextTerms

### 10.1 入力

forecast の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- `TopicGenerationResult.Seed.ForecastDomain`
- `TopicGenerationResult.Seed.TrendKeywords`
- keyword extraction で得た代表キーワード
- NHK / Google News / trend 由来の seed headlines
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 10.2 語句候補

forecast では次の種類を優先して抽出・生成する。

- 未来展望の分岐に関係する技術・制度・市場・社会用語
- ドメイン固有の背景語句
- 3〜10年後の変化を考えるうえで必要な前提語句
- 現在の兆しと将来影響をつなぐ語句

### 10.3 禁止事項

- 未来予測を事実として断定しない。
- 語句説明を専門解説だけで終わらせない。
- トレンド seed やニュース seed を Viewer / TTS にそのまま列挙しない。
- 外部 provider 名、検索経路、内部診断を発話本文へ漏らさない。

## 11. sessionContext への注入

`ContextTerms` は `formatTopicGenerationContext()` で内部補助として整形する。
Viewer / TTS / topic display には出さない。

推奨フォーマット:

```text
【関連語句】
- {term}: {meaning} / お題との関係: {relevance}
- ...

注意:
- 上の語句説明をそのまま読み上げない。
- 会話の中で必要な時だけ自然に使う。
- 出典名、内部seed、provider名を発話本文に出さない。
```

## 12. キャッシュとの関係

対象カテゴリの topic cache は、topic 文字列だけではなく、`TopicGenerationResult` 全体を保持する。

保持対象:

- `topic`
- `category`
- `strategy`
- `seed`
- `interestingness_axis`
- `opening_hook`
- `avoid`
- `context_terms`
- `provider`

キャッシュ上限は既存の topic cache 仕様に従い、各カテゴリまたは各 forecast domain ごとに10件を目標とする。

## 13. 失敗時動作

### 13.1 enrichment 失敗

関連語句生成に失敗しても、topic が有効なら topic cache へ入れてよい。
その場合、次をログに残す。

```text
context_terms_generation_failed
category=single|double|external|movie|news|forecast
topic=<topic>
error_code=<code>
```

session 開始時に `ContextTerms` が空の場合は、従来通り `InterestingnessAxis / OpeningHook / Avoid / Seed` だけで会話する。

### 13.2 topic 生成失敗

topic 生成自体が失敗した場合は、従来通り該当カテゴリの topic cache に入れない。
別カテゴリへのすり替えは禁止する。

## 14. 実装チェックリスト

- [x] `TopicContextTerm` を共通型として追加する。
- [x] `TopicGenerationResult.ContextTerms` を追加する。
- [ ] `single` topic cache 補充時に `ContextTerms` を生成する。
- [ ] `double` topic cache 補充時に `ContextTerms` を生成する。
- [ ] `external` topic cache 補充時に `ContextTerms` を生成する。
- [x] `movie` topic cache 補充時に Movie DB 由来の `ContextTerms` を生成する。
- [ ] `news` topic cache 補充時に `ContextTerms` を生成する。
- [ ] `forecast` topic cache 補充時に `ContextTerms` を生成する。
- [x] `formatTopicGenerationContext()` が `ContextTerms` を内部補助として注入する。
- [x] Viewer / TTS の topic 表示に `ContextTerms` が混入しない。
- [x] `ContextTerms` 生成失敗時も topic 自体は利用可能にする。
- [x] `movie` の category / strategy を維持する。
- [ ] `single` / `double` / `external` / `news` / `forecast` の category / strategy を維持する。
- [ ] `story-simple` に本仕様の `ContextTerms` を混ぜない。
- [ ] テストで `ContextTerms` が sessionContext に入ること、Viewer/TTS に出ないことを確認する。
