# CLAUDE.md - PicoClaw プロジェクトルール

**作成日**: 2026-02-24
**プロジェクト名**: PicoClaw (picoclaw_multiLLM)
**目的**: 超軽量 AI アシスタントのマルチ LLM ルーティング実装

---

## 1. プロジェクト概要

### 1.1 プロジェクト名

**PicoClaw** (picoclaw_multiLLM)

### 1.2 目的

- LINE/Slack からの指示を受け、複数の LLM を適切にルーティングして実行する超軽量 AI アシスタント
- メモリ使用量 <10MB、低スペックハードウェア（$10 デバイス）で動作
- Chat（会話）/ Worker（実行）/ Coder（設計・実装）の役割分離による効率的な自動化

### 1.3 主要機能

- **マルチ LLM ルーティング**: Chat（Mio）、Worker（Shiro）、Coder1（Aka）、Coder2（Ao）、Coder3（Claude）の自動振り分け
- **承認フロー**: 破壊的操作には承認が必須（job_id ベースの追跡）
- **Auto-Approve モード**: Scope/TTL 付き自動承認（即時 OFF 可能）
- **ヘルスチェック**: Ollama 常駐監視、自動再起動
- **セッション管理**: 日次カットオーバー、メモリ管理
- **ログ保存**: 構造化ログ、Obsidian 連携

### 1.4 使用言語・プラットフォーム

- **使用言語**: Go 1.23
- **プラットフォーム**: Linux (主に)、macOS (開発環境)
- **依存 LLM**:
  - Ollama (chat-v1, worker-v1)
  - DeepSeek (Coder1)
  - OpenAI (Coder2)
  - Anthropic Claude API (Coder3)

---

## 2. ルールファイルの階層

このプロジェクトでは、以下のルールファイルを階層的に参照します。

### 2.1 共通ルール（すべてのプロジェクトで適用）

以下のファイルは `rules/common/` に配置され、プロジェクト横断で適用される基本方針です。

**起動時に必ず読み込むルール**:
- **`rules/common/GLOBAL_AGENT.md`**: AI 開発の共通方針（憲法レベル）
  - コミュニケーション原則
  - ペアプログラミングのコア原則
  - コード修正における思考憲法
  - データ処理の基本原則

**ドメイン別の詳細ルール**（必要に応じて参照）:
- **`rules/common/rules_architecture.md`**: アーキテクチャ・設計・リトライ戦略
- **`rules/common/rules_backend.md`**: バックエンド開発の詳細ルール
- **`rules/common/rules_frontend.md`**: フロントエンド開発の詳細ルール
- **`rules/common/rules_logging.md`**: ログ・観測・マスキングの詳細ガイド
- **`rules/common/rules_security.md`**: セキュリティ・依存関係・アクセス制御
- **`rules/common/rules_testing.md`**: テスト・TDD・E2E の詳細ガイド

### 2.2 プロジェクト固有ルール（PicoClaw 専用）

PicoClaw プロジェクト固有の注意事項は、このファイル（CLAUDE.md）に直接記述します。

---

## 3. PicoClaw 固有の注意事項

### 3.1 実装の一次参照

**最優先**: `docs/01_正本仕様/実装仕様.md`
- すべての実装判断はこのファイルを一次参照とする
- 仕様の曖昧さ・矛盾がある場合は、このファイルを更新してから実装する

**補助参照**:
- `docs/02_v2統合分割仕様/` - 分割された詳細仕様
- `docs/05_LLM運用プロンプト設計/` - LLM 固有の運用ルール
- `docs/06_実装ガイド進行管理/` - 実装プラン・進捗管理

### 3.2 ドキュメント分類

ドキュメントの分類と用途は `docs/00_ドキュメント分類一覧.md` を参照:
- **coding**: 実装時に直接参照する仕様
- **ops**: 運用時に参照する手順
- **prompt**: プロンプト資産として使う
- **reference**: 背景・比較・監査の参照用
- **archive**: 履歴保存用

### 3.3 重要な設計原則

#### 3.3.1 責務の分離（Chat/Worker/Coder）

| 役割 | 責務 | 実行内容 |
|------|------|----------|
| **Chat** | 意思決定・承認管理 | ユーザー対話、ルーティング決定、承認要求送信 |
| **Worker** | 実行・道具係 | ファイル編集、コマンド実行、テスト実行、差分適用 |
| **Coder** | 設計・実装案作成 | 仕様策定、コード生成、`plan` と `patch` の生成 |

