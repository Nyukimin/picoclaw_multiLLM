# 再現範囲と用語

## 目的

別 PC で次の状態を **ビット単位ではなく手順・構成として再現**する。

- `python server_editor.py`（引数なし）が **TCP 8000** で待ち受ける
- **`GET http://127.0.0.1:8000/api/version`** が応答する
- **`GET http://127.0.0.1:8000/api/models_info`** が少なくとも 1 モデルを返す
- **`POST /api/g2p` → `POST /api/synthesis`** で **WAV** が返る

## 「SBV2 の基本部分以外」とするもの

次は **本ドキュメント群では詳細を固定しない**（上流の Style-Bert-VITS2 本体の領域）。

- 声質モデルの学習パイプライン・ハイパーパラメータ
- `model_assets` 内の **独自モデル**の中身（ファイル名・話者設計など）

ただし **`model_assets` のディレクトリ構造**（`config.json`・`*.safetensors`・`style_vectors.npy`）は推論に必須のため、[04_INITIALIZE_DATA_AND_MODELS.md](./04_INITIALIZE_DATA_AND_MODELS.md) で固定する。

## 本書で固定する「実装仕様」

| 区分 | 固定内容 |
|------|----------|
| 起動 | `server_editor.py` に **追加 CLI 引数を付けない**（RenCrow `start-sbv2.ps1` と同じ） |
| 作業ディレクトリ | リポジトリルート（`server_editor.py` があるディレクトリ） |
| ポート | **8000**（`--port` 未指定時の既定） |
| パス設定 | `configs/paths.yml` の `assets_root` が **`model_assets`** を指す（既定） |
| 仮想環境 | `venv\Scripts\python.exe` を最優先（`start-sbv2.ps1` の解決順） |

## 再現性を最大化する運用

1. 動作確認済み PC で **`pip freeze`** を取得し、チームで共有する（[03_PYTHON_INSTALL_AND_LOCKFILE.md](./03_PYTHON_INSTALL_AND_LOCKFILE.md)）。
2. **Python のマイナーバージョン**をそろえる（例: 3.10.x 同士）。
3. **CUDA / PyTorch の組み合わせ**を README の cu118 手順に合わせるか、CPU 固定なら **CPU 用 torch のみ**に統一する。
