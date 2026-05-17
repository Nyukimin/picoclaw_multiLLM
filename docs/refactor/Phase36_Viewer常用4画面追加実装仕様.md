# Phase36 Viewer 常用4画面追加実装仕様

## 目的

Phase36 では、既存 Viewer に常用入口として次の 4 画面を追加する。

- Home / Daily Desk
- Develop
- Instructions
- Reports

この 4 画面は既存タブを置き換えない。既存の `Ops` / `Overview` / `Roles` / `Progress` / `Chat Timeline` / `System` / `Memory` / `News Pack` / `IdleChat` / `Sessions` / `Jobs` は残し、内部監視・詳細確認・デバッグ用途として維持する。

Phase36 の目的は、新しい backend を先に作り直すことではない。既存 Viewer API と既存 Viewer state を利用し、人が毎日使うための上位ビューを追加することである。

## 正本仕様との関係

Viewer の既存契約は `docs/10_新仕様/05_Viewer仕様.md` を優先する。

特に次を守る。

- Viewer 表示本文、SSE event、event log、history、audio trigger、lipsync trigger、runtime config 表示を混同しない。
- 音声 chunk は本文表示の根拠ではない。
- Viewer 変更は DOM 存在だけで完了扱いしない。
- 最低 1 session 相当の確認、または E2E / Playwright 相当の確認を行う。
- 既存の `/viewer/send`、`/viewer/status`、`/viewer/logs`、`/viewer/evidence/*`、`/viewer/verification/*`、`/viewer/memory/*`、`/viewer/source-registry` の契約を壊さない。

## 現在の Viewer 構成

現在の主な実装箇所:

| 領域 | 実装箇所 |
| --- | --- |
| Viewer HTML | `internal/adapter/viewer/viewer.html` |
| Viewer 共通 JS | `internal/adapter/viewer/assets/js/viewer.js` |
| タブ別 JS | `internal/adapter/viewer/assets/js/tabs/*.js` |
| タブ別 CSS | `internal/adapter/viewer/assets/css/tabs/ops.css` |
| Viewer handler | `internal/adapter/viewer/*_handler.go` |
| route 登録 | `cmd/picoclaw/routes.go` |

既存タブ:

- `ops`
- `overview`
- `roles`
- `progress`
- `timeline`
- `system`
- `memory`
- `news-pack`
- `idlechat`
- `sessions`
- `jobs`

Phase36 ではここに `home` / `develop` / `instructions` / `reports` を追加する。

## 実装方針

### 最重要方針

- 既存画面を削除しない。
- 既存タブの id、route、handler、API response を変更しない。
- 新 4 画面は表示集約レイヤとして作る。
- `viewer.js` への直書きを増やしすぎず、画面ごとの描画は `assets/js/tabs/*.js` に分ける。
- 初期実装では backend summary API を作らず、既存 API と localStorage で構成する。
- 破壊的操作を直接実行するボタンは置かない。必要な操作は Chat / Instructions へ流す。

### 初期実装方式

初期実装は frontend MVP とする。

追加するファイル:

```text
internal/adapter/viewer/assets/css/tabs/desk.css
internal/adapter/viewer/assets/js/tabs/home.js
internal/adapter/viewer/assets/js/tabs/develop.js
internal/adapter/viewer/assets/js/tabs/instructions.js
internal/adapter/viewer/assets/js/tabs/reports.js
```

更新するファイル:

```text
internal/adapter/viewer/viewer.html
internal/adapter/viewer/assets/js/viewer.js
```

Go handler は初期実装では追加しない。

## 追加タブ

### タブ順

初期実装では次の順にする。

```text
Home
Chat Timeline
Develop
Instructions
Reports
Memory
News Pack
IdleChat
Ops
Overview
Roles
Progress
System
Sessions
Jobs
```

`mobilePanelSelect` も同じ順に揃える。

### HTML 追加

`viewer.html` の nav に次を追加する。

```html
<button class="tab-btn active" data-tab="home">Home</button>
<button class="tab-btn" data-tab="develop">Develop</button>
<button class="tab-btn" data-tab="instructions">Instructions</button>
<button class="tab-btn" data-tab="reports">Reports</button>
```

