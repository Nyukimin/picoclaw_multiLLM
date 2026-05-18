# AI Native Engineering Workflow 仕様

## 1. 目的

本仕様は、RenCrow における Worker / Coder / Heavy Worker / DCI / Tool Harness が、単なるチャット型 AI ではなく、実際の開発環境上で安定して作業するための運用基盤を定義する。

本仕様では、この考え方を **AI Native Engineering Workflow** と呼ぶ。

目的は、モデル単体の性能に頼るのではなく、以下を整備することで、AI の実作業性能を引き上げることである。

```text
プロジェクト記憶
初期スキャン
worktree 分離
CLI ツール
MCP / 外部接続
Skill / Command
Subagent 分離
Token / Context 管理
Heavy Worker 起動条件
CI/CD 連携
```

この仕様は、Claude Code 固有の運用をそのまま移植するものではない。Claude Code 系の実践で有効とされる「AI をチャットボットではなく、AI 開発環境として扱う」という考え方を、RenCrow の Worker / Coder 構成へ再設計して取り込むものである。

重要なのは「より良いプロンプト」だけではなく、「モデルの周囲に適切なシステムを作ること」である。

## 2. 位置づけ

本仕様は、以下の仕様群と接続する。

```text
19_DCI_直接コーパス探索仕様
  原文を調べ直す能力

20_Tool_Harness_Contract_Mediation仕様
  ツール呼び出しを検証、修復、安全実行する能力

21_AI_Native_Engineering_Workflow仕様
  Worker / Coder が働く開発環境そのものを整える仕様
```

RenCrow における本仕様の位置づけは以下である。

```text
User / Chat
  ↓
Task Request
  ↓
Worker / Coder Routing
  ↓
Project Init Pack
  ↓
Project Memory / Rules / Skills
  ↓
Worktree / Isolated Workspace
  ↓
Tool Harness
  ↓
DCI / CLI / File / Test / Patch
  ↓
Proposal / Report / Review
  ↓
Human Approval / Worker Execution
```

## 3. 基本方針

RenCrow の AI 開発運用では、以下を原則とする。

1. AI にいきなり修正させない。
2. まずプロジェクトを初期スキャンする。
3. プロジェクト記憶を明示ファイルとして持つ。
4. main branch を直接汚さない。
5. feature / experiment は worktree で分離する。
6. CLI ツールを標準化し、探索、解析を高速化する。
7. 再利用可能な作業は command / skill 化する。
8. 調査や分析は subagent に隔離し、メイン文脈を汚さない。
9. token / context / tool call を計測する。
10. Heavy Worker は必要時のみ起動する。
11. CI/CD では AI に直接 merge させず、review / proposal に制限する。
12. すべての作業単位に EventId を付与する。

## 4. RenCrow 内の責務分担

### 4.1 Chat

Chat はユーザーとの会話インタフェースである。

```text
責務:
- ユーザー意図の受け取り
- タスクの整理
- Worker / Coder への依頼
- 進行状況の提示
- 最終報告の提示
```

Chat は原則として、コード修正や shell 実行を直接行わない。

### 4.2 Worker

Worker は実行主体である。

```text
責務:
- ファイル読み取り
- grep / rg / DCI 探索
- test 実行
- patch 適用
- git diff 確認
- worktree 作成
- staging 出力
- 実行ログ保存
```

Worker は Tool Harness と Command Gate を必ず通す。

### 4.3 Coder

Coder は設計、実装案、patch proposal 生成を担当する。

```text
責務:
- コード調査
- 変更方針作成
- patch proposal 作成
- 影響範囲分析
- テスト案作成
- レビュー指摘対応
```

Coder は破壊的操作を直接実行しない。

### 4.4 Heavy Worker

Heavy Worker は高コスト、高文脈の作業専用である。

```text
起動条件:
- 大規模リファクタ
- 巨大リポジトリ解析
- 複数ファイル横断推論
- アーキテクチャ変更
- 通常 Worker / Coder で判断不能
```

通常作業では起動しない。

### 4.5 Subagent

