# PROJECT_AGENT.md - PicoClaw プロジェクトエージェント

**作成日**: 2026-02-24
**プロジェクト名**: PicoClaw (picoclaw_multiLLM)
**目的**: 超軽量 AI アシスタントのマルチ LLM ルーティング実装

---

## 1. プロジェクト概要

### 1.1 プロジェクト名

**PicoClaw** (picoclaw_multiLLM)

### 1.2 目的

LINE/Slack からの指示を受け、複数の LLM を適切にルーティングして実行する超軽量 AI アシスタント。
メモリ使用量 <10MB、低スペックハードウェア（$10 デバイス）で動作することを目指す。

### 1.3 主要機能

- **マルチ LLM ルーティング**: Chat/Worker/Coder1/Coder2/Coder3 の自動振り分け
- **承認フロー**: job_id ベースの追跡、破壊的操作の承認制御
- **Auto-Approve モード**: Scope/TTL 付き自動承認
- **ヘルスチェック**: Ollama 常駐監視、自動再起動
- **セッション管理**: 日次カットオーバー、メモリ管理
- **ログ保存**: 構造化ログ、Obsidian 連携

### 1.4 使用言語・プラットフォーム

- **使用言語**: Go 1.23
- **プラットフォーム**: Linux (主に)、macOS (開発環境)
- **依存 LLM**:
  - Ollama: chat-v1:latest, worker-v1:latest（ローカル常駐）
  - DeepSeek API: Coder1（仕様設計）
  - OpenAI API: Coder2（実装）
  - Anthropic Claude API: Coder3（高品質コーディング/推論）

---

## 2. プロジェクト固有の注意事項

### 2.1 実行環境

#### 2.1.1 プラットフォーム

- **主要ターゲット**: Linux (Raspberry Pi 等の低スペックデバイス)
- **開発環境**: macOS, Linux
- **非サポート**: Windows（必要な場合は個別に検証）

#### 2.1.2 メモリ制約（最重要）

- **目標**: メモリ使用量 <10MB（Go プロセス単体）
- **理由**: $10 デバイスでの動作を保証するため
- **実装上の注意**:
  - 大きなデータ構造をメモリに保持しない
  - セッションデータは必要最小限に
  - ログは外部ファイル/Obsidian に即座に書き出し
  - 日次カットオーバーでメモリリセット

#### 2.1.3 Ollama 依存（重要）

- **必須サービス**: Ollama がローカルで常駐していることが前提
- **常駐化**: `keep_alive: -1` で Chat/Worker モデルを常駐
- **ヘルスチェック**: 起動時・毎回の LLM 呼び出し前に実行
- **MaxContext 制約**: 8192 を超える context_length のモデルは NG
  - 理由: 大きすぎる num_ctx によるロード失敗を防止
  - 検証: `pkg/health/checks.go` の `ModelRequirement.MaxContext`

### 2.2 実装の一次参照（必須遵守）

#### 2.2.1 正本仕様

すべての実装判断は **`docs/01_正本仕様/実装仕様.md`** を一次参照とする。

- 仕様の曖昧さ・矛盾がある場合は、このファイルを更新してから実装する
- 実装と仕様の不整合は技術的負債として最優先で解消する
- 仕様変更時は、必ず `docs/01_正本仕様/実装仕様.md` を更新する

#### 2.2.2 補助参照

- **分割仕様**: `docs/02_v2統合分割仕様/`
- **LLM 運用**: `docs/05_LLM運用プロンプト設計/`
- **実装ガイド**: `docs/06_実装ガイド進行管理/`
- **ドメイン知識**: `rules/rules_domain.md`

### 2.3 Go 言語固有の制約

#### 2.3.1 エラーハンドリング（必須）

- すべてのエラーは適切にハンドリングする
- `err != nil` チェックを省略しない
- エラーメッセージには文脈を含める（`fmt.Errorf("context: %w", err)`）
- パニックは極力避ける（不可避な場合は recover で捕捉）

#### 2.3.2 並行処理

- goroutine は必要最小限に
- channel のクローズ漏れに注意
- context.Context でキャンセル可能にする
- sync.Mutex の Unlock 漏れに注意（defer を使用）

#### 2.3.3 命名規則

- パッケージ名: 小文字、単数形（例: `agent`, `session`, `approval`）
- 公開関数: PascalCase（例: `NewManager`, `CreateJob`）
- 非公開関数: camelCase（例: `selectCoderRoute`, `parseCoder3Output`）
- 定数: PascalCase または UPPER_SNAKE_CASE（例: `RouteCode3`, `StatusPending`）

---

## 3. 責務の分離（Chat/Worker/Coder）

### 3.1 三役割モデル

