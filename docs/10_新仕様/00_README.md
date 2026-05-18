# RenCrow 新仕様

## このフォルダの役割

`docs/10_新仕様/` は、RenCrow の現行正本仕様 `docs/01_正本仕様/` と Phase28 から Phase34 までのリファクタリング結果を統合した、新規リポジトリ移行用の最新仕様セットである。

新規リポジトリへ RenCrow を移す場合は、このフォルダを仕様入口として持っていく。旧リポジトリ内では `docs/01_正本仕様/` が履歴上の正本仕様として残るが、移行先で読むべき最新の整理済み仕様はこのフォルダである。

現行実装判断は `docs/10_新仕様/` を入口にする。旧 docs は履歴参照であり、現行実装として必要な項目は `13_実装項目インベントリ.md` と該当仕様文書へ反映する。

## 統合元

- `docs/01_正本仕様/`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase34_モジュール分割完了監査.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- 現在の実装コード

`docs/codebase-map/` は補助地図であり、仕様判断の最終根拠ではない。実装と矛盾する場合は、現在のコード、Phase34 の完了監査、この新仕様群を優先する。

## 読み順

1. `01_新仕様_概要.md`
2. `02_モジュール構成仕様.md`
3. `03_モジュール関係図.html`
4. `04_Chat_Worker_Coder仕様.md`
5. `05_Viewer仕様.md`
6. `06_IdleChat仕様.md`
7. `07_STT_TTS仕様.md`
8. `08_LLM_provider仕様.md`
9. `09_Memory_SourceRegistry仕様.md`
10. `10_検証仕様.md`
11. `11_分割再設計候補.md`
12. `12_新規リポジトリ移行方針.md`
13. `13_実装項目インベントリ.md`
14. `18_知識記憶システム構想.md`
15. `19_DCI_直接コーパス探索仕様.md`

## 文書一覧

| 文書 | 役割 |
| --- | --- |
| `00_README.md` | 新仕様セットの入口、読み順、各文書の役割 |
| `01_新仕様_概要.md` | RenCrow の目的、原則、主要コンポーネントの位置づけ |
| `02_モジュール構成仕様.md` | Clean Architecture 風の層、主要実装箇所、仕様変更時に触る場所 |
| `03_モジュール関係図.html` | モジュールの意味とつながりを概要図から詳細図まで示す HTML 図解 |
| `04_Chat_Worker_Coder仕様.md` | Chat / Worker / Coder の責務、route chain、plan / patch / execution 境界 |
| `05_Viewer仕様.md` | Viewer 表示、SSE event、log、history、audio trigger の契約 |
| `06_IdleChat仕様.md` | IdleChat の raw response、view data、audio trigger、forecast/story/normal mode |
| `07_STT_TTS仕様.md` | STT 入力経路、TTS provider / bridge / audio router、口パク trigger |
| `08_LLM_provider仕様.md` | Chat / Worker / Heavy / Wild / Coder provider と factory / middleware |
| `09_Memory_SourceRegistry仕様.md` | conversation memory、L1SQLite、VectorDB、DuckDB、Source Registry |
| `10_検証仕様.md` | unit / integration / e2e / live / browser 検証の標準 |
| `11_分割再設計候補.md` | 1 対 1 で説明しにくい箇所や将来の分割候補 |
| `12_新規リポジトリ移行方針.md` | 新規リポジトリへ持っていく docs と持っていかない旧 docs |
| `13_実装項目インベントリ.md` | 現行実装項目の実装済み / 部分実装 / 未実装 / 移行対象外一覧 |
| `18_知識記憶システム構想.md` | Personal Archive、Creative Knowledge、News、Daily Intake、Dream Consolidation など知識・記憶の入口構想 |
| `19_DCI_直接コーパス探索仕様.md` | RAG / VectorDB だけではなく、原文コーパスへ戻って証拠を調べ直す DCI 仕様 |

## 基本方針

- モジュール化と疎結合を最重要原則とする。
- 単にファイルを分けるだけではモジュール化と扱わない。
- 仕様変更時に触る主担当ファイルを説明できる状態を維持する。
- Chat / Worker / Coder の責務境界を崩さない。
- Coder は破壊的操作を直接実行せず、plan / patch / proposal を生成する。
- Worker は実行主体として file / shell / git / test / patch 適用を担当する。
- fallback は正常系ではない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- archive 文書を一次参照にしない。
