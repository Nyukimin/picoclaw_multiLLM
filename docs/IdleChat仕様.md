# IdleChat 仕様書

**作成日**: 2026-03-15
**最終更新**: 2026-03-15
**対象バージョン**: v4 (distributed mode)
**ステータス**: 実装完了・運用中

---

## 1. 概要

IdleChat は、ユーザーが一定時間操作しないアイドル時間に **エージェント同士（Mio / Shiro 等）が自律的に雑談する** 仕組みである。通常モード（ランダムトピック）と未来展望モード（ドメイン特化）の2つのセッション形式を持つ。

### 1.1 目的

- アイドル時間を活用してエージェントの「人格」を表現する
- ユーザーに楽しめるコンテンツ（雑談・架空映画妄想・未来展望）を自動生成する
- Viewer / TTS 経由でリアルタイム表示・読み上げする

### 1.2 セッション形式

| 項目 | 通常モード | 未来展望モード |
|---|---|---|
| トピック選択 | ランダム（260ジャンル + 外部シード） | 6ドメイン固定順回し |
| 情報源 | NHK RSS + Wikipedia | トレンド + NHK + Google News（3段階） |
| ターン数 | 12ターン/トピック、最大50/セッション | **100ターン/ドメイン、最大600/セッション** |
| 起動方法 | 自動（アイドル検知）/ 手動 | **手動のみ**（「未来展望」ボタン） |
| セッション形式 | 単発トピック | 番組形式（ドメインアナウンス → お題 → 議論） |
| テーマ反復抑制 | ループ検出（5種類） | ループ検出 + 蓄積型テーマ抑制 |
| 要約 | Worker (Shiro) → Mio 読み上げ | Worker (Shiro) + 継続考察テーマ → Mio 読み上げ |
| Strategy 表示 | `single: トピック名`, `double: ...`, `external: ...` | `forecast/AI技術`, `forecast/経済` 等 |
| 詳細仕様 | 本ドキュメント | `docs/未来展望セッション仕様.md` |

### 1.3 設計思想

- **本番タスク最優先**: ユーザーアクティビティで即中断
- **イベントドリブン TTS**: TTS 完了イベントで次のアクションに進む（推定ベースではない）
- **品質制御**: 4段階のリトライ + 5種類のループ検出で会話品質を維持
- **多様性確保**: 260ジャンル + 外部シード + 映画モードでトピック枯渇を防止
- **話者ごとの LLM 分離**: `speakerLLMs` で Mio と Shiro に異なる LLM を割当可能

---

## 2. アーキテクチャ

### 2.1 コンポーネント構成

```
internal/application/idlechat/
├── orchestrator.go       # IdleChatOrchestrator 本体（ライフサイクル・発話生成・ループ検出・TTS連携）
├── orchestrator_test.go  # テスト
├── forecast_session.go   # 未来展望セッション（ドメイン定義・トレンド収集・テーマ抑制）
├── topic_generator.go    # トピック生成戦略・外部シード取得
└── topic_store.go        # TopicStore（セッション要約の永続化）
```

### 2.2 依存関係

| 依存先 | 用途 |
|---|---|
| `domain/llm.LLMProvider` | LLM 呼び出し（トピック生成・発話生成・要約） |
| `domain/session.CentralMemory` | 会話履歴の記録・参照 |
| `domain/transport.Message` | メッセージ型（`MessageTypeIdleChat`） |
| `adapter/viewer.EventHub` | Viewer SSE へのイベントブロードキャスト |

### 2.3 LLM 役割分担

| 処理 | 担当 | 理由 |
|---|---|---|
| 通常トピック生成 | Mio (gemma3:4b) | 軽量・高速 |
| 未来展望キーワード抽出 | Coder2 (GPT) | 深い文脈理解が必要 |
| 未来展望トピック生成 | Coder2 (GPT) | 未来展望の質が重要 |
| ディスカッション発話 | 各話者の LLM | ペルソナ維持 |
| 既出テーマ抽出 | Worker (Shiro/qwen3.5:9b) | 要約タスク、ローカル無料 |
| まとめ生成 | Worker (Shiro/qwen3.5:9b) | 要約タスク、ローカル無料 |

### 2.4 データフロー

