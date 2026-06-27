# IdleChat 前準備仕様

**作成日**: 2026-06-27
**対象**: IdleChat の topic 生成前、session 開始前、各カテゴリ固有 seed 準備
**位置づけ**: `IdleChat仕様.md`、`IdleChat_Topic_Generator_Judge詳細仕様.md`、`IdleChat仕様_再精査まとめ.md` の前段処理を明文化する横断仕様。

## 1. 目的

IdleChat は、各カテゴリの topic を生成する前に、共通の session 準備とカテゴリ別 seed 準備を行う。

この前準備の目的は次の通り。

- category / strategy / session_id を確定する。
- topic 生成に必要な seed を不足なく用意する。
- 直近履歴と重複しない topic を作れる状態にする。
- Viewer / TTS / STT / interrupt の境界を開始前に整える。
- 失敗時に別カテゴリ成功へすり替えない。

## 2. 共通前準備

全カテゴリで、topic 生成前に次を行う。

| 項目 | 内容 |
|---|---|
| session 採番 | `session_id` を採番する。通常 IdleChat は 1 topic につき 1 session。 |
| category 決定 | `single / double / external / movie / news / forecast / story-simple` のいずれかを確定する。 |
| strategy 決定 | category に対応する strategy を確定する。原則 category と同名。 |
| 直近履歴取得 | 直近 topic / summary / category / strategy を読む。 |
| 重複抑制 | 完全一致と近すぎる topic を避ける。 |
| LLM alias 選択 | Mio / Shiro / Forecast など用途に応じた alias を選ぶ。 |
| context 準備 | interrupt 可能な context を作る。 |
| Viewer 状態確認 | active session、active audio viewer、active input viewer を確認する。 |
| TTS 状態確認 | 旧 session の TTS pending / playback ack が混入しない状態にする。 |
| STT 境界確認 | STT 入力は通常 Chat 側で扱い、IdleChat へ直接流さない。 |

開始前の禁止事項:

- seed 不足を別カテゴリ成功として扱わない。
- 古い session の topic / TTS / STT event を現在 session に混ぜない。
- `story` category / sessionMode を使わない。
- `story-simple` を `story` に正規化しない。

## 3. single の前準備

single は、1つのジャンルや題材を深掘りするカテゴリ。

前準備:

1. ジャンルプールから1件選ぶ。
2. 直近 topic と重複しないか確認する。
3. 人物・物・場所・場面など、具体アンカーを足せる状態にする。
4. 抽象語だけの topic にならないよう、seed に具体要素を持たせる。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `genre_a` | 主題ジャンル |
| `anchor_person` | 任意。人物アンカー |
| `anchor_object` | 任意。物アンカー |
| `anchor_place` | 任意。場所アンカー |
| `anchor_scene` | 任意。場面アンカー |

失敗時:

- ジャンルが取れない場合は `single_seed_unavailable` として診断する。
- double / external などへ黙って切り替えない。

## 4. double の前準備

double は、2つの離れたジャンルを掛け合わせるカテゴリ。

前準備:

1. ジャンルを2件選ぶ。
2. 近すぎる組み合わせを避ける。
3. 遠すぎて接続不能な組み合わせを避ける。
4. 共通構造、対比、変換関係が作れるか確認する。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `genre_a` | 1つ目のジャンル |
| `genre_b` | 2つ目のジャンル |
| `connection_axis` | 任意。共通構造や接続軸 |

失敗時:

- 2ジャンルが揃わない場合は `double_seed_unavailable` とする。
- single として成功したように扱わない。

## 5. external の前準備

external は、外部素材とジャンルを組み合わせるカテゴリ。

前準備:

1. Wikipedia Random などから外部素材を1件取得する。
2. provider 名、取得経路、URL などは内部メタとして保持する。
3. topic 本文には provider 名や取得経路を出さない。
4. 外部素材とジャンルを自然に接続できるか確認する。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `external_title` | 外部素材のタイトル |
| `external_text` | 外部素材の要約または抜粋 |
| `provider` | 内部メタ。Viewer topic へ出さない |
| `genre_a` | 接続するジャンル |

失敗時:

- 外部素材が取れない場合は `external_seed_unavailable` とする。
- News と混同しない。
- provider 名を表示 topic に出して成功扱いしない。

## 6. movie の前準備

movie は、架空映画 topic を作るカテゴリ。

