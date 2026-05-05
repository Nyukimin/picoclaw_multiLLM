import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('viewer exposes memory inspector and news pack UI hooks', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  assert.match(html, /data-tab="memory"/);
  assert.match(html, /id="panel-memory"/);
  assert.match(html, /id="memoryNamespace"/);
  assert.match(html, /id="memoryBody"/);
  assert.match(html, /id="newsPackBody"/);
  assert.match(html, /function refreshMemorySnapshot/);
  assert.ok(html.includes('/viewer/memory/snapshot'));
});
