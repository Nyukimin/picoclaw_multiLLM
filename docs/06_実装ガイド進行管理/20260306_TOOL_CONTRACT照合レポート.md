# TOOL_CONTRACT 照合レポート

**作成日**: 2026-03-06
**対象**: `internal/infrastructure/tools/` 配下の全ツール + Step 4-7 実装
**照合元**: `TOOL_CONTRACT.md` v1.0（2026-03-05）

---

## 1. 総評

契約文書は「先に置く」思想で作られているが、実装は契約制定前のコードがそのまま残っている。Step 4-7 で追加した `subagent` を含め、ToolRunner 全体が契約非準拠。

**準拠率の概算**: 約 15%（非対話性とテスト一部のみ合格）

---

## 2. セクション別の照合

### 2.1 入出力の統一（TOOL_CONTRACT 1）-- 不合格

| 契約ルール | 現状 | 判定 |
|-----------|------|------|
| JSON が一次経路 | `map[string]interface{}` で受け取り、`string` で返す。入力は暗黙的 JSON 風だが、出力は生文字列 | 不合格 |
| JSON 出力がデフォルト | 全ツールが `string` を返す（構造化されていない） | 不合格 |
| エラーも JSON | `fmt.Errorf` の生文字列。`error.code/message/details` 形式なし | 不合格 |
| stdout=結果, stderr=ログ | Go 関数なので直接該当しないが、ログは `log.Printf` で混在 | 要改善 |

**根本原因**: `ToolFunc` の戻り値が `(string, error)` であり、構造化レスポンスを返す設計になっていない。

### 2.2 安全レール（TOOL_CONTRACT 2）-- 不合格

| 契約ルール | 現状 | 判定 |
|-----------|------|------|
| dry-run 必須（書き込み系） | `file_write`, `shell`, `subagent` いずれも dry-run なし | 不合格 |
| 承認フラグ | メタデータに `requires_approval` 宣言なし | 不合格 |
| パストラバーサル | `file_write` にチェックなし | **危険** |
| 制御文字チェック | 全ツール未実装 | 不合格 |
| ID 汚染チェック | 全ツール未実装 | 不合格 |
| フィールド制限（取得系） | `fields` パラメータなし | 不合格 |
| ページング（取得系） | `file_list` に limit 1000 ハードコードのみ | 不合格 |
| デフォルト上限 | 設定可能な limit なし | 不合格 |

**特に深刻な項目**:
- `shell` -- 任意コマンド実行可能、バリデーションなし、dry-run なし
- `file_write` -- パストラバーサル未対策（`../` で任意パスに書き込み可能）

### 2.3 予測可能性（TOOL_CONTRACT 3）-- 部分合格

| 契約ルール | 現状 | 判定 |
|-----------|------|------|
| 非対話 | 全ツール非対話 | **合格** |
| `generated_at` | web_search 結果に含まれない | 不合格 |
| タイムアウト設定可能 | web_search は 10s 固定、shell は ctx 依存（外部設定不可） | 不合格 |
| 無限待ち禁止 | subagent にタイムアウトなし | **危険** |
| リトライ方針 | 全ツール未実装 | 不合格 |

### 2.4 増殖に耐える運用（TOOL_CONTRACT 4）-- 不合格

| 契約ルール | 現状 | 判定 |
|-----------|------|------|
| tool_id 必須 | ツールに ID がない（文字列キーのみ） | 不合格 |
| version 必須 | バージョン管理なし | 不合格 |
| SKILL.md 同梱 | 既存スキルは契約の frontmatter 形式と不一致（`tool_id`, `version`, `invariants` がない） | 不合格 |
| 廃止宣言 | 仕組みなし | 不合格 |
| 単一責務 | `shell` が万能すぎる | 要改善 |

---

## 3. ツール別 Definition of Done チェック

### 3.1 全ツール共通チェック

| チェック項目 | shell | file_read | file_write | file_list | web_search | subagent |
|-------------|:-----:|:---------:|:----------:|:---------:|:----------:|:--------:|
| JSON 入力 | -- | -- | -- | -- | -- | -- |
| JSON 出力 | NG | NG | NG | NG | NG | NG |
| JSON エラー | NG | NG | NG | NG | NG | NG |
| 入力バリデーション | NG | NG | NG | NG | OK* | OK* |
| SKILL.md | NG | NG | NG | NG | NG | NG |
| テスト | NG | NG | NG | NG | OK | OK |

`*` 最低限の空チェックのみ。契約が要求する脅威対策（パストラバーサル等）は未実装。

入力について `--` としたのは、Go の内部関数として `map[string]interface{}` を受け取っており、外部 JSON 入力との境界が曖昧なため。厳密には「JSON が一次経路」の設計にはなっていない。

### 3.2 書き込み系の追加チェック

| チェック項目 | shell | file_write | subagent |
|-------------|:-----:|:----------:|:--------:|
| dry-run (plan) | NG | NG | NG |
| 承認フラグ | NG | NG | NG |

### 3.3 取得系の追加チェック

| チェック項目 | file_read | file_list | web_search |
|-------------|:---------:|:---------:|:----------:|
| フィールド制限 | NG | NG | NG |
| ページング | N/A | NG | NG |
| デフォルト上限 | N/A | 1000固定 | 5固定 |

---

## 4. Step 4-7 実装の契約照合

Step 4-7 はツール層ではなくドメイン/インフラ層の実装だが、契約思想との整合を確認する。

### 4.1 Step 4: スレッド自動検出

