# 他 PC 構築チェックリスト（コピー用）

- [ ] OS: Windows 10/11、パスに **日本語・空白なし**
- [ ] Python **3.9〜3.11** のうち、チームで決めた **1 つのマイナー**をインストール
- [ ] `devices/SBV2/Style-Bert-VITS2` を **同一相対配置**で配置（RenCrow クローン推奨）
- [ ] `py -m venv venv` → `venv\Scripts\activate`
- [ ] PyTorch: **GPU** → README の `cu118` + `torch<2.4` / **CPU** → 公式の CPU wheel に統一
- [ ] `pip install -r requirements.txt`（`devices/SBV2/Style-Bert-VITS2/requirements.txt`）
- [ ] `pip install fastapi "uvicorn[standard]" scipy requests`（不足時のみ）
- [ ] `python initialize.py --only_infer`（オンライン・HF 取得可の環境で）
- [ ] `configs/paths.yml` あり、`assets_root: model_assets`
- [ ] `model_assets` に少なくとも 1 モデル（`initialize` 既定 or 手動コピー）
- [ ] 初回 `server_editor.py` 起動で **`static/`** 取得 or 動作 PC から `static/` コピー
- [ ] 起動: `python server_editor.py`（**引数なし**）、`WorkingDirectory` = SBV2 ルート
- [ ] `http://127.0.0.1:8000/api/version` が **200**
- [ ] `http://127.0.0.1:8000/api/models_info` が **200** かつ **1 件以上**
- [ ] （任意）動作 PC の `pip freeze` を **`sbv2-pip-freeze.lock.txt`** として保管し、次回は `pip install -r` で固定
