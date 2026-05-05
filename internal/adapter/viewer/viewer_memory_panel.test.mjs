import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('viewer exposes memory inspector and news pack UI hooks', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  assert.match(html, /data-tab="memory"/);
  assert.match(html, /id="panel-memory"/);
  assert.match(html, /id="memoryNamespace"/);
  assert.match(html, /id="memorySession"/);
  assert.match(html, /id="memoryLayerBody"/);
  assert.match(html, /id="sourceRegistryBody"/);
  assert.match(html, /id="sourceRegistryYAML"/);
  assert.match(html, /id="memoryBody"/);
  assert.match(html, /id="newsPackBody"/);
  assert.match(html, /id="recallTraceBody"/);
  assert.match(html, /function refreshMemorySnapshot/);
  assert.match(html, /function refreshMemoryLayers/);
  assert.match(html, /function refreshSourceRegistry/);
  assert.match(html, /function refreshRecallTraces/);
  assert.ok(html.includes('/viewer/memory/snapshot'));
  assert.ok(html.includes('/viewer/memory/layers'));
  assert.ok(html.includes('/viewer/source-registry'));
  assert.ok(html.includes('/viewer/recall/traces'));
  assert.ok(html.includes('/viewer/memory/state'));
  assert.ok(html.includes('/viewer/memory/promote'));
});
