# PicoClaw ドキュメント

**最終更新**: 2026-03-10

---

## 仕様体系の全体像

PicoClaw の仕様は **4つの系統** で構成される。

```
[基盤思想]                          [実装仕様]
  TOOL_CONTRACT.md (リポジトリルート)   仕様.md (要件定義)
  会話LLM仕様_v1.0.md                 実装仕様_v3.md (CA)
  拡張設計仕様.md                      実装仕様_分散実行_v4.md
                                     実装仕様_会話LLM_v5.md
                                     実装仕様_会話エンジン_v5.1.md

[設計文書]                          [運用仕様]
  Chat_Worker_Coder_アーキテクチャ.md   実装仕様_OpenClaw移植_v1.md
                                       02_OpenClaw移植詳細仕様/
                                       LLM運用/
```

### 読む順序

| 目的 | 最初に読む | 次に読む |
|------|----------|---------|
| 全体理解 | 仕様.md → この README | Chat_Worker_Coder_アーキテクチャ.md |
| 実装作業 | 実装仕様_v3.md | 対象領域の実装仕様（v4/v5/v5.1） |
| 機能追加 | 実装仕様_OpenClaw移植_v1.md | TOOL_CONTRACT.md |
| 会話システム | 会話LLM仕様_v1.0.md | 実装仕様_会話LLM_v5.md → v5.1 |
| データ基盤拡張 | 拡張設計仕様.md | -- |

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

### 1.2 会話LLM仕様_v1.0.md

**会話の設計原則** -- 時間軸で育つ対話を成立させるためのシステム構造。

| 内容 | 要点 |
|------|------|
| 会話の単位 | Message / Turn / Thread / Session / Conversation |
| 記憶レイヤー | 短期(RAM) / 中期(Redis→DuckDB) / 長期(VectorDB) / KB / Persona / UserProfile |
| 責務分離 | Chat=見た目 / Worker=想起+判断+記録 / Coder=実装 |
| 処理フロー | 入力→想起→判断→生成→記録（Spawn禁止、同期のみ） |
| JSON I/F | Chat→Worker / Worker→Coder / Worker→Memory の3契約 |
| 実装状況 | 付録Aに30項目の照合表（73%実装済み） |

### 1.3 拡張設計仕様.md

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

### 2.1 仕様.md（要件定義）

プロジェクトの目的、用語、ルーティングカテゴリ、セキュリティ、セッション、ログの要件。

### 2.2 実装仕様_v3.md（Clean Architecture）-- 3,067行

v3.0 の完全実装仕様。**全実装の基盤**。

| 内容 | 状態 |
|------|------|
| Clean Architecture 4層構造 | 実装完了 |
| Worker即時実行（承認フロー廃止） | 実装完了 |
| Domain/Application/Infrastructure/Adapter | 実装完了 |
| テストカバレッジ 87.1% | 達成済み |

### 2.3 実装仕様_分散実行_v4.md -- 2,334行

v3.0 の上に分散実行機能を追加する仕様。

| 内容 | 状態 |
|------|------|
| Transport層（Local/SSH） | 実装完了 |
| picoclaw-agent（スタンドアロン） | 実装完了 |
| DistributedOrchestrator | 実装完了 |
| 本番有効化 | Claude クレジット補充待ち |

### 2.4 実装仕様_会話LLM_v5.md

3層記憶インフラ（Redis/DuckDB/Qdrant）の実装仕様。

| 内容 | 状態 |
|------|------|
| Phase 1: ドメイン層 | 実装完了 |
| Phase 2: 3層ストア | 実装完了 |
| Phase 3: Embedder/Summarizer | 実装完了 |
| 統合テスト 9件 | 全通過 |

### 2.5 実装仕様_会話エンジン_v5.1.md

ConversationEngine（RecallPack + Persona）の実装仕様。

| 内容 | 状態 |
|------|------|
| ConversationEngine I/F | 実装完了 |
| RecallPack 生成 | 実装完了 |
| Persona 注入 | 実装完了 |
| Thread 自動判定 | 未実装 |
| UserProfile 自動抽出 | 未実装 |

### 2.6 実装仕様_チャネル拡張_v1.md

Discord / Slack / 音声入出力のアダプター追加仕様。

| 内容 | 状態 |
|------|------|
| ChannelAdapter 共通インターフェース | 実装完了（HTTP/Webhook基盤） |
| Discord アダプター (WebSocket Gateway) | 部分実装（Webhook/Interaction） |
| Slack アダプター (Socket Mode) | 部分実装（Events API） |
| 音声アダプター (STT + TTS) | 未実装 |
| セッション ID 規約（チャネル横断） | 実装完了 |
| 設定ファイル拡張 (channels) | 実装完了（Telegram/Discord/Slack） |

### 2.7 実装仕様_OpenClaw移植_v1.md

