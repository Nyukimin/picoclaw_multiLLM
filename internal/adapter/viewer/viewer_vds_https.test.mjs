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
  assert.match(js, /vdsState\.ws\.send\(JSON\.stringify\(control\)\)/);
  assert.match(js, /function sendVDSSessionCommit\(\)/);
  assert.match(js, /type:\s*'session\.commit'/);
});

test('viewer vds_sub sends binary pcm through voice-chat websocket', () => {
  assert.match(js, /function sendVDSAudioChunk\(pcm16\)/);
  assert.match(js, /vdsState\.ws\.send\(chunk\.buffer\)/);
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
