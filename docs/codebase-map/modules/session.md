---
generated_at: 2026-02-28T18:30:00+09:00
run_id: run_20260228_170007
phase: 1
step: "1-5"
profile: picoclaw_multiLLM
artifact: module
module_group_id: session
---

# セッション管理

## 概要

チャネル単位でユーザーとの会話履歴を管理し、日次カットオーバーによる自動アーカイブとメモリ最小化を実現する。セッションは `pkg/session/` で定義し、永続化は JSON 形式でファイル保存、日次ノートは `workspace/memory/YYYYMM/YYYYMMDD.md` に Markdown として保存される。

## 関連ドキュメント
- プロファイル: codebase-analysis-profile.yaml
- 外部資料: docs/codebase-map/refs_mapping.md (session セクション)
  - `docs/01_正本仕様/実装仕様.md` (9章: 状態管理)
  - `docs/02_v2統合分割仕様/実装仕様_v2_09_状態管理.md`
  - `docs/Obsidian_仕様.md` (永続化形式の参考)

---

## 役割と責務

### 主要な責務

1. **セッションライフサイクル管理**
   - セッションの作成・取得・更新・削除
   - メッセージ履歴の追加・取得・切り詰め
   - セッションサマリの管理

2. **永続化機構**
   - セッション状態を JSON ファイルとして保存（`{storage}/<channel>_<chatID>.json`）
   - セッションキーに含まれる `:` をファイル名では `_` にサニタイズ（Windows 互換）
   - 起動時に既存セッションをロード

3. **日次カットオーバー**
   - 毎日 04:00 (Asia/Tokyo) を境界として、前日のセッション内容をアーカイブ
   - アーカイブ先: `workspace/memory/YYYYMM/YYYYMMDD.md`
   - カットオーバー後はセッションをリセット（Messages と Summary をクリア）

4. **セッションフラグ管理**
   - `LocalOnly`: ローカル実行モード
   - `PrevPrimaryRoute`: 前回ルート情報
   - `OriginMessageID` / `OriginRoute`: 元メッセージ情報（再ルーティング用）
   - `PendingOriginReply`: Origin への返信待ち状態
   - `WorkOverlayTurnsLeft` / `WorkOverlayDirective`: 仕事モード制御
   - `PendingApprovalJobID`: Coder3 承認待ちジョブ ID

### 対外インターフェース

**pkg/agent パッケージへの提供**:
- `NewSessionManager(storage string) *SessionManager`: セッション管理インスタンス生成
- `GetOrCreate(key string) *Session`: セッション取得または作成
- `AddMessage(sessionKey, role, content string)`: メッセージ追加
- `AddFullMessage(sessionKey string, msg providers.Message)`: ツールコールを含む完全メッセージ追加
- `GetHistory(key string) []providers.Message`: メッセージ履歴取得
- `SetHistory(key string, history []providers.Message)`: 履歴の一括更新
- `GetSummary(key string) string`: セッションサマリ取得
- `SetSummary(key string, summary string)`: サマリ設定
- `TruncateHistory(key string, keepLast int)`: 履歴切り詰め
- `Save(key string) error`: セッション永続化
- `GetUpdatedTime(key string) time.Time`: 最終更新時刻取得
- `ResetSession(key string)`: セッションリセット（Messages/Summary クリア、Flags は保持）
- `GetFlags(key string) SessionFlags`: フラグ取得
- `SetFlags(key string, flags SessionFlags)`: フラグ設定

**pkg/providers パッケージへの依存**:
- `providers.Message`: メッセージ構造（Role, Content, ToolCalls, ToolCallID, Media）

### 内部構造

**主要データ構造**:
```
Session:
  - Key: セッション識別子 (channel:chatID)
  - Messages: []providers.Message
  - Summary: string
  - Flags: SessionFlags
  - Created, Updated: time.Time

SessionFlags:
  - LocalOnly: bool
  - PrevPrimaryRoute: string
  - OriginMessageID, OriginRoute: string
  - PendingOriginReply: bool
  - WorkOverlayTurnsLeft: int
  - WorkOverlayDirective: string
  - PendingApprovalJobID: string
```

**セッションキーの構成**:
- 形式: `<channel>:<chatID>`
- 例: `telegram:123456`, `slack:C0123`, `discord:987654321`

---

## 依存関係

### 外部依存

- **標準ライブラリ**:
  - `encoding/json`: セッション永続化
  - `os`, `path/filepath`: ファイルシステム操作
  - `sync`: 並行アクセス制御 (RWMutex)
  - `time`: タイムスタンプ管理

- **pkg/providers**:
  - `providers.Message`: メッセージ構造定義

### 被依存

- **pkg/agent** (主要な利用者):
  - `AgentLoop`: セッション永続化とカットオーバー実行
  - `Router`: ルーティング決定時のフラグ参照・更新
  - `ContextBuilder`: (間接的) メッセージ履歴の参照

