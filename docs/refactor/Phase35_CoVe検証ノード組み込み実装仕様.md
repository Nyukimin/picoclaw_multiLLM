# Phase35 CoVe 検証ノード組み込み実装仕様

## 目的

Phase35 では、Chain-of-Verification 風の回答検証システムを RenCrow に組み込むための実装仕様を定義する。

CoVe は単なるプロンプト技術として扱わない。RenCrow では、会話生成後、最終表示・TTS・channel response の前に置く Verification Pipeline として扱う。

目的は次の通りである。

- 回答前に claim を抽出する。
- memory / KB / Source Registry / search cache / raw external source / execution evidence に照らして claim を検証する。
- 誤記憶、誤引用、誤要約、未検証ソースの断定を減らす。
- 検証結果を `verified` / `weakly_supported` / `unsupported` / `conflict` / `not_checked` として扱う。
- CoVe の結果を正解保証として扱わない。
- RenCrow のモジュール化、疎結合、Chat / Worker / Coder 境界、Viewer 表示・音声・口パク・ログ分離を崩さない。

## CoVe の事実確認

参照元:

- arXiv: https://arxiv.org/abs/2309.11495
- ACL Anthology: https://aclanthology.org/2024.findings-acl.212/

Chain-of-Verification Reduces Hallucination in Large Language Models は、2023 年に arXiv 投稿され、ACL Findings 2024 に掲載された手法である。

論文上の基本手順は次の 4 段階である。

1. draft response を生成する。
2. draft を fact-check するための verification questions を計画する。
3. verification questions に独立して答える。
4. verification 結果を踏まえて final verified response を生成する。

RenCrow で採用する方針:

- factored CoVe 寄りにする。
- 検証ノードへ draft 全文を渡しすぎない。
- 検証ノードには、抽出済み claim、検証 query、参照可能な evidence source を渡す。
- LLM raw output を evidence として扱わない。
- CoVe を「ハルシネーションを減らす検証補助」として扱い、正解保証として扱わない。
- 「94%高精度」のような一次論文の代表表現として確認できない宣伝的表現は採用しない。

論文では、Wikidata 系リスト質問、closed-book MultiSpanQA、長文生成で改善が報告されている。確認できる代表値は、Wikidata 系 precision `0.17 -> 0.36`、MultiSpanQA F1 `0.39 -> 0.48`、長文生成 FACTSCORE `55.9 -> 71.4` である。

## RenCrow への組み込み位置

現行の `MessageOrchestrator` は、route を決めた後に `messageRouteDispatcher.ExecuteTask` で route-specific generation を実行し、`messageResponseAssembler.Build` で `ProcessMessageResponse` を組み立てる。

Phase35 の挿入位置は次とする。

```text
adapter input
  -> MessageOrchestrator
  -> route decision
  -> route-specific generation
  -> draft response
  -> Verification Pipeline
  -> response with VerificationReport
  -> ProcessMessageResponse
  -> Viewer / channel response
```

実装時の注意:

- 初期実装では optional hook として接続する。
- Verification Pipeline が未設定の場合は既存挙動を変えない。
- Verification Pipeline の失敗は正常成功として隠さない。
- handler、DTO、SSE event、TTS chunk、Viewer 表示契約を一度に変更しない。
- TTS 連携の最終位置は実装時に慎重に確認する。Phase35 では「最終表示・TTS 前」を目標にするが、既存の `messageRouteDispatcher` が route 内で TTS stream を finalize しているため、最初は TTS 挙動変更を対象外にして、Viewer / response の検証状態付与から始める。

## 提案モジュール構成

### `internal/domain/verification`

検証の domain contract を置く。

置くもの:

- `Claim`
- `ClaimID`
- `VerificationQuestion`
- `VerificationQuestionID`
- `EvidenceRef`
- `EvidenceSourceType`
- `VerificationStatus`
- `VerificationReport`
- `VerificationPolicy`
- `TriggerLevel`
- `VerificationErrorKind`

置かないもの:

