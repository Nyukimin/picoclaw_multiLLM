# Marin Live2D Data Package

これは、MarinをLive2D Cubism向けに制作するための入稿・設計データです。
現在の画像は1枚絵のキャラクターシートなので、このZIPには完成済みの `.moc3` ではなく、**レイヤー分解PSDを作るための仕様・参照画像・リギング表**を入れています。

## 内容

- `references/`：元シート、全身、表情、三面図、ディテールの参照PNG
- `references/Marin_cut_guide.png`：Live2D分割の目安ガイド
- `tables/parts_layer_list.csv`：PSDレイヤー分け一覧
- `tables/parameter_table.csv`：Cubismパラメータ一覧
- `tables/expression_table.csv`：表情プリセット案
- `docs/PSD_LAYER_TREE.txt`：PSDレイヤー構成案
- `live2d_scaffold/`：Cubism書き出し後に使う設定テンプレート

## 重要

このパッケージだけではLive2D Cubismでそのまま動きません。次に必要なのは、参照画像をもとにした**レイヤー分解PSD**です。
1枚絵から自動分割しただけでは、まばたき・口パク・髪揺れ・腕揺れで隠れ部分が破綻しやすいため、Live2D用に描き足しが必要です。

## 生理学制約

- 腕：2本
- 脚：2本
- 指：左右5本ずつ
- 手の差分を作る場合も、余分な指や腕を増やさないこと

## 推奨制作順

1. `references/Marin_fullbody_reference.png` を下絵にして、3000x4500px程度で描き直す
2. `tables/parts_layer_list.csv` に沿ってPSDレイヤー分け
3. 目・口・眉・髪・腕・スカート・チャームに隠れ部分を描き足す
4. Live2D CubismにPSDを読み込み、ArtMeshとDeformerを設定
5. `tables/parameter_table.csv` のパラメータを作成
6. `tables/expression_table.csv` を参考に表情を作成
7. `live2d_scaffold/` のテンプレートをCubism書き出し後のファイル名に合わせて調整

## 最低限のLive2D可動範囲

- 顔：Angle X/Y/Z
- 目：まばたき、視線移動、ウインク
- 口：開閉、笑顔・むすっと口
- 眉：上下、困り、怒り
- 体：軽い左右揺れ、呼吸
- 物理：髪、アホ毛、スカート、チェーン、バッグチャーム
