# Phase1 完了判定

## 判定

Phase 1「`cmd/picoclaw` の composition root 整理」は完了とする。

`cmd/picoclaw/main.go` は、現在次の責務に限定されている。

- `main()` による CLI subcommand dispatch。
- `cmdRun()` による config load、signal handling、HTTP server startup。
- route registration 関数群の呼び出し。
- runtime factory / dependency wiring 関数の呼び出し。
- config path と assets git repo path の解決。

`main.go` から、長い route registration 本体、LLM provider factory、health/status/doctor runtime、CLI operations、runtime dependency wiring は分離済みである。

## 実施した Phase

| Phase | 文書 | 実装結果 |
|---|---|---|
| Phase 1-1 | `docs/refactor/Phase1-1_route登録分割仕様.md` | `cmdRun()` 内の HTTP route 登録を機能単位の registration 関数へ分割 |
| Phase 1-2 | `docs/refactor/Phase1-2_route登録ファイル分離手順.md` | route registration 関数群を `cmd/picoclaw/routes.go` へ分離 |
| Phase 1-3 | `docs/refactor/Phase1-3_LLM_provider_factory分離計画.md` | LLM provider factory を `cmd/picoclaw/llm_runtime_factory.go` へ分離 |
| Phase 1-4 | `docs/refactor/Phase1-4_health_runtime分離計画.md` | health / status / doctor と health service wiring を `cmd/picoclaw/health_runtime.go` へ分離 |
| Phase 1-5 | `docs/refactor/Phase1-5_CLI_operations分離計画.md` | CLI operations を `cmd/picoclaw/cli_operations.go` へ分離 |
| Phase 1-6 | `docs/refactor/Phase1-6_runtime_dependencies分離計画.md` | runtime dependency wiring を `cmd/picoclaw/runtime_dependencies.go` へ分離 |

## 現在のファイル責務

### `cmd/picoclaw/main.go`

- process entrypoint。
- CLI dispatch。
- service startup。
- top-level runtime assembly の呼び出し。
- HTTP server startup。

### `cmd/picoclaw/routes.go`

- HTTP route path と handler 接続。
- route group ごとの registration。
- handler 本体は持たない。

### `cmd/picoclaw/llm_runtime_factory.go`

- primary LLM provider の生成。
- local LLM alias の base URL / model / timeout 解決。
- conversation summary provider / embedder の生成。
- local LLM warmup。

### `cmd/picoclaw/health_runtime.go`

- health / status / doctor CLI。
- health service 構築。
- local LLM / Ollama / TTS debug health check の組み立て。

### `cmd/picoclaw/cli_operations.go`

- channels / gateway / ollama / logs / evidence / source-registry / knowledge / help の CLI 本体。
- CLI 用 parser、store loader、output helper。

### `cmd/picoclaw/runtime_dependencies.go`

- `Dependencies` lifecycle。
- Adapter / Application / Infrastructure wiring。
- distributed / local agent wiring。
- IdleChat HTTP handler。
- coder setup。
- runtime background job 起動。

## 保持した既存契約

Phase 1 では次を変更していない。

- handler 本体。
- DTO。
- SSE event。
- Viewer JS / CSS。
- IdleChat raw / view / audio 契約。
- STT / TTS provider 挙動。
- LLM provider 挙動。
- route path。
- nil handler 条件。
- runtime config の意味。
- fallback を正常系として扱わない方針。
- Viewer 表示、音声、口パク、ログを混同しない方針。
- repo example と live runtime config を混同しない方針。

## docs/codebase-map/ との関係

`docs/codebase-map/` は Phase 1 の対象選定に使った。

- `docs/codebase-map/modules/entrypoints_config_docs.md` は `main.go` に provider build、dependencies build、CLI commands、runtime service が集中していることを示していた。
- `docs/codebase-map/結合ポイントマップ.md` は `cmd/picoclaw/main.go` を主要結合点として示していた。
- `docs/codebase-map/modules/潜在バグ一覧.md` は runtime config と LLM / Viewer 診断の混同リスクを示していた。

ただし `docs/codebase-map/` は正本仕様ではない。実装判断は `docs/01_正本仕様/実装仕様.md` と現在コードを優先した。

## 検証

各実装 Phase で次を実行した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
git diff --check
```

Phase 1 最終確認でも同じ検証を実行し、成功している。

## 残リスク

- `cmd/picoclaw/runtime_dependencies.go` は既存の集中点を移動したため、まだ大きい。
- Phase 1 では挙動変更を避けるため、`Dependencies` の内部責務までは分割していない。
- `runtime_dependencies.go` のさらなる分割は、Chat / Worker / Coder route chain や Worker execution の責務整理に入る Phase 2 以降で、仕様とテストを先に固定してから扱う。
- live runtime config は Phase 1 の配置変更では意味変更していないため、live `~/.picoclaw/config.yaml` の変更確認は行っていない。

## Phase 2 に進む前の確認事項

Phase 2「Chat / Worker / Coder route chain の明確化」に進む前に、次を確認する。

- `MessageOrchestrator.ProcessMessage` の route 分岐を対象にするか。
- Chat / Worker / Coder の入力、出力、event、error contract を先に文書化するか。
- Viewer から最低 1 セッションを追う検証をどの段階で入れるか。
- fallback を正常系として扱わない検証をどの test / live observation で固定するか。
- Coder proposal 生成と Worker 実行境界をどのファイルから確認するか。

## 完了条件との照合

- `cmd/picoclaw/main.go` の責務が composition root として説明できる。
- route registration は `routes.go` へ分離済み。
- runtime wiring / dependency wiring / config/debug/display options の分離理由は Phase 1-3 から Phase 1-6 の文書に記録済み。
- 分離先は既存責務境界に沿っており、新しい巨大な service / manager / helper / util は作っていない。
- 各分離単位の入力、出力、副作用、永続化、ログ、エラー契約は各 Phase 文書に記録済み。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` は成功している。
- `git diff --check` は成功している。
- Phase 文書と実装差分は Push 済み。