Subagent は文脈隔離用の一時エージェントである。

```text
用途:
- コードベース調査
- 依存関係追跡
- UX 分析
- ログ調査
- ドキュメント要約
- テスト失敗原因分析
```

Subagent は、調査結果だけをメイン文脈へ返す。

## 5. Project Memory

### 5.1 目的

Project Memory は、プロジェクト固有の知識を AI が毎回再学習しなくて済むようにするための永続ファイル群である。

Project Memory には以下を含める。

```text
architecture decisions
coding patterns
debugging notes
edge cases
product context
recurring mistakes
```

RenCrow では、これを単一ファイルに詰め込まず、役割ごとに分離する。

### 5.2 推奨ファイル構成

```text
.ai/
  PROJECT_MEMORY.md
  ARCHITECTURE_DECISIONS.md
  CODING_PATTERNS.md
  DEBUGGING_NOTES.md
  RECURRING_MISTAKES.md
  PRODUCT_CONTEXT.md
  EDGE_CASES.md
  RULES.md
  SKILL_INDEX.md
```

Claude Code 互換を重視する場合は、以下を併置してもよい。

```text
CLAUDE.md
```

ただし、RenCrow の正本は `.ai/` 配下の分割ファイルとする。

### 5.3 PROJECT_MEMORY.md

```markdown
# PROJECT_MEMORY

## Project Summary
このプロジェクトの目的。

## Current Architecture
現在の構成。

## Important Decisions
重要な決定事項。

## Active Constraints
守るべき制約。

## Known Risks
既知のリスク。

## Current Priorities
現在の優先事項。
```

### 5.4 RECURRING_MISTAKES.md

```markdown
# RECURRING_MISTAKES

## 過去に起きた失敗
- 何が起きたか
- 原因
- 再発防止

## AI がやりがちな誤り
- 既存設計を無視して新規実装する
- 破壊的変更を先に提案する
- テストせず完了報告する
```

## 6. Project Init Pack

### 6.1 目的

Project Init Pack は、新しいコードベースに入った時に、Worker / Coder が最初に作成するプロジェクト理解パックである。

RenCrow では、これを明示的な初期化ワークフローとして実装する。

### 6.2 実行タイミング

以下の場合に実行する。

```text
- 新規リポジトリを開いた時
- 既存リポジトリで .ai/PROJECT_MEMORY.md がない時
- 大幅な構成変更後
- Coder がプロジェクト理解不足と判断した時
- ユーザーが「初期スキャンして」と依頼した時
```

### 6.3 生成物

```text
.ai/project_profile.md
.ai/source_map.md
.ai/dependency_map.md
.ai/test_commands.md
.ai/build_commands.md
.ai/coding_conventions.md
.ai/risk_notes.md
```

### 6.4 Project Init Flow

```text
1. list root files
2. detect language / framework
3. read README / package / config
4. inspect source tree
5. detect build / test commands
6. detect entrypoints
7. detect architecture boundaries
8. detect risky files
9. generate project_profile
10. register Source Registry entries
```

### 6.5 project_profile.md 例

````markdown
# Project Profile

## Overview
プロジェクト概要。

## Languages
- TypeScript
- Python

## Frameworks
- React
- FastAPI

## Entry Points
- src/main.ts
- src/server.py

## Build

```bash
npm run build
```

## Test

```bash
npm test
pytest
```

## Architecture Notes

- UI と API は分離
- Worker は直接 DB を書かない

## Risk Notes

- scripts/ は破壊的操作を含む可能性あり
- .env は読み取り禁止
````

## 7. Worktree 運用

### 7.1 目的

AI による並列作業や実験を安全に行うため、main branch を直接変更しない。

RenCrow では、Worker / Coder の実装作業は原則 worktree 上で行う。ただし、既存運用でユーザーが明示的に現在 worktree での docs 編集や軽微修正を許可した場合は、その指示を優先し、差分範囲を限定して扱う。

### 7.2 基本ルール

```text
main:
  直接変更禁止

feature worktree:
  実装作業

experiment worktree:
  試行錯誤

review worktree:
  レビュー・比較

hotfix worktree:
  緊急修正
```

