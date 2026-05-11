# LLM 運用 Archive 2026-05-11

このディレクトリは、`docs/LLM運用/` の整理時に退避した履歴文書を置く。

archive 配下は履歴参照用であり、現行実装・運用判断の一次参照には使わない。

## 退避した文書

- `Coder3_Claude_API仕様.md`: 旧 Coder3 / Claude API 前提の個別仕様。現行 proxy では `Coder` を公開しないため退避。
- `LLM_Ollama常駐管理.md`: Ollama 常駐管理前提の文書。現行の主経路は MLX / OpenAI 互換 API のため退避。
- `LLM_Worker_Spec_v1_0.md`: 旧 Worker 個別仕様。現行の role / API 仕様は `docs/LLM運用/LLM/LLM仕様.md` と `docs/LLM運用/サーバとクライアント/` を参照する。
- `LLM_モデル役割メモ.md`: 古い 2 プロセス前提と現行 4 role 前提が混在していたため退避。
- `llm-roles.md`: `LLM仕様.md` と内容が重複するため退避。
- `mlx_mgmt_memory_status_request.md`: 追加依頼書。現行仕様は `LLMメモリ監視API仕様.md` と `Viewer_LLM_Ops_Status仕様.md` を参照する。

