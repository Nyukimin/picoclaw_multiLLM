# 09_旧仕様_20260310

このディレクトリは、**2026-03-10 時点で現行実装と不整合になった旧仕様**を退避したアーカイブです。  
正本としては利用せず、履歴参照専用とします。

## 移動理由

- 旧仕様が `Draft` / `実装予定` のまま残り、現行コードと乖離していたため
- OpenClaw移植の正本を `docs/実装仕様_OpenClaw移植_v1.md` と
  `docs/02_OpenClaw移植詳細仕様/` に一本化したため
- `docs/README.md` の一次参照導線を現行仕様へ整理するため

## 収録ファイルと置換先

| 旧仕様（本ディレクトリ） | 置換先（現行一次参照） |
|---|---|
| `移植仕様.md` | `docs/実装仕様_OpenClaw移植_v1.md` |
| `仕様_会話エンジン_v1.1.md` | `docs/実装仕様_会話エンジン_v5.1.md` |
| `実装仕様_設定ファイル整理_v1.md` | `config.yaml.example` / `internal/adapter/config/` |
| `実装仕様_ペルソナ自己編集_v1.md` | `internal/infrastructure/persona/` / `internal/domain/agent/` |
| `実装仕様_ライブビュワー_v1.md` | `internal/adapter/viewer/` / `docs/02_OpenClaw移植詳細仕様/詳細実装仕様_07_App_Platform導線の差.md` |
| `20260304_会話LLM統合設計プラン.md` | `docs/実装仕様_会話LLM_v5.md` |

## 運用ルール

- この配下のファイルは更新しない（読み取り専用）
- 新規の正本仕様は `docs/` 直下または `docs/02_OpenClaw移植詳細仕様/` に作成する