| 役割 | 責務 | 実行内容 | 実装パッケージ |
|------|------|----------|----------------|
| **Chat** | 意思決定・承認管理 | ユーザー対話、ルーティング決定、承認要求送信 | `pkg/agent/loop.go` |
| **Worker** | 実行・道具係 | ファイル編集、コマンド実行、テスト実行、差分適用 | `pkg/agent/loop.go` (Worker ルート) |
| **Coder** | 設計・実装案作成 | 仕様策定、コード生成、`plan` と `patch` の生成 | `pkg/agent/loop.go` (CODE ルート) |

### 3.2 不変ルール（統治原則）

1. **Coder は破壊的操作を直接実行しない**
   - Coder の出力は `plan` と `patch` のみ
   - 実際の適用（ファイル書込み、コマンド実行）は Worker が担当

2. **実行前に承認状態を確認する**
   - Coder3 の出力には `need_approval` フラグが含まれる
   - `need_approval=true` の場合、Chat が承認要求を送信
   - 承認後に Worker が実行

3. **job_id でジョブを追跡する**
   - すべての承認ジョブに job_id を付与（`pkg/approval/job.go`）
   - ログに job_id を記録（`pkg/logging/logger.go`）
   - セッションに承認待ち job_id を保存（`pkg/session/manager.go`）

---

## 4. プロジェクト固有の禁止事項

### 4.1 実装レベルの禁止

#### 4.1.1 メモリ関連

- ❌ **大きなデータ構造をメモリに長時間保持する**
  - 例: 全ログをメモリに読み込む、全セッションを永続化
  - 代替: ストリーミング処理、必要時のみ読み込み

- ❌ **不要な goroutine を大量に起動する**
  - 例: ループ内で goroutine を無制限に起動
  - 代替: worker pool パターン、semaphore で制御

#### 4.1.2 Ollama 関連

- ❌ **MaxContext（8192）を超えるモデルをロードする**
  - 理由: 大きすぎる num_ctx によるロード失敗
  - 検証: `pkg/health/checks.go` でチェック済み

- ❌ **`keep_alive: -1` なしで Chat/Worker を呼び出す**
  - 理由: モデルのロード/アンロードによる遅延
  - 実装: `pkg/providers/ollama_provider.go` で設定

#### 4.1.3 承認フロー関連

- ❌ **Coder が破壊的操作を直接実行する**
  - 例: Coder が直接ファイルを削除・上書き
  - 正: Coder は `patch` を生成、Worker が適用

- ❌ **job_id なしで承認フローを管理する**
  - 理由: ログ追跡、承認状態管理が不可能
  - 実装: `pkg/approval/job.go` で job_id 生成必須

#### 4.1.4 API キー関連

- ❌ **API キーを平文で設定ファイルに保存する**
  - 理由: セキュリティリスク
  - 正: 環境変数から取得（`ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`, `OPENAI_API_KEY`）

#### 4.1.5 品質保証関連

- ❌ **LINT チェックなしでコードをコミットする**
  - 理由: コード品質の低下、バグの見逃し
  - 必須: コミット前に `golangci-lint run` を実行

- ❌ **テストなしで実装を追加・変更する**
  - 理由: リグレッション、バグの見逃し
  - 必須: TDD サイクルに従い、テストを先に書く

### 4.2 運用レベルの禁止

- ❌ **仕様を読まずに実装する**
  - 必ず `docs/01_正本仕様/実装仕様.md` を参照

- ❌ **承認なしに破壊的変更を適用する**
  - 削除、リネーム、広範囲の上書きには承認必須

- ❌ **正本仕様を更新せずに実装を変更する**
  - 実装と仕様の不整合を防ぐため、仕様を先に更新

- ❌ **LINT エラーを無視してコミットする**
  - すべての LINT エラーを解消してからコミット

---

## 5. ルーティング決定の詳細

### 5.1 ルーティングカテゴリ

| カテゴリ | 用途 | LLM | 実装 |
|---------|------|-----|------|
| `CHAT` | 会話・意思決定 | Ollama (chat-v1) | `pkg/agent/router.go` |
| `PLAN` | 計画策定 | - | - |
| `ANALYZE` | 分析 | - | - |
| `OPS` | 運用操作 | - | - |
| `RESEARCH` | 調査 | - | - |
| `CODE` | コーディング（汎用） | 設定による | `pkg/agent/router.go` |
| `CODE1` | 仕様設計向け | DeepSeek | `pkg/agent/loop.go` |
| `CODE2` | 実装向け | OpenAI | `pkg/agent/loop.go` |
| `CODE3` | 高品質コーディング/推論 | Claude API | `pkg/agent/loop.go` |

### 5.2 優先順位（固定）