- LLM provider 呼び出し。
- DB query。
- HTTP handler。
- Viewer 表示変換。
- Source Registry の promote 処理。

### `internal/application/verification`

Verification Pipeline の orchestration を置く。

置くもの:

- claim extraction
- verification planning
- independent verification
- evidence collection
- evidence evaluation
- contradiction check
- final revision
- dry-run pipeline
- policy evaluation

置かないもの:

- provider 固有 HTTP request。
- SQLite schema。
- Viewer DOM / CSS / JS。
- TTS / audio router 処理。
- Coder patch 実行。

### `internal/infrastructure/persistence/verification`

検証 report / evidence trace を保存する永続化実装を置く候補である。

初期実装では、既存 `internal/infrastructure/persistence/conversation` への最小追加も許容する。ただし、verification report が会話記憶、Source Registry、execution report と混ざる場合は専用 package に分ける。

置くもの:

- `VerificationReportRepository` 実装。
- report JSONL / SQLite store。
- claim / evidence trace の保存。
- job_id / session_id / route / status で引ける index。

置かないもの:

- claim 抽出ロジック。
- final revision ロジック。
- Source Registry promote 判断。
- Viewer 表示 DTO。

### `cmd/picoclaw/runtime_verification.go`

composition root として Verification Pipeline を組み立てる。

置くもの:

- domain / application / infrastructure の wiring。
- config からの enable / trigger policy 読み取り。
- `MessageOrchestrator` への optional hook 注入。

置かないもの:

- claim 抽出本体。
- evidence 評価本体。
- provider 固有処理。
- DB state 遷移本体。
- Viewer 表示判断。

### `internal/adapter/viewer`

Viewer に verification status を投影する場合のみ変更する。

置くもの:

- verification report API。
- job detail / evidence panel への verification summary 投影。
- `verified` / `weakly_supported` / `unsupported` / `conflict` / `not_checked` の表示。

置かないもの:

- 検証判断本体。
- Source Registry promote。
- LLM raw log の根拠化。
- TTS chunk からの本文生成。

## 各モジュールの契約

| モジュール | 入力 | 出力 | 副作用 | 永続化 | ログ | エラー契約 | 置いてはいけない責務 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `internal/domain/verification` | 文字列、ID、status 値 | contract / value object | なし | なし | なし | invalid status / invalid claim は value validation error | LLM 呼び出し、DB、Viewer |
| `internal/application/verification` | draft response、route、session_id、job_id、RecallPack / evidence source | revised response、VerificationReport | LLM / evidence source 呼び出し | repository 経由のみ | `verification.*` event / structured log | 検証失敗を success にしない。失敗時は `not_checked` または `conflict` と error kind を返す | provider 固有実装、DB schema、Viewer DOM |
| `internal/infrastructure/persistence/verification` | VerificationReport | 保存済み report / query result | DB / file write | JSONL / SQLite 等 | persistence error log | 保存失敗を握りつぶさない。回答本文とは分離して返す | claim extraction、final revision |
| `cmd/picoclaw/runtime_verification.go` | config、provider、stores | assembled verifier | wiring のみ | なし | startup / disabled reason | wiring 失敗時は disable 理由を明示 | 検証ロジック本体 |
| `internal/adapter/viewer` | report ID / job ID / session ID | display DTO / JSON | HTTP response / SSE optional | なし | handler error | report unavailable を通常本文成功に混ぜない | verification 判断本体 |

## Verification Pipeline

推奨フロー:

```text
User Input
  -> Recall / route-specific generation
  -> Draft Response
  -> Claim Extraction
  -> Verification Plan
  -> Independent Verification
  -> Evidence Evaluation
  -> Final Revision
  -> Response with VerificationReport
```

### 1. Claim Extraction

入力:

- draft response
- user message
- route
- session_id
- job_id

出力:

- `[]verification.Claim`

方針:

- claim は検証可能な単位に分ける。
- 感情表現、創作表現、主観表現は factual claim として扱わない。
- claim には source hint と verification priority を付ける。

### 2. Verification Plan

入力:

- claims
- route
- trigger policy

