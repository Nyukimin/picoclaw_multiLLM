'use strict';

const WS_PATHS = ['/stt-ws', '/ws'];

function buildErrorEvent(message) {
  return {
    type: 'error',
    error: String(message || 'unknown error'),
  };
}

function buildStatusEvent(text) {
  return {
    type: 'status',
    text: String(text || ''),
  };
}

function framesFromMs(ms, frameSamples, sampleRate = 16000) {
  const safeMs = Number.isFinite(ms) && ms > 0 ? ms : 1;
  const safeFrameSamples =
    Number.isFinite(frameSamples) && frameSamples > 0 ? frameSamples : 1536;
  const frameDurationMs = (safeFrameSamples / sampleRate) * 1000;
  return Math.max(1, Math.ceil(safeMs / frameDurationMs));
}

function buildVadTuning({
  frameSamples = 1536,
  silenceEndMs = 700,
  minSpeechMs = 250,
  sampleRate = 16000,
} = {}) {
  const frameDurationMs = (frameSamples / sampleRate) * 1000;
  return {
    frameDurationMs,
    redemptionFrames: framesFromMs(silenceEndMs, frameSamples, sampleRate),
    minSpeechFrames: framesFromMs(minSpeechMs, frameSamples, sampleRate),
  };
}

function concatAudio(a, b) {
  if (!a || a.length === 0) return b || new Float32Array(0);
  if (!b || b.length === 0) return a;
  const merged = new Float32Array(a.length + b.length);
  merged.set(a, 0);
  merged.set(b, a.length);
  return merged;
}

function createFinalizeHoldController({
  holdMs = 180,
  onFinalize,
  setTimer = (fn, ms) => setTimeout(fn, ms),
  clearTimer = (timerId) => clearTimeout(timerId),
} = {}) {
  let pendingTimer = null;
  let pendingAudio = null;

  function hasPending() {
    return pendingAudio !== null;
  }

  function reset() {
    if (pendingTimer !== null) {
      clearTimer(pendingTimer);
      pendingTimer = null;
    }
    pendingAudio = null;
  }

  function schedule(audio) {
    reset();
    pendingAudio = audio;
    pendingTimer = setTimer(async () => {
      const target = pendingAudio;
      pendingTimer = null;
      pendingAudio = null;
      if (target && typeof onFinalize === 'function') {
        await onFinalize(target);
      }
    }, holdMs);
  }

  function cancelAndTakeAudio(extraAudio = null) {
    const target = pendingAudio;
    reset();
    return concatAudio(target, extraAudio);
  }

  async function flushNow() {
    if (!pendingAudio) return false;
    const target = pendingAudio;
    reset();
    if (typeof onFinalize === 'function') {
      await onFinalize(target);
    }
    return true;
  }

  return {
    hasPending,
    schedule,
    cancelAndTakeAudio,
    flushNow,
    reset,
  };
}

function shouldRetry(errorCode) {
  return (
    errorCode === 'PROVIDER_TIMEOUT' ||
    errorCode === 'PROVIDER_EXCEPTION' ||
    errorCode === 'PROVIDER_HTTP_ERROR'
  );
}

async function inferWithRetry({
  infer,
  maxRetry = 0,
  phase = 'unknown',
  sendStatus = () => {},
}) {
  let attempt = 0;
  while (true) {
    const result = await infer();
    const errorCode = result?.errorCode ?? null;
    if (!errorCode || !shouldRetry(errorCode)) {
      return result || { text: '', errorCode: null };
    }

    if (attempt >= maxRetry) {
      sendStatus(`stt provider ${errorCode.toLowerCase()} (failed)`);
      return { text: '', errorCode };
    }

    attempt += 1;
    sendStatus(
      `stt provider ${errorCode.toLowerCase()} (retrying ${attempt}/${maxRetry})`
    );
  }
}

async function handleControlMessage(rawData, { state, sendEvent, onFinalPending }) {
  let msg;
  try {
    msg = JSON.parse(rawData.toString());
  } catch {
    sendEvent(buildErrorEvent('invalid JSON'));
    return;
  }

  if (msg.type === 'config') {
    if (typeof msg.mimeType === 'string' && msg.mimeType) {
      state.clientMimeType = msg.mimeType;
    }
    return;
  }

  if (msg.type === 'vad') {
    return;
  }

  if (msg.type === 'final_pending') {
    if (typeof msg.mimeType === 'string' && msg.mimeType) {
      state.clientMimeType = msg.mimeType;
    }
    state.expectFinalAudio = true;
    if (typeof onFinalPending === 'function') {
      await onFinalPending();
    }
    return;
  }

  sendEvent(buildErrorEvent(`unknown message type: ${msg.type}`));
}

module.exports = {
  WS_PATHS,
  buildErrorEvent,
  buildStatusEvent,
  buildVadTuning,
  createFinalizeHoldController,
  inferWithRetry,
  handleControlMessage,
};