| 観点 | 評価 |
|------|------|
| 入出力 | `ThreadBoundaryResult` 構造体で型安全。契約の JSON 入出力とは別レイヤー | 問題なし |
| 安全レール | Detector は読み取り専用（副作用なし）。Engine 側で FlushThread + CreateThread を実行 | OK |
| 予測可能性 | Embedding nil 時はスキップ（安全側フォールバック） | OK |

### 4.2 Step 5: チャットコマンド

| 観点 | 評価 |
|------|------|
| 入出力 | `ChatCommandResult` 構造体。レスポンスは人間向け文字列 | 問題なし（UI 層） |
| 安全レール | `/compact`, `/new` は破壊的操作だが確認なし | 要検討 |
| 予測可能性 | 同じコマンド → 同じ動作（状態依存だが冪等性は高い） | OK |

### 4.3 Step 6: UserProfile 自動抽出

| 観点 | 評価 |
|------|------|
| 入出力 | LLM → JSON パース。`extractJSON` でベストエフォート抽出 | OK |
| 安全レール | パース失敗は空結果（安全側）。インメモリのみで永続化なし | OK |
| 予測可能性 | LLM 依存のため非決定的。ただし best-effort 設計で副作用は限定的 | 許容範囲 |

### 4.4 Step 7: SkillsLoader + Subagent

| 観点 | 評価 |
|------|------|
| SkillsLoader | 読み取り専用。SKILL.md の frontmatter パースは契約テンプレートと**不一致**（`tool_id`, `version`, `invariants` を未解析） | 要改善 |
| Subagent | タイムアウトなし、dry-run なし、エラーは生文字列 | **契約違反** |

---

## 5. 危険度ランキング

優先的に修正すべき項目を危険度順に整理する。

| 順位 | 項目 | 危険度 | 理由 |
|------|------|--------|------|
| 1 | `shell` の無制限実行 | **Critical** | 任意コマンド実行可能。バリデーション・dry-run・承認すべてなし |
| 2 | `file_write` のパストラバーサル | **Critical** | `../` で任意パスに書き込み可能 |
| 3 | `subagent` の無限待ち | High | タイムアウトなし。エージェントがハングすると呼び出し元も停止 |
| 4 | 全ツールの非構造化エラー | High | エラーハンドリングが文字列マッチに依存。自動リトライ判断不可 |
| 5 | 全ツールの非構造化レスポンス | Medium | パイプライン接続（ツール連鎖）が困難 |
| 6 | SKILL.md の契約テンプレート不一致 | Medium | SkillsLoader が `tool_id`, `invariants` を読めない |
| 7 | 取得系のページングなし | Low | 現時点ではデータ量が小さいため実害は限定的 |

---

## 6. 推奨アクションプラン

### Phase A: 安全レールの緊急修正（最優先）

1. **`file_write` にパストラバーサルチェック追加**
   - `filepath.Clean` + ワークスペース外パス拒否
2. **`shell` に許可リスト or サンドボックス導入**
   - 最低限: 禁止コマンドリスト（`rm -rf /`, `dd`, `mkfs` 等）
3. **`subagent` にタイムアウト追加**
   - `context.WithTimeout` でデフォルト 30 秒

### Phase B: ToolRunner の構造化リファクタリング

1. **`ToolResponse` 構造体の導入**
   ```go
   type ToolResponse struct {
       Data        interface{} `json:"data"`
       GeneratedAt time.Time   `json:"generated_at"`
   }
   type ToolError struct {
       Code    string      `json:"code"`
       Message string      `json:"message"`
       Details interface{} `json:"details,omitempty"`
   }
   ```
2. **`ToolFunc` のシグネチャ変更**
   ```go
   // Before
   type ToolFunc func(ctx context.Context, args map[string]interface{}) (string, error)
   // After
   type ToolFunc func(ctx context.Context, args map[string]interface{}) (*ToolResponse, *ToolError)
   ```
3. **共通バリデーションミドルウェア**
   - パストラバーサル、制御文字、ID 汚染を共通層でチェック

### Phase C: 契約メタデータの整備

1. **ToolDescriptor 構造体**
   ```go
   type ToolDescriptor struct {
       ToolID           string
       Version          string
       Category         string // "query" | "mutation" | "admin"
       RequiresApproval bool
       DryRun           bool
       Deprecated       bool
       ReplacedBy       string
   }
   ```
2. **registerTools を宣言的に変更**
   - 各ツールに Descriptor を紐付け、実行前に契約チェック
3. **SKILL.md の frontmatter を契約テンプレートに統一**
   - `tool_id`, `version`, `invariants` を必須化
4. **SkillsLoader の拡張**
   - 契約テンプレートの全フィールドをパース

### Phase D: dry-run フレームワーク

1. **書き込み系ツールに `mode` パラメータ追加**（`"plan"` or `"execute"`）
2. **plan モードでは副作用なし、差分を JSON で返す**
3. **承認フロー統合**（Worker がゲートキーパー）

---

## 7. 結論

TOOL_CONTRACT.md は設計思想として正しく、RenCrow の成長に不可欠なガードレール。しかし現時点の実装は契約制定前のコードがそのまま残っており、**契約と実装の間に大きなギャップがある**。

Step 4-7 のドメイン層実装（スレッド検出、チャットコマンド、UserProfile、SkillsLoader）は比較的健全だが、ツール層（ToolRunner + 各ツール実装）は契約の要求水準に達していない。

**次のアクション**: Phase A（安全レールの緊急修正）を最優先で実施し、その後 Phase B-D で段階的に契約準拠へ移行する。

---

**最終更新**: 2026-03-06
**作成者**: Claude Opus 4.6（自動照合）
**次回レビュー**: Phase A 完了時