**不変ルール**:
- Coder は原則として破壊的操作を**直接実行せず**、`plan` と `patch` を生成
- 実行は Worker が承認後に担当
- Chat が承認フローを管理

#### 3.3.2 承認フロー（必須）

- Coder3（Claude API）による提案には**承認が必須**
- job_id でジョブを追跡（ログ、承認状態）
- 承認コマンド: `/approve <job_id>`, `/deny <job_id>`
- Auto-Approve モードは Scope/TTL 付き、即時 OFF 可能

#### 3.3.3 ルーティングカテゴリ

- `CHAT`: 会話・意思決定
- `PLAN`: 計画策定
- `ANALYZE`: 分析
- `OPS`: 運用操作
- `RESEARCH`: 調査
- `CODE`: コーディング（汎用）
- `CODE1`: 仕様設計向け（DeepSeek 等）
- `CODE2`: 実装向け（OpenAI 等）
- `CODE3`: 高品質コーディング/推論（Claude API 専用）

ルーティング決定は以下の優先順位で行う:
1. 明示コマンド（`/code`, `/code1`, `/code2`, `/code3` 等）
2. ルール辞書（強証拠）
3. 分類器（LLM による判定）
4. 安全側フォールバック（`CHAT`）

### 3.4 実装環境の制約

#### 3.4.1 メモリ制約

- **目標**: メモリ使用量 <10MB
- **手法**:
  - Ollama の軽量モデル使用（chat-v1, worker-v1）
  - セッションメモリの最小化
  - 日次カットオーバーによるメモリリセット

#### 3.4.2 Ollama 常駐管理

- `keep_alive: -1` で Chat/Worker モデルを常駐化
- ヘルスチェックで Ollama の状態監視
- MaxContext 制約（8192）を超えるモデルは NG
- 自動再起動スクリプト（`systemctl --user restart ollama`）

#### 3.4.3 API キー管理

- **Anthropic API キー**: 環境変数 `ANTHROPIC_API_KEY` から取得（平文保存禁止）
- **DeepSeek API キー**: 環境変数 `DEEPSEEK_API_KEY` から取得
- **OpenAI API キー**: 環境変数 `OPENAI_API_KEY` から取得
- シークレットストア推奨、設定ファイルへの平文保存は禁止

### 3.5 開発フロー

#### 3.5.1 実装前の確認

1. **仕様確認**: `docs/01_正本仕様/実装仕様.md` を読む
2. **実装プラン**: `docs/06_実装ガイド進行管理/` の最新プランを確認
3. **既存パターン**: 既存コードの命名規則・構造を尊重

#### 3.5.2 コード修正時の原則

- **理解なき提案の禁止**: コードを変える前に、既存の動作を把握する
- **根本原因の徹底追求**: エラーを消すだけでなく、原因を探る
- **命名と設計意図の尊重**: 既存の変数名・関数名・構造を尊重
- **論理的一貫性の死守**: その場しのぎのハックを避ける

#### 3.5.3 テスト

- **ユニットテスト**: `go test ./pkg/...` で全テストを実行
- **カバレッジ目標**: 重要パッケージ（agent, approval, session）は 80% 以上
- **統合テスト**: End-to-End シナリオでルーティング・承認フローを検証

### 3.6 ログとトレーサビリティ

#### 3.6.1 ログイベント種別

- `router.decision` - ルーティング決定
- `classifier.error` - 分類器エラー
- `worker.success` / `worker.fail` - Worker 実行結果
- `approval.requested` / `approval.granted` / `approval.denied` - 承認フロー
- `coder.plan_generated` - Coder による plan/patch 生成

#### 3.6.2 必須保存項目

- `job_id`: ジョブ識別子
- `initial_route`, `final_route`: ルーティング情報
- `approval_status`: 承認状態（`pending`, `granted`, `denied`, `auto_approved`）
- `coder_output`: Coder の生成した plan/patch/risk の要約

---

## 4. プロジェクト固有の禁止事項

### 4.1 コード修正

- ❌ **仕様を読まずに実装する**（必ず `docs/01_正本仕様/実装仕様.md` を参照）
- ❌ **Coder が破壊的操作を直接実行する**（plan/patch のみ生成、実行は Worker）
- ❌ **承認なしに破壊的変更を適用する**（削除、リネーム、広範囲の上書き）
- ❌ **job_id なしで承認フローを管理する**（すべての承認ジョブに job_id 必須）

