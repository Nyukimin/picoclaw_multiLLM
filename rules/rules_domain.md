# rules_domain.md - PicoClaw ドメイン固有ルール

**作成日**: 2026-02-24
**プロジェクト名**: PicoClaw (picoclaw_multiLLM)
**目的**: PicoClaw プロジェクト固有の技術的詳細・実装パターン

---

## 1. Go 言語固有のベストプラクティス

### 1.1 エラーハンドリングパターン

#### 1.1.1 基本原則

```go
// ✅ Good: エラーに文脈を追加
func processFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("failed to read file %s: %w", path, err)
    }
    // ...
}

// ❌ Bad: エラーをそのまま返す
func processFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err  // 文脈がない
    }
    // ...
}
```

#### 1.1.2 カスタムエラー型

**定義場所**: 各パッケージの `errors.go`

```go
// pkg/worker/errors.go
package worker

import "errors"

var (
    ErrPatchParseFailed = errors.New("patch parse failed")
    ErrCommandFailed    = errors.New("command execution failed")
    ErrInvalidAction    = errors.New("invalid action type")
    ErrProtectedFile    = errors.New("protected file modification")
)
```

**使用例**:
```go
func (m *Manager) GetJob(jobID string) (*Job, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    job, ok := m.jobs[jobID]
    if !ok {
        return nil, fmt.Errorf("%w: %s", ErrJobNotFound, jobID)
    }
    return job, nil
}
```

### 1.2 並行処理パターン

#### 1.2.1 sync.Mutex の使用

```go
// ✅ Good: defer で Unlock を確実に呼ぶ
func (m *Manager) CreateJob(jobID string, ...) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if _, exists := m.jobs[jobID]; exists {
        return ErrJobExists
    }
    m.jobs[jobID] = &Job{...}
    return nil
}

// ❌ Bad: Unlock 漏れのリスク
func (m *Manager) CreateJob(jobID string, ...) error {
    m.mu.Lock()

    if _, exists := m.jobs[jobID]; exists {
        m.mu.Unlock()  // 複数箇所に Unlock
        return ErrJobExists
    }
    m.jobs[jobID] = &Job{...}
    m.mu.Unlock()  // 複数箇所に Unlock
    return nil
}
```

#### 1.2.2 読み取り専用の場合は RLock

```go
// ✅ Good: 読み取り専用は RLock
func (m *Manager) GetJob(jobID string) (*Job, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    job, ok := m.jobs[jobID]
    if !ok {
        return nil, ErrJobNotFound
    }
    return job, nil
}
```

### 1.3 構造体設計パターン

#### 1.3.1 コンストラクタパターン

```go
// ✅ Good: New* 関数でゼロ値を避ける
type Manager struct {
    mu   sync.RWMutex
    jobs map[string]*Job
}

func NewManager() *Manager {
    return &Manager{
        jobs: make(map[string]*Job),
    }
}

// ❌ Bad: ゼロ値で使うと panic
var mgr Manager  // mgr.jobs == nil → panic on access
```

#### 1.3.2 オプショナルパラメータパターン

```go
// ✅ Good: functional options パターン
type ProviderOption func(*Provider)

func WithTimeout(timeout time.Duration) ProviderOption {
    return func(p *Provider) {
        p.timeout = timeout
    }
}

func NewProvider(baseURL string, opts ...ProviderOption) *Provider {
    p := &Provider{
        baseURL: baseURL,
        timeout: 30 * time.Second,  // デフォルト
    }
    for _, opt := range opts {
        opt(p)
    }
    return p
}

// 使用例
provider := NewProvider("http://localhost:11434",
    WithTimeout(60 * time.Second),
)
```

### 1.4 命名規則の詳細

#### 1.4.1 パッケージ名

```go
// ✅ Good: 小文字、単数形
package agent
package worker
package session

// ❌ Bad: 複数形、大文字、アンダースコア
package agents
package Worker
package session_manager
```

#### 1.4.2 インターフェース名

