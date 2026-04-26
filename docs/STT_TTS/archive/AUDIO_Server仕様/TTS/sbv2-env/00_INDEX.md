# SBV2 実行環境の再現（ドキュメント索引）

他 PC で **RenCrow と同一の Style-Bert-VITS2 Editor サーバ**（`server_editor.py`）環境を再現するための仕様書です。

**対象外（「SBV2 の基本部分」に含まれるもの）**

- Style-Bert-VITS2 の学習手順・学習アルゴリズムの説明
- 独自学習モデルの作り方（本書は **推論サーバの環境**のみ固定）

**本書で 100% 固定するもの**

- OS・ディレクトリ規約・Python 手順・依存関係の入れ方
- `initialize.py`・`configs/paths.yml`・`model_assets` の関係
- RenCrow の `start-sbv2.ps1` と等価な起動仕様（引数・作業ディレクトリ・ポート・ログ）
- 動作確認コマンド

## 読む順序

| # | ファイル | 内容 |
|---|----------|------|
| 1 | [01_SCOPE.md](./01_SCOPE.md) | 再現範囲・用語 |
| 2 | [02_REPOSITORY_LAYOUT_AND_PATHS.md](./02_REPOSITORY_LAYOUT_AND_PATHS.md) | 配置・パス規約 |
| 3 | [03_PYTHON_INSTALL_AND_LOCKFILE.md](./03_PYTHON_INSTALL_AND_LOCKFILE.md) | venv・PyTorch・requirements |
| 4 | [04_INITIALIZE_DATA_AND_MODELS.md](./04_INITIALIZE_DATA_AND_MODELS.md) | `initialize.py`・BERT・既定モデル |
| 5 | [05_START_SCRIPT_AND_RUNTIME.md](./05_START_SCRIPT_AND_RUNTIME.md) | 起動行・ポート・ログ・ミューテックス |
| 6 | [06_VERIFICATION_AND_TROUBLESHOOTING.md](./06_VERIFICATION_AND_TROUBLESHOOTING.md) | 検証・切り分け |

### Whisper（別 PC・voice-bridge）

| # | ファイル | 内容 |
|---|----------|------|
| — | [10_WHISPER_REMOTE_PC.md](./10_WHISPER_REMOTE_PC.md) | 別 PC の whisper-server と `WHISPER_URL`（`09` と同形式） |

## 一次情報（リポジトリ内）

- サーバ実装: `devices/SBV2/Style-Bert-VITS2/server_editor.py`
- 起動スクリプト: `ops/audioio/start-sbv2.ps1`
- 依存一覧: `devices/SBV2/Style-Bert-VITS2/requirements.txt`
- API 仕様: `docs/SBV2_SERVER_IMPLEMENTATION_REQUIREMENTS.md`
