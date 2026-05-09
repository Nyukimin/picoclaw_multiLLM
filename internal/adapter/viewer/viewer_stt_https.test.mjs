import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('viewer STT websocket can be configured for Mac streaming endpoint', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /fetch\('\/viewer\/runtime-config'/);
  assert.match(js, /cfg\.stt_stream_url/);
  assert.match(js, /voiceBridgeURL:\s*`\$\{window\.location\.protocol === 'https:' \? 'wss:' : 'ws:'\}\/\/\$\{window\.location\.host\}\/stt`/);
});

test('viewer microphone input is the STT production entrypoint', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /navigator\.mediaDevices\.getUserMedia\(\{/);
  assert.match(js, /sampleRate:\s*16000/);
  assert.match(js, /chunkSamples:\s*1600/);
  assert.match(js, /resampleToPCM16\(pcm, sttState\.inputSampleRate \|\| 48000, 16000\)/);
  assert.match(js, /sttState\.ws\.send\(chunk\.buffer\)/);
  assert.match(js, /sttState\.ws\.close\(\)/);
  assert.match(js, /sendViewerMessage\(message\)/);
});

test('viewer voice chat sends final text only in normal timeline chat without stopping capture on idle view', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /let activeViewerTab = 'timeline'/);
  assert.match(js, /function isVoiceChatAllowed\(\) \{\s*return activeViewerTab === 'timeline' && !document\.body\.classList\.contains\('live-mode'\);/);
  const switchTabStart = js.indexOf('function switchTab(tab) {');
  const switchTabEnd = js.indexOf('function switchAdjacentPanel', switchTabStart);
  assert.ok(switchTabStart >= 0 && switchTabEnd > switchTabStart, 'switchTab block not found');
  const switchTabSource = js.slice(switchTabStart, switchTabEnd);
  assert.doesNotMatch(switchTabSource, /stopSTT\(\)/);
  assert.match(js, /micBtn\.disabled = !voiceAllowed && !sttState\.isRecording;/);
  assert.match(js, /if \(!isVoiceChatAllowed\(\)\) \{\s*console\.warn\('\[STT\] Final ignored outside normal chat:', finalText\);/);
});
