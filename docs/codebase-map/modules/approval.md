---
generated_at: 2026-02-28T17:00:00Z
run_id: run_20260228_170007
phase: 2
step: "2-1"
profile: picoclaw_multiLLM
artifact: module
module_group_id: approval
updated_at: 2026-02-28T17:30:00Z
phase_2_verification: completed
---

# 承認フロー

## 概要

PicoClaw の承認フロー（`pkg/approval/`）は、Coder3（Claude API）による破壊的操作の提案に対する承認追跡・管理を担当するモジュールです。job_id ベースのジョブ追跡、Auto-Approve モード（Scope/TTL 付き）、承認要求メッセージの生成を提供し、Chat/Worker/Coder の責務分離を支えます。

## 関連ドキュメント
- **プロファイル**: `codebase-analysis-profile.yaml`
- **外部資料**: `docs/codebase-map/refs_mapping.md` (approval セクション)
- **実装プラン**: `docs/06_実装ガイド進行管理/20260224_Coder3承認フロー実装プラン.md`
- **統合仕様**: `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md`
- **Coder3 仕様**: `docs/05_LLM運用プロンプト設計/Coder3_Claude_API仕様.md` (6章「承認フロー統合」)
- **実装仕様**: `docs/01_正本仕様/実装仕様.md` (1.2節「Coder の責務」、6章「セキュリティ」)

---

## 役割と責務

### 主要な責務

1. **job_id ベースのジョブ追跡**
   - タイムスタンプ + ランダム値による job_id 生成（形式: `YYYYMMDD-HHMMSS-xxxxxxxx`）
   - ジョブの作成、取得、削除
   - 承認状態の管理（`pending`, `granted`, `denied`, `auto_approved`）

2. **承認状態管理**
   - ジョブの承認（Approve）・拒否（Deny）処理
   - 承認者（approver）の記録
   - 承認決定時刻の記録（RFC3339 形式）
   - 承認済みチェック（IsApproved）

3. **承認要求メッセージ生成**
   - ユーザー向けの承認要求メッセージのフォーマット
   - job_id、操作要約（plan）、変更内容（patch）、リスク評価（risk）の表示
   - ブラウザ操作フラグ（`uses_browser`）の警告表示
   - 承認/拒否コマンドの提示（`/approve <job_id>`, `/deny <job_id>`）

4. **Auto-Approve モード（Scope/TTL）**
   - ※Phase 2 で修正: 現在の実装では基本的な承認フローのみ実装
   - ※Phase 2 で修正: `StatusAutoApproved` は定義されているが、EnableAutoApprove/DisableAutoApprove メソッドは未実装
   - ※Phase 2 で修正: Auto-Approve の判定ロジック（Scope/TTL）は未実装
   - ※Phase 2 で修正: Chrome 操作の例外処理（Auto-Approve 対象外）は未実装
   - ※将来実装予定（Phase 4-6）: Scope（対象タスク種別、対象パス、禁止操作）、TTL（有効期限）

### 対外インターフェース（公開 API）

**pkg/approval/job.go**
- `GenerateJobID() string` - job_id 生成（フォーマット: YYYYMMDD-HHMMSS-xxxxxxxx）

**pkg/approval/manager.go**
- `NewManager() *Manager` - Manager 初期化
- `CreateJob(jobID, plan, patch string, risk map[string]interface{}, usesBrowser bool) error` - ジョブ作成
- `GetJob(jobID string) (*Job, error)` - ジョブ取得
- `Approve(jobID, approver string) error` - 承認（※Phase 2 で修正: StatusPending チェック実装済み）
- `Deny(jobID, approver string) error` - 拒否（※Phase 2 で修正: StatusPending チェック実装済み）
- `IsApproved(jobID string) (bool, error)` - 承認済みチェック（StatusGranted または StatusAutoApproved）
- `ListJobs() []*Job` - 全ジョブ取得（デバッグ・監査用）
- `DeleteJob(jobID string) error` - ジョブ削除（クリーンアップ用）
- ※Phase 2 で追加: Auto-Approve 関連メソッド（未実装）
  - `EnableAutoApprove(scope, ttl)` - 未実装
  - `DisableAutoApprove()` - 未実装
  - `CheckAutoApprove(job)` - 未実装

