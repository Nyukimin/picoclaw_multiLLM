# リポジトリ配置とパス規約

## 前提

- **OS**: Windows 10 / 11（RenCrow の `start-sbv2.ps1` は PowerShell 想定）
- **パス**: Style-Bert-VITS2 の README どおり、**日本語・空白を含まないパス**に置く（`D:\RenCrow\...` のように ASCII のみ推奨）

## RenCrow 内での正規の場所

| 種別 | パス（リポジトリルートを `RenCrow` とする） |
|------|---------------------------------------------|
| SBV2 ソース・venv | `RenCrow\devices\SBV2\Style-Bert-VITS2\` |
| サーバエントリ | 上記の `server_editor.py` |
| 推論モデルルート | 上記の `model_assets\`（`configs/paths.yml` の `assets_root`） |
| パス設定 | `RenCrow\devices\SBV2\Style-Bert-VITS2\configs\paths.yml` |
| 起動ログ（RenCrow 既定） | `RenCrow\logs\audioio\` |

他 PC で **ドライブ文字や親フォルダ名が違ってよい**。重要なのは **上記の相対構造**（`devices\SBV2\Style-Bert-VITS2` 以下が同一）を保つこと。

## `configs/paths.yml`（必須）

初回に存在しない場合、`server_editor.py` 経由で `configs/default_paths.yml` がコピーされて生成される。手動でもよい。

**既定の意味**（`configs/default_paths.yml` と同じ内容でよい）:

```yaml
dataset_root: Data
assets_root: model_assets
```

- **推論のみ**なら `dataset_root` は未使用に近いが、ファイルは揃えておく。
- **`assets_root`** は **相対パス**で、基準は **リポジトリの Style-Bert-VITS2 ルート**（`server_editor.py` のカレントディレクトリ）。

## モデル配置（推論）

`model_assets` 直下に **モデル名フォルダ**が並ぶ（README 参照）。

```
model_assets
├── your_model
│   ├── config.json
│   ├── *.safetensors
│   └── style_vectors.npy
└── ...
```

`initialize.py` 実行済みなら、既定の JVNV 等がダウンロードされ、この構造になる。

## 環境変数

**RenCrow の `start-sbv2.ps1` は環境変数を設定しない**。  
再現時も **特別な `PATH` 以外の必須 env は無し**とする（Hugging Face のプロキシ等が必要なネットワークだけ別途）。

## ファイアウォール

**TCP 8000** の受信をブロックしないこと（ローカルホストのみなら通常問題なし）。