1. **明示コマンド**: `/code`, `/code1`, `/code2`, `/code3` 等
2. **ルール辞書**: 強証拠による確定
3. **分類器**: LLM による判定
4. **安全側フォールバック**: `CHAT`

### 5.3 CODE3 の選択条件

**自動選択のキーワード**（`pkg/agent/loop.go` の `selectCoderRoute()`）:
- "高品質"
- "仕様策定"
- "複雑な推論"
- "重大バグ"
- "失敗コスト"

**明示的呼び出し**:
- `/code3` コマンド

---

## 6. 承認フローの実装詳細

### 6.1 標準フロー

```
1. Chat がジョブ作成（job_id 付与）
   ↓
2. Coder3 が plan/patch を生成
   ↓
3. Chat がユーザーへ承認要求（LINE/Slack）
   ↓
4. ユーザーが承認/拒否（/approve または /deny コマンド）
   ↓
5. 承認の場合: Worker が適用実行
   拒否の場合: ジョブをキャンセル
   ↓
6. 結果を通知、ログ保存
```

### 6.2 job_id の形式

**生成**: `pkg/approval/job.go` の `GenerateJobID()`

**形式**: `YYYYMMDD-HHMMSS-xxxxxxxx`
- `YYYYMMDD-HHMMSS`: タイムスタンプ（JST）
- `xxxxxxxx`: 8 桁の 16 進数ランダム値

**例**: `20260224-153045-a1b2c3d4`

### 6.3 承認コマンド

- **承認**: `/approve <job_id>`
- **拒否**: `/deny <job_id>`

**実装**: `pkg/agent/router.go` の `parseRouteCommand()`

---

## 7. セッション管理の詳細

### 7.1 日次カットオーバー

- **タイミング**: 日本時間 00:00（JST）
- **目的**: メモリリセット、ログ整理
- **実装**: `pkg/session/manager.go`

### 7.2 SessionFlags

**保存項目**（`pkg/session/manager.go`）:
```go
type SessionFlags struct {
    LocalOnly            bool    // /local モード中か
    PrevPrimaryRoute     string  // 前回のルート
    PendingApprovalJobID string  // 承認待ちの job_id
}
```

### 7.3 メモリ管理

- `short_memory` 中心で運用
- `recent_turns` は最小限（直近 3〜5 ターン）
- 長期記憶は Obsidian に保存

---

## 8. ヘルスチェックの詳細

### 8.1 チェックタイミング

- **起動時**: アプリケーション起動時に Ollama の状態確認
- **LLM 呼び出し前**: 毎回の LLM 呼び出し前にヘルスチェック
- **失敗時**: Ollama 再起動 → リトライ

### 8.2 チェック項目

**実装**: `pkg/health/checks.go`

1. **Ollama プロセス確認**: `OllamaCheck()`
2. **モデルロード確認**: `OllamaModelsCheck()`
   - 必要なモデルがロードされているか
   - context_length が MaxContext（8192）以下か

### 8.3 自動再起動

**コマンド**: `systemctl --user restart ollama`
**設定**: `providers.ollama_restart_command` で変更可能

---

## 9. ログとトレーサビリティ

### 9.1 ログイベント種別

**既存**（`pkg/logging/logger.go`）:
- `router.decision` - ルーティング決定
- `classifier.error` - 分類器エラー
- `worker.success` / `worker.fail` - Worker 実行結果
- `route.override` - ルート上書き
- `loop.stop` - ループ停止
- `final.route` - 最終ルート

**承認フロー追加**:
- `approval.requested` - 承認要求
- `approval.granted` - 承認許可
- `approval.denied` - 承認拒否
- `approval.auto_approved` - 自動承認
- `coder.plan_generated` - Coder による plan 生成

### 9.2 必須保存項目

- `job_id`: ジョブ識別子
- `approval_status`: 承認状態（`pending`, `granted`, `denied`, `auto_approved`）
- `approval_requested_at`: 承認要求時刻
- `approval_decided_at`: 承認決定時刻
- `approver`: 承認者（ユーザーID）
- `coder_output`: Coder の生成した plan/patch/risk の要約

---

## 10. 開発フロー

### 10.1 TDD（テスト駆動開発）サイクル（必須）

すべての新機能追加・バグ修正は TDD サイクルに従う:

#### 10.1.1 Red（失敗するテストを書く）

1. **要件を理解**: 何を実装するか明確にする
2. **テストケースを洗い出し**: 正常系・異常系・境界値
3. **テストを書く**: 期待される動作を表現
4. **テストを実行**: 失敗することを確認（Red）

```bash
# テストを実行（失敗することを確認）
go test ./pkg/approval/... -v
```