```go
// ✅ Good: -er サフィックス
type Provider interface {
    SendMessage(ctx context.Context, req *Request) (*Response, error)
}

type Checker interface {
    Check() (bool, string)
}

// ❌ Bad: Interface サフィックス
type ProviderInterface interface {
    // ...
}
```

---

## 2. LLM プロバイダー統合の詳細

### 2.1 LLMProvider インターフェース

**定義**: `pkg/providers/provider.go`

```go
type LLMProvider interface {
    SendMessage(ctx context.Context, req *Request) (*Response, error)
    Name() string
}
```

### 2.2 プロバイダー実装一覧

| プロバイダー | ファイル | 用途 | 認証方式 |
|------------|---------|------|---------|
| Ollama | `ollama_provider.go` | Chat/Worker（ローカル） | なし（localhost） |
| DeepSeek | `deepseek_provider.go` | Coder1（仕様設計） | API キー |
| OpenAI | `openai_provider.go` | Coder2（実装） | API キー |
| Claude | `claude_provider.go` | Coder3（高品質） | API キー |

### 2.3 共通パターン

#### 2.3.1 API キーの取得

```go
// ✅ Good: 環境変数から取得、エラーハンドリング
func NewClaudeProvider(cfg *config.Config) (*ClaudeProvider, error) {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
    }

    return &ClaudeProvider{
        baseURL: "https://api.anthropic.com",
        apiKey:  apiKey,
        model:   cfg.RouteLLM.Coder3Model,
    }, nil
}
```

#### 2.3.2 タイムアウト設定

```go
// ✅ Good: context でタイムアウト制御
func (p *ClaudeProvider) SendMessage(ctx context.Context, req *Request) (*Response, error) {
    ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()

    // HTTP リクエスト
    httpReq, err := http.NewRequestWithContext(ctx, "POST", url, body)
    // ...
}
```

#### 2.3.3 Ollama の keep_alive 設定

```go
// ✅ Good: keep_alive: -1 で常駐化
func (p *OllamaProvider) buildRequest(req *Request) map[string]interface{} {
    return map[string]interface{}{
        "model":      p.model,
        "messages":   req.Messages,
        "keep_alive": -1,  // 常駐化（重要）
        "options": map[string]interface{}{
            "num_ctx": 8192,
        },
    }
}
```

---

## 3. ルーティングロジックの実装詳細

### 3.1 ルーティング決定フロー

**実装**: `pkg/agent/router.go`

```
入力メッセージ
    ↓
1. 明示コマンド解析（parseRouteCommand）
    ↓ なし
2. ルール辞書マッチング（matchRuleDict）
    ↓ マッチなし
3. 分類器呼び出し（classifyRoute）
    ↓ 失敗 or 低信頼度
4. フォールバック（CHAT）
```

### 3.2 明示コマンド解析

**実装**: `pkg/agent/router.go` の `parseRouteCommand()`

```go
func parseRouteCommand(text string) string {
    text = strings.TrimSpace(strings.ToLower(text))

    if strings.HasPrefix(text, "/code3") {
        return RouteCode3
    }
    if strings.HasPrefix(text, "/code2") {
        return RouteCode2
    }
    if strings.HasPrefix(text, "/code1") {
        return RouteCode1
    }
    if strings.HasPrefix(text, "/code") {
        return RouteCode
    }
    if strings.HasPrefix(text, "/approve") {
        return RouteApprove
    }
    if strings.HasPrefix(text, "/deny") {
        return RouteDeny
    }
    // ... 他のコマンド

    return ""  // コマンドなし
}
```

### 3.3 Coder ルート選択ロジック

**実装**: `pkg/agent/loop.go` の `selectCoderRoute()`

```go
func selectCoderRoute(msg string) string {
    msg = strings.ToLower(msg)

    // CODE1: 仕様設計向け
    code1Keywords := []string{"仕様", "設計", "architecture", "spec"}
    for _, kw := range code1Keywords {
        if strings.Contains(msg, kw) {
            return RouteCode1
        }
    }

    // CODE2: 実装向け
    code2Keywords := []string{"実装", "implement", "バグ修正", "fix"}
    for _, kw := range code2Keywords {
        if strings.Contains(msg, kw) {
            return RouteCode2
        }
    }

    // CODE3: 高品質コーディング/推論向け
    code3Keywords := []string{"高品質", "仕様策定", "複雑な推論", "重大バグ", "失敗コスト"}
    for _, kw := range code3Keywords {
        if strings.Contains(msg, kw) {
            return RouteCode3
        }
    }

    // デフォルト: CODE
    return RouteCode
}
```

