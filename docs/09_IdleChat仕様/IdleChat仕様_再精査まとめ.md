# IdleChat 仕様 再精査まとめ

**作成日**: 2026-06-27
**位置づけ**: 復活した IdleChat 仕様群を再精査し、正本化・実装照合の入口として整理するまとめ。

## 参照元

- `docs/01_正本仕様/03_IdleChat.md`
- `docs/09_IdleChat仕様/IdleChat仕様.md`
- `docs/09_IdleChat仕様/06_IdleChat仕様.md`
- `docs/09_IdleChat仕様/IdleChat_Topic_Generator_Judge詳細仕様.md`
- `docs/09_IdleChat仕様/IdleChat前準備仕様.md`
- `docs/09_IdleChat仕様/idlechat_dialogue_interestingness_spec.md`
- `docs/09_IdleChat仕様/IdleChat即時停止仕様.md`
- `docs/09_IdleChat仕様/会話ID仕様.md`
- `docs/09_IdleChat仕様/未来展望セッション仕様.md`
- `docs/09_IdleChat仕様/実装仕様_story-simple_v1.md`

## 1. IdleChatの目的

IdleChat は、ユーザーが一定時間操作しないアイドル時間に、Mio / Shiro などのエージェント同士が自律的に会話する機能である。

目的は次の3つ。

- アイドル時間を使ってエージェントの人格を表現する。
- 雑談、架空映画、ニュース深掘り、未来展望、story-simple の軽量リメイク短編など、ユーザーが眺めて楽しめるコンテンツを自動生成する。
- Viewer / TTS を通じてリアルタイム表示と読み上げを行う。

ただし、IdleChat は通常 Chat / STT / ユーザー操作より優先されない。ユーザー介入が始まった瞬間に、IdleChat は停止または無効化される。

## 2. モードとカテゴリ

IdleChat には大きく次のモードがある。

| 種別 | 内容 | 起動 |
|---|---|---|
| 通常 IdleChat | 雑談系トピックを自動生成し、Mio / Shiro が会話する | アイドル検知 / 手動 |
| Forecast | 6ドメイン固定順で未来展望セッションを行う | 手動起動が基本 |
| story-simple | 昔話の骨格を使い、主人公を別存在に置き換える軽量リメイク短編 | 自動または手動 |

注意点:

- 通常雑談カテゴリは主に `single / double / external / movie / news`。
- IdleChat 全体の評価カテゴリは `forecast / story-simple` も含めた7カテゴリ。
- Story は削除。story-simple は Story の代替として実装済みの独立モードであり、Story とは別物。

## 3. 7カテゴリの正本

IdleChat のカテゴリ正本は次の7つ。

```text
single / double / external / movie / news / forecast / story-simple
```

通常評価または強制評価のローテーションは次の順序。

```text
single -> double -> external -> movie -> news -> forecast -> story-simple
```

| カテゴリ | strategy / mode | 内容 | 注意 |
|---|---|---|---|
| `single` | `single` | 1つのジャンルを深掘りする | 人物・物・場所・場面など具体アンカーを入れる |
| `double` | `double` | 2ジャンルの意外な掛け合わせを深掘りする | 共通構造が見える題名にする |
| `external` | `external` | Wikipedia Random など外部刺激とジャンルを組み合わせる | provider 名や取得経路を本文に出さない |
| `movie` | `movie` | 「〜ってどんな映画？」形式の架空映画を深掘りする | 隠し属性ではなく明示カテゴリにする |
| `news` | `news` | ニュース見出し1件の論点・背景・影響を深掘りする | ランダムジャンルや external と混ぜない |
| `forecast` | `forecast` | 未来展望セッション | 6ドメイン固定順 |
| `story-simple` | `story-simple` | 昔話の骨格と置換主人公による軽量リメイク短編 | Story とは別物 |

正当性条件:

- 正本、詳細仕様、実装、Viewer、履歴、ログ、TTS event、E2E でカテゴリ一覧が一致する。
- `single -> double -> external -> movie -> news -> forecast -> story-simple` を最低1巡できる。
- topic / category / strategy が Viewer 表示、履歴、ログ、TTS event で追跡できる。
- seed 取得失敗、生成失敗、カテゴリ未対応を別カテゴリ成功として扱わない。
- `story` category / sessionMode / 起動 API に戻さない。

## 4. session / topic / summary 境界

通常 IdleChat の境界は次の通り。

```text
1 session = 1 topic = 1 summary
```

話題を切り替える場合は、前の topic の session を完了し、summary と topicBreak を終えてから、新しい session として次 topic を開始する。

禁止事項:

- 同一 `session_id` 内で複数 topic を扱う。
- `topic-00` から `topic-01` へ同じ session のまま進める。
- summary なしで topic を切り替えたように見せる。
- 古い session の event を現在の Viewer 表示へ混ぜる。

## 5. topic生成とjudge

topic 生成の前には、`IdleChat前準備仕様.md` に従って共通前準備とカテゴリ別 seed 準備を完了しておく。

