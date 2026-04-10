# Python・仮想環境・依存パッケージ

## Python バージョン

- **公式**: `pyproject.toml` では `requires-python = ">=3.9"`（3.9 / 3.10 / 3.11 が列挙）
- **再現性**: チームで **同一マイナー**にそろえる（例: **3.10.11** 固定）

## 仮想環境の場所（RenCrow と一致）

`start-sbv2.ps1` の `Resolve-PythonExe` は次の順で探す。

1. `{RepoDir}\venv\Scripts\python.exe`
2. `{RepoDir}\.venv\Scripts\python.exe`
3. `{RepoDir}\Scripts\python.exe`

**推奨**: `Style-Bert-VITS2` ルートに **`venv`** を作る。

```powershell
cd D:\path\to\RenCrow\devices\SBV2\Style-Bert-VITS2
py -3.10 -m venv venv
.\venv\Scripts\Activate.ps1
```

## PyTorch の入れ方（README 準拠・GPU / CUDA）

公式 README（リポジトリ `README.md`）の例:

```powershell
pip install "torch<2.4" "torchaudio<2.4" --index-url https://download.pytorch.org/whl/cu118
```

- **GPU なし**の PC では、上記の代わりに **CPU 向け**の PyTorch を公式サイトのコマンドで入れるか、README の `Install-Style-Bert-VITS2-CPU.bat` 相当の手順に合わせる。
- `server_editor.py` は **`cuda` 不可時に自動で `cpu` に落ちる**が、**torch が CPU ビルドでないと**インストール時点で失敗する場合があるため、マシンに合わせた wheel を選ぶ。

## 依存一覧のインストール

リポジトリ同梱の **`requirements.txt`** を正とする（ルート: `devices/SBV2/Style-Bert-VITS2/requirements.txt`）。

```powershell
pip install -U pip
pip install -r requirements.txt
```

### 注意（Windows）

- `onnxruntime-directml` は **Windows** 用マーカー付き行で入る（`requirements.txt` 内の `sys_platform == 'win32'`）
- `numpy<2`、**`torch<2.4`** など **ピン** がある行はそのまま守る

### `server_editor.py` に間接的に必要なもの

- `gradio` 等が **fastapi / uvicorn** を引きずる想定だが、`pip check` で不足があれば次を明示追加する。

```powershell
pip install "fastapi" "uvicorn[standard]" "scipy" "requests"
```

（`scipy` は `wavfile` 用。`requirements.txt` に含まれない場合でも **librosa** 依存等で入ることが多い。）

## パッケージを「完全固定」する手順（推奨）

1. 動作している PC で venv を有効化し、次を実行する。

```powershell
pip freeze | Out-File -Encoding utf8 sbv2-pip-freeze.lock.txt
```

2. 別 PC では **同じ Python マイナー** を入れたうえで:

```powershell
pip install -r sbv2-pip-freeze.lock.txt
```

3. ロックファイルを **Git または社内共有フォルダ**に置く（機密に注意）。

**注意**: フルフリーズは **CUDA 版 torch / CPU 版 torch** で別ファイルに分けると齟齬が減る。

## 開発モード（パッケージとしての style-bert-vits2）

`pip install -e .` は **任意**（`server_editor.py` はリポジトリ上の `style_bert_vits2` を直接 import）。RenCrow の `start-sbv2.ps1` は **`pip install -e` を要求しない**。
