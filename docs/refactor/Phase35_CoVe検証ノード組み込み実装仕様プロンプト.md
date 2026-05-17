# Phase35 CoVe 検証ノード組み込み実装仕様プロンプト

```text
Goal:
  RenCrow に Chain-of-Verification 風の回答検証システムを組み込むための実装仕様書を作成してください。

Repository:
  /home/nyukimi/picoclaw_multiLLM

目的:
  - CoVe を単なるプロンプト技術ではなく、RenCrow の会話生成後に置く Verification Pipeline として設計する。
  - 回答前に claim を抽出し、memory / KB / Source Registry / search cache / 必要時の外部検索に照らして検証できる構造にする。
  - CoVe の結果を正解保証として扱わず、verified / weakly_supported / unsupported / conflict / not_checked のような検証状態として扱う。
  - RenCrow の既存方針であるモジュール化、疎結合、Chat / Worker / Coder 責務境界、Viewer 表示・音声・口パク・ログ分離を崩さない。

必ず参照するもの:
  1. AGENTS.md
  2. CLAUDE.md
  3. docs/10_新仕様/00_README.md
  4. docs/10_新仕様/01_新仕様_概要.md
  5. docs/10_新仕様/02_モジュール構成仕様.md
  6. docs/10_新仕様/03_モジュール関係図.html
  7. docs/10_新仕様/04_Chat_Worker_Coder仕様.md
  8. docs/10_新仕様/05_Viewer仕様.md
  9. docs/10_新仕様/08_LLM_provider仕様.md
  10. docs/10_新仕様/09_Memory_SourceRegistry仕様.md
  11. docs/10_新仕様/10_検証仕様.md
  12. docs/10_新仕様/11_分割再設計候補.md
  13. Chain-of-Verification 論文:
      - https://arxiv.org/abs/2309.11495
      - https://aclanthology.org/2024.findings-acl.212/
  14. 現在の実装コード:
      - internal/application/orchestrator/message_orchestrator_*.go
      - internal/application/orchestrator/message_orchestrator_response.go
      - internal/application/orchestrator/message_orchestrator_routes.go
      - internal/domain/conversation/recall_pack.go
      - internal/infrastructure/persistence/conversation/l1_sqlite_*.go
      - internal/application/sourcefetcher
      - internal/infrastructure/llm
      - internal/adapter/viewer

作成する文書:
  - docs/refactor/Phase35_CoVe検証ノード組み込み実装仕様.md

制約:
  - この作業では実装仕様書だけを作成する。
  - コード変更はしない。
  - docs/refactor/ 配下の Markdown 追加だけにする。
  - TODO / TBD の仮置きは残さない。
  - CoVe を正解保証として扱わない。
  - 「94%高精度」のような一次論文で代表表現として確認できない宣伝的表現は採用しない。
  - fallback を正常系として扱わない。
  - LLM raw log、Viewer 表示本文、TTS chunk、検証 report、Source Registry state を混同しない。
  - Source Registry を無審査で正式 memory / knowledge に昇格しない。
  - Coder に破壊的操作を直接実行させない。
  - cmd/picoclaw に検証ロジック本体を置かない。
  - 巨大な service / manager / helper / util を新設しない。

文書に必ず含める内容:

1. 目的
   - CoVe を RenCrow の Verification Pipeline として扱うこと。
   - 会話生成後、最終表示・TTS 前に検証状態を付与すること。
   - 誤記憶、誤引用、誤要約、未検証ソースの断定を減らすこと。

2. CoVe の事実確認
   - 論文上の基本手順:
     - draft response
     - verification questions
     - independent answers
     - final verified response
   - RenCrow では factored CoVe 寄りにし、検証ノードへ draft 全文を渡しすぎない方針。
   - CoVe はハルシネーションを減らす手法であり、完全な正解保証ではないこと。

3. RenCrow への組み込み位置
   - MessageOrchestrator の route-specific generation 後。
   - ProcessMessageResponse を返す前。
   - Viewer final 表示、TTS、channel response の前。
   - ただし handler、DTO、SSE event、TTS chunk、Viewer 表示契約を一度に変更しない。

4. 提案モジュール構成
   - internal/domain/verification
     - Claim
     - VerificationQuestion
     - EvidenceRef
     - VerificationStatus
     - VerificationReport
     - VerificationPolicy
   - internal/application/verification
     - claim extraction
     - verification planning
     - independent verification
     - evidence evaluation
     - final revision
   - internal/infrastructure/persistence/verification または既存 conversation persistence への最小追加
     - verification report / evidence trace 保存
   - cmd/picoclaw/runtime_verification.go
     - wiring のみ
   - internal/adapter/viewer
     - Viewer に表示する場合のみ verification status を投影

5. 各モジュールの責務
   各モジュールについて以下を書く:
   - 入力
   - 出力
   - 副作用
   - 永続化
   - ログ
   - エラー契約
   - 置いてはいけない責務

6. Verification Pipeline
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

7. VerificationStatus
   最低限以下を定義する:
   - verified
   - weakly_supported
   - unsupported
   - conflict
   - not_checked

   各 status の意味、表示方針、ログ方針を書く。

8. Trigger Policy
   全回答に適用しない。
   以下のような段階適用を設計する:
   - low:
     - casual_chat
     - emotional_response
     - 原則 not_checked または quick consistency check
   - medium:
     - memory_reference
     - recommendation
     - knowledge_db_answer
     - claim_extract + memory / KB check + final revision
   - high:
     - news
     - factual_claim
     - citation_required
     - user_memory_write
     - external_search_result
     - claim_extract + verification questions + independent retrieval + source check + contradiction check + final revision

9. Evidence Source
   RenCrow で使える根拠候補を整理する:
   - RecallPack
   - conversation memory
   - L1SQLite
   - VectorDB thread memory
   - VectorDB KB
   - DuckDB archive
   - Source Registry
   - search cache
   - raw external source
   - execution report / evidence

   ただし LLM raw output を根拠扱いしないことを明記する。

10. Memory / Source Registry との関係
   - observed / candidate / validated / promoted を維持する。
   - CoVe の verified と Source Registry の promoted を混同しない。
   - ユーザー記憶保存では high verification を要求する。
   - 未検証 source を正式 memory / knowledge にしない。

11. Viewer / TTS / Log との関係
   - Viewer 表示本文と verification report を分ける。
   - TTS chunk を検証対象の根拠にしない。
   - raw log を表示本文や根拠と混同しない。
   - Viewer に出す場合は verification status / evidence summary / conflict warning を別表示にする。

12. Chat / Worker / Coder との関係
   - Chat はユーザー対話、route 判断、結果返却を維持する。
   - Worker は実行主体であり、検証 pipeline の実行やログ記録を担える。
   - Coder は plan / patch / proposal 生成のみ。
   - Coder に CoVe の適用や破壊的操作を直接実行させない。

13. LLM provider との関係
   - 既存 provider を使う。
   - CoVe 専用 provider は最初から作らない。
   - role ごとの provider 境界を崩さない。
   - raw log middleware と verification report を分ける。

14. 段階実装計画
   Phase35 内でも小段階に分ける:
   - 35-1 domain contract 作成
   - 35-2 application pipeline の dry-run 実装
   - 35-3 MessageOrchestrator への optional hook 接続
   - 35-4 Memory / KB / Source Registry evidence 接続
   - 35-5 Viewer 観測表示
   - 35-6 high-risk trigger のみ有効化
   - 35-7 e2e / live 確認

15. テスト方針
   - domain verification status の unit test
   - claim extraction の contract test
   - evidence evaluation の unit test
   - MessageOrchestrator hook の integration test
   - Source Registry 無審査 promote が起きないこと
   - unsupported / conflict が fallback 成功扱いされないこと
   - Viewer 表示、TTS、raw log、verification report が混ざらないこと
   - 必要なら e2e で high-risk answer の検証表示を確認すること

16. 完了条件
   - 実装仕様書が docs/refactor/ に作成されている。
   - CoVe の扱いが RenCrow の既存モジュール構成に対応している。
   - どのモジュールに何を置くかが明記されている。
   - 実装順と検証条件が明記されている。
   - 仕様外の LangGraph や未定義キャラクター名を前提にしていない。
   - ユーザーが次に「実装してよいか」を判断できる。

実行手順:
  1. git status を確認する。
  2. 「必ず参照するもの」に列挙した文書・論文・実装コードを読む。
  3. CoVe 論文の一次情報を確認する。
  4. 現在の MessageOrchestrator / Memory / Source Registry / LLM provider / Viewer の実装境界を確認する。
  5. 組み込み可能性を仕様として整理する。
  6. docs/refactor/Phase35_CoVe検証ノード組み込み実装仕様.md を作成する。
  7. コード変更は行わない。
  8. git diff --check を実行する。
  9. 最後に、作成ファイル、仕様の要点、実装前に確認すべきことを報告する。
```
