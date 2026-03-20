
```markdown
# RenCrow Emotion Voice System Specification
Emotion Planner + TTS Adapter

Version: 1.0
Author: RenCrow Architecture
Status: **Partial Implementation** (Updated: 2026-03-20)

---

# 1. Purpose

RenCrow の音声出力を以下の条件で実装する。

- 状況に応じた感情を持つ音声を生成する
- TTSエンジンに依存しない設計にする
- 将来のTTS変更に耐える

このため、音声生成を以下の3層構造とする。

```

Chat → Emotion Planner → TTS Adapter → TTS Engine

```

Emotion Planner は **発話感情を決定する**。  
TTS Adapter は **感情状態を各TTSエンジンのパラメータに変換する**。

---

# 2. Implementation Status (2026-03-20)

## 2.1 Implemented Components

### ✅ TTS Infrastructure Layer

**Location**: `internal/infrastructure/tts/`

**Core Components**:
- `synthesizer.go` - Provider interface and fallback synthesizer
- `player.go` - Audio playback with command execution
- `errors.go` - Error taxonomy and classification

**Provider Implementation**:
- `sbv2_provider.go` - Style-Bert-VITS2 provider (primary)
- `provider_unavailable.go` - Unavailable provider (fallback placeholder)

**Supporting Components**:
- `audio_path.go` - Audio path resolution (relative→absolute, Windows/Linux cross-platform)
- `audio_url.go` - Audio URL resolution
- `audio_sink.go` - AudioSink interface and PlaybackAudioSink implementation
- `client_bridge.go` - **WebSocket-based streaming TTS client** (real-time session management)
- `reorder_buffer.go` - Audio chunk reordering buffer (out-of-order chunk handling)

### ✅ Error Taxonomy

**Synthesis Errors**:
- `ErrProviderUnavailable` - TTS provider not configured/unreachable
- `ErrSynthesisFailed` - Audio synthesis failed
- `ErrInvalidInput` - Invalid synthesis input

**Playback Errors**:
- `ErrPlaybackFailed` - Audio playback failed
- `ErrCommandNotFound` - Playback command not found/configured
- `ErrAudioFileNotFound` - Generated audio file not found

**Repair Errors**:
- `ErrRepairExhausted` - All repair attempts exhausted (autonomous executor)

**Classification Function**:
```go
ClassifyTTSError(err error) (errorKind, failureReason string)
```

Returns structured error classification for autonomous executor's `ExecutionReport`.

### ✅ Streaming TTS Infrastructure

**WebSocket-based real-time TTS** via ClientBridge:
- Session-based streaming (StartSession, PushText, EndSession)
- **EmotionState integration** - Accepts emotion parameters per text chunk
- Out-of-order chunk reordering
- Automatic fallback to HTTP synthesize endpoint
- AudioSink for ordered chunk playback

**Key Point**: While **Emotion Planner is not implemented**, the infrastructure to receive and transmit `EmotionState` to TTS providers **is fully operational**. Manual or future LLM-based emotion generation can be integrated without infrastructure changes.

### ✅ Autonomous Executor Integration

TTS capability pack (`tts_delivery`) integrated into autonomous executor:
- Route-aware contract normalization
- Error classification for operational debugging
- Retry/repair mechanics with attempt tracking

## 2.2 Current Limitations

**Provider Strategy**:
- **sbv2 only** - No fallback to other providers
- Other providers (Azure, ElevenLabs) - Interface defined, not implemented
- Enabled only when user explicitly configures in settings

**Error Handling**:
- Provider unavailable → Error display only (no automatic fallback)
- Playback exit code interpretation not yet categorized

**Missing Components**:
- ❌ Emotion Planner (not implemented)
- ❌ Audio Cache (not implemented)
- ❌ Voice Profile management (not implemented)
- ❌ Context-aware emotion adjustment (not implemented)

---

# 3. System Architecture

## 3.1 Component Overview

### Design (Target Architecture)

```
Chat
├ Dialogue Manager
├ Text Planner
├ Emotion Planner (❌ Not Implemented)
└ TTS Request Builder
↓
TTS Adapter (⚠️ Partial - sbv2 only)
↓
TTS Engine (✅ sbv2 Provider)
↓
Audio Output (✅ CommandPlayer)
```

### Current Implementation

```
Chat/Worker
↓
SynthesisInput {text, emotion, voice_profile, output_dir}
↓
FallbackSynthesizer (✅ Implemented)
├─ sbv2_provider (✅ Primary)
└─ provider_unavailable (✅ Fallback placeholder)
↓
SynthesisOutput {provider, voice_id, audio_file_path, duration_ms}
↓
CommandPlayer (✅ Implemented)
├─ Command execution with fallback
└─ PlaybackResult {command, exit_code}
↓
Audio Output
```

---

# 4. Responsibility Separation

## Chat

役割

- ユーザーとの対話窓口
- 世間話・会話文脈の管理
- Worker結果のユーザー向け翻訳
- Emotion決定
- TTS生成要求

Chat は RenCrow の **人格レイヤー** として振る舞う。

---

## Worker

役割

- 処理の実行
- 処理結果の要約
- Emotion判断に必要なメタデータの提供

Worker は **感情決定を行わない**。

---

## TTS Adapter

役割

- EmotionState を各TTSエンジンのパラメータへ変換

TTS Adapter は **ロジックを持たない変換レイヤー** とする。

---

# 5. Emotion Planner (Not Implemented)

**Status**: ❌ Not Implemented (Design only)

Emotion Planner は Chat コンポーネント内に配置する。

理由

- Chat が世間話を管理する唯一のコンポーネント
- Chat がすべてのユーザー報告の窓口
- 発話感情は「処理結果」ではなく「伝え方」に属する

Worker は感情を決定せず、判断材料のみ提供する。

---

# 6. Emotion Decision Model

Emotion Planner は三段階で感情を決定する。

```

