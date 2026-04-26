# RenCrow TTS 新ブリッジ仕様（Client）

本書は、RenCrow から RenCrow_TTS を利用するための **Client 契約仕様** です。  
本仕様では、音声生成経路を `POST /synthesis` に一本化します。

---

## 1. 目的

- TTS 呼び出し経路を `POST /synthesis` に統一する
- SBV2 推論制御を `provider_params` で透過的に渡す
- エラー処理を `error.code` ベースで安定化する

---

## 2. スコープ

### 対象
- RenCrow -> RenCrow_TTS の HTTP 契約
- ヘッダ運用
- エラー処理契約

### 非対象
- WebSocket `/sessions` 経路
- `POST /synthesize` 互換経路
- 旧ブリッジ仕様との後方互換

---

## 3. エンドポイント契約

### 3.1 GET `/health/live`

- 用途: 生存確認
- 成功レスポンス:

```json
{ "status": "live" }
```

### 3.2 GET `/health/ready`

- 用途: 合成可否確認
- 判定条件:
  - HTTP 200
  - `status == "ready"`
  - `provider == "sbv2"`（存在する場合）

### 3.3 POST `/synthesis`（唯一の合成経路）

#### 必須ヘッダ

- `Content-Type: application/json`
- `X-RenCrow-TTS-Request-Id: <client_generated_id>`

#### リクエスト

| フィールド | 型 | 必須 | 内容 |
|---|---|---:|---|
| `text` | string | yes | 合成テキスト |
| `voice_id` | string | yes | 使用 voice_id |
| `speed` | float | no | 話速 |
| `pitch` | float | no | ピッチ |
| `provider_params` | object | no | SBV2 制御パラメータ |

`provider_params` 許可キー:

- `model_name`
- `model_file`
- `speaker_id`
- `speaker_name`
- `style`
- `style_weight`
- `language`
- `sdp_ratio`
- `noise`
- `noise_w`
- `split_interval`
- `line_split`
- `length`

#### レスポンス（成功）

| フィールド | 型 | 内容 |
|---|---|---|
| `request_id` | string | サーバ処理ID |
| `audio_path` | string \| null | 音声ファイル相対パス |
| `audio_url` | string \| null | 取得可能URL |
| `duration_ms` | int \| null | 音声長 |
| `voice_id` | string \| null | 解決後 voice |

---

## 4. エラー契約

失敗時フォーマット:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "..."
  }
}
```

代表的な `error.code`:

- `invalid_request`（400）
- `text_too_long`（400）
- `voice_not_found`（404）
- `audio_not_found`（404）
- `engine_unavailable`（503）
- `synthesis_failed`（500）

**Client 実装ルール**
- 分岐は必ず `error.code` で行う
- `message` の文言一致で分岐しない

---

## 5. Client 運用ルール

- `voice_id` は常に明示する（未指定運用禁止）
- `X-RenCrow-TTS-Request-Id` は Client 側で生成して付与する
- `provider_params` は許可キーのみ送信する
- `audio_url` が空の場合は `audio_path` から解決して再生する

---

## 6. 受け入れチェックリスト

1. `/health/live` が 200 + `status=live`
2. `/health/ready` が 200 + `status=ready`
3. `/synthesis` が 2xx + `audio_path` または `audio_url`
4. `X-RenCrow-TTS-Request-Id` を全リクエストで付与
5. `provider_params` 変更で音声結果が変化
6. 不正 `provider_params` で `400 invalid_request`
7. 未知 `voice_id` で `404 voice_not_found`

