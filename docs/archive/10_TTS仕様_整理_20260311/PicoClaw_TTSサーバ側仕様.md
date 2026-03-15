了解。平文でそのまま出すね。

````md
# RenCrow リアルタイムTTS連携 仕様書

## 1. 目的

本仕様は、RenCrow におけるリアルタイム音声出力機能を実現するための、以下 2 つの実装境界を定義する。

- RenCrow 側実装
- TTS Server 側実装

対象とする目的は以下の通り。

- LLM のストリーミング出力を全文待ちせずに音声化する
- Emotion Planner / Speech Planner を介して自然な発話単位へ整形する
- Style-Bert-VITS2 を CPU 常駐で安定運用する
- 男声 / 女声を同時常駐可能とする
- 将来的な TTS エンジン差し替えを可能にする

---

## 2. スコープ

本仕様に含むもの：

- RenCrow から TTS Server への接続方式
- セッション管理
- ストリーミング文字列の受け渡し
- Speech Buffer
- Emotion Planner 連携
- Speech Planner 連携
- TTS Adapter
- Synth Worker
- voice_id による音声切替
- キュー処理
- health check
- ログ設計
- フェイルセーフ

本仕様に含まないもの：

- 音声認識（STT）
- 会話内容の意味理解
- LLM 推論自体
- 音声再生デバイス制御
- 学習機能
- GPU 最適化
- 音声配信 CDN

---

## 3. 前提

- RenCrow の Chat は Gemma3 系ストリーミング出力を行う
- TTS Engine は当面 Style-Bert-VITS2 を使用する
- 日本語 BERT は 1 本だけロードし、複数音声モデルで共有する
- TTS は CPU 常駐で運用する
- 接続は HTTP 系で行い、リアルタイム本流は WebSocket とする
- SSH は保守・起動・ログ確認専用であり、アプリ間通信には使用しない

---

## 4. 全体構成

### 4.1 論理構成

```text
User
  ↓
RenCrow Chat
  ↓
LLM Token Stream
  ↓
RenCrow Speech Bridge
  ↓
TTS Server (WebSocket)
  ├─ Session Manager
  ├─ Speech Buffer
  ├─ Emotion Planner
  ├─ Speech Planner
  ├─ TTS Adapter
  ├─ Synth Worker
  └─ Audio Output Queue
  ↓
WAV chunk / audio bytes / audio path
  ↓
RenCrow Audio Output
````

### 4.2 責務分離

#### RenCrow 側

* LLM 出力を受け取る
* session を開始 / 維持 / 終了する
* token / text delta を TTS Server に渡す
* 音声出力結果を受け取る
* 再生または転送を行う

#### TTS Server 側

* ストリーミング文字列を蓄積する
* 発話単位へ chunk 化する
* 感情 / prosody を決める
* 音声向け整形を行う
* TTS Engine に変換して音声生成する

---

## 5. RenCrow 側仕様

## 5.1 RenCrow 側の新規コンポーネント

RenCrow 側には以下のコンポーネントを追加する。

* Speech Bridge
* TTS Client
* Audio Sink

### Speech Bridge

LLM ストリームを受け取り、TTS 用 session に流す層。

### TTS Client

TTS Server との WebSocket 接続を担当する層。

### Audio Sink

生成された音声 chunk を受け取り、再生または保存へ流す層。

---

## 5.2 RenCrow 側の責務

RenCrow 側は以下のみを責務とする。

1. 応答 session の開始
2. text delta の逐次送信
3. session 終了通知
4. 音声 chunk 受信
5. 再生順制御

RenCrow 側は以下を持たない。

* chunking 判定
* prosody 計算
* TTS エンジン依存変換
* 発音辞書管理

これらは TTS Server 側責務とする。

---

## 5.3 RenCrow 側の session ライフサイクル

1. Chat が新しい応答開始
2. Speech Bridge が session_id を払い出す
3. TTS Client が session_start を送信
4. LLM token / text delta ごとに text_delta を送信
5. TTS Server から audio_chunk_ready を受信
6. Audio Sink が順番に再生
7. Chat 応答終了時に session_end を送信
8. TTS Server が未送信バッファを flush
9. session 完了

---

## 5.4 RenCrow 側が持つべき最小メタ情報

各応答 session には以下を付与する。

* session_id
* response_id
* voice_id
* speech_mode
* event
* urgency
* conversation_mode
* user_attention_required

例：

```json
{
  "session_id": "sess-001",
  "response_id": "resp-20260310-001",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "event": "conversation",
  "urgency": "normal",
  "conversation_mode": "chat",
  "user_attention_required": false
}
```

---

## 5.5 RenCrow 側の送信ルール

RenCrow 側は token 単位ではなく、LLM 出力 delta 単位で送信してよい。

ただし以下を守る。

* 文字列順序を崩さない
* 同一 session_id に対して順番通り送る
* session_end を必ず送る
* 再送時は chunk 重複防止のため sequence を持つ

推奨メタ情報：

* seq
* emitted_at
* is_final

---

## 5.6 RenCrow 側の再生ルール

RenCrow 側は TTS Server から返る音声を session 単位で受け取る。

再生ルール：

* chunk_index 昇順で再生
* 欠番がある場合は一定時間待機
* timeout 後は欠番をスキップ可能
* session_end 後は残 chunk を再生して閉じる

---

## 6. TTS Server 側仕様

## 6.1 TTS Server の責務

TTS Server は以下を担当する。

1. session 管理
2. text buffer 管理
3. 発話 chunk 確定
4. Emotion Planner 実行
5. Speech Planner 実行
6. TTS Adapter 実行
7. Style-Bert-VITS2 推論
8. 音声返却

---

## 6.2 TTS Server の内部構成

```text
tts_server/
  app.py
  config.py
  session_manager.py
  speech_buffer.py
  emotion_planner.py
  speech_planner.py
  tts_adapter.py
  synth_engine.py
  voice_registry.py
  schemas.py
  output_queue.py
  health.py
  logger.py