**注意**:
- キーワードマッチングは初期実装として簡易的
- 将来的には分類器（LLM）による判定に移行可能
- 明示的な `/code3` コマンドが最優先

---

## 4. セッション管理の実装詳細

### 5.1 SessionFlags の永続化

**実装**: `pkg/session/manager.go`

```go
type SessionFlags struct {
    LocalOnly        bool   `json:"local_only"`
    PrevPrimaryRoute string `json:"prev_primary_route"`
}

func (m *Manager) UpdateFlags(sessionID string, updater func(*SessionFlags)) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    session, exists := m.sessions[sessionID]
    if !exists {
        return fmt.Errorf("session not found: %s", sessionID)
    }

    // フラグを更新
    updater(&session.Flags)

    // 永続化（ファイルまたは DB）
    if err := m.saveSession(session); err != nil {
        return fmt.Errorf("failed to save session: %w", err)
    }

    return nil
}
```

### 5.2 日次カットオーバー

**タイミング**: JST 00:00

**実装**:
```go
func (m *Manager) CheckDailyCutover() {
    now := time.Now()
    today := now.Format("2006-01-02")

    if m.lastCutoverDate != today {
        m.performCutover()
        m.lastCutoverDate = today
    }
}

func (m *Manager) performCutover() {
    m.mu.Lock()
    defer m.mu.Unlock()

    // 古いセッションをアーカイブ
    for id, session := range m.sessions {
        if session.LastActiveAt.Before(time.Now().Add(-24 * time.Hour)) {
            m.archiveSession(session)
            delete(m.sessions, id)
        }
    }

    // メモリをリセット
    debug.FreeOSMemory()
}
```

---

## 6. ヘルスチェックの実装詳細

### 6.1 OllamaCheck の実装

**実装**: `pkg/health/checks.go`

```go
func OllamaCheck(baseURL string, timeout time.Duration) func() (bool, string) {
    return func() (bool, string) {
        ctx, cancel := context.WithTimeout(context.Background(), timeout)
        defer cancel()

        url := baseURL + "/api/tags"
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            return false, fmt.Sprintf("request error: %v", err)
        }

        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            return false, fmt.Sprintf("connection failed: %v", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != 200 {
            return false, fmt.Sprintf("status %d", resp.StatusCode)
        }

        return true, "Ollama OK"
    }
}
```

### 6.2 OllamaModelsCheck の実装

**実装**: `pkg/health/checks.go`

```go
type ModelRequirement struct {
    Name       string
    MinContext int  // 0 でなければ、これ未満は NG
    MaxContext int  // 0 でなければ、これを超えると NG（8192 推奨）
}

func OllamaModelsCheck(baseURL string, timeout time.Duration, required []ModelRequirement) func() (bool, string) {
    return func() (bool, string) {
        // /api/ps エンドポイントでロード済みモデルを確認
        url := baseURL + "/api/ps"

        // ... HTTP リクエスト ...

        // MaxContext チェック
        for _, req := range required {
            if req.MaxContext > 0 && loadedModel.ContextLength > req.MaxContext {
                return false, fmt.Sprintf("%s context_length %d exceeds max %d",
                    req.Name, loadedModel.ContextLength, req.MaxContext)
            }
        }

        return true, "All models OK"
    }
}
```

### 6.3 ヘルスチェックの統合

**実装**: `pkg/agent/loop.go`

```go
func (s *AgentLoop) checkHealth() error {
    // Ollama プロセス確認
    if ok, msg := s.ollamaCheck(); !ok {
        // 再起動試行
        if err := s.restartOllama(); err != nil {
            return fmt.Errorf("ollama restart failed: %w", err)
        }
    }

    // モデルロード確認
    if ok, msg := s.modelsCheck(); !ok {
        return fmt.Errorf("models check failed: %s", msg)
    }

    return nil
}
```