```
IdleChatOrchestrator
  │
  │ emitTimelineEvent(TimelineEvent) → <-chan struct{} (TTS完了チャネル)
  │     ↓
  │ SetEventEmitter (main.go)
  │     ↓  TimelineEvent → OrchestratorEvent 変換 + TTS非同期発火
  │
  EventHub.OnEvent()
  │     ↓  JSON → SSE broadcast
  │
  viewer.html (Timeline + IdleChat パネル)
```

---

## 3. ブレイク体系

全モード共通。TTS イベントドリブン。

| タイミング | 待ち時間 | 起点 |
|---|---|---|
| 同一話者内の句間 | 200ms | TTS チャンクの `pause_after`（デフォルト） |
| 話者交代（Mio↔Shiro） | 500ms | TTS 完了イベント後 (`speakerBreak`) |
| トピック/ドメイン交代 | 1000ms | TTS 完了イベント後 (`topicBreak`) |

**TTS 完了待ちの仕組み**:
- `emitTimelineEvent()` が `<-chan struct{}` を返す
- TTS Server の `OnSessionCompleted` → `notifyIdleChatTTSCompleted` → チャネル close
- `waitForTTSDone(ch)` で待機。TTS 未設定なら `nil` が返り即座にスキップ

---

## 4. 通常モード

### 4.1 ライフサイクル

```
Start()
  └─ goroutine: monitorLoop() — 30秒ごとに checkAndStartChat()
       ├─ chatBusy/workerBusy → スキップ
       ├─ nextTopicAt 前 → スキップ
       ├─ アイドル時間 < intervalMin（manualMode でなければ）→ スキップ
       └─ runChatSession()
```

### 4.2 トピック生成戦略

| 戦略 | 確率 | 内容 |
|---|---|---|
| `StrategySingleGenre` | 40% | 260個のジャンルプールから1個選び深掘り |
| `StrategyDoubleGenre` | 30% | 2ジャンルの意外な掛け合わせ |
| `StrategyExternalStimulus` | 30% | Wikipedia Random / NHK News RSS + ジャンル |

- **映画モード**: 20% の確率で `「〜ってどんな映画？」` 形式
- **外部シード**: 起動時に1日1回取得（Wikipedia 10件、NHK 10件）
- **重複排除**: 直近12トピックと類似度チェック、最大3回リトライ

### 4.3 セッション実行

```
runChatSession():
  1. generateTopicFromChat() → トピック生成
  2. ターンループ（最大 maxTurnsPerTopic=12）
     ├─ generateResponse() → 発話生成
     ├─ ensureTrailingPeriod() → 末尾に「。」追記
     ├─ emit → waitForTTSDone → waitBreak(speakerBreak)
     ├─ ループ検出（detectLoopReason）
     └─ 中断/エラー/ループ → break
  3. saveSummary() → Worker 要約 → Mio 読み上げ → topicBreak
```

### 4.4 ループ検出（5種類）

| 種別 | 条件 |
|---|---|
| `exact_repeat` | 直近4発話内に完全一致 |
| `alternating_repeat` | A-B-A-B パターン（類似度 ≥ 0.9） |
| `template_repeat` | 話者テンプレートの繰り返し |
| `high_similarity` | 直近10発話の類似度が高い |
| `what_if_repeat` | 「もし〜だったら/なら」が半数以上 |

### 4.5 発話生成リトライ（4段階）

| 段階 | 条件 | リトライ内容 |
|---|---|---|
| 1. 無効応答 | `invalidIdleResponse` | 「自然な会話文で言い直して」 |
| 2. スタイル問題 | `needsIdleStyleRetry` | 「別の手で自然に返して」 |
| 3. プロンプト漏出 | `hasPromptLeak` | 「指示文の断片を消して」 |
| 4. 発言帰属違反 | `violatesAttribution` | 「相手の案を受ける形に」 |

---

## 5. 要約と読み上げ

全モード共通。トピック/ドメインの議論終了後:

```
1. saveSummary / saveForecastSummary → Worker (Shiro) が要約生成
2. TopicStore に永続化（JSON Lines）
3. Timeline に idlechat.summary イベント emit
4. speakSummary() → Mio が要約を読み上げ（TTS完了待ち）
5. topicBreak (1000ms) → 次のトピック/ドメインへ
```

### 5.1 SessionSummary

