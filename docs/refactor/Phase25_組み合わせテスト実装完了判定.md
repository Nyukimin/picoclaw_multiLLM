# Phase25 組み合わせテスト実装完了判定

## 目的

この文書は、`docs/refactor/Phase25_組み合わせテスト設計.md` に基づいて追加したテスト実装と検証結果を記録する完了判定である。

## Objective の具体化

Phase25 の実装完了条件は次の通り。

- 設計書の高優先不足テストを、外部依存なしで固定できるものから TDD 化する。
- 外部依存が必要なものは、skip を成功扱いせず、環境変数で明示的に有効化する E2E として実装する。
- Viewer / Browser は DOM 存在だけでなく、live service に対する送信と Viewer 表示・ログ境界を確認する。
- repo example と live runtime config を混同しない検証を持つ。
- 通常テスト、タグ付き E2E、live E2E、browser E2E を実行して成功させる。
- 未追跡の `tests/` は触らない。

## Prompt-to-artifact checklist

| Phase25 要件 | 追加・確認した artifact | 判定 |
| --- | --- | --- |
| CODE1 / CODE2 の tagged E2E | `test/e2e/phase25_routing_contract_test.go` | 完了 |
| explicit CODE route の差分 | `/code`, `/code1`, `/code2`, `/code3` の routing contract を確認 | 完了 |
| repo example と live config を混同しない | `internal/adapter/config/config_example_contract_test.go`, `test/e2e/phase25_live_runtime_contract_test.go` | 完了 |
| Viewer 表示、音声、口パク、ログを混同しない | `internal/adapter/viewer/viewer_static_contract_test.go` | 完了 |
| Browser / Viewer の 1 session | `test/e2e/phase25_browser_viewer_test.go` | 完了 |
| live `/health` と Viewer runtime config | `PICOCLAW_LIVE_E2E=1 go test -tags=e2e ./test/e2e -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v` | 完了 |
| external dependency skip を成功扱いしない | API key / Ollama / browser / live は skip 条件を test log に明示 | 完了 |
| 全体 Go test | `GOCACHE=/tmp/picoclaw-gocache go test ./...` | 完了 |
| tagged E2E | `GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e` | 完了 |
| browser E2E | `PICOCLAW_BROWSER_E2E=1 PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 go test -tags=e2e ./test/e2e -run TestE2E_Phase25BrowserViewerSessionContract -v` | 完了 |

## 追加したテスト

### `test/e2e/phase25_routing_contract_test.go`

明示 route の E2E 契約を固定する。

- `/code` -> `CODE`
- `/code1` -> `CODE1`
- `/code2` -> `CODE2`
- `/code3` -> `CODE3`

外部 LLM に到達せず、`MioAgent.DecideAction` の explicit command 経路を通す。`confidence=1.0` と `reason="Explicit command"` も契約として確認する。

### `test/e2e/phase25_live_runtime_contract_test.go`

`PICOCLAW_LIVE_E2E=1` のときだけ live service を確認する。

- `GET /health`
- `GET /viewer/runtime-config`
- `local_llm` が runtime config として返ること
- `stt_stream_url` が Viewer STT 契約として返ること

repo example を live runtime の代替として扱わないため、明示 env がない場合は skip する。

### `test/e2e/phase25_browser_viewer_test.go`

`PICOCLAW_BROWSER_E2E=1` のときだけ Playwright を使って live Viewer を確認する。

確認するもの:

- `#inp`, `#sendBtn`
- 通常 Chat 表示領域
- Ops event log
- IdleChat live log
- TTS now playing
- Mio / Shiro lipsync DOM
- 通常 chat mic control
- IdleChat start control
- live `/viewer/send` への送信
- 送信メッセージが Viewer 表示または Ops log に出ること

DOM 存在だけで終わらせず、live service に対する送信と UI 反映まで確認する。

### `internal/adapter/config/config_example_contract_test.go`

repo example config の契約を固定する。

- `config/config.yaml.example` が `LoadConfig` で読めること
- `local_llm.enabled` が有効であること
- `stt.stream_url` があること
- `vtuber.characters.shiro` と `audio_router.device_map.shiro` が別階層に存在すること

### `internal/adapter/viewer/viewer_static_contract_test.go`

Viewer の表示、音声、口パク、ログ、Memory / Source Registry の DOM 境界を固定する。

確認対象:

- 通常 Chat timeline
- IdleChat live / summary
- TTS playback status
- Mio / Shiro lipsync
- Ops event log
- STT trace
- Source Registry panel
- Memory layer panel
- mic control
- IdleChat control
- browser audio controls

## 実行した検証

### 通常テスト

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./...
```

結果:

- 成功

### tagged E2E

```bash
GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e
```

結果:

- 成功

### live runtime E2E

```bash
PICOCLAW_LIVE_E2E=1 GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v
```

結果:

- 成功

確認した live endpoint:

- `http://127.0.0.1:18790/health`
- `http://127.0.0.1:18790/viewer/runtime-config`

### browser Viewer E2E

```bash
PICOCLAW_BROWSER_E2E=1 PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 GOCACHE=/tmp/picoclaw-gocache go test -tags=e2e ./test/e2e -run TestE2E_Phase25BrowserViewerSessionContract -v
```

結果:

- 成功

## 外部依存の扱い

次の検証は、環境未準備時に skip される。skip は成功扱いではなく、外部依存がないことの記録である。

- Claude / DeepSeek / OpenAI / Gemini API key
- Google Search API key / CSE
- Ollama endpoint
- browser E2E の Playwright / live service
- live runtime E2E の service endpoint

今回の環境では、live service と browser Viewer E2E は実行して成功している。

## 残る外部依存リスク

以下は実装済みのテスト入口を持つが、外部環境が準備された状態での継続確認が必要である。

- API key 準備済みの external provider Generate。
- Google Search Chat / Worker の実 API。
- Ollama 実体が reachable な環境での Chat route。
- STT server と browser mic permission を使う実録音。
- TTS server と audio device / VTube Studio を使う口パク実機観測。
- distributed ssh / mailbox / direct の実 agent。

これらは fallback 成功とは扱わず、環境準備済みの dedicated run で pass 記録を残す。

## 完了判定

Phase25 の組み合わせテスト設計に対して、現在のリポジトリと live service で実装可能な高優先検証は追加済みである。

通常テスト、tagged E2E、live runtime E2E、browser Viewer E2E が成功しているため、Phase25 は現在の実行環境において完了と判定する。
