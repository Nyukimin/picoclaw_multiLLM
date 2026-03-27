# RenCrow ドキュメント

**最終更新**: 2026-03-26

---

## ディレクトリ構成

```
docs/
├── README.md                        # この索引
├── 01_正本仕様/                     # v3.0 Clean Architecture 実装仕様（正本）
├── 02_OpenClaw移植詳細仕様/         # OpenClaw 移植の分割詳細仕様
├── 03_設計文書/                     # 設計思想・要件定義・アーキテクチャ
├── 04_実装仕様_機能拡張/            # v4 以降の機能別実装仕様
├── 05_スナップショット/              # 日付付き現況整理（参照用）
├── 06_実装ガイド進行管理/           # 実装手順・進行記録
├── 07_IdleChat仕様/                 # IdleChat サブシステム仕様
├── LLM運用/                        # LLM 別運用仕様
├── TTS仕様/                        # TTS 仕様
├── VTS仕様/                        # VTS 仕様
├── tooling/                        # ツール開発ガイド
└── archive/                        # 旧仕様（読み取り専用）
```

---

## 仕様体系の全体像

```
[統合仕様（起点）]
  01_正本仕様/仕様.md  ← 全体理解はここから

[基盤思想]                            [実装仕様]
  TOOL_CONTRACT.md (リポジトリルート)   01_正本仕様/実装仕様.md (CA 詳細)
  03_設計文書/会話LLM仕様_v1.0.md      04_実装仕様_機能拡張/
  03_設計文書/拡張設計仕様.md

[設計文書]                            [運用仕様]
  03_設計文書/                         04_実装仕様_機能拡張/実装仕様_OpenClaw移植_v1.md
    Chat_Worker_Coder_アーキテクチャ    02_OpenClaw移植詳細仕様/
    仕様_エージェント構成               LLM運用/
```

### 読む順序

| 目的 | 最初に読む | 次に読む |
|------|----------|---------|
| 全体理解 | **01_正本仕様/仕様.md**（統合仕様） | 03_設計文書/Chat_Worker_Coder_アーキテクチャ.md |
| 実装作業 | 01_正本仕様/実装仕様.md | 04_実装仕様_機能拡張/ 配下の対象仕様 |
| 機能追加 | 04_実装仕様_機能拡張/実装仕様_OpenClaw移植_v1.md | TOOL_CONTRACT.md |
| 会話システム | 03_設計文書/会話LLM仕様_v1.0.md | 04_実装仕様_機能拡張/実装仕様_会話LLM_v5.md → v5.1 |
| データ基盤拡張 | 03_設計文書/拡張設計仕様.md | -- |
| IdleChat | 07_IdleChat仕様/IdleChat仕様.md | 07_IdleChat仕様/ 配下の各仕様 |

---

## 1. 基盤思想（設計原則・契約）

実装の前提となる不変のルール。コードより先に存在し、全仕様に優先する。

### 1.1 TOOL_CONTRACT.md（リポジトリルート）

**ツール契約** -- Coder が作るツールの入出力・安全・品質の根本ルール。

| 内容 | 要点 |
|------|------|
| 入出力の統一 | JSON一次経路、stdout=結果/stderr=ログ |
| 安全レール | dry-run必須、入力バリデーション、フィールド制限 |
| 予測可能性 | 非対話、タイムアウト固定、無限待ち禁止 |
| 増殖耐性 | tool_id+version、単一責務、SKILL同梱、廃止宣言 |
| DoD（完成条件） | 6項目チェックリスト |
| 受領フロー | Worker がゲートキーパー |

### 1.2 03_設計文書/会話LLM仕様_v1.0.md

**会話の設計原則** -- 時間軸で育つ対話を成立させるためのシステム構造。

