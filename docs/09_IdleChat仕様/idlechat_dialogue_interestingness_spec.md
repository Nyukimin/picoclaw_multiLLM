# IdleChat 対話面白さ向上仕様

## 0. 目的

本仕様は、IdleChat において生成済み topic を受け取り、その後のエージェント同士の対話内容をカテゴリ別に面白く展開するための仕様である。

前段の topic generator / topic judge は「面白い入口」を作る。本仕様は、その入口から実際の対話を自然に深めるための、対話演出、発話生成、品質判定、ログ、テストを定義する。

Story は削除済み。story-simple は Story の代替として実装済みの独立カテゴリであり、Story とは別物として扱う。

対象カテゴリ:

```text
single / double / external / movie / news / forecast / story-simple
```

## 1. 既存 IdleChat との接続

通常 IdleChat は次の境界を守る。

```text
1 session = 1 topic = 1 summary
```

本仕様は `generateResponse()` の品質をカテゴリ別に高めるものであり、topic 生成や session 境界を変更しない。

変更対象の責務:

| 責務 | 内容 |
|---|---|
| DialogueDirector | カテゴリ別の対話方針とターン設計 |
| DialoguePrompt | 発話生成 prompt の組み立て |
| DialogueQualityChecker | 発話品質判定と retry reason |
| DialogueState | 対話アークの進行状態 |

## 2. 基本設計

topic と dialogue の責務を分ける。

| 種別 | 責務 |
|---|---|
| topic | Viewer / History / Summary / TTS の正本となる短い題名 |
| dialogue | topic を受けて、カテゴリ固有の面白さを会話内で展開する |

Mio / Shiro の人格そのものは persona 側に置く。本仕様では IdleChat 内のカテゴリ別役割オーバーレイだけを定義する。

発話本文に出してはいけないもの:

- category
- strategy
- prompt
- seed
- provider
- JSON
- internal score
- retry reason

## 3. 共通の発話品質仕様

1発話の制約:

- 1発話は短めにする。
- 直前発話を受ける。
- ユーザーへ直接質問しない。
- topic / category / prompt / seed / provider などの内部メタを出さない。
- 同じ構文や同じ結論を繰り返さない。
- 「面白いですね」「興味深いですね」だけで終わらない。

直前発話の受け方:

- 相手の具体語を1つ拾う。
- その語を少しずらす、深める、反例を出す、具体場面へ落とす。
- ただし相手の発話を要約するだけで終わらない。

## 4. カテゴリ別仕様

### 4.1 Single

面白さの核:

1つの題材を狭く深く観察する。大きな一般論ではなく、細部の違和感、使われ方、置かれた場面を掘る。

必須ムーブ:

- 具体物や場面を出す。
- 抽象語を1段具体に落とす。
- 最後に小さな発見を残す。

禁止:

- ジャンル紹介だけで終わる。
- あるあるの羅列にする。

### 4.2 Double

面白さの核:

遠い2領域の間に共通構造を見つける。

必須ムーブ:

- A と B の表面的な違いを一度認める。
- 共通する構造、制約、遅延、交換、保存、儀式などを見つける。
- 片方の語彙をもう片方へ移植する。

禁止:

- 「意外な組み合わせですね」で止まる。
- A と B を別々に説明するだけで終わる。

### 4.3 External

面白さの核:

外から来た素材を会話の中で自然化する。

必須ムーブ:

- 素材の固有性を拾う。
- provider 名や取得経路を出さず、題材として扱う。
- ジャンル側と自然に接続する。

禁止:

- Wikipedia、外部刺激、ランダム記事などのメタ語を出す。
- 素材を紹介して終わる。

### 4.4 Movie

面白さの核:

存在しない映画を会話の中で少しずつ立ち上げる。

必須ムーブ:

- タイトルからジャンル、画、主人公、葛藤を想像する。
- 映像的な場面を1つ出す。
- 会話によって架空映画の輪郭を増やす。

禁止:

- あらすじを一気に全部説明する。
- 実在映画の紹介に寄せすぎる。