- **pkg/channels**:
  - Discord チャネルなどで間接的にセッションキーを生成

---

## 構造マップ

### ファイル構成

```
pkg/session/
├── manager.go           (351行) - SessionManager 実装
└── manager_test.go      (215行) - ユニットテスト
```

### セッションライフサイクル

```
[起動]
  └─→ NewSessionManager(storage)
       └─→ loadSessions() - 既存セッションを JSON からロード

[実行時]
  ├─→ GetOrCreate(sessionKey) - セッション取得/作成
  ├─→ AddMessage(sessionKey, role, content) - メッセージ追加
  ├─→ SetFlags(sessionKey, flags) - フラグ更新
  └─→ Save(sessionKey) - 永続化 (tmp ファイル → rename で原子性確保)

[日次カットオーバー] (AgentLoop.maybeDailyCutover から呼び出し)
  ├─→ GetUpdatedTime(sessionKey) - 最終更新時刻取得
  ├─→ 境界チェック (now >= 前日の 04:00 JST && updated < 前日の 04:00 JST)
  ├─→ GetHistory / GetSummary - セッション内容取得
  ├─→ MemoryStore.SaveDailyNoteForDate(date, note) - 日次ノート保存
  └─→ ResetSession(sessionKey) + Save(sessionKey) - セッションリセット

[メモリ圧迫時]
  ├─→ TruncateHistory(sessionKey, keepLast) - 履歴切り詰め
  └─→ SetSummary(sessionKey, summary) - サマリ更新
```

### 永続化機構

**セッションファイル**:
- 保存先: `{storage}/<sanitized_key>.json`
- サニタイズ: `:` → `_` (Windows 互換)
- 原子性: 一時ファイル書き込み → `os.Rename` で確定
- 検証: `filepath.IsLocal` でパストラバーサル防止

**日次ノート** (pkg/agent/memory.go で実装):
- 保存先: `workspace/memory/YYYYMM/YYYYMMDD.md`
- 形式: Markdown (ヘッダー + セッションサマリ + メッセージ要約)
- カットオーバー境界: 04:00 JST (CutoverHour)
- 論理日付: 04:00 より前の活動は前日扱い (GetLogicalDate)

**起動時ロード**:
- `loadSessions()`: `{storage}/*.json` を読み込み
- JSON 内の `Key` フィールドを使ってメモリ内マップを構築
- エラーファイルは無視（continue）

---

## 落とし穴・注意点

### 設計上の制約

1. **セッションキーとファイル名の不一致に注意**
   - セッションキー: `telegram:123456`
   - ファイル名: `telegram_123456.json`
   - ※ JSON 内の `Key` フィールドで元のキーが保持される

2. **日次カットオーバーのタイミング**
   - 毎日 04:00 JST を境界とする
   - 04:00 より前の活動は「前日」として扱われる
   - `GetLogicalDate` で論理日付を計算
   - ※ タイムゾーンは `Asia/Tokyo` 固定 (CutoverTimezone)

3. **ResetSession の挙動**（※Phase 2 で確認: L310-323）
   - Messages と Summary はクリア（L320-321）
   - **Flags は保持される**（LocalOnly, PrevPrimaryRoute など）
   - Created タイムスタンプは保持、Updated は現在時刻に更新（L322）
   - ※Phase 2 で確認: `session.Flags` には一切触れず、Messages と Summary のみクリア

4. **並行アクセス制御**（※Phase 2 で確認: L34-38, L169-246）
   - `sync.RWMutex` で保護（L36）
   - Save() 時は read lock で snapshot 取得（L185-205） → unlock 後に I/O（L207-246）
   - ※Phase 2 で確認: スナップショット取得後の変更は反映されない（意図的な設計、L192-204 でディープコピー）
   - ※Phase 2 で確認: SetHistory も完全なディープコピーを実行（L282-295, "Create a deep copy to strictly isolate internal state"）

### 既知の問題・リスク

1. **セッションファイルの肥大化**
   - 長期間使用すると Messages 配列が巨大化
   - 対策: TruncateHistory, SetSummary による定期圧縮
   - 日次カットオーバーによる自動リセット

2. **ファイル I/O の失敗時**
   - Save() エラーは呼び出し側でログ記録のみ（処理は継続）
   - セッション状態はメモリに残るため、次回 Save() でリカバリ可能

