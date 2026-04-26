'use strict';

const path = require('path');
const fs = require('fs');
const http = require('http');
const express = require('express');
const WS = require('ws');
const { WebSocketServer } = WS;
const fetch = require('node-fetch');
const FormData = require('form-data');
const { NonRealTimeVAD, Message, utils } = require('@ricky0123/vad-node');
const {
  WS_PATHS,
  buildVadTuning,
  buildStatusEvent,
  createFinalizeHoldController,
  inferWithRetry,
  handleControlMessage,
} = require('./stt-gateway-contract');

const RNNOISE_SYNC_PATH = path.join(__dirname, '..', '..', 'node_modules', '@jitsi', 'rnnoise-wasm', 'dist', 'rnnoise-sync.js');

function parseIntEnv(name, fallback) {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function parseBoolEnv(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined) return fallback;
  return String(raw).toLowerCase() === 'true' || String(raw) === '1';
}

function normalizeBusyPolicy(raw) {
  return raw === 'queue_latest' ? 'queue_latest' : 'drop';
}

const PORT = parseIntEnv('STT_PORT', parseIntEnv('PORT', 8090));
const STT_PROVIDER_URL = process.env.STT_PROVIDER_URL || process.env.WHISPER_URL || 'http://127.0.0.1:8080/inference';
const STT_MIN_AUDIO_BYTES = parseIntEnv('STT_MIN_AUDIO_BYTES', 32044);
const STT_FRAME_SAMPLES = parseIntEnv('STT_FRAME_SAMPLES', 1536);
const STT_DRAFT_INTERVAL_MS = parseIntEnv('STT_DRAFT_INTERVAL_MS', 3000);
const STT_DRAFT_MIN_MS = parseIntEnv('STT_DRAFT_MIN_MS', 900);
// Conversation-oriented defaults:
// - Slightly longer silence window to reduce over-segmentation in natural pauses.
// - Small finalize hold to merge quick restarts after VAD speech end.
// - Lower min speech threshold to keep short backchannels (e.g. "うん", "はい").
const STT_SILENCE_END_MS = parseIntEnv('STT_SILENCE_END_MS', 850);
const STT_FINALIZE_HOLD_MS = parseIntEnv('STT_FINALIZE_HOLD_MS', 240);
const STT_MIN_SPEECH_MS = parseIntEnv('STT_MIN_SPEECH_MS', 180);
const STT_TIMEOUT_MS = parseIntEnv('STT_TIMEOUT_MS', 30000);
const STT_BUSY_POLICY = normalizeBusyPolicy((process.env.STT_BUSY_POLICY || 'drop').toLowerCase());
const STT_DRAFT_ENABLED = parseBoolEnv('STT_DRAFT_ENABLED', false);
const STT_ALLOW_NON_RIFF = parseBoolEnv('STT_ALLOW_NON_RIFF', false);
const STT_MAX_RETRY = parseIntEnv('STT_MAX_RETRY', 1);
const STT_FINAL_SHORT_TEXT_LEN = parseIntEnv('STT_FINAL_SHORT_TEXT_LEN', 6);
const STT_FINAL_DRAFT_FALLBACK_MIN_LEN = parseIntEnv('STT_FINAL_DRAFT_FALLBACK_MIN_LEN', 8);
const VAD_TUNING = buildVadTuning({
  frameSamples: STT_FRAME_SAMPLES,
  silenceEndMs: STT_SILENCE_END_MS,
  minSpeechMs: STT_MIN_SPEECH_MS,
});

function sttLog(level, event, fields = {}) {
  const payload = {
    ts: new Date().toISOString(),
    event,
    ...fields,
  };
  const line = `[stt] ${JSON.stringify(payload)}`;
  if (level === 'error') console.error(line);
  else if (level === 'warn') console.warn(line);
  else console.log(line);
}

// ── WAV utilities ──────────────────────────────────────────────────────────

/**
 * WAV バイナリから PCM16 サンプルとサンプルレートを抽出する。
 * WAV ヘッダがなければ null を返す。
 */
