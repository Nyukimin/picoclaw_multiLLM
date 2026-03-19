# 詳細実装仕様 03: Tools体系の差

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`

---

## 1. 概要

RenCrow の tools 体系は、OpenClaw のような独立 registry 製品ではなく、`ToolRunner` 中心の実装である。  
ただし現行コードには、宣言・実行・構造化出力・tool calling 連携に必要な最小要素がすでに入っている。

現行の主な構成要素:

1. `ToolMetadata`
2. `ToolManifest`
3. `ToolResponse`
4. `RunnerV2`
5. `llm.ToolDefinition`

---

## 2. 宣言モデル

### 2.1 ToolMetadata

**ファイル**: `internal/domain/tool/metadata.go`

```go
type ToolMetadata struct {
    ToolID      string
    Version     string
    Category    string
    DryRun      bool
    Deprecated  bool
    ReplacedBy  string
    Invariants  []string
    Description string
    Parameters  map[string]any
}
```

意味:

- `ToolID`, `Version`: 識別
- `Category`: `query | mutation | admin`
- `Description`, `Parameters`: tool calling 用
- `Invariants`: 守るべき制約のメモ

### 2.2 ToolManifest

**ファイル**: `internal/domain/tool/manifest.go`

```go
type ToolManifest struct {
    ID          string
    Version     string
    Description string
    InputSchema map[string]any
    OutputSchema map[string]any
    SideEffect  SideEffect
    TimeoutSec  int
}
```

現行では `ToolManifest` が正本 registry ではなく、`ManifestFromMetadata(meta)` で既存 metadata を manifest 風に変換するアダプタとして使われる。

`SideEffect` の分類:

- `none`
- `local_write`
- `network`
- `process`

現行変換ルール:

- `mutation` -> `local_write`
- `admin` -> `process`
- それ以外 -> `none`

---

## 3. 実行契約

### 3.1 RunnerV2

**ファイル**: `internal/domain/tool/runner.go`

```go
type RunnerV2 interface {
    ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*ToolResponse, error)
    ListTools(ctx context.Context) ([]ToolMetadata, error)
}
```

`RunnerV2` が現行 tools 実行の中心 I/F であり、OpenClaw 的な executor/registry 分離はここに集約されている。

### 3.2 ToolResponse

**ファイル**: `internal/domain/tool/response.go`

```go
type ToolResponse struct {
    Result      any
    Error       *ToolError
    Metadata    map[string]any
    GeneratedAt time.Time
}
```

特徴:

- 成功・失敗を 1 つの構造で表現
- `Metadata` に KB 保存用などの構造化追加情報を持てる
- `String()` で文字列化可能
- `JSON()` で全体シリアライズ可能

旧草案の `json.RawMessage` 固定より、現行は `any + metadata` の柔軟な形になっている。

---

## 4. ToolRunner の現行実装

**ファイル**: `internal/infrastructure/tools/runner.go`

`ToolRunner` は次を提供する。

- `Execute()` 系の legacy 実行
- `ExecuteV2()` 系の構造化実行
- `ListTools()`
- `ToolDefinitions()`
- `RegisterSubagent()`

内蔵ツール例:

- `shell`
- `file_read`
- `file_write`
- `file_list`
- `web_search`
- `subagent`（条件付き）

### 4.1 ToolDefinitions()

`ToolDefinitions()` は `ToolMetadata` を `llm.ToolDefinition` へ変換し、tool calling 対応 LLM に渡す。

現行ルール:

- `Description` と `Parameters` をそのまま使う
- `subagent` は再帰防止のため除外

### 4.2 構造化エラー

V1 エラーは V2 では `ToolResponse.Error` に正規化される。  
そのため tool loop や policy runner は、文字列ではなく構造化エラー前提で処理できる。

---

## 5. Contract / Autonomous との接続

### 5.1 Contract 正規化

**ファイル**: `internal/application/contract/normalizer.go`  
**ファイル**: `internal/domain/contract/contract.go`

自由文 request は `Contract` に正規化される。

主な項目:

- `Goal`
- `Acceptance`
- `Constraints`
- `Artifacts`
- `Verification`
- `Rollback`

これは OpenClaw の execution contract を RenCrow 向けに簡略化した現行形である。

### 5.2 Autonomous 実行

`application/autonomous` は `Contract` を前提に `Plan -> Execute -> Verify -> Repair` を回す。  
ただし tools 体系の正本は依然として `ToolRunner` / `RunnerV2` 側であり、独立 registry から動的ロードする構成ではない。

---

## 6. OpenClaw 観点での差分

現行到達点:

- ツール宣言メタデータあり
- tool calling 用 `ToolDefinition` あり
- 構造化レスポンスあり
- security/policy 実行ラッパーあり
- contract 正規化あり

未到達:

1. 永続 `ToolRegistry` はない
2. capability index によるタグ検索はない
3. timeout / retries / policy decision を 1 つに束ねた `ExecutionEnvelope` はない
4. manifest の外部 YAML/JSON カタログ読み込みはない
5. MCP ツールとの統一 registry はない

---

## 7. 確認観点

- `ListTools()` が metadata を返す
- `ToolDefinitions()` が LLM 用定義を返す
- `ToolResponse` が success/error を構造化する
- `PolicyRunner` が `RunnerV2` を wrap できる
- subagent の tool loop が `ToolDefinition` を使う

以上をもって、RenCrow の tools 体系は「宣言不足で未整備」ではなく、`ToolRunner + ToolMetadata + ToolResponse` を核にした現行実装として扱う。今後の差分は registry/capability index のような抽象化強化である。