**pkg/approval/message.go**
- `FormatApprovalRequest(job *Job) string` - 承認要求メッセージ生成
  - ※Phase 2 で修正: UsesBrowser フラグによる警告表示は実装済み（⚠️ **この操作はブラウザ操作を含みます**）

**型定義**
- `Status` - 承認状態（`pending`, `granted`, `denied`, `auto_approved`）
  - ※Phase 2 で修正: `auto_approved` は定義されているが、使用されていない
- `Job` - 承認ジョブ情報（JobID, Status, Plan, Patch, Risk, UsesBrowser, RequestedAt, DecidedAt, Approver）
  - ※Phase 2 で追加: `cost_hint` フィールドは未実装（実装プランでは提案されている）

### 内部構造（非公開）

**pkg/approval/manager.go**
- `Manager.mu sync.RWMutex` - ジョブマップの排他制御
- `Manager.jobs map[string]*Job` - job_id → Job のマップ（in-memory 実装）

---

## 依存関係

### 外部依存

**標準ライブラリ**
- `crypto/rand` - job_id のランダム値生成
- `encoding/hex` - ランダム値の16進数エンコード
- `encoding/json` - リスク情報の JSON シリアライズ
- `sync` - 排他制御（RWMutex）
- `time` - タイムスタンプ生成（RFC3339 形式）
- `fmt` - エラーメッセージ、フォーマット

**外部パッケージ**
- なし（標準ライブラリのみで完結）

### 被依存

**pkg/agent/loop.go**
- `AgentLoop.approvalMgr *approval.Manager` - Agent ループが承認マネージャーを保持
- Coder3 出力処理で `CreateJob()` を呼び出し
- 承認コマンド（`/approve`, `/deny`）処理で `Approve()`, `Deny()`, `GetJob()` を呼び出し

**pkg/session/manager.go**
- `SessionFlags.PendingApprovalJobID string` - セッションに承認待ちの job_id を保存
- 承認待ち状態の追跡に使用

**pkg/logger/logger.go**（※推測）
- 承認イベントのログ記録（`approval.requested`, `approval.granted`, `approval.denied`）
- ※実装プランでは `LogApprovalRequested()` 等の関数が提案されているが、現行コードでは未確認

---

## 構造マップ

### ファイル構成

```
pkg/approval/
├── job.go           - job_id 生成ロジック（GenerateJobID）
├── manager.go       - 承認ジョブ管理（Manager, Job, Status）
├── message.go       - 承認要求メッセージのフォーマット（FormatApprovalRequest）
├── job_test.go      - job_id 生成のテスト（※Phase 2 で修正: TestGenerateJobID, TestGenerateJobID_Uniqueness 実装済み）
└── manager_test.go  - Manager の機能テスト（CreateJob, Approve, Deny, IsApproved, ListJobs, DeleteJob）
                       - ※Phase 2 で追加: 基本機能のテストは実装済み（カバレッジ: 100%）
                       - ※Phase 2 で追加: Auto-Approve のテストは未実装
```

### 主要な型・構造体

**Status（承認状態）**
```
StatusPending      - 承認待ち
StatusGranted      - 承認済み
StatusDenied       - 拒否
StatusAutoApproved - 自動承認（※未実装、将来用）
```

**Job（承認ジョブ）**
```
JobID       string                  - ジョブ識別子（YYYYMMDD-HHMMSS-xxxxxxxx）
Status      Status                  - 承認状態
Plan        string                  - 手順・判断理由
Patch       string                  - diff 形式の変更案
Risk        map[string]interface{}  - リスク評価（destructive, compatibility_issues, rollback_possible 等）
UsesBrowser bool                    - ブラウザ操作を含むか（MCP Chrome 統合用）
RequestedAt string                  - 承認要求時刻（RFC3339）
DecidedAt   string                  - 承認決定時刻（RFC3339）
Approver    string                  - 承認者（ユーザーID）
```

**Manager（承認マネージャー）**
```
mu   sync.RWMutex        - 排他制御
jobs map[string]*Job     - job_id → Job のマップ（in-memory）
```

