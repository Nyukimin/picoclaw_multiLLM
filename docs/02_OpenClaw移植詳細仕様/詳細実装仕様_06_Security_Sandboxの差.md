# 詳細実装仕様 06: Security/Sandboxの差

**更新日**: 2026-03-19  
**ステータス**: 現行実装ベース  
**親仕様**: `docs/実装仕様_OpenClaw移植_v1.md`  
**関連**: `詳細実装仕様_01_実行基盤とセキュリティ境界.md`

---

## 1. 概要

本仕様は `01` が扱う実行ポリシーのうち、特に OpenClaw 由来の `SecurityProfile` と sandbox 表現の差分に絞って整理する。  
現行 RenCrow は、権限スコープを表す profile モデルを持つが、実行基盤としては主に workspace レベルの制御を用いている。

---

## 2. SecurityProfile

**ファイル**: `internal/domain/security/profile.go`

```go
type SecurityProfile struct {
    Name            string
    FilesystemScope string
    NetworkScope    string
    ProcessScope    string
    GitScope        string
    SandboxLevel    string
}
```

有効値:

- `FilesystemScope`: `workspace | readonly | none`
- `NetworkScope`: `blocked | allowlist | full`
- `ProcessScope`: `none | limited | full`
- `GitScope`: `read | safe_write | full`
- `SandboxLevel`: `workspace | process | container`

### 2.1 プリセット

- `StrictProfile()`
  - workspace / allowlist / limited / safe_write / workspace
- `BalancedProfile()`
  - workspace / full / limited / safe_write / workspace
- `DevProfile()`
  - workspace / full / full / full / process

`PolicyEngine.profileByMode()` は `security.policy_mode` からこれらを選ぶ。

---

## 3. 現行 enforcement の位置づけ

profile は domain に存在するが、現行 enforcement は全面的に profile オブジェクトを評価する形ではない。  
実際に強く効いているのは次の項目である。

- `NetworkScope`
- `DenyCommands`
- `WorkspaceEnforced`

具体的には:

- network tool への deny / allowlist 判定
- shell コマンドシグネチャ拒否
- workspace 外 `file_write` の拒否

つまり profile は「全権限モデルの正本」ではなく、現状では `PolicyEngine` の既定値供給源に近い。

---

## 4. SandboxLevel の現況

`SandboxLevel` には次の値がある。

- `workspace`
- `process`
- `container`

しかし現行コードで実運用されているのは主に `workspace` レベルである。

到達点:

- workspace 外 path の拒否
- tool 実行前ポリシー
- WorkerExecutionService の protected file / workspace 制御

未到達:

- container sandbox の実行基盤統合
- process/container ごとの実際の隔離切替
- OS レベル namespace や cgroup 相当の管理

---

## 5. 監査イベント分類

実行監査では次の event type を使う。

- `security.decision`
- `security.violation`

意味:

- allow 系 -> `security.decision`
- deny 系 -> `security.violation`

この分類は `execution.Service` と JSONL repository によって記録される。

---

## 6. OpenClaw 観点との差分

現行到達点:

- profile プリセットあり
- network scope あり
- workspace sandbox 相当あり
- deny command 監査あり

未到達:

1. approval mode や ask フローはない
2. sandbox level は主にモデル定義で、container 実装はない
3. filesystem/network/process/git の全組み合わせを一元強制する統合 engine ではない
4. tool 実行外の任意外部プロセス全体を包括的に拘束するわけではない

---

## 7. 確認観点

- `SecurityProfile.Validate()` が scope 値を検証する
- `policy_mode` から strict/balanced/dev が選ばれる
- `network_scope=allowlist` が host allowlist に効く
- `sandbox_level=container` があっても現状は実行隔離を切り替えない

以上をもって、RenCrow の security/sandbox は OpenClaw の完全移植ではなく、「workspace 中心の実行統制 + profile モデル導入」まで到達している現行実装として扱う。