### 4.2 設定・運用

- ❌ **API キーを平文で設定ファイルに保存する**（環境変数またはシークレットストア）
- ❌ **MaxContext（8192）を超える Ollama モデルをロードする**（ヘルスチェックで NG）
- ❌ **Chat/Worker モデルを常駐化せずに使う**（`keep_alive: -1` 必須）

### 4.3 ドキュメント

- ❌ **正本仕様を更新せずに実装を変更する**（実装と仕様の不整合を防ぐ）
- ❌ **アーカイブファイルを直接編集する**（`docs/03_旧分割仕様アーカイブ/` は読み取り専用）

---

## 5. 開発タスクの進め方

### 5.1 新機能追加

1. **仕様確認**: `docs/01_正本仕様/仕様.md` で要件を確認
2. **実装仕様作成**: `docs/01_正本仕様/実装仕様.md` に追記
3. **実装プラン作成**: `docs/06_実装ガイド進行管理/` に日付付きプランを作成
4. **実装**: プランに従って段階的に実装
5. **テスト**: ユニットテスト・統合テストで検証
6. **ドキュメント更新**: `docs/00_ドキュメント分類一覧.md` を更新

### 5.2 バグ修正

1. **再現確認**: エラーログ・症状を記録
2. **原因調査**: コードを読み、根本原因を特定
3. **修正**: 最小限の変更で根本原因を解消
4. **テスト**: 修正箇所と関連機能をテスト
5. **ドキュメント**: 必要に応じて注意事項を追記

### 5.3 リファクタリング

1. **目的明確化**: 何のためのリファクタリングか（保守性向上、性能改善等）
2. **影響範囲特定**: 変更が影響する箇所を洗い出し
3. **テスト追加**: リファクタリング前にテストを充実させる
4. **段階的実施**: 一度に大きく変えず、小さく確実に進める
5. **検証**: 既存テストがすべて通ることを確認

---

## 6. 参照リンク

### 6.1 正本仕様

- **実装仕様**: `docs/01_正本仕様/実装仕様.md`（実装の一次参照）
- **要件仕様**: `docs/01_正本仕様/仕様.md`（上位要求）

### 6.2 LLM 運用

- **Coder3 仕様**: `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md`
- **Worker 仕様**: `docs/05_LLM運用プロンプト設計/LLM_Worker_Spec_v1_0.md`
- **DeepSeek 運用**: `docs/05_LLM運用プロンプト設計/LLM_deepseek運用仕様.md`
- **Ollama 管理**: `docs/05_LLM運用プロンプト設計/LLM_Ollama常駐管理.md`

### 6.3 実装ガイド

- **Coder3 統合仕様**: `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md`
- **Coder3 実装プラン**: `docs/06_実装ガイド進行管理/20260224_Coder3承認フロー実装プラン.md`

### 6.4 共通ルール

- **グローバル方針**: `rules/common/GLOBAL_AGENT.md`
- **アーキテクチャ**: `rules/common/rules_architecture.md`
- **バックエンド**: `rules/common/rules_backend.md`
- **セキュリティ**: `rules/common/rules_security.md`
- **テスト**: `rules/common/rules_testing.md`
- **ログ**: `rules/common/rules_logging.md`

---

## 7. 起動時の読み込み順序

Claude Code（または同等の AI エディタ）の起動時には、以下の順序でルールを読み込みます:

1. **このファイル（CLAUDE.md）** - プロジェクト概要と固有ルール
2. **`rules/common/GLOBAL_AGENT.md`** - AI 開発の共通方針（必須）
3. **`rules/PROJECT_AGENT.md`** - PicoClaw プロジェクト固有の詳細ルール
4. **`rules/rules_domain.md`** - PicoClaw ドメイン固有の技術的詳細
5. **`docs/01_正本仕様/実装仕様.md`** - 実装の一次参照（実装タスク時）
6. **ドメイン別ルール** - 必要に応じて参照:
   - `rules/common/rules_architecture.md`
   - `rules/common/rules_backend.md`
   - `rules/common/rules_security.md`
   - `rules/common/rules_testing.md`
   - `rules/common/rules_logging.md`

---

**最終更新**: 2026-02-24
**バージョン**: 1.0
**メンテナンス**: 仕様変更時は必ずこのファイルと実装仕様を同期させること