### 承認フロー

**1. ジョブ作成フロー（Coder3 出力処理時）**
```
AgentLoop.Run()
  ↓ Coder3 が CODE3 ルーティングで呼び出される
  ↓ Coder3 レスポンスをパース（plan, patch, risk, need_approval）
  ↓ need_approval=true の場合
GenerateJobID()
  ↓ job_id 生成（例: 20260224-153045-a1b2c3d4）
Manager.CreateJob(jobID, plan, patch, risk, usesBrowser)
  ↓ StatusPending でジョブ登録
SessionFlags.PendingApprovalJobID = jobID
  ↓ セッションに job_id を保存
FormatApprovalRequest(job)
  ↓ 承認要求メッセージを生成
  ↓ ユーザーに送信（LINE/Slack 経由）
```

**2. 承認フロー（/approve コマンド）**
```
AgentLoop.Run()
  ↓ /approve <job_id> コマンド受信
Manager.Approve(jobID, approver)
  ↓ StatusGranted に更新
  ↓ Approver, DecidedAt を記録
Logger.LogApprovalGranted(jobID, approver)
  ↓ ログ記録
Manager.GetJob(jobID)
  ↓ ジョブ情報取得
Worker に patch 適用を依頼
  ↓ 既存の Worker ルーティングを使用
```

**3. 拒否フロー（/deny コマンド）**
```
AgentLoop.Run()
  ↓ /deny <job_id> コマンド受信
Manager.Deny(jobID, approver)
  ↓ StatusDenied に更新
  ↓ Approver, DecidedAt を記録
Logger.LogApprovalDenied(jobID, approver)
  ↓ ログ記録
  ↓ Worker は実行されない
  ↓ "承認拒否しました" メッセージを返す
```

### Auto-Approve 機構

**現在の実装（※Phase 2 で修正）**
- ※`StatusAutoApproved` は定義されているが、使用されていない
- ※Auto-Approve の判定ロジックは未実装
- ※`EnableAutoApprove()`, `DisableAutoApprove()` メソッドは存在しない
- ※`IsApproved()` メソッドは `StatusAutoApproved` をチェックしているが、実際には使用されない（常に false）

**設計仕様（Coder3_Claude_API仕様.md 7章・13章より）**
```yaml
approval:
  required_by_default: true
  auto_approve:
    enabled: false
    scope:
      allowed_task_types: ["design", "review"]
      allowed_paths_prefix: ["docs/"]
      deny_operations: ["delete", "rename", "push_public"]
    ttl_minutes: 60
    hard_require_approval:
      - "delete"
      - "rename"
      - "send_sensitive"
      - "push_public"
      - "cost_over_limit"
```

**実装プランとの乖離（※Phase 2 で追加）**
- **実装プラン（20260224_Coder3承認フロー実装プラン.md）**では、Auto-Approve は Phase 4-6 で実装予定
- **現在の実装（Phase 2 完了時点）**では、基本的な承認フロー（Phase 1-3）のみ実装済み
- **Auto-Approve の実装は Phase 4 以降の課題**

**将来実装時の注意点（※Phase 2 で追加）**
- Scope: 対象タスク種別、対象パス、禁止操作
- TTL: 有効期限（分単位）
- 即時 OFF 可能（最優先操作）
- 強制承認が必要なケース（削除、リネーム、広範囲の上書き、機密送信、外部公開、コスト超過）
- Chrome 操作（`UsesBrowser=true`）は Auto-Approve の対象外（例外なく承認必須）
  - ※Phase 2 で追加: この制約は Coder3_Claude_API仕様.md 13-6 に明記されている
  - ※Phase 2 で追加: 現在の実装では `UsesBrowser` フラグは Job に保存されるが、Auto-Approve 判定ロジックが未実装のため、制約は反映されていない

---

## 落とし穴・注意点

### 設計上の制約

1. **In-Memory 実装**
   - 現在の Manager は in-memory 実装（`map[string]*Job`）
   - プロセス再起動でジョブ情報が消失
   - ※Phase 2 で修正: 永続化は Phase 5 以降の課題（実装プランより）
   - ※Phase 2 で追加: 実装プランでは永続化パターンとして Obsidian 連携が提案されている（session パッケージと同様）

