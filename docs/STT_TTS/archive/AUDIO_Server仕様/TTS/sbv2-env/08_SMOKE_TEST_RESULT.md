# SBV2（server_editor）スモークテスト実施メモ

実施環境: `http://127.0.0.1:8000`（Style-Bert-VITS2 `server_editor.py`）

## 実施日

2026-04-06（自動テスト実行）

## 結果

| 手順 | 期待 | 結果 |
|------|------|------|
| `GET /api/version` | 200、バージョン文字列 | OK（例: `"2.7.0"`） |
| `GET /api/models_info` | 200、モデル配列 | OK（複数モデル） |
| `POST /api/g2p` | `mora`/`tone` 配列 | OK |
| `POST /api/synthesis` | `audio/wav`、非ゼロ長 | OK |

## 注意（`modelFile`）

`POST /api/synthesis` の `modelFile` は **ファイル名だけでは失敗しうる**（500）。  
**`GET /api/models_info` の該当モデルの `files[]` の文字列をそのまま**渡す（例: `model_assets\jvnv-F1-jp\jvnv-F1-jp_e160_s14000.safetensors`）。

## 再実行用ワンライナー（Python）

```powershell
cd D:\RenCrow\devices\SBV2\Style-Bert-VITS2
python -c "import json,urllib.request; B='http://127.0.0.1:8000'; mi=json.loads(urllib.request.urlopen(B+'/api/models_info').read()); m=next(x for x in mi if x['name']=='jvnv-F1-jp'); t='\u3053\u3093\u306b\u3061\u306f'; mo=json.loads(urllib.request.urlopen(urllib.request.Request(B+'/api/g2p',data=json.dumps({'text':t}).encode(),headers={'Content-Type':'application/json'})).read()); b={'model':'jvnv-F1-jp','modelFile':m['files'][0],'text':t,'moraToneList':mo,'speaker':'jvnv-F1-jp'}; w=urllib.request.urlopen(urllib.request.Request(B+'/api/synthesis',data=json.dumps(b,ensure_ascii=False).encode(),headers={'Content-Type':'application/json'}),timeout=180).read(); print(len(w))"
```

成功時、最後に WAV のバイト数が表示される。
