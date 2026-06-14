import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');

test('viewer runtime-config loads voice chat fields into vdsState', () => {
  assert.match(js, /cfg\.voice_chat_stream_url/);
  assert.match(js, /cfg\.voice_chat_enabled/);
  assert.match(js, /cfg\.voice_input_mode/);
  assert.match(js, /vdsState\.voiceChatURL/);
  assert.match(js, /vdsState\.voiceChatEnabled/);
  assert.match(js, /vdsState\.voiceInputMode/);
});

test('viewer defaults voice input mode to stt_primary', () => {
  assert.match(js, /function normalizeVoiceInputMode\(/);
  assert.match(js, /return 'stt_primary'/);
});

test('viewer vds_sub opens voice-chat websocket with session.start control', () => {
  assert.match(js, /function sendVDSSessionStart\(\)/);
  assert.match(js, /type:\s*'session\.start'/);
  assert.match(js, /format:\s*'pcm16le'/);
  assert.match(js, /voice_input_mode:\s*'vds_sub'/);
  assert.match(js, /prompt:\s*VDS_DEFAULT_PROMPT/);
  assert.match(js, /vdsState\.ws\.send\(JSON\.stringify\(control\)\)/);
  assert.match(js, /function sendVDSSessionCommit\(\)/);
  assert.match(js, /type:\s*'session\.commit'/);
});

test('viewer vds_sub sends binary pcm through voice-chat websocket', () => {
  assert.match(js, /function sendVDSAudioChunk\(pcm16\)/);
  assert.match(js, /vdsState\.ws\.send\(chunk\.buffer\)/);
});

test('viewer vds_sub uses a lower VAD threshold than STT text capture', () => {
  assert.match(js, /const STT_VAD_START_LEVEL = 12/);
  assert.match(js, /const STT_VAD_END_LEVEL = 8/);
  assert.match(js, /const VDS_VAD_START_LEVEL = 4/);
  assert.match(js, /const VDS_VAD_END_LEVEL = 3/);
  const vadStart = js.indexOf('function handleVDSVADFrame');
  assert.ok(vadStart >= 0, 'handleVDSVADFrame not found');
  const vadEnd = js.indexOf('function stopVDSUtteranceBySilence', vadStart);
  assert.ok(vadEnd > vadStart, 'stopVDSUtteranceBySilence block not found');
  const vadSource = js.slice(vadStart, vadEnd);
  assert.match(vadSource, /VDS_VAD_END_LEVEL/);
  assert.match(vadSource, /VDS_VAD_START_LEVEL/);
});

test('viewer vds_sub does not call sendViewerMessage on llm.final success path', () => {
  const handleStart = js.indexOf('function handleVDSFinalMessage(msg)');
  assert.ok(handleStart >= 0, 'handleVDSFinalMessage not found');
  const handleEnd = js.indexOf('function toggleVDS()', handleStart);
  assert.ok(handleEnd > handleStart, 'toggleVDS block not found');
  const handleSource = js.slice(handleStart, handleEnd);
  assert.doesNotMatch(handleSource, /sendViewerMessage\(/);
  assert.doesNotMatch(handleSource, /handleSTTFinalText\(/);
  assert.doesNotMatch(handleSource, /\bsend\(\)/);
});

test('viewer vds_sub renders llm.delta locally before final', () => {
  assert.match(js, /function renderVDSDeltaResponse\(reason\)/);
  assert.match(js, /vdsState\.llmDeltaText \+= String\(msg\.text \|\| ''\)/);
  assert.match(js, /renderVDSDeltaResponse\('stream'\)/);
  assert.match(js, /scheduleVDSDeltaIdleFinalize\(\)/);
  assert.match(js, /vds-local-response/);
});

test('viewer vds_sub final timeout can finalize received delta', () => {
  assert.match(js, /function finalizeVDSDeltaResponse\(reason\)/);
  assert.match(js, /if \(finalizeVDSDeltaResponse\('timeout'\)\) return/);
  assert.match(js, /renderVDSDeltaResponse\('delta_idle'\)/);
  assert.match(js, /const VDS_DELTA_IDLE_FINALIZE_MS = 2500/);
  assert.match(js, /completeVDSUtteranceStop\('local_delta'\)/);
  assert.match(js, /detail:\s*'local_delta:' \+ String\(reason \|\| 'delta'\)/);
});

test('viewer vds_sub does not abort while waiting for final', () => {
  const stopStart = js.indexOf('function stopVDS()');
  assert.ok(stopStart >= 0, 'stopVDS not found');
  const stopEnd = js.indexOf('function abortVDSImmediately', stopStart);
  assert.ok(stopEnd > stopStart, 'abortVDSImmediately block not found');
  const stopSource = js.slice(stopStart, stopEnd);
  assert.match(stopSource, /if \(vdsState\.isStopping\) return/);
});

test('viewer mic button routes through voice input mode dispatcher', () => {
  assert.match(js, /function toggleVoiceInput\(\)/);
  assert.match(js, /if \(isVDSSubMode\(\)\)/);
  assert.match(js, /micBtn\.addEventListener\('click', \(\) => \{\s*interruptIdleChatForUserInput\('stt_button'\);\s*toggleVoiceInput\(\);/s);
});

test('viewer stt_primary keeps STT start control unchanged', () => {
  assert.match(js, /function sendSTTStartControl\(\)/);
  assert.match(js, /type:\s*'start'/);
  assert.doesNotMatch(js, /sendSTTStartControl\([\s\S]*session\.start/);
});