```go
type SessionSummary struct {
    SessionID       string        // "idle-{unix}" or "forecast-{unix}"
    Title           string        // "3月15日の{topic}の話題まとめ"
    Topic           string        // トピック文字列
    Strategy        TopicStrategy // "single: ...", "forecast/AI技術" 等
    Summary         string        // Worker による要約
    StartedAt       string        // RFC3339
    EndedAt         string        // RFC3339
    Turns           int
    LoopRestarted   bool
    LoopReason      string
    TopicProvider   string        // "mio" or "forecast"
    SummaryProvider string        // "shiro" or "coder2"
    Transcript      []string      // "{speaker}: {content}"
}
```

---

## 6. Viewer 連携

### 6.1 REST API

| エンドポイント | メソッド | 用途 |
|---|---|---|
| `/viewer/idlechat/start` | POST | 通常モード手動開始 |
| `/viewer/idlechat/forecast` | POST | 未来展望モード開始 |
| `/viewer/idlechat/stop` | POST | 停止（両モード共通） |
| `/viewer/idlechat/status` | GET | 状態取得 |
| `/viewer/idlechat/logs` | GET | 履歴取得（両モード統合） |

### 6.2 Viewer UI

**入力バーのコントロール:**
- 「IdleChat開始」「IdleChat停止」ボタン — 通常モード
- 「未来展望」ボタン（青系、独立配置） — 未来展望モード
- 状態表示: `IdleChat: off` / `on` / `on (talking)`

**IdleChat パネル（タブ切替）:**
- Mode カード: Manual / Chat Active バッジ
- Current Topic: 進行中トピック
- 履歴テーブル: Title, Strategy, Topic, Turns, Loop, Started, Ended, Summary
  - forecast 行は左ボーダー青 + Strategy 列青色で視覚区別

**Timeline:**
- `idlechat.message` / `idlechat.summary` イベントがリアルタイム表示
- ルート色: `IDLECHAT` = 紫

### 6.3 双方向制御

```
IdleChat → Viewer: 発話/要約イベント → EventHub → SSE → ブラウザ表示
Viewer → IdleChat: ユーザーメッセージ → message.received → NotifyActivity() → 中断
```

`shouldStopIdleChatByEvent()`: route が `IDLECHAT` や TTS イベントは無視、`message.received` のみ中断トリガー。

---

## 7. 並行安全性

| 機構 | 用途 |
|---|---|
| `sync.Mutex` (`o.mu`) | 全フィールドの排他制御 |
| `context.Context` (`o.ctx`) | goroutine キャンセル伝播 |
| `sync.WaitGroup` (`o.wg`) | Stop() での終了待機 |
| `sync.RWMutex` (`cacheMu`) | DailySeedCache のスレッドセーフアクセス |
| `sync.RWMutex` (`trendMu`) | TrendCache のスレッドセーフアクセス |

---

## 8. 定数

| 定数 | 値 | 用途 |
|---|---|---|
| `idleCheckInterval` | 30s | monitorLoop チェック間隔 |
| `maxTurnsPerTopic` | 12 | 通常モードの1トピック最大ターン数 |
| `speakerBreak` | 500ms | 話者交代ブレイク |
| `topicBreak` | 1000ms | トピック/ドメイン交代ブレイク |
| `defaultChunkPause` | 200ms | TTS チャンク間ブレイク（audio_sink） |
| `forecastTurnsPerDomain` | 100 | 未来展望の1ドメイン最大ターン数 |
| `forecastCheckpointInterval` | 15 | 未来展望のテーマ抑制チェック間隔 |

---

## 9. ファイル一覧

| ファイル | 責務 |
|---|---|
| `internal/application/idlechat/orchestrator.go` | IdleChatOrchestrator 本体、発話生成、ループ検出、TTS連携、sessionContext |
| `internal/application/idlechat/forecast_session.go` | 未来展望セッション、トレンド収集、テーマ抑制、ドメイン定義 |
| `internal/application/idlechat/topic_generator.go` | 通常モードのトピック戦略、外部シード、RSS パーサ |
| `internal/application/idlechat/topic_store.go` | TopicStore（JSON Lines 永続化） |
| `internal/infrastructure/tts/audio_sink.go` | TTS チャンク再生 + 句間ブレイク（`pause_after` / `defaultChunkPause`） |
| `internal/adapter/viewer/viewer.html` | Viewer フロントエンド |
| `internal/adapter/viewer/hub.go` | EventHub（SSE ブロードキャスト） |
| `cmd/picoclaw/main.go` | 初期化・API 登録・イベントブリッジ・TTS 連携 |
| `cmd/picoclaw/idlechat_tts.go` | IdleChat TTS 非同期発火・完了通知 |
