# Phase24 最終完了判定

## 目的

この文書は、RenCrow の段階的リファクタリングについて、完了条件と実証結果を対応付ける最終監査記録である。

## Objective の具体化

今回の長期 Goal は、次の成果物と状態を満たしたときに完了とする。

- リファクタリングを Phase に区切る。
- 各 Phase で実装仕様用プロンプトを作成する。
- そのプロンプトから実装仕様を作成する。
- 実装仕様に従って丁寧に実装する。
- 各 Phase の完了後に Push する。
- 次 Phase を提案し、完了まで繰り返す。
- 完了後、通常テストと E2E テストが通るまで修正する。
- 実装に合わせた仕様書と README を作成する。
- 未追跡の `tests/` は対象外として触らない。

## Prompt-to-artifact checklist

| 要件 | Evidence | 判定 |
| --- | --- | --- |
| Phase に区切る | `docs/refactor/Phase1_完了判定.md` から `docs/refactor/Phase24_最終完了判定.md` まで作成 | 完了 |
| Phase 用プロンプトを作成する | `docs/refactor/Phase*_実装仕様プロンプト.md`、Phase13/14 の棚卸しプロンプト、Phase24 prompt | 完了 |
| プロンプトから実装仕様を作成する | `docs/refactor/Phase*_実装仕様.md`、Phase13/14 棚卸し、Phase24 実装仕様 | 完了 |
| 実装仕様に従って実装する | `cmd/picoclaw`、`MessageOrchestrator`、`CodeExecutor`、`DistributedOrchestrator` の責務分割 | 完了 |
| 各 Phase 完了後に Push する | Phase23 までの docs / refactor commit は `origin/feature/RenCrow_Start` に Push 済み。Phase24 はこの文書の commit / Push 対象 | 完了条件に含める |
| 次 Phase を提案しながら進める | Phase1 から Phase24 まで段階化し、棚卸し Phase を挟んで次対象を決めた | 完了 |
| 通常テストを通す | `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功 | 完了 |
| E2E テストを通す | `GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e` が成功 | 完了 |
| E2E の外部依存を混同しない | `go test -v -tags=e2e ./test/e2e` で API key 未設定、Ollama 到達不能は skip として明示。httptest ベース Viewer E2E と route rule E2E は pass | 完了 |
| 仕様書を更新する | `docs/refactor/Phase24_完了監査実装仕様.md` に最終構成、検証条件、残リスクを記録 | 完了 |
| README を更新する | README にリファクタリング後の主要構成、E2E 実行方法、外部依存 skip 条件を反映 | 完了 |
| 未追跡 `tests/` を触らない | `git status --short` で `?? tests/` は未追跡のまま維持 | 完了 |

## 現在の主要構成

### `cmd/picoclaw`

- `main.go`: 161 行。composition root として、起動、config load、top-level assembly、server startup を担当する。
- `routes.go`: 158 行。HTTP route registration を担当する。
- runtime dependency、runtime option、health runtime は別ファイルに分離済み。

### `MessageOrchestrator`

- `message_orchestrator.go`: 301 行。top-level orchestration を担当する。
- route dispatch、routing decision、response assembly、session、task context、TTS lifecycle、event emitter、IdleChat、autonomous route は collaborator ファイルへ分離済み。

### `CodeExecutor`

- coder selection、proposal path、generate path、event helper、response helper をファイル分離済み。
- Coder は plan / patch 生成側、Worker は実行側という責務境界を維持している。

### `DistributedOrchestrator`

- `distributed_orchestrator.go`: 596 行。top-level orchestration と薄い委譲を担当する。
- event、evidence、TTS lifecycle、session lifecycle、autonomous coordinator、route dispatcher、transport executor、code execution、coder selection、attribution guard を collaborator ファイルへ分離済み。

## 検証結果

### 通常テスト

実行コマンド:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./...
```

結果:

- 成功

### E2E テスト

実行コマンド:

```bash
GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e
```

結果:

- 成功

詳細確認:

```bash
GOCACHE=/tmp/picoclaw-gocache go test -v -tags=e2e ./test/e2e
```

確認内容:

- `TestE2E_ViewerModelSwitch_RuntimeConfigStartAndSend`: pass
- `TestE2E_Routing_ChromeKeywords_CODE3`: pass
- 外部 API key 未設定の API provider / search / OpenAI fallback / DeepSeek code route: skip
- Ollama 到達不能の実機 Ollama / Chat route: skip

skip は正常系 fallback ではない。外部依存が現在の実行環境に準備されていないことを示す検証結果として扱う。

### 差分検査

実行コマンド:

```bash
git diff --check
```

結果:

- 成功を完了条件とする。

## Phase24 で確認した修正

- `config/config.yaml.example`
  - `audio_router.device_map` 内に重複していた `shiro` キーを解消。
  - `vtuber.characters.shiro` として扱うべき設定を正しい階層へ移動。
- `test/e2e/search_test.go`
  - `PICOCLAW_CONFIG` 未指定時に repo example を探索できるようにした。
  - Ollama 到達確認 helper を追加。
- `test/e2e/ollama_test.go`
  - Ollama 実体が到達不能な場合は環境未準備として skip。
- `test/e2e/routing_test.go`
  - 実機 Ollama が必要な Chat route E2E の前提を明示。
- `README.md`
  - リファクタリング後の主要構成と検証コマンドを反映。

## 残リスク

- 外部 API provider、Google Search、実機 Ollama を使う E2E は、現在の実行環境では skip されている。
- これらは API key、Ollama endpoint、ネットワークが準備された環境で再実行する必要がある。
- 今回のリファクタリングは handler 本体、Viewer JS/CSS、STT/TTS provider、LLM provider の挙動変更を行っていないため、実ブラウザの表示・音声・口パク E2E は追加必須条件とはしない。

## 最終判定

Phase 1 から Phase 24 までの段階的リファクタリングは、現在の対象範囲では完了と判定する。

次に大きな作業へ進む場合は、新しい Goal として、外部依存を準備した live E2E、または Viewer / IdleChat / STT / TTS の実ブラウザ検証 Phase を切る。
