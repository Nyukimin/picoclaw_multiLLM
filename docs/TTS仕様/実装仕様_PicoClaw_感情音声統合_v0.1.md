# 実装仕様: RenCrow 感情音声統合 v0.1

**作成日**: 2026-03-11  
**対象ブランチ**: `integration/openclaw-parity`  
**正本参照**:
- `docs/TTS仕様/Emotion Planner のアルゴリズム仕様.md`
- `docs/TTS仕様/TTS感情音声システム仕様.md`

---

## 1. 目的

本仕様は、RenCrow の感情音声出力を既存実装へ段階導入するための **最小改修案** を定義する。  
対象は以下の3点である。

- 読み上げ対象を `agent.response` のみに限定する
- RenCrow 側へ `EmotionState` を導入する
- TTS サーバを `emotion_state` 優先の後方互換モードへ移行する

本仕様は最終完成形ではなく、既存の `session_start -> text_delta -> session_end` 契約を維持したまま、責務分離を正しい方向へ寄せるための v0.1 実装仕様とする。

---

## 2. 背景と課題

現行実装には以下の課題がある。

- TTS が `agent.thinking` 相当の途中テキストや括弧注釈まで読み上げる
- Emotion Planner の責務が TTS サーバ側に寄っている
- 仕様上の責務分離
  - `Chat -> Emotion Planner -> TTS Adapter -> TTS Engine`
  に対して未整合がある

今回の v0.1 では、最終形である `speech_chunk` 化や Emotion Planner 完全移設までは行わず、最小差分で以下を達成する。

- RenCrow が「何を読ませるか」を制御する
- RenCrow が EmotionState を保持して TTS へ渡せるようにする
- TTS サーバは「感情決定」ではなく「感情付き発話の変換と合成」へ責務を縮小する

---

## 3. 実装範囲

### 3.1 RenCrow 側

- `agent.response` のみを TTS 対象とする
- `EmotionState` / `EmotionContext` / `EmotionVector` / `Prosody` / `ReasonTrace` を追加する
- 既存の `TTSBridge` 契約
  - `StartSession`
  - `PushText`
  - `EndSession`
  を維持する
- `PushText` に送る本文は、読み上げ可能な最終応答テキストのみとする

### 3.2 TTS サーバ側

- `/sessions` WebSocket 契約は維持する
- `text_delta` に `emotion_state` を持てるようにする
- `emotion_state` が存在する場合はそれを優先し、サーバ内の感情推定は fallback にする
- `emotion_state` が存在しない既存クライアントは従来通り動作可能とする

### 3.3 Viewer 側

- `tts.audio_chunk` を受けて `audio_url` を再生する現行フローを維持する
- UI やイベント表示仕様の変更は行わない

---

## 4. 非スコープ

本 v0.1 では以下を対象外とする。

- `speech_chunk` への全面移行
- Emotion Planner の LLM 補助
- voice 自動選択ロジックの高度化
- TTS サーバ API の全面再設計
- Viewer UI の改修
- Worker メタデータの全面見直し

---

## 5. 責務分離

### 5.1 Chat / RenCrow

役割:

- ユーザーへ読み上げるべき本文を決定する
- Event / Context / Text Features / VoiceProfile から EmotionState を決定する
- TTS 用 session を管理する

### 5.2 TTS Adapter

役割:

- EmotionState を TTS エンジン固有パラメータへ変換する
- WebSocket / HTTP による TTS transport を管理する

TTS Adapter は変換レイヤーであり、感情決定ロジックを持たない。

### 5.3 TTS Engine Server

役割:

- 受け取った text と emotion_state を元に音声を合成する
- `audio_path` / `audio_url` を返す

TTS サーバ内の `derive_emotion()` は v0.1 では fallback として残すが、新規経路では使用しない。

---

## 6. RenCrow 側データモデル

### 6.1 EmotionVector

内部表現は以下の6軸とする。

- `warmth`
- `cheerfulness`
- `seriousness`
- `alertness`
- `calmness`
- `expressiveness`

各値の範囲は `0.0` から `1.0`。

