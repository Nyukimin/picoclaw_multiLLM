# PicoClaw ドキュメント

**最終更新**: 2026-03-05

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
  Chat_Worker_Coder_アーキテクチャ.md   移植仕様.md
  仕様_会話エンジン_v1.1.md             LLM運用/
```

### 読む順序

| 目的 | 最初に読む | 次に読む |
|------|----------|---------|
| 全体理解 | 仕様.md → この README | Chat_Worker_Coder_アーキテクチャ.md |
| 実装作業 | 実装仕様_v3.md | 対象領域の実装仕様（v4/v5/v5.1） |
| 機能追加 | 移植仕様.md | TOOL_CONTRACT.md |
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
| ChannelAdapter 共通インターフェース | 未実装 |
| Discord アダプター (WebSocket Gateway) | 未実装 |
| Slack アダプター (Socket Mode) | 未実装 |
| 音声アダプター (STT + TTS) | 未実装 |
| セッション ID 規約（チャネル横断） | 設計完了 |
| 設定ファイル拡張 (channels) | 未実装 |

---

## 3. 設計文書

### 3.1 Chat_Worker_Coder_アーキテクチャ.md

Chat/Worker/Coder の役割・責務・指揮命令系統。分散実行の設計思想を含む。

### 3.2 仕様_会話エンジン_v1.1.md

会話エンジンの上位仕様（要件レベル）。v5.0/v5.1 実装仕様の前提。

---

## 4. 運用仕様

### 4.1 移植仕様.md

OpenClaw 機能を基準にした PicoClaw への移植計画。

| 内容 | 要点 |
|------|------|
| 現状分析 | 44機能中17実装済み (39%) + 独自4機能 |
| 全機能判定 | 移植(10) / 適応(11) / 見送り(11) / 実装済み(4) |
| Phase 1 | 上流移植完了（ContextBuilder, Health, Skills, Subagent） |
| Phase 2 | CLI管理コマンド（status, health, doctor, models, memory） |
| Phase 3 | チャット内コマンド（/status, /stop, /compact, /context, /new） |
| Phase 4 | チャネル拡張（Telegram, Discord, Slack） |
| Phase 5 | 自動化拡張（Subagent仕上げ, cron） |
| Phase 6 | 音声（Edge TTS） |

### 4.2 LLM運用/

| ファイル | 内容 |
|---------|------|
| Coder3_Claude_API仕様.md | Claude API 運用、Proposal生成 |
| LLM_Worker_Spec_v1_0.md | Worker（Shiro）の仕様 |
| LLM_Ollama常駐管理.md | Ollama 常駐管理、ヘルスチェック |

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
  +-- 移植仕様.md（機能拡張計画）

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