---

## 7. ログ実装の詳細

### 7.1 ログフォーマット

**形式**: 構造化ログ（JSON）

```json
{
  "timestamp": "2026-02-24T15:30:45+09:00",
  "level": "info",
  "event": "worker.executed",
  "job_id": "job_1709123456_abc12345",
  "session_id": "20260224-LINE-U123456789",
  "execution_status": "success",
  "executed_commands": 3,
  "failed_commands": 0,
  "git_commit_hash": "a1b2c3d4"
}
```

### 7.2 ログ記録関数

**実装**: `pkg/logging/logger.go`

```go
func LogWorkerExecuted(jobID, sessionID string, executedCmds, failedCmds int, gitCommit string) {
    log.Printf("[worker.executed] job_id=%s session_id=%s executed=%d failed=%d git_commit=%s",
        jobID, sessionID, executedCmds, failedCmds, gitCommit)
}

func LogWorkerSuccess(jobID, sessionID, summary string) {
    log.Printf("[worker.success] job_id=%s session_id=%s summary=%s", jobID, sessionID, summary)
}

func LogWorkerFail(jobID, sessionID, errorMsg string) {
    log.Printf("[worker.fail] job_id=%s session_id=%s error=%s", jobID, sessionID, errorMsg)
}
```

### 7.3 Obsidian 連携

**保存場所**: `logs/obsidian/YYYY-MM-DD.md`

**フォーマット**:
```markdown
## 15:30:45 - worker.executed

- Job ID: job_1709123456_abc12345
- Session ID: 20260224-LINE-U123456789
- Status: success
- Executed: 3 commands
- Failed: 0 commands
- Git Commit: a1b2c3d4

---
```

---

## 8. テストパターンと TDD

### 8.1 TDD サイクル（必須）

すべての新機能追加・バグ修正は TDD サイクルに従う。

#### 8.1.1 Red-Green-Refactor サイクル

```
1. Red（失敗するテストを書く）
   ↓
2. Green（テストを通す最小実装）
   ↓
3. Refactor（リファクタリング）
   ↓
4. LINT チェック
   ↓
   繰り返し
```

#### 8.1.2 Red フェーズの例

```go
// pkg/approval/manager_test.go
func TestManager_CreateJob_Duplicate(t *testing.T) {
    // Arrange
    mgr := NewManager()
    jobID := "test-job-001"

    // 最初のジョブ作成は成功
    err := mgr.CreateJob(jobID, "plan", "patch", nil)
    if err != nil {
        t.Fatalf("First CreateJob should succeed: %v", err)
    }

    // Act: 同じ job_id で再度作成を試みる
    err = mgr.CreateJob(jobID, "plan2", "patch2", nil)

    // Assert: エラーが返されることを期待（Red）
    if err == nil {
        t.Error("Expected error for duplicate job_id, got nil")
    }
    if !errors.Is(err, ErrJobExists) {
        t.Errorf("Expected ErrJobExists, got %v", err)
    }
}
```

#### 8.1.3 Green フェーズの例

```go
// pkg/approval/manager.go
func (m *Manager) CreateJob(jobID, plan, patch string, risk map[string]interface{}) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // 重複チェックを追加（Green: テストを通す）
    if _, exists := m.jobs[jobID]; exists {
        return fmt.Errorf("%w: %s", ErrJobExists, jobID)
    }

    m.jobs[jobID] = &Job{
        JobID:       jobID,
        Status:      StatusPending,
        Plan:        plan,
        Patch:       patch,
        Risk:        risk,
        RequestedAt: time.Now().Format(time.RFC3339),
    }
    return nil
}
```

#### 8.1.4 Refactor フェーズ

- コードの重複を排除
- 変数名・関数名を改善
- 構造を最適化
- **LINT チェック実行**（必須）

```bash
# Refactor 後に必ず実行
golangci-lint run ./pkg/worker/...
go test ./pkg/worker/... -v
```

### 8.2 ユニットテストの構造

