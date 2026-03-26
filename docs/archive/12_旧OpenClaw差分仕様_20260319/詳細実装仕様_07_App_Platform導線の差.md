# 詳細実装仕様 07: App/Platform導線の差

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 1. 概要

RenCrow はすでに、LINE / Viewer / CLI / Chrome bridge など複数の入口を単一メッセージ処理基盤へ寄せる仕組みを持っている。  
現行の中心は次の 4 つである。

1. `/entry` の unified entry
2. `entry.stage` による進行イベント
3. Chrome bridge の status / SSE
4. Viewer / evidence による後追い導線

---

## 2. Unified Entry

### 2.1 Request

**ファイル**: `internal/adapter/entry/handler.go`

```json
{
  "platform": "line|viewer|cli|chrome",
  "channel": "line|telegram|discord|slack|viewer|local",
  "user_id": "...",
  "session_id": "...",
  "message": "..."
}
```

`HandleWithObserver(process, observer)` は以下を行う。

1. JSON decode
2. message 必須検証
3. `NormalizeEntryPlatformChannel(platform, channel)`
4. `user_id` 未指定時は `anonymous`
5. `session_id` 未指定時は `BuildSessionID(...)`
6. `process(...)` を実行
7. 結果を JSON で返す

### 2.2 Result

```go
type Result struct {
    SessionID   string
    Route       string
    JobID       string
    Response    string
    EvidenceRef string
}
```

返却 JSON には `ok`, `session_id`, `route`, `job_id`, `response`, `evidence_ref` が入る。

---

## 3. 進行イベント

### 3.1 Stage 定義

`entry.Stage` として現行で使うのは次である。

- `received`
- `planning`
- `applying`
- `verifying`
- `completed`
- `failed`

`/entry` handler は observer があれば、この順に stage を通知する。

### 3.2 EventHub 連携

**ファイル**: `cmd/picoclaw/main.go`

`entryHandler` は observer から `entry.stage` を `EventHub` へ流す。  
これにより Viewer と Chrome bridge の双方が session 単位の進行を観測できる。

イベントには少なくとも次が乗る。

- `session_id`
- `job_id`
- `route`
- `content` = stage 名

---

## 4. Chrome bridge

### 4.1 エンドポイント

**ファイル**: `internal/adapter/chrome/bridge_handler.go`

- `POST /chrome/bridge`
- `GET /chrome/bridge/status?session_id=...`
- `GET /chrome/bridge/events?session_id=...`

### 4.2 POST /chrome/bridge

入力:

```json
{
  "request_id": "...",
  "user_id": "...",
  "session_id": "...",
  "message": "..."
}
```

動作:

- `platform="chrome"`
- `channel="local"`
- `request_id` 未指定時は `req-{unixnano}`
- `session_id` 未指定時は `BuildSessionID(..., "local", userID)`

戻り値:

- `ok`
- `request_id`
- `accepted_at`
- `session_id`
- `route`
- `job_id`
- `response`
- `evidence_ref`

### 4.3 GET /chrome/bridge/status

history から対象 session の最後の `entry.stage` を引き、現在 stage / route / job_id を返す。

### 4.4 GET /chrome/bridge/events

SSE で session 単位の `OrchestratorEvent` を返す。

特徴:

- `Last-Event-ID` を解釈
- `EventHub.History()` から再送
- `session_id` でフィルタ

---

## 5. Viewer / evidence 導線

Viewer は platform 導線の一部として機能している。

現行で追えるもの:

- `Timeline` / `System` / `Progress` の live event
- `Jobs` の live jobs
- `evidence` の完了証跡
- `persisted JSON logs`

特に `evidence_ref` は `/viewer/evidence/detail?job_id=...` への導線として使える。

つまり現行の完了導線は、

1. `/entry` or bridge で job を起動
2. `entry.stage` で進行観測
3. `evidence` で完了証跡参照

という 3 層になっている。

---

## 6. OpenClaw 観点との差分

現行到達点:

- platform 非依存の `/entry` がある
- platform/channel の正規化がある
- session 導線が統一されている
- Chrome bridge が status + SSE を持つ
- 完了証跡へ Viewer から到達できる

未到達:

1. platform ごとの通知粒度切替はない
2. `request_id` の冪等性保証はない
3. UI ごとの journey service 分離はなく、observer 連携が中心
4. evidence_ref は job_id ベースで、完全な外部 URL 契約ではない

---

## 7. 確認観点

- `/entry` が `platform/channel/user/session/message` を受ける
- `NormalizeEntryPlatformChannel()` が `cli/chrome -> local` を行う
- `entry.stage` が EventHub に出る
- Chrome bridge SSE が `Last-Event-ID` 再接続に対応する
- 完了後に evidence 参照へ到達できる

以上をもって、RenCrow の app/platform 導線は「分断された入口」ではなく、unified entry と進行イベントでかなり統一された現行実装として扱う。