出力:

- `[]verification.VerificationQuestion`
- required evidence source

方針:

- yes/no に偏りすぎる質問だけにしない。
- 元 draft の文言をそのまま肯定させる質問を避ける。
- factored CoVe として、検証質問は claim と query 中心にする。

### 3. Independent Verification

入力:

- verification questions
- allowed evidence sources

出力:

- evidence answers
- evidence refs

方針:

- draft 全文を verifier に渡しすぎない。
- `RecallPack`、conversation memory、KB、Source Registry、search cache、raw external source を必要に応じて参照する。
- LLM raw output は evidence ではなく観測ログとして扱う。

### 4. Evidence Evaluation

入力:

- claims
- evidence answers
- evidence refs

出力:

- claim ごとの `VerificationStatus`
- contradiction / unsupported reason

方針:

- `verified` は「参照可能な根拠と矛盾しない」という意味に限定する。
- Source Registry の `promoted` と CoVe の `verified` を混同しない。
- unsupported / conflict を fallback 成功として扱わない。

### 5. Final Revision

入力:

- draft response
- claim statuses
- evidence summary

出力:

- revised response
- VerificationReport

方針:

- `unsupported` claim は断定しない。
- `conflict` claim は矛盾があることを明示する。
- `not_checked` claim は必要に応じて未確認として扱う。
- 表示本文、verification report、raw log を分ける。

## VerificationStatus

| status | 意味 | 表示方針 | ログ方針 |
| --- | --- | --- | --- |
| `verified` | 参照可能な根拠で支持され、既知の根拠と矛盾しない | 通常表示。必要に応じて根拠 summary を添える | claim_id、evidence_ref、source_type を記録 |
| `weakly_supported` | 一部根拠はあるが、根拠が弱い、古い、単一 source、または間接的 | 「可能性が高い」「関連がある」など断定を弱める | weak reason、source count、retrieved_at を記録 |
| `unsupported` | 根拠が見つからない、または根拠として不十分 | 断定しない。回答から削るか未確認と書く | unsupported reason を記録 |
| `conflict` | 根拠間、または memory / KB / source 間に矛盾がある | 矛盾があることを明示し、断定しない | conflicting evidence refs を記録 |
| `not_checked` | trigger 対象外、検証未実施、または外部依存未準備 | 通常雑談では非表示可。high-risk では未検証と表示 | skip reason、policy level、dependency state を記録 |

## Trigger Policy

全回答には適用しない。推論コストと latency を抑えるため、risk に応じて段階適用する。

| level | 対象 | 処理 | 既定挙動 |
| --- | --- | --- | --- |
| low | casual chat、emotional response、創作的な短文 | 原則 `not_checked`。必要なら quick consistency check | 回答本文は既存通り。report は minimal |
| medium | memory_reference、recommendation、knowledge_db_answer、作品情報、ユーザー嗜好参照 | claim extraction、memory / KB / search cache check、final revision | 弱い根拠は断定を弱める |
| high | news、factual_claim、citation_required、user_memory_write、external_search_result、Source Registry promote に関係する回答 | claim extraction、verification questions、independent retrieval、source check、contradiction check、final revision | unsupported / conflict を明示し、未検証の断定を避ける |

初期実装では `medium` と `high` の dry-run から始める。実回答の書き換えは、report が安定してから有効化する。

## Evidence Source

RenCrow で使用できる evidence source は次である。