### 4.5 News

面白さの核:

ニュースを紹介するのではなく、論点、背景、影響、判断の難しさを短い対話で立体化する。

必須ムーブ:

- 何が変わるのかを具体化する。
- 誰に影響が出るかを考える。
- 単純な賛否で終わらせない。

禁止:

- ニュース本文の読み上げだけにする。
- ランダムジャンルや外部素材と混ぜる。

### 4.6 Forecast

面白さの核:

現在の兆しから、何が変わり、誰に影響し、どんな分岐があり得るかを考える。

必須ムーブ:

- 兆しを1つ挙げる。
- 変化先を複数に分岐させる。
- 3年後、5年後、10年後など時間軸を意識する。

禁止:

- 未来を断定する。
- ただの技術紹介やニュース紹介にする。

### 4.7 Story-Simple

面白さの核:

昔話の骨格を保ちつつ、主人公が別の存在になったことで世界の常識や登場人物の反応がどう変わるかを楽しむ。

必須ムーブ:

- 元話の骨格を認識できるようにする。
- 置換後の主人公らしさを強く出す。
- 事件 -> 解決 -> オチのテンポを保つ。
- 大げさで軽いエンタメに寄せる。

禁止:

- Story の多段階 pipeline、story category、story sessionMode に戻す。
- 元話の全文再話だけで終わる。
- 解説やメタ発言を本文に混ぜる。

## 5. DialogueQualityChecker

共通チェック:

| reason | 内容 |
|---|---|
| `dialogue_no_uptake` | 直前発話を受けていない |
| `dialogue_too_generic` | 抽象的すぎる |
| `dialogue_internal_meta_leak` | 内部メタが出ている |
| `dialogue_category_axis_missing` | カテゴリ固有の面白さがない |
| `dialogue_repetition` | 同じ構文・同じ結論の反復 |

カテゴリ別最低条件:

| category | 最低条件 |
|---|---|
| `single` | 細部や具体場面がある |
| `double` | 2領域の共通構造がある |
| `external` | 外部素材が自然化されている |
| `movie` | 映像・主人公・葛藤のいずれかが増えている |
| `news` | 影響・背景・判断の難しさがある |
| `forecast` | 未来の分岐がある |
| `story-simple` | 元話骨格と置換主人公の効果がある |

## 6. Summary 連携

summary はカテゴリ別に着地点を変える。

| category | summary の着地点 |
|---|---|
| `single` | 具体観察から得た小さな発見 |
| `double` | 2領域に共通していた構造 |
| `external` | 外部素材が会話内でどう意味化されたか |
| `movie` | 架空映画として立ち上がった核 |
| `news` | 論点、影響、判断の難しさ |
| `forecast` | 未来の分岐と継続考察テーマ |
| `story-simple` | 置換主人公によって変わった元話の面白さ |

## 7. テスト仕様

共通:

- 内部メタが発話本文に出ない。
- 直前発話を受ける。
- category ごとの品質軸が反映される。
- retry reason がログに残る。

カテゴリ別:

- `single`: 細部観察がある。
- `double`: 共通構造がある。
- `external`: provider 名が出ない。
- `movie`: 架空映画の輪郭が増える。
- `news`: ニュース紹介だけで終わらない。
- `forecast`: 未来の分岐がある。
- `story-simple`: 元話骨格と置換主人公が両方残る。

E2E:

- `single -> double -> external -> movie -> news -> forecast -> story-simple` を1巡する。
- Viewer / TTS / History / Summary の topic/category/strategy 追跡を壊さない。
- `story` category / sessionMode が混入しない。

## 8. 受け入れ条件

1. 7カテゴリの対話品質ログが確認できる。
2. Story は削除され、`story` category / sessionMode を使わない。
3. story-simple は独立カテゴリとして発話品質チェックと summary に反映される。
4. 内部メタや prompt 断片が発話本文に出ない。
5. Viewer / TTS / History / Summary の追跡情報が一致する。
