# RenCrow 新仕様

## このフォルダの役割

`docs/10_新仕様/` は、Phase 1 から Phase 25 までのリファクタリング結果を、今後の実装判断に使いやすい形で再整理するための仕様フォルダである。

現時点の正本仕様は引き続き `docs/01_正本仕様/実装仕様.md` とする。このフォルダは、正本仕様を置き換えるものではなく、リファクタリング後の現在構成、責務境界、検証条件をまとめた「新構成仕様」として扱う。

## 参照元

- `docs/01_正本仕様/実装仕様.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase24_最終完了判定.md`
- `docs/refactor/Phase25_組み合わせテスト設計.md`
- `docs/refactor/Phase25_組み合わせテスト実装完了判定.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`

## 文書一覧

- `新仕様_概要.md`
  - 新仕様の目的、原則、正本仕様との関係。
- `モジュール構成仕様.md`
  - リファクタリング後の主要モジュール構成と責務境界。
- `検証仕様.md`
  - Phase25 後のテスト分類、実行コマンド、外部依存の扱い。

## 基本方針

- モジュール化と疎結合を最重要原則とする。
- Chat / Worker / Coder の責務境界を維持する。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。
- repo example と live runtime config を混同しない。
- archive 文書を一次参照にしない。
