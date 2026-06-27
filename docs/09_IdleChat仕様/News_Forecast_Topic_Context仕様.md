# News / Forecast Topic Context 仕様

**作成日**: 2026-06-28
**対象**: IdleChat の `news` / `forecast` お題キャッシュ、会話前コンテキスト、関連語句・語句説明
**親仕様**: `IdleChat仕様.md`, `IdleChat前準備仕様.md`, `IdleChat_Topic_Generator_Judge詳細仕様.md`, `未来展望セッション仕様.md`

## 1. 目的

`news` と `forecast` は、単なるお題文字列だけでは会話が薄くなりやすい。
そのため、両カテゴリでは topic cache に「お題」に加えて、関連語句、語句の意味、話題との関係を保持する。

目的は次の通り。

- Mio / Shiro がニュースや未来展望を、表面的な感想ではなく背景・論点・影響まで自然に話せるようにする。
- ただし、Viewer / TTS には説明カードをそのまま出さず、会話生成の内部補助としてだけ使う。
- `news` と `forecast` のカテゴリ境界を保ち、`external` や一般雑談へすり替えない。
- お題キャッシュ10件の中に、会話開始時に必要な最低限の補助文脈を同梱する。

## 2. 対象カテゴリ

本仕様の対象は次の2カテゴリに限定する。

| category | 対象 |
|---|---|
| `news` | ニュース見出し1件を起点にした話題 |
| `forecast` | トレンド・ニュース・ドメインから作る未来展望話題 |

`single / double / external / movie / story-simple` には原則として `ContextTerms` を付けない。
将来追加する場合も、本仕様とは別にカテゴリ別の必要性を確認する。

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
| `term` | yes | 関連語句。固有名詞、制度名、技術語、社会的論点など |
| `meaning` | yes | 語句の短い意味。1〜2文。専門用語を避ける |
| `relevance` | yes | その語句がお題にどう関係するか |
| `source` | no | `news`, `trend`, `google_news`, `nhk`, `generated` など |

### 3.2 件数

- 1 topic あたり `3〜8` 件を目安とする。
- 最低 `3` 件を目標とする。
- ただし enrichment に失敗した場合でも、topic 自体が有効ならキャッシュしてよい。

## 4. 生成タイミング

関連語句の生成は、ユーザーが待つ session 開始時ではなく、原則として topic cache 補充時に行う。

```text
topic cache refill
  -> seed 取得
  -> topic 生成
  -> context terms 生成
  -> TopicGenerationResult としてキャッシュ
```

session 開始時は、キャッシュ済みの `TopicGenerationResult` を取り出し、`ContextTerms` を `sessionContext` に変換して会話生成へ渡す。

## 5. news の ContextTerms

### 5.1 入力

news の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- `TopicGenerationResult.Seed.News.Title`
- `TopicGenerationResult.Seed.News.Category`
- `TopicGenerationResult.Seed.News.Source`
- `TopicGenerationResult.Seed.News.URL`
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 5.2 語句候補

news では次の種類を優先して抽出・生成する。

- ニュース見出しに含まれる制度名・組織名・地名・技術名
- 背景を理解するために必要な社会制度や業界用語
- 判断が割れやすい論点を表す語句
- 影響を受ける主体を理解するための語句

### 5.3 禁止事項

- ニュース本文を取得していないのに、本文を読んだように説明しない。
- 見出しから確定できない因果関係を断定しない。
- `news` を `external` として扱わない。
- 説明をそのまま発話させる前提の長文解説にしない。

## 6. forecast の ContextTerms

### 6.1 入力

forecast の enrichment は、次の情報を入力にする。

- `TopicGenerationResult.Topic`
- `TopicGenerationResult.Seed.ForecastDomain`
- `TopicGenerationResult.Seed.TrendKeywords`
- keyword extraction で得た代表キーワード
- NHK / Google News / trend 由来の seed headlines
- `TopicGenerationResult.InterestingnessAxis`
- `TopicGenerationResult.OpeningHook`

### 6.2 語句候補

forecast では次の種類を優先して抽出・生成する。

- 未来展望の分岐に関係する技術・制度・市場・社会用語
- ドメイン固有の背景語句
- 3〜10年後の変化を考えるうえで必要な前提語句
- 現在の兆しと将来影響をつなぐ語句

### 6.3 禁止事項

- 未来予測を事実として断定しない。
- 語句説明を専門解説だけで終わらせない。
- トレンド seed やニュース seed を Viewer / TTS にそのまま列挙しない。
- 外部 provider 名、検索経路、内部診断を発話本文へ漏らさない。

## 7. sessionContext への注入

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

## 8. キャッシュとの関係

`news` / `forecast` の topic cache は、topic 文字列だけではなく、`TopicGenerationResult` 全体を保持する。

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

## 9. 失敗時動作

### 9.1 enrichment 失敗

関連語句生成に失敗しても、topic が有効なら topic cache へ入れてよい。
その場合、次をログに残す。

```text
context_terms_generation_failed
category=news|forecast
topic=<topic>
error_code=<code>
```

session 開始時に `ContextTerms` が空の場合は、従来通り `InterestingnessAxis / OpeningHook / Avoid / Seed` だけで会話する。

### 9.2 topic 生成失敗

topic 生成自体が失敗した場合は、従来通り該当カテゴリの topic cache に入れない。
別カテゴリへのすり替えは禁止する。

## 10. 実装チェックリスト

- [ ] `TopicContextTerm` を共通型として追加する。
- [ ] `TopicGenerationResult.ContextTerms` を追加する。
- [ ] `news` topic cache 補充時に `ContextTerms` を生成する。
- [ ] `forecast` topic cache 補充時に `ContextTerms` を生成する。
- [ ] `formatTopicGenerationContext()` が `ContextTerms` を内部補助として注入する。
- [ ] Viewer / TTS の topic 表示に `ContextTerms` が混入しない。
- [ ] `ContextTerms` 生成失敗時も topic 自体は利用可能にする。
- [ ] `news` は `news`、`forecast` は `forecast` の category / strategy を維持する。
- [ ] テストで `ContextTerms` が sessionContext に入ること、Viewer/TTS に出ないことを確認する。
