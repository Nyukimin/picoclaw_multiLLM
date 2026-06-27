# IdleChat Movie DB Context Memo 仕様

**作成日**: 2026-06-28
**対象**: IdleChat `movie` カテゴリ、自前 Movie DB、映画・俳優・監督等の関連メモ
**親仕様**: `IdleChat仕様.md`, `IdleChat_Topic_Context_Memo仕様.md`, `IdleChat_Topic_Cache仕様.md`

## 1. 目的

`movie` カテゴリは架空映画のお題を扱う。
ただし、会話を完全な思いつきだけにすると、映像、人物、監督性、俳優の質感が薄くなりやすい。

そのため、`movie` では自前の Movie DB から関連する映画、俳優、監督、スタッフ等の情報を引き、会話生成用の内部メモとして提供する。

このメモは、架空映画を実在映画として扱うためのものではない。
既存作品や人物の情報を、人物造形、演出、場面、葛藤、余韻の参考にするための補助情報である。

## 2. 基本方針

- Movie DB は自前DBを使う。
- DBが無い、検索できない、関連情報が無い場合でも `movie` topic は有効なまま扱う。
- メモは `TopicGenerationResult.ContextTerms` に格納する。
- Viewer / TTS / topic display にはメモを直接出さない。
- 会話生成時の `sessionContext` にだけ内部補助として注入する。
- 発話本文では、DB名、取得経路、URL、内部メタを出さない。
- 架空映画のお題を、実在作品として断定しない。

## 3. DB検索仕様

### 3.1 DBパス解決

Movie DB は次の順で解決する。

1. `PICOCLAW_MOVIE_CATALOG_DB`
2. 明示設定された DB path
3. `tmp/eiga_catalog/eiga_catalog.sqlite`
4. `tmp/eiga_catalog_smoke/eiga_catalog.sqlite`

DBが見つからない場合は soft unavailable とし、エラーで IdleChat を止めない。

### 3.2 使用テーブル

主に次のテーブルを使う。

| table | 用途 |
|---|---|
| `movies` | 映画タイトル、URL、あらすじ |
| `people` | 人物名、プロフィール、略歴 |
| `movie_people` | 映画と人物の関係、役割、情報源 |

`movie_people` は、俳優、監督、スタッフ、人物フィルモグラフィの接続情報として扱う。

### 3.3 検索入力

検索入力は次を使う。

- `TopicGenerationResult.Topic`
- `TopicGenerationResult.Seed.Genre1`

`「〜」ってどんな映画？` 形式の topic から、内部タイトル部分を取り出して検索語を作る。
助詞や記号を落とし、2文字以上の語をキーワードとして使う。

### 3.4 検索対象

映画検索:

- `movies.title`
- `movies.synopsis`

人物検索:

- `people.name`
- `people.biography`
- `movie_people.person_name`
- `movie_people.movie_title`
- `movie_people.role`

検索は部分一致でよい。
結果は、映画・人物を合わせて最大6件程度を目安にする。

## 4. メモ形式

Movie DB から得た情報は、`ContextTerms` として保持する。

```go
TopicContextTerm{
    Term:      "...",
    Meaning:   "...",
    Relevance: "...",
    Source:    "movie_catalog:movie" // or "movie_catalog:person"
}
```

映画メモ:

- `term`: 映画タイトル
- `meaning`: DB上の作品情報と短いあらすじ
- `relevance`: 架空映画の場面、葛藤、質感の参考であること
- `source`: `movie_catalog:movie`

人物メモ:

- `term`: 人物名
- `meaning`: DB上の人物情報、主な役割、関連作
- `relevance`: 俳優、監督、スタッフの役割や過去作の空気を人物造形・演出の参考にすること
- `source`: `movie_catalog:person`

## 5. sessionContext への注入

`formatTopicGenerationContext()` は、`ContextTerms` を `【関連メモ】` として内部文脈に整形する。

注入例:

```text
【関連メモ】
- マージン・コール: 映画DBにある作品。あらすじ: ... / お題との関係: ...
- ケビン・スペイシー: 映画DBにある映画関係者。主な役割: 出演。関連作: ... / お題との関係: ...

注意:
- このメモは会話の参考にだけ使う。
- 出典名、DB名、内部メタを発話本文へ出さない。
- 架空映画を実在作として扱わない。
```

## 6. キャッシュとの関係

`movie` topic cache は `TopicGenerationResult` を保持する。
DB検索が成功した場合は `context_terms` を同梱してよい。

cache refill 時にメモ生成を試みる。
既存の cache item に `context_terms` が無い場合は、pop 時に補完してよい。

## 7. 失敗時動作

次の場合でも、topic 自体は利用可能とする。

- Movie DB が無い
- 必要テーブルが無い
- 検索結果が0件
- DB検索中にエラーが出た

エラー時はログに診断を残し、別カテゴリへすり替えない。

## 8. 実装チェックリスト

- [x] `TopicGenerationResult.ContextTerms` を実装する。
- [x] Movie DB の read-only lookup を実装する。
- [x] `PICOCLAW_MOVIE_CATALOG_DB` と既定DBパスを解決する。
- [x] `movie` topic 生成後に Movie DB memo を付与する。
- [x] stock refill 時に `context_terms` を同梱する。
- [x] stock pop 時に `context_terms` が無い場合も補完できる。
- [x] `formatTopicGenerationContext()` が `ContextTerms` を内部補助として注入する。
- [x] Viewer / TTS の topic 表示へメモを混入させない。
- [x] DB無しは soft unavailable として扱う。