既存 `overview` の `active` は外し、`home` を初期 active にする。

`main` に次の panel を追加する。

```html
<section id="panel-home" class="panel active">...</section>
<section id="panel-develop" class="panel">...</section>
<section id="panel-instructions" class="panel">...</section>
<section id="panel-reports" class="panel">...</section>
```

既存 `panel-overview` の `active` は外す。

## 共通デザイン仕様

4 画面は常用ビューとして、既存の監視テーブルよりも読みやすさを優先する。

方針:

- 大きな文字
- 大きなカード
- 余白多め
- 情報量少なめ
- 夜の作業机の雰囲気
- 必要な情報だけを表示
- 詳細は折りたたみ
- カードを過剰に入れ子にしない

CSS は初期実装では `desk.css` にまとめる。

CSS 変数:

```css
:root {
  --desk-bg: #0f1420;
  --desk-panel: rgba(22, 29, 43, 0.86);
  --desk-panel-strong: rgba(31, 40, 58, 0.92);
  --desk-border: rgba(165, 190, 255, 0.18);
  --desk-text: #eef3ff;
  --desk-muted: #9aa8c7;
  --desk-accent: #8fb7ff;
  --desk-warn: #ffd28f;
  --desk-danger: #ff9a9a;
}
```

代表クラス:

```text
daily-desk-title
daily-desk-card
daily-desk-body
daily-desk-muted
daily-desk-input
desk-grid
desk-hero
desk-card-list
desk-action-row
```

## 既存 API 利用方針

初期実装で使う API:

| 用途 | API |
| --- | --- |
| 全体状態 | `/viewer/status` |
| event log / 最後の会話 | `/viewer/logs?scope=persisted&limit=40` |
| Jobs / Evidence | `/viewer/evidence/recent?limit=20` |
| Evidence summary | `/viewer/evidence/summary` |
| Evidence detail | `/viewer/evidence/detail?job_id=...` |
| Verification recent | `/viewer/verification/recent?limit=20` |
| Verification summary | `/viewer/verification/summary` |
| Verification detail | `/viewer/verification/detail?job_id=...` |
| Memory snapshot | `/viewer/memory/snapshot` |
| Memory layers | `/viewer/memory/layers` |
| Source Registry | `/viewer/source-registry` |
| Recall traces | `/viewer/recall/traces` |
| 送信 | `/viewer/send` |

新 4 画面は、既存 `viewer.js` の state を直接破壊しない。必要な取得処理はタブ別 JS に置き、共通 state へ読み取り専用に近い形で投影する。

## Home / Daily Desk

### 目的

起動直後に最初に見る常用ホームである。今日 RenCrow で何をするかをすぐ始められる状態にする。

### 表示要素

- 今日の状態
- 最後の会話
- 進行中の作業
- 未読レポート相当
- 未完了指示
- 呼び出せるキャラクター
- 大きな入力欄

### 状態判定

| 条件 | 状態 |
| --- | --- |
| error event または failed report がある | 要確認 |
| running job がある | 作業中 |
| 新しい report がある | 報告あり |
| それ以外 | 通常 |

### データ元

| 表示 | データ元 |
| --- | --- |
| 今日の状態 | `/viewer/status`, `/viewer/logs`, `/viewer/evidence/summary`, `/viewer/verification/summary` |
| 最後の会話 | `/viewer/logs?scope=persisted&limit=40` |
| 進行中の作業 | `state.jobs`, `/viewer/status` |
| 未読レポート相当 | `/viewer/evidence/recent`, `/viewer/verification/recent` |
| 未完了指示 | localStorage `rencrow.viewer.instructions.v1` |
| 呼び出せるキャラクター | `state.agents`, Roles 相当 |

### 入力欄

Home の入力欄は `/viewer/send` を使う。

仕様:

