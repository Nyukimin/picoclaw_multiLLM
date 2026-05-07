# STT_TTS 実装ガイド

`docs/STT_TTS` は、音声仕様を **Client仕様**（Chat視点）と **Server仕様**（提供側視点）で管理する。  
本ページは実装着手の起点とする。

## 1. 先に決めること（実装前）
- STT/TTS は Chat サーバ経由を維持する
- STT/TTS は Provider 非依存契約で記述する
- 音声は単発だけでなく複数本（複数チャンク/複数トラック）を前提とする

## 2. 実装順序（推奨）
1. `AUDIO_Server仕様` で API/実装仕様を確定
2. `AUDIO_Client仕様` で Chat 呼び出し契約を同期
3. 両者のフィールド整合（必須/任意/型）を確認
4. 受け入れ基準で疎通テスト

## 3. 参照先

### Client仕様（Chat から見た契約）
- `docs/STT_TTS/AUDIO_Client仕様/README.md`

### Server仕様（提供側契約）
- `docs/STT_TTS/AUDIO_Server仕様/README.md`

### 現行外部連携メモ
- `docs/STT_TTS/STT_Remote_HTTPS仕様.md` - Mac上の `rencrow-stt` をHTTPS reverse proxy越しに使う仕様
- `docs/STT_TTS/STT_Streaming_Client仕様.md` - Mac上の `rencrow-stt` へWSS PCM chunkを送るブラウザクライアント仕様
- `docs/STT_TTS/IrodoriTTS_HTTP_API仕様.md` - Mac上のIrodori-TTS Gradio HTTP API仕様

### 旧資料
- `docs/STT_TTS/archive`

## 4. 実装完了条件（DoD）
- Client/Server で API 名称・必須フィールド・エラー契約が一致
- Provider 名称が本文契約を汚染しない（実装例セクションのみ）
- 複数本音声（chunk/track）契約が API と実装仕様に明記されている
- README から着手順序が一意に辿れる

## 5. API-DOD運用
- API文書の `API-DOD-*` を実装/レビューのチェックIDとして使用する
- 命名規則: `API-DOD-<領域>-<側>-<連番>`
  - 領域: `STT` または `TTS`
  - 側: `C`（Client）または `S`（Server）
- 仕様変更時は関連する `API-DOD-*` も同時更新する
- 各IDの検証コマンド例と証跡記録欄は、各 `*_API.md` の API-DOD 節を正本とする
- PR単位の集約判定は `docs/STT_TTS/API_DOD_CHECKLIST.md` へ記録する

## 6. 運用ルール
- 正規仕様は `AUDIO_Client仕様` / `AUDIO_Server仕様` のみ
- 旧文書は削除せず `archive` へ移動
- 仕様更新時は Client/Server を同一PRで同期
- レビュー記録は `docs/STT_TTS/API_DOD_CHECKLIST.md` を利用する

## 7. 再起動運用定義
- RenCrow の規定ポートは `18790` に固定する。
- 「再起動」の操作定義は以下とする。
  1. 動作中 RenCrow を停止（Kill）する
  2. RenCrow を起動する
  3. `GET /health` と `GET /ready` を確認する
- ポート競合がある場合は、競合プロセスを停止してから再起動する。