**ファイル命名**: `*_test.go`

**例**: `pkg/worker/executor_test.go`

```go
package worker

import (
    "testing"
)

func TestManager_CreateJob(t *testing.T) {
    // Arrange
    mgr := NewManager()
    jobID := "test-job-001"
    plan := "test plan"
    patch := "test patch"
    risk := map[string]interface{}{"destructive": false}

    // Act
    err := mgr.CreateJob(jobID, plan, patch, risk)

    // Assert
    if err != nil {
        t.Fatalf("CreateJob failed: %v", err)
    }

    job, err := mgr.GetJob(jobID)
    if err != nil {
        t.Fatalf("GetJob failed: %v", err)
    }

    if job.Status != StatusPending {
        t.Errorf("Expected status=pending, got %s", job.Status)
    }
}
```

### 8.3 テーブル駆動テスト

```go
func TestGenerateJobID(t *testing.T) {
    tests := []struct {
        name string
        want string  // 正規表現パターン
    }{
        {
            name: "valid format",
            want: `^\d{8}-\d{6}-[a-f0-9]{8}$`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            jobID := GenerateJobID()

            matched, _ := regexp.MatchString(tt.want, jobID)
            if !matched {
                t.Errorf("JobID format mismatch: got %s, want pattern %s", jobID, tt.want)
            }
        })
    }
}
```

### 8.4 モックの使用

**httptest の使用例**:

```go
func TestOllamaCheck(t *testing.T) {
    // モックサーバー起動
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"models":[]}`))
    }))
    defer server.Close()

    // ヘルスチェック実行
    checkFn := OllamaCheck(server.URL, 5*time.Second)
    ok, msg := checkFn()

    if !ok {
        t.Errorf("Expected ok=true, got ok=false, msg=%s", msg)
    }
}
```

---

### 8.5 テストカバレッジの測定

```bash
# カバレッジ測定
go test ./pkg/worker/... -coverprofile=coverage.out

# HTML レポート生成
go tool cover -html=coverage.out

# カバレッジ率を表示
go test ./pkg/worker/... -cover
```

**目標カバレッジ**:
- 重要パッケージ（`agent`, `worker`, `session`）: 80% 以上
- その他のパッケージ: 70% 以上

---

## 9. LINT チェック（必須）

### 9.1 golangci-lint のセットアップ

#### 9.1.1 インストール

```bash
# 最新版をインストール
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# バージョン確認
golangci-lint version
```

#### 9.1.2 設定ファイル

**`.golangci.yml`** をプロジェクトルートに配置:

```yaml
linters:
  enable:
    - errcheck      # エラーチェック漏れ
    - govet         # 疑わしいコード構造
    - staticcheck   # 静的解析
    - gosimple      # コード簡素化提案
    - unused        # 未使用コード検出
    - ineffassign   # 無駄な代入
    - gofmt         # フォーマット確認
    - goimports     # import 整理
    - goconst       # 定数化可能な文字列
    - misspell      # スペルチェック
    - unconvert     # 不要な型変換
    - unparam       # 未使用パラメータ

linters-settings:
  errcheck:
    check-blank: true
    check-type-assertions: true

  govet:
    check-shadowing: true

  staticcheck:
    checks: ["all"]

run:
  timeout: 5m
  tests: true
  skip-dirs:
    - vendor
    - .serena

issues:
  exclude-use-default: false
  max-issues-per-linter: 0
  max-same-issues: 0
```

### 9.2 LINT チェックの実行

#### 9.2.1 基本実行

```bash
# すべてのパッケージをチェック
golangci-lint run

# 特定パッケージのみ
golangci-lint run ./pkg/worker/...

# 自動修正（可能な場合）
golangci-lint run --fix
```

#### 9.2.2 CI/CD での実行

```bash
# エラーがある場合は失敗させる
golangci-lint run --max-issues-per-linter=0 --max-same-issues=0
```

### 9.3 LINT エラーの対処

#### 9.3.1 優先度

1. **エラー**: 必ず修正（コミット禁止）
2. **警告**: 可能な限り修正
3. **情報**: 確認して判断

#### 9.3.2 例外的に無視する場合

**nolint ディレクティブ**（最小限に使用）:

```go
// 特定の Linter のみ無視
// nolint:errcheck // 理由: この戻り値は常に nil
_ = file.Close()

