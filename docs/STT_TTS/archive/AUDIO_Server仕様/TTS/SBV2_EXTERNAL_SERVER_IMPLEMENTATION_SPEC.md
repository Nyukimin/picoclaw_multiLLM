# SBV2 外部サーバ実装仕様（SBV2インストール済み前提）

本仕様は、**SBV2 自体のインストールのみ完了**している環境で、別サーバ（別リポジトリ）に TTS サービスを実装するための詳細仕様です。  
PicoClaw からの利用互換を最優先にし、`docs/SBV2_SERVER_IMPLEMENTATION_REQUIREMENTS.md` の契約を実装可能な粒度まで分解しています。

---

## 1. 目的とスコープ

### 1.1 目的
- PicoClaw が以下 2 経路で SBV2 を利用できるサーバを提供する。
  - Autonomous 直呼び出し: `POST /synthesis`
  - TTS Client Bridge: `GET /health/ready` + `POST /synthesize` + `WS /sessions`

### 1.2 スコープ
- 本仕様に含む:
  - HTTP / WebSocket API
  - 推論実行フロー
  - 音声ファイル管理
  - エラー応答契約
  - 運用・監視・テスト
- 本仕様に含まない:
  - SBV2 の学習
  - モデルの追加訓練
  - PicoClaw 側コード変更

### 1.3 前提
- Python 実行環境は構築済み
- SBV2 依存パッケージとモデル配置は完了済み
- 実装先サーバは FastAPI + Uvicorn を採用可能

### 1.4 配置・到達性の前提（重要）

#### 物理・ネットワーク構成（想定）
- **ユーザーエンド**は、ブラウザ利用端末や PicoClaw 実行端末など、宅内サーバ群の外側からアクセスするクライアントを指す。
- **ブラウザを含むユーザーエンド**はリモートからアクセスし得る。
- **ブラウザ以外のサーバ群**（本 TTS サービス、関連バックエンド、宅内で動かす連携プロセスなど）は、**同一宅内ルータ配下の LAN 上に置く**想定とする（同一ホームネットワーク内で相互参照しうる）。

#### 通常運用（構築・到達）
- 接続の基盤は **基本的に Tailscale** とする（例: `*.ts.net`、MagicDNS。サブネットルーティングは運用ポリシーに従う）。
- Tailscale 経由で **HTTP(S) と WebSocket がクライアントから安定して到達できる**ことを標準とする。

#### Tailscale 不調時のフォールバック（最終手段）
- Tailscale が不安定・利用不能で**どうしようもない場合に限り**、**ユーザーエンド以外**（宅内サーバ同士の通信、設定で指定する `127.0.0.1` 以外の内部向け URL など）は **宅内プライベート IP（同一ルータ LAN）** で到達できるように **切り替え可能にしておく**（例: `192.168.x.x`）。あくまで**緊急用**であり、復旧後は **Tailscale 前提に戻す**ことを推奨する。
- 公開インターネット直開放や、本番の既定を LAN IP に固定することは推奨しない。

#### 実装・設定上の注意
- **設定 URL の一貫性**: `tts.http_base_url` / `tts.ws_url` / `tts.sbv2.base_url` は、当該クライアントから実際に到達できるオリジンに合わせる。ページが HTTPS のときは **Mixed Content** にならないよう、API も **HTTPS**、WS は **WSS** を検討する（Tailscale HTTPS / 証明書運用に合わせる）。
- **`audio_path` と `audio_url`**: `audio_path` はサーバ上のパスである。リモートブラウザが再生主体の場合、**`audio_url`（または静的配信）**が再生安定に寄与する（REQ の「`audio_path` または `audio_url`」契約と整合）。
- **CORS / WS**: ブラウザから直接 API を叩く構成では、必要な **CORS** と **WebSocket のアップグレード許可**をサーバ側で満たす。

---

## 2. 推奨アーキテクチャ

### 2.1 コンポーネント
- `api/http.py`: HTTP エンドポイント実装
- `api/ws.py`: WebSocket セッション実装
- `core/runtime.py`: 起動状態、ロード済みモデル、セッション状態
- `core/synthesizer.py`: SBV2 呼び出し、WAV 保存
- `core/chunker.py`: text_delta 分割ロジック
- `core/voice_registry.py`: `voice_id` -> モデル/話者設定マッピング
- `core/errors.py`: エラーコード定義

### 2.2 スレッド/排他戦略
- 同時推論で GPU/CPU が飽和しないように、以下のいずれかを実装:
  - `asyncio.Semaphore(1..N)` で同時推論数を制限
  - `asyncio.Lock()` で直列化（実装容易、遅延増加）
- 目標:
  - 応答の安定性 > スループット
  - 最初は `N=1` で開始し、負荷試験後に増やす

### 2.3 音声キャッシュ方針
- 生成先ディレクトリ例: `./cache`
- ファイル命名:
  - oneshot: `oneshot-{uuid}_{chunk}.wav`
  - session: `{session_id}_{chunk_index:03d}.wav`
