# LLM モデル役割メモ

**作成日**: 2026-05-04

この文書は、RenCrow で使う LLM の役割分担メモである。
正式な設定値ではなく、モデル選定・ルーティング・プロンプト調整時の運用目安として扱う。

## 現在方針

ローカルサーバへ切り替える。今回の呼び出し先は Ollama ではなく MLX。

| 役割 | エイリアス | モデル |
|------|--------|--------|
| Chat | Chat | Gemma E4B |
| 通常 Worker | Worker | Qwen3.6 35B 通常版 |
| 創作 Wild | Chat | Gemma E4B（現運用ではChatへ集約） |

基本ルール:

- エイリアスは従来どおり Chat / Worker を使う。
- Chat の呼び出し先を新Chatへ切り替える。
- Worker の呼び出し先を新Workerへ切り替える。
- 物語生成、画像プロンプト生成、創作寄りの画像解析などは Wild を使う。

呼び出し方式メモ:

- アプリ内部では `LLMProvider.Generate()` を呼ぶ。
- 現行実装の Chat / Worker 主経路は `cmd/picoclaw/main.go` で Ollama provider を直接生成している。
- MLX サーバは OpenAI 互換 API として接続する。
- RenCrow 側は MLX 専用 provider を新規実装しない。
- OpenAI 互換ローカル provider の base URL と model 名を切り替えて接続する。
- model 名は現運用では `Chat` / `Worker` を使う。`Wild` 専用モデルは未起動のため、Wild用途は `Chat` に集約する。

MLX サーバ連携条件:

- endpoint は OpenAI 互換の `/v1/chat/completions` を使う。
- 正しい呼び出し先は `http://192.168.1.31:8081/v1/chat/completions` model `Chat`、`http://192.168.1.31:8082/v1/chat/completions` model `Worker`。
- Chat / Worker 推論サーバは `/ready` を実装しない。クライアントは `http://<HOST>:8081/ready` や `http://<HOST>:8082/ready` を readiness 判定に使ってはいけない。
- 到達確認は、軽量な `POST /v1/chat/completions`（`max_tokens: 1`）またはサーバが提供する `GET /health` を使う。
- RenCrow 側の同時リクエスト数は 1 推奨。
- 初回ロードや初回モデル取得は遅くなるため、timeout は 120秒以上を推奨。
- `Chat` / `Worker` / `Wild` はrole別URLを設定できる。未設定roleは `base_url` へfallbackする。
- 現運用では `Wild` は専用ポートを持たず、`wild_base_url` は Chat と同じ `8081`、`wild_model` は `Chat` とする。
- warmup は起動後に各roleへ `max_tokens: 1` の短い request を送る。未起動roleがある場合、または Wild を Chat に集約している場合は重複warmupに注意する。
- streaming や tool calling は、必要になった時点で MLX サーバ側の対応状況を確認する。

2026-05-09現在の確認済み構成:

- Chat: `http://192.168.1.31:8081`, model `Chat`, 実体 `unsloth/gemma-4-E4B-it-UD-MLX-4bit`
- Worker: `http://192.168.1.31:8082`, model `Worker`, 実体 `mlx-community/Qwen3.6-35B-A3B-4bit`
- Wild: 専用プロセス未起動。現運用では `http://192.168.1.31:8081`, model `Chat` を使う。
- `8083` は Wild 専用プロセス用の予約扱いで、現運用では使わない。

現運用の設定例:

```yaml
local_llm:
  enabled: true
  provider: local_openai
  base_url: "http://192.168.1.31:8081"
  chat_base_url: "http://192.168.1.31:8081"
  worker_base_url: "http://192.168.1.31:8082"
  wild_base_url: "http://192.168.1.31:8081"
  chat_model: "Chat"
  worker_model: "Worker"
  wild_model: "Chat"
  timeout_sec: 120
  warmup: false
  global_concurrency: 2
  model_concurrency: 1
```

Wild 専用プロセスを起動した後の将来設定例:

```yaml
local_llm:
  enabled: true
  provider: local_openai
  base_url: "http://192.168.1.31:8081"
  chat_base_url: "http://192.168.1.31:8081"
  worker_base_url: "http://192.168.1.31:8082"
  wild_base_url: "http://192.168.1.31:8083"
  chat_model: "Chat"
  worker_model: "Worker"
  wild_model: "Wild"
  timeout_sec: 120
  warmup: true
  global_concurrency: 2
  model_concurrency: 1
```

`local_llm.enabled=true` の場合、Chat / Worker 主経路は Ollama ではなく OpenAI互換ローカル provider を使う。
Ollama設定は互換運用用に残すが、MLX運用では必須ではない。

## 1. Chat

**エイリアス**: Chat

**呼び出し先**: 新Chat

**モデル**: Gemma E4B

主用途:

- 会話テンポ
- ルミナ人格
- 音声UI
- ユーザーとの自然なやり取り

運用メモ:

- ユーザーが直接触れる応答品質を優先する。
- 短いやり取り、間、言い換え、音声で聞いたときの自然さを重視する。
- 実務処理や重い整理は Worker 側へ寄せる。

## 2. 通常 Worker

**エイリアス**: Worker

**呼び出し先**: 新Worker

**モデル**: Qwen3.6 35B 通常版

主用途:

- 実務処理
- 要約
- 整理
- RAG
- 通常の画像解析
- UI・資料・スクショ理解

運用メモ:

- 正確性、構造化、再利用しやすい出力を優先する。
- 会話の最終表現よりも、判断材料・要約・整理結果を Chat に返す役割。
- 一般的な視覚理解や資料読解はこちらに寄せる。

## 3. 創作 Wild

**エイリアス**: Wild

**モデル**: 現運用では `Chat`。将来、Wild専用プロセスを起動した場合のみ Qwen3.6 35B Heretic などへ切り替える。

主用途:

- 物語生成
- 画像プロンプト生成
- 創作用の画像解析
- 雰囲気・構図・衣装・質感の抽出

運用メモ:

- 創作寄りの発想、表現密度、雰囲気の抽出を優先する。
- 事実整理や通常業務よりも、物語・ビジュアル・演出の生成に使う。
- 画像解析でも、客観的なUI理解ではなく創作用の解釈や素材化を担当する。