### 7.3 ディレクトリ例

```text
repo/
  .git/

../worktrees/
  repo-feature-dci/
  repo-feature-tool-harness/
  repo-experiment-ui/
  repo-review-pr-123/
```

### 7.4 Worktree 作成コマンド例

```bash
git worktree add ../worktrees/repo-feature-dci -b feature/dci
```

### 7.5 Worktree メタ情報

RenCrow は worktree を登録する。

```json
{
  "worktree_id": "wt_20260518_0001",
  "repo": "rencrow",
  "path": "../worktrees/repo-feature-dci",
  "branch": "feature/dci",
  "purpose": "DCI実装",
  "owner_agent": "Coder",
  "created_at": "2026-05-18T12:00:00Z",
  "status": "active"
}
```

## 8. 標準 CLI ツール

### 8.1 目的

AI がコードベースを効率よく探索、解析できるよう、標準 CLI ツールを整備する。

RenCrow では、以下を標準ツールとする。

```text
rg
fd
jq
git
python3
node
```

### 8.2 必須ツール

```text
rg:
  高速全文検索。DCI でも使用。

fd:
  高速ファイル発見。

jq:
  JSON 解析、整形。

git:
  diff / status / worktree / branch 確認。

python3:
  読取、解析スクリプト。

node:
  JS / TS 系プロジェクトで使用。
```

### 8.3 Command Gate

CLI ツールは Tool Harness と Command Gate を必ず通す。

```text
許可:
  rg
  fd
  jq
  git status
  git diff
  git worktree list
  sed -n
  head
  tail
  wc

条件付き:
  npm test
  npm run build
  go test
  pytest
  python3 scripts/read_only_analysis.py

拒否:
  rm
  mv
  chmod
  chown
  git push
  git reset --hard
  npm install
  pip install
  curl
  wget
```

## 9. MCP / 外部接続

### 9.1 目的

MCP や外部接続は、ライブドキュメント、ブラウザ、DB、API、デザインシステムなど、ローカルコーパス外の文脈を取得するために使う。

RenCrow では、MCP は直接本番 DB へ接続しない。

### 9.2 原則

```text
MCP output
  ↓
staging
  ↓
validator
  ↓
Source Registry
  ↓
promoted DB
```

MCP で得た情報を、そのまま正式記憶や Knowledge DB へ入れてはいけない。

### 9.3 MCP 使用場面

```text
- 公式ドキュメント確認
- ブラウザ調査
- API 仕様確認
- デザインシステム参照
- 外部 DB 確認
- Notion / Docs / issue 参照
```

### 9.4 禁止

```text
- MCP から本番 DB へ直接 write
- MCP 取得情報の無検証昇格
- ユーザー記憶の直接確定
- secret / token の自動取得
```

## 10. Skill / Plugin 設計

### 10.1 目的

再利用可能な専門作業を Skill として定義する。

RenCrow では、plugin という語より **Skill** を優先する。

### 10.2 Skill 構成

```text
skills/
  frontend-review/
    SKILL.md
    examples/
    scripts/

  architecture-review/
    SKILL.md
    templates/

  refactor-safety/
    SKILL.md
    checklists/

  test-generation/
    SKILL.md
    templates/

  documentation/
    SKILL.md
```

### 10.3 Skill 定義例

```markdown
# SKILL: architecture-review

## Purpose
設計変更が既存アーキテクチャと矛盾しないか確認する。

## Inputs
- 対象仕様
- 関連ファイル
- 変更案

## Steps
1. Project Memory を読む
2. Source Registry から正本仕様を確認
3. 変更案との差分を見る
4. 破壊的影響を列挙
5. 採用可否を提案する

## Output
- 良い点
- 懸念点
- 変更必須点
- 保留点
```

## 11. Reusable Slash Commands

### 11.1 目的

毎回プロンプトを書き直さず、定型作業をコマンド化する。

RenCrow では、slash command を `commands/*.md` として管理する。

### 11.2 ディレクトリ構成