topic 生成は「カテゴリ別 seed / prompt / candidate / judge / validation」の流れで考える。

基本フロー:

1. category を決める。
2. category に必要な seed を取得する。
3. category 別 prompt で topic candidate を生成する。
4. deterministic validation を通す。
5. Interestingness Judge で採点する。
6. topic / category / strategy / seed / diagnostic を記録する。

カテゴリ別の要点:

- `news` は news seed が必須。取得失敗時は `news_seed_unavailable` などで診断し、成功扱いにしない。
- `external` は素材とジャンルを自然に接続する。取得経路や provider 名は本文に出さない。
- `movie` は必ず `「〜」ってどんな映画？` 形式を守る。
- `forecast` は将来変化の問いとして、対象領域と変化先が分かる topic にする。
- `story-simple` は元話と置換後の主人公を含む導入タイトルを扱う。

お題読み上げでは、通常カテゴリと Forecast は既存 topic に `今日のお題。` を前置するだけにする。LLM による再生成、要約、言い換えは禁止する。
story-simple は独立モードの導入発話として、元話と置換後の主人公を含む短いタイトルを生成してよい。

## 6. dialogue品質

topic 生成は「面白い入口」を作る処理であり、dialogue 品質仕様はその入口から実際の会話を深めるための処理である。

対象カテゴリ:

```text
single / double / external / movie / news / forecast / story-simple
```

基本方針:

- topic と dialogue の責務を分ける。
- category ごとに「対話が発見すべきもの」を変える。
- Mio / Shiro の人格は persona 側に置き、IdleChat ではカテゴリ別の役割オーバーレイを使う。
- 内部メタ、category、prompt、seed、provider、JSON などを発話本文に出さない。

カテゴリ別の面白さの核:

| カテゴリ | 面白さの核 |
|---|---|
| `single` | 1つの題材を狭く深く観察する |
| `double` | 遠い2領域の共通構造を見つける |
| `external` | 偶然の外部素材を自然な意味へ変換する |
| `movie` | 存在しない映画を共同で立ち上げる |
| `news` | ニュースの論点、背景、影響、判断の難しさを立体化する |
| `forecast` | 現在の兆しから未来の分岐を考える |
| `story-simple` | 昔話の骨格を保ち、置換主人公による変化を楽しむ |

## 7. Viewer / TTS / raw境界

境界の正本は次の整理。

| 種別 | 役割 | 混ぜてはいけない先 |
|---|---|---|
| raw response | LLM が返した素の応答。診断・監査用 | Viewer 本文、TTS、次ターン prompt |
| view data | Viewer 表示用に整形された本文 | raw 保存領域 |
| audio trigger | TTS と口パクを動かす trigger | 本文表示の唯一根拠 |
| prompt injection data | 次ターン prompt に注入する文脈 | Viewer 本文、TTS |

Viewer 表示の正本は `message_id / turn_index / active_transcript` であり、TTS chunk は表示タイミングや同期に使う。

TTS 失敗時:

- TTS timeout / request failure はエラーとして記録する。
- それだけを理由に会話システム全体を停止しない。
- fallback 表示は成功扱いにせず、fallback としてログ・状態で区別する。

## 8. 即時停止

IdleChat は、通常 Chat / STT のユーザー介入より優先されない。

停止トリガー:

1. STT ボタン押下。
2. Chat 入力欄に1文字でも入力。
3. paste。
4. composition / IME 入力開始。
5. その他、入力意図が確認できる操作。

送信ボタン、Enter 送信、STT final、ユーザー発話完了を待ってはいけない。

基本方針:

- Frontend はローカル UI を先に reset する。
- Server は active generation を invalid 化する。
- context cancel 可能な処理は cancel する。
- cancel 不能または間に合わなかった応答は generation / session / response 照合で discard する。
- interrupt API は LLM / TTS / STT / summary / quality_review の完了を待たない。

## 9. ID設計

ID 設計の目的は、古い応答、重複表示、TTS/STT/Viewer の混線を防ぐこと。

| 単位 | ID |
|---|---|
| セッション | `session_id` |
| メッセージ | `message_id`, `turn_index` |
| TTS 発話 | `utterance_id` |
| chunk | `chunk_index`, `seq` |
| Viewer | `viewer_client_id` |

IdleChat の原則:

- 1 topic ごとに1つの `session_id`。
- 同一 `session_id` 内で `turn_index` は単調増加し、重複しない。
- 同じ発言を表す hydrate / SSE event / TTS payload / fallback reveal は同じ `message_id` を使う。
- 表示順は `turn_index` を優先し、timestamp や TTS 到着順だけで決めない。
- 複数 Viewer では、音声操作対象とマイク操作対象をそれぞれ active viewer として単一化する。

## 10. Forecast

Forecast は、6つの専門ドメインを固定順で回し、トレンド、NHK、Google News などから将来変化の問いを生成する未来展望セッションである。

重要な制約:

