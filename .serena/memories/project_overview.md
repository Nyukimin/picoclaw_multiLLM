# PicoClaw プロジェクト概要

## プロジェクトの目的
PicoClaw は超軽量なパーソナル AI アシスタントです。Go 言語で実装され、<10MB のメモリ使用量で $10 ハードウェア上でも動作することを目標としています。

## 主要機能
- **マルチチャネル対応**: Telegram、Discord、LINE、Slack、QQ、DingTalk、Feishu、WhatsApp、OneBot
- **マルチ LLM プロバイダー**: Ollama、OpenRouter、Anthropic、OpenAI、Zhipu、Gemini、Groq、DeepSeek など
- **ルーティング拡張**: Chat（Mio）、Worker（Shiro）、Coder（Aka/Ao/Gin）の役割分担
- **ツール**: Web 検索、ファイル操作、シェル実行、MCP統合、スケジュール管理（cron）、スキル管理
- **ヘルスチェックと自動復旧**: Ollama 監視と自動再起動
- **Worker即時実行**: Coderが生成したpatchをWorkerが自動実行（仕様策定済み、実装進行中）

## アーキテクチャ概要

### v3クリーンアーキテクチャ（移行中）
```
入力（LINE/Slack/etc.）
  → Adapter層（LINE Handler等）
  → Application層（MessageOrchestrator）
  → Domain層（Mio/Shiro/CoderAgent、Routing、Session）
  → Infrastructure層（LLM Providers、MCP、Tools）
  → 結果返却
```

**現状**:
- ✅ Domain層: 90%完成（agent, routing, patch, proposal, session, task）
- ✅ Infrastructure層: 95%完成（llm, mcp, tools, routing, persistence）
- ✅ Adapter層: 85%完成（config, line）
- ⚠️ Application層: 40%完成（orchestrator基本のみ、Worker実行ロジック未実装）

### レガシーアーキテクチャ（v2以前）
```
入力（LINE/Slack/etc.）
  → PicoClaw Gateway（受信/送信・セッション管理）
  → Router/LoopController（分岐・制約・回数管理）
  → ワーカー（Chat/Worker/Coder）
  → 入口へ返信
```

**状態**: pkg/agent/等に残存（main.goで未使用、削除候補）

## 技術スタック
- **言語**: Go 1.23
- **ビルドシステム**: Makefile、GoReleaser
- **デプロイ**: Docker Compose、systemd
- **パッケージ構造**: 
  - `cmd/picoclaw/` - メインエントリーポイント
  - `internal/` - v3クリーンアーキテクチャ実装（domain/application/adapter/infrastructure）
  - `pkg/` - レガシー実装（v2以前、削除候補）
- **テスト**: 全体カバレッジ 87.1%（internal/配下）

## ブランチ戦略
- **main**: 安定版（v2アーキテクチャ）
- **proposal/clean-architecture**: v3クリーンアーキテクチャ移行中（現在のブランチ）

## ドキュメント

### 正式仕様（実装の一次参照）
- **v3クリーンアーキテクチャ版**: `docs/01_正本仕様/実装仕様_v3_クリーンアーキテクチャ版.md` (3,067行、完成)
- **v2版**: `docs/01_正本仕様/実装仕様.md`
- **要件定義**: `docs/01_正本仕様/仕様.md`

### 実装ガイド
- **Worker即時実行設計**: `docs/06_実装ガイド進行管理/20260228_Worker即時実行ロジック設計.md` (774行)
- **承認フロー廃止プラン**: `docs/06_実装ガイド進行管理/20260228_承認フロー廃止プラン.md`
- **その他**: `docs/06_実装ガイド進行管理/`

### LLM運用
- **Coder3仕様**: `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md`
- **Worker仕様**: `docs/05_LLM運用プロンプト設計/LLM_Worker_Spec_v1_0.md`
- **DeepSeek運用**: `docs/05_LLM運用プロンプト設計/LLM_deepseek運用仕様.md`
- **Ollama管理**: `docs/05_LLM運用プロンプト設計/LLM_Ollama常駐管理.md`

### その他
- **分割仕様**: `docs/02_v2統合分割仕様/`
- **アーカイブ**: `docs/03_旧分割仕様アーカイブ/`

## 現在の実装状況（2026-03-02時点）

| カテゴリ | 完成度 | 詳細 |
|---------|--------|------|
| **インフラ基盤** | ✅ 95% | LLM/MCP/Tools/Config/Session |
| **ドメイン層** | ✅ 90% | Agent/Routing/Patch定義 |
| **アダプター層** | ✅ 85% | LINE統合、設定管理 |
| **アプリケーション層** | ⚠️ 40% | Orchestrator基本のみ、Worker実行ロジック欠如 |
| **統合フロー** | ❌ 10% | Coder→Worker自動連携が未実装 |
| **全体** | ⚠️ 約60% | 基盤は堅牢、核心機能が未完 |

**次のマイルストーン**: WorkerExecutionService実装（推定2-3日）
