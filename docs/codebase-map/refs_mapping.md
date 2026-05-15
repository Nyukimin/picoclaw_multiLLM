---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 0
step: "0b"
profile: rencrow-core-map
artifact: survey_mapping
---

# 既存調査マッピング

## 概要

`docs/調査/` は存在しなかったため、Phase 0b では正本仕様、LLM運用、ルール、プロンプト、コーディング仕様を既存調査相当の参照資料としてマッピングした。  
このファイルは Phase 1 の各モジュール地図がどの資料を優先参照すべきかを示す。

## 関連ドキュメント

- `docs/01_正本仕様/実装仕様.md`
- `docs/01_正本仕様/03_エージェント定義.md`
- `docs/01_正本仕様/04_ルーティング.md`
- `docs/LLM運用/README.md`
- `docs/コーディング/coding_agent_modes.md`
- `docs/コーディング/coding_agent_implementation_spec.md`
- `rules/PROJECT_AGENT.md`
- `rules/common/`
- `prompts/README.md`

## モジュール別マッピング

| module_group | 関連資料 | 用途 |
|---|---|---|
| `entrypoints_config_docs` | `AGENTS.md`, `CLAUDE.md`, `docs/01_正本仕様/実装仕様.md`, `docs/LLM運用/`, `rules/`, `prompts/` | 作業ルール、起動・設定・LLM運用、プロンプト外部化の根拠 |
| `domain` | `docs/01_正本仕様/03_エージェント定義.md`, `docs/01_正本仕様/04_ルーティング.md`, `docs/08_仕様再構築/` | Agent、Route、Task、Patch、Memory の契約 |
| `application` | `docs/01_正本仕様/実装仕様.md`, `docs/04_実装仕様_機能拡張/`, `docs/07_IdleChat仕様/` | Orchestrator、Worker実行、IdleChat、会話エンジン |
| `infrastructure` | `docs/LLM運用/`, `docs/STT_TTS/`, `docs/01_正本仕様/09_セキュリティ.md` | LLM provider、永続化、セキュリティ、音声入出力 |
| `adapter` | `docs/09_Viewer/`, `docs/03_設計文書/ログViewer仕様.md`, `docs/LLM運用/サーバとクライアント/Viewer_LLM_Ops_Status仕様.md` | Viewer、LINE/Slack/Discord等チャネル、HTTP handler |

## 注意点

- `docs/archive/` は履歴参照専用であり、実装判断の一次参照には使わない。
- `docs/調査/` が無いため、潜在バグ一覧の重複チェックは正本仕様・機能拡張仕様・運用 docs との照合に限定した。
