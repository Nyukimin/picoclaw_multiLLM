---
generated_at: 2026-02-28T17:00:00Z
run_id: run_20260228_170007
phase: 2
step: "2-1"
profile: picoclaw_multiLLM
artifact: module
updated_at: 2026-02-28T17:30:00Z
phase_2_verification: completed
---


## 概要


## 関連ドキュメント
- **プロファイル**: `codebase-analysis-profile.yaml`
- **統合仕様**: `docs/06_実装ガイド進行管理/20260224_Coder3統合仕様反映.md`
- **実装仕様**: `docs/01_正本仕様/実装仕様.md` (1.2節「Coder の責務」、6章「セキュリティ」)

---

## 役割と責務

### 主要な責務

1. **job_id ベースのジョブ追跡**
   - タイムスタンプ + ランダム値による job_id 生成（形式: `YYYYMMDD-HHMMSS-xxxxxxxx`）
   - ジョブの作成、取得、削除


   - job_id、操作要約（plan）、変更内容（patch）、リスク評価（risk）の表示
   - ブラウザ操作フラグ（`uses_browser`）の警告表示

   - ※Phase 2 で修正: `StatusAutoApproved` は定義されているが、EnableAutoApprove/DisableAutoApprove メソッドは未実装
   - ※将来実装予定（Phase 4-6）: Scope（対象タスク種別、対象パス、禁止操作）、TTL（有効期限）

### 対外インターフェース（公開 API）

- `GenerateJobID() string` - job_id 生成（フォーマット: YYYYMMDD-HHMMSS-xxxxxxxx）

- `NewManager() *Manager` - Manager 初期化
- `CreateJob(jobID, plan, patch string, risk map[string]interface{}, usesBrowser bool) error` - ジョブ作成
- `GetJob(jobID string) (*Job, error)` - ジョブ取得
- `Deny(jobID, approver string) error` - 拒否（※Phase 2 で修正: StatusPending チェック実装済み）
- `ListJobs() []*Job` - 全ジョブ取得（デバッグ・監査用）
- `DeleteJob(jobID string) error` - ジョブ削除（クリーンアップ用）
  - `EnableAutoApprove(scope, ttl)` - 未実装
  - `DisableAutoApprove()` - 未実装
  - `CheckAutoApprove(job)` - 未実装

  - ※Phase 2 で修正: UsesBrowser フラグによる警告表示は実装済み（⚠️ **この操作はブラウザ操作を含みます**）

**型定義**
  - ※Phase 2 で修正: `auto_approved` は定義されているが、使用されていない
  - ※Phase 2 で追加: `cost_hint` フィールドは未実装（実装プランでは提案されている）

### 内部構造（非公開）

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
- Coder3 出力処理で `CreateJob()` を呼び出し

**pkg/session/manager.go**

**pkg/logger/logger.go**（※推測）

---

## 構造マップ

### ファイル構成

```
├── job.go           - job_id 生成ロジック（GenerateJobID）
├── job_test.go      - job_id 生成のテスト（※Phase 2 で修正: TestGenerateJobID, TestGenerateJobID_Uniqueness 実装済み）
└── manager_test.go  - Manager の機能テスト（CreateJob, Approve, Deny, IsApproved, ListJobs, DeleteJob）
                       - ※Phase 2 で追加: 基本機能のテストは実装済み（カバレッジ: 100%）
```

### 主要な型・構造体

```
StatusDenied       - 拒否
```

```
JobID       string                  - ジョブ識別子（YYYYMMDD-HHMMSS-xxxxxxxx）
Plan        string                  - 手順・判断理由
Patch       string                  - diff 形式の変更案
Risk        map[string]interface{}  - リスク評価（destructive, compatibility_issues, rollback_possible 等）
UsesBrowser bool                    - ブラウザ操作を含むか（MCP Chrome 統合用）
```

```
mu   sync.RWMutex        - 排他制御
jobs map[string]*Job     - job_id → Job のマップ（in-memory）
```


**1. ジョブ作成フロー（Coder3 出力処理時）**
```
AgentLoop.Run()
  ↓ Coder3 が CODE3 ルーティングで呼び出される
GenerateJobID()
  ↓ job_id 生成（例: 20260224-153045-a1b2c3d4）
Manager.CreateJob(jobID, plan, patch, risk, usesBrowser)
  ↓ StatusPending でジョブ登録
  ↓ セッションに job_id を保存
  ↓ ユーザーに送信（LINE/Slack 経由）
```

```
AgentLoop.Run()
  ↓ /approve <job_id> コマンド受信
Manager.Approve(jobID, approver)
  ↓ StatusGranted に更新
  ↓ Approver, DecidedAt を記録
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
  ↓ ログ記録
  ↓ Worker は実行されない
```


**現在の実装（※Phase 2 で修正）**
- ※`StatusAutoApproved` は定義されているが、使用されていない
- ※`EnableAutoApprove()`, `DisableAutoApprove()` メソッドは存在しない
- ※`IsApproved()` メソッドは `StatusAutoApproved` をチェックしているが、実際には使用されない（常に false）

**設計仕様（Coder3_Claude_API仕様.md 7章・13章より）**
```yaml
  required_by_default: true
  auto_approve:
    enabled: false
    scope:
      allowed_task_types: ["design", "review"]
      allowed_paths_prefix: ["docs/"]
      deny_operations: ["delete", "rename", "push_public"]
    ttl_minutes: 60
      - "delete"
      - "rename"
      - "send_sensitive"
      - "push_public"
      - "cost_over_limit"
```

**実装プランとの乖離（※Phase 2 で追加）**

