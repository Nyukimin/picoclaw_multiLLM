# PicoClaw 3役固定・Spawn禁止構成 仕様（統合版）

## 0. 目的
Chat / Worker / Coder の3役固定（Spawn禁止）で、PicoClawの標準ループ（収集→分析→サマリ→次の一手）を安定運用する。  
外部LLMは「エージェント増殖」ではなく「Coderの実行先（プロバイダ）切替」として扱う。

## 1. 固定のLLM分別（決定事項）
- Chat = Qwen3-vl（local）
- Worker = Qwen2.5（local）
- Coder1 = DeepSeek（web）
- Coder2 = ChatGPT（web）

## 2. 全体アーキテクチャ（PicoClaw標準に寄せた形）
- エージェント（役割）は3つで固定：Chat / Worker / Coder
- Coderは「1エージェント」だが、実行先を2段ギアで持つ
  - 既定：Coder1（DeepSeek）
  - 昇格：Coder2（ChatGPT）
- Spawnは禁止（新規エージェント生成・動的増殖はしない）

## 3. 役割分担（境界の固定）

### 3.1 Chat（窓口・軽作業）
責務
- 会話の受け取りと意図の確定（目的・制約・優先度）
- 画像/スクショの一次読み取り（何が写っているか、どこが問題か）
- Workerに渡す「依頼書」を作る（入力を整形して渡す）
- 例外的に、30秒で終わる即答（定義説明・Yes/No・超短い手順）だけはChatで返す

禁止/非推奨
- 調査の実行、設計の確定、実装の判断、外部LLMの呼び分け（これはWorker側）

基本方針
- delegate_to_worker = true を原則（例外のみ直答）

### 3.2 Worker（司令塔）
責務
- PicoClawループの主導：収集→分析→サマリ→次の一手
- タスク分解（Subagentテンプレを用いたモード切替で実現）
- ルーティング判断（Coder1→Coder2昇格含む）
- Coder出力の検品（受け入れ条件で合否判定）
- 最終的なユーザー返答の骨子生成（Chatへ渡す）

### 3.3 Coder（成果物生成）
責務
- Workerの指示（ゴール・制約・受け入れ条件）に従い、成果物を生成
  - 例：コード差分、設定ファイル、手順、テスト観点
- 不足がある場合は「不足」と明示して返す（勝手に仕様を膨らませない）

## 4. Subagent（Spawnなしで分業する仕組み）
Subagentは「別プロセス」ではなく「テンプレ（モード切替）」としてWorker内部で扱う。

推奨モード（例）
- research：情報収集（Web/ログ/既存データ）
- factcheck：根拠確認、矛盾検出
- extract：要点抽出、構造化
- plan：手順化、受け入れ条件の作成
- qa：自己点検（仕様逸脱、穴、危険な推測）

## 5. ルーティング（割り振り）担当と規約
担当
- ルーティングは Worker（またはWorker直前のRouterコード）が行う

原則
- まず Coder1（DeepSeek）
- 次条件で Coder2（ChatGPT）へ昇格
  - Coder1の連続失敗（例：2回）
  - 原因切り分けが必要（再現性低いバグ、環境差、複雑な依存）
  - 大きい差分/複数ファイル跨ぎの改修
  - 設計判断・リファクタを伴う

運用の必須ガード
- 外部Coderへ送る情報は「必要最小限」
- APIキー/秘密情報/個人情報は送らない（必ずマスク）
- Coder出力は必ずWorkerが受け入れ条件で検品してからChatへ渡す

## 6. Spawn禁止の“破れない”担保
二重ロック
- 設定で禁止：spawn_enabled=false、max_agents=3、固定登録以外ロードしない
- 実行時ガード：spawn要求/新規エージェント生成命令を検出したらRunnerが拒否し、Workerはテンプレ分解へ誘導

## 7. メッセージ契約（依頼書フォーマット）
Chat→Worker、Worker→Coder、Coder→Worker の受け渡しは共通封筒で行う。  
すべての処理単位に EventId を付与する。

推奨封筒（例）
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

## 8. 標準フロー（固定）
1) Chat：ユーザー入力→意図整理→依頼書作成→Workerへ  
2) Worker：分解→必要なら収集→分析→サマリ→次の一手  
3) Worker：実装が必要なら Coder を呼ぶ（既定DeepSeek、必要ならChatGPTへ昇格）  
4) Coder：成果物生成→Workerへ返す  
5) Worker：受け入れ条件で検品→要点整理→Chatへ返す  
6) Chat：ユーザーへ返答（必要最小限の説明で）

## 9. 失敗時の扱い（最小ルール）
- 仕様不足：Coderは「不足」を返す→Workerが不足点を依頼書に追記
- 連続失敗：失敗カウンタをEventId系列で管理し、閾値で昇格
- 不確実性：推測は推測と明示し、根拠/未確認点を残す