function parseWavPcm16(buf) {
  if (buf.length < 44 || buf[0] !== 0x52 || buf[1] !== 0x49) return null;
  const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  let offset = 12;
  let sampleRate = 16000;
  let dataOffset = null;
  let dataSize = null;

  while (offset + 8 <= buf.length) {
    const id = String.fromCharCode(buf[offset], buf[offset + 1], buf[offset + 2], buf[offset + 3]);
    const size = view.getUint32(offset + 4, true);
    if (id === 'fmt ') {
      sampleRate = view.getUint32(offset + 12, true);
    } else if (id === 'data') {
      dataOffset = offset + 8;
      dataSize = Math.min(size, buf.length - dataOffset);
      break;
    }
    offset += 8 + size + (size % 2 !== 0 ? 1 : 0); // word-align
  }

  if (dataOffset === null) return null;
  const pcm16 = new Int16Array(buf.buffer, buf.byteOffset + dataOffset, Math.floor(dataSize / 2));
  return { pcm16, sampleRate };
}

function pcm16ToFloat32(pcm16) {
  const f32 = new Float32Array(pcm16.length);
  for (let i = 0; i < pcm16.length; i++) f32[i] = pcm16[i] / 32768.0;
  return f32;
}

/** Float32Array → PCM16 WAV Buffer（Whisper に渡す用） */
function float32ToWav(float32) {
  return Buffer.from(utils.encodeWAV(float32, 1, 16000, 1, 16));
}

function concatFloat32(arrays) {
  const total = arrays.reduce((n, a) => n + a.length, 0);
  const out = new Float32Array(total);
  let off = 0;
  for (const a of arrays) { out.set(a, off); off += a.length; }
  return out;
}

// ── Whisper ────────────────────────────────────────────────────────────────

function normalizeText(text) {
  return String(text || '').replace(/\s+/g, ' ').trim();
}

function normalizeSessionTag(raw) {
  const value = String(raw || '').trim().toLowerCase();
  if (!value) return '';
  return value.replace(/[^a-z0-9_-]/g, '');
}

function chooseFinalText(inferredText, latestDraftText) {
  const inferred = normalizeText(inferredText);
  const draft = normalizeText(latestDraftText);

  if (!inferred) {
    return { text: draft, source: draft ? 'draft_fallback_empty_final' : 'empty' };
  }
  if (!draft) {
    return { text: inferred, source: 'provider' };
  }

  // Rescue overly short/fragmented finals (e.g. "います。") when a much richer draft exists.
  const inferredLen = inferred.length;
  const draftLen = draft.length;
  const providerLooksShort = inferredLen <= STT_FINAL_SHORT_TEXT_LEN;
  const draftClearlyRicher = draftLen >= Math.max(STT_FINAL_DRAFT_FALLBACK_MIN_LEN, inferredLen + 3);
  const draftContainsInferred = draft.includes(inferred) || inferred.includes(draft);

  if (providerLooksShort && draftClearlyRicher && draftContainsInferred) {
    return { text: draft, source: 'draft_fallback_short_final' };
  }

  return { text: inferred, source: 'provider' };
}

async function transcribeBuffer(buffer) {
  const result = await providerInfer(buffer, 'audio/wav', { phase: 'unknown' });
  return result.text;
}