```text
commands/
  init-project.md
  review-architecture.md
  generate-tests.md
  security-audit.md
  refactor-plan.md
  dci-search.md
  tool-harness-check.md
```

### 11.3 command 定義例

````markdown
# /review-architecture

## Purpose
変更案が既存アーキテクチャと矛盾しないか確認する。

## Agent
Coder + architecture-review skill

## Required Context
- PROJECT_MEMORY.md
- ARCHITECTURE_DECISIONS.md
- 対象仕様
- git diff

## Steps
1. 正本仕様を読む
2. 変更案の目的を 1 行で要約
3. 既存方針との一致点を列挙
4. 矛盾点を列挙
5. 修正案を出す

## Output

```text
結論:
一致点:
懸念:
修正案:
保留:
```
````

## 12. Subagent Context Isolation

### 12.1 目的

メインエージェントの文脈汚染を防ぐ。

長い調査、ログ解析、依存関係追跡などをメイン文脈で行うと、回答品質が落ちる。そのため、Subagent を一時的に起動し、結果だけを戻す。

### 12.2 Subagent 種別

```text
ResearchAgent:
  コードベース調査

DebugAgent:
  エラー原因調査

UXAgent:
  UI / UX 分析

DocAgent:
  ドキュメント整理

DependencyAgent:
  依存関係追跡

TestAgent:
  テスト失敗分析
```

### 12.3 実行フロー

```text
Main Coder
  ↓
Subagent Task
  ↓
Subagent isolated context
  ↓
Tool Harness / DCI
  ↓
Subagent Report
  ↓
Main Coder receives only summary
```

### 12.4 Subagent Report 形式

```markdown
# Subagent Report

## Task
調査内容。

## Scope
見た範囲。

## Findings
- 発見1
- 発見2

## Evidence
- file path / line / event id

## Confidence
0.82

## Limitations
- 未確認範囲
```

## 13. Token / Context Tracking

### 13.1 目的

AI 作業のコスト、文脈肥大、不要な tool call を管理する。

RenCrow では、課金だけでなく、ローカル LLM の context budget、KV cache 負荷、latency 管理にも使う。

### 13.2 記録項目

```text
session_id
event_id
agent
model
input_tokens
output_tokens
context_tokens
tool_call_count
dci_call_count
repair_count
latency_ms
estimated_cost
kv_cache_estimate
```

### 13.3 DB 案

```sql
CREATE TABLE IF NOT EXISTS ai_context_usage (
  event_id TEXT PRIMARY KEY,
  session_id TEXT,
  agent TEXT NOT NULL,
  model TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  context_tokens INTEGER,
  tool_call_count INTEGER DEFAULT 0,
  dci_call_count INTEGER DEFAULT 0,
  repair_count INTEGER DEFAULT 0,
  latency_ms INTEGER,
  estimated_cost REAL,
  kv_cache_estimate REAL,
  created_at TEXT NOT NULL
);
```

### 13.4 Context Budget

```yaml
context_budget:
  default:
    recall_pack_tokens: 1500
    project_memory_tokens: 2000
    tool_result_tokens: 2000
    conversation_tokens: 3000

  heavy:
    recall_pack_tokens: 4000
    project_memory_tokens: 6000
    tool_result_tokens: 6000
    conversation_tokens: 8000
```

## 14. Heavy Worker / High-token Provider Policy

### 14.1 目的

高コスト、高文脈モデルを必要時のみ使う。

RenCrow では、これを Heavy Worker 起動条件として扱う。

### 14.2 起動条件

```text
- 対象ファイル数が 20 を超える
- 関連仕様が複数にまたがる
- 変更がアーキテクチャ境界を超える
- 通常 Coder が不確実性を高く報告した
- 2 回試行して失敗した
- ユーザーが明示的に深掘りを依頼した
```

### 14.3 起動しない条件

```text
- 単純な文言修正
- 小さなテスト追加
- 既知パターンの修正
- grep で確認できるだけの作業
- 仕様書への軽微追記
```

## 15. IDE / Editor Integration

### 15.1 目的

