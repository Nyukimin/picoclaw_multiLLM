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

test('viewer sends STT start control before streaming audio chunks', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /function sendSTTStartControl\(\)/);
  assert.match(js, /type:\s*'start'/);
  assert.match(js, /sample_rate:\s*sampleRate/);
  assert.match(js, /channels:\s*1/);
  assert.match(js, /format:\s*'pcm_s16le'/);
  assert.match(js, /sttState\.ws\.send\(JSON\.stringify\(control\)\)/);

  const onopenStart = js.indexOf('sttState.ws.onopen = () => {');
  const onopenEnd = js.indexOf('sttState.ws.onmessage =', onopenStart);
  assert.ok(onopenStart >= 0 && onopenEnd > onopenStart, 'STT onopen block not found');
  const onopenSource = js.slice(onopenStart, onopenEnd);
  assert.match(onopenSource, /sendSTTStartControl\(\);/);

  const startControl = js.indexOf('function sendSTTStartControl()');
  const sendChunk = js.indexOf('function sendSTTAudioChunk(pcm16)');
  assert.ok(sendChunk >= 0 && startControl > sendChunk, 'start control helper should be near chunk sender');
});

test('viewer voice chat sends final text only in normal timeline chat without stopping capture on idle view', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /let activeViewerTab = 'home'/);
  assert.match(js, /function isVoiceChatAllowed\(\) \{\s*return activeViewerTab === 'timeline' && !document\.body\.classList\.contains\('live-mode'\);/);
  const switchTabStart = js.indexOf('function switchTab(tab) {');
  const switchTabEnd = js.indexOf('function switchAdjacentPanel', switchTabStart);
  assert.ok(switchTabStart >= 0 && switchTabEnd > switchTabStart, 'switchTab block not found');
  const switchTabSource = js.slice(switchTabStart, switchTabEnd);
  assert.doesNotMatch(switchTabSource, /stopSTT\(\)/);
  assert.match(js, /micBtn\.disabled = !voiceAllowed && !sttState\.isRecording;/);
  assert.match(js, /if \(!isVoiceChatAllowed\(\)\) \{\s*console\.warn\('\[STT\] Final ignored outside normal chat:', finalText\);/);
});

test('viewer treats Mac STT partial events as recognition drafts without chat fallback', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /type !== 'draft' && type !== 'partial' && type !== 'final'/);
  assert.match(js, /\(msg\.type === 'draft' \|\| msg\.type === 'partial'\) && msg\.text/);
  assert.match(js, /sttState\.lastRecognitionText = String\(msg\.text \|\| ''\)\.trim\(\);/);
  assert.doesNotMatch(js, /handleSTTFinalText\(pendingText\)/);
  assert.doesNotMatch(js, /recordSTTCaptureEvent\('final', pendingText\)/);
});

test('viewer renders partial and final STT captions outside the chat input', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  const css = fs.readFileSync('internal/adapter/viewer/assets/css/viewer.css', 'utf8');
  assert.match(html, /id="sttCaption"/);
  assert.match(js, /const sttCaptionEl = document\.getElementById\('sttCaption'\)/);
  assert.match(js, /function updateSTTCaption\(\)/);
  assert.match(js, /sttCaptionEl\.textContent = '暫定字幕: ' \+ partialText/);
  assert.match(js, /sttCaptionEl\.textContent = '確定字幕: ' \+ finalText/);
  assert.match(js, /sttState\.partialCaptionText = sttState\.lastRecognitionText/);
  assert.match(js, /sttState\.finalCaptionText = sttState\.lastRecognitionText/);
  assert.match(css, /\.stt-caption\.draft/);
  assert.match(css, /\.stt-caption\.final/);
});

test('viewer sends STT stop control and waits for final or error before closing', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /function sendSTTStopControl\(\)/);
  assert.match(js, /sttState\.ws\.send\(JSON\.stringify\(\{ type: 'stop' \}\)\)/);
  assert.match(js, /recordSTTCaptureEvent\('stop', 'requested'\)/);
  assert.match(js, /function scheduleSTTFinalWaitTimeout\(\)/);
  assert.match(js, /timed out waiting for final/);
  assert.match(js, /function completeSTTStop\(\)/);

  const stopStart = js.indexOf('function stopSTT()');
  const stopEnd = js.indexOf('function completeSTTStop()', stopStart);
  assert.ok(stopStart >= 0 && stopEnd > stopStart, 'stopSTT block not found');
  const stopSource = js.slice(stopStart, stopEnd);
  assert.match(stopSource, /flushSTTAudioChunkBuffer\(\);/);
  assert.match(stopSource, /sendSTTStopControl\(\);/);
  assert.match(stopSource, /scheduleSTTFinalWaitTimeout\(\);/);
  assert.doesNotMatch(stopSource, /handleSTTFinalText/);

  const finalStart = js.indexOf("} else if (msg.type === 'final') {");
  const finalEnd = js.indexOf("} else if (msg.type === 'reply_reset')", finalStart);
  assert.ok(finalStart >= 0 && finalEnd > finalStart, 'final message block not found');
  const finalSource = js.slice(finalStart, finalEnd);
  assert.match(finalSource, /handleSTTFinalText\(sttState\.lastRecognitionText\)/);
  assert.match(finalSource, /sttState\.ws\.close\(\)/);
});

test('viewer STT autotest uses runtime STT base URL for provider inference', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  assert.match(js, /sttBaseURL:\s*''/);
  assert.match(js, /cfg\.stt_base_url/);
  assert.match(js, /function buildSTTProviderURLForAutoTest\(\)/);
  assert.match(js, /base \+ '\/v1\/audio\/transcriptions'/);
  assert.match(js, /provider_url: providerURL/);
});

test('viewer renders live microphone input level on the mic button', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  const css = fs.readFileSync('internal/adapter/viewer/assets/css/viewer.css', 'utf8');
  assert.match(js, /inputLevel:\s*0/);
  assert.match(js, /function calculateSTTInputLevel\(pcm16\)/);
  assert.match(js, /updateSTTInputLevel\(calculateSTTInputLevel\(pcm16\)\)/);
  assert.match(js, /micBtn\.style\.setProperty\('--mic-level-pct'/);
  assert.match(js, /updateSTTInputLevel\(0\);/);
  assert.match(css, /#micBtn\.has-level/);
  assert.match(css, /var\(--mic-level-pct\)/);
});