```

---

## 6.3 起動時ロード

起動時に以下をロードする。

1. 設定読込
2. voice registry 読込
3. JP BERT tokenizer load
4. JP BERT model load
5. female voice model load
6. male voice model load
7. WebSocket listen 開始
8. ready 状態へ遷移

重要：

* ready 前に BERT / voice をロード完了させる
* 稼働中に毎回 BERT を再ロードしない

---

## 6.4 voice registry

音声切替は `voice_id` を利用する。

例：

```json
{
  "female_01": {
    "model_name": "amitaro",
    "speaker_id": 0,
    "default_style": "Neutral",
    "voice_profile": "lumina_female"
  },
  "male_01": {
    "model_name": "male_model_a",
    "speaker_id": 0,
    "default_style": "Neutral",
    "voice_profile": "lumina_male"
  }
}
```

RenCrow は `voice_id` のみを知る。
Style-Bert-VITS2 の model_name や speaker_id は TTS Server が隠蔽する。

---

## 7. 通信仕様

## 7.1 接続方式

* 保守：SSH
* 健康確認：HTTP
* リアルタイム合成：WebSocket

---

## 7.2 HTTP エンドポイント

### GET /health/live

プロセス生存確認。

応答例：

```json
{
  "status": "live"
}
```

### GET /health/ready

BERT と音声モデルのロード状態確認。

応答例：

```json
{
  "status": "ready",
  "bert": "loaded",
  "voices": ["female_01", "male_01"]
}
```

### GET /health/models

ロード済み voice_id 一覧を返す。

---

## 7.3 WebSocket エンドポイント

### WS /sessions

RenCrow はこの WebSocket に接続する。

---

## 7.4 WebSocket メッセージ種別

### session_start

```json
{
  "type": "session_start",
  "session_id": "sess-001",
  "response_id": "resp-001",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "context": {
    "event": "conversation",
    "urgency": "normal",
    "conversation_mode": "chat",
    "user_attention_required": false
  }
}
```

### text_delta

```json
{
  "type": "text_delta",
  "session_id": "sess-001",
  "seq": 1,
  "text": "今日はですね、",
  "emitted_at": "2026-03-10T18:30:00+09:00"
}
```

### session_end

```json
{
  "type": "session_end",
  "session_id": "sess-001",
  "is_final": true
}
```

### audio_chunk_ready

```json
{
  "type": "audio_chunk_ready",
  "session_id": "sess-001",
  "chunk_index": 0,
  "text": "今日はですね、",
  "audio_path": "cache/sess-001_000.wav",
  "duration_ms": 820
}
```

### error

```json
{
  "type": "error",
  "session_id": "sess-001",
  "code": "VOICE_NOT_FOUND",
  "message": "voice_id female_99 is not registered"
}
```

---

## 8. Session Manager 仕様

## 8.1 session state

各 session は以下を持つ。

```json
{
  "session_id": "sess-001",
  "status": "streaming",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "text_buffer": "",
  "next_seq": 1,
  "next_chunk_index": 0,
  "last_token_ts": "2026-03-10T18:30:00+09:00",
  "context": {
    "event": "conversation",
    "urgency": "normal"
  }
}
```

---

## 8.2 session state machine

状態遷移は以下とする。

* created
* streaming
* flushing
* completed
* error
* timeout

遷移：

* session_start -> created -> streaming
* text_delta -> streaming 維持
* session_end -> flushing
* flush 完了 -> completed
* 例外 -> error
* 無通信 timeout -> timeout

---

## 9. Speech Buffer 仕様

## 9.1 目的

Speech Buffer は LLM の text delta を蓄積し、音声として読める単位に切り出す。

Speech Buffer は感情を判断しない。
責務は「読める塊を作ること」のみ。

---

## 9.2 flush 条件

以下の順で判定する。

1. 文末記号 `。！？`
2. 読点 `、` かつ一定文字数以上
3. 30文字超で分割候補
4. 45文字超で強制分割
5. 一定時間 token が来ない
6. session_end 受信時に全残バッファ flush

---

## 9.3 出力形式

```json
{
  "session_id": "sess-001",
  "chunk_index": 0,
  "raw_text": "今日はですね、"
}
```

---

## 10. Emotion Planner 仕様

Emotion Planner は既存仕様を使用する。

### 入力

* event
* context
* text feature
* voice profile

### 出力

* primary_emotion
* emotion_vector
* prosody
* reason_trace

例：

```json
{
  "primary_emotion": "warm",
  "emotion_vector": {
    "warmth": 0.72,
    "cheerfulness": 0.40,
    "seriousness": 0.32,
    "alertness": 0.18,
    "calmness": 0.68,
    "expressiveness": 0.36
  },
  "prosody": {
    "speed": 0.48,
    "pitch": 0.53,
    "pause": 0.55,
    "expressiveness": 0.39
  },
  "reason_trace": {
    "event": "conversation",
    "applied_context_rules": ["normal_urgency"],
    "applied_text_features": [],
    "voice_profile": "lumina_female"
  }
}
```

---

## 11. Speech Planner 仕様

## 11.1 目的

Speech Planner は発話を読み上げ向けに整形する。

責務：

* chunking 補正
* text normalization
* pause planning

---

## 11.2 text normalization

以下を許可する。

* AI -> エーアイ
* LLM -> エル・エル・エム
* URL -> 省略または読み飛ばし
* 記号連続 -> 圧縮
* 長すぎる括弧補足 -> 削除または後置

以下は禁止する。

* 意味変更
* 結論の削除
* 感情の追加
* 不明略語の勝手な展開

---

## 11.3 pause planning

pause は以下の 4 段階。

* none
* short
* medium
* long

ルール例：

* `、` -> short
* `。` -> medium
* 文終了かつ段落切れ相当 -> long
* alertness 高 -> pause を短め
* calmness 高 -> pause を長め

---

## 11.4 speech_plan 出力例

```json
{
  "chunk_index": 0,
  "normalized_text": "今日はですね、",
  "speech_mode": "conversational",
  "pause_after": "short",
  "delivery_trace": {
    "chunk_rule_hits": ["comma_split"]
  }
}
```

---

## 12. TTS Adapter 仕様

## 12.1 目的

Emotion Planner と Speech Planner の中間表現を、Style-Bert-VITS2 固有パラメータへ変換する。

---

## 12.2 入力

* normalized_text
* voice_id
* emotion_vector
* prosody
* pause_after

---

## 12.3 出力例

```json
{
  "model_name": "amitaro",
  "speaker_id": 0,
  "style": "Neutral",
  "style_weight": 1.8,
  "sdp_ratio": 0.4,
  "length": 1.0,
  "noise": 0.6,
  "noise_w": 0.8
}
```

---

## 12.4 設計原則

* Emotion Planner は TTS エンジン名を知らない
* Speech Planner は TTS エンジン名を知らない
* Adapter だけが TTS エンジン依存を持つ

---

## 13. Synth Worker 仕様

## 13.1 目的

Style-Bert-VITS2 を常駐保持し、chunk ごとに音声を生成する。

---

## 13.2 常駐ロード対象

* JP BERT tokenizer
* JP BERT model
* female voice model
* male voice model

---

## 13.3 実行単位

1 chunk 単位で `infer()` を呼ぶ。

---

## 13.4 キュー処理

初期実装では直列処理とする。

理由：

* CPU 常駐では並列推論より直列の方が安定しやすい
* 同時発話より順番再生が優先
* 順番保証がしやすい

---

## 13.5 出力

* wav bytes または wav file path
* sample rate
* duration_ms
* synth_elapsed_ms

---

## 14. 音声返却方式

初期実装は WAV ファイル出力とする。

### 理由

* デバッグが容易
* chunk ごとの確認が容易
* 再生失敗時の切り分けが容易

返却例：

```json
{
  "type": "audio_chunk_ready",
  "session_id": "sess-001",
  "chunk_index": 1,
  "text": "少し面白い話があります。",
  "audio_path": "cache/sess-001_001.wav",
  "duration_ms": 1320
}
```

将来的には WebSocket バイナリ転送へ拡張可能とする。

---

## 15. レイテンシ指標

TTS Server は以下のメトリクスを計測する。

* buffer_wait_ms
* emotion_plan_ms
* speech_plan_ms
* synth_ms
* total_chunk_latency_ms

RenCrow 側は以下を計測する。

* llm_first_token_ms
* tts_first_chunk_ms
* first_audio_play_ms
* total_response_audio_ms

---

## 16. エラー処理

## 16.1 RenCrow 側

* WebSocket 切断時は session を abort
* chunk 順序欠損時は一定時間待機
* timeout 後は欠番をスキップ可能
* 再生失敗時はログのみ記録し応答継続可

---

## 16.2 TTS Server 側

* voice_id 不明 -> default voice
* Emotion Planner 失敗 -> neutral
* Speech Planner 失敗 -> 原文そのまま
* infer 失敗 -> error event を返し次 chunk 継続
* session_end 受信時は残 buffer を強制 flush
* 一定時間無通信 -> timeout

---

## 16.3 error code 例

* SESSION_NOT_FOUND
* INVALID_SEQ
* VOICE_NOT_FOUND
* MODEL_NOT_READY
* SYNTH_FAILED
* SESSION_TIMEOUT

---

## 17. ログ仕様

## 17.1 RenCrow 側ログ

* session_start sent
* text_delta sent
* session_end sent
* audio_chunk received
* audio_play start
* audio_play done
* session_closed

---

## 17.2 TTS Server 側ログ

* session_created
* text_delta_received
* chunk_flushed
* emotion_planned
* speech_normalized
* synth_started
* synth_finished
* audio_chunk_ready
* session_completed
* error

全ログには以下を含める。

* session_id
* response_id
* chunk_index
* voice_id
* elapsed_ms
* event_id

---

## 18. セキュリティ

* TTS Server は初期実装では 127.0.0.1 バインド
* 外部公開しない
* 認証は初期実装では省略可能
* 将来リモート化する場合は reverse proxy + 認証を追加
* SSH は保守専用であり、音声生成 API を SSH 越しに叩かない

---

## 19. デプロイ方針

## 19.1 初期構成

* RenCrow 本体と TTS Server は同一ホスト
* 接続先は localhost
* Style-Bert-VITS2 は CPU 常駐
* voice は男声 / 女声の 2 本常駐

---

## 19.2 将来構成

* RenCrow と TTS Server の別ホスト化
* WebSocket over LAN
* WAV バイナリ転送
* Adapter 差し替えによる別 TTS エンジン追加

---

## 20. 最小実装範囲

初版で実装するもの：

* TTS Server WebSocket
* `session_start / text_delta / session_end`
* Speech Buffer flush
* Emotion Planner v1
* Speech Planner 最小版
* Style-Bert-VITS2 CPU 常駐
* voice_id 切替
* WAV 出力
* RenCrow 側 Speech Bridge
* RenCrow 側 Audio Sink
* health API

初版で実装しないもの：

* GPU 分岐
* バイナリ音声ストリーム返却
* 並列推論
* 動的 voice load / unload
* STT 統合
* 学習

---

## 21. 実装順

### Phase 1

* TTS Server 単体起動
* BERT / voice preload
* `/health/ready` 実装

### Phase 2

* `session_start / text_delta / session_end`
* Speech Buffer 実装
* chunk ごとの WAV 出力

### Phase 3

* Emotion Planner / Speech Planner 統合
* voice_id 切替

### Phase 4

* RenCrow Speech Bridge 実装
* Audio Sink 実装
* session 連携

### Phase 5

* latency 計測
* timeout / error 処理
* ログ整備

---

## 22. 設計上の結論

本設計における重要な結論は以下。

1. RenCrow と TTS Server の接続は HTTP 系、実運用は WebSocket とする
2. SSH は保守用であり、音声生成経路に使わない
3. TTS Server は単なる `text -> wav` 変換器ではなく、発話実行器として設計する
4. 日本語BERTは 1 回だけロードし、男声 / 女声で共有する
5. CPU 常駐を前提とし、GPU 4GB 案は採用しない
6. RenCrow 側は chunking や prosody を持たず、TTS Server に責務を寄せる
7. 全文待ちせず chunk 単位で先に話し始める構成を採用する

---

## 23. 補足：実装の最終的な一行要約

RenCrow は LLM の文字列を逐次 TTS Server に流し、TTS Server はそれを発話単位に確定・感情付与・読み上げ整形した上で、常駐 Style-Bert-VITS2 により音声化し、chunk 単位で RenCrow へ返却する。

```
```