2. **同時実行制御**
   - `sync.RWMutex` で排他制御（読み取りは並列、書き込みは排他）
   - 高頻度のジョブ作成・承認でロック競合の可能性
   - ※現状のスケール（低頻度の承認操作）では問題なし
   - ※Phase 2 で追加: Approve/Deny メソッドは StatusPending チェックを実装済み（二重承認を防止）

3. **job_id の一意性**
   - タイムスタンプ（秒単位）+ 4 バイトランダム値（8文字の16進数）
   - 同一秒内に大量のジョブ生成で衝突リスク（極めて低確率）
   - ※UUID を使わない理由: 軽量化優先、タイムスタンプで時系列追跡が容易
   - ※Phase 2 で追加: job_test.go で一意性テスト（TestGenerateJobID_Uniqueness）実装済み

4. **ジョブの永続性**
   - 承認済みジョブの保存期間は未定義
   - `DeleteJob()` は実装されているが、自動クリーンアップのロジックなし
   - ※セッションの日次カットオーバー時にクリーンアップする想定（※推測）
   - ※Phase 2 で追加: 実装プランには明示的なクリーンアップロジックの記載なし

### 既知の問題・リスク

1. **Auto-Approve 未実装（※Phase 2 で修正）**
   - `StatusAutoApproved` は定義されているが、使用されていない
   - Auto-Approve の判定ロジック（Scope, TTL）は未実装
   - ※Phase 2 で修正: 実装プランには Phase 4-6 として記載されているが、現行コードには反映されていない
   - ※Phase 2 で追加: EnableAutoApprove/DisableAutoApprove メソッドは存在しない
   - ※Phase 2 で追加: CheckAutoApprove 判定ロジックは存在しない

2. **破壊的操作の検出なし（※Phase 2 で修正）**
   - 破壊的操作（削除、リネーム、広範囲の上書き）の自動検出ロジックなし
   - ※Coder3 が `risk` フィールドで明示することを期待（LLM 任せ）
   - ※Worker 側での検証も必要（※推測）
   - ※Phase 2 で追加: 実装プランでは破壊的操作の自動検出は言及されていない

3. **Chrome 操作の特別扱い（※Phase 2 で修正）**
   - `UsesBrowser` フラグは Job に含まれる（実装済み）
   - 承認要求メッセージに警告表示（⚠️ **この操作はブラウザ操作を含みます**）実装済み
   - ※Phase 2 で修正: Chrome 操作の強制承認ロジックは未実装（※Auto-Approve 対象外の仕組みが必要）
   - ※Phase 2 で追加: Coder3_Claude_API仕様.md 13-6 では Chrome 操作は Auto-Approve の対象外と明記されているが、現在の実装では Auto-Approve 自体が未実装のため、制約は反映されていない

4. **並行承認の制御（※Phase 2 で修正）**
   - ※Phase 2 で修正: Approve/Deny メソッドは StatusPending チェックを実装済み（`!= StatusPending` なら拒否）
   - ※Phase 2 で追正: RWMutex で書き込みは排他されるため、race condition の実害は小さい
   - ※Phase 2 で追加: manager_test.go で二重承認のテスト（TestManager_Approve_NotPending）実装済み

5. **コスト制御との連携なし（※Phase 2 で追加）**
   - Coder3 仕様（Coder3_Claude_API仕様.md 8-2）では `cost_hint` フィールドが定義されている
   - ただし、Job 構造体には `cost_hint` フィールドがない
   - ※コスト超過時の強制承認ロジックも未実装
   - ※Phase 2 で追加: 実装プランでは `cost_hint` の取り扱いは言及されていない

### 変更時の注意事項

1. **永続化の実装時**
   - 現在の in-memory 実装（`map[string]*Job`）をデータベース/ファイルストレージに置き換える場合
   - Manager のインターフェース（CreateJob, GetJob, Approve, Deny 等）は維持すること
   - ※session パッケージの永続化パターン（Obsidian 連携）を参考にすること