// 複数の Linter を無視
// nolint:errcheck,gosec
password := os.Getenv("PASSWORD")

// 1 行全体を無視
//nolint
func legacyFunction() { /* ... */ }
```

**重要**: nolint を使う場合は**必ずコメントで理由を記載**する。

#### 9.3.3 よくある LINT エラーと対処

| エラー | 原因 | 対処 |
|--------|------|------|
| `Error return value not checked` | エラーチェック漏れ | `if err != nil` で適切にハンドリング |
| `Unused variable` | 未使用変数 | 削除するか `_` で明示的に無視 |
| `Should have comment` | 公開関数のコメント不足 | godoc 形式でコメント追加 |
| `Cognitive complexity too high` | 関数が複雑すぎる | 小さな関数に分割 |
| `Magic number` | マジックナンバー | 定数として定義 |

### 9.4 コミット前チェック（必須手順）

```bash
# 1. LINT チェック
golangci-lint run
# → エラーがある場合はすべて修正

# 2. テスト実行
go test ./pkg/... -v
# → すべてのテストが通ることを確認

# 3. カバレッジ確認
go test ./pkg/... -cover
# → 目標カバレッジを満たしているか確認

# 4. すべて OK ならコミット
git add .
git commit -m "feat: 機能追加"
```

---

## 10. パフォーマンス最適化

### 10.1 メモリ使用量の監視

```go
import "runtime"

func logMemoryUsage() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)

    log.Printf("Memory: Alloc=%d MB, TotalAlloc=%d MB, Sys=%d MB",
        m.Alloc/1024/1024,
        m.TotalAlloc/1024/1024,
        m.Sys/1024/1024)
}
```

### 10.2 ガベージコレクション制御

```go
import "runtime/debug"

// 日次カットオーバー時にメモリ解放
func (m *Manager) performCutover() {
    // セッションクリーンアップ
    // ...

    // メモリを OS に返却
    debug.FreeOSMemory()
}
```

### 10.3 goroutine リーク防止

```go
// ✅ Good: context でキャンセル可能
func (p *Provider) SendMessage(ctx context.Context, req *Request) (*Response, error) {
    ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()

    done := make(chan *Response)
    errCh := make(chan error)

    go func() {
        resp, err := p.doRequest(req)
        if err != nil {
            errCh <- err
            return
        }
        done <- resp
    }()

    select {
    case resp := <-done:
        return resp, nil
    case err := <-errCh:
        return nil, err
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

---

## 11. デバッグとトラブルシューティング

### 11.1 よくある問題

#### 11.1.1 Ollama 接続エラー

**症状**: `connection failed: dial tcp [::1]:11434: connect: connection refused`

**原因**:
- Ollama プロセスが起動していない
- ポート番号が違う
- ファイアウォールでブロック

**解決**:
```bash
# Ollama 起動確認
systemctl --user status ollama

# 再起動
systemctl --user restart ollama

# ポート確認
curl http://localhost:11434/api/tags
```

#### 11.1.2 MaxContext 超過エラー

**症状**: `context_length 131072 exceeds max 8192`

**原因**: モデルの context_length が大きすぎる

**解決**:
```bash
# モデルを再作成（Modelfile で num_ctx を指定）
cat > Modelfile <<EOF
FROM mistral:latest
PARAMETER num_ctx 8192
EOF

ollama create chat-v1:latest -f Modelfile
```

#### 11.1.3 job_id not found エラー

**症状**: `job test-job-001 not found`

**原因**:
- セッション再起動で in-memory ジョブが消失
- job_id のタイプミス

**解決**:
- 永続化の実装（Phase 4-5 で対応予定）
- job_id のコピー&ペースト推奨

---

**最終更新**: 2026-02-24
**バージョン**: 1.0
**メンテナンス**: ルールを変更した場合は、このファイルと `PROJECT_AGENT.md` の両方を更新してください。
