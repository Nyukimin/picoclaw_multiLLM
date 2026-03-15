# IdleChat 仕様書

**作成日**: 2026-03-15
**対象バージョン**: v4 (distributed mode)
**ステータス**: 実装完了・運用中

---

## 1. 概要

IdleChat は、ユーザーが一定時間操作しないアイドル時間に **エージェント同士（Mio / Shiro 等）が自律的に雑談する** 仕組みである。

### 1.1 目的

- アイドル時間を活用してエージェントの「人格」を表現する
- ユーザーに楽しめるコンテンツ（雑談・架空映画妄想）を自動生成する
- Viewer / TTS 経由でリアルタイム表示・読み上げする

### 1.2 設計思想

- **本番タスク最優先**: ユーザーアクティビティで即中断
- **品質制御**: 4段階のリトライ + 5種類のループ検出で会話品質を維持
- **多様性確保**: 260ジャンル + 外部シード + 映画モードでトピック枯渇を防止
- **話者ごとの LLM 分離**: `speakerLLMs` で Mio と Shiro に異なる LLM を割当可能

---

## 2. アーキテクチャ

### 2.1 コンポーネント構成

```
internal/application/idlechat/
├── orchestrator.go       # IdleChatOrchestrator 本体（ライフサイクル・発話生成・ループ検出）
├── orchestrator_test.go  # テスト
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

### 2.3 データフロー全体図

```
IdleChatOrchestrator
  │
  │ emitTimelineEvent(TimelineEvent)
  │     ↓
  │ SetEventEmitter (main.go)
  │     ↓  TimelineEvent → OrchestratorEvent に変換
  │
  EventHub.OnEvent()
  │     ↓  JSON シリアライズ → broadcast
  │
  SSE (Server-Sent Events) /viewer/sse
  │     ↓
  viewer.html (ブラウザ JavaScript)
      ↓
  Timeline 表示 + IdleChat パネル更新
```

---

## 3. ライフサイクル

### 3.1 起動〜監視

```
Start()
  ├─ fetchDailySeeds()       ← 非同期: Wikipedia/NHK シード取得
  └─ goroutine: monitorLoop()
       └─ 30秒ごとに checkAndStartChat()
```

### 3.2 セッション開始条件 (`checkAndStartChat`)

以下の **全条件** を満たしたとき `runChatSession()` を開始する:

| 条件 | 説明 |
|---|---|
| `!chatActive` | 既にセッション進行中でない |
| `!chatBusy` | Chat (Mio) が処理中でない |
| `!workerBusy` | Worker (Shiro/Coder) が処理中でない |
| `now >= nextTopicAt` | クールダウン期間が経過している |
| アイドル時間 ≥ `intervalMin` **または** `manualMode` | 自動モード: 一定時間アイドル / 手動モード: 即時 |

### 3.3 セッション中断条件

以下のいずれかで `chatActive = false` → ターンループが break する:

- `NotifyActivity()`: ユーザーメッセージ到着
- `SetChatBusy(true)`: Chat (Mio) が処理開始
- `SetWorkerBusy(true)`: Worker が処理開始
- `StopManualMode()`: 手動モード停止
- `ctx.Done()`: アプリケーション終了

### 3.4 停止

```
Stop()
  ├─ cancel()   ← context キャンセル
  └─ wg.Wait()  ← goroutine 終了待機
```

---

## 4. セッション実行 (`runChatSession`)

### 4.1 フロー

```
1. generateTopicFromChat() → トピック生成（3戦略から選択）
2. ターンループ（残りターン数分、1トピックあたり最大 maxTurnsPerTopic=12）
   ├─ speaker / nextSpeaker を交互に切り替え
   ├─ generateResponse() → LLM で発話生成
   ├─ isResponseTooSimilar() → emit 前の類似度チェック
   ├─ CentralMemory に記録 + TimelineEvent emit
   ├─ waitForTTS() → TTS 読み上げ待機
   ├─ maxTurnsPerTopic チェック
   ├─ detectLoopReason() → ループ検出
   └─ 中断 / エラー / ループ検知 → break
