# STT サーバーログ出力 依頼文（パス非依存版 / 2026-04-13）

以下、実装担当への依頼文です。

---

## 件名
`/stt-ws` 用 STTイベントログ（`[stt]` JSON）出力のお願い

## 背景
- クライアント側ログは仕様どおり取得できています。
- `/stt-ws` 経路自体は自動テストで `speech_start/draft/final` を確認済みです。
- ただし、サーバー側ログが `ConnState` 中心のため、本文（`draft/final`）の突合ができません。

### 現時点の観測結果（要約）
- 比較結果は `server_events=0`（サーバーログから STT本文イベントを抽出不可）
- 一方で疎通テスト自体は成功（WS経路で `speech_start/draft/final` を受信）

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
4. 比較スクリプトで `server_events > 0` になり、client/server 突合が可能になる

## 実行例（環境依存プレースホルダ）
```bash
python3 <COMPARE_SCRIPT_PATH> \
  --client-log "<CLIENT_LOG_PATH>" \
  --server-log "<SERVER_LOG_PATH>" \
  --output "<COMPARE_REPORT_PATH>"
```

## 共有物（任意だが推奨）
- `<STT_SHARE_MEMO_PATH>`（共有対象一覧、ハッシュ、観測メモ）
- `<CLIENT_LOG_PATH>`
- `<SERVER_LOG_PATH>`
- `<COMPARE_REPORT_PATH>`
- `<STT_E2E_RESULT_PATH>`

## 重要（誤解防止）
- パス/ディレクトリ構成は環境依存です。`*_PATH` は各環境の実在パスに置換してください。
- 評価対象はパスではなく、`[stt]` ログの `event / session_id / text` が出力されることです。
- 既存の `ConnState` ログ運用は維持し、追加で `[stt]` 行を出力していただければ問題ありません。

---