- 既定では外部 Coder API を使わない。
- `idle_chat.forecast_external_enabled: true` を明示した場合のみ、外部 Coder provider を Forecast provider として使ってよい。
- 外部 provider 失敗時に別の外部 provider へ自動切替しない。
- Strategy 表示は `forecast/AI技術`、`forecast/経済` のように通常モードと区別する。

## 11. story-simple

story-simple は、昔話の骨格を使い、主人公を別の存在に置き換えて短編としてリメイクする独立モードである。

Story は、多段階パイプラインと専用コーパスを前提にしたが、構築中に非現実的であることが分かったため削除する。
story-simple は Story のフォールバックではなく、Story の代わりに採用する実装済みモードである。

story-simple:

- `data/story/*.json` に依存せず、コード内インラインの物語ソースを使う。
- `forecastLLM()` を使う想定がある。
- `sessionMode == "story-simple"` として扱う。

整理方針:

- カテゴリ正本は `story-simple`。
- `story` category / sessionMode / 起動 API は使わない。
- story-simple は Story とは別物として、Viewer / History / Summary / TTS event で `story-simple` として追跡する。

## 12. 実装照合チェックリスト

### 12.1 カテゴリとローテーション

- [ ] 実装側のカテゴリ一覧が `single / double / external / movie / news / forecast / story-simple` と一致している。
- [ ] `story` category / sessionMode が使われていない。
- [ ] `single -> double -> external -> movie -> news -> forecast -> story-simple` を強制評価または E2E で1巡できる。
- [ ] News と External のすり替えが起きない。
- [ ] Movie が `movie=true` の隠し属性だけで扱われていない。

### 12.2 session / topic / summary

- [ ] 通常 IdleChat が `1 session = 1 topic = 1 summary` になっている。
- [ ] 同一 `session_id` 内で複数 topic を進めていない。
- [ ] summary 完了後に topicBreak を経て次 session に進む。
- [ ] 古い session の event が現在表示へ混入しない。

### 12.3 topic生成とjudge

- [ ] `IdleChat前準備仕様.md` に沿って category 別 seed が準備されている。
- [ ] category 別 prompt / seed / validation / judge の責務が分離されている。
- [ ] News seed 不足時に成功扱いしない。
- [ ] External の provider 名や取得経路が Viewer topic に出ない。
- [ ] Movie topic が `「〜」ってどんな映画？` 形式を守る。
- [ ] story-simple の導入タイトルと本文生成が Story 多段階 pipeline に依存していない。

### 12.4 dialogue品質

- [ ] カテゴリ別 dialogue 方針が実装または prompt に反映されている。
- [ ] 内部メタ、category、prompt、seed、provider、JSON が発話本文に出ない。
- [ ] 直前発話への受け、カテゴリ軸、会話の進展が品質チェックされる。
- [ ] retry reason にカテゴリ別品質失敗を記録できる。

### 12.5 Viewer / TTS / raw

- [ ] Viewer 本文、raw response、TTS speech_text、prompt injection data が分離されている。
- [ ] 表示順が `turn_index` を正としている。
- [ ] 同一 `message_id` の DOM が重複作成されない。
- [ ] TTS chunk 同期は表示タイミングに使い、本文正本を壊していない。
- [ ] TTS 失敗や timeout が fallback 成功扱いになっていない。

### 12.6 即時停止

- [ ] Chat 入力1文字目、paste、IME開始、STTボタンで interrupt が発火する。
- [ ] interrupt API が LLM / TTS / STT / summary 完了を待たず ACK する。
- [ ] `StopManualMode()` と `Interrupt(reason string)` の責務が分かれている。
- [ ] stale generation の `idlechat.message` / `idlechat.summary` / TTS chunk が破棄される。
- [ ] Viewer 実機で入力開始瞬間の停止と stale event 不採用を確認できる。

### 12.7 ID設計

- [ ] `session_id / message_id / turn_index / utterance_id / chunk_index / viewer_client_id` の責務が重複していない。
- [ ] hydrate / SSE / TTS / fallback reveal が同じ `message_id` を使う。
- [ ] `active_audio_viewer_id` と一致しない TTS ack が待ち解除に使われない。
- [ ] `active_input_viewer_id` と一致しない STT result が会話へ反映されない。

### 12.8 Forecast

- [ ] Forecast provider が既定で外部 Coder API を使わない。
- [ ] `idle_chat.forecast_external_enabled: true` の明示時だけ外部 provider を使う。
- [ ] 失敗時に別の外部 provider へ自動切替しない。
- [ ] Strategy が `forecast/<domain>` として表示・履歴・ログで追跡できる。

### 12.9 story-simple

- [ ] story-simple が独立モードとして扱われている。
- [ ] `story` category / sessionMode / 起動 API が残っていない。
- [ ] story-simple が Story の fallback として扱われていない。
- [ ] story-simple が E2E ローテーションに含まれている。
- [ ] Viewer / History / Summary / TTS event で `story-simple` として追跡できる。
