# 承認フロー廃止プロジェクト - 進捗状況

**最終更新**: 2026-03-01
**ステータス**: ✅ v3.0実装仕様 100%完成、実装即開始可能

---

## 完了したタスク

### Task #16: v3.0実装仕様を承認フロー廃止後の設計に修正 ✓
**完了日**: 2026-03-01
**コミット**: `9d6a5ad` (proposal/clean-architecture)
**ファイル**: `docs/01_正本仕様/実装仕様_v3_クリーンアーキテクチャ版.md`

**主要変更**:
1. **削除**（承認フロー関連）:
   - ApprovalFlow アグリゲート（4.7章）
   - AutoApprovePolicy（4.8章）
   - Event Sourcing for approval tracking（10章）
   - ApprovalService（17.3章）
   - データベーススキーマ（20章: events/jobs/auto_approve_policies）

2. **追加**（Worker即時実行）:
   - Worker即時実行仕様（9章）
   - PatchCommand 値オブジェクト（4.7章）
   - WorkerExecutionService 完全実装（17.3章）
     - executeFileEdit: 7種のAction（create/update/delete/append/rename/mkdir/copy）
     - executeShellCommand: Env/WorkDir/Shell対応
     - executeGitOperation: 4種のGit操作
     - parsePatch/parseJSONPatch/parseMarkdownPatch: 完全実装
   - セーフガード設計（1.2章）
     - Git auto-commit（実行前後、失敗時は中断）
     - 保護ファイルパターン（.env*, *credentials*, *.key, *.pem）
     - 実行前サマリ表示
     - ワークスペース制限
     - ドライモード

3. **修正**（フロー整合性）:
   - MessageOrchestrator: ApprovalService削除、Worker即時実行追加
   - Package構成: approval/削除、patch/追加
   - テスト観点: 承認テスト削除、Worker即時実行テスト追加
   - 実装プラン: Phase 1-4をWorker即時実行中心に再構成

**技術詳細**:
- PatchCommand拡張: mkdir/rename/copy + Env/WorkDir/Shell
- parseJSONPatch: JSON配列バリデーション、必須フィールドチェック
- parseMarkdownPatch: 正規表現パターンマッチ（```言語:ファイルパス\n内容\n```）
- エラーハンドリング: 継続モード（デフォルト）/中断モード（StopOnError=true）

**実装完全性**:
- WorkerExecutionService: 100%コーディング可能（全メソッド実装詳細あり）
- セーフガード: 6種類実装（必須3、推奨2、オプション1）
- PatchCommand: 全Action対応（file_edit 7種、shell_command、git_operation 4種）

**変更量**: 778行追加、786行削除（実質-8行、構造改善による最適化）

### Phase 4: 実装詳細補完とエラーハンドリング ✓
**完了日**: 2026-03-01
**コミット**: `1635afb` (proposal/clean-architecture)

**主要変更**:
1. **未定義関数実装** (Category C):
   - parseExplicitCommand: 9種の明示コマンド解析
   - buildMessages/buildRequest: LLMリクエスト構築
   - extractProposalFromResponse: Coderルート用Proposal抽出
   - extract関数群: Plan/Patch/Risk/CostHint セクション抽出

2. **レイヤー分離修正** (Category D):
   - ParsePatch を Domain 層に移動 (section 4.9)
   - Application 層の重複実装削除 (parsePatch/parseJSONPatch/parseMarkdownPatch)

3. **欠落実装詳細** (Category F):
   - Session エンティティ追加 (section 4.10)
     - NewSession/AddTask/GetHistory/SetMemory/GetMemory/ClearMemory
     - SessionRepository インターフェース
     - 日次カットオーバー対応（セッションIDフォーマット: `YYYYMMDD-{channel}-{chatID}`）
   - LLMIteratorService/MemoryService: 既存実装確認（完全実装済み）

