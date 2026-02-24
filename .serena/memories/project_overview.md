# PicoClaw プロジェクト概要

## プロジェクトの目的
PicoClaw は超軽量なパーソナル AI アシスタントです。Go 言語で実装され、<10MB のメモリ使用量で $10 ハードウェア上でも動作することを目標としています。

## 主要機能
- **マルチチャネル対応**: Telegram、Discord、LINE、Slack、QQ、DingTalk、Feishu、WhatsApp、OneBot
- **マルチ LLM プロバイダー**: Ollama、OpenRouter、Anthropic、OpenAI、Zhipu、Gemini、Groq、DeepSeek など
- **ルーティング拡張**: Chat（Mio）、Worker、Coder の役割分担
- **ツール**: Web 検索、ファイル操作、シェル実行、スケジュール管理（cron）、スキル管理
- **ヘルスチェックと自動復旧**: Ollama 監視と自動再起動

## アーキテクチャ概要
```
入力（LINE/Slack/etc.）
  → PicoClaw Gateway（受信/送信・セッション管理）
  → Router/LoopController（分岐・制約・回数管理）
  → ワーカー（Chat/Worker/Coder）
  → 入口へ返信
```

## 技術スタック
- **言語**: Go 1.25.7
- **ビルドシステム**: Makefile、GoReleaser
- **デプロイ**: Docker Compose、systemd
- **パッケージ構造**: cmd/（エントリーポイント）、pkg/（ライブラリ）

## ドキュメント
- **正式仕様**: `docs/01_正本仕様/実装仕様.md`
- **要件定義**: `docs/01_正本仕様/仕様.md`
- **分割仕様**: `docs/02_v2統合分割仕様/`
- **LLM 運用**: `docs/05_LLM運用プロンプト設計/`
- **実装ガイド**: `docs/06_実装ガイド進行管理/`
