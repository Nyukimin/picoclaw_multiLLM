# RenCrow OpenAI-Compatible MLX API

## Base URLs

Chat / Worker / Heavy / Wild は別プロセス、別ポートで起動する。Chat が重い処理で詰まることを避けるため、RenCrow 側でも用途ごとに base URL を分ける。

| Use | Base URL | Config |
| --- | --- | --- |
| Chat | `http://127.0.0.1:8081` | `configs/chat-server.toml` |
| Worker | `http://127.0.0.1:8082` | `configs/worker-server.toml` |
| Heavy | `http://127.0.0.1:8083` | `configs/heavy-server.toml` |
| Wild | `http://127.0.0.1:8084` | `configs/wild-server.toml` |

LAN から呼ぶ場合は各 config の `host` を `0.0.0.0` に変更し、RenCrow 側では `http://<MacのIP>:8081` のように指定する。

## Models

RenCrow 側は用途ごとに base URL と `model` 名を切り替える。

| Use | model | Backing MLX model |
| --- | --- | --- |
| Chat | `Chat` | `/Users/yukimi/models/gemma-4-E4B-it-UD-MLX-4bit` |
| Worker | `Worker` | `/Users/yukimi/models/Qwen3-VL-30B-A3B-Thinking-4bit` |
| Heavy | `Heavy` | `/Users/yukimi/models/Qwen3.5-122B-A10B-4bit` |
| Wild | `Wild` | `/Users/yukimi/models/Qwen3.6-35B-A3B-Abliterated-Heretic-MLX-4bit` |

現行の公開 model 名は `Chat` / `Worker` / `Heavy` / `Wild` の 4 つ。
`Coder` は Worker 設定内に用途名として残しているが、現行 proxy ではクライアントへ公開しない。

## Endpoints

- `POST /v1/chat/completions`
- `GET /v1/models`
- `GET /health`
- `GET /v1/health`

`GET /v1/models` はクライアントが送信に使う公開 model 名を `id` で返し、表示・確認用の実体 model 名を `backend_model` で返す。

```json
{
  "object": "list",
  "data": [
    {
      "id": "Worker",
      "object": "model",
      "owned_by": "local",
      "backend_model": "/Users/yukimi/models/Qwen3-VL-30B-A3B-Thinking-4bit"
    }
  ]
}
```

RenCrow 側から `/v1/chat/completions` に指定する `model` は `id` を使う。
`backend_model` は画面表示・状態確認用で、リクエストの model 名には使わない。

## Request

OpenAI互換の chat completions request を受け付ける。

```json
{
  "model": "Chat",
  "messages": [
    { "role": "system", "content": "You are a concise assistant." },
    { "role": "user", "content": "こんにちは" }
  ],
  "temperature": 0.2,
  "max_tokens": 256,
  "stream": false
}
```

対応済みの主要 field:

- `model`
- `messages`
- `temperature`
- `max_tokens`
- `stream`
- `top_p`
- `top_k`
- `min_p`

## Response

non-streaming は OpenAI互換で `choices[0].message.content` を返す。

streaming は `text/event-stream` の SSE 形式で `data: {...}` chunk を返す。

## Auth

現状は認証なし。RenCrow 側の API key は空文字、または任意の dummy 値でよい。

## Limits

- Chat サーバ既定の `max_tokens`: `2048`
- Worker / Heavy サーバ既定の `max_tokens`: `4096`
- Wild サーバ既定の `max_tokens`: `2048`
- request ごとの `max_tokens`: 指定可能
- Chat tokenizer config の実用上限は未明示
- Wild tokenizer config の `model_max_length`: `262144`
- Worker は初回呼び出し時にモデルを取得するため、取得後に config を確認する

## Operational Notes

- 各用途は別プロセスで常駐させる。
- Chat プロセスは `Chat` 以外の model 名を拒否する。
- Worker プロセスは `Worker` 以外の model 名を拒否する。
- Heavy プロセスは `Heavy` 以外の model 名を拒否する。
- Wild プロセスは `Wild` 以外の model 名を拒否する。
- 各プロセス内は安定性優先で単一リクエスト処理にしている。
- RenCrow 側の同時リクエストは用途ごとに 1 が安全。
- 初回リクエストはモデルロードとダウンロードで遅い。常駐後は同じモデルがメモリ上に残る。
- warmup は起動後に各 base URL へ `max_tokens: 1` の短い request を送る。

## Start Commands

```sh
uv run mlx-servers Chat
uv run mlx-servers Worker
uv run mlx-servers Heavy
uv run mlx-servers Wild
```

通常運用の Chat + Worker をまとめて起動する場合:

```sh
uv run mlx-servers
```

## Health / Restart Commands

用途ごとに health check:

```sh
uv run mlx-health Chat
uv run mlx-health Worker
uv run mlx-health Heavy
uv run mlx-health Wild
```

全用途まとめて health check:

```sh
uv run mlx-health
```

5分に1回の継続 health check:

```sh
uv run mlx-health-watch
```

用途ごとに再起動:

```sh
uv run mlx-restart Chat
uv run mlx-restart Worker
uv run mlx-restart Heavy
uv run mlx-restart Wild
```

全用途まとめて再起動:

```sh
uv run mlx-restart
```

ログは `run/Chat.log`、`run/Worker.log`、`run/Heavy.log`、`run/Wild.log` に出る。

ポート番号は固定。再起動時に別ポートへずらさない。既存の該当ポートの listener を停止し、同じポートで起動し直す。

health check 間隔は 300秒。health request timeout は 10秒。再起動時の起動待ち timeout は 600秒。

## Tool Calling

専用プロセス構成では未対応。Chat が詰まらないことと OpenAI互換 chat completions を優先する。

RenCrow 連携の最低要件には tool calling を含めない方針。
