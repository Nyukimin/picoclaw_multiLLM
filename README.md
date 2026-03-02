# PicoClaw - 超軽量AIアシスタント（v3 Clean Architecture）

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Test Coverage](https://img.shields.io/badge/coverage-83.6%25-brightgreen)](https://github.com/Nyukimin/picoclaw_multiLLM)
[![Architecture](https://img.shields.io/badge/architecture-Clean%20Architecture-blue)](docs/実装仕様_v3.md)

> **メモリ使用量 <10MB で動作する、マルチLLMルーティング対応の超軽量AIアシスタント**

---

## 📋 目次

- [概要](#-概要)
- [主要機能](#-主要機能)
- [アーキテクチャ](#-アーキテクチャ)
- [実装状況](#-実装状況)
- [クイックスタート](#-クイックスタート)
- [設定](#-設定)
- [開発](#-開発)
- [ドキュメント](#-ドキュメント)
- [貢献](#-貢献)
- [ライセンス](#-ライセンス)

---

## 🎯 概要

**PicoClaw** は、Go言語で実装された超軽量なパーソナルAIアシスタントです。

### 特徴

- **超軽量**: メモリ使用量 <10MB、$10デバイスでも動作
- **マルチLLM対応**: Ollama、Claude、DeepSeek、OpenAI等を統合
- **インテリジェントルーティング**: Chat（会話）/ Worker（実行）/ Coder（設計・実装）の自動振り分け
- **Worker即時実行**: Coderが生成したpatchをWorkerが自動実行（承認フロー廃止）
- **Clean Architecture**: v3.0クリーンアーキテクチャで保守性向上
- **高テストカバレッジ**: internal/配下 83.6%（Domain層 93.5%）

### 技術スタック

- **言語**: Go 1.23
- **アーキテクチャ**: Clean Architecture（4層構造）
- **LLMプロバイダー**: Ollama, Anthropic Claude, DeepSeek, OpenAI
- **チャネル**: LINE, Slack, Telegram, Discord等（計画）
- **ツール**: Web検索、ファイル操作、シェル実行、MCP統合

---

## ✨ 主要機能

### 1. マルチLLMルーティング

PicoClawは、タスクの種類に応じて最適なLLMを自動選択します：

| 役割 | 愛称 | LLM | 責務 |
|------|------|-----|------|
| **Chat** | Mio | Ollama (chat-v1) | 会話、意思決定、ルーティング判定 |
| **Worker** | Shiro | Ollama (worker-v1) | ファイル操作、コマンド実行、差分適用 |
| **Coder1** | Aka | DeepSeek | 仕様設計、アーキテクチャ検討 |
| **Coder2** | Ao | OpenAI | 実装、コード生成 |
| **Coder3** | Gin | Anthropic Claude | 高品質コーディング、推論 |

**ルーティングカテゴリ**:
- `CHAT` - 会話・意思決定
- `PLAN` - 計画策定
- `ANALYZE` - 分析
- `OPS` - 運用操作
- `RESEARCH` - 調査
- `CODE` / `CODE1` / `CODE2` / `CODE3` - コーディング

### 2. Worker即時実行（v3.0新機能）

Coder3（Claude）が生成したProposal（plan + patch）をWorkerが即座に実行します：

```
ユーザー指示 → ルーティング → Coder3がProposal生成
  → WorkerExecutionService.ExecuteProposal()
  → Git auto-commit（オプション）
  → 実行結果返却
```

**セーフガード機能**:
- ✅ Git auto-commit（変更を自動コミット、ロールバック可能）
- ✅ 保護ファイルパターン（.env*, *credentials*, *.key, *.pem）
- ✅ 実行前サマリ表示（コマンド数・種別を表示）
- ✅ Workspace制限（workspace外への書き込み禁止）
- ✅ エラーハンドリング（StopOnError/ContinueOnError）

**サポートする操作**:
- ファイル操作: create, update, delete, append, mkdir, rename, copy
- シェルコマンド実行（タイムアウト・Env対応）
- Git操作（commit, push等）

### 3. ヘルスチェックと自動復旧

- Ollama常駐監視（`keep_alive: -1`）
- ヘルスチェックによる自動再起動
- MaxContext制約チェック（8192）

### 4. セッション管理

- 日次カットオーバー
- メモリ管理
- ログ保存（構造化ログ、Obsidian連携）

---

## 🏗️ アーキテクチャ

### v3.0 Clean Architecture（現在のブランチ: proposal/clean-architecture）

```
入力（LINE/Slack/etc.）
  ↓
┌─────────────────────────────────────────────┐
│ Adapter層（LINE Handler等）                 │
│ - config/, line/                            │
└─────────────────────────────────────────────┘
  ↓
┌─────────────────────────────────────────────┐
│ Application層（MessageOrchestrator）        │
│ - orchestrator/, service/                   │
│ - WorkerExecutionService（Worker即時実行）  │
└─────────────────────────────────────────────┘
  ↓
┌─────────────────────────────────────────────┐
│ Domain層（Mio/Shiro/CoderAgent等）          │
│ - agent/, routing/, patch/, proposal/      │
│ - session/, task/                           │
└─────────────────────────────────────────────┘
  ↓
┌─────────────────────────────────────────────┐
│ Infrastructure層（LLM/MCP/Tools）           │
│ - llm/ (claude, deepseek, ollama, openai)  │
│ - mcp/, tools/, routing/, persistence/     │
└─────────────────────────────────────────────┘
  ↓
結果返却
```

**パッケージ構成**:
```
picoclaw/
├── cmd/picoclaw/              # メインエントリーポイント
├── internal/                  # v3クリーンアーキテクチャ実装
│   ├── adapter/               # 外部I/F（config, line）
│   ├── application/           # ユースケース（orchestrator, service）
│   ├── domain/                # ドメインロジック（agent, routing等）
│   └── infrastructure/        # 外部システム（llm, mcp, tools）
├── pkg/                       # レガシー実装（v2以前、削除候補）
├── docs/                      # ドキュメント
│   ├── 仕様.md                # 要件定義
│   ├── 実装仕様_v3.md         # v3実装仕様（3,067行）
│   ├── LLM運用/               # LLM運用仕様
│   └── archive/               # アーカイブ
└── config.yaml.example        # 設定例
```

---

## 📊 実装状況

**ブランチ**: `proposal/clean-architecture`（v3.0実装中）

| カテゴリ | 完成度 | 詳細 |
|---------|--------|------|
| **承認フロー廃止** | ✅ 100% | pkg/approval/ 削除完了 |
| **Worker即時実行** | ✅ 100% | WorkerExecutionService実装完了 |
| **Coder→Worker統合** | ✅ 100% | MessageOrchestrator統合完成 |
| **Infrastructure層** | ✅ 95% | LLM/MCP/Tools/Config/Session |
| **Domain層** | ✅ 90% | Agent/Routing/Patch定義 |
| **Adapter層** | ✅ 85% | LINE統合、設定管理 |
| **Application層** | ✅ 70% | Orchestrator、Worker実行ロジック |
| **全体** | ✅ 87% | 核心機能100%完成 |

**テストカバレッジ**:
- **internal/全体**: 83.6% ✅
- Config: 94.6%
- Domain層: 平均93.5%
- Infrastructure層: 平均87.2%
- LINE Adapter: 85.9%
- Orchestrator: 70.0%
- Service: 65.4%

**最近の主要実装**（2026-03-02完了）:
- ✅ 承認フロー全廃（pkg/approval/ 削除、~590行）
- ✅ Worker即時実行（WorkerExecutionService、390行 + テスト651行）
- ✅ Coder3統合（CODE3ルート、Proposal → Worker自動連携）
- ✅ セーフガード実装（保護ファイル、workspace制限等）
- ✅ Git auto-commit対応
- ✅ ドキュメント再編成（docs/ シンプル化）

---

## 🚀 クイックスタート

### 前提条件

- Go 1.23以降
- Ollama（chat-v1、worker-v1モデル）
- API キー（Anthropic/DeepSeek/OpenAI等、オプション）

### 1. インストール

```bash
# リポジトリクローン
git clone https://github.com/Nyukimin/picoclaw_multiLLM.git
cd picoclaw_multiLLM

# ブランチ切り替え（v3クリーンアーキテクチャ版）
git checkout proposal/clean-architecture

# 依存関係インストール
make deps

# ビルド
make build

# または直接ビルド
go build -o picoclaw ./cmd/picoclaw
```

### 2. Ollama モデル準備

```bash
# Ollamaインストール（未インストールの場合）
curl -fsSL https://ollama.com/install.sh | sh

# モデルダウンロード
ollama pull chat-v1       # Chat（Mio）用
ollama pull worker-v1     # Worker（Shiro）用

# 常駐化（keep_alive: -1）
ollama run chat-v1 --keep-alive -1
ollama run worker-v1 --keep-alive -1
```

### 3. 設定ファイル作成

```bash
# 設定例をコピー
cp config.yaml.example config.yaml

# API キーを環境変数に設定
export ANTHROPIC_API_KEY="your-claude-api-key"    # Coder3用（オプション）
export DEEPSEEK_API_KEY="your-deepseek-api-key"  # Coder1用（オプション）
export OPENAI_API_KEY="your-openai-api-key"      # Coder2用（オプション）
```

### 4. 実行

```bash
# サーバー起動
./picoclaw

# または
go run ./cmd/picoclaw
```

サーバーは `http://0.0.0.0:8080` で起動します。

---

## ⚙️ 設定

### config.yaml 基本設定

```yaml
server:
  port: 8080
  host: "0.0.0.0"

ollama:
  base_url: "http://localhost:11434"
  chat_model: "chat-v1"
  worker_model: "worker-v1"

claude:
  # API Key は環境変数 ANTHROPIC_API_KEY から読み込み
  model: "claude-sonnet-4-20250514"

deepseek:
  # API Key は環境変数 DEEPSEEK_API_KEY から読み込み
  model: "deepseek-chat"

openai:
  # API Key は環境変数 OPENAI_API_KEY から読み込み
  model: "gpt-4o-mini"

session:
  storage_dir: "./data/sessions"

log:
  level: "info"
  format: "json"
```

### Worker実行設定（重要）

```yaml
worker:
  # Git auto-commit（実行前後に自動コミット）
  auto_commit: false
  commit_message_prefix: "[Worker Auto-Commit]"

  # タイムアウト設定（秒）
  command_timeout: 300  # シェルコマンド実行タイムアウト（5分）
  git_timeout: 30       # Git操作タイムアウト（30秒）

  # エラーハンドリング
  stop_on_error: false  # false=継続モード、true=中断モード

  # ワークスペース設定
  workspace: "."  # Patch実行のルートディレクトリ

  # 保護ファイルパターン（機密情報保護）
  protected_patterns:
    - ".env*"
    - "*credentials*"
    - "*.key"
    - "*.pem"

  # 保護ファイル検出時の動作
  action_on_protected: "error"  # "error"=エラー停止、"skip"=スキップ、"log"=警告ログのみ

  # 実行前サマリ表示
  show_execution_summary: true  # 実行前にコマンド数・種別を表示
```

### API キー設定

**環境変数で設定（推奨）**:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
export DEEPSEEK_API_KEY="sk-..."
export OPENAI_API_KEY="sk-..."
```

---

## 💻 開発

### ビルド

```bash
# 開発ビルド
make build

# 全プラットフォーム向けビルド
make build-all

# インストール
make install
```

### テスト

```bash
# 全テスト実行
make test

# カバレッジ確認
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### ディレクトリ構成

```
picoclaw/
├── cmd/picoclaw/                      # メインアプリケーション
│   └── main.go                        # エントリーポイント（DI設定）
├── internal/                          # v3クリーンアーキテクチャ
│   ├── adapter/                       # Adapter層
│   │   ├── config/                    # 設定管理
│   │   └── line/                      # LINE統合
│   ├── application/                   # Application層
│   │   ├── orchestrator/              # メッセージオーケストレーター
│   │   └── service/                   # Worker実行サービス
│   ├── domain/                        # Domain層
│   │   ├── agent/                     # エージェント（Mio/Shiro/Coder）
│   │   ├── llm/                       # LLMインターフェース
│   │   ├── patch/                     # Patch定義（7種の操作）
│   │   ├── proposal/                  # Proposal定義（plan/patch/risk）
│   │   ├── routing/                   # ルーティング
│   │   ├── session/                   # セッション
│   │   └── task/                      # タスク
│   └── infrastructure/                # Infrastructure層
│       ├── llm/                       # LLMプロバイダー実装
│       │   ├── claude/
│       │   ├── deepseek/
│       │   ├── ollama/
│       │   └── openai/
│       ├── mcp/                       # MCP統合
│       ├── persistence/               # 永続化
│       ├── routing/                   # ルーティング実装
│       └── tools/                     # ツール実装
├── pkg/                               # レガシー実装（v2以前）
├── docs/                              # ドキュメント
│   ├── 仕様.md                        # 要件定義
│   ├── 実装仕様_v3.md                 # v3実装仕様（3,067行）
│   ├── LLM運用/                       # LLM運用仕様
│   │   ├── Coder3_Claude_API仕様.md
│   │   ├── LLM_Ollama常駐管理.md
│   │   └── LLM_Worker_Spec_v1_0.md
│   └── archive/                       # アーカイブ
├── config.yaml.example                # 設定例
├── Makefile                           # ビルドファイル
└── README.md                          # このファイル
```

---

## 📚 ドキュメント

### 正本仕様（実装の一次参照）

- **[docs/仕様.md](docs/仕様.md)** - 要件定義（286行）
- **[docs/実装仕様_v3.md](docs/実装仕様_v3.md)** - v3クリーンアーキテクチャ版（3,067行）

### LLM運用

- **[docs/LLM運用/Coder3_Claude_API仕様.md](docs/LLM運用/Coder3_Claude_API仕様.md)** - Coder3仕様
- **[docs/LLM運用/LLM_Worker_Spec_v1_0.md](docs/LLM運用/LLM_Worker_Spec_v1_0.md)** - Worker仕様
- **[docs/LLM運用/LLM_Ollama常駐管理.md](docs/LLM運用/LLM_Ollama常駐管理.md)** - Ollama管理

### プロジェクトルール

- **[CLAUDE.md](CLAUDE.md)** - AI開発ルール、プロジェクト固有ルール

### その他

- **[docs/README.md](docs/README.md)** - ドキュメント一覧
- **[docs/archive/](docs/archive/)** - 旧ドキュメント（参考資料）

---

## 🤝 貢献

プルリクエスト歓迎！以下のガイドラインに従ってください：

### 開発フロー

1. **仕様確認**: `docs/実装仕様_v3.md` を読む
2. **ブランチ作成**: `feature/xxx` または `fix/xxx`
3. **実装**: コーディング規約に従う
4. **テスト**: ユニットテスト・統合テストを追加
5. **ドキュメント更新**: 必要に応じて `docs/実装仕様_v3.md` を更新
6. **プルリクエスト**: `proposal/clean-architecture` ブランチへ

### コーディング規約

- Go標準のコーディングスタイル（`gofmt`, `go vet`）
- Clean Architectureの原則を尊重
- テストカバレッジ: 新規コードは70%以上
- コミットメッセージ: [Conventional Commits](https://www.conventionalcommits.org/)

### テスト

```bash
# 全テスト実行
go test ./...

# カバレッジ確認
go test ./internal/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 📄 ライセンス

MIT License

---

## 🎯 次のマイルストーン

### Phase 6: mainブランチへのマージ（計画中）

- [ ] プルリクエスト作成
- [ ] コードレビュー
- [ ] 統合テスト
- [ ] mainブランチへのマージ

### Phase 7: リリース準備（計画中）

- [ ] リリースノート作成
- [ ] タグ作成（v3.0.0）
- [ ] バイナリビルド
- [ ] ドキュメント最終確認

### 将来の計画

- [ ] Slack統合
- [ ] Telegram統合
- [ ] Discord統合
- [ ] MCP統合の拡張
- [ ] スキル管理機能
- [ ] Web UI

---

## 💡 使用例

### LINEから実行

```
ユーザー: /code3 pkg/test/hello.go に Hello World を出力する関数を追加して
```

**期待される動作**:
1. Coder3（Claude）がProposal生成（plan/patch/risk）
2. WorkerがPatch即時実行
3. （auto_commit=trueの場合）Git自動コミット
4. 実行結果返信

---

## 🐛 トラブルシューティング

### Ollamaモデルが見つからない

```bash
# モデル一覧確認
ollama list

# モデルダウンロード
ollama pull chat-v1
ollama pull worker-v1
```

### Worker実行が失敗する

1. Git auto-commit設定確認: `config.yaml` の `worker.auto_commit`
2. Workspace設定確認: `worker.workspace`
3. ログ確認: 標準出力の `[Worker]` プレフィックス行

### ロールバックが必要な場合

```bash
# 最新のコミットを確認
git log --oneline -5 | grep "Worker Auto-Commit"

# ロールバック
git reset --hard HEAD~1

# 特定のコミットに戻る
git reset --hard <commit-hash>
```

---

## 📞 サポート

- **Issue**: [GitHub Issues](https://github.com/Nyukimin/picoclaw_multiLLM/issues)
- **ドキュメント**: [docs/](docs/)
- **仕様**: [docs/実装仕様_v3.md](docs/実装仕様_v3.md)

---

**PicoClaw v3.0** - Clean Architecture for Ultra-Lightweight AI Assistant