| 内容 | 要点 |
|------|------|
| 会話の単位 | Message / Turn / Thread / Session / Conversation |
| 記憶レイヤー | 短期(RAM) / 中期(Redis→DuckDB) / 長期(VectorDB) / KB / Persona / UserProfile |
| 責務分離 | Chat=見た目 / Worker=想起+判断+記録 / Coder=実装 |
| 処理フロー | 入力→想起→判断→生成→記録（Spawn禁止、同期のみ） |
| JSON I/F | Chat→Worker / Worker→Coder / Worker→Memory の3契約 |
| 実装状況 | 付録Aに30項目の照合表（73%実装済み） |

### 1.3 03_設計文書/拡張設計仕様.md

**データ基盤の拡張ルール** -- エンタメDB（Core/Domain/Relations/Similarities）の成長戦略。

| 内容 | 要点 |
|------|------|
| 4原則 | Core安定 / Relations-Similarities分離 / TopKのみ / entity_id不変 |
| カテゴリ追加 | 6項目チェックリスト（A〜F） |
| 情報源追加 | entity_id中心の統合、provenance記録、コンフリクト解決 |
| Embedding | model別複数保存、metric命名規約、再計算ジョブ |
| クロスメディア | パターンA(Relations) → パターンB(IP上位エンティティ) |
| スケール | TopK O(N*K)、増分更新+定期リビルド |

---

## 2. 正本仕様（実装の一次参照）

実装時に直接参照する仕様書。変更がある場合は仕様を先に更新してから実装する。

### 2.0 01_正本仕様/仕様.md（統合仕様）★ 全体理解の起点

すべての仕様を整合性を持ってまとめた**マスター仕様書**。設計原則・エージェント定義・ルーティング・物理構成・IdleChat・実装状況・文書インデックスを一箇所で参照できる。

初めて全体を把握したい場合・仕様の矛盾を確認したい場合はここを読む。

**セクション別仕様**（各 §の詳細を独立ファイルに整理）:

| ファイル | 対応 §| 内容 |
|---------|------|------|
| `01_正本仕様/02_設計原則.md` | §2 | 責務三分割・CA4層・DDD・セーフガード |
| `01_正本仕様/03_エージェント定義.md` | §3 | Mio/Shiro/Coder の構造体・設定・ディスパッチ |
| `01_正本仕様/04_ルーティング.md` | §4 | v6.1 決定フロー・RuleDictionary・ループ制御 |
| `01_正本仕様/05_物理構成.md` | §5 | 3ノード・SSH Transport・LLM パラメータ |
| `01_正本仕様/06_会話エンジン.md` | §6 | 4層記憶・ConversationEngine・RecallPack |
| `01_正本仕様/07_機能拡張.md` | §7 | SubagentManager（実装済み）・AgentPersona・CapabilityAdaptation |
| `01_正本仕様/08_IdleChat.md` | §8 | 通常/未来展望/ストーリーモード・トピック戦略 |
| `01_正本仕様/09_セキュリティ.md` | §9 | API キー管理・クラウド制限・Worker セーフガード |
| `01_正本仕様/10_ログ.md` | §10 | 構造化ログ・必須フィールド・マスキング |

### 2.1 現況確認用

| ファイル | 内容 | 位置づけ |
|------|------|------|
| `05_スナップショット/現状実装仕様_20260319.md` | 2026-03-19 時点のコード実装ベース現況整理 | 現在値の確認用 |

### 2.2 03_設計文書/仕様.md（要件定義）

プロジェクトの目的、用語、ルーティングカテゴリ、セキュリティ、セッション、ログの要件。

### 2.3 01_正本仕様/実装仕様.md（Clean Architecture）-- 3,265行

v3.0 の完全実装仕様。**全実装の基盤**。旧コピー（`実装仕様_v3.md`）は `archive/11_旧仕様_20260326/` に移動済み。

| 内容 | 状態 |
|------|------|
| Clean Architecture 4層構造 | 実装完了 |
| Worker即時実行（Worker即時実行化） | 実装完了 |
| Domain/Application/Infrastructure/Adapter | 実装完了 |
| テストカバレッジ 87.1% | 達成済み |

### 2.3 04_実装仕様_機能拡張/実装仕様_分散実行_v4.md -- 2,334行

v3.0 の上に分散実行機能を追加する仕様。