async function providerInfer(audioBuffer, mimeType = 'audio/wav', options = {}) {
  const {
    phase = 'unknown',
    sessionId = 'unknown',
    requestId = 'unknown',
    signal,
  } = options;
  const buffer = Buffer.isBuffer(audioBuffer) ? audioBuffer : Buffer.from(audioBuffer);
  if (buffer.length < STT_MIN_AUDIO_BYTES) {
    sttLog('warn', 'provider_skipped_short_audio', {
      session_id: sessionId,
      request_id: requestId,
      phase,
      input_bytes: buffer.length,
      error_code: 'AUDIO_TOO_SHORT',
      result: 'skipped',
    });
    return { text: '', errorCode: 'AUDIO_TOO_SHORT' };
  }

  const form = new FormData();
  form.append('file', buffer, { filename: 'audio.wav', contentType: mimeType });
  form.append('response_format', 'json');

  const ac = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    ac.abort();
  }, STT_TIMEOUT_MS);

  if (signal) {
    if (signal.aborted) ac.abort();
    else signal.addEventListener('abort', () => ac.abort(), { once: true });
  }

  const startedAt = Date.now();
  try {
    const res = await fetch(STT_PROVIDER_URL, {
      method: 'POST',
      body: form,
      headers: form.getHeaders(),
      signal: ac.signal,
    });
    if (!res.ok) {
      sttLog('warn', 'provider_http_error', {
        session_id: sessionId,
        request_id: requestId,
        phase,
        elapsed_ms: Date.now() - startedAt,
        input_bytes: buffer.length,
        error_code: 'PROVIDER_HTTP_ERROR',
        status: res.status,
        result: 'fail',
      });
      return { text: '', errorCode: 'PROVIDER_HTTP_ERROR' };
    }
    const payload = await res.json();
    const text = normalizeText(payload.text);
    sttLog('log', 'provider_success', {
      session_id: sessionId,
      request_id: requestId,
      phase,
      elapsed_ms: Date.now() - startedAt,
      input_bytes: buffer.length,
      result: text ? 'success' : 'skipped',
    });
    return { text, errorCode: null };
  } catch (e) {
    const errorCode = timedOut ? 'PROVIDER_TIMEOUT' : 'PROVIDER_EXCEPTION';
    sttLog('warn', 'provider_exception', {
      session_id: sessionId,
      request_id: requestId,
      phase,
      elapsed_ms: Date.now() - startedAt,
      input_bytes: buffer.length,
      error_code: errorCode,
      message: e.message,
      result: 'fail',
    });
    return { text: '', errorCode };
  } finally {
    clearTimeout(timer);
  }
}

// ── LLM reply stub（後で差し替え） ─────────────────────────────────────────

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

async function streamReply(finalText, send) {
  send({ type: 'reply_reset' });
  for (const ch of `了解です。${finalText}`) {
    send({ type: 'reply_delta', text: ch });
    await sleep(8);
  }
}

// ── VAD 初期化（起動時に1回だけ） ──────────────────────────────────────────

let sharedVad = null;

async function initVAD() {
  sharedVad = await NonRealTimeVAD.new({
    positiveSpeechThreshold: 0.7,
    negativeSpeechThreshold: 0.35,
    redemptionFrames: VAD_TUNING.redemptionFrames,
    frameSamples: STT_FRAME_SAMPLES,
    preSpeechPadFrames: 1,
    minSpeechFrames: VAD_TUNING.minSpeechFrames,
  });
  console.log('[voice-bridge] Silero VAD loaded');
}

// ── Express + WebSocket ────────────────────────────────────────────────────

const app = express();
app.use(express.static(path.join(__dirname, 'public')));

app.get('/health', (_req, res) => res.json({ ok: true }));

app.get('/rnnoise-sync.js', (_req, res) => {
  try {
    const src = fs.readFileSync(RNNOISE_SYNC_PATH, 'utf8');
    const patched = src.replace(
      'var _scriptDir = import.meta.url;',
      'var _scriptDir = typeof self !== "undefined" ? self.location.href : "";'
    );
    res.setHeader('Content-Type', 'application/javascript');
    res.send(patched);
  } catch (e) {
    res.status(500).send('// rnnoise-sync.js load failed: ' + e.message);
  }
});

