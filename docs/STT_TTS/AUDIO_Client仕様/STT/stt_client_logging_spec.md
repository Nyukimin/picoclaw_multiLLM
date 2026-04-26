# STT クライアントログ仕様書

## 1. 目的
- STT クライアントのイベントを、サーバーログと突合可能な形式で記録する。
- `session_id` を軸に、同一セッションの `speech_start` / `draft` / `final` を比較可能にする。

## 2. 対象
- `viewer` など WebSocket で STT イベントを受信するクライアント実装。
- 接続先は `ws://.../stt-ws` または `wss://.../stt-ws` を想定。

## 3. ログフォーマット（必須）
### 3.1 ヘッダ部
以下キーをテキスト先頭に記録する。

```text
# Client STT Log
client_url: <クライアントURL>
ws_url: <WebSocket URL>
test_time: <YYYY/M/D HH:MM:SS ~ YYYY/M/D HH:MM:SS>
session_id: <session_id>
spoken_text: <発話内容の要約>
```

### 3.2 イベント部
1イベントを2行で記録する。

1. `HH:MM:SS · event_type`
2. `payload`（なければ `-`）

例:
```text
11:18:20 · speech_start
-
11:18:22 · draft
おはようございます
11:18:23 · final
おはようございます。
```

## 4. イベント定義
- `speech_start`
  - 発話開始を示すイベント
  - payload は `-`
- `draft`
  - 暫定認識テキスト
  - payload は文字列（空文字は記録しない）
- `final`
  - 確定認識テキスト
  - payload は文字列（空文字は記録しない）

## 5. session_id 仕様
- サーバーから `session_info` イベントで受信した `session_id` をヘッダに保存する。
- 1ログファイル内は単一 `session_id` を原則とする。
- `session_id` が取れない場合は `session_id: (unknown)` を記録する。

## 6. 時刻仕様
- 表示時刻は `HH:MM:SS`（24時間制）とする。
- クライアント側時刻で記録する。
- サーバーとの時差がある環境では、`test_time` にタイムゾーン情報を補足してよい。

## 7. 受け入れ基準
- ヘッダに `client_url`, `ws_url`, `test_time`, `session_id` が存在する。
- イベント行が `HH:MM:SS · event_type` 形式である。
- `speech_start` の payload は `-`。
- `draft` / `final` の payload は文字列。
- ログファイルを `tools/stt/compare_stt_logs.py` へ渡して比較レポートを生成できる。

## 8. 推奨保存先
- `/mnt/d/RenCrow/tmp/tmp/client_stt_log.txt`

## 9. 比較コマンド例
```bash
python3 /mnt/d/RenCrow/tools/stt/compare_stt_logs.py \
  --client-log "/mnt/d/RenCrow/tmp/tmp/client_stt_log.txt" \
  --server-log "/mnt/d/RenCrow/tmp/tmp/voice_bridge_YYYYMMDD_HHMMSS_HHMMSS.log" \
  --output "/mnt/d/RenCrow/tmp/tmp/stt_compare_report_latest.md"
```

## 10. 運用メモ
- `final` がない場合でもログは保存する（原因切り分けに必要）。
- `draft` のみ連続するケースは、VAD/終端判定設定の見直し対象。
- 比較時はまず `session_id` 一致を確認し、その後に本文差分を確認する。
