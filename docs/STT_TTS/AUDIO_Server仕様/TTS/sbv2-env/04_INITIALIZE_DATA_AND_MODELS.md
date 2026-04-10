# initialize・データ取得・モデル配置

## ネットワーク

初回は **Hugging Face Hub** から BERT・既定 TTS モデル等を取得する。**オフラインのみの PC では同一ファイルをコピーする必要がある**。

## `initialize.py` の役割

`devices/SBV2/Style-Bert-VITS2/initialize.py` は主に次を行う。

1. **BERT**（`bert/bert_models.json` に基づく）を `bert/` 以下にダウンロード
2. **`--skip_default_models` 未指定**なら **既定の `model_assets` 用ファイル**（JVNV 等）を取得
3. **`--only_infer` 未指定**なら学習用プリトレイン等も取得（推論サーバだけなら省略可）
4. **`configs/paths.yml`** が無ければ `default_paths.yml` をコピー

## 推論サーバのみ（RenCrow の推奨に近い）

```powershell
cd D:\path\to\RenCrow\devices\SBV2\Style-Bert-VITS2
.\venv\Scripts\Activate.ps1
python initialize.py --only_infer
```

- **`--only_infer`**: `download_slm_model` / `download_pretrained_models` / `download_jp_extra_pretrained_models` を **スキップ**する
- BERT と **既定モデル**（`download_default_models`）は **`--skip_default_models` 未指定**なら入る

## 既定モデルを入らない（空の model_assets から手動配置）

```powershell
python initialize.py --only_infer --skip_default_models
```

その場合は **`model_assets`** に自前で `config.json` / `*.safetensors` / `style_vectors.npy` を配置する必要がある（[02_REPOSITORY_LAYOUT_AND_PATHS.md](./02_REPOSITORY_LAYOUT_AND_PATHS.md)）。

## Editor 静的ファイル（`static/`）

`server_editor.py` を **`python server_editor.py` で直接起動**した場合（`__main__`）:

- `skip_static_files` が **False**（既定）なら、GitHub **Style-Bert-VITS2-Editor** の `out.zip` を取得して `static/` に展開する
- **`--skip_static_files`** を付けると **ダウンロードをスキップ**（API は動くがブラウザ UI が無い可能性）

**RenCrow の `start-sbv2.ps1` は CLI 引数を付けない**ため、**初回起動は GitHub から static を取りに行く**（ネットワーク必須）。オフライン再現では **動作済み PC の `static/` フォルダを丸ごとコピー**する。

## ユーザー辞書・pyopenjtalk

- `server_editor.py` 起動時に **`update_dict()`** が走り、辞書データはリポジトリ同梱の仕組みに依存する
- **同じリポジトリのコピー**を使う限り、通常は追加手順なし

## `configs/paths.yml` の上書き

`initialize.py` は `--assets_root` / `--dataset_root` で **YAML を書き換え可能**。  
RenCrow 既定では **変更不要**（`assets_root: model_assets`）。