3. **Windows 互換性**（※Phase 2 で確認: L160-167, L169-182）
   - セッションキーの `:` がボリューム区切りと誤認される問題
   - `sanitizeFilename()` (L165-167) で `_` に置換
   - `filepath.IsLocal()` でパストラバーサル検証（L180）
   - ※Phase 2 で確認: L176-182 で ".", パストラバーサル（`/\`）、OS 予約名を拒否
   - ※Phase 2 で確認: JSON 内の `Key` フィールド（L193）で元のキーが保持される（loadSessions で復元、L275）

### 変更時の注意事項

1. **Session 構造体の拡張**
   - 新フィールド追加時は `json:"field,omitempty"` タグを推奨
   - SessionFlags に追加する場合も同様
   - 既存セッションとの互換性を保つ（omitempty で未設定時は JSON に含めない）

2. **日次カットオーバーロジックの変更**
   - カットオーバー時刻を変更する場合は CutoverHour 定数を更新
   - タイムゾーン変更は CutoverTimezone 定数を更新
   - GetCutoverBoundary, GetLogicalDate の一貫性を保つ

3. **永続化形式の変更**
   - 現在は JSON 形式のみ
   - Obsidian 連携を強化する場合、日次ノート形式の拡張を検討
   - ※ Obsidian_仕様.md 参照（タグ設計、frontmatter など）

4. **セッションキー命名の変更**
   - 現在の形式: `<channel>:<chatID>`
   - 変更する場合、sanitizeFilename() の検証ロジックも更新
   - ※ filepath.IsLocal() が `.`, `..`, 絶対パス、デバイス名を拒否

5. **Flags の追加・削除**
   - Flags 追加時は omitempty タグを付与
   - 削除時は既存セッションとの互換性を確認
   - ※ JSON unmarshal 時に未知フィールドは無視されるため後方互換

---

## 実装の詳細メモ

### セキュリティ対策

- **パストラバーサル防止**: `filepath.IsLocal()` で `.`, `..`, `/`, `\` を含むキーを拒否
- **OS 予約名の検出**: Windows の `NUL`, `COM1` などを `IsLocal()` が検出
- **原子性**: `os.CreateTemp()` + `os.Rename()` で途中状態の露出を防止

### テストカバレッジ

manager_test.go で以下を検証:
- セッションキーのサニタイズ（コロン → アンダースコア）
- パストラバーサル拒否（`.`, `..`, `/`, `\`）
- SessionFlags の永続化（LocalOnly, PrevPrimaryRoute など）
- WorkOverlayFlags の永続化
- GetUpdatedTime の挙動
- ResetSession の挙動（Messages/Summary クリア、Flags 保持）

### 依存パッケージとの結合点

- **pkg/agent/loop.go**:
  - `maybeDailyCutover()` でカットオーバー判定と実行
  - `Save()` を 10 箇所以上で呼び出し（フラグ更新時、メッセージ追加後など）

- **pkg/agent/router.go**:
  - `GetFlags()` でルーティング判定に使用（LocalOnly, PrevPrimaryRoute）
  - `SetFlags()` でルート情報を更新

- **pkg/agent/memory.go**:
  - MemoryStore が日次ノートの保存を担当
  - GetCutoverBoundary, GetLogicalDate を提供

---

**メンテナンス方針**:
- セッション管理は agent パッケージの中核機能であり、破壊的変更は慎重に行う
- 日次カットオーバーのタイミング変更は運用影響が大きい（既存ユーザーの習慣）
- Flags の追加は後方互換性を保つため omitempty タグを徹底
- 永続化形式の変更時はマイグレーションパスを用意

**関連する TODO/課題**:
※ 推測: Obsidian 連携の強化（タグ付け、内部リンク生成など）が将来的な拡張候補。

---

## Phase 2 検証結果

### 検証日時
2026-02-28

### 検証内容
- `pkg/session/manager.go` (351 行) との突合せ完了

### 発見された差異・追加事項
1. **ResetSession の実装詳細を追記**（※Phase 2 で追加）:
   - L310-323 で実装、Flags は一切触れず Messages と Summary のみクリア

2. **並行アクセス制御の詳細を追記**（※Phase 2 で追加）:
   - Save() のスナップショット取得は L185-205、ディープコピーで隔離
   - SetHistory も L282-295 でディープコピーを実行（コメント: "Create a deep copy to strictly isolate internal state"）

3. **Windows 互換性の実装詳細を追記**（※Phase 2 で追加）:
   - `sanitizeFilename()` は L165-167 で実装
   - パストラバーサル検証は L176-182 で実装（".", パス区切り、OS 予約名を拒否）
   - JSON 内の `Key` フィールド（L193, L275）で元のキーが保持される

### 構造マップの正確性検証
- ✅ ファイル構成: 正確（manager.go のみ、351 行）
- ✅ セッションライフサイクル: 正確（NewSessionManager, GetOrCreate, Save 等を確認）
- ✅ 永続化機構: 正確（JSON 形式、サニタイズ、原子性を確認）

### 落とし穴の網羅性検証
- ✅ ResetSession の挙動: 実コードで確認、Flags 保持を追記
- ✅ 並行アクセス制御: 実コードで確認、ディープコピーの実装を追記
- ✅ Windows 互換性: 実コードで確認、検証ロジックの詳細を追記

### 設計書との乖離
- なし（実装仕様 `docs/01_正本仕様/実装仕様.md` 9章「状態管理」と整合）

---

**最終更新**: 2026-02-28 (Phase 2 検証完了)
**解析担当**: Claude Sonnet 4.5 (codebase-analysis)
