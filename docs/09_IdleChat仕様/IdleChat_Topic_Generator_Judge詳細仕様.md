# IdleChat Topic Generator / Interestingness Judge 詳細仕様

## 0. 背景と目的

IdleChat の topic generator は、IdleChat セッションの入口となる topic をカテゴリ別に生成する。
Interestingness Judge は、生成された topic 候補を評価し、短く、具体的で、会話が広がる topic を選ぶ。

Story は構築中に非現実的であることが分かったため削除する。
story-simple は Story の代替として実装済みの独立カテゴリ / mode であり、Story とは別物として扱う。

## 1. 対象カテゴリ

対象カテゴリは次の7つ。

```text
single / double / external / movie / news / forecast / story-simple
```

通常評価または強制評価のローテーションは次の順序。

```text
single -> double -> external -> movie -> news -> forecast -> story-simple
```

| category | strategy | 内容 |
|---|---|---|
| `single` | `single` | 1ジャンルを具体アンカー付きで深掘りする |
| `double` | `double` | 2ジャンルの意外な共通構造を扱う |
| `external` | `external` | 外部素材とジャンルを自然に接続する |
| `movie` | `movie` | `「〜」ってどんな映画？` 形式の架空映画 |
| `news` | `news` | ニュース見出し1件の論点・背景・影響 |
| `forecast` | `forecast` | 未来展望の変化・分岐 |
| `story-simple` | `story-simple` | 昔話の骨格と置換主人公による軽量リメイク短編 |

禁止:

- `story` category を使わない。
- `story-simple` を `story` に正規化しない。
- `story-simple` を Story の fallback として扱わない。

## 2. お題品質の正本

| category | topic品質 |
|---|---|
| `single` | 人物・物・場所・場面など具体アンカーがある |
| `double` | 2領域の共通構造が見える |
| `external` | provider 名、取得経路、記事・検索結果などのメタ語を出さない |
| `movie` | 必ず `「〜」ってどんな映画？` 形式にする |
| `news` | ニュースの論点・背景・影響を扱い、ランダムジャンルを混ぜない |
| `forecast` | 対象領域と変化先が分かる未来の問いにする |
| `story-simple` | 元話、置換後の主人公、リメイク方向が分かる |

基準 topic は品質目安であり、固定出力してはいけない。

## 3. TTS / Viewer 契約

Story-Simple 以外の topic は、取得済み topic に `今日のお題。` を前置するだけの決定的処理とする。

- topic 本文を LLM で再生成、要約、言い換えしない。
- カテゴリ名、strategy、seed、provider 名を読み上げ本文へ入れない。
- Viewer / History / Summary の正本は変換前 topic とする。

story-simple は、元話と置換後の主人公を含む導入タイトルを生成してよい。ただし、Story の多段階生成、`story` category、`story` sessionMode、Story タイトル生成 prompt へ戻してはいけない。

## 4. データ型

```go
type TopicCategory string

const (
    TopicCategorySingle      TopicCategory = "single"
    TopicCategoryDouble      TopicCategory = "double"
    TopicCategoryExternal    TopicCategory = "external"
    TopicCategoryMovie       TopicCategory = "movie"
    TopicCategoryNews        TopicCategory = "news"
    TopicCategoryForecast    TopicCategory = "forecast"
    TopicCategoryStorySimple TopicCategory = "story-simple"
)
```

`NormalizeTopicCategory` は `story` を受け入れない。旧値として入ってきた場合は unsupported category として明示的に失敗させる。

```go
type TopicSeed struct {
    Category TopicCategory `json:"category"`

    // common
    GenreA string `json:"genre_a,omitempty"`
    GenreB string `json:"genre_b,omitempty"`

    // external
    ExternalTitle string `json:"external_title,omitempty"`
    ExternalText  string `json:"external_text,omitempty"`
    Provider      string `json:"provider,omitempty"`

    // news
    NewsTitle    string `json:"news_title,omitempty"`
    NewsCategory string `json:"news_category,omitempty"`
    NewsSource   string `json:"news_source,omitempty"`
    NewsURL      string `json:"news_url,omitempty"`

    // forecast
    ForecastDomain  string `json:"forecast_domain,omitempty"`
    ForecastKeyword string `json:"forecast_keyword,omitempty"`

    // story-simple
    TaleTitle   string `json:"tale_title,omitempty"`
    Protagonist string `json:"protagonist,omitempty"`
    Synopsis    string `json:"synopsis,omitempty"`
}
```

