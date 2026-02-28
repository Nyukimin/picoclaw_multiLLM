# 承認フロー廃止プロジェクト - 進捗状況

**最終更新**: 2026-02-28
**ステータス**: 設計完了、実装準備完了

---

## 完了したタスク

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

### ステップ4: ドキュメント更新
- `docs/01_正本仕様/実装仕様.md` を更新
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