### 6.2 Prosody

正規化された prosody 値は以下を持つ。

- `speed`
- `pitch`
- `pause`
- `expressiveness`

### 6.3 ReasonTrace

判断根拠は以下を持つ。

- `event`
- `applied_context_rules`
- `applied_text_features`
- `voice_profile`

### 6.4 EmotionState

Emotion Planner の出力は以下とする。

```json
{
  "primary_emotion": "warm",
  "emotion_vector": {
    "warmth": 0.72,
    "cheerfulness": 0.40,
    "seriousness": 0.18,
    "alertness": 0.12,
    "calmness": 0.68,
    "expressiveness": 0.36
  },
  "prosody": {
    "speed": 0.48,
    "pitch": 0.53,
    "pause": 0.55,
    "expressiveness": 0.36
  },
  "reason_trace": {
    "event": "conversation",
    "applied_context_rules": ["user_waiting"],
    "applied_text_features": ["greeting"],
    "voice_profile": "lumina_female"
  }
}
```

### 6.5 EmotionContext

初期実装では以下を持つ。

- `conversation_mode`
- `user_waiting_time_sec`
- `time_of_day`
- `previous_event`
- `retry_count`
- `consecutive_failures`
- `urgency`
- `user_attention_required`

---

## 7. Emotion Planner アルゴリズム

Emotion Planner は RenCrow 側に配置する。  
アルゴリズムは正本仕様に従い、以下の順序で実装する。

1. Event から Base Emotion Vector を取得する
2. Context Rules を適用する
3. Text Feature Rules を適用する
4. VoiceProfile Bias を適用する
5. Vector を clamp / normalize する
6. `primary_emotion` を決定する
7. `prosody` を生成する
8. `reason_trace` を出力する

### 7.1 重み

- Event: `70%`
- Context: `20%`
- Text: `10%`

### 7.2 初期イベント集合

- `task_success`
- `task_failure`
- `approval_requested`
- `approval_completed`
- `warning`
- `error`
- `analysis_report`
- `conversation`
- `system_notification`

### 7.3 初期感情カテゴリ

- `calm`
- `warm`
- `cheerful`
- `serious`
- `alert`

### 7.4 Fail Safe

- 未知イベントは `system_notification` 扱い
- context 欠落時は無補正
- text feature 抽出失敗時は無補正
- 全軸は常に `0.0..1.0` へ clamp
- prosody もエンジン安全範囲へ clamp

---

## 8. 読み上げ本文フィルタ

### 8.1 基本方針

TTS に送るのは **ユーザーへ聞かせるべき最終本文のみ** とする。

### 8.2 対象イベント

- 読み上げ対象: `agent.response`
- 読み上げ対象外: `agent.thinking`

### 8.3 除去対象

以下は TTS 送信前に除去する。

- 全体を囲む括弧注釈
- コードブロック
- URL
- 記号のみ断片
- 空行のみの断片

### 8.4 route ごとの扱い

- `CHAT`: 最終返答本文のみ
- `PLAN`: ユーザー向け要約本文のみ
- `CODE`: ユーザー向け要約本文のみ
- `RESEARCH`: ユーザー向け要約本文のみ
- `ANALYZE`: ユーザー向け要約本文のみ

### 8.5 v0.1 の制約

既存の token ストリームを逐次そのまま TTS へ流さない。  
v0.1 では、フィルタ済みの最終応答テキストを `PushText` に渡す。

---

## 9. TTS Bridge / Transport 契約

### 9.1 維持する既存インタフェース

RenCrow 側では以下を維持する。

- `StartSession(ctx, req)`
- `PushText(ctx, sessionID, text)`
- `EndSession(ctx, sessionID)`

### 9.2 StartSession 拡張

`TTSSessionStart` は以下の情報を持てるものとする。

- `SessionID`
- `ResponseID`
- `VoiceID`
- `SpeechMode`
- `Event`
- `Urgency`
- `ConversationMode`
- `UserAttentionRequired`
- `Context`
- `VoiceProfile`

### 9.3 PushText の扱い