| 内容 | 状態 |
|------|------|
| Transport層（Local/SSH） | 実装完了 |
| picoclaw-agent（スタンドアロン） | 実装完了 |
| DistributedOrchestrator | 実装完了 |
| 本番有効化 | Claude クレジット補充待ち |

### 2.4 04_実装仕様_機能拡張/実装仕様_会話LLM_v5.md

3層記憶インフラ（Redis/DuckDB/Qdrant）の実装仕様。

| 内容 | 状態 |
|------|------|
| Phase 1: ドメイン層 | 実装完了 |
| Phase 2: 3層ストア | 実装完了 |
| Phase 3: Embedder/Summarizer | 実装完了 |
| 統合テスト 9件 | 全通過 |

### 2.5 04_実装仕様_機能拡張/実装仕様_会話エンジン_v5.1.md

ConversationEngine（RecallPack + Persona）の実装仕様。

| 内容 | 状態 |
|------|------|
| ConversationEngine I/F | 実装完了 |
| RecallPack 生成 | 実装完了 |
| Persona 注入 | 実装完了 |
| Thread 自動判定 | 実装完了（best-effort） |
| UserProfile 自動抽出 | 実装完了（in-memory / best-effort） |

### 2.6 04_実装仕様_機能拡張/実装仕様_チャネル拡張_v1.md

Discord / Slack / 音声入出力のアダプター追加仕様。

| 内容 | 状態 |
|------|------|
| ChannelAdapter 共通インターフェース | 実装完了（HTTP/Webhook基盤） |
| Discord アダプター (WebSocket Gateway) | 部分実装（Webhook/Interaction） |
| Slack アダプター (Socket Mode) | 部分実装（Events API） |
| 音声アダプター (STT + TTS) | 部分実装（TTS/Audio Router、STT未実装） |
| セッション ID 規約（チャネル横断） | 実装完了 |
| 設定ファイル拡張 (channels) | 実装完了（Telegram/Discord/Slack） |

### 2.7 04_実装仕様_機能拡張/実装仕様_OpenClaw移植_v1.md

OpenClawの実装実行能力をGo基盤へ段階移植するための仕様。

| 内容 | 状態 |
|------|------|
| Execution Contract（依頼→実行契約） | 部分実装（正規化/検証） |
| Autonomous Executor（Plan→Apply→Verify→Repair） | 部分実装（最小ループ / `/entry` 経路） |
| TTS Capability Pack（OpenAI→ElevenLabs→local） | 部分実装（TTS運用系あり、正本仕様は整理中） |
| Evidence（execution_report） | 部分実装（Execution監査ログ） |

### 2.8 04_実装仕様_機能拡張/実装仕様_ケイパビリティ適応_v1.md（設計段階）

単一ソース・ケイパビリティ適応型エージェントの実装仕様。v4.0 分散実行の上に構築。

| 内容 | 状態 |
|---|---|
| NodeCapabilities 起動時検出（OS/メモリ/LLM疎通） | 未実装 |
| ケイパビリティベース LLM ルーティング（静的配線から動的選択へ） | 未実装 |
| ToolRegistry（Shiro生成ツールの永続化・共有） | 未実装 |
| Shiro → Coder ツール共有フロー（プラットフォームフィルタ・承認） | 未実装 |
| クロスプラットフォームバイナリ戦略（Makefile + ビルドタグ） | 未実装 |

### 2.9 04_実装仕様_機能拡張/実装仕様_エージェントペルソナ_v1.md（設計段階）

Shiro/Coder に軽量なペルソナ（AgentPersona）とインメモリ短期記憶（LightMemory）を付与する実装仕様。

| 内容 | 状態 |
|------|------|
| AgentPersona 型（domain layer） | 未実装 |
| LightMemory 型（インメモリ FIFO） | 未実装 |
| config.yaml `agents:` セクション拡張 | 未実装 |
| ShiroAgent / CoderAgent への Builder 注入 | 未実装 |
| IdleChat personalities との統合（Phase 1） | 未実装 |

### 2.10 02_OpenClaw移植詳細仕様/（分割仕様）

