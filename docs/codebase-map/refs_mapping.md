---
generated_at: 2026-02-28T17:00:20+09:00
run_id: run_20260228_170007
phase: 0
step: "0a"
profile: picoclaw_multiLLM
---

# refs マッピング

## 概要

docs/ ディレクトリ配下の既存仕様・運用ドキュメントをプロファイルの refs_keywords でモジュールグループにマッピングした結果。Phase 1 の各モジュール解析時に、関連ドキュメントとして参照する。

## 関連ドキュメント

- プロファイル: `/home/nyukimi/picoclaw_multiLLM/codebase-analysis-profile.yaml`
- ドキュメント分類: `docs/00_ドキュメント分類一覧.md`

---

## マッピング結果

### core（エントリポイント・設定）

**キーワード**: 起動, エントリポイント, 設定

**マッチしたドキュメント**:
- `docs/01_正本仕様/実装仕様.md` (coding) - 実装の唯一正本、1章「スコープ・責務境界」で起動フロー・役割命名を定義
- `docs/01_正本仕様/仕様.md` (reference) - 上位要求・要件定義、「目的と前提」でシステム全体の設計方針
- `docs/02_v2統合分割仕様/実装仕様_v2_10_設定閾値.md` (coding, ops) - 設定値・閾値の詳細仕様
- `docs/02_v2統合分割仕様/実装仕様_v2_01_スコープ責務.md` (coding) - 責務境界の詳細

### agent（ルーティング・ループ）

**キーワード**: ルーティング, エージェント, 分類器

**マッチしたドキュメント**:
- `docs/01_正本仕様/実装仕様.md` (coding) - 2章「ルーティング決定仕様」、3章「ループ制御と再ルート」
- `docs/02_v2統合分割仕様/実装仕様_v2_02_ルーティング.md` (coding) - ルーティング決定の詳細（明示コマンド・辞書・分類器）
- `docs/02_v2統合分割仕様/実装仕様_v2_03_ループ再ルート.md` (coding) - ループ制御・再ルーティング仕様
- `docs/05_LLM運用プロンプト設計/LLM_分類整理.md` (prompt) - LLM ベースの分類器設計
- `docs/08_AIからの提案/routing-policy.md` (reference) - ルーティングポリシーの提案
- `docs/08_AIからの提案/agents.md` (reference) - エージェント設計の提案

### llm_provider（LLM プロバイダー）

**キーワード**: LLM, プロバイダー, Ollama, Claude, DeepSeek

**マッチしたドキュメント**:
- `docs/05_LLM運用プロンプト設計/LLM_Ollama常駐管理.md` (ops, prompt) - Ollama 常駐化・keep_alive 仕様
- `docs/05_LLM運用プロンプト設計/LLM_ollama世代管理.md` (ops) - Ollama モデル世代管理
- `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md` (coding, prompt) - Claude API (Coder3) 統合仕様
- `docs/05_LLM運用プロンプト設計/LLM_deepseek運用仕様.md` (ops, prompt) - DeepSeek (Coder1) 運用仕様
- `docs/05_LLM運用プロンプト設計/LLM_Worker_Spec_v1_0.md` (coding, prompt) - Worker (Shiro) 仕様
- `docs/05_LLM運用プロンプト設計/CHAT_PERSONA設計.md` (prompt) - Chat (Mio) ペルソナ設計
- `docs/05_LLM運用プロンプト設計/Mio_キャラクター設定_v1.md` (prompt) - Mio キャラクター詳細
- `docs/06_実装ガイド進行管理/20260220_Mio人格復旧_runbook.md` (ops) - Mio ペルソナ復旧手順
- `docs/06_実装ガイド進行管理/chat-v1_移行手順.md` (ops) - chat-v1 モデル移行手順

### approval（承認フロー）

**キーワード**: 承認, Auto-Approve, job_id

**マッチしたドキュメント**:
- `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md` (coding, prompt) - Coder3 の承認フロー仕様（6章）
- `docs/06_実装ガイド進行管理/20260224_Coder3承認フロー実装プラン.md` (ops) - 承認フロー実装プラン
- `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md` (ops) - Coder3 統合仕様の反映手順
- `docs/01_正本仕様/実装仕様.md` (coding) - 1.2節「Coder の責務」、6章「セキュリティ」で承認フロー概要

### session（セッション管理）

