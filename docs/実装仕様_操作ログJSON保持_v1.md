# 実装仕様: 操作ログ JSON 保持 v1

**作成日**: 2026-03-19
**ステータス**: 実装進行中
**対象**: RenCrow Viewer / Chat / Worker / Coder 操作ログ

## 1. 概要

RenCrow の操作ログは、ライブ監視用の in-memory event と、後追い監査用の persisted JSONL に分離して扱う。

本仕様の目的は以下。

- Chat / Worker / Coder の挙動を JSON で後追いできるようにする
- ログを無期限保持せず、TTL で定期削除する
- 削除処理そのものも JSON で監査できるようにする
- 不具合調査の知見を docs と Skill の両方に残す

## 2. 永続ログ

### 2.1 正本ファイル

- 操作ログ: `workspace/orchestrator_event_log.jsonl`
- GC 監査ログ: `workspace/orchestrator_event_gc.jsonl`

### 2.2 1 行 1 event

操作ログは `orchestrator.OrchestratorEvent` を JSONL で append-only 保存する。

最低限の追跡項目:

- `timestamp`
- `type`
- `from`
- `to`
- `route`
- `job_id`
- `session_id`
- `channel`
- `chat_id`
- `content`

### 2.3 監視イベント

後追い対象イベントは少なくとも以下。

- `message.received`
- `routing.decision`
- `agent.start`
- `agent.dispatch`
- `agent.note`
- `agent.thinking`
- `agent.response`
- `agent.error`
- `mailbox.sent`
- `mailbox.waiting`
- `mailbox.received`
- `mailbox.error`
- `worker.retry_request`
- `worker.classified_failure`
- `entry.stage`

## 3. 保持と削除

### 3.1 保持ポリシー

- 基準: `timestamp` ベース TTL
- デフォルト保持期間: `14日`
- デフォルト削除間隔: `60分`

### 3.2 削除方式

GC は以下の手順で行う。

1. 元ファイルを読む
2. TTL 内の行だけ temp ファイルへ書く
3. rename で置換する

これにより、ログは append-only 運用のまま compaction できる。

### 3.3 GC 監査ログ

GC ごとに以下を `orchestrator_event_gc.jsonl` に残す。

- `started_at`
- `finished_at`
- `source_path`
- `retention_days`
- `before_count`
- `after_count`
- `deleted_count`
- `decode_error_count`
- `timestamp_error_count`
- `status`
- `error`

`status` は `ok`, `partial_error`, `error` を使用する。

## 4. 外部確認 API

操作ログ確認の正本 API は以下。

- `GET /viewer/logs`
- `GET /viewer/agents`
- `GET /viewer/agent/detail?id=...`
- `GET /viewer/jobs`
- `GET /viewer/job/detail?job_id=...`
- `GET /viewer/audit/summary`

`/viewer/logs` は以下の filter を受ける。

- `scope=live|persisted`
- `type`
- `agent`
- `route`
- `job_id`
- `session_id`
- `chat_id`
- `limit`

## 5. 設定

設定キー:

```yaml
viewer_log:
  enabled: true
  path: "./workspace/orchestrator_event_log.jsonl"
  retention_days: 14
  gc_interval_minutes: 60
```

## 6. 知見の保存先

仕様・I/F の正本は docs に置く。運用時に即使う調査手順と不具合対策は Skill に置く。

- docs: 本書
- skill: `workspace/skills/log-ops/SKILL.md`

Skill には以下を残す。

- まず見る API
- persisted log の見方
- よくある停止点
- 失敗時の切り分け順
- GC 異常時の確認手順
