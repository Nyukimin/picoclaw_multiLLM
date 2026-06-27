# story-simple 実装仕様 v1.0

**対象**: IdleChat story-simple モード
**主な実装箇所**: `internal/application/idlechat/story_mode_simple.go`
**位置づけ**: Story の代替として実装済みの軽量物語リメイク機能。Story とは別物。

## 1. 概要

story-simple は、昔話の骨格を使い、主人公を別の存在に置き換えて短編としてリメイクする IdleChat の独立モードである。

Story は、多段階パイプラインと専用コーパスを前提にしたが、構築中に非現実的であることが分かったため仕様対象から削除する。story-simple は Story のフォールバックではなく、Story の代わりに採用する実装済みモードである。

## 2. Story との違い

| 項目 | Story | story-simple |
|---|---|---|
| 位置づけ | 削除済み | 採用 |
| パイプライン | 多段階 | ワンショット |
| データソース | `data/story/*.json` 前提 | コード内インライン |
| 生成単位 | 物語コーパスの改変 | 昔話 + 主人公置換 |
| sessionMode | `story` | `story-simple` |
| 扱い | 使用しない | 独立モード |

禁止:

- story-simple を `story` category に正規化しない。
- story-simple を Story の fallback として扱わない。
- `story` sessionMode / 起動 API / validator を復活させない。

## 3. データソース

story-simple はコード内で直接定義された `simpleStoryTales` を使う。JSON ファイルには依存しない。

対象となる昔話:

| タイトル | あらすじ |
|---|---|
| 桃太郎 | 川から桃が流れてきて生まれた子が、犬・猿・雉を仲間に鬼ヶ島へ鬼退治に行く |
| 一寸法師 | 親指ほどの小さな武士が針を刀に都へ上り、鬼を倒して打ち出の小槌で大きくなる |
| 浦島太郎 | 亀を助けた漁師が竜宮城へ招かれ、帰ると何百年も経っていて老人になる |
| かぐや姫 | 竹から生まれた娘が貴族たちの求婚を難題で退け、月へ帰っていく |
| 鶴の恩返し | 助けた鶴が娘に化けて機を織るが、見ることを禁じられた部屋を覗かれて去る |
| 舌切り雀 | 親切な翁が舌を切られた雀を助け、意地悪な婆が欲張って痛い目に遭う |
| 花咲かじいさん | 犬の教えで金を掘り当てた翁が、灰で枯れ木に花を咲かせて殿様に褒められる |
| さるかに合戦 | 猿に騙されたカニの子が栗・蜂・臼と協力して仇を討つ |
| 笠地蔵 | 雪の中の地蔵に笠をかぶせた翁夫婦の元へ、夜中に宝物が届く |
| 金太郎 | 山で熊と相撲を取って育った怪力の子が、坂田金時として武将に仕える |

主人公候補:

`AIロボット` / `サラリーマン` / `宅配業者` / `YouTuber` / `コンビニ店員` /
`定年退職したおじいさん` / `高校生` / `宇宙人` / `忍者` / `猫` /
`ドラゴン` / `魔法使い見習い` / `探偵` / `料理人`

## 4. LLM 設定

| パラメータ | 値 |
|---|---|
| モデル | `forecastLLM()` |
| MaxTokens | 2500 |
| Temperature | 0.9 |

## 5. プロンプト構造

```text
system:
あなたは昔話リメイク作家です。ユーザーの指示に従って、笑えるくらい大袈裟で面白い短編を書いてください。

user:
昔話「<タイトル>」を、主人公を「<主人公>」に置き換えてリメイクしてください。
元の話のあらすじ: <synopsis>

条件:
- 主人公が「<主人公>」になったことで、世界設定・社会の常識・登場人物の反応もすべて大胆に変わる
- 元の話の骨格（事件 -> 解決 -> オチ）は残す
- テンポよく、会話と描写を交えて
- 大げさなくらい面白く仕上げる
- 2000文字前後
- タイトルは1行目に「【タイトル】」形式で書く
- 本文のみ出力し、解説・メタ発言を出さない
```

## 6. 出力フォーマット

```text
今夜の物語です。『<元タイトル>』を、主人公を<主人公>に置き換えたら——
改題は『<生成タイトル>』。
<本文>
『<元タイトル>』を下敷きにした、主人公<主人公>のお話でした。
```

本文は Viewer 表示と TTS 読み上げに使うため、段落単位で扱えるように整形する。

## 7. 起動方法

```go
func (o *IdleChatOrchestrator) StartSimpleStoryMode() error
func (o *IdleChatOrchestrator) RunSimpleStorySession()
```

`StartSimpleStoryMode()` は参加者が1名以上いれば起動できる。

## 8. category / strategy

| 項目 | 値 |
|---|---|
| category | `story-simple` |
| strategy | `story-simple` |
| sessionMode | `story-simple` |

`story` は使わない。

## 9. 検証観点

- story-simple が `story` に正規化されない。
- story-simple が Story の fallback として扱われない。
- `story` sessionMode / API / validator が残っていない。
- Viewer / History / Summary / TTS event で `story-simple` として追跡できる。
- `single -> double -> external -> movie -> news -> forecast -> story-simple` の E2E ローテーションに含められる。