4. **エラーハンドリング詳細** (Category G):
   - Worker 失敗時のロールバック処理
     - Git auto-commit ON: `git reset --hard HEAD~1` でロールバック
     - Git auto-commit OFF: 手動リカバリ促進メッセージ
     - 詳細ログ記録（failed_command, error, rollback_commit）
   - 分類器エラー時の詳細ログ
     - エラー型 (`fmt.Sprintf("%T", err)`)
     - メッセージ先頭100文字
     - フォールバック先ルート（CHAT固定）
   - MCP 部分失敗時のロールバック
     - 継続モード: 失敗スキップ、残り実行、成功数/失敗数記録
     - 中断モード: 即座に中断、Git ロールバック
     - 部分成功カウントログ記録

**変更量**: 200行追加、107行削除（実質+93行）

**実装準備完了度**: 100%
- ✅ 全インターフェース定義完了 (7個)
- ✅ 全データ型定義完了 (Task, JobID, Proposal, PatchCommand, etc.)
- ✅ 全関数シグネチャ定義完了
- ✅ エラーハンドリング詳細仕様完備
- ✅ セッション管理仕様完備
- ✅ 実装可能状態達成

---

## 以前完了したタスク

### Task #13: 仕様変更ドキュメント作成 ✓
**ファイル**: `docs/06_実装ガイド進行管理/20260228_承認フロー廃止プラン.md`

- PicoClaw の基本原則「完全自動」に基づき、承認フロー（Phase 1-3 実装済み、Phase 4-6 未実装）を完全廃止
- 新フロー: ユーザー指示 → ルーティング → Coder3 が plan/patch 生成 → **Worker が即時実行** → 結果返却
- セーフガード:
  - 詳細ログ（必須）
  - Git auto-commit（推奨）
  - 定期バックアップ（推奨）
  - Dry-run モード（オプション、将来拡張）
- 破壊的操作のリスクはユーザー責任として明記
- 実装スケジュール: 2-3週間

### Task #14: 削除箇所リスト生成 ✓
**ファイル**: `docs/06_実装ガイド進行管理/20260228_承認フロー削除箇所リスト.md`

**削除対象（合計 ~590行）**:
1. `pkg/approval/` ディレクトリ全体
   - job.go (19行)
   - manager.go (151行)
   - message.go (36行)
   - job_test.go (~100行)
   - manager_test.go (~300行)

2. `pkg/agent/loop.go`:
   - L25: import "github.com/sipeed/picoclaw/pkg/approval"
   - L59: approvalMgr フィールド
   - L224: approvalMgr 初期化
   - L497-L520: Coder3 承認処理ブロック（24行）
   - L1917-L1925: /approve コマンドハンドラ（9行）
   - L1940-L1947: /deny コマンドハンドラ（8行）
   - L1960: NeedApproval フィールド

3. `pkg/agent/router.go`:
   - L22-L23: RouteApprove/RouteDeny 定数
   - L237-L240: /approve, /deny コマンド解析

4. `pkg/session/manager.go`:
   - L31: PendingApprovalJobID フィールド

5. `pkg/logger/logger.go`:
   - L241-L284: LogApproval* 関数（44行）

削除検証チェックリスト付き。

### Task #15: Worker 実行ロジック設計 ✓
**ファイル**: `docs/06_実装ガイド進行管理/20260228_Worker即時実行ロジック設計.md`

**設計概要**:

#### データ構造
```go
type PatchCommand struct {
    Type    string            // "file_edit", "shell_command", "git_operation"
    Action  string            // "create", "update", "delete", "run", "add", "commit"
    Target  string            // ファイルパスまたはコマンド
    Content string            // ファイル内容またはコマンド引数
    Metadata map[string]string
}

type PatchExecutionResult struct {
    Success      bool
    ExecutedCmds int
    FailedCmds   int
    Results      []CommandResult
    Summary      string
    GitCommit    string // auto-commit時のハッシュ
}
```