3. saveSummary() → Worker (Shiro) で要約生成、TopicStore に永続化
4. nextTopicAt 設定（クールダウン）
```

### 4.2 話者の決定

- `chatSpeakerIndex()`: `"mio"` を最初の話者に選択
- 以降は `participants` 配列のインデックスを `+1 % len` で交互に切り替え
- 各話者に対して `providerForSpeaker(name)` で個別 LLM を選択可能

### 4.3 クールダウン

| 終了理由 | クールダウン |
|---|---|
| 正常終了 / ループ検出 | `minTopicInterval` (10秒) |
| セッション中断 / 生成エラー | `max(minTopicInterval, intervalMin分)` |

---

## 5. トピック生成

### 5.1 戦略 (`TopicStrategy`)

| 戦略 | 確率 | 内容 |
|---|---|---|
| `StrategySingleGenre` | 40% | 260個のジャンルプールから1個選び深掘り |
| `StrategyDoubleGenre` | 30% | 2ジャンルの意外な掛け合わせ |
| `StrategyExternalStimulus` | 30% | Wikipedia Random / NHK News RSS + ジャンル |

### 5.2 映画モード

- 20% の確率で有効化
- トピック形式: `「{タイトル}」ってどんな映画？`
- 架空映画の妄想会話（実在作品名の使用禁止）

### 5.3 ジャンルプール（260個）

| カテゴリ | 数 | 例 |
|---|---|---|
| 学問・研究分野 | 30 | 昆虫学, RNA生物学, 地理学 |
| 自然・環境・地理 | 25 | 火山活動, 氷河, サンゴ礁 |
| 生物・生命 | 25 | 共生, 擬態, 冬眠 |
| 文化・芸術・伝統 | 25 | 茶道, 歌舞伎, 落語 |
| 音楽・芸能 | 15 | 交響詩, ジャズ, 即興 |
| 社会制度・システム | 20 | 刑務所制度, 選挙, 通貨 |
| 技術・工学 | 20 | RNA治療, 発酵技術, 印刷 |
| 日常・生活 | 20 | 睡眠, 料理, 散歩 |
| 抽象概念・感情 | 20 | 記憶, 忘却, 郷愁 |
| 時間・周期・暦 | 15 | 満月, 日食, 春分 |
| 物質・現象 | 15 | 蒸発, 燃焼, 共鳴 |
| 空間・場所・建築 | 20 | 橋, 灯台, 路地 |
| 道具・機構 | 15 | 歯車, バネ, レンズ |
| 記号・表現・伝達 | 15 | 文字, 暗号, 比喩 |
| 遊び・娯楽 | 10 | 将棋, 麻雀, けん玉 |
| その他・カオス | 10 | 噂, 迷信, 都市伝説 |

### 5.4 外部シード

起動時に1日1回取得しキャッシュ (`DailySeedCache`):

| ソース | 取得数 | API |
|---|---|---|
| Wikipedia Random | 10件 | `ja.wikipedia.org/w/api.php?action=query&list=random` |
| NHK News RSS | 10件 | `www.nhk.or.jp/rss/news/cat0.xml` |

### 5.5 語彙メモ連携

`recentTopics` プロバイダ（glossary）から最近の時事語彙を取得し、トピック生成プロンプトと発話プロンプトの両方に注入する。詳細断言ではなく発想補助として使用。

### 5.6 重複排除

- 直近12トピックと類似度チェック (`topicTooSimilar`)
- 最大3回リトライ（温度を段階的に上げる: 0.9, 0.95, 1.0）
- 全て類似 → フォールバックトピックを使用

### 5.7 禁止キーワード

`AI`, `タイムマシン`, `過去`, `未来`, `宇宙人`, `もし`, `だったら`, `なら`, `想像`, `考えて`

---

## 6. 発話生成 (`generateResponse`)

### 6.1 プロンプト構成

```
[system] 話者のペルソナ (getSystemPrompt)
[user/assistant] セッション内の直近4発話（role を from/speaker で判定）
[user] 発言帰属ガード (buildIdleResponseGuardPrompt)
[system] 語彙メモ（あれば）
[user] 映画モード指示（該当時）
[user] ターンプロンプト (buildIdleTurnPrompt)
```

### 6.2 LLM パラメータ

| パラメータ | 値 |
|---|---|
| `MaxTokens` | 160 |
| `Temperature` | mio/shiro: 0.65, その他: 設定値 |

### 6.3 4段階リトライ

生成結果に問題がある場合、段階的にリトライする:

| 段階 | 条件 | リトライプロンプト |
|---|---|---|
| 1. 無効応答 | `invalidIdleResponse(raw)` | 「記号だけや空文をやめて、自然な会話文を1-2文で言い直してください」 |
| 2. スタイル問題 | `needsIdleStyleRetry(...)` | 「評価や言い直し宣言は書かず、別の手で自然に返してください」 |
| 3. プロンプト漏出 | `hasPromptLeak(text)` | 「指示文の断片を消して、自然な会話文だけを1-2文で言い直してください」 |
| 4. 発言帰属違反 | `violatesAttribution(text, other)` | 「発言帰属が曖昧です。相手の案を受ける形にして、1-2文で言い直してください」 |

### 6.4 サニタイズ

`sanitizeIdleResponse()`: 生成結果からトピック文字列の反復や不要なメタ表現を除去。

---

## 7. ループ検出

### 7.1 検出タイミング

- **emit 前**: `isResponseTooSimilar()` — transcript に追加する前に検出
- **emit 後**: `detectLoopReason()` — 6ターン以上の transcript に対して毎ターン検出

### 7.2 検出パターン（5種類）

| 種別 | 関数 | 条件 |
|---|---|---|
| `exact_repeat` | `detectLoopReason` | 直近4発話内に正規化後の完全一致がある |
| `alternating_repeat` | `hasAlternatingLoop` | A-B-A-B パターン（8ターン以上、類似度 ≥ 0.9） |
| `template_repeat` | `hasSpeakerTemplateLoop` | 話者テンプレートの繰り返し |
| `high_similarity` | `hasHighSimilarityLoop` | 直近10発話の類似度が全体的に高い |
| `what_if_repeat` | `isWhatIfRepetition` | 「もし〜だったら/なら」が直近8発話の半数以上 |

### 7.3 ループ検出後の動作

1. ターンループを break
2. `saveSummary()` で要約を生成（`annotateLoopSummary` で注記を付与）
3. クールダウン設定 → 次のトピックへ

### 7.4 ループ理由ラベル

| reason | ラベル |
|---|---|
| `template_repeat` | テンプレ反復で打ち切り |
| `alternating_repeat` | 交互反復で打ち切り |
| `exact_repeat` / `high_similarity` / `pre_emit_similarity` | 類似発話の反復で打ち切り |
| `what_if_repeat` | 仮定表現の反復で打ち切り |
| `topic_turn_limit` | (ラベルなし — 正常なトピック切替) |
| `interrupted` | 中断で終了 |
| `generation_error` | 生成エラーで終了 |
| `invalid_response` | 返答崩れで終了 |

---

## 8. 要約・永続化

### 8.1 要約生成 (`summarizeByWorker`)

- Worker (Shiro) の LLM で生成
- プロンプト: 「1. いちばん面白かった点 2. 何が話を前に進めたか 3. 次に広がりそうな観点」
- `MaxTokens: 800`, `Temperature: 0.4`
- エラー時はトランスクリプトの先頭200文字をフォールバック

### 8.2 SessionSummary

```go
type SessionSummary struct {
    SessionID       string        // "idle-{unix}"
    Title           string        // "3月15日の{topic}の話題まとめ"
    Topic           string        // トピック文字列
    Strategy        TopicStrategy // single / double / external
    Summary         string        // Shiro による要約
    StartedAt       string        // RFC3339
    EndedAt         string        // RFC3339
    Turns           int           // 実行ターン数
    LoopRestarted   bool          // ループ/中断/エラーで終了したか
    LoopReason      string        // 終了理由
    TopicProvider   string        // "mio"
    SummaryProvider string        // "shiro"
    Transcript      []string      // "{speaker}: {content}" の配列
}
```

### 8.3 TopicStore (`topic_store.go`)

- ファイルパス: `{session.storage_dir}/idlechat_topics.jsonl`
- 形式: JSON Lines（1行1セッション）
- メモリキャッシュ: 直近 `maxStoreCache` 件
- `Append()`: ファイル末尾に追記
- `GetRecent(limit)`: 新しい順で返却

### 8.4 CentralMemory への記録

- 各発話: `MessageTypeIdleChat`, from=speaker, to=nextSpeaker
- トピック通知: from="user", to="mio"
- 要約: from="shiro", to="idlechat_summary"

---

## 9. Viewer 連携

### 9.1 イベント変換ブリッジ

`main.go` の `SetEventEmitter` で `TimelineEvent` → `OrchestratorEvent` に変換:

```go
idleChatOrch.SetEventEmitter(func(ev idlechat.TimelineEvent) {
    deps.eventHub.OnEvent(orchestrator.NewEvent(
        ev.Type,       // "idlechat.message" / "idlechat.summary"
        ev.From, ev.To, ev.Content,
        "IDLECHAT",    // route 固定
        "",            // jobID
        ev.SessionID,  // "idle-{unix}"
        "idlechat",    // channel
        "idlechat",    // chatID
    ))
    emitIdleChatTTSAsync(ttsBridge, ev)  // TTS 非同期発火
})
```

### 9.2 双方向制御（中断ブリッジ）

`idleAwareEventListener`:

- **IdleChat → Viewer**: 発話/要約イベント → EventHub → SSE → ブラウザ表示
- **Viewer → IdleChat**: ユーザーメッセージ送信 → `message.received` イベント → `shouldStopIdleChatByEvent()` → `NotifyActivity()` → セッション中断

`shouldStopIdleChatByEvent()` の判定:
- route が `IDLECHAT` → 無視（自分自身のイベントでは中断しない）
- `tts.audio_chunk` / from=`tts` → 無視
- `message.received` → **中断トリガー**

### 9.3 REST API エンドポイント

| エンドポイント | メソッド | 用途 | レスポンス |
|---|---|---|---|
| `/viewer/idlechat/start` | POST | 手動モード開始 | `{ok, manual_mode, chat_active, current_topic}` |
| `/viewer/idlechat/stop` | POST | 手動モード停止 | `{ok, manual_mode, chat_active, current_topic}` |
| `/viewer/idlechat/status` | GET | 状態取得 | `{ok, manual_mode, chat_active, current_topic}` |
| `/viewer/idlechat/logs` | GET | 履歴取得 | `{ok, manual_mode, chat_active, current_topic, history[]}` |

`/logs` の `limit` パラメータ: クエリ文字列 `?limit=N` (1-200, デフォルト20)

### 9.4 フロントエンド（viewer.html）

#### UI 構成

**入力バーのコントロール:**
- 「IdleChat開始」ボタン → `POST /viewer/idlechat/start`
- 「IdleChat停止」ボタン → `POST /viewer/idlechat/stop`
- 状態表示: `IdleChat: off` / `IdleChat: on` / `IdleChat: on (talking)`

**IdleChat パネル（タブ切替）:**

| セクション | 内容 |
|---|---|
| Mode カード | Manual (on/off)、Chat Active (on/off) のバッジ表示 |
| Current Topic | 進行中のトピック |
| 履歴テーブル | Title, Strategy, Topic, Turns, Loop Restart, Started, Ended, Summary |

**展開可能なトランスクリプト:**
- 各履歴行をクリックで展開 → チャットバブル形式で表示
- 話者名を色分け表示（エージェント情報の色・アイコン）
- 「Copy Chat」ボタンでテキスト形式コピー

#### JavaScript 関数

| 関数 | 役割 |
|---|---|
| `renderIdleChat()` | `state.idleChat` からパネル全体を DOM 再構築 |
| `refreshIdleStatus()` | `/idlechat/status` を fetch → ボタン有効/無効 + 状態更新 |
| `refreshIdleLogs()` | `/idlechat/logs?limit=20` を fetch → history 取得 → 再描画 |
| `controlIdle(path)` | start/stop ボタン → POST → `refreshIdleStatus()` |
| `setIdleState(manual, active)` | 状態テキスト + CSS クラス更新 |
| `formatIdleChatTranscript(row)` | コピー用テキスト整形 |

#### SSE イベント受信

メインタイムラインの `addMsgToTimeline(ev)` が `idlechat.message` / `idlechat.summary` タイプのイベントも処理し、リアルタイムでタイムラインビューに表示する。

---

## 10. 定数・設定値

### 10.1 ハードコード定数

| 定数 | 値 | 用途 |
|---|---|---|
| `idleCheckInterval` | 30秒 | monitorLoop のチェック間隔 |
| `minTopicInterval` | 10秒 | トピック切替の最小クールダウン |
| `ttsCharsPerSecond` | 8.0 | TTS 待機時間の計算基準 |
| `ttsMinWait` | 2秒 | TTS 最小待機 |
| `ttsMaxWait` | 20秒 | TTS 最大待機 |
| `maxTurnsPerTopic` | 12 | 1トピックあたりの最大ターン数 |

### 10.2 設定値（config.yaml）

| フィールド | 説明 |
|---|---|
| `idle_chat.participants` | 参加者リスト (例: `["mio", "shiro"]`) |
| `idle_chat.interval_min` | アイドル判定閾値（分） |
| `idle_chat.max_turns` | セッション全体の最大ターン数 |
| `idle_chat.temperature` | デフォルト温度（mio/shiro は 0.65 固定） |
| `idle_chat.personalities` | 話者ごとのペルソナ定義 |
| `session.storage_dir` | TopicStore の保存先ディレクトリ |

---

## 11. 並行安全性

| 機構 | 用途 |
|---|---|
| `sync.Mutex` (`o.mu`) | 全フィールド（状態フラグ・history・emitEvent 等）の排他制御 |
| `context.Context` (`o.ctx`) | goroutine のキャンセル伝播 |
| `sync.WaitGroup` (`o.wg`) | `Stop()` での goroutine 終了待機 |
| `sync.RWMutex` (`cacheMu`) | DailySeedCache のスレッドセーフなアクセス |

- `monitorLoop` は単一 goroutine で動作
- `checkAndStartChat` → `runChatSession` は同期実行（同時に1セッションのみ）
- 外部からの中断（`NotifyActivity` 等）は mutex 経由で `chatActive` フラグを操作

---

## 12. ファイル一覧

| ファイル | 役割 |
|---|---|
| `internal/application/idlechat/orchestrator.go` | IdleChatOrchestrator 本体 |
| `internal/application/idlechat/topic_generator.go` | トピック生成戦略・外部シード取得 |
| `internal/application/idlechat/topic_store.go` | TopicStore（JSON Lines 永続化） |
| `internal/application/idlechat/orchestrator_test.go` | テスト |
| `internal/adapter/viewer/viewer.html` | Viewer フロントエンド（IdleChat パネル含む） |
| `internal/adapter/viewer/hub.go` | EventHub（SSE ブロードキャスト） |
| `internal/adapter/viewer/handler.go` | SSE ハンドラ・ページ配信 |
| `cmd/picoclaw/main.go` | IdleChat 初期化・API 登録・イベントブリッジ |
