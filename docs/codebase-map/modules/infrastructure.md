---
generated_at: "2026-05-15T08:05:48Z"
run_id: run_20260515_080548
phase: 2
step: "10"
profile: rencrow-core-map
artifact: module
module_group_id: infrastructure
---

# Infrastructure 層

## 概要

`internal/infrastructure/` は LLM provider、tool runner、security policy、persistence、transport、STT/TTS、VTuber、MCP など外部技術の実装を担う。  
Domain/Application の契約に対して、HTTP、DB、filesystem、shell、音声サーバ、外部LLM接続などの実体を提供する層である。

## 関連ドキュメント

- `internal/infrastructure/llm/`
- `internal/infrastructure/tools/runner.go`
- `internal/infrastructure/security/policy_engine.go`
- `internal/infrastructure/persistence/conversation/l1_sqlite_store.go`
- `internal/infrastructure/persistence/conversation/real_manager.go`
- `internal/infrastructure/tts/`
- `internal/infrastructure/stt/`
- `docs/LLM運用/`
- `docs/STT_TTS/`

## 役割

- `llm`: OpenAI/Gemini/Ollama/Claude/DeepSeek provider と middleware、factory。
- `tools`: shell/file/web_search/subagent/register_tool などの tool runner と許可リスト。
- `security`: policy engine、sandbox guard、policy runner。
- `persistence`: conversation memory、L1 SQLite、DuckDB、Redis、VectorDB、execution report、session、toolregistry。
- `tts`, `stt`, `vtuber`, `audiorouter`: 音声入出力とキャラクター連携。
- `transport`: local/ssh/router/logger。
- `routing`, `classifier`, `capability`, `health`, `mcp`, `persona`: 周辺実装。

## 構造マップ

```text
internal/infrastructure
  ├─ llm
  │   ├─ factory.CreateProvider
  │   ├─ providers/openai|ollama|claude|gemini|deepseek
  │   └─ middleware/rawlog|datetime|limited
  ├─ tools
  │   ├─ ToolRunner.Execute / ExecuteV2
  │   ├─ shell/file/web_search/subagent
  │   └─ allowed commands / write paths
  ├─ security
  │   ├─ PolicyEngine.Evaluate
  │   └─ SandboxGuard / PolicyRunner
  ├─ persistence
  │   ├─ conversation RealConversationManager
  │   ├─ L1SQLiteStore staging/source registry/memory/news/knowledge
  │   └─ execution/session/toolregistry stores
  └─ stt / tts / vtuber / transport / health / mcp
```

## 外部依存・被依存

- Application は LLM、tool、persistence、security、transport の具象実装としてこの層を使う。
- Adapter は config と handler 経由でこの層の status や store を公開する。
- `L1SQLiteStore` は Source Registry、staging、memory candidate、news、knowledge など複数の状態遷移を一つの store に持つ。

## 落とし穴・注意点

- LLM provider の endpoint/model 設定は runtime config と live サービスの実態を必ず確認する。docs だけで判断しない。
- `ToolRunner` と `WorkerExecutionService` の両方に file/shell 系の安全境界があるため、片方だけ見て許可判断を完了しない。
- `L1SQLiteStore` は observed/candidate/confirmed や staging validation/promote を扱う。直接 confirmed/pinned に昇格させる経路を追加しない。
- Source Registry の保存・sweep・promote は trust score、validation status、license note、raw hash の整合性が重要。
- ※Phase 2 で追加: TTS/STT/AudioRouter は Viewer ブラウザ側の実挙動と一体で確認しないと、Go テストだけでは再生・録音の成立を保証できない。

## 読むべき場面

- LLM 接続、raw log、thinking leak、timeout、health の調査。
- tool/shell/file write の許可・拒否理由を追うとき。
- memory candidate、Source Registry、staging/validator を変更するとき。
- STT/TTS/VTuber/AudioRouter の provider 実装を確認するとき。