const server = http.createServer(app);
function handleWsConnection(ws) {
  if (!sharedVad) {
    ws.close(1011, 'VAD not ready');
    return;
  }

  // NOTE: sharedVad.frameProcessor は接続ごとに reset して使い回す。
  // 同時接続が複数想定される場合は NonRealTimeVAD.new() を接続ごとに呼ぶこと。
  const fp = sharedVad.frameProcessor;
  fp.reset();
  fp.resume();

  let isSpeaking = false;
  let speechChunks = [];              // 発話中フレームの蓄積（draft 用）
  let pcmRemainder = new Float32Array(0); // フレーム境界の端数
  let draftTimer = null;
  let busy = false;
  let busyPhase = '';
  let msgSeq = 0;
  let currentDraftText = '';
  const sessionState = {
    clientMimeType: 'audio/webm',
    expectFinalAudio: false,
  };
  const sessionTag = normalizeSessionTag(process.env.STT_SESSION_ID_TAG);
  const sessionId = sessionTag
    ? `sess-${sessionTag}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
    : `sess-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
  const clipText = (text, maxLen = 160) => {
    const normalized = normalizeText(text);
    if (!normalized) return '';
    return normalized.length <= maxLen ? normalized : `${normalized.slice(0, maxLen)}...`;
  };
  const logWsEvent = (event, fields = {}) => {
    sttLog('log', event, {
      session_id: sessionId,
      ...fields,
    });
  };

  const send = (obj) => {
    if (ws.readyState === WS.OPEN) ws.send(JSON.stringify(obj));
  };
  const emitSessionInfo = (reason) => {
    send({ type: 'session_info', session_id: sessionId });
    logWsEvent('ws_session_info_emit', { reason });
  };
  // Correlation ID for client/server log matching.
  emitSessionInfo('connection_open');
  logWsEvent('ws_session_open');

  function nextRequestId(phase) {
    msgSeq += 1;
    return `${sessionId}-${phase}-${msgSeq}`;
  }

  function stopDraftTimer() {
    if (draftTimer) { clearInterval(draftTimer); draftTimer = null; }
  }

  async function inferWithPolicy(phase, audioBuffer, mimeType = 'audio/wav') {
    const requestId = nextRequestId(phase);
    if (busy) {
      if (STT_BUSY_POLICY === 'queue_latest') {
        sttLog('warn', 'busy_queue_latest_not_implemented', {
          session_id: sessionId,
          request_id: requestId,
          phase,
          busy_phase: busyPhase,
          error_code: 'BUSY_POLICY_FALLBACK_TO_DROP',
        });
      }
      sttLog('warn', 'busy_drop', {
        session_id: sessionId,
        request_id: requestId,
        phase,
        busy_phase: busyPhase,
        result: 'skipped',
      });
      return '';
    }
    busy = true;
    busyPhase = phase;
    try {
      const result = await inferWithRetry({
        maxRetry: STT_MAX_RETRY,
        phase,
        // Avoid noisy status spam while speaking; keep status for finalization path.
        sendStatus: (text) => {
          if (phase !== 'draft') {
            send(buildStatusEvent(text));
          }
        },
        infer: () => providerInfer(audioBuffer, mimeType, {
          phase,
          sessionId,
          requestId,
        }),
      });
      return result?.text || '';
    } finally {
      busy = false;
      busyPhase = '';
    }
  }

  async function handleSpeechEnd(audioFloat32) {
    stopDraftTimer();
    isSpeaking = false;
    speechChunks = [];
    try {
      const wavBuf = float32ToWav(audioFloat32);
      const inferred = await inferWithPolicy('final', wavBuf, 'audio/wav');
      const chosen = chooseFinalText(inferred, currentDraftText);
      const finalText = chosen.text;
      if (finalText) {
        if (chosen.source !== 'provider' && currentDraftText) {
          send(buildStatusEvent('stt final fallback: use latest draft'));
        }
        console.log(`[voice-bridge] transcribed: "${finalText}"`);
        send({ type: 'final', text: finalText });
        logWsEvent('ws_final_emit', {
          source: chosen.source,
          text: clipText(finalText),
          text_len: finalText.length,
        });
        await streamReply(finalText, send);
        currentDraftText = '';
      } else {
        console.log('[voice-bridge] transcribed: (empty — skipped)');
        logWsEvent('ws_final_empty', { reason: 'empty_text' });
      }
    } catch (e) {
      console.error('[voice-bridge] final transcription:', e);
    }
  }

  const finalizeHoldController = createFinalizeHoldController({
    holdMs: STT_FINALIZE_HOLD_MS,
    onFinalize: handleSpeechEnd,
  });

  async function finalizePendingSpeech() {
    if (await finalizeHoldController.flushNow()) return;
    if (!isSpeaking || speechChunks.length === 0) return;
    const audio = concatFloat32(speechChunks);
    await handleSpeechEnd(audio);
  }

  async function processFrames(float32Samples) {
    // 前回の端数と結合
    let all;
    if (pcmRemainder.length > 0) {
      all = new Float32Array(pcmRemainder.length + float32Samples.length);
      all.set(pcmRemainder);
      all.set(float32Samples, pcmRemainder.length);
    } else {
      all = float32Samples;
    }

    let offset = 0;
    while (offset + STT_FRAME_SAMPLES <= all.length) {
      const frame = all.slice(offset, offset + STT_FRAME_SAMPLES);
      offset += STT_FRAME_SAMPLES;

      let result;
      try {
        result = await fp.process(frame);
      } catch (e) {
        console.error('[voice-bridge] VAD process error:', e);
        continue;
      }

      const { msg, audio } = result;

      if (msg === Message.SpeechStart) {
        const mergedStartAudio = finalizeHoldController.cancelAndTakeAudio(frame);
        isSpeaking = true;
        currentDraftText = '';
        speechChunks = [mergedStartAudio];
        send({ type: 'speech_start' });
        console.log('[voice-bridge] speech start detected');
        logWsEvent('ws_speech_start', {
          draft_enabled: STT_DRAFT_ENABLED,
        });

        if (STT_DRAFT_ENABLED) {
          draftTimer = setInterval(async () => {
            if (!isSpeaking || speechChunks.length === 0) return;
            const snap = concatFloat32(speechChunks);
            const draftDurationMs = (snap.length / 16000) * 1000;
            if (draftDurationMs < STT_DRAFT_MIN_MS) return;
            try {
              const text = await inferWithPolicy('draft', float32ToWav(snap), 'audio/wav');
              if (text) {
                currentDraftText = text;
                send({ type: 'draft', text });
                logWsEvent('ws_draft_emit', {
                  source: 'vad_interval',
                  text: clipText(text),
                  text_len: text.length,
                });
              }
            } catch (e) {
              sttLog('warn', 'draft_infer_exception', {
                session_id: sessionId,
                phase: 'draft',
                error_code: 'PROVIDER_EXCEPTION',
                message: e.message,
              });
            }
          }, STT_DRAFT_INTERVAL_MS);
        }

      } else if (isSpeaking && msg !== Message.SpeechEnd) {
        speechChunks.push(frame);
      }

      if (msg === Message.SpeechEnd && audio) {
        const finalizedAudio = concatFloat32(speechChunks);
        console.log(`[voice-bridge] speech end (${(audio.length / 16000).toFixed(1)}s)`);
        stopDraftTimer();
        isSpeaking = false;
        speechChunks = [];
        if (STT_FINALIZE_HOLD_MS > 0) {
          finalizeHoldController.schedule(finalizedAudio);
        } else {
          await handleSpeechEnd(finalizedAudio);
        }
      }
    }

    pcmRemainder = all.slice(offset);
  }

  ws.on('message', async (data, isBinary) => {
    if (!isBinary) {
      // Re-emit on config control message for clients/proxies that may miss
      // the very first unsolicited server frame right after WS upgrade.
      try {
        const control = JSON.parse(data.toString());
        if (control && control.type === 'config') {
          emitSessionInfo('config_echo');
        }
      } catch {
        // Ignore parse errors here; handleControlMessage returns structured error.
      }
      await handleControlMessage(data, {
        state: sessionState,
        sendEvent: (evt) => send(evt),
        onFinalPending: finalizePendingSpeech,
      });
      return;
    }

    const buf = Buffer.isBuffer(data) ? data : Buffer.from(data);
    if (sessionState.expectFinalAudio) {
      sessionState.expectFinalAudio = false;
      finalizeHoldController.reset();
      stopDraftTimer();
      isSpeaking = false;
      let finalBuffer = null;
      let finalMimeType = 'audio/wav';
      const parsedFinal = parseWavPcm16(buf);
      if (parsedFinal) {
        finalBuffer = float32ToWav(pcm16ToFloat32(parsedFinal.pcm16));
        finalMimeType = 'audio/wav';
      } else if (sessionState.clientMimeType) {
        // Accept browser-native encoded final chunks (e.g. webm/mp4) and let provider convert.
        finalBuffer = buf;
        finalMimeType = sessionState.clientMimeType;
      } else if (speechChunks.length > 0) {
        // If client sent a non-WAV chunk at stop time, finalize from buffered speech.
        finalBuffer = float32ToWav(concatFloat32(speechChunks));
        finalMimeType = 'audio/wav';
      }
      speechChunks = [];
      if (!finalBuffer) {
        send(buildStatusEvent('stt final skipped: unsupported final audio format'));
        return;
      }
      const text = await inferWithPolicy('final', finalBuffer, finalMimeType);
      const chosen = chooseFinalText(text, currentDraftText);
      const finalText = chosen.text;
      if (finalText) {
        if (chosen.source !== 'provider' && currentDraftText) {
          send(buildStatusEvent('stt final fallback: use latest draft'));
        }
        send({ type: 'final', text: finalText });
        logWsEvent('ws_final_emit', {
          source: chosen.source,
          text: clipText(finalText),
          text_len: finalText.length,
        });
        await streamReply(finalText, send);
        currentDraftText = '';
      }
      return;
    }

    const parsed = parseWavPcm16(buf);
    if (!parsed) {
      sttLog('warn', 'invalid_audio', {
        session_id: sessionId,
        phase: 'ingest',
        error_code: 'INVALID_AUDIO',
        input_bytes: buf.length,
        allow_non_riff: STT_ALLOW_NON_RIFF,
      });
      if (STT_ALLOW_NON_RIFF && STT_DRAFT_ENABLED) {
        const text = await inferWithPolicy('draft', buf, sessionState.clientMimeType);
        if (text) {
          send({ type: 'draft', text });
          logWsEvent('ws_draft_emit', {
            source: 'non_riff',
            text: clipText(text),
            text_len: text.length,
          });
        }
      }
      return;
    }

    await processFrames(pcm16ToFloat32(parsed.pcm16));
  });

  ws.on('close', () => {
    stopDraftTimer();
    fp.reset();
    isSpeaking = false;
    speechChunks = [];
    pcmRemainder = new Float32Array(0);
    sessionState.expectFinalAudio = false;
    finalizeHoldController.reset();
  });
}