- placeholder: `今日なにをする？ 会話・開発・調査・指示をここから始める`
- default: Chat
- 宛先候補: Chat / Worker / Coder / Heavy / Wild
- 添付は既存 input bar の契約を壊さず、初期実装では Home 専用添付 UI は追加しない。
- 送信後は Chat Timeline へ遷移するか、Home の最後の会話カードを更新する。

### 操作

| 操作 | 動作 |
| --- | --- |
| 会話を再開 | `timeline` タブへ遷移 |
| Developで見る | `develop` タブへ遷移 |
| Reportsで読む | `reports` タブへ遷移 |
| Instructionsを見る | `instructions` タブへ遷移 |
| Agent を選ぶ | Home 入力欄の宛先を変更 |

## Develop

### 目的

AI に任せている開発作業の現在地を読む画面である。コードエディタではなく、進行中タスク、担当 Agent、フェーズ、確認待ち、次の操作を主役にする。

### 表示要素

- 現在の開発タスク
- 指示内容
- 担当: Worker / Coder / Heavy / Wild
- 進捗フェーズ
- 作業ログ要約
- 差分・成果物
- 確認待ち
- エラーと再試行
- 次に押すボタン

### フェーズ推定

既存 event / job から次へ分類する。

| 推定フェーズ | 判定例 |
| --- | --- |
| Planning | plan / proposal / route decision |
| Implementing | patch / apply / file changed |
| Testing | go test / e2e / verify command |
| Verifying | verification / evidence / audit |
| Reporting | report / summary / completed response |
| Waiting User | blocked / needs confirmation |
| Failed | failed / error |
| Done | passed / complete / done |

### データ元

| 表示 | データ元 |
| --- | --- |
| Current Task | `state.jobs`, `/viewer/logs` |
| Agent / Phase | `state.agents`, event log |
| 作業ログ要約 | `/viewer/logs`, `/viewer/evidence/detail` |
| 差分・成果物 | evidence steps / verification / artifacts 相当 |
| 確認待ち | localStorage instructions の `blocked`、error event |

### 操作

初期実装で表示するボタン:

- 続ける
- 止める
- 再試行
- 前提を見直す
- Reportsで読む
- Chatで相談する

ただし、これらは破壊的操作を直接実行しない。初期実装では Chat / Instructions へ文脈付きで遷移する。

## Instructions

### 目的

指示キュー画面である。Chat / Home / Develop から生まれた作業指示を追跡する。

### MVP 保存方式

初期実装は localStorage とする。

key:

```text
rencrow.viewer.instructions.v1
```

MVP の Instruction model:

```json
{
  "instruction_id": "inst_20260517_000001",
  "source": "home|chat|develop|manual",
  "text": "ViewerにHome画面を追加して",
  "status": "open|running|blocked|done|cancelled",
  "priority": "low|normal|high|urgent",
  "target_agent": "Chat|Worker|Coder|Heavy|Wild",
  "created_at": "2026-05-17T09:00:00+09:00",
  "updated_at": "2026-05-17T09:20:00+09:00",
  "due_hint": null,
  "timing_hint": "today|next|after_current_job",
  "job_ids": ["job_..."],
  "route": "Chat->Worker",
  "last_summary": "現在Workerが仕様反映中",
  "blocked_reason": null,
  "cancel_reason": null
}
```

### ステータス

| status | 意味 |
| --- | --- |
| open | まだ実行されていない |
| running | Job 化されて進行中 |
| blocked | 確認待ち・前提不足 |
| done | 完了 |
| cancelled | 取り消し |

### 操作

- 新規指示作成
- 優先度変更
- 対象 Agent 変更
- Job 化候補として Chat へ送る
- キャンセル
- Chat で相談
- Reports へ遷移

### backend 昇格時の API 案

Phase36 では実装しないが、次段階で次を検討する。

```text
GET    /viewer/instructions
POST   /viewer/instructions
PATCH  /viewer/instructions/{instruction_id}
POST   /viewer/instructions/{instruction_id}/retry
POST   /viewer/instructions/{instruction_id}/cancel
POST   /viewer/instructions/{instruction_id}/promote-to-job
```

## Reports

### 目的