#### 主要関数
1. `parsePatch(patch string) ([]PatchCommand, error)`
   - JSON配列またはMarkdownコードブロックをパース
   - サポート: ```go:<filepath>```, ```bash```, ```git```

2. `executeWorkerPatch(ctx, patch, sessionKey) (*PatchExecutionResult, error)`
   - エントリーポイント
   - パース → 順次実行 → ログ記録 → auto-commit（オプション）

3. `executeCommand(ctx, cmd) (string, error)`
   - Type別ディスパッチ: file_edit / shell_command / git_operation

4. `executeFileEdit(ctx, cmd) (string, error)`
   - create, update, delete, append
   - workspace外書き込み禁止

5. `executeShellCommand(ctx, cmd) (string, error)`
   - bash経由実行、タイムアウト5分

6. `executeGitOperation(ctx, cmd) (string, error)`
   - add, commit, reset, checkout

7. `autoCommitChanges(ctx, patch) (string, error)`
   - git add -A → git commit -m "[Worker Auto-Commit] ..."

#### 設定追加（pkg/config/config.go）
```go
type WorkerConfig struct {
    AutoCommit         bool   // デフォルト: false
    CommitMessagePrefix string // デフォルト: "[Worker Auto-Commit]"
    CommandTimeout     int    // 秒、デフォルト: 300
    GitTimeout         int    // 秒、デフォルト: 30
    StopOnError        bool   // デフォルト: false（継続モード）
}
```

#### エラーハンドリング戦略
- **継続モード**（デフォルト）: 1つ失敗しても残りを実行
- **中断モード**（オプション）: 最初の失敗で即中断

#### 実装フェーズ
1. 基本構造（PatchCommand, parsePatch JSON版, executeFileEdit）
2. コマンド拡張（executeShell, executeGit, parsePatch Markdown版）
3. auto-commit とエラーハンドリング
4. 統合とテスト

---

## 次のステップ（未実施）

### ステップ1: 削除実行
`docs/06_実装ガイド進行管理/20260228_承認フロー削除箇所リスト.md` に従って削除:
1. `rm -rf pkg/approval/`
2. loop.go, router.go, session/manager.go, logger/logger.go から該当行を削除
3. 削除検証チェックリスト確認
4. `go build ./cmd/picoclaw` で確認

### ステップ2: Worker実行ロジック実装
`docs/06_実装ガイド進行管理/20260228_Worker即時実行ロジック設計.md` に従って実装:
1. フェーズ1: 基本構造
2. フェーズ2: コマンド拡張
3. フェーズ3: auto-commit
4. フェーズ4: 統合テスト

### ステップ3: テスト
- ユニットテスト: parsePatch, executeFileEdit, executeShell, executeGit
- 統合テスト: End-to-End patch実行シナリオ

### ステップ4: ドキュメント更新 ✓ (v3.0仕様のみ完了)
- ✅ `docs/01_正本仕様/実装仕様_v3_クリーンアーキテクチャ版.md` を更新（完了）
- ⏳ `docs/01_正本仕様/実装仕様.md` を更新（未実施、既存仕様）
- 承認フロー関連記述を削除、Worker即時実行に置き換え

---

## 重要な決定事項

1. **設計オプション選択**: Option A（即時実行、確認なし）
2. **基本原則**: PicoClaw は「完全自動」
3. **MCP操作**: 承認なし（即時実行）
4. **破壊的操作**: ユーザー責任、セーフガードで緩和
5. **エラーハンドリング**: 継続モード（デフォルト）

---

## 関連ファイル

- 仕様変更: `docs/06_実装ガイド進行管理/20260228_承認フロー廃止プラン.md`
- 削除リスト: `docs/06_実装ガイド進行管理/20260228_承認フロー削除箇所リスト.md`
- Worker設計: `docs/06_実装ガイド進行管理/20260228_Worker即時実行ロジック設計.md`
- 正本仕様: `docs/01_正本仕様/実装仕様.md`（更新必要）
