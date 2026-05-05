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
| 創作 Wild | Wild | Qwen3.6 35B Heretic |

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
- model 名は `Chat` / `Worker` / `Wild` を使う。

MLX サーバ連携条件:

- endpoint は OpenAI 互換の `/v1/chat/completions` を使う。
- RenCrow 側の同時リクエスト数は 1 推奨。
- 初回ロードや初回モデル取得は遅くなるため、timeout は 120秒以上を推奨。
- warmup は起動後に `Chat` / `Worker` / `Wild` それぞれへ `max_tokens: 1` の短い request を送る。
- streaming や tool calling は、必要になった時点で MLX サーバ側の対応状況を確認する。

実装設定例:

```yaml
local_llm:
  enabled: true
  provider: local_openai
  base_url: "http://127.0.0.1:8080"
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

**モデル**: Qwen3.6 35B Heretic

主用途:

- 物語生成
- 画像プロンプト生成
- 創作用の画像解析
- 雰囲気・構図・衣装・質感の抽出

運用メモ:

- 創作寄りの発想、表現密度、雰囲気の抽出を優先する。
- 事実整理や通常業務よりも、物語・ビジュアル・演出の生成に使う。
- 画像解析でも、客観的なUI理解ではなく創作用の解釈や素材化を担当する。
