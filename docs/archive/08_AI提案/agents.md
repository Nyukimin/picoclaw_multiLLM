# agents.md

## 0. 原則
- 役割は3つで固定：Chat / Worker / Coder
- Spawnは禁止：新規エージェント生成はしない
- Subagentはテンプレ（モード）としてWorker内部で扱う

## 1. Agents（役割定義）

### 1.1 Chat
LLM
- Qwen3-vl（local）

責務
- 会話の受け取り、意図の確定（目的・制約・優先度）
- 画像/スクショの一次読み取り
- Workerへ渡す依頼書（envelope）を作る
- 例外的に、即答（超短い定義・Yes/No・30秒手順）はChatで完結してよい

基本設定
- delegate_to_worker = true（原則）
- tool_use = minimal（原則）

### 1.2 Worker
LLM
- Qwen2.5（local）

責務
- 収集→分析→サマリ→次の一手（PicoClawループの主導）
- タスク分解（Subagentテンプレでモード切替）
- ルーティング（Coder呼び分け、DeepSeek→ChatGPT昇格）
- Coder出力の検品（acceptanceで合否判定）
- Chatへ返す最終骨子（短く、根拠と不確実性を分離）

推奨モード（Subagentテンプレ）
- research / factcheck / extract / plan / qa

### 1.3 Coder
実行先（プロバイダ）
- primary：DeepSeek（web）
- fallback：ChatGPT（web）

責務
- Workerの implement 依頼に従って成果物生成
  - コード差分、設定ファイル、手順、テスト観点
- 仕様不足は「不足」として返す
- スコープ逸脱をしない

## 2. メッセージ契約（Envelope）
すべての処理単位に EventId を付与する。

推奨フォーマット
```json
{
  "event_id": "E-YYYYMMDD-000123",
  "from": "chat|worker|coder",
  "to": "worker|coder|chat",
  "intent": "research|analyze|implement|summarize|qa",
  "goal": "目的（1文）",
  "constraints": ["禁止事項", "前提条件", "優先順位"],
  "inputs": {
    "text": "入力テキスト",
    "images": ["optional"],
    "files": ["optional"],
    "logs": ["optional"]
  },
  "expected_output": "出力の型（短く）",
  "acceptance": ["受け入れ条件（必要なら）"],
  "risk_notes": ["不確実性・注意点（必要なら）"]
}
```

## 3. 標準フロー
1) Chat：ユーザー入力→依頼書→Workerへ  
2) Worker：分解→（必要なら収集）→分析→サマリ→次の一手  
3) Worker：必要なら Coder（既定DeepSeek、条件でChatGPTへ昇格）  
4) Coder：成果物→Workerへ  
5) Worker：検品→要点整理→Chatへ  
6) Chat：ユーザーへ返答

## 4. ガード（固定）
- spawn_enabled = false
- max_agents = 3（Chat/Worker/Coder）
- 外部送信前の秘密情報マスク（キー/トークン/個人情報）
- Coder出力は必ずWorkerが検品してから外へ出す