const wsGateways = new Map();
for (const wsPath of WS_PATHS) {
  const gateway = new WebSocketServer({ noServer: true });
  gateway.on('connection', handleWsConnection);
  wsGateways.set(wsPath, gateway);
}

server.on('upgrade', (request, socket, head) => {
  let pathname = '';
  try {
    pathname = new URL(request.url || '/', 'http://localhost').pathname;
  } catch {
    socket.destroy();
    return;
  }

  const gateway = wsGateways.get(pathname);
  if (!gateway) {
    socket.destroy();
    return;
  }

  gateway.handleUpgrade(request, socket, head, (ws) => {
    gateway.emit('connection', ws, request);
  });
});

// ── 起動 ────────────────────────────────────────────────────────────────────

initVAD()
  .then(() => {
    sttLog('log', 'startup_config', {
      stt_provider_url: STT_PROVIDER_URL,
      stt_timeout_ms: STT_TIMEOUT_MS,
      stt_min_audio_bytes: STT_MIN_AUDIO_BYTES,
      stt_draft_interval_ms: STT_DRAFT_INTERVAL_MS,
      stt_draft_min_ms: STT_DRAFT_MIN_MS,
      stt_silence_end_ms: STT_SILENCE_END_MS,
      stt_finalize_hold_ms: STT_FINALIZE_HOLD_MS,
      stt_min_speech_ms: STT_MIN_SPEECH_MS,
      stt_draft_enabled: STT_DRAFT_ENABLED,
      effective_redemption_frames: VAD_TUNING.redemptionFrames,
      effective_min_speech_frames: VAD_TUNING.minSpeechFrames,
      stt_frame_samples: STT_FRAME_SAMPLES,
      stt_busy_policy: STT_BUSY_POLICY,
      stt_allow_non_riff: STT_ALLOW_NON_RIFF,
      stt_max_retry: STT_MAX_RETRY,
      stt_final_short_text_len: STT_FINAL_SHORT_TEXT_LEN,
      stt_final_draft_fallback_min_len: STT_FINAL_DRAFT_FALLBACK_MIN_LEN,
      port: PORT,
    });
    server.listen(PORT, () => {
      console.log(`voice-bridge listening on http://127.0.0.1:${PORT}`);
    });
  })
  .catch((err) => {
    console.error('[voice-bridge] VAD init failed:', err);
    process.exit(1);
  });
