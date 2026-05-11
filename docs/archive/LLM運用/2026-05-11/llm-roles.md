# LLM Roles

## Chat

Model: `/Users/yukimi/models/gemma-4-E4B-it-UD-MLX-4bit`

- 会話テンポ
- ルミナ人格
- 音声UI
- ユーザーとの自然なやり取り

## Worker

Model: `/Users/yukimi/models/Qwen3-VL-30B-A3B-Thinking-4bit`

- 実務処理
- 要約
- 整理
- RAG
- 通常の画像解析
- UI・資料・スクショ理解

## Coder

現行 proxy ではクライアントへ公開しない。
コード修正方針、影響範囲、テスト観点は `Worker` を使う。

## Heavy

Model: `/Users/yukimi/models/Qwen3.5-122B-A10B-4bit`

- 深考察
- 前提の見直し
- 失敗原因分析
- ローカル最終レビュー

## Wild

Model: `/Users/yukimi/models/Qwen3.6-35B-A3B-Abliterated-Heretic-MLX-4bit`

- 物語生成
- 画像プロンプト生成
- 創作用の画像解析
- 雰囲気・構図・衣装・質感の抽出