| ファイル | 内容 | 状態 |
|---------|------|------|
| 詳細実装仕様_02_チャネル網羅不足.md | Telegram/Discord/Slack追加と共通イベント契約 | 現行実装ベース |
| 詳細実装仕様_03_Tools体系の差.md | ToolManifest/Registry/ExecutionEnvelope | 現行実装ベース |
| 詳細実装仕様_04_Nodes_デバイス能力の差.md | NodeCapabilityと要件ベース選定 | 現行実装ベース |
| 詳細実装仕様_05_Gateway_Ops_CLIの差.md | gateway/channels/status/health/doctor/logs | 現行実装ベース |
| 詳細実装仕様_06_Security_Sandboxの差.md | SecurityProfileと権限スコープ・監査 | 現行実装ベース |
| 詳細実装仕様_07_App_Platform導線の差.md | Unified Entryと進行イベント統一 | 現行実装ベース |

補助資料:
- `OpenClaw機能差分比較表_20260310.md`（OpenClawとの機能差分サマリ）

---

## 3. 設計文書（03_設計文書/）

### 3.1 Chat_Worker_Coder_アーキテクチャ.md

Chat/Worker/Coder の役割・責務・指揮命令系統。分散実行の設計思想を含む。

### 3.2 仕様_エージェント構成.md

RenCraw を複数の物理機体上で分散動作させる際の、Chat / Worker / Coder の責務分離・物理配置・自己拡張フローを定義する構成仕様。（著: ルミナ, v0.1）

### 3.3 ログViewer仕様.md

Viewer のタブ構成、SSE、evidence API、IdleChat 制御を含む運用/UI 仕様。

### 3.4 実装仕様_ログViewer_v1.md

ログViewerの実装責務、EventHub、SSE、evidence、IdleChat連携、および目標仕様との差分を整理した実装仕様。

### 3.5 実装仕様_操作ログJSON保持_v1.md

Chat / Worker / Coder の操作ログを JSONL で永続化し、TTL で削除するための実装仕様。

### 3.6 KB運用ガイド.md

Knowledge Base の運用方針・スキーマ定義・更新手順。

---

## 4. 機能仕様（07_IdleChat仕様/）

### 4.1 IdleChat仕様.md

IdleChat の全体仕様。セッション構造、ブレイク体系、TTS/Viewer 連携、ループ検出、要約読み上げの共通仕様。

### 4.2 未来展望セッション仕様.md

IdleChat の未来展望モード（Forecast Session）仕様。6ドメインを順に回しトレンドから未来展望を議論する番組形式。Google Trends / Reddit / はてブ / NHK RSS によるトピック生成パイプラインを含む。

### 4.3 実装仕様_ストーリーモード_v1.md

IdleChat のストーリーモード実装仕様。8ステップパイプライン（Step 1〜6 決定論的、Step 7〜8 LLM）、5改変スタイル、17アクティブ作品のコーパス、品質検証・フォールバック階層を含む。

---

## 5. 運用仕様

### 5.1 運用ガイド/

| ファイル | 内容 |
|---------|------|
| **Coder設定ガイド.md** | Coder1-4 の設定方法（API キー取得、環境変数、SSH 分散実行、トラブルシューティング）★ユーザー必読 |
| 分散実行_前提条件とセットアップ.md | SSH 分散実行の詳細セットアップ手順 |

### 5.2 LLM運用/

| ファイル | 内容 |
|---------|------|
| Coder3_Claude_API仕様.md | Claude API 運用、Proposal生成 |
| LLM_Worker_Spec_v1_0.md | Worker（Shiro）の仕様 |
| LLM_Ollama常駐管理.md | Ollama 常駐管理、ヘルスチェック |

### 5.4 06_実装ガイド進行管理/

| ファイル | 内容 |
|---------|------|
| 20260309_OpenClaw移植_runbook.md | OpenClaw移植の実機検証手順（E2E実再生完了判定） |
| 20260317_idlechat_story_tuning_memo.md | IdleChat ストーリーモード調整メモ |
| 20260321_ストーリーモード_仕様と実装状況.md | ストーリーモード仕様と実装状況の照合 |

