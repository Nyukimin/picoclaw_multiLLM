# 69 セッションログ + DCI統合仕様

## 概要

RenCrow に「会話セッション単位のターンログ」を追加し、それを DCI（直接コーパス探索）で
検索可能にする。また同じ仕組みで Codex・Claude Code のセッションログも参照できる。

---

## 1. RenCrow セッションログ

### 1.1 書き込み先

```
~/.picoclaw/logs/sessions/{YYYY-MM}/session_{YYYY-MM-DD}_{session_id}.jsonl
```

### 1.2 エントリ形式

```json
{"ts":"2026-06-06T12:00:00Z","session_id":"viewer","channel":"line","role":"user","content":"質問内容"}
{"ts":"2026-06-06T12:00:02Z","session_id":"viewer","channel":"line","role":"assistant","route":"CHAT","job_id":"job_...","content":"回答内容"}
```

### 1.3 実装

- `internal/infrastructure/logging/session_log_writer.go`
  - `SessionLogWriter.WriteUser()` / `WriteAssistant()`
  - セッションIDは安全な文字列に正規化（64文字上限）
- `MessageOrchestrator.SetSessionTurnLogger(l SessionTurnLogger)` で注入
- `cmd/picoclaw/runtime_orchestrator.go` で DI 配線済み
- ProcessMessage の直後に WriteUser、完了後に WriteAssistant を呼ぶ

---

## 2. DCI セッションログ統合

### 2.1 対応ソース

| ソース名 | パス | フォーマット |
|----------|------|-------------|
| rencrow  | `~/.picoclaw/logs/sessions/` | `rencrow` |
| codex    | `~/.codex/sessions/` | `codex` |
| claude   | `~/.claude/projects/-home-nyukimi-picoclaw-multiLLM/` | `claude` |

### 2.2 実装

- `internal/infrastructure/persistence/dci/session_log_candidate_provider.go`
  - `SessionLogCandidateProvider.CandidateFiles()` — 直近90日のJSONLをスキャン
  - `scoreSessionFile()` — クエリ・タームの出現頻度でスコアリング（先頭200行）
  - フォーマット別パーサ: RenCrow `content`、Codex `payload.content`、Claude `message.content[]`
- `cmd/picoclaw/runtime_dependencies.go`
  - `buildSessionLogSources()` — 設定があればそれを使用、なければデフォルト3ソース
  - セッションログソースのパスは CorpusAllowlist に自動追加（DCI がファイルを grep できる）
  - `dciapp.WithSourceCandidateProvider()` で DCI Explorer に登録

### 2.3 設定（オプション）

`picoclaw.yml` で明示設定もできる:

```yaml
dci:
  session_log_sources:
    - name: rencrow
      path_dir: "${HOME}/.picoclaw/logs/sessions"
      format: rencrow
    - name: codex
      path_dir: "${HOME}/.codex/sessions"
      format: codex
    - name: claude
      path_dir: "${HOME}/.claude/projects/-home-nyukimi-picoclaw-multiLLM"
      format: claude
```

設定が空の場合は上記のデフォルトが自動適用される。

---

## 3. 効果

- 「前回どう直した？」「この問題は解決済み？」という質問に DCI が自動でセッション履歴を参照して回答できる
- Codex・Claude Code のセッション履歴も同一の検索経路で参照可能
- CoderLoop の観察ステップから `git grep` と同様にセッション履歴を参照可能

---

## 4. 関連ファイル

- `internal/infrastructure/logging/session_log_writer.go`
- `internal/infrastructure/persistence/dci/session_log_candidate_provider.go`
- `internal/adapter/config/config_types.go` — `DCIConfig.SessionLogSources`
- `internal/application/orchestrator/message_orchestrator.go` — `SessionTurnLogger` インターフェース
- `cmd/picoclaw/runtime_orchestrator.go` — DI 配線
- `cmd/picoclaw/runtime_dependencies.go` — `buildSessionLogSources()`
