import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

class FakeElement {
  constructor(id = '') {
    this.id = id;
    this.innerHTML = '';
    this.textContent = '';
    this.style = {};
    this.title = '';
    this.children = [];
  }
  querySelector(selector) {
    if (selector !== 'span') return null;
    if (!this.children.length) this.children.push(new FakeElement('span'));
    return this.children[0];
  }
}

function sourceBetween(source, startNeedle, endNeedle) {
  const start = source.indexOf(startNeedle);
  const end = source.indexOf(endNeedle, start);
  assert.ok(start >= 0, `start not found: ${startNeedle}`);
  assert.ok(end > start, `end not found: ${endNeedle}`);
  return source.slice(start, end);
}

test('viewer exposes memory inspector and news pack UI hooks', () => {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  const opsJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/ops.js', 'utf8');
  const memoryJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/memory.js', 'utf8');
  const newsPackJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/news-pack.js', 'utf8');
  const rolesJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/roles.js', 'utf8');
  const timelineJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/timeline.js', 'utf8');
  const idleChatJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/idlechat.js', 'utf8');
  const css = fs.readFileSync('internal/adapter/viewer/assets/css/viewer.css', 'utf8');
  const opsCss = fs.readFileSync('internal/adapter/viewer/assets/css/tabs/ops.css', 'utf8');
  const viewer = html + '\n' + js + '\n' + opsJs + '\n' + memoryJs + '\n' + newsPackJs + '\n' + rolesJs + '\n' + timelineJs + '\n' + idleChatJs;
  assert.match(html, /data-tab="memory"/);
  assert.match(html, /id="panel-memory"/);
  assert.match(html, /class="theme-modern"/);
  assert.match(html, /class="theme-switcher"/);
  assert.match(html, /data-theme="classic"/);
  assert.match(html, /data-theme="modern"/);
  assert.match(html, /data-theme="compact"/);
  assert.match(html, /id="mobilePanelSelect"/);
  assert.match(html, /id="mobilePanelPrev"/);
  assert.match(html, /id="mobilePanelNext"/);
  assert.match(html, /<option value="idlechat">IdleChat<\/option>/);
  assert.match(html, /id="memoryNamespace"/);
  assert.match(html, /id="memorySession"/);
  assert.match(html, /id="memoryLayerBody"/);
  assert.match(html, /id="memoryEventBody"/);
  assert.match(html, /id="searchCacheBody"/);
  assert.match(html, /id="memoryPromoteKind"/);
  assert.match(html, /id="memoryPromoteID"/);
  assert.match(html, /id="sourceRegistryBody"/);
  assert.match(html, /id="sourceRegistryYAML"/);
  assert.match(html, /id="memoryBody"/);
  assert.match(html, /id="newsPackBody"/);
  assert.match(html, /id="llmMemoryCards"/);
  assert.match(html, /id="llmMemorySystemBar"/);
  assert.match(html, /id="llmMemoryProcessLists"/);
  assert.match(html, /id="llmMemoryRoles"/);
  assert.match(html, /id="llmRuntimeConfigCards"/);
  assert.match(html, /id="llmOpsConfigState"/);
  assert.match(html, /data-tab="news-pack"/);
  assert.match(html, /id="panel-news-pack"/);
  assert.match(html, /id="newsPackDetail"/);
  assert.match(html, /id="newsUsageBody"/);
  assert.match(html, /id="recallTraceBody"/);
  assert.match(html, /<th>Decision<\/th>/);
  assert.match(html, /<th>Reason<\/th>/);
  assert.match(memoryJs, /item\.Decision/);
  assert.match(memoryJs, /item\.Reason/);
  assert.match(memoryJs, /function refreshMemorySnapshot/);
  assert.match(newsPackJs, /function refreshNewsPack/);
  assert.match(newsPackJs, /function renderNewsPackPanel/);
  assert.match(newsPackJs, /function newsUsageCount/);
  assert.match(newsPackJs, /function newsRelatedMemoryMatches/);
  assert.match(html, /id="newsRelatedMemoryBody"/);
  assert.match(memoryJs, /function refreshMemoryLayers/);
  assert.match(memoryJs, /function refreshMemoryEvents/);
  assert.match(memoryJs, /function renderMemoryEvents/);
  assert.match(js, /function applyViewerTheme/);
  assert.match(js, /viewer\.theme/);
  assert.match(js, /function switchAdjacentPanel/);
  assert.match(js, /mobilePanelSelect\.addEventListener\('change'/);
  assert.match(html, /assets\/css\/tabs\/ops\.css/);
  assert.match(html, /assets\/js\/tabs\/ops\.js/);
  assert.match(html, /assets\/js\/tabs\/memory\.js/);
  assert.match(html, /assets\/js\/tabs\/news-pack\.js/);
  assert.match(html, /assets\/js\/tabs\/roles\.js/);
  assert.match(html, /assets\/js\/tabs\/timeline\.js/);
  assert.match(html, /assets\/js\/tabs\/idlechat\.js/);
  assert.match(timelineJs, /function addMsgToTimeline/);
  assert.doesNotMatch(timelineJs, /function addIdleMsgToTimeline/);
  assert.match(idleChatJs, /function addIdleMsgToTimeline/);
  assert.match(idleChatJs, /function appendIdleLiveMessageEvent/);
  assert.doesNotMatch(idleChatJs, /function addMsgToTimeline/);
  assert.match(js, /function setTTSSpeechText/);
  assert.match(js, /function renderChatTTSSpeechText/);
  assert.match(js, /function renderIdleTTSSpeechText/);
  assert.match(opsJs, /function renderLlmMemoryStatus/);
  assert.match(opsJs, /Available RAM/);
  assert.match(opsJs, /Swap Used/);
  assert.match(opsJs, /Memory Pressure/);
  assert.match(opsJs, /Compressed/);
  assert.match(opsJs, /File Cache/);
  assert.match(opsJs, /Wired/);
  assert.match(opsJs, /Top Memory Processes/);
  assert.match(opsJs, /Model Processes/);
  assert.match(opsJs, /Available for LLM/);
  assert.match(opsJs, /Used for LLM/);
  assert.match(opsJs, /Safe Available/);
  assert.match(opsJs, /Safety Margin/);
  assert.match(opsJs, /function memoryGiB/);
  assert.match(opsJs, /function renderMemoryProcessList/);
  assert.match(opsJs, /function renderLocalLLMRuntimeConfig/);
  assert.match(css, /html\{width:100%;max-width:100vw;overflow-x:hidden\}/);
  assert.match(css, /main\{[^}]*max-width:100vw;[^}]*overflow-x:hidden/);
  assert.match(css, /\.panel\{[^}]*max-width:100%/);
  assert.match(opsCss, /#panel-ops,#panel-ops \*\{min-width:0\}/);
  assert.match(opsCss, /\.llm-ops-raw\{[^}]*max-width:100%;[^}]*white-space:pre-wrap;word-break:break-word/);
  assert.match(opsCss, /#llmOpsPanel \.debug-actions\{display:grid;grid-template-columns:1fr;gap:6px\}/);
  assert.match(opsCss, /\.ops-grid,\.llm-memory-grid,\.llm-memory-process-grid,\.llm-runtime-grid\{grid-template-columns:minmax\(0,1fr\)\}/);
  assert.match(viewer, /\/v1\/chat\/completions/);
  assert.match(viewer, /llm_ops_configured/);
  assert.match(viewer, /LLM_OPS_TOKEN missing/);
  assert.match(viewer, /memory\.system/);
  assert.match(viewer, /memory\.llm_by_role/);
  assert.match(memoryJs, /function refreshSourceRegistry/);
  assert.match(memoryJs, /function runSourceRegistryEntry/);
  assert.match(memoryJs, /function refreshRecallTraces/);
  assert.match(html, /data-tab="roles"/);
  assert.match(html, /id="panel-roles"/);
  assert.match(html, /id="roleSelectorBody"/);
  assert.match(html, /id="roleFilter"/);
  assert.match(js, /const ROLE_TARGETS/);
  assert.match(rolesJs, /function renderRoleSelector/);
  assert.match(rolesJs, /function selectRoleTarget/);
  assert.match(rolesJs, /function applyRoleTargetToMessage/);
  assert.match(html, /Chat/);
  assert.match(html, /Worker/);
  assert.match(html, /Wild/);
  assert.ok(rolesJs.includes("return '/ops ' + trimmed"));
  assert.ok(rolesJs.includes("return '/wild ' + trimmed"));
  assert.ok(rolesJs.includes("return '/code2 ' + trimmed"));
  assert.ok(rolesJs.includes("return '/code3 ' + trimmed"));
  assert.ok(rolesJs.includes("return '/code4 ' + trimmed"));
  assert.ok(viewer.includes('/viewer/memory/snapshot'));
  assert.ok(viewer.includes('/viewer/memory/layers'));
  assert.ok(viewer.includes('/viewer/memory/events'));
  assert.ok(viewer.includes('/viewer/source-registry'));
  assert.ok(viewer.includes('/viewer/recall/traces'));
  assert.ok(viewer.includes('/viewer/memory/state'));
  assert.ok(viewer.includes('/viewer/memory/promote'));
  assert.ok(viewer.includes('target_kind'));
  assert.ok(viewer.includes('target_id'));
});

test('viewer renders expanded llm ops memory fields', () => {
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  const opsJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/ops.js', 'utf8');
  const elements = new Map();
  const document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, new FakeElement(id));
      return elements.get(id);
    },
    createElement() {
      return new FakeElement();
    },
  };
  const source = `
function esc(s) { return String(s || ''); }
function escAttr(s) { return String(s || ''); }
const state = {
  ops: {
    llmStatus: {
      memory: {
        system: {
          total_gib: 64,
          used_gib: 40,
          free_gib: 8,
          available_gib: 16,
          swap_used_gib: 0,
          memory_pressure: 'normal',
          compressed_gib: 0,
          file_cache_gib: 10,
          wired_gib: 7,
          available_for_llm_gib: 11.5,
          used_for_llm_gib: 116.5,
          safe_available_for_llm_gib: 3.5,
          llm_safety_margin_gib: 8,
        },
        llm_by_role: {
          Chat: {pid: 111, rss_mib: 2048},
          Worker: {pid: 222, rss_mib: 4096},
        },
        top_memory_processes: [{name: 'python', pid: 123, rss_mib: 1024}],
        model_processes: [{role: 'Chat', model: 'qwen', pid: 111, rss_mib: 2048}],
      },
      roles: {Chat: {health_ok: true}, Worker: {health_ok: true}},
    },
    localLLM: {},
    llmStatusError: '',
  },
};
` + sourceBetween(js, 'function normState', 'function fmt') +
sourceBetween(js, 'function stateClass', 'function bump') +
sourceBetween(opsJs, 'function llmRoleMemoryState', 'async function refreshLlmOpsStatus') + `
renderLlmMemoryStatus();
globalThis.__cards = document.getElementById('llmMemoryCards').innerHTML;
globalThis.__processes = document.getElementById('llmMemoryProcessLists').innerHTML;
`;
  const context = vm.createContext({document});
  vm.runInContext(source, context);

  for (const label of ['Total RAM', 'Used RAM', 'Free RAM', 'Available RAM', 'Swap Used', 'Memory Pressure', 'Compressed', 'File Cache', 'Wired', 'Available for LLM', 'Used for LLM', 'Safe Available', 'Safety Margin']) {
    assert.ok(context.__cards.includes(label), `${label} should render`);
  }
  assert.ok(context.__cards.includes('0.00 GiB'), 'reported zero memory values should render as 0.00 GiB');
  assert.ok(context.__cards.includes('llm-memory-indicator state-running'), 'healthy memory metrics should render OK indicators');
  assert.ok(context.__cards.includes('OK'), 'healthy memory metrics should show OK status text');
  assert.ok(context.__processes.includes('Top Memory Processes'));
  assert.ok(context.__processes.includes('Model Processes'));
  assert.ok(context.__processes.includes('python'));
  assert.ok(context.__processes.includes('qwen'));
});
