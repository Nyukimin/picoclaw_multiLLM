# STT 共有データ一覧（サーバー担当向け）

作成日時: 2026-04-13 13:03:xx（自動生成）

---

## 観測サマリ

| 項目 | 結果 |
|---|---|
| STT 推論（HTTP直接 `/inference`） | **1/1 成功**（553.4 ms） |
| `/stt-ws` WebSocket 経路 | **1/1 成功** |
| `session_id` | **`(unknown)` のまま**（サーバーから `session_info` 未受信） |
| 発話内容（ブラウザ側 final） | `あお。` |
| 推論結果（e2e probe）| `AEUAO.O.HAYO.O.ございます。 天使。` |
| **ブラウザ/e2e 結果の一致** | **不一致**（同一 WAV から別の転写が得られた） |

### 問題点

1. **session_id が (unknown)**: サーバーが `session_info` メッセージを WebSocket 経由で送っていない（または届いていない）。
2. **転写テキストの乖離**:
   - ブラウザ（ストリーミング推論）→ `あお。`
   - e2e probe（バッチ推論、同一 WAV ファイル）→ `AEUAO.O.HAYO.O.ございます。 天使。`
   - 同一音声なのに結果が大きく異なる。ストリーミングとバッチで処理経路が違う可能性。
3. **draft の揺れが激しい**: 22:03:03〜22:03:10 の間に `エイイ!` / `うえ` / `エオ` / `おあ` / `あお。` など多数の draft が出た後、final = `あお。` に落ち着いた。

---

## 共有ファイル一覧

### メインログ

| ファイル | 説明 | サイズ | sha256 |
|---|---|---|---|
| `tmp/client_stt_log.txt` | ブラウザ側 STT イベントログ | 706 B | `ec2bae9b…785cf434` |
| `tmp/stt_e2e_from_mic_latest.json` | e2e probe 実行結果 | 620 B | `833b3ade…c2874ca` |
| `tmp/client_stt_input_latest.wav` | 最新マイク入力 WAV | 281 KB | `c743f05e…aca69c5` |

### アーカイブ WAV

| ファイル |
|---|
| `tmp/stt_inputs/client_stt_input_20260413_130311.wav` |

---

## クライアントログ詳細（client_stt_log.txt）

```
client_url: https://fujitsu-ubunts.tailb07d8d.ts.net/viewer
ws_url:     wss://fujitsu-ubunts.tailb07d8d.ts.net/stt-ws
test_time:  2026/4/13 22:03:03 ~ 22:03:10
session_id: (unknown)
spoken_text: あお。

イベント列（新→旧）:
  22:03:10 · draft  → ています。
  22:03:09 · draft  → ありがとうございます。
  22:03:09 · draft  → おさえます。
  22:03:08 · draft  → おさえ
  22:03:08 · draft  → おはよう
  22:03:08 · speech_start
  22:03:08 · final  → あお。
  22:03:06 · draft  → あお。
  22:03:06 · draft  → おあ
  22:03:05 · draft  → エオ
  22:03:05 · draft  → うえ
  22:03:04 · draft  → イー ウー
  22:03:04 · draft  → エイイ!
  22:03:03 · draft  → はえ
  22:03:03 · draft  → あ。
  22:03:03 · speech_start
```

---

## e2e probe 詳細（stt_e2e_from_mic_latest.json）

```
provider_url:   http://192.168.1.36:8080/inference
ws_url:         ws://127.0.0.1:18790/stt-ws
wav:            tmp/client_stt_input_latest.wav
timestamp:      2026-04-13 13:03:12

[HTTP推論]
  #1: ok=true, 553.4ms
      text = "AEUAO.O.HAYO.O.ございます。 天使。"

[WebSocket]
  #1: events=[speech_start, draft, final]
      final = "AEUAO.O.HAYO.O.ございます。 天使。"
      ok=true

inference_success: 1/1
ws_success:        1/1
```

---

## 比較コマンド（サーバーログ入手後に実行）

```bash
python3 docs/STT_TTS/tools/compare_stt_logs.py \
  --client-log "tmp/client_stt_log.txt" \
  --server-log "tmp/voice_bridge_YYYYMMDD_HHMMSS_HHMMSS.log" \
  --output "tmp/stt_compare_report_latest.md"
```

---

## 依頼文リンク

- サーバーログ出力依頼（パス非依存版）: `docs/STT_TTS/AUDIO_Client仕様/STT/stt_server_logging_request_path_agnostic_2026-04-13.md`
- session_id 問題 証拠付き問い合わせ: `docs/STT_TTS/AUDIO_Client仕様/STT/stt_server_inquiry_with_proof_2026-04-13.md`
