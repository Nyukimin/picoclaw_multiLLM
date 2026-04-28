# STT サーバーログ出力 依頼文（2026-04-13）

以下、実装担当への依頼文です。

---

## 件名
`/stt-ws` 用 STTイベントログ（[stt] JSON）出力のお願い

## 背景
- クライアント側ログは仕様どおり取得できています。
- `/stt-ws` 経路自体は自動テストで `speech_start/draft/final` を確認済みです。
- ただし、サーバー側ログが `ConnState` 中心のため、`compare_stt_logs.py` で `server_events=0` になります。

## 依頼内容
`/stt-ws` 経路の client/server 突合を可能にするため、サーバー側で STTイベントログを機械可読形式で出力してください。

## 必須ログ形式
以下の JSON を `[stt]` プレフィックス付き 1行で出力してください。

```text
[stt] {"event":"speech_start|draft|final|provider_success|provider_exception|...","ts":"<ISO8601>","session_id":"<session_id>","text":"<draft/final text>"}
```

### 必須キー
- `event`
- `ts`
- `session_id`

### `draft` / `final` 時の必須キー
- `text`

## 最低限必要なイベント
- `speech_start`
- `draft`
- `final`

## 参考ログ例
```text
[stt] {"event":"speech_start","ts":"2026-04-13T02:51:41.120Z","session_id":"sess-mnxxxx-abc123"}
[stt] {"event":"draft","ts":"2026-04-13T02:51:42.010Z","session_id":"sess-mnxxxx-abc123","text":"おはようございます"}
[stt] {"event":"final","ts":"2026-04-13T02:51:43.220Z","session_id":"sess-mnxxxx-abc123","text":"おはようございます。"}
[stt] {"event":"provider_success","ts":"2026-04-13T02:51:43.300Z","session_id":"sess-mnxxxx-abc123","phase":"final","result":"success"}
```

## 受け入れ条件
1. 同一 `session_id` で `speech_start -> draft -> final` が `[stt]` JSON で出力される
2. `[stt]` の全行に `session_id` が含まれる
3. `draft/final` 行に `text` が含まれる
4. 比較スクリプト実行時に `server_events > 0` となる

## 共有データ（再現用）
- `tmp/stt_share_for_server.md`（共有対象ファイル一覧 + sha256）
- `tmp/client_stt_log.txt`
- `tmp/voice_bridge_20260413_115141_115148.log`（現行サーバーログ切り出し）
- `tmp/stt_compare_report_latest.md`（現状: `server_events=0`）
- `tmp/stt_e2e_from_mic_latest.json`（/stt-ws で event受信確認済み）

## 比較コマンド
```bash
python3 docs/STT_TTS/tools/compare_stt_logs.py \
  --client-log "tmp/client_stt_log.txt" \
  --server-log "tmp/voice_bridge_YYYYMMDD_HHMMSS_HHMMSS.log" \
  --output "tmp/stt_compare_report_latest.md"
```

## 補足
- `ConnState` のみでは本文突合ができません。
- `session_id` を軸に client/server を合わせて比較します。
- 既存運用ログはそのまま残し、追加で `[stt]` 行を出していただければ問題ありません。

---