| source | 位置づけ | 主な実装候補 | 注意 |
| --- | --- | --- | --- |
| RecallPack | prompt 注入用の文脈 | `internal/domain/conversation/recall_pack.go` | prompt text だけを真実の保存先にしない |
| conversation memory | 会話履歴、summary、thread memory | `internal/domain/conversation`, `internal/infrastructure/persistence/conversation` | Viewer 表示 state と混同しない |
| L1SQLite | event、staging、source registry、news、knowledge、search cache | `l1_sqlite_*.go` | state transition を飛ばさない |
| VectorDB thread memory | 類似する過去会話 / long facts | `vectordb_thread_memory.go` | KB と thread memory を混同しない |
| VectorDB KB | Knowledge DB vector search | `vectordb_kb.go` | Source Registry や archive まで吸収しない |
| DuckDB archive | archive、thread summary、export | `duckdb_*.go` | live state ではなく archive として扱う |
| Source Registry | 外部ソースの observed / candidate / validated / promoted | `sourcefetcher`, `l1_sqlite_source_registry.go` | 無審査 promote 禁止 |
| search cache | fresh cache / similar cache | `l1_sqlite_search_cache.go`, ToolRunner web search cache | TTL / retrieved_at を見る |
| raw external source | 外部取得本文 | Source Registry staging の raw text 等 | LLM summary draft を根拠にしない |
| execution report / evidence | Worker execution の追跡 | `internal/domain/execution`, `internal/infrastructure/persistence/execution` | ユーザー向け本文と混同しない |

LLM raw output は evidence source ではない。raw log は provider 応答の観測用であり、Viewer 表示本文や検証根拠と分ける。

## Memory / Source Registry との関係

CoVe は Memory / Source Registry の状態遷移を置き換えない。

維持する状態遷移:

```text
observed
  -> candidate / staging
  -> validated or rejected
  -> promoted to memory / news / knowledge
```

重要な境界:

- CoVe の `verified` は Source Registry の `promoted` ではない。
- Source Registry の `validated` / `promoted` は永続化 state である。
- VerificationReport は回答時点の検証 trace である。
- ユーザー記憶保存では high verification を要求する。
- 未検証 source を正式 memory / knowledge にしない。
- `RawText` と `SummaryDraft` を混同しない。
- prompt 注入された文脈を根拠そのものとして扱わない。

## Viewer / TTS / Log との関係

Viewer 表示本文、verification report、raw log、TTS chunk、lipsync trigger は別契約である。

| 領域 | 扱い |
| --- | --- |
| Viewer 表示本文 | ユーザーが読む最終本文。検証により弱められた表現や conflict 表示を含められる |
| VerificationReport | claim、status、evidence_ref、reason、dependency state を持つ診断情報 |
| raw log | provider 応答の観測用。根拠扱いしない |
| TTS chunk | 音声再生と口パク trigger。本文表示や検証根拠にしない |
| event log | 状態遷移と追跡の記録。Viewer 表示契約そのものではない |

Viewer に出す場合:

- `/viewer/evidence/*` または job detail に verification summary を投影する。
- `verified` / `weakly_supported` / `unsupported` / `conflict` / `not_checked` を区別して表示する。
- conflict は通常の成功表示に埋め込まない。
- Viewer 表示 state と永続化 state を同じものとして扱わない。

TTS 連携:

- Phase35 初期実装では TTS chunk の契約を変更しない。
- 回答本文を書き換える段階では、TTS に渡す本文が revised response か draft response かを実装前に確定する。
- TTS / lipsync の検証は Viewer / audio の実セッション確認を含める。

## Chat / Worker / Coder との関係

Chat / Worker / Coder の責務境界を維持する。

| 役割 | Phase35 での扱い |
| --- | --- |
| Chat | ユーザー対話、route 判断、結果返却を維持する。Verification Pipeline の結果を最終応答に反映するが、破壊的操作はしない |
| Worker | 実行主体として、検証 pipeline の実行、ログ記録、外部依存確認を担える |
| Coder | plan / patch / proposal の生成のみ。CoVe の適用や破壊的操作を直接実行しない |

Coder route では、Coder proposal を CoVe で「実行可能」と保証しない。Coder の成果物は候補であり、実行、採用、ログ記録、安全確認は Worker が担当する。

## LLM provider との関係

既存 provider を使う。

方針:

- CoVe 専用 provider を最初から作らない。
- `internal/domain/llm.LLMProvider` の既存 `Generate` 契約を使う。
- role ごとの provider 境界を崩さない。
- provider factory、middleware、raw log の責務を増やさない。
- raw log middleware と VerificationReport を分ける。
- provider fallback は正常系ではない。

