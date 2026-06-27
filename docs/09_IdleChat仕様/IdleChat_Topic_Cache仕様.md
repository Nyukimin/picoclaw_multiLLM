# IdleChat Topic Cache 仕様

**作成日**: 2026-06-28
**対象**: IdleChat の `single / double / external / movie / news / forecast / story-simple` お題キャッシュ
**親仕様**: `IdleChat仕様.md`, `IdleChat前準備仕様.md`, `IdleChat_Topic_Generator_Judge詳細仕様.md`

## 1. 目的

IdleChat は、session 開始時に topic 生成待ちで止まらないよう、全カテゴリのお題を事前にキャッシュする。
キャッシュは、ユーザー操作・通常 Chat・Worker 処理を邪魔しない範囲でバックグラウンド補充する。

## 2. 対象カテゴリと目標数

対象カテゴリは次の7つ。

```text
single / double / external / movie / news / forecast / story-simple
```

キャッシュ目標数は次の通り。

| category | cache target | 単位 |
|---|---:|---|
| `single` | 10 | category |
| `double` | 10 | category |
| `external` | 10 | category |
| `movie` | 10 | category |
| `news` | 10 | category |
| `forecast` | 10 | forecast domain |
| `story-simple` | 10 | category |

`forecast` は6ドメインを持つため、各ドメインごとに10件を目標とする。

## 3. 補充タイミング

キャッシュ補充は次のタイミングで行う。

- IdleChat 起動後
- cache から topic を1件 pop した直後
- inline topic 生成に成功した直後
- forecast domain の cache が目標数を下回った時

補充は session 開始処理をブロックしない。
cache が空の場合のみ、既存の inline topic 生成に fallback してよい。

## 4. 非干渉条件

キャッシュ補充は、次の状態では LLM 生成を開始しない。

- `chatActive == true`
- `chatBusy == true`
- `workerBusy == true`

この状態では待機し、IdleChat 本体、通常 Chat、Worker 処理を優先する。

また、複数カテゴリの補充が同時に発火しても、LLM を使う topic 補充は原則1本ずつ実行する。
これにより、バックグラウンド補充が Worker / ChatWorker を詰まらせないようにする。

## 5. カテゴリ別キャッシュ内容

### 5.1 single / double / external / movie / news

通常5カテゴリは `TopicGenerationResult` を単位としてキャッシュする。

保持対象:

- `topic`
- `category`
- `strategy`
- `seed`
- `interestingness_axis`
- `opening_hook`
- `avoid`
- `candidates`
- `judge`
- `provider`
- `context_terms`（`single` / `double` / `external` / `news` のみ任意）

`single` / `double` / `external` / `news` は `IdleChat_Topic_Context_Memo仕様.md` に従い、必要に応じて `context_terms` も保持する。
`movie` は架空映画設定の性質が強いため、本仕様では `context_terms` の対象外とする。

### 5.2 forecast

forecast は `ForecastDomain` ごとに `PreparedTopic` をキャッシュする。

保持対象:

- `domain`
- `topic`
- `seeds`
- `created`
- `context_terms`

`IdleChat_Topic_Context_Memo仕様.md` に従い、関連語句・語句の意味を `TopicGenerationResult` 相当の文脈情報として保持する。

### 5.3 story-simple

story-simple は、LLM生成前の「元話 + 置換主人公 + topic result」をキャッシュする。

保持対象:

- `tale_title`
- `synopsis`
- `protagonist`
- `topic`
- `category = story-simple`
- `strategy = story-simple`

story-simple は `story` の fallback ではない。
`story` category / sessionMode / pipeline に戻してはいけない。

## 6. pop / refill の契約

cache から topic を使う時は、先頭から1件 pop する。
pop した topic と同一または重複相当の topic が残っている場合は、同時に取り除いてよい。

pop 後、残数が目標数未満なら補充を予約する。
補充はバックグラウンドで行い、session 進行を止めない。

## 7. 重複排除

cache へ push する時は、既存 topic と重複する topic を入れない。

重複判定は、表示 topic を正規化したキーで行う。
完全一致または明らかに同一 topic と見なせるものは破棄する。

## 8. 失敗時動作

### 8.1 cache 補充失敗

cache 補充に失敗しても IdleChat 本体を停止しない。
ログに失敗理由を残し、次回の補充機会で再試行する。

### 8.2 cache 空

cache が空の場合は、該当カテゴリの既存 inline topic 生成へ fallback してよい。
ただし、別カテゴリへすり替えて成功扱いにしてはいけない。

例:

- `news` seed がない場合、`external` として成功扱いにしない。
- `story-simple` seed がない場合、`story` へ戻さない。
- `forecast` provider 失敗時に別外部 provider へ自動切替しない。

## 9. Viewer / TTS 境界

cache の内部情報は Viewer / TTS にそのまま出さない。
Viewer / TTS に出る topic は、従来通り表示用 topic と読み上げ用 speech topic の契約に従う。

- 通常カテゴリ / forecast: `今日のお題。<topic>` を TTS 専用に生成する。
- story-simple: 専用の導入発話・本文発話として扱う。
- seed / provider / candidates / judge / context terms は Viewer topic や TTS topic に混入させない。

## 10. 実装チェックリスト

- [ ] `single / double / external / movie / news` が各10件 target の cache を持つ。
- [ ] `forecast` が各 domain 10件 target の cache を持つ。
- [ ] `story-simple` が10件 target の cache を持つ。
- [ ] pop 後に補充が予約される。
- [ ] `chatActive / chatBusy / workerBusy` 中は補充生成を開始しない。
- [ ] LLM を使う補充が同時多発しない。
- [ ] cache 空時は同カテゴリ inline 生成に fallback する。
- [ ] 別カテゴリへのすり替え成功扱いが起きない。
- [ ] Viewer / TTS に cache 内部情報が漏れない。
