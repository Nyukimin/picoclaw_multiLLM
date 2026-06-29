# IdleChat 仕様

## 目的

IdleChat は、ユーザー操作がない時間にエージェント同士が自律的に会話する機能である。

通常 Chat / STT / ユーザー操作が最優先であり、ユーザー介入が始まった場合は IdleChat を即時停止または無効化する。

## LLM alias

IdleChat では speaker ごとに LLM provider を分ける。

| speaker | alias | 用途 |
|---|---|---|
| Mio | Chat | 軽量な話題生成、読み上げ、相槌 |
| Shiro | ChatWorker / Worker | IdleChat 内の短文会話発話は ChatWorker、要約補助は Worker |
| Forecast | local Forecast provider | 未来展望のキーワード抽出・topic生成 |

`ChatWorker` は RenCrow_LLM Worker endpoint (`:8082`) 上の短文用 alias であり、IdleChat の Shiro 発話に使う。RenCrow_LLM 側の正本では `ChatWorker` は `nothink` / GPT-OSS `low` / `max_tokens` cap 8192、有効入力 context budget 16384 とする。要約補助、通常の Shiro / Worker Core / Coder 検証は同じ endpoint 上の `Worker` alias を使い、`reasoning` / GPT-OSS `high` / `max_tokens` cap 65536、有効入力 context budget 65536 とする。

`ChatWorker` と `Worker` は同じ Ollama model runner (`rencrow-gpt-oss-120b:64k`) を共有する。再ロードを避けるため、Ollama へ送る `num_ctx` は 65536 のまま維持し、`ChatWorker` の 1/4 context は RenCrow_LLM proxy 側の有効入力 budget として扱う。

IdleChat は request payload の `metadata` で用途判定しない。用途の切り分けは alias で行う。system prompt 内の `/no_think` や `この会話はidleChatです` は thinking / 表示本文制御のための指示であり、max_tokens cap の routing 根拠にしない。

LLM 呼び出しの queue wait と generation timeout は分けて扱う。特に Mio は `Chat` alias の短い体感応答が必要なため、本文生成で timeout が出た場合は `queue` 待ちと `generate` 実行時間を混同しない。

## モード

| mode | 内容 | 主な実装箇所 |
|---|---|---|
| normal | single / double / external / movie / news の通常雑談 | `internal/application/idlechat/orchestrator.go`, `topic_generator.go` |
| forecast | 未来展望・トピック探索 | `internal/application/idlechat/forecast_*.go` |
| story-simple | 昔話の骨格を使い、主人公を別存在に置き換える軽量リメイク短編 | `internal/application/idlechat/story_mode_simple.go` |

Story は、構築中に非現実的であることが分かったため仕様対象から削除する。
story-simple は Story の代替として実装済みの独立モードであり、Story とは別物として扱う。

Viewer は IdleChat の Live Timeline、Summary Review、History を表示する。これらは raw response そのものではなく、表示用 state と診断情報の投影である。

## raw / view / audio 境界

IdleChat では、編集後の view data と LLM の素の raw response を分ける。

| 種別 | 用途 |
|---|---|
| raw response | LLM が返した未加工応答。診断・監査用 |
| view data | Timeline / Summary / History に出す整形済み本文 |
| speech text | TTS provider に渡す読み上げ文字列 |
| prompt context | 次ターン生成に渡す内部文脈 |

raw response をそのまま Viewer 本文、TTS、次ターン prompt に流してはいけない。

## お題読み上げ

IdleChat のお題読み上げは、取得済み topic へ `今日のお題。` を前置するだけの決定的処理とする。

- 対象カテゴリは `single / double / external / movie / news / forecast / story-simple`。
- topic 本文を LLM で再生成、要約、言い換えしない。
- カテゴリ名、内部 strategy、seed、provider 名を読み上げ本文へ入れない。
- story-simple は元話と置換後の主人公を含む導入タイトルを生成してよい。ただし `story` category / sessionMode / 多段階 Story 仕様へ戻してはいけない。
- 最終読み上げ文字列は `今日のお題。<topic>` の 1 発話単位とし、TTS `speech_text` としてのみ扱う。
- Viewer の描画正本は変換前の topic / display event とする。

## STT との境界

STT input は通常 Chat に流す。IdleChat に直接流さない。

ユーザー入力が来た場合、IdleChat は中断または状態更新の対象になるが、音声入力そのものを IdleChat 会話として扱わない。

## 主な実装箇所

| 領域 | パス |
|---|---|
| orchestrator | `internal/application/idlechat/orchestrator.go` |
| topic generation | `internal/application/idlechat/topic_generator.go` |
| topic store | `internal/application/idlechat/topic_store.go` |
| forecast | `internal/application/idlechat/forecast_*.go` |
| story-simple | `internal/application/idlechat/story_mode_simple.go` |
| viewer | `internal/adapter/viewer/viewer.html` |
| TTS bridge | `cmd/picoclaw/idlechat_tts.go` |

## raw response 診断

IdleChat では、編集後の view data と LLM の raw response を分ける。

- Summary Review / History は、表示状態、境界、終了状態を追うための観測面である。
- raw response は診断用として見えるようにしてよいが、ユーザー向け本文として扱わない。
- raw response が空、エラー、timeout の場合は fallback 成功扱いにしない。

## event 契約

IdleChat は Viewer / TTS / log に event を出す。

必須の追跡単位:

- `session_id`
- `message_id`
- `turn_index`
- `topic`
- `category`
- `strategy`
- `tts_error` / `fallback` などの診断状態

## TTS 待ち合わせ

IdleChat は TTS の完了を待って会話テンポと口パクを揃える。ただし、TTS 未完了で IdleChat 全体を永久停止させてはいけない。

- TTS playback ack は `active_audio_viewer_id` と一致する Viewer からのものだけを採用する。
- timeout 後に遅れて届いた音声は、`session_id / utterance_id / chunk_index` が現在の発話と一致しない限り再生しない。
- 古い音声を次 session に持ち越してはいけない。
- TTS 失敗はログに残し、成功や通常 fallback として隠さない。

## 検証

確認対象:

- normal / forecast / story-simple の開始条件が維持される。
- `single / double / external / movie / news / forecast / story-simple` を1巡できる。
- News と External がすり替わらない。
- Movie が独立カテゴリとして記録される。
- raw response と view data が分離される。
- TTS timeout が会話全体停止にも成功扱いにもならない。
- Story の仕様・起動条件・validator が残っていない。
- story-simple が Story のフォールバックではなく、独立モードとして扱われる。