推奨:

- 初期実装では Chat / Heavy / Worker の既存 provider から verifier に使う provider を config で選べるようにする。
- 検証用 provider が未設定なら `not_checked` と skip reason を返す。
- provider error / timeout / empty response は `not_checked` または `conflict` 相当の report error として扱い、成功扱いしない。

## 段階実装計画

### Phase35-1 domain contract 作成

対象:

- `internal/domain/verification`

内容:

- `VerificationStatus`
- `Claim`
- `VerificationQuestion`
- `EvidenceRef`
- `VerificationReport`
- `VerificationPolicy`
- validation helper

完了条件:

- status の意味が unit test で確認されている。
- invalid status / empty claim の扱いが明確である。
- LLM / DB / Viewer へ依存していない。

### Phase35-2 application pipeline の dry-run 実装

対象:

- `internal/application/verification`

内容:

- `Verifier` interface。
- `Pipeline`。
- claim extraction。
- policy 判定。
- dry-run report。

完了条件:

- draft を書き換えずに VerificationReport だけ返せる。
- low / medium / high の trigger policy を判定できる。
- unsupported / conflict を success にしない。

### Phase35-3 MessageOrchestrator への optional hook 接続

対象:

- `internal/application/orchestrator/message_orchestrator_*.go`
- `cmd/picoclaw/runtime_verification.go`

内容:

- `SetVerificationPipeline` などの hook を追加する。
- `routeDispatcher.ExecuteTask` 後、`responses.Build` 前に report を得る。
- 初期状態では disabled で既存挙動を維持する。

完了条件:

- verifier 未設定時に既存テストが通る。
- verifier 設定時に job_id / session_id / route 付き report が作られる。
- handler / DTO / SSE event / TTS chunk は変更しない。

### Phase35-4 Memory / KB / Source Registry evidence 接続

対象:

- `internal/application/verification`
- `internal/infrastructure/persistence/conversation`
- `internal/application/sourcefetcher`

内容:

- `EvidenceReader` interface を定義する。
- RecallPack、conversation memory、L1SQLite、VectorDB KB、search cache、Source Registry を evidence source として読む。
- `RawText` と `SummaryDraft` を区別する。

完了条件:

- LLM raw output を evidence として使っていない。
- Source Registry の未検証 state が promoted 扱いされない。
- search cache は TTL / retrieved_at を見て扱う。

### Phase35-5 Viewer 観測表示

対象:

- `internal/adapter/viewer`

内容:

- verification report を job detail / evidence panel に投影する。
- status、claim count、conflict count、unsupported count、evidence summary を表示する。

完了条件:

- Viewer 表示本文と report が分かれている。
- raw log と report が分かれている。
- DOM 存在だけでなく最低 1 session の表示確認を行う。

### Phase35-6 high-risk trigger のみ有効化

対象:

- `internal/application/verification`
- runtime config

内容:

- news、factual_claim、citation_required、user_memory_write、external_search_result を high として扱う。
- medium / low は dry-run または `not_checked` から開始する。

完了条件:

- 全回答に強制適用していない。
- high-risk で unsupported / conflict が表示または report される。
- latency / token cost の増加が観測できる。

### Phase35-7 e2e / live 確認

対象:

- Viewer / route / Memory / Source Registry / provider

内容:

- high-risk answer の検証 report を E2E で確認する。
- live service で `/health` と Viewer 表示を確認する。
- external dependency skip は成功扱いしない。

完了条件:

- `go test ./internal/domain/verification ./internal/application/verification` が成功している。
- `go test ./internal/application/orchestrator` が成功している。
- `go test ./internal/infrastructure/persistence/conversation ./internal/application/sourcefetcher ./internal/adapter/viewer` が必要範囲で成功している。
- `go test -count=1 -tags=e2e ./test/e2e` または明記した代替確認が成功している。

## テスト方針

### unit test

対象:

- `internal/domain/verification`
- `internal/application/verification`

確認:

- status validation。
- empty claim rejection。
- evidence source type validation。
- trigger policy 判定。
- unsupported / conflict が success に変換されない。