OpenClawの実装実行能力をGo基盤へ段階移植するための仕様。

| 内容 | 状態 |
|------|------|
| Execution Contract（依頼→実行契約） | 部分実装（正規化/検証） |
| Autonomous Executor（Plan→Apply→Verify→Repair） | 設計完了 |
| TTS Capability Pack（OpenAI→ElevenLabs→local） | 設計完了 |
| Evidence（execution_report） | 部分実装（Execution監査ログ） |

### 2.8 OpenClaw移植詳細仕様（分割）

`docs/02_OpenClaw移植詳細仕様/` 配下に、依存順で詳細実装仕様を配置する。

| ファイル | 内容 | 状態 |
|---------|------|------|
| 詳細実装仕様_01_実行基盤とセキュリティ境界.md | Tools実行制御・承認・監査ログ・運用CLI | 実装進行中 |
| 詳細実装仕様_02_チャネル網羅不足.md | Telegram/Discord/Slack追加と共通イベント契約 | 実装進行中 |
| 詳細実装仕様_03_Tools体系の差.md | ToolManifest/Registry/ExecutionEnvelope | 実装進行中 |
| 詳細実装仕様_04_Nodes_デバイス能力の差.md | NodeCapabilityと要件ベース選定 | 実装進行中 |
| 詳細実装仕様_05_Gateway_Ops_CLIの差.md | gateway/channels/status/health/doctor/logs | 実装進行中 |
| 詳細実装仕様_06_Security_Sandboxの差.md | SecurityProfileと権限スコープ・監査 | 実装進行中 |
| 詳細実装仕様_07_App_Platform導線の差.md | Unified Entryと進行イベント統一 | 実装進行中 |

補助資料:
- `OpenClaw機能差分比較表_20260310.md`（OpenClawとの機能差分サマリ）

---

## 3. 設計文書

### 3.1 Chat_Worker_Coder_アーキテクチャ.md

Chat/Worker/Coder の役割・責務・指揮命令系統。分散実行の設計思想を含む。

---

## 4. 運用仕様

### 4.1 実装仕様_OpenClaw移植_v1.md

OpenClaw 能力移植の正本仕様。実装は `02_OpenClaw移植詳細仕様/` の分割仕様を一次参照として進行する。

### 4.2 LLM運用/

| ファイル | 内容 |
|---------|------|
| Coder3_Claude_API仕様.md | Claude API 運用、Proposal生成 |
| LLM_Worker_Spec_v1_0.md | Worker（Shiro）の仕様 |
| LLM_Ollama常駐管理.md | Ollama 常駐管理、ヘルスチェック |

### 4.3 実装ガイド進行管理

| ファイル | 内容 |
|---------|------|
| 20260309_OpenClaw移植_runbook.md | OpenClaw移植の実機検証手順（E2E実再生完了判定） |

---

## 5. 仕様間の依存関係

```
仕様.md（要件）
  |
  +-- 実装仕様_v3.md（CA基盤）
  |     |
  |     +-- 実装仕様_分散実行_v4.md（v3の上に追加）
  |     |
  |     +-- 実装仕様_会話LLM_v5.md（3層記憶インフラ）
  |     |     |
  |     |     +-- 実装仕様_会話エンジン_v5.1.md（RecallPack + Persona）
  |     |
  |     +-- 実装仕様_チャネル拡張_v1.md（Discord/Slack/音声）
  |
  +-- Chat_Worker_Coder_アーキテクチャ.md（設計思想）
  |
  +-- 実装仕様_OpenClaw移植_v1.md（OpenClaw移植の正本）

会話LLM仕様_v1.0.md（設計原則）
  |
  +-- 実装仕様_会話LLM_v5.md + v5.1 の上位思想

拡張設計仕様.md（データ基盤）
  |
  +-- PicoClaw 外のエンタメDB基盤に適用

TOOL_CONTRACT.md（ツール契約）
  |
  +-- 全ツール実装に適用（Coder/Worker が参照）
```

---

## 6. アーカイブ

`archive/` 配下のドキュメントは参考資料。直接編集しない。

**重要**: `docs/archive/` 配下は履歴参照専用。実装の一次参照として使用しない。  
実装判断は `docs/` 直下の正本仕様と `docs/02_OpenClaw移植詳細仕様/` を参照する。

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
| codebase-map/ | コードベースマップ |

---

## 7. メンテナンスルール

| ルール | 詳細 |
|--------|------|
| 仕様先行 | 実装変更前に仕様を更新する |
| 正本は docs/ 直下 | archive/ には入れない |
| TOOL_CONTRACT はルート | docs/ に移動しない |
| アーカイブは読み取り専用 | 直接編集しない |
| この README を更新 | 仕様追加時は必ずこの索引を更新する |

---

**プロジェクトルート**: `/home/nyukimi/picoclaw_multiLLM/`
**ブランチ**: `proposal/clean-architecture`