Base Emotion ← Event
Context Adjustment ← Context
Text Adjustment ← Text Features

```

重み

```

Event     70%
Context   20%
Text      10%

````

---

# 7. EmotionState

Emotion Planner の出力。

```json
{
  "emotion": "warm",
  "intensity": 0.45,
  "speed": 1.05,
  "pitch": 1.02,
  "pause": "normal",
  "expressiveness": 0.35,
  "reason": {
    "context": ["user_waiting"],
    "text_features": ["gratitude"]
  }
}
````

---

# 8. Emotion Categories

初期バージョンでは以下を使用する。

```

calm
warm
cheerful
serious
alert

```

説明

**calm**
落ち着いた説明

**warm**
柔らかい会話

**cheerful**
成功・ポジティブ

**serious**
注意・重要説明

**alert**
警告

---

# 9. Event Input

Emotion Planner の主要入力。

```

task_success
task_failure
warning
error
analysis_report
conversation
system_notification

```

イベント → Base Emotion

```

task_success → cheerful
task_failure → serious
warning → alert
error → alert
analysis_report → calm
conversation → warm
system_notification → calm

```

---

# 10. Context Input

Emotion Planner に渡される文脈情報。

```json
{
  "conversation_mode": "report",
  "user_waiting_time": 25,
  "time_of_day": "night",
  "previous_event": "task_failure",
  "retry_count": 1
}
```

文脈による影響例

長時間待ち
→ warmth 上昇

深夜
→ speech_speed 減少

失敗後成功
→ cheerful 上昇

---

# 11. Text Features

発話テキストから検出する軽量特徴。

例

```

gratitude
apology
confirmation
warning_phrase
success_phrase

```

例

```

ありがとうございます → gratitude
問題ありません → confirmation