AI 作業の可視性を高める。

RenCrow では、初期 MVP では必須にしない。

### 15.2 方針

```text
MVP:
  CLI + worktree + report

次段階:
  VS Code / Cursor 連携
  diff 表示
  proposal 表示
  test 結果表示

将来:
  RenCrow Viewer 内で patch proposal 表示
```

## 16. CI/CD 連携

### 16.1 目的

AI を開発ライフサイクルに組み込む。

RenCrow では、AI に直接 merge や push を許可しない。

### 16.2 許可すること

```text
- PR review comment 生成
- architecture rule check
- test failure summary
- security audit report
- documentation diff check
- coding standard check
- fix proposal 生成
```

### 16.3 禁止すること

```text
- 自動 merge
- 自動 push
- main branch への直接 commit
- secret を含むログ投稿
- 破壊的修正の自動適用
```

### 16.4 CI Bot Flow

```text
PR opened
  ↓
RenCrow CI Review Job
  ↓
Project Memory load
  ↓
diff scan
  ↓
architecture rule check
  ↓
test result analysis
  ↓
review comment / report
  ↓
human approval
```

## 17. Source Registry との関係

Project Init、DCI、MCP、CI で得た情報は、Source Registry に接続する。

```text
observed
  ↓
candidate
  ↓
validated
  ↓
promoted
```

以下は直接 promoted しない。

```text
- MCP 取得情報
- Subagent の推測
- CI Bot の指摘
- Coder の仮説
- DCI で見つけた未検証断片
```

## 18. EventId

すべての作業単位に EventId を付与する。

イベント種別:

```text
project_init_started
project_init_completed
project_memory_updated
worktree_created
worktree_removed
command_invoked
skill_invoked
subagent_started
subagent_completed
context_budget_exceeded
heavy_worker_requested
heavy_worker_started
ci_review_started
ci_review_completed
```

## 19. DB 設計

### 19.1 ai_workflow_event

```sql
CREATE TABLE IF NOT EXISTS ai_workflow_event (
  event_id TEXT PRIMARY KEY,
  parent_event_id TEXT,
  event_type TEXT NOT NULL,
  agent TEXT,
  repo TEXT,
  worktree_id TEXT,
  command_name TEXT,
  skill_name TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  completed_at TEXT,
  summary TEXT
);
```

### 19.2 project_memory_index

```sql
CREATE TABLE IF NOT EXISTS project_memory_index (
  id TEXT PRIMARY KEY,
  repo TEXT NOT NULL,
  file_path TEXT NOT NULL,
  memory_type TEXT NOT NULL,
  title TEXT,
  summary TEXT,
  content_hash TEXT,
  updated_at TEXT NOT NULL
);
```

### 19.3 worktree_registry

```sql
CREATE TABLE IF NOT EXISTS worktree_registry (
  worktree_id TEXT PRIMARY KEY,
  repo TEXT NOT NULL,
  path TEXT NOT NULL,
  branch TEXT NOT NULL,
  purpose TEXT,
  owner_agent TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  closed_at TEXT
);
```

### 19.4 command_registry

```sql
CREATE TABLE IF NOT EXISTS command_registry (
  command_name TEXT PRIMARY KEY,
  file_path TEXT NOT NULL,
  description TEXT,
  default_agent TEXT,
  required_skill TEXT,
  updated_at TEXT NOT NULL
);
```

## 20. 設定ファイル案

### 20.1 configs/ai_workflow.yaml

