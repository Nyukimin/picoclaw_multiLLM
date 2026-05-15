# Phase0 cmd構成整理プロンプト

以下をそのまま LLM / Codex に渡せる Goal 設定用プロンプトとして使う。

```text
Goal:
RenCrow のリファクタリング Phase 0 として、最初に着手する対象を確定し、cmd/picoclaw/main.go の composition root 整理に入るための事前調査・計画文書を作成してください。

作業対象:
- Repository: /home/nyukimi/picoclaw_multiLLM
- Phase: リファクタリング Phase 0
- 最初の対象候補: cmd/picoclaw/main.go の composition root 整理

目的:
- いきなりコード変更に入らず、まずリファクタリング対象を明確にする。
- cmd/picoclaw/main.go に集まっている wiring / runtime factory / handler 登録 / 実処理の境界を確認する。
- composition root に残すべきものと、Application / Infrastructure / Adapter 側へ移す候補を分ける。
- モジュール化と疎結合を最重要方針として、差し替え可能な境界を意識した移行計画にする。
- コード変更前に、移動対象・触らない対象・検証方法を短く確定する。

必ず参照するもの:
1. AGENTS.md
2. CLAUDE.md
3. docs/01_正本仕様/実装仕様.md
4. docs/codebase-map/アーキテクチャ総合.md
5. docs/codebase-map/結合ポイントマップ.md
6. docs/codebase-map/ユースケース逆引き.md
7. docs/codebase-map/modules/entrypoints_config_docs.md
8. docs/refactor/リファクタリング指針.md
9. docs/refactor/フォルダ構成方針.md
10. docs/refactor/段階移行計画.md
11. docs/refactor/検証方針.md
12. cmd/picoclaw/main.go と周辺ファイル

制約:
- この Phase 0 ではコード変更しない。
- docs/refactor/ 配下の調査・計画文書作成だけにする。
- ファイル名は日本語にする。
- 実装仕様の正本は docs/01_正本仕様/実装仕様.md のままとする。
- archive 文書を一次参照にしない。
- TODO / TBD の仮置きは残さない。
- 大規模移動や実装変更は開始しない。
- 未追跡の tests/ は今回の対象外として触らない。

作成する文書:
- docs/refactor/Phase0_cmd構成整理計画.md

文書に必ず含める内容:
1. Phase 0 の目的
   - コード変更前に対象範囲を固定すること
   - composition root 整理の入口を作ること

2. 現在の cmd/picoclaw/main.go の役割整理
   - 起動処理
   - CLI 引数処理
   - HTTP server 起動
   - runtime config 読み込み
   - runtime factory
   - handler 登録
   - IdleChat / STT / TTS / VTuber / Viewer / LLM ops などの wiring
   - 実処理が混ざっている可能性のある箇所

3. composition root に残すもの
   - process 起動
   - CLI entrypoint
   - dependency wiring
   - server 起動
   - adapter / application / infrastructure の接続

4. 移動候補
   - Application に寄せる候補
   - Infrastructure に寄せる候補
   - Adapter に寄せる候補
   - まだ動かさない候補

5. 触らない対象
   - WorkerExecutionService の安全境界
   - MessageOrchestrator の route dispatch 本体
   - Viewer 表示契約
   - IdleChat の raw/view/audio 契約
   - runtime config の意味変更
   - LLM provider の挙動変更
   - STT/TTS provider の挙動変更

6. モジュール化・疎結合の観点
   - 単なるファイル分割ではなく、入力・出力・副作用・ログ・エラー契約を明確にすること
   - interface / contract / event / DTO / adapter の境界を確認すること
   - 便利だから共有する、似ているからまとめる、だけの共通化を避けること
   - 巨大な service / manager / helper / util を新設しないこと

7. 最初の小さい移行単位案
   - 1回目に動かしてよい候補
   - 動かす前に追加または確認するテスト
   - 移動後に確認するコマンド
   - live runtime 確認が必要かどうか

8. 検証方法
   - Go test の対象
   - cmd/picoclaw 関連テスト
   - 必要なら health 確認
   - Viewer / IdleChat / STT / TTS が関係する場合の追加確認
   - runtime config を伴う場合は ~/.picoclaw/config.yaml と /health を確認すること

9. Phase 0 の完了条件
   - 移動対象が明確である
   - 触らない対象が明確である
   - 最初の移行単位が小さい
   - 検証方法が明記されている
   - コード変更前にユーザーが判断できる状態になっている

実行手順:
1. 参照文書を読む。
2. cmd/picoclaw/main.go と周辺ファイルを読む。
3. composition root に残す責務と、移動候補を分類する。
4. docs/refactor/Phase0_cmd構成整理計画.md を作成する。
5. コード変更は行わない。
6. 最後に、作成ファイル、要点、次にユーザーへ確認すべきことを報告する。
```