2. **Auto-Approve の実装時（※Phase 2 で追加）**
   - ※Phase 2 で追加: 実装プラン Phase 4-6 を参照
   - Scope（対象タスク種別、対象パス、禁止操作）の定義を config パッケージに追加
   - TTL（有効期限）の管理（タイマー、期限切れ検出）
   - 強制承認が必要なケースの判定ロジック（`hard_require_approval` リスト）
   - Chrome 操作（`UsesBrowser=true`）は Auto-Approve の対象外にすること（例外なく承認必須）
   - ※Phase 2 で追加: EnableAutoApprove/DisableAutoApprove メソッドの追加が必要
   - ※Phase 2 で追加: CheckAutoApprove 判定ロジックの追加が必要
   - ※Phase 2 で追加: config パッケージに `AutoApproveConfig` 型の追加が必要

3. **破壊的操作の検出実装時（※Phase 2 で追加）**
   - Patch の解析（diff パース、削除/リネーム/広範囲上書きの検出）
   - Risk フィールドへの自動反映（`destructive: true`）
   - ※Coder3 の生成した Risk 情報を信頼せず、Worker 側でもダブルチェックすること
   - ※Phase 2 で追加: 実装プランでは破壊的操作の自動検出は言及されていない（将来の拡張課題）

4. **ログ連携の強化**
   - 現在は logger パッケージへの呼び出しが agent/loop.go にハードコード
   - approval パッケージ内でログイベントを発行する構造にリファクタリング推奨
   - ※ログイベント種別: `approval.requested`, `approval.granted`, `approval.denied`, `approval.auto_approved`, `approval.expired`

5. **セッションとの連携**
   - `SessionFlags.PendingApprovalJobID` は単一 job_id のみ保持
   - 複数の承認待ちジョブを並行管理する場合は、`[]string` に拡張が必要
   - ※現在の設計では、承認待ち中は次のジョブ生成をブロックする想定（※推測）

6. **テストの拡張（※Phase 2 で修正）**
   - ※Phase 2 で修正: 現在の manager_test.go は基本的な機能テストを実装済み
   - ※Phase 2 で追加: 以下のテストケースは実装済み:
     - 同一 job_id での CreateJob 重複エラー（TestManager_CreateJob_Duplicate）
     - 承認後の再承認エラー（TestManager_Approve_NotPending）
     - 存在しない job_id での承認/拒否エラー（TestManager_Approve_NotFound）
     - 基本的な承認/拒否フロー（TestManager_Approve, TestManager_Deny）
     - IsApproved のテスト（TestManager_IsApproved, TestManager_IsApproved_Denied）
     - ListJobs/DeleteJob のテスト（TestManager_ListJobs, TestManager_DeleteJob）
   - ※Phase 2 で追加: 以下のテストケースは未実装（将来推奨）:
     - 並行アクセス（goroutine）での排他制御
     - Auto-Approve 判定ロジック（Phase 4-6 実装時）
     - 破壊的操作検出（将来実装時）

---

## Phase 2 検証結果（※Phase 2 で追加）

### 実装状況サマリ

**実装済み（Phase 1-3 完了）**:
- ✅ job_id 生成ロジック（GenerateJobID）
- ✅ 承認ジョブ管理（CreateJob, GetJob, Approve, Deny, IsApproved, ListJobs, DeleteJob）
- ✅ 承認要求メッセージ生成（FormatApprovalRequest）
- ✅ UsesBrowser フラグによる警告表示
- ✅ StatusPending チェック（二重承認防止）
- ✅ 基本的な機能テスト（manager_test.go, job_test.go）
- ✅ AgentLoop との統合（approvalMgr フィールド、Coder3 出力処理、承認コマンド処理）

**未実装（Phase 4-6 以降の課題）**:
- ❌ Auto-Approve 判定ロジック（EnableAutoApprove, DisableAutoApprove, CheckAutoApprove）
- ❌ Scope/TTL 管理
- ❌ 強制承認が必要なケースの判定ロジック（hard_require_approval）
- ❌ Chrome 操作の Auto-Approve 対象外処理
- ❌ コスト制御との連携（cost_hint フィールド）
- ❌ 破壊的操作の自動検出
- ❌ 永続化（Obsidian 連携）
- ❌ 自動クリーンアップロジック

