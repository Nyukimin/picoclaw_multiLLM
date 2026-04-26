# 検証・トラブルシューティング

## 1. ポート確認（PowerShell）

```powershell
Get-NetTCPConnection -State Listen -LocalPort 8000
```

## 2. HTTP 検証

```powershell
Invoke-WebRequest -Uri "http://127.0.0.1:8000/api/version" -UseBasicParsing
```

- 200 かつ本文にバージョン文字列があれば **サーバ生存**

```powershell
Invoke-WebRequest -Uri "http://127.0.0.1:8000/api/models_info" -UseBasicParsing
```

- 200 かつ JSON 配列に **1 件以上**のモデルがあれば **`model_assets` 認識済み**

## 3. 合成の最小確認（手順）

1. `models_info` の **`name`** と **`files` の 1 要素**をメモする  
2. `POST /api/g2p` に `{"text":"テスト"}` を送る  
3. 返った JSON を **`moraToneList`** にそのまま載せ、`POST /api/synthesis` に  
   `model`, `modelFile`, `text`, `moraToneList` を JSON で送る  
4. `Content-Type: audio/wav` かつボディサイズ > 0

`curl` 例（g2p）:

```powershell
curl -s -X POST "http://127.0.0.1:8000/api/g2p" -H "Content-Type: application/json" -d "{\"text\":\"テスト\"}"
```

## 4. よくある失敗

| 現象 | 確認 |
|------|------|
| `Models not found` で即終了 | `model_assets` が空、`model_holder.model_names` が 0。`initialize.py` または手動配置 |
| 起動はするが初回だけ極端に遅い | BERT の初回ロード。GPU なしだとさらに長い |
| `stderr` に HF ダウンロード失敗 | プロキシ・証明書・オフライン。`initialize.py` を事前にオンラインで実行するか、ファイルをコピー |
| `static` が無く UI が出ない | `static/` をコピーするか、`--skip_static_files` が **無い**状態で起動して取得（API は別） |
| 別 PC だけ `pyopenjtalk` エラー | Python バージョン・venv 混在。`pip check` と `python -V` を一致させる |

## 5. 本ドキュメント群との整合性

API の詳細は **`docs/SBV2_SERVER_IMPLEMENTATION_REQUIREMENTS.md`** を参照。