前準備:

1. category / strategy を `movie` に固定する。
2. `「〜」ってどんな映画？` の形式を守る。
3. 実在映画紹介に寄らないようにする。
4. タイトルから会話が広がる余地を作る。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `movie_title_seed` | 架空映画タイトルの核 |
| `genre_hint` | 任意。映画ジャンルや雰囲気 |
| `visual_anchor` | 任意。映像的な手がかり |

失敗時:

- 形式を満たさない topic は validation error とする。
- `movie=true` のような隠し属性だけで movie 扱いしない。

## 7. news の前準備

news は、ニュース見出し1件を深掘りするカテゴリ。

前準備:

1. RSS などからニュース seed を1件取得する。
2. `title / category / source / url` を保持する。
3. ランダムジャンルや external 素材を混ぜない。
4. ニュース本文の紹介ではなく、論点・背景・影響へ展開できるか確認する。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `news_title` | ニュース見出し |
| `news_category` | ニュース分類 |
| `news_source` | 取得元 |
| `news_url` | URL |

失敗時:

- seed が取れない場合は `news_seed_unavailable` とする。
- external へ黙ってすり替えない。
- random genre と混ぜて成功扱いしない。

## 8. forecast の前準備

forecast は、未来展望セッションのカテゴリ。

前準備:

1. 対象ドメインを決める。
2. トレンドを収集する。
3. キーワードを抽出する。
4. ニュースを深掘りする。
5. 既出テーマと重複しないか確認する。
6. Forecast provider を選ぶ。
7. 外部 provider 使用可否を確認する。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `forecast_domain` | AI技術、経済などの対象ドメイン |
| `forecast_keyword` | 抽出キーワード |
| `trend_items` | トレンド候補 |
| `news_items` | 深掘り用ニュース |
| `recent_forecast_themes` | 既出テーマ |

外部 provider:

- 既定では外部 Coder API を使わない。
- `idle_chat.forecast_external_enabled: true` が明示された場合のみ使ってよい。
- 失敗時に別の外部 provider へ自動切替しない。

失敗時:

- trend / news が不足した場合は診断を残す。
- 外部 provider 失敗を別 provider 成功へ黙ってすり替えない。

## 9. story-simple の前準備

story-simple は、昔話の骨格を使い、主人公を別存在に置き換える軽量リメイク短編。

前準備:

1. `simpleStoryTales` から元話を1件選ぶ。
2. `protagonistOptions` から置換主人公を1件選ぶ。
3. 元話の事件、解決、オチの骨格を保持する。
4. 主人公が変わることで、世界設定、社会常識、登場人物の反応が変わるようにする。
5. category / strategy / sessionMode を `story-simple` に固定する。

生成前に必要な seed:

| seed | 内容 |
|---|---|
| `tale_title` | 元話タイトル |
| `synopsis` | 元話の短いあらすじ |
| `protagonist` | 置換後の主人公 |
| `rewrite_hint` | 任意。変化の方向 |

禁止:

- `story` category に正規化しない。
- `story` sessionMode を使わない。
- Story の多段階 pipeline / validator / 起動 API を使わない。
- Story の fallback として扱わない。

失敗時:

- 元話または主人公が選べない場合は `story_simple_seed_unavailable` とする。
- Story へ fallback しない。

## 10. 前準備の完了条件

前準備は次を満たした時点で完了とする。

- category / strategy / session_id が確定している。
- category に必要な seed が揃っている。
- seed 不足時の diagnostic が定義されている。
- 直近履歴との重複確認が済んでいる。
- Viewer / TTS / STT の active 境界が確認済み。
- interrupt 可能な context が準備されている。
- `story` ではなく `story-simple` として扱うべき箇所が固定されている。

## 11. 実装照合チェックリスト

- [ ] `prepareTopicSeed(category)` 相当の責務が存在する。
- [ ] seed 不足時に別カテゴリ成功へすり替えない。
- [ ] `news_seed_unavailable` などの診断がログに残る。
- [ ] External の provider 名が Viewer topic に出ない。
- [ ] Forecast の外部 provider 使用が明示設定に従う。
- [ ] story-simple の seed が `story` に正規化されない。
- [ ] session 開始前に active Viewer / TTS / STT 境界が確認される。
- [ ] interrupt 後の古い seed / topic / TTS event が次 session に混入しない。
