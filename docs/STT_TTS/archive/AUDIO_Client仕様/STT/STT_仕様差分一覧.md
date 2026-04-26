# STT 仕様差分一覧（2026-04-10）

## 目的
`docs/STT` 内の STT 関連文書について、仕様と現状インタフェースの差分を一目で確認できるように整理する。

## 対象文書
- `docs/STT/STT仕様.md`
- `docs/STT/STT_現状実装仕様.md`
- `docs/STT/STT_Whisper実装仕様.md`
- `docs/STT/STT_WHISPER_SPEC_SUMMARY.md`

## 差分サマリ（重要度順）

| 観点 | `STT仕様.md` | `STT_現状実装仕様.md` | `STT_Whisper実装仕様.md` | `STT_WHISPER_SPEC_SUMMARY.md` | 判定 |
|---|---|---|---|---|---|
| 通常運用の基準系統 | `server.js` を基準と明記 | `npm start -> server.js` と明記 | `server.js/server-https.js` 併記（基準明示弱め） | `server.js` 主系統と明記 | 概ね一致 |
| `MIN_AUDIO_BYTES`（主系統） | `server.js=32044` | `server.js=32044` | `256` と記載（HTTPS系の値） | `server.js=32044` | **不一致** |
| `MIN_AUDIO_BYTES`（HTTPS系） | `server-https.js=256` | `server-https.js=256` | `256` | `server-https.js=256` | 一致 |
| `final_pending` の扱い（主系統） | `server.js` は互換 no-op と明記 | no-op と明記 | 詳細記載なし（仕様.md参照誘導） | no-op と明記 | 部分不一致 |
| MIME 決定 | 主系統は WAV 前提、HTTPSは RIFF優先+mimeType | 同左 | RIFF優先+mimeType（HTTPS寄り） | 主系統WAV前提、HTTPSでmimeType | 概ね一致 |
| Whisper HTTP 契約 | `POST /inference`, multipart, `response_format=json`, `text` | 同左 | 同左 | 同左 | 一致 |
| エラー時挙動 | warn + 空文字継続 | warn + 空結果寄り継続 | `AbortError` 再スローを明記 | warn + 空結果継続 | 表現差分 |
| 起動仕様（start-whisper） | host/port/model/convert/split/環境変数 | 同左 | 同左 | 同左 | 一致 |

## 仕様と現状インタフェースの整理

- 現状インタフェースの基準は `server.js`（HTTP `:8090`、`/ws`、VAD主導フロー）。
- HTTPS 系統（`server-https.js`）は、`final_pending` 実処理・`AbortController` など別挙動を持つ。
- したがって、STT の「正本」を作る場合は、以下の2層で明示するのが安全。
  - レイヤーA: **通常運用基準（server.js）**
  - レイヤーB: **HTTPS拡張系（server-https.js）**

## 修正優先候補

1. `STT_Whisper実装仕様.md` の `MIN_AUDIO_BYTES=256` 表記に「HTTPS系基準」注記を追加する。
2. 4文書すべてに「通常運用基準は `npm start -> server.js`」を統一文言で明記する。
3. `final_pending` について、主系統 no-op / HTTPS実処理の差を共通注記に統一する。

## タスクリスト（次アクション）

- [ ] 文書正本を `STT仕様.md` に寄せるか、別ファイルに切り出すか決定
- [ ] 用語統一（主系統/通常運用基準/HTTPS系統）
- [ ] 4文書へ同一注記テンプレート反映