```yaml
ai_workflow:
  project_init:
    required_before_modify: true
    outputs:
      - ".ai/project_profile.md"
      - ".ai/source_map.md"
      - ".ai/test_commands.md"
      - ".ai/build_commands.md"

  project_memory:
    root: ".ai/"
    files:
      - "PROJECT_MEMORY.md"
      - "ARCHITECTURE_DECISIONS.md"
      - "CODING_PATTERNS.md"
      - "DEBUGGING_NOTES.md"
      - "RECURRING_MISTAKES.md"
      - "PRODUCT_CONTEXT.md"
      - "EDGE_CASES.md"
      - "RULES.md"

  worktree:
    enabled: true
    required_for_write: true
    base_dir: "../worktrees/"
    branch_prefixes:
      feature: "feature/"
      experiment: "experiment/"
      hotfix: "hotfix/"
      review: "review/"

  cli_tools:
    required:
      - "rg"
      - "fd"
      - "jq"
      - "git"

  subagents:
    enabled: true
    return_summary_only: true
    max_parallel: 4

  context_tracking:
    enabled: true
    warn_at_ratio: 0.8
    stop_at_ratio: 0.95

  heavy_worker:
    enabled: true
    require_reason: true

  ci_review:
    enabled: false
    allow_comment: true
    allow_merge: false
    allow_push: false
```

## 21. 実装ファイル案

RenCrow の現行 Go 構成に合わせ、実装候補は以下とする。

```text
internal/domain/aiworkflow/
  project_memory.go
  worktree.go
  command.go
  context_usage.go
  event.go

internal/application/aiworkflow/
  project_init.go
  project_memory.go
  worktree_manager.go
  command_registry.go
  skill_registry.go
  subagent_manager.go
  context_tracker.go
  heavy_worker_policy.go
  ci_review.go
  workflow_logger.go

internal/infrastructure/persistence/aiworkflow/
  sqlite_events.go
  sqlite_project_memory.go
  sqlite_worktrees.go
  sqlite_commands.go

cmd/picoclaw/
  runtime_ai_workflow.go
```

## 22. MVP 実装順

### Phase 1: Project Init

- project scan
- project_profile 生成
- source_map 生成
- build / test command 検出
- Project Memory index 登録

### Phase 2: Project Memory

- `.ai/` ファイル群作成
- `PROJECT_MEMORY.md` 更新ルール
- recurring mistakes 登録
- architecture decisions 登録

### Phase 3: Worktree

- worktree 作成
- worktree_registry
- write 作業は worktree 必須
- main 直接変更禁止

### Phase 4: Commands / Skills

- `commands/*.md`
- skill registry
- `/review-architecture`
- `/generate-tests`
- `/dci-search`

### Phase 5: Subagents

- isolated context 実行
- Subagent Report
- メイン文脈への要約返却

### Phase 6: Context Tracking

- token / context / tool call 記録
- context budget 警告
- Heavy Worker 起動判定

### Phase 7: CI Review

- PR diff 読み取り
- architecture rule check
- review report 生成
- human approval 前提

## 23. 成功指標

```text
project_init_coverage
project_memory_hit_rate
worktree_usage_rate
main_branch_direct_write_count
command_reuse_count
subagent_success_rate
context_budget_exceeded_count
heavy_worker_appropriate_use_rate
ci_review_issue_detection_rate
```

特に重要な指標:

```text
main branch 直接変更 = 0
project init 未実行での write = 0
破壊的操作の自動実行 = 0
同じ説明の繰り返し回数の低下
Coder task completion rate の向上
```

## 24. 設計上の結論

AI コーディングの品質は、モデル単体では決まらない。

実作業性能は以下の掛け算で決まる。

```text
モデル能力
x プロジェクト記憶
x 初期スキャン品質
x ツールハーネス
x worktree 分離
x command / skill 運用
x context 管理
x 安全ゲート
```

RenCrow では、AI を単なるチャットボットとして扱わない。

Worker / Coder / Heavy Worker が、整備された開発環境の中で、調査、提案、検証、実行を分担する。そのために、本仕様を Worker / Coder 実行基盤の中核仕様として採用する。

## 25. まとめ

本仕様は、RenCrow の AI 開発環境運用を定義する。

対象は以下である。

```text
Project Memory
Project Init Pack
Git worktree
CLI tools
MCP staging
Skill
Reusable Command
Subagent isolation
Token / Context tracking
Heavy Worker policy
CI/CD review
```

この仕様により、RenCrow は「AI に作業を頼む」段階から、「AI が働ける開発環境を運用する」段階へ進む。

これは、Coder / Worker / DCI / Tool Harness の効果を最大化するための土台である。