人間が読むための作業完了報告画面である。既存 Jobs / Evidence / Verification は材料であり、Reports では要約、変更点、確認内容、失敗、未確認、次の判断として整形する。

### ReportView

MVP では `/viewer/evidence/*` と `/viewer/verification/*` から frontend 側で組み立てる。

```json
{
  "report_id": "rep_job_...",
  "job_id": "job_...",
  "title": "Viewer Home画面追加仕様",
  "status": "success|failed|partial|unknown",
  "summary": "作業内容の要約",
  "changed": [],
  "verified": [],
  "failed": [],
  "unconfirmed": [],
  "evidence_refs": [],
  "artifacts": [],
  "next_decision": [],
  "created_at": "2026-05-17T09:30:00+09:00",
  "read_at": null
}
```

### 表示

- Report List
- Report Detail
- Summary
- 何を変更したか
- 何を確認したか
- 検証結果
- 失敗したこと
- 未確認のこと
- Evidence
- 成果物
- 次の判断

### 操作

- Copy Summary
- Copy Markdown
- Mark as Read
- Open Job
- Open Evidence
- Open Artifact
- Create Follow-up Instruction

`Mark as Read` は MVP では localStorage に保存する。

key:

```text
rencrow.viewer.reportReads.v1
```

### Markdown Export

```markdown
# 作業完了報告: {title}

## 要約
{summary}

## 何を変更したか
- {changed}

## 何を確認したか
- {verified}

## 検証結果
{verification_result}

## 失敗したこと
- {failed}

## 未確認のこと
- {unconfirmed}

## Evidence
- {evidence_refs}

## 成果物
- {artifacts}

## 次の判断
- {next_decision}
```

## 画面間連携

| 起点 | 遷移 |
| --- | --- |
| Home 最後の会話 | Chat Timeline |
| Home 進行中の作業 | Develop |
| Home 未読レポート | Reports |
| Home 未完了指示 | Instructions |
| Home Agent | Chat Timeline または Home 入力欄の宛先変更 |
| Chat 作業依頼 | Instructions 候補作成 |
| Worker / Coder 宛送信 | Develop に現在タスク表示 |
| Job 完了 | Reports に表示 |
| Develop 進行中 Job | Jobs 詳細 |
| Develop 検証結果 | Reports |
| Develop 追加指示 | Instructions |
| Instructions Job 化 | Develop |
| Instructions 完了 Report | Reports |
| Reports 次の判断 | Instructions |
| Reports 関連 Job | Develop / Jobs |
| Reports 関連会話 | Chat Timeline |

## 実装手順

### Phase36-1 frontend MVP

1. baseline test を実行する。
2. `viewer.html` に `home` / `develop` / `instructions` / `reports` のタブと panel を追加する。
3. `home` を初期 active にし、`overview` の active を外す。
4. `mobilePanelSelect` に 4 画面を追加し、順序を nav と揃える。
5. `desk.css` を追加して読み込む。
6. `home.js` / `develop.js` / `instructions.js` / `reports.js` を追加して読み込む。
7. 新 JS では既存 API を fetch し、人間向けカードへ変換する。
8. Instructions と report read state は localStorage に保存する。
9. 既存 `viewer.js` は必要最小限の共通関数呼び出しだけにする。
10. JS 構文チェック、Go test、E2E を実行する。

### Phase36-2 backend summary API

Phase36-1 が実機で使えることを確認してから、次を追加する。

```text
/viewer/home/summary
/viewer/develop/summary
/viewer/instructions
/viewer/reports
```

Go 側候補:

```text
internal/adapter/viewer/instruction_handler.go
internal/adapter/viewer/instruction_store.go
internal/adapter/viewer/instruction_types.go
internal/adapter/viewer/report_handler.go
internal/adapter/viewer/report_builder.go
internal/adapter/viewer/report_types.go
```

### Phase36-3 既存タブ整理

Phase36 では実施しない。将来、次の吸収を検討する。

- Roles -> Home / Develop
- Progress -> Develop / Reports
- Jobs -> Reports / Developer
- News Pack -> Memory
- Ops / System / Sessions -> Developer / Debug