```go
type TopicGenerationResult struct {
    Topic      string        `json:"topic"`
    Category   TopicCategory `json:"category"`
    Strategy   string        `json:"strategy"`
    Seed       TopicSeed     `json:"seed"`
    Diagnostic string        `json:"diagnostic,omitempty"`
}
```

`Topic`, `Category`, `Strategy` は Viewer 表示、履歴、ログ、TTS event で追跡できること。

## 5. カテゴリ別生成仕様

### 5.1 Single

- 1ジャンルを選ぶ。
- 人物、物、場所、場面のうち少なくとも2つの具体アンカーを入れる。
- 抽象語だけの topic にしない。

### 5.2 Double

- 2ジャンルを選ぶ。
- 単なる並列ではなく、共通構造、対比、変換関係が見える題名にする。
- `A と B について` のような弱い topic を避ける。

### 5.3 External

- 外部素材1件とジャンルを組み合わせる。
- provider 名、取得経路、ランダム記事、検索結果などのメタ語を topic に出さない。
- ニュースを深掘りする場合は `news` を使う。

### 5.4 Movie

- 必ず `「〜」ってどんな映画？` の形にする。
- `movie` category / strategy として記録する。
- Single / Double / External の隠しフラグとして扱わない。

### 5.5 News

- news seed 1件から topic を生成する。
- ランダムジャンルや外部素材を混ぜない。
- seed 取得失敗時は `news_seed_unavailable` などで診断し、別カテゴリ成功扱いにしない。

### 5.6 Forecast

- domain と keyword をもとに、将来変化の問いを作る。
- 未来を断定せず、分岐や影響範囲が見える題名にする。
- 外部 Coder API は `idle_chat.forecast_external_enabled: true` が明示された場合のみ使う。

### 5.7 Story-Simple

- `simpleStoryTales` の元話と `protagonistOptions` の主人公候補を使う。
- 元話の骨格と置換後の主人公が topic から分かるようにする。
- `story` category、Story 多段階 pipeline、Story validator は使わない。

例:

```text
桃太郎の主人公を宅配業者に置き換えたら、鬼ヶ島への配送はどう変わるか
```

## 6. Interestingness Judge

Judge は topic 候補を新規生成しない。候補を採点し、採用理由と懸念だけを返す。

採点軸:

| 軸 | 点 | 内容 |
|---|---|---|
| concreteness | 0-25 | 具体的な像がある |
| category_fit | 0-25 | カテゴリ固有の勝ち筋に合う |
| conversation_potential | 0-25 | Mio / Shiro が会話を広げられる |
| freshness | 0-15 | 直近 topic と似すぎない |
| cleanliness | 0-10 | メタ語、prompt漏れ、説明過多がない |

## 7. 決定的バリデーション

共通:

- 空文字を禁止する。
- 長すぎる topic を禁止する。
- prompt / JSON / provider / seed など内部語を禁止する。
- 直近 topic との完全一致を禁止する。

カテゴリ別:

- `movie`: `「` と `」ってどんな映画？` を含む。
- `news`: `NewsTitle` が存在する。
- `external`: provider 名や取得経路を topic に出さない。
- `forecast`: domain または keyword が残る。
- `story-simple`: `TaleTitle` と `Protagonist` が分かる。

## 8. Error / Diagnostic

| code | 意味 |
|---|---|
| `unsupported_category` | 未対応カテゴリ。`story` が来た場合もこれに含める |
| `news_seed_unavailable` | news seed 不足 |
| `external_seed_unavailable` | external seed 不足 |
| `forecast_seed_unavailable` | forecast seed 不足 |
| `story_simple_seed_unavailable` | story-simple seed 不足 |
| `topic_validation_failed` | deterministic validation 失敗 |
| `topic_generation_failed` | LLM 生成失敗 |

## 9. テスト仕様

- `single -> double -> external -> movie -> news -> forecast -> story-simple` を1巡できる。
- `story` が category として使われない。
- `story-simple` が `story` に正規化されない。
- News seed 不足時に External へすり替わらない。
- Movie が `movie` category / strategy として記録される。
- Viewer / History / Summary / TTS event で topic/category/strategy が一致する。

## 10. 受け入れ条件

1. 正本・詳細仕様・実装・Viewer・E2E のカテゴリ一覧が `single / double / external / movie / news / forecast / story-simple` で一致する。
2. `story` category / sessionMode / 起動 API は仕様対象に残らない。
3. `story-simple` は独立モードとして起動・記録・表示できる。
4. 失敗時に別カテゴリ成功扱いにしない。
5. E2E で7カテゴリ1巡と topic/category/strategy 追跡を確認できる。