- 保持期間:
  - 既定 24h で定期削除
- パス返却:
  - API 契約上 `audio_path` を必ず返却
  - 必要に応じて `audio_url` も併記

---

## 3. API 詳細仕様（必須）

## 3.1 `GET /health/live`

### 目的
- プロセス生存確認（Liveness）

### レスポンス
```json
{
  "status": "live"
}
```

### ステータス
- `200 OK`

---

## 3.2 `GET /health/ready`

### 目的
- 推論可能状態確認（Readiness）

### レスポンス（必須）
```json
{
  "status": "ready",
  "voices": ["female_01", "male_01", "mio"]
}
```

### 仕様
- 起動直後で未ロードなら:
  - `200` + `{"status":"starting","voices":[]}` でも可
- PicoClaw 連携時は `status == "ready"` が必要
- `voice_id` 指定運用する場合、`voices` に含める

---

## 3.3 `POST /synthesis`（Autonomous 直呼び出し）

### 目的
- PicoClaw の `tts.sbv2.base_url` から直接呼ばれる本命 API

### リクエスト
```json
{
  "text": "こんにちは",
  "voice_id": "mio",
  "emotion": "calm",
  "speed": 1.0,
  "pitch": 0.0
}
```

### バリデーション
- `text`:
  - 必須
  - trim 後に空文字なら `400`
- `voice_id`:
  - 任意（未指定時はデフォルトボイス）
  - 不正値は `404` または `400`
- `emotion` / `speed` / `pitch`:
  - 任意
  - 未対応でも受理し、内部で無視可

### レスポンス（必須）
```json
{
  "audio_path": "cache/oneshot-abc123.wav",
  "duration_ms": 1234,
  "voice_id": "mio"
}
```

### 契約
- 2xx を返すこと
- `audio_path` は必須（空不可）
- `duration_ms` は省略可
- `voice_id` は省略可だが返却推奨

### 実装メモ
- `duration_ms = int((len(audio) / sample_rate) * 1000)` で算出可能
- `voice_id` エイリアスを持つこと（例: `mio -> female_01`）

---

## 3.4 `POST /synthesize`（Bridge Fallback）

### 目的
- WebSocket 未使用時のフォールバック

### リクエスト
```json
{
  "text": "こんにちは",
  "voice_id": "female_01",
  "emotion_state": {
    "primary_emotion": "calm"
  }
}
```

### レスポンス
```json
{
  "text": "こんにちは",
  "audio_path": "cache/oneshot-xyz.wav",
  "audio_url": "https://<tailnet-host>.ts.net:8765/audio/oneshot-xyz.wav"
}
```

### 契約
- 2xx を返すこと
- `audio_path` または `audio_url` のどちらか必須
- 互換性のため、**両方返却推奨**

---

## 3.5 `WS /sessions`（Bridge ストリーミング）

### 受信メッセージ（PicoClaw -> サーバ）

1) `session_start`
```json
{
  "type": "session_start",
  "session_id": "sess-123",
  "response_id": "resp-123",
  "character": "mio",
  "voice_id": "female_01",
  "speech_mode": "conversational",
  "context": {
    "event": "conversation",
    "urgency": "normal",
    "conversation_mode": "chat",
    "user_attention_required": false,
    "user_waiting_time_sec": 0
  }
}
```

2) `text_delta`（複数回）
```json
{
  "type": "text_delta",
  "session_id": "sess-123",
  "seq": 1,
  "text": "こんにちは",
  "emitted_at": "2026-02-22T14:00:00Z",
  "emotion_state": {
    "primary_emotion": "warm",
    "prosody": {
      "speed": 1.0,
      "pitch": 0.0,
      "pause": 0.1,
      "expressiveness": 0.6
    }
  }
}
```

3) `session_end`
```json
{
  "type": "session_end",
  "session_id": "sess-123",
  "is_final": true
}
```

### 送信メッセージ（サーバ -> PicoClaw）

1) `audio_chunk_ready`（0 回以上）
```json
{
  "type": "audio_chunk_ready",
  "chunk_index": 0,
  "text": "こんにちは",
  "audio_path": "cache/sess-123_000.wav",
  "audio_url": "https://<tailnet-host>.ts.net:8765/audio/sess-123_000.wav",
  "pause_after": "short"
}
```

2) `session_completed`
```json
{
  "type": "session_completed",
  "session_id": "sess-123"
}
```

3) `error`
```json
{
  "type": "error",
  "code": "synthesis_failed",
  "message": "detail message"
}
```

### WS 契約詳細
- `seq` は厳密に単調増加で受理（期待値不一致は `error`）
- `chunk_index` も単調増加
- `session_end` 受信時は残バッファを flush してから `session_completed`
- 例外時は可能な限り `type=error` を返してセッション破棄

---

## 4. 音声生成ロジック仕様