## テスト方針

### unit / package test

既存 handler 契約を壊さないことを確認する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/viewer
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

### 全体確認

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./...
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
node --check internal/adapter/viewer/assets/js/viewer.js
node --check internal/adapter/viewer/assets/js/tabs/home.js
node --check internal/adapter/viewer/assets/js/tabs/develop.js
node --check internal/adapter/viewer/assets/js/tabs/instructions.js
node --check internal/adapter/viewer/assets/js/tabs/reports.js
git diff --check
```

### live / browser 確認

Viewer 変更なので、実装後はブラウザまたは同等の E2E で確認する。

確認項目:

- `/viewer` が開く。
- 起動直後に Home が active で表示される。
- タブ遷移で既存タブが壊れていない。
- Home の入力欄から `/viewer/send` 相当で送信できる。
- Develop に進行中 Job または空状態が正しく出る。
- Instructions で localStorage の作成、状態変更、キャンセルができる。
- Reports で Evidence / Verification 由来の report view が読める。
- Copy Summary / Copy Markdown が動く。
- Viewer 表示、音声、口パク、ログが混ざっていない。

## 受け入れ条件

### Home

- 起動直後に Home が表示される。
- 今日の状態が一目で分かる。
- 最後の会話が見える。
- 進行中 Job が見える。
- 未読 Report 相当が見える。
- 未完了 Instruction が見える。
- 大きな入力欄から送信できる。

### Develop

- 進行中 Job が分かる。
- 担当 Agent が分かる。
- 現在フェーズが分かる。
- 直近ログが要約されている。
- エラーと再試行回数が分かる。
- 次に押すべきボタンが分かる。
- 破壊的操作を直接実行するボタンがない。

### Instructions

- 指示を一覧できる。
- `open` / `running` / `blocked` / `done` / `cancelled` が区別できる。
- 優先度を表示できる。
- 対象 Agent を表示できる。
- 関連 Job へ遷移できる。
- 再指示・キャンセルができる。

### Reports

- 完了報告を人間向けに読める。
- 変更点が分かる。
- 確認内容が分かる。
- 検証結果が分かる。
- 失敗・未確認が隠れない。
- Evidence を参照できる。
- Copy Summary ができる。
- Markdown Export ができる。

## リスク

| リスク | 対策 |
| --- | --- |
| `viewer.js` がさらに巨大化する | 新 4 画面の描画は `tabs/*.js` に置く |
| 既存タブが壊れる | 既存 panel id / tab id / handler を変更しない |
| Reports がログ一覧に戻る | ReportView として summary / changed / verified / failed / unconfirmed に変換する |
| Instructions が正式 queue と誤解される | MVP は localStorage と明記し、backend 昇格を別 Phase にする |
| Home 入力欄と既存 input bar が二重管理になる | `/viewer/send` 契約に寄せ、添付は初期実装では既存 input bar を優先する |
| 音声・口パク・ログが混ざる | 新 4 画面は TTS chunk を本文根拠にしない |
| 未読状態が永続化されない | MVP は localStorage、backend read state は次段階 |

## 実装対象外

Phase36-1 では次を対象外にする。

- 既存タブの削除
- `Ops / Overview / System / Sessions / Jobs` の統合
- backend summary API の新設
- Instruction の backend 永続化
- Report の backend 永続化
- 破壊的操作を直接実行する UI
- Viewer 表示契約、SSE event、TTS chunk、lipsync trigger の変更
- `/viewer/send` の API 契約変更

## 完了条件

- 実装仕様が `docs/refactor/Phase36_Viewer常用4画面追加実装仕様.md` に作成されている。
- 追加 4 画面の目的、表示要素、データ元、操作、受け入れ条件が明記されている。
- 既存 Viewer を壊さない制約が明記されている。
- 初期実装で追加・更新するファイルが明記されている。
- backend summary API は Phase36-1 の対象外として分離されている。
- テスト方針と live / browser 確認項目が明記されている。
- TODO / TBD の仮置きが残っていない。