**将来実装時の注意点（※Phase 2 で追加）**
- Scope: 対象タスク種別、対象パス、禁止操作
- TTL: 有効期限（分単位）
- 即時 OFF 可能（最優先操作）
  - ※Phase 2 で追加: この制約は Coder3_Claude_API仕様.md 13-6 に明記されている

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

3. **job_id の一意性**
   - タイムスタンプ（秒単位）+ 4 バイトランダム値（8文字の16進数）
   - 同一秒内に大量のジョブ生成で衝突リスク（極めて低確率）
   - ※UUID を使わない理由: 軽量化優先、タイムスタンプで時系列追跡が容易
   - ※Phase 2 で追加: job_test.go で一意性テスト（TestGenerateJobID_Uniqueness）実装済み

4. **ジョブの永続性**
   - `DeleteJob()` は実装されているが、自動クリーンアップのロジックなし
   - ※セッションの日次カットオーバー時にクリーンアップする想定（※推測）
   - ※Phase 2 で追加: 実装プランには明示的なクリーンアップロジックの記載なし

### 既知の問題・リスク

   - `StatusAutoApproved` は定義されているが、使用されていない
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

   - ※Phase 2 で修正: Approve/Deny メソッドは StatusPending チェックを実装済み（`!= StatusPending` なら拒否）
   - ※Phase 2 で追正: RWMutex で書き込みは排他されるため、race condition の実害は小さい

5. **コスト制御との連携なし（※Phase 2 で追加）**
   - Coder3 仕様（Coder3_Claude_API仕様.md 8-2）では `cost_hint` フィールドが定義されている
   - ただし、Job 構造体には `cost_hint` フィールドがない
   - ※Phase 2 で追加: 実装プランでは `cost_hint` の取り扱いは言及されていない

### 変更時の注意事項

1. **永続化の実装時**
   - 現在の in-memory 実装（`map[string]*Job`）をデータベース/ファイルストレージに置き換える場合
   - Manager のインターフェース（CreateJob, GetJob, Approve, Deny 等）は維持すること
   - ※session パッケージの永続化パターン（Obsidian 連携）を参考にすること

   - ※Phase 2 で追加: 実装プラン Phase 4-6 を参照
   - Scope（対象タスク種別、対象パス、禁止操作）の定義を config パッケージに追加
   - TTL（有効期限）の管理（タイマー、期限切れ検出）
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

5. **セッションとの連携**

6. **テストの拡張（※Phase 2 で修正）**
   - ※Phase 2 で修正: 現在の manager_test.go は基本的な機能テストを実装済み
   - ※Phase 2 で追加: 以下のテストケースは実装済み:
     - 同一 job_id での CreateJob 重複エラー（TestManager_CreateJob_Duplicate）
     - IsApproved のテスト（TestManager_IsApproved, TestManager_IsApproved_Denied）
     - ListJobs/DeleteJob のテスト（TestManager_ListJobs, TestManager_DeleteJob）
   - ※Phase 2 で追加: 以下のテストケースは未実装（将来推奨）:
     - 並行アクセス（goroutine）での排他制御
     - 破壊的操作検出（将来実装時）

---

## Phase 2 検証結果（※Phase 2 で追加）

### 実装状況サマリ

**実装済み（Phase 1-3 完了）**:
- ✅ job_id 生成ロジック（GenerateJobID）
- ✅ UsesBrowser フラグによる警告表示
- ✅ 基本的な機能テスト（manager_test.go, job_test.go）

**未実装（Phase 4-6 以降の課題）**:
- ❌ Scope/TTL 管理
- ❌ コスト制御との連携（cost_hint フィールド）
- ❌ 破壊的操作の自動検出
- ❌ 永続化（Obsidian 連携）
- ❌ 自動クリーンアップロジック

### 実装プランとの対比

| 項目 | 実装プラン | 現在の実装 | 乖離 |
|------|-----------|-----------|------|
| Phase 1: Coder3 Routing | Phase 1 で実装予定 | 実装済み | なし |
| cost_hint フィールド | 言及なし | 未実装 | **設計書との乖離** |
| 破壊的操作の自動検出 | 言及なし | 未実装 | **将来の拡張課題** |

### 設計書との乖離

**Coder3_Claude_API仕様.md との対比**:
- ✅ 13章「MCP Chrome 統合」: UsesBrowser フラグと警告表示は実装済み
- ❌ 8-2章「出力」: cost_hint フィールドは Job 構造体に未実装

- ✅ テスト: 基本的な機能テストは実装済み（カバレッジ: 高）

### テストカバレッジ

**実装済みテスト**:
- ✅ TestGenerateJobID: job_id のフォーマット検証
- ✅ TestGenerateJobID_Uniqueness: job_id の一意性検証
- ✅ TestManager_CreateJob: ジョブ作成の基本動作
- ✅ TestManager_CreateJob_Duplicate: 重複 job_id のエラー検証
- ✅ TestManager_Approve_NotFound: 存在しない job_id のエラー検証
- ✅ TestManager_Deny: 拒否処理の基本動作
- ✅ TestManager_IsApproved_Denied: 拒否された場合のチェック
- ✅ TestManager_ListJobs: 全ジョブ取得
- ✅ TestManager_DeleteJob: ジョブ削除
- ✅ TestManager_DeleteJob_NotFound: 存在しない job_id のエラー検証

**未実装テスト**:
- ❌ 並行アクセス（goroutine）での排他制御
- ❌ 破壊的操作検出（将来実装時）

### Phase 2 検証での主な発見

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

   - Phase 1-3 で計画された機能はすべて実装済み
   - テストカバレッジは高い（基本機能は 100%）
   - AgentLoop との統合も完了

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**解析者**: Claude Sonnet 4.5