### 5.5 05_スナップショット/

| ファイル | 内容 |
|---------|------|
| 現状実装仕様_20260319.md | 2026-03-19 時点のコード実装ベース現況 |
| ログViewer現行仕様サマリ_20260319.md | Viewer 運用タブ・API の一枚まとめ |

---

## 6. 仕様間の依存関係

```
01_正本仕様/仕様.md（統合仕様・全体索引）
  |
  +-- 03_設計文書/仕様.md（要件）
  |
  +-- 01_正本仕様/実装仕様.md（CA基盤）
  |     |
  |     +-- 04_実装仕様_機能拡張/実装仕様_分散実行_v4.md（v3の上に追加）
  |     |
  |     +-- 04_実装仕様_機能拡張/実装仕様_会話LLM_v5.md（3層記憶インフラ）
  |     |     |
  |     |     +-- 04_実装仕様_機能拡張/実装仕様_会話エンジン_v5.1.md（RecallPack + Persona）
  |     |
  |     +-- 04_実装仕様_機能拡張/実装仕様_チャネル拡張_v1.md（Discord/Slack/音声）
  |     |
  |     +-- 04_実装仕様_機能拡張/実装仕様_分散実行_v4.md
  |           |
  |           +-- 04_実装仕様_機能拡張/実装仕様_ケイパビリティ適応_v1.md
  |                 |
  |                 +-- 04_実装仕様_機能拡張/実装仕様_サブエージェント_v1.md（ReActLoop、前提）
  |
  +-- 03_設計文書/Chat_Worker_Coder_アーキテクチャ.md（設計思想）
  |
  +-- 04_実装仕様_機能拡張/実装仕様_OpenClaw移植_v1.md（OpenClaw移植の正本）

03_設計文書/会話LLM仕様_v1.0.md（設計原則）
  |
  +-- 04_実装仕様_機能拡張/実装仕様_会話LLM_v5.md + v5.1 の上位思想

03_設計文書/拡張設計仕様.md（データ基盤）
  |
  +-- RenCrow 外のエンタメDB基盤に適用

TOOL_CONTRACT.md（ツール契約）
  |
  +-- 全ツール実装に適用（Coder/Worker が参照）
```

---

## 7. アーカイブ（archive/）

`archive/` 配下のドキュメントは参考資料。直接編集しない。

| ディレクトリ | 内容 |
|------------|------|
| 01_正本仕様_v2/ | v2実装仕様（v3完成により不要） |
| 02_v2統合分割仕様/ | v2統合版仕様 |
| 03_旧分割仕様/ | 旧版仕様 |
| 04_監査差分分析/ | 分析レポート |
| 05_LLM運用_その他/ | その他のLLM運用 |
| 06_実装ガイド/ | 完了済み実装ガイド |
| 07_調査/ | 調査レポート |
| 08_AI提案/ | AI提案・設計案・照合レポート |
| 09_旧仕様_20260310/ | 現行実装と不整合になった旧仕様 |
| 10_TTS仕様_整理_20260311/ | TTS仕様整理（旧クライアント/サーバ仕様） |
| 11_旧仕様_20260326/ | 実装仕様_v3.md 旧コピー（01_正本仕様に統合）、story_quality_issues.md 観察ログ |
| codebase-map/ | コードベースマップ |

---

## 8. メンテナンスルール

| ルール | 詳細 |
|--------|------|
| 仕様先行 | 実装変更前に仕様を更新する |
| 正本は 01_正本仕様/ | archive/ には入れない |
| 実装仕様は 04_実装仕様_機能拡張/ | 新規実装仕様はここに追加 |
| TOOL_CONTRACT はルート | docs/ に移動しない |
| アーカイブは読み取り専用 | 直接編集しない |
| この README を更新 | 仕様追加時は必ずこの索引を更新する |

---

**プロジェクトルート**: `/home/nyukimi/picoclaw_multiLLM/`
**ブランチ**: `feature/rencrow`
