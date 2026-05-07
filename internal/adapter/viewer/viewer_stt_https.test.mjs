import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('viewer STT websocket can be configured for Mac streaming endpoint', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  assert.match(html, /fetch\('\/viewer\/runtime-config'/);
  assert.match(html, /cfg\.stt_stream_url/);
  assert.match(html, /voiceBridgeURL:\s*`\$\{window\.location\.protocol === 'https:' \? 'wss:' : 'ws:'\}\/\/\$\{window\.location\.host\}\/stt`/);
});

test('viewer microphone input is the STT production entrypoint', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  assert.match(html, /navigator\.mediaDevices\.getUserMedia\(\{/);
  assert.match(html, /type:\s*'start'/);
  assert.match(html, /type:\s*'audio'/);
  assert.match(html, /format:\s*'pcm_s16le'/);
  assert.match(html, /sttState\.ws\.send\(JSON\.stringify\(\{ type: 'stop' \}\)\)/);
  assert.match(html, /sendViewerMessage\(message\)/);
});
