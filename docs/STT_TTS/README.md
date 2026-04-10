# STT_TTS ドキュメント分類

`docs/STT_TTS` は以下の3区分で管理する。

## 1. STT
STT（Whisper / voice-bridge）に関する仕様・実装状況。

配置先: `docs/STT_TTS/STT`

主なファイル:
- `STT仕様.md`
- `STT_現状実装仕様.md`
- `STT_Whisper実装仕様.md`
- `STT_WHISPER_SPEC_SUMMARY.md`
- `STT_実装状況.md`
- `10_WHISPER_REMOTE_PC.md`

## 2. TTS
TTS（SBV2 等）に関する仕様・API 契約・運用情報。

配置先: `docs/STT_TTS/TTS`

主なファイル:
- `12_SBV2_TTS_現状仕様.md`
- `SBV2_SERVER_TEAM_CONTACT_QUESTIONS.md`

## 3. COMMON（STT/TTS 共通事項）
STT/TTS 共通で参照する接続方針・移行・CORS などの共通知識。

配置先: `docs/STT_TTS/COMMON`

主なファイル:
- `STT_TTS_接続基本事項.md`
- `11_WIN11_HP01_SERVER_MIGRATION.md`
- `09_BROWSER_AND_CORS.md`

## 4. AUDIO_Server仕様（現状サーバ仕様）

音声サーバの現状仕様は `docs/STT_TTS/AUDIO_Server仕様` を参照する。

配置構成:
- `docs/STT_TTS/AUDIO_Server仕様/STT`（STT サーバ側仕様）
- `docs/STT_TTS/AUDIO_Server仕様/TTS`（TTS サーバ側仕様）
- `docs/STT_TTS/AUDIO_Server仕様/COMMON`（共通仕様）
- `docs/STT_TTS/AUDIO_Server仕様/Chat_Server`（Chat サーバ連携メモ）

注意:
- 基本事項として STT/TTS は Chat サーバ経由接続を優先する
- 現状仕様と正本仕様に差分がある場合は、差分を明記して段階的に同期する

## 運用ルール
- 新規ドキュメント作成時は、まず STT / TTS / COMMON のどれに属するかを決める
- STT/TTS 両方に関わる方針・制約・移行手順は `COMMON` に置く
- ファイル移動時は、関連ドキュメントの参照パスを同時に更新する
