# 起動仕様（RenCrow `start-sbv2.ps1` と 100% 等価）

## 参照元

`ops/audioio/start-sbv2.ps1`（リポジトリ内の一次情報）

## 固定パラメータ（スクリプト内）

| 項目 | 値 |
|------|-----|
| `RepoDir`（既定） | `D:\RenCrow\devices\SBV2\Style-Bert-VITS2` 他 PC では **同じ相対パスに合わせて書き換え** |
| `ServerScript` | `Join-Path $RepoDir 'server_editor.py'` |
| `Port` | **8000** |
| `StartupTimeoutSec` | **120** |
| `MutexName` | **`Global\RenCrow-Start-SBV2`** |
| `LogDir`（既定） | `D:\RenCrow\logs\audioio` |

## 起動コマンド（実質）

```text
FilePath: <RepoDir>\venv\Scripts\python.exe  （存在しない場合は .venv / Scripts の順）
ArgumentList: server_editor.py のみ（追加なし）
WorkingDirectory: <RepoDir>
WindowStyle: Hidden
RedirectStandardOutput: <LogDir>\sbv2.stdout.log
RedirectStandardError:  <LogDir>\sbv2.stderr.log
```

## CLI 引数が無いことの意味（`server_editor.py` 既定）

`argparse` 既定値（抜粋）:

| 引数 | 既定 |
|------|------|
| `--model_dir` | `get_path_config()` の `assets_root`（通常 `model_assets`） |
| `--device` | `cuda`（CUDA 不可なら `server_editor.py` 内で `cpu` に変更） |
| `--port` | **8000** |
| `--inbrowser` | 付けない（ブラウザを自動で開かない） |
| `--line_length` / `--line_count` | **None**（行長・行数制限なし） |
| `--skip_static_files` | **False**（`__main__` で static 取得を試みる） |
| `--preload_onnx_bert` | **False** |

## 手動起動（スクリプトなしで再現）

```powershell
cd D:\path\to\RenCrow\devices\SBV2\Style-Bert-VITS2
.\venv\Scripts\python.exe server_editor.py
```

上記は **`start-sbv2.ps1` と同じ引数**。

## 先行起動の扱い

- **TCP 8000 が既に LISTEN** なら、スクリプトは **起動をスキップ**（ログに `Port 8000 is already listening`）
- **ミューテックス** `Global\RenCrow-Start-SBV2` を取得できない場合は **即 return**（別プロセスが起動中）

## 成功判定

120 秒以内に **ポート 8000 が Listen** になること。失敗時は `sbv2.stderr.log` を確認。

## バインドアドレス

`uvicorn.run(app, host="0.0.0.0", port=port)` のため、**全インターフェースで待受**（ローカル以外からも到達可能）。ローカル専用にしたい場合は **ファイアウォールで制限**するか、`server_editor.py` を変更する（本再現仕様の範囲外）。