```

---

# 12. Worker Response Format

Worker は Emotion Planner 用メタデータを含む。

```json
{
  "event_type": "task_success",
  "severity": "low",
  "requires_user_attention": false,
  "user_impact": "medium",
  "retry_count": 1,
  "summary": "同期処理が完了しました",
  "details": {
    "duration_sec": 42,
    "items_processed": 18
  }
}
```

---

# 13. TTS Adapter Interface

Adapter インタフェース。

```

generateVoice(text, emotionState, voiceProfile)

```

戻り値

```

audioBuffer

```

---

# 14. Azure Adapter Example (Not Implemented)

EmotionState

```

emotion = warm
intensity = 0.4
speed = 1.05
pitch = 1.02

```

SSML変換

```

style = friendly
rate = +5%
pitch = +2%

```

SSML例

```xml
<speak>
  <voice name="ja-JP-NanamiNeural">
    <prosody rate="+5%" pitch="+2%">
      確認できました。続行します。
    </prosody>
  </voice>
</speak>
```

---

# 15. ElevenLabs Adapter Example (Not Implemented)

EmotionState

```

emotion = cheerful
intensity = 0.6
expressiveness = 0.5

```

変換

```

stability
similarity_boost
style_exaggeration

```

例

```

stability = 0.45
similarity_boost = 0.75
style_exaggeration = 0.5

```

---

# 16. Voice Profile (Not Implemented)

声の人格は VoiceProfile で管理する。

```json
{
  "voice_id": "lumina",
  "base_pitch": 1.0,
  "base_speed": 1.0,
  "warmth_bias": 0.1,
  "expressiveness_bias": 0.2
}
```

---

# 17. Audio Cache (Not Implemented)

音声生成コスト削減のためキャッシュを行う。

キー

```

hash(
text,
emotion,
voice_profile,
tts_engine
)

```

同一条件の場合は音声を再利用する。

---

# 18. Voice Generation Pipeline

```

Worker Result
      ↓
Chat
      ↓
Text Planner
      ↓
Emotion Planner
      ↓
TTS Adapter
      ↓
TTS Engine
      ↓
Audio Cache
      ↓
Audio Output

```

---

# 19. Future Extensions

追加Emotion

```

relief
curious
playful
concerned

```

将来拡張

* LLM補助Emotion Planner
* ユーザー個別音声調整
* Voice Cloning
* Local TTS統合

---

# 20. Implementation Notes

## 20.1 sbv2 Provider

**Style-Bert-VITS2** (`sbv2_provider.go`) is the primary TTS provider.

**Features**:
- Emotional voice synthesis with Style-Bert-VITS2
- EmotionState mapping to sbv2 parameters (emotion, intensity, speed, pitch)
- HTTP API integration
- Audio file generation with configurable output directory

**Configuration**:
```yaml
tts:
  sbv2:
    enabled: true
    endpoint: "http://localhost:5000"
    voice_id: "lumina"
```

**Error Handling**:
- HTTP errors → `ErrSynthesisFailed`
- Empty text → `ErrInvalidInput`
- Provider unavailable → `ErrProviderUnavailable`

## 20.2 Command Player

**CommandPlayer** (`player.go`) executes audio playback commands.

**Features**:
- Multiple command fallback (e.g., `mpv`, `ffplay`, `aplay`)
- Command specification with argument templates
- Exit code tracking in `PlaybackResult`

**Configuration**:
```yaml
tts:
  playback:
    commands:
      - name: "mpv"
        args: ["--no-video", "{audio}"]
      - name: "ffplay"
        args: ["-nodisp", "-autoexit", "{audio}"]