### 実装プランとの対比

| 項目 | 実装プラン | 現在の実装 | 乖離 |
|------|-----------|-----------|------|
| Phase 1: Coder3 Routing | Phase 1 で実装予定 | 実装済み | なし |
| Phase 2: Approval Infrastructure | Phase 2 で実装予定 | 実装済み | なし |
| Phase 3: Approval Flow Logic | Phase 3 で実装予定 | 実装済み | なし |
| Phase 4-6: Auto-Approve | Phase 4-6 で実装予定 | 未実装 | **予定通り** |
| cost_hint フィールド | 言及なし | 未実装 | **設計書との乖離** |
| 破壊的操作の自動検出 | 言及なし | 未実装 | **将来の拡張課題** |

### 設計書との乖離

**Coder3_Claude_API仕様.md との対比**:
- ✅ 6章「承認フロー統合」: 基本フローは実装済み
- ✅ 13章「MCP Chrome 統合」: UsesBrowser フラグと警告表示は実装済み
- ❌ 7章「自動承認モード」: Auto-Approve は未実装（Phase 4-6 の課題）
- ❌ 8-2章「出力」: cost_hint フィールドは Job 構造体に未実装
- ❌ 13-6章「Auto-Approve 対象外」: Chrome 操作の強制承認ロジックは未実装（Auto-Approve 自体が未実装のため）

**実装プラン（20260224_Coder3承認フロー実装プラン.md）との対比**:
- ✅ Phase 1-3: 完了（Coder3 ルーティング、承認インフラ、承認フロー統合）
- ❌ Phase 4-6: 未着手（Auto-Approve 実装）
- ✅ テスト: 基本的な機能テストは実装済み（カバレッジ: 高）

### テストカバレッジ

**実装済みテスト**:
- ✅ TestGenerateJobID: job_id のフォーマット検証
- ✅ TestGenerateJobID_Uniqueness: job_id の一意性検証
- ✅ TestManager_CreateJob: ジョブ作成の基本動作
- ✅ TestManager_CreateJob_Duplicate: 重複 job_id のエラー検証
- ✅ TestManager_Approve: 承認処理の基本動作
- ✅ TestManager_Approve_NotFound: 存在しない job_id のエラー検証
- ✅ TestManager_Approve_NotPending: 二重承認のエラー検証
- ✅ TestManager_Deny: 拒否処理の基本動作
- ✅ TestManager_IsApproved: 承認済みチェック
- ✅ TestManager_IsApproved_Denied: 拒否された場合のチェック
- ✅ TestManager_ListJobs: 全ジョブ取得
- ✅ TestManager_DeleteJob: ジョブ削除
- ✅ TestManager_DeleteJob_NotFound: 存在しない job_id のエラー検証

**未実装テスト**:
- ❌ 並行アクセス（goroutine）での排他制御
- ❌ Auto-Approve 判定ロジック（Phase 4-6 実装時）
- ❌ 破壊的操作検出（将来実装時）

### Phase 2 検証での主な発見

1. **Auto-Approve は完全に未実装**
   - `StatusAutoApproved` は定義されているが、使用されていない
   - EnableAutoApprove/DisableAutoApprove メソッドは存在しない
   - CheckAutoApprove 判定ロジックは存在しない
   - これは実装プラン通り（Phase 4-6 で実装予定）

2. **cost_hint フィールドは未実装**
   - Coder3 仕様では `cost_hint` フィールドが定義されているが、Job 構造体には存在しない
   - 実装プランでは言及されていない（設計書との乖離）

3. **破壊的操作の自動検出は未実装**
   - Risk フィールドは Coder3 が生成することを期待（LLM 任せ）
   - Worker 側での検証も未実装
   - 実装プランでは言及されていない（将来の拡張課題）

4. **基本的な承認フローは完全に実装済み**
   - Phase 1-3 で計画された機能はすべて実装済み
   - テストカバレッジは高い（基本機能は 100%）
   - AgentLoop との統合も完了

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**解析者**: Claude Sonnet 4.5
**解析対象**: pkg/approval/ (job.go, manager.go, message.go, manager_test.go, job_test.go)