v0.1 ではメソッド名と session 契約は維持するが、送信する本文は

- `agent.response` 由来
- フィルタ済み
- EmotionState が確定済み

のテキストに限定する。

---

## 10. TTS サーバ wire 仕様

### 10.1 WebSocket エンドポイント

- `ws://<host>:8765/sessions`

### 10.2 既存メッセージ

以下は維持する。

- `session_start`
- `text_delta`
- `session_end`

### 10.3 `text_delta` 拡張

`text_delta` は v0.1 で以下を追加できるものとする。

```json
{
  "type": "text_delta",
  "session_id": "viewer-...",
  "seq": 1,
  "text": "おはようございます。",
  "emotion_state": {
    "primary_emotion": "warm",
    "emotion_vector": {
      "warmth": 0.72,
      "cheerfulness": 0.40,
      "seriousness": 0.18,
      "alertness": 0.12,
      "calmness": 0.68,
      "expressiveness": 0.36
    },
    "prosody": {
      "speed": 0.48,
      "pitch": 0.53,
      "pause": 0.55,
      "expressiveness": 0.36
    },
    "reason_trace": {
      "event": "conversation",
      "applied_context_rules": ["user_waiting"],
      "applied_text_features": ["greeting"],
      "voice_profile": "lumina_female"
    }
  }
}
```

### 10.4 優先順位

TTS サーバの感情決定優先順位は以下とする。

1. `emotion_state` が明示されている場合はそれを使用する
2. `emotion_state` が無い場合は既存 `event / speech_mode` から fallback 推定する

### 10.5 レスポンス

以下は現行維持。

- `audio_chunk_ready`
- `session_completed`
- `error`

`audio_url` は browser 再生用 URL として継続利用する。

---

## 11. 実装反映位置

### 11.1 RenCrow 側

既存変更点の中心は以下。

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/infrastructure/tts/client_bridge.go`

### 11.2 新規追加想定

Emotion Planner まわりは以下の新規領域へ追加する。

- `internal/application/tts/`

想定モジュール:

- `emotion_planner.go`
- `text_filter.go`
- `models.go`

### 11.3 TTS サーバ側

リポジトリ外管理の `tts_server.py` を対象とし、以下を追加する。

- `emotion_state` 受信
- `emotion_state` 優先の adapter 変換
- 既存 `derive_emotion()` の fallback 化

---

## 12. テスト計画

### 12.1 Emotion Planner 単体

- Event ごとの base vector
- Context 補正
- Text feature 補正
- normalization
- `primary_emotion` 判定

### 12.2 Text Filter 単体

- `agent.thinking` を除外する
- 括弧注釈を除去する
- URL / コードブロックを除去する
- 空断片を破棄する

### 12.3 Orchestrator

- `agent.response` のみ TTS 送信される
- `agent.thinking` では `PushText` が呼ばれない
- route ごとに user 向け本文のみが TTS へ渡る

### 12.4 TTS Client

- `emotion_state` 付き送信 JSON 契約
- `emotion_state` 無しの後方互換
- `session_start -> text_delta -> session_end` 維持

### 12.5 E2E

- Lenovo Viewer で最終返答のみが読まれる
- `tts.audio_chunk` により音声再生される
- `/cache/*.wav` が browser から取得される
- `/sessions` が WebSocket として正常処理される

---

## 13. 完了判定

次を満たした時点で、本 v0.1 実装を完了とする。

1. RenCrow が `agent.response` のみを TTS へ送る
2. RenCrow が `EmotionState` を生成し、TTS へ送信できる
3. TTS サーバが `emotion_state` 優先で合成できる
4. 既存クライアントは `emotion_state` 無しでも動作する
5. Lenovo Viewer で補足文ではなく最終返答本文のみが再生される

---

## 14. 次段階

本 v0.1 完了後の次段階は以下。

- `speech_chunk` を明示契約として導入する
- Emotion Planner を完全に RenCrow 側へ移し、TTS サーバから感情決定ロジックを除去する
- voice 選択と speech planning を拡張する
- Worker メタデータと EmotionContext を連携させる