**キーワード**: セッション, カットオーバー, 永続化

**マッチしたドキュメント**:
- `docs/02_v2統合分割仕様/実装仕様_v2_09_状態管理.md` (coding) - session/memory 仕様の詳細
- `docs/01_正本仕様/実装仕様.md` (coding) - 9章「状態管理」
- `docs/Obsidian_仕様.md` (ops) - Obsidian 連携による永続化仕様
- `docs/Obsidian_サンプル.md` (reference) - Obsidian 出力サンプル

### mcp（MCP 統合）

**キーワード**: MCP, Chrome, DevTools

**マッチしたドキュメント**:
- `docs/06_実装ガイド進行管理/20260225_MCP_Chrome統合手順.md` (ops) - MCP Chrome DevTools Protocol 統合手順
- `docs/06_実装ガイド進行管理/win11_manual_setup.md` (ops) - Windows 11 環境での MCP セットアップ
- `docs/06_実装ガイド進行管理/README_Win11_Setup.md` (ops) - Windows 11 MCP セットアップ概要

### infra（ログ・設定）

**キーワード**: ログ, logger, config

**マッチしたドキュメント**:
- `docs/02_v2統合分割仕様/実装仕様_v2_07_ログ.md` (coding) - ログ仕様の詳細
- `docs/01_正本仕様/実装仕様.md` (coding) - 7章「ログ」
- `docs/06_実装ガイド進行管理/20260220_常駐監視運用手順.md` (ops) - 常駐監視とログ監視
- `docs/06_実装ガイド進行管理/20260220_監視実装検証レポート.md` (reference) - 監視実装の検証結果
- `docs/06_実装ガイド進行管理/20260224_ヘルスチェック強化とテスト追加.md` (ops) - ヘルスチェック強化（ログ関連）
- `docs/06_実装ガイド進行管理/20260219_224918_運用自動監視再起動仕様.md` (ops) - 自動監視・再起動の運用仕様

---

## 統計

- **総ドキュメント数**: 60+ ファイル (docs/)
- **マッピング対象**: 37 ファイル（分類: coding, ops, prompt）
- **アーカイブ除外**: 7 ファイル（分類: archive）
- **モジュールグループ数**: 7 グループ

---

---

## 既存調査マッピング（Phase 0b）

PicoClaw プロジェクトの既存調査記録（監査差分分析、実装ガイド進行管理の進捗レポート・検証結果）をモジュールグループにマッピング。

### 全体（複数グループ横断）

- `docs/04_監査差分分析/仕様_実装仕様_対応表.md` - 仕様と実装仕様の対応表（core, agent に関連）
- `docs/04_監査差分分析/仕様_実装仕様_差分メモ.md` - 仕様と実装仕様の差分メモ（core, agent に関連）
- `docs/06_実装ガイド進行管理/20260219_174342_現在進捗レポート.md` - 全体進捗レポート

### approval（承認フロー）

- `docs/06_実装ガイド進行管理/20260224_Coder3承認フロー実装プラン.md` - 承認フロー実装の詳細プラン
- `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md` - Coder3 統合仕様の反映手順と検証結果

### llm_provider（LLM プロバイダー）

- `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md` - Coder3（Claude API）統合の実装ガイド

### mcp（MCP 統合）

- `docs/06_実装ガイド進行管理/20260225_MCP_Chrome統合手順.md` - MCP Chrome DevTools Protocol 統合の進行中ガイド

### infra（ログ・設定）

- `docs/06_実装ガイド進行管理/20260220_監視実装検証レポート.md` - 監視実装の検証結果
- `docs/06_実装ガイド進行管理/20260224_ヘルスチェック強化とテスト追加.md` - ヘルスチェック強化の実装ガイド

---

## 注記

- **複数グループへのマッピング**: 1つのドキュメントが複数のモジュールグループに関連する場合がある（例: `実装仕様.md` は全グループに関連）。
- **分類タグ**: ドキュメント分類一覧（`docs/00_ドキュメント分類一覧.md`）の実施区分タグ（coding, ops, prompt, reference, archive）に従う。
- **アーカイブの扱い**: `archive` タグのファイル（旧分割仕様）は参照のみで、実装には反映しない。
- **既存調査の扱い**: 監査差分分析と実装ガイドの進捗レポート・検証結果を既存調査として扱う。Phase 2 の異常検出時に、これらとの重複チェックを行う。
