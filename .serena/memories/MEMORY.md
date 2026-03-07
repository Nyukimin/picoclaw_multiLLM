# PicoClaw Memory Index

## Design Principles (MUST FOLLOW)
- **既存コードの設計が間違っていたら、踏襲せず最初に指摘して正す。** 問題のあるパターンに合わせて追加コードを書くのではなく、根本から直す提案をすること。
- **一般的・標準的な手法が存在するなら、それを最初に採用する。** 独自実装を書く前に「これは標準ライブラリやフレームワークの一般的なパターンで解決できないか？」を必ず確認する。
- **トライアンドエラーは2回まで。** 2回試して解決しなければ、立ち止まって原因を分析し直す。同じアプローチの微修正を繰り返さない。別の仮説を立てるか、ユーザーに相談する。
- **E2Eテストは本番と同じ経路を通す。** ハードコードや `os.Getenv` 直読みではなく、本番コードと同じ設定読込パス（config.yaml → Config struct）を使う。本番で動くことを証明するのがE2Eの目的。
- **指摘は原則として吸収する。** 1箇所で受けた指摘を、その場だけの修正で終わらせない。同じ原則が当てはまる他の箇所にも適用する。

## Active Work
- **[Phase 4.2 Worker RESEARCH自動保存](phase42_worker_research_autosave_progress.md)** — 🔄 実装中（ビルドエラー修正待ち）
  - ✅ ToolResponse Metadata拡張完了
  - ✅ web_search V2実装完了
  - ✅ Mio ConversationManager統合完了
  - ⏸️ LegacyRunner ExecuteV2実装が必要
  - ⏸️ main.go DI統合が必要

## Completed in This Session (2026-03-07)
- **[KB運用ガイド](../docs/KB運用ガイド.md)** ✅ 作成完了（400行、運用者向け完全ガイド）
- **[実装仕様更新](../docs/実装仕様_会話LLM_v5.md)** ✅ Phase 4.1/4.2反映完了
- **[kb-admin CLI](../cmd/kb-admin/)** ✅ 実装完了（search/stats/list/cleanup）
- **[KB基盤 Phase 4.1](conversation_llm_v5_status.md)** ✅ 全完了
  - SaveKB/SearchKB (VectorDB)
  - SaveWebSearchToKB (RealManager)
  - RAG統合 (ConversationEngine)
  - KB管理CLI

## Recent Completions
- **[サブエージェント v1](subagent_status.md)** — 全完了。Chat()実装済み（Claude/OpenAI/DeepSeek ToolCallingProvider準拠）
- **[ルーティング v6](routing_v6_status.md)** — Phase 1-4 全完了・push済み
- **[Conversation LLM v5.0](conversation_llm_v5_status.md)** — Phase 1-3完了、E2E動作確認済み（Redis/Qdrant/DuckDB実接続OK）
- [E2Eテスト本番経路統一](e2e_testing_progress.md) — 全4ファイル完了
- [上流→CA移植](upstream_port_progress.md) — MemoryStore✓ Heartbeat✓ ContextBuilder✓ Subagent✓
- [Web Search](web_search_implementation_status.md) — Google Custom Search API統合完了

## Next Actions (優先順)
1. **LegacyRunner ExecuteV2実装** - ビルドエラー解消（最優先）
2. **main.go DI統合** - Mio に ConversationManager 注入
3. **KB自動保存E2Eテスト** - 動作確認
4. **Phase 4.2 完了** - 運用整備（Embedder初期化、本番デプロイ準備）
5. PR作成検討 - proposal/clean-architecture → main

## Key Facts
- Go 1.23, Clean Architecture (domain/application/infrastructure/adapter)
- **注意: `Nyukimin` のタイポ (`Nyukimi`) が繰り返し発生。import パスを書くとき必ず `Nyukimin`（末尾 n）を確認すること。**
- Ollama remote: kawaguchike-llm (100.83.207.6:11434), model: chat-v1 (qwen3-vl 8.8B)
- LINE webhook: `https://fujitsu-ubunts.tailb07d8d.ts.net/webhook`
- Tailscale Funnel: `tailscale funnel --bg 18790` (systemd版は不安定→廃止)
- 秘密値: `~/.picoclaw/.env` (chmod 600) → `source` or systemd `EnvironmentFile=`
- Config: `${ENV_VAR}` 展開 (`os.ExpandEnv`)、`loadFromEnv()` は廃止済み
- Config読込: `PICOCLAW_CONFIG` 環境変数 (--config フラグは未実装)
- ログローテーション: `~/.picoclaw/bin/log-rotate.sh` (cron 毎日04:00)
- [Web Search Config](web_search_config.md) — Chat（即答/一般知識）とWorker（RESEARCH/エンタメDB）の検索対象分離
- **Branch:** proposal/clean-architecture（未マージ、Phase 4.2作業中）

## Build Status
⚠️ **ビルドエラー中** - LegacyRunner が ExecuteV2 未実装
- cmd/picoclaw/main.go: Line 323, 407, 408
- 対応: tool.LegacyRunner に ExecuteV2 実装が必要

# currentDate
Today's date is 2026-03-07.
