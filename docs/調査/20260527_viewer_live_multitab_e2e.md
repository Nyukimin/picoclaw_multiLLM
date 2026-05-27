# Viewer live mode multi-tab E2E 記録

## 結論

2026-05-27 に live mode の multi-tab audio owner 切替を確認した。

- Viewer A が音声 owner を claim 後、Viewer B が `speaker_unlock` で owner を奪取した。
- owner 切替後の TTS playback ACK は Viewer B から送信され、3件すべて HTTP 200 だった。
- backend の TTS pending は最終的に 0 になった。
- Viewer A には `TTS_CHUNK_TIMEOUT` も fallback 表示も出なかった。
- Viewer A には診断イベント `NON_ACTIVE_AUDIO_VIEWER_PENDING_SKIPPED` が出た。
- Viewer B の `#chat` には live IdleChat 本文が表示された。

この検証範囲では、TTS生成、active owner の表示、音声再生 ACK、pending drain は本線として成立している。

## 証跡

- 実行結果 JSON: `/tmp/rencrow_idlechat_multitab_live_e2e_20260527.json`
- Viewer A screenshot: `/tmp/rencrow_idlechat_multitab_live_e2e_A_20260527.png`
- Viewer B screenshot: `/tmp/rencrow_idlechat_multitab_live_e2e_B_20260527.png`

主な結果:

| 項目 | 結果 |
| --- | --- |
| A claim | `viewer-tab-63ad9d0f-d4e4-420f-a49b-424bb9686f90` |
| B claim | `viewer-tab-131a0923-a386-431e-9c0e-a451ff4f2438` |
| ACK sender | Viewer B |
| ACK status | 200 x 3 |
| ACK response_id | `forecast-1779853493:0000`, `forecast-1779853493:0001`, `forecast-1779853493:0002` |
| final pending | `pending_session_count=0`, `pending_response_count=0` |
| after stop pending | `pending_session_count=0`, `pending_response_count=0` |
| Viewer A timeout | なし |
| Viewer A fallback | なし |
| Viewer A diagnostic | `NON_ACTIVE_AUDIO_VIEWER_PENDING_SKIPPED` |
| Viewer B timeout | なし |
| Viewer B display | `#chat` に IdleChat 本文表示 |

## live mode の観測手順

live mode の IdleChat 表示対象は `#chat` とする。`#idleLiveLog` は通常 mode の IdleChat tab 用であり、live mode の目視対象ではない。

検証手順:

1. `/viewer/idlechat/stop` で既存セッションを停止する。
2. `/viewer/idlechat/status` で `pending_session_count=0` と `pending_response_count=0` を確認する。
3. `http://127.0.0.1:18790/viewer?mode=live` を開く。
4. hidden の IdleChat tab はクリックしない。
5. `#liveAudioBtn` をクリックして active audio owner を claim する。
6. `/viewer/idlechat/start` で IdleChat を開始する。
7. 観測対象を分けて確認する。
   - 表示: `#chat`
   - ACK: `/viewer/tts/playback-ack` の HTTP status と payload
   - pending: `/viewer/idlechat/status` の `tts_pending`
   - timeout: `TTS_CHUNK_TIMEOUT` の有無
   - fallback: fallback 表示や本文の推測表示の有無
8. 終了後 `/viewer/idlechat/stop` を実行し、pending 0 を再確認する。

## multi-tab owner 切替の期待値

Viewer A が owner を持った状態で Viewer B が audio owner を claim した場合:

- Viewer B が以後の active audio owner になる。
- Viewer B が TTS chunk を表示、再生、ACK する。
- Viewer A は non-active owner として live TTS pending timeout を armed しない。
- Viewer A が live message を受けても、fallback 表示で本文を出さない。
- Viewer A は必要に応じて診断ログに `NON_ACTIVE_AUDIO_VIEWER_PENDING_SKIPPED` を出す。
- ACK 済み判定、pending drain、session 進行の正本は backend 側にある。

## console error 分類

今回の live E2E では console error が 217 件記録された。分類すると以下の4系統だった。

| 種別 | 件数 | endpoint / 発生箇所 | 判定 |
| --- | ---: | --- | --- |
| `Failed to load resource: 503` | 62 | `/viewer/sandbox?limit=20` | Sandbox store unavailable に付随する browser resource error |
| `HTTP 503: sandbox store unavailable` | 31 | `refreshSandboxData()` | Sandbox disabled / store unavailable の診断。TTS/ACK本線ではない |
| `Failed to load resource: 404` | 62 | `/viewer/verification/recent`, `/viewer/verification/summary` | Verification API route 未登録に付随する browser resource error |
| `HTTP 404: 404 page not found` | 62 | `refreshVerification()`, `refreshVerificationSummary()` | Verification runtime/store 未初期化により route が未登録。TTS/ACK本線ではない |

実 endpoint 確認:

| endpoint | HTTP | 内容 |
| --- | ---: | --- |
| `/viewer/sandbox?limit=20` | 503 | `sandbox store unavailable` |
| `/viewer/evidence/summary` | 200 | evidence summary JSON |
| `/viewer/verification/recent?limit=20` | 404 | `404 page not found` |
| `/viewer/verification/summary` | 404 | `404 page not found` |
| `/viewer/idlechat/status` | 200 | IdleChat status JSON |

これらは現時点では live IdleChat / TTS / ACK の失敗証跡ではない。ただし console を汚し、実際の Viewer runtime error と混ざるため、P2 の診断ノイズとして別件整理する価値がある。

## 修正要否

今回の TTS/ACK 本線確認を阻害する P0/P1 は確認されなかった。

残課題:

- P2: live mode でも Ops / Evidence 系の定期 refresh が走り、使わない optional endpoint の 503/404 が console error として増える。
- P2: `/viewer/verification/*` は runtime 無効時に route 未登録となるため、Viewer 側には明示的な unavailable 診断として扱う余地がある。
- P2: `/viewer/sandbox` は unavailable を 503 で返しており、Viewer 上のエラー表示としては正しいが、live mode の IdleChat 検証ではノイズになる。

推奨する次の最小修正:

1. live mode では非表示 panel の optional refresh を抑止する、または unavailable を console error ではなく panel 内診断に限定する。
2. Verification runtime 無効時の挙動を決める。
   - backend で unavailable handler を登録する。
   - または Viewer が 404 を optional unavailable として扱い、console error にしない。
3. 上記は TTS/ACK 本線とは別 commit にする。

## 再実行時の注意

- `page.goto(..., waitUntil: 'networkidle')` は使わない。SSE や定期 refresh で待ち続ける可能性がある。
- live mode では hidden IdleChat tab の visible 待ちをしない。
- `#chat` に表示される本文、ACK payload、pending status、timeout/fallback を分けて判定する。
- `chunk_ready` や `tts.audio_chunk` だけで ACK 成功と判断しない。
- `TTS_CHUNK_TIMEOUT` が出た場合は、active owner が chunk を受け取れなかったのか、non-active owner が pending timeout を armed したのかを `viewer_client_id` と `active_audio_viewer_id` で分ける。