## 4.1 ボイスレジストリ
- 例:
```yaml
female_01:
  model_name: amitaro
  speaker_id: 0
  style: Neutral
  style_weight: 2.0
  language: JP
mio:
  alias_of: female_01
male_01:
  model_name: shin-gozaki-jp
  speaker_id: 0
  style: Neutral
  style_weight: 2.0
  language: JP
```

### 要件
- `voice_id` 未指定時はデフォルトボイスにフォールバック
- `alias_of` を解決した最終 ID をログに出す

## 4.2 テキスト正規化
- 実装推奨:
  - 連続空白圧縮
  - URL 置換（読み上げ困難な文字列対策）
  - 句読点の連続抑制
- 注意:
  - 過剰正規化で原文意味を壊さない

## 4.3 チャンク分割（WS）
- 優先順:
  1. 文末記号（`。！？`）
  2. 読点（`、`）
  3. 最大長（例 45 文字）
- `session_end` 時は必ず `force_flush`

---

## 5. エラー仕様

## 5.1 HTTP エラー
- 代表コード:
  - `400`: invalid request（空 text など）
  - `404`: voice not found
  - `503`: not ready
  - `500`: synthesis failed

### HTTP エラーボディ推奨
```json
{
  "error": {
    "code": "voice_not_found",
    "message": "voice_id 'xxx' is not registered"
  }
}
```

## 5.2 WebSocket エラーコード
- `VOICE_NOT_FOUND`
- `SESSION_NOT_FOUND`
- `INVALID_SEQ`
- `SYNTHESIS_FAILED`
- `UNKNOWN_MESSAGE_TYPE`

### WS エラーフォーマット
```json
{
  "type": "error",
  "session_id": "sess-123",
  "code": "INVALID_SEQ",
  "message": "expected seq=2, got seq=4"
}
```

---

## 6. タイムアウト・性能要件

## 6.1 推奨タイムアウト目標
- Connect: 3 秒以内に接続確立
- Ready API: 300 ms 以内（通常時）
- `/synthesis`: 20 秒以内を目標（長文除く）
- WS chunk gap: 3 秒以内で次チャンク通知を目標

## 6.2 性能チューニング指針
- 初回ロード遅延を避けるため、起動時に:
  - tokenizer
  - BERT
  - TTS モデル（必要最小限）
  を preload
- 同時推論数は小さく始める（1〜2）

---

## 7. セキュリティ・運用

## 7.1 入力防御
- `text` 長さ上限（例: 1000 文字）
- ファイルパス組み立ては固定ディレクトリ配下のみ
- 任意パス読み出し禁止

## 7.2 ログ
- 構造化ログ推奨:
  - request_id
  - session_id
  - voice_id
  - elapsed_ms
  - result(success/fail)
- 音声生成失敗時は stack trace を内部ログに残す

## 7.3 監視
- メトリクス:
  - ready 状態
  - 推論時間 p50/p95
  - error rate
  - active sessions

---

## 8. 実装手順（別サーバ側）

1. FastAPI プロジェクト初期化
2. RuntimeState 実装（ready/voice/session/synth_lock）
3. voice registry 実装（alias 対応）
4. SBV2 ローダ実装（起動時 preload）
5. `GET /health/live` 実装
6. `GET /health/ready` 実装
7. `POST /synthesis` 実装（最優先）
8. `POST /synthesize` 実装
9. `WS /sessions` 実装（start/delta/end）
10. キャッシュ掃除ジョブ実装
11. 統合テスト実装
12. 負荷試験・タイムアウト調整

---

## 9. 受け入れテスト項目（完了条件）

### 9.1 HTTP
- [ ] `/health/ready` が `200` + `status=ready`
- [ ] `/synthesis` が 2xx + `audio_path` 非空
- [ ] `/synthesize` が 2xx + `audio_path or audio_url`

### 9.2 WS
- [ ] `session_start -> text_delta -> session_end` で `audio_chunk_ready -> session_completed`
- [ ] `seq` 異常で `type=error` が返る
- [ ] 不正 `voice_id` で `type=error` が返る

### 9.3 音声ファイル
- [ ] 返却 `audio_path` が実在し再生可能
- [ ] 相対パス返却時に `audio_path_root` 前提で解決可能

---

## 10. 実装上の注意（重要）

- 互換性優先のため、**`/synthesis` と `/synthesize` を両方実装**する。
- `audio_path` は常に返す（`audio_url` だけにしない）。
- `voice_id` は運用側が変わりやすいため、alias テーブルを持つ。
- 仕様外フィールドを受け取っても、失敗させず無視する設計を基本とする。
- エラーレスポンスは人間可読な `message` を必ず含める。

---

## 11. 実装成果物チェックリスト（別サーバリポジトリ）

- [ ] `README.md`（起動手順）
- [ ] `openapi.json` または API ドキュメント
- [ ] 環境変数一覧 (`.env.example`)
- [ ] `tests/test_http_contract.py`
- [ ] `tests/test_ws_contract.py`
- [ ] `tests/test_voice_registry.py`
- [ ] `tests/test_error_contract.py`

以上。