#### 10.1.2 Green（テストを通す最小実装）

1. **最小限の実装**: テストを通すだけのコードを書く
2. **テストを実行**: すべて通ることを確認（Green）

```bash
# テストを実行（成功することを確認）
go test ./pkg/approval/... -v
```

#### 10.1.3 Refactor（リファクタリング）

1. **コードの改善**: 重複排除、命名改善、構造最適化
2. **テストを実行**: リファクタ後も通ることを確認
3. **LINT チェック**: コード品質を確認

```bash
# LINT チェック（必須）
golangci-lint run ./pkg/approval/...

# テストを実行（リファクタ後も成功することを確認）
go test ./pkg/approval/... -v
```

#### 10.1.4 コミット前チェック（必須）

```bash
# 1. すべてのテストを実行
go test ./pkg/... -v

# 2. LINT チェック
golangci-lint run

# 3. エラーがなければコミット
git add .
git commit -m "feat: 機能追加"
```

### 10.2 新機能追加

1. **仕様確認**: `docs/01_正本仕様/仕様.md` で要件を確認
2. **実装仕様作成**: `docs/01_正本仕様/実装仕様.md` に追記
3. **実装プラン作成**: `docs/06_実装ガイド進行管理/` に日付付きプランを作成
4. **TDD サイクル**: Red → Green → Refactor を繰り返す（10.1 参照）
5. **統合テスト**: End-to-End シナリオで検証
6. **LINT チェック**: `golangci-lint run` でコード品質確認
7. **ドキュメント更新**: `docs/00_ドキュメント分類一覧.md` を更新

### 10.3 バグ修正

1. **再現確認**: エラーログ・症状を記録
2. **原因調査**: コードを読み、根本原因を特定
3. **TDD サイクル**:
   - **Red**: バグを再現する失敗テストを書く
   - **Green**: バグを修正してテストを通す
   - **Refactor**: 修正箇所を改善
4. **回帰テスト**: 修正箇所と関連機能をテスト
5. **LINT チェック**: `golangci-lint run` で確認
6. **ドキュメント**: 必要に応じて注意事項を追記

### 10.4 LINT チェックの詳細

#### 10.4.1 LINT ツール

**golangci-lint**（必須）:
```bash
# インストール
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 実行
golangci-lint run

# 特定パッケージのみ
golangci-lint run ./pkg/approval/...

# 自動修正（可能な場合）
golangci-lint run --fix
```

#### 10.4.2 チェック項目

**有効な Linter**（`.golangci.yml` で設定）:
- `errcheck`: エラーチェック漏れ
- `govet`: 疑わしいコード構造
- `staticcheck`: 静的解析
- `gosimple`: コード簡素化提案
- `unused`: 未使用コード検出
- `ineffassign`: 無駄な代入
- `gofmt`: フォーマット確認
- `goimports`: import 整理

#### 10.4.3 LINT エラーの対処

**優先度**:
1. **エラー**: 必ず修正（コミット禁止）
2. **警告**: 可能な限り修正
3. **情報**: 確認して判断

**例外的に無視する場合**:
```go
// nolint:errcheck // 理由: この戻り値は常に nil
_ = file.Close()
```

### 10.5 テスト

**ユニットテスト**:
```bash
go test ./pkg/... -v
```

**カバレッジ**:
```bash
go test ./pkg/approval/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

**目標カバレッジ**:
- 重要パッケージ（`agent`, `approval`, `session`）: 80% 以上
- その他のパッケージ: 70% 以上

---

## 11. 共通ルールへの参照

このプロジェクトでは、以下の共通ルールを参照します:

### 11.1 必須参照

- **AI 開発の共通方針**: `../common/GLOBAL_AGENT.md`
  - ペアプログラミングのコア原則
  - コード修正における思考憲法
  - データ処理の基本原則

### 11.2 ドメイン別ルール（必要に応じて）

- **アーキテクチャ**: `../common/rules_architecture.md`
- **バックエンド**: `../common/rules_backend.md`
- **セキュリティ**: `../common/rules_security.md`
- **テスト**: `../common/rules_testing.md`
- **ログ**: `../common/rules_logging.md`

---

## 12. プロジェクト固有の詳細ルール

### 12.1 ドメイン固有ルール

詳細は **`rules_domain.md`** を参照:
- Go 言語固有のベストプラクティス
- LLM プロバイダー統合の詳細
- ルーティングロジックの実装詳細
- 承認フローの実装パターン
- セッション管理の実装詳細
- ヘルスチェックの実装詳細

---

**最終更新**: 2026-02-24
**バージョン**: 1.0
**メンテナンス**: プロジェクトの運用が変わったときは、このファイルを必ず最新状態に更新してください。