### integration test

対象:

- `internal/application/orchestrator`
- `internal/infrastructure/persistence/conversation`
- `internal/application/sourcefetcher`

確認:

- `MessageOrchestrator` hook が verifier 未設定時に既存挙動を変えない。
- verifier 設定時に job_id / session_id / route 付き report が返る。
- Source Registry 無審査 promote が起きない。
- validated / promoted / rejected が区別される。
- search cache の fresh / expired が区別される。

### Viewer / E2E

確認:

- Viewer 表示本文、verification report、event log、raw log が混ざらない。
- TTS chunk が検証根拠にならない。
- high-risk answer で verification status が確認できる。
- conflict / unsupported が通常成功表示に埋もれない。

### 推奨コマンド

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/verification ./internal/application/verification
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/persistence/conversation ./internal/application/sourcefetcher ./internal/adapter/viewer
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
git diff --check
```

実装途中で `internal/domain/verification` や `internal/application/verification` が未作成の場合、該当 package のテストは作成後に実行する。未作成 package の skip は成功扱いしない。

## エラー契約

| 状況 | 扱い |
| --- | --- |
| verifier disabled | `not_checked`。disabled reason を report に残す |
| provider 未設定 | `not_checked`。dependency unavailable として扱う |
| provider error / timeout | `not_checked` または `conflict`。成功扱いしない |
| evidence source unavailable | source ごとに unavailable を記録し、high-risk では断定を避ける |
| claim extraction failed | report error。draft を無検証成功として隠さない |
| unsupported claim | 断定しない。削除または未確認表現へ修正 |
| conflicting evidence | conflict として表示 / report。片方を勝手に採用しない |
| report persistence failed | 回答本文と分離して error を返すか、report unavailable を記録する |

## 実装時の禁止事項

- CoVe を正解保証として扱う。
- 「94%高精度」を仕様文言や UI 文言にする。
- fallback を正常系として扱う。
- LLM raw log を evidence として扱う。
- Viewer 表示本文、verification report、TTS chunk、Source Registry state を同じ DTO に混ぜる。
- Source Registry を無審査で正式 memory / knowledge に昇格する。
- Coder に CoVe 適用や破壊的操作を直接実行させる。
- `cmd/picoclaw` に検証ロジック本体を置く。
- 巨大な `service` / `manager` / `helper` / `util` を新設する。
- provider factory や raw log middleware に verification 判断を混ぜる。

## 実装前に確認すること

1. Verification Pipeline を最初は dry-run にするか、final revision まで有効にするか。
2. TTS に渡す本文を draft のままにするか、revised response にするか。
3. Viewer に verification status をどの画面へ表示するか。
4. VerificationReport を専用 persistence package にするか、conversation persistence へ最小追加するか。
5. high-risk trigger の初期判定を route ベースにするか、claim extraction 結果ベースにするか。
6. 外部検索を Phase35 に含めるか、search cache / Source Registry / KB のみから始めるか。

## 完了条件

Phase35 実装仕様としての完了条件は次である。

- CoVe の扱いが RenCrow の既存モジュール構成に対応している。
- `internal/domain/verification`、`internal/application/verification`、persistence、`cmd/picoclaw/runtime_verification.go`、Viewer の責務が明記されている。
- `MessageOrchestrator` への挿入位置が明記されている。
- Verification Pipeline の入力、出力、副作用、永続化、ログ、エラー契約が明記されている。
- `verified` / `weakly_supported` / `unsupported` / `conflict` / `not_checked` の意味が明記されている。
- Trigger Policy が low / medium / high で定義されている。
- Evidence Source として使える既存 RenCrow モジュールが整理されている。
- Memory / Source Registry、Viewer / TTS / Log、Chat / Worker / Coder、LLM provider との境界が明記されている。
- 段階実装計画とテスト方針が明記されている。
- 仕様外の LangGraph や未定義キャラクター名を前提にしていない。
- コード変更は行っていない。
- ユーザーが次に「実装してよいか」を判断できる。