```

**Error Handling**:
- No commands configured → `ErrCommandNotFound`
- All commands failed → `ErrPlaybackFailed` (wraps last error)
- Command not found (exec error) → exit_code -1
- Command exit non-zero → exit_code from process

## 20.3 WebSocket Streaming TTS (ClientBridge)

**ClientBridge** (`client_bridge.go`) provides real-time streaming TTS for conversational interactions.

**Features**:
- WebSocket-based session management
- Real-time text streaming with `text_delta` messages
- EmotionState transmission per text chunk
- Out-of-order chunk reordering (via `reorderBuffer`)
- Automatic fallback to HTTP `/synthesize` endpoint
- Health check and voice availability verification

**Session Flow**:
```
StartSession(sessionID, characterID, voiceID, context)
  → WebSocket connection established
  → session_start message sent

PushText(sessionID, text, emotionState)
  → text_delta message sent (seq, text, emotion_state)
  → Audio chunks received asynchronously
  → Chunks buffered and reordered
  → PlaybackAudioSink plays chunks in order

EndSession(sessionID)
  → session_end message sent
  → WebSocket connection closed
```

**Configuration**:
```yaml
tts:
  client_bridge:
    http_base_url: "http://localhost:5000"
    ws_url: "ws://localhost:5000/ws"
    voice_id: "lumina_female"
    speech_mode: "conversational"
    connect_timeout: "3s"
    receive_timeout: "15s"
    chunk_gap_timeout: "3s"
```

**EmotionState Integration**:
- ClientBridge accepts `ttsapp.EmotionState` per text chunk
- Emotion parameters (primary_emotion, prosody.speed, prosody.pitch, etc.) are sent to TTS server
- TTS server adjusts voice synthesis based on emotion state
- **Design alignment**: Matches the Emotion Planner output format (sections 5-12)

**Reorder Buffer**:
- Handles out-of-order audio chunks (network reordering)
- Gap timeout: if chunk N is missing for >3s, skip to next available chunk
- Final drain: flushes all pending chunks on session completion

**Audio Sink**:
- `PlaybackAudioSink` (`audio_sink.go`) consumes ordered audio chunks
- Resolves relative audio paths to absolute playback paths
- Plays each chunk via `CommandPlayer`
- Applies `pause_after` delay between chunks (default: 200ms)

## 20.4 Error Classification for Autonomous Executor

`ClassifyTTSError()` function maps TTS errors to structured error kinds:

| Error | Error Kind | Usage |
|-------|-----------|-------|
| `ErrProviderUnavailable` | `provider_unavailable` | TTS service down/not configured |
| `ErrSynthesisFailed` | `synthesis_failed` | Audio generation failed |
| `ErrInvalidInput` | `invalid_input` | Empty/invalid text |
| `ErrPlaybackFailed` | `playback_failed` | Audio playback failed |
| `ErrCommandNotFound` | `command_not_found` | No playback command available |
| `ErrAudioFileNotFound` | `audio_file_not_found` | Generated audio missing |
| `ErrRepairExhausted` | `repair_exhausted` | All repair attempts failed |
| Unknown | `tts_unknown` | Unclassified TTS error |

**Autonomous Executor Integration**:
- TTS errors are saved to `ExecutionReport.TTSErrorKind`
- Error kind enables operational debugging and statistics
- Repair/retry mechanics track `attempt_count` and `repair_count`

## 20.5 Current vs. Design Gap

**Implemented**:
- ✅ TTS infrastructure (Provider, Player, Errors)
- ✅ sbv2 provider integration
- ✅ Command-based audio playback
- ✅ **WebSocket streaming TTS** (ClientBridge, EmotionState transmission)
- ✅ Audio chunk reordering and playback pipeline
- ✅ Autonomous executor integration

**Design Only (Not Implemented)**:
- ❌ Emotion Planner (sections 5-12)
- ❌ Context-aware emotion adjustment
- ❌ Audio Cache (section 17)
- ❌ Voice Profile management
- ❌ Azure/ElevenLabs adapters (sections 14-15)

**Next Steps**:
1. Implement Emotion Planner in Chat component
2. Add context-aware emotion adjustment
3. Implement Audio Cache for cost reduction
4. Add Voice Profile management

```
