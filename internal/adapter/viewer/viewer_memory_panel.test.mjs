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
  appendChild(child) {
    this.children.push(child);
    this.innerHTML += child.innerHTML || child.textContent || '';
    return child;
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
  assert.match(html, /data-chat-route="worker">Worker/);
  assert.match(html, /data-chat-route="heavy">Heavy/);
  assert.match(html, /data-chat-route="wild">Wild/);
  assert.match(html, /id="memoryNamespace"/);
  assert.match(html, /id="memorySession"/);
  assert.match(html, /id="memoryLayerBody"/);
  assert.match(html, /id="memoryEventBody"/);
  assert.match(html, /id="searchCacheBody"/);
  assert.match(html, /id="knowledgeMemoryBody"/);
  assert.match(html, /id="knowledgeMemoryDetail"/);
  assert.match(html, /id="knowledgeMemoryTypeFilter"/);
  assert.match(html, /id="knowledgeMemoryReviewFilter"/);
  assert.match(html, /id="knowledgeMemoryFlagFilter"/);
  assert.match(html, /id="knowledgePersonalCount"/);
  assert.match(html, /id="knowledgeSourceCount"/);
  assert.match(html, /id="knowledgeDreamCount"/);
  assert.match(html, /id="memoryPromoteKind"/);
  assert.match(html, /id="memoryPromoteID"/);
  assert.match(html, /id="sourceRegistryBody"/);
  assert.match(html, /id="sourceRegistryRunStatus"/);
  assert.match(html, /id="sourceRegistryYAML"/);
  assert.match(html, /id="memoryBody"/);
  assert.match(html, /id="newsPackBody"/);
  assert.match(html, /id="llmMemoryCards"/);
  assert.match(html, /id="llmMemorySystemBar"/);
  assert.match(html, /id="llmMemoryProcessLists"/);
  assert.match(html, /id="llmMemoryRoles"/);
  assert.match(html, /id="llmRuntimeConfigCards"/);
  assert.match(html, /id="llmOpsConfigState"/);
  assert.match(html, /id="toolHarnessBody"/);
  assert.match(html, /id="dciTraceBody"/);
  assert.match(html, /id="dciSearchInput"/);
  assert.match(html, /id="dciSearchBtn"/);
  assert.match(html, /id="dciSearchResult"/);
  assert.match(html, /id="sandboxBody"/);
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
  assert.match(memoryJs, /function refreshKnowledgeMemoryLedger/);
  assert.match(memoryJs, /function fetchMemoryKnowledgeDetail/);
  assert.match(memoryJs, /function relatedSourceRegistryStagingItems/);
  assert.match(memoryJs, /function relatedKnowledgeMemoryRows/);
  assert.match(html, /Related Staging/);
  assert.match(html, /<th>Knowledge<\/th>/);
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
  assert.match(timelineJs, /function applyChatRouteAliasToMessage/);
  assert.match(timelineJs, /function buildViewerSendRequest/);
  assert.match(timelineJs, /function ensureViewerLLMReadyForRequest/);
  assert.match(timelineJs, /function viewerLLMStopRolesBeforeStart/);
  assert.match(timelineJs, /\/viewer\/llm-ops\/health/);
  assert.match(timelineJs, /\/viewer\/llm-ops\/stop/);
  assert.match(timelineJs, /\/viewer\/llm-ops\/start/);
  assert.match(timelineJs, /worker: \{label: 'Worker', baseURL: 'http:\/\/127\.0\.0\.1:8082', model: 'Worker', routePrefix: '\/ops'\}/);
  assert.match(timelineJs, /heavy: \{label: 'Heavy', baseURL: 'http:\/\/127\.0\.0\.1:8083', model: 'Heavy', routePrefix: '\/analyze'\}/);
  assert.match(timelineJs, /wild: \{label: 'Wild', baseURL: 'http:\/\/127\.0\.0\.1:8084', model: 'Wild', routePrefix: '\/wild'\}/);
  assert.match(timelineJs, /function syncChatRouteAliasesFromRuntimeConfig/);
  assert.match(js, /const body = buildViewerSendRequest\(message\)/);
  assert.match(js, /await ensureViewerLLMReadyForRequest\(body\)/);
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
  assert.match(opsJs, /function renderToolHarnessEvents/);
  assert.match(opsJs, /function toolHarnessOpsCard/);
  assert.match(opsJs, /function renderDCITraces/);
  assert.match(opsJs, /function dciOpsCard/);
  assert.match(opsJs, /function bindDCISearchControls/);
  assert.match(opsJs, /\/viewer\/dci\/search/);
  assert.match(opsJs, /function renderSandboxStatus/);
  assert.match(opsJs, /function sandboxOpsCard/);
  assert.match(opsJs, /sandboxArtifacts/);
  assert.match(opsJs, /sandboxGateLogs/);
  assert.match(opsJs, /function previewSandboxPromotion/);
  assert.match(opsJs, /sandbox promotion diff preview/);
  assert.match(opsJs, /function sandboxDiffRiskFlags/);
  assert.match(opsJs, /risk flags/);
  assert.match(opsJs, /manual review/);
  assert.ok(viewer.includes('/viewer/sandbox/promotions/preview'));
  assert.match(opsJs, /function skillGovernanceOpsCard/);
  assert.match(opsJs, /skillManifests/);
  assert.match(opsJs, /coderTranscripts/);
  assert.match(opsJs, /skill_trigger_missed requires review/);
  assert.match(opsJs, /function workstreamOpsCard/);
  assert.match(opsJs, /function latestWorkstreamVaultUpdates/);
  assert.match(opsJs, /function renderWorkstreamVaultReviews/);
  assert.match(opsJs, /function reviewWorkstreamVaultUpdate/);
  assert.match(opsJs, /function formatWorkstreamVaultPreview/);
  assert.match(opsJs, /preview side-by-side/);
  assert.match(opsJs, /function revenueOpsCard/);
  assert.match(opsJs, /function latestPersonaMetaProfileUpdates/);
  assert.match(opsJs, /function renderPersonaMetaReviews/);
  assert.match(opsJs, /function reviewPersonaMetaUpdate/);
  assert.match(opsJs, /function complexityHotspotOpsCard/);
  assert.match(opsJs, /function superAgentOpsCard/);
  assert.match(opsJs, /function heavyWorkerRuntimeOpsCard/);
  assert.match(opsJs, /function knowledgeMemoryOpsCard/);
  assert.match(opsJs, /function fetchKnowledgeMemoryDetail/);
  assert.match(opsJs, /Knowledge Memory Detail/);
  assert.match(viewer, /refreshHeavyWorkerRuntimeDiagnostics/);
  assert.match(viewer, /\/viewer\/ai-workflow\/heavy-worker\/runtime-diagnostics/);
  assert.match(opsJs, /workstreamGoals/);
  assert.match(css, /html\{width:100%;max-width:100vw;overflow-x:hidden\}/);
  assert.match(css, /linear-gradient\(135deg,#050713/);
  assert.match(css, /body::after/);
  assert.match(css, /repeating-linear-gradient/);
  assert.match(css, /backdrop-filter:blur/);
  assert.match(css, /\.lipsync-stage\{/);
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
  assert.match(memoryJs, /function renderSourceRegistryRunStatus/);
  assert.match(memoryJs, /function refreshSourceRegistryStaging/);
  assert.match(memoryJs, /function validateSourceRegistryStaging/);
  assert.match(memoryJs, /function promoteSourceRegistryStaging/);
  assert.match(html, /id="sourceRegistryStagingBody"/);
  assert.match(memoryJs, /sourceRegistryLastRun/);
  assert.match(memoryJs, /warnings=/);
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
  assert.ok(viewer.includes('/viewer/tool-harness/recent'));
  assert.ok(viewer.includes('/viewer/dci/recent'));
  assert.ok(viewer.includes('/viewer/sandbox'));
  assert.ok(viewer.includes('/viewer/skill-governance/recent'));
  assert.ok(viewer.includes('missed triggers'));
  assert.ok(viewer.includes('coder_transcripts'));
  assert.ok(viewer.includes('/viewer/workstreams'));
  assert.ok(viewer.includes('workstreamGoals'));
  assert.ok(viewer.includes('workstreamArtifacts'));
  assert.ok(viewer.includes('workstreamSteering'));
  assert.ok(viewer.includes('workstreamHeartbeats'));
  assert.ok(viewer.includes('workstreamVaultUpdates'));
  assert.ok(viewer.includes('workstreamVaultReviewResult'));
  assert.ok(viewer.includes('workstreamVaultPreviewResult'));
  assert.ok(viewer.includes('/viewer/workstreams/vault-updates/review'));
  assert.ok(viewer.includes('/viewer/workstreams/vault-updates/preview'));
  assert.ok(viewer.includes('Workstream Vault Review'));
  assert.ok(viewer.includes('function previewWorkstreamVaultUpdate'));
  assert.ok(viewer.includes('/viewer/revenue'));
  assert.ok(viewer.includes('/viewer/revenue/human-decision-gate/review'));
  assert.ok(viewer.includes('revenueProducts'));
  assert.ok(viewer.includes('revenueSummary'));
  assert.ok(viewer.includes('kpi_trend'));
  assert.ok(viewer.includes('product_sales'));
  assert.ok(viewer.includes('customer_voice_types'));
  assert.ok(viewer.includes('revenueChannelDrafts'));
  assert.ok(viewer.includes('channel_drafts'));
  assert.ok(viewer.includes('channel drafts'));
  assert.ok(viewer.includes('Revenue Channel Drafts'));
  assert.ok(viewer.includes('revenueChannelDraftBody'));
  assert.ok(viewer.includes('function renderRevenueChannelDrafts'));
  assert.ok(viewer.includes('external_send_applied'));
  assert.ok(viewer.includes('draft only'));
  assert.ok(viewer.includes('Revenue Drilldown'));
  assert.ok(viewer.includes('revenueDrilldownResult'));
  assert.ok(viewer.includes('function renderRevenueDrilldown'));
  assert.ok(viewer.includes('function revenueDrilldownLines'));
  assert.ok(viewer.includes('KPI trend graph'));
  assert.ok(viewer.includes('Product sales graph'));
  assert.ok(viewer.includes('Customer voice graph'));
  assert.ok(viewer.includes('revenueHumanDecisions'));
  assert.ok(viewer.includes('revenueDecisionReviewResult'));
  assert.ok(viewer.includes('function reviewRevenueHumanDecision'));
  assert.ok(viewer.includes('paid events'));
  assert.ok(viewer.includes('human decisions pending'));
  assert.ok(viewer.includes('Revenue Human Decision Gate'));
  assert.ok(viewer.includes('/viewer/persona-observation'));
  assert.ok(viewer.includes('personaObservationLogs'));
  assert.ok(viewer.includes('personaMetaProfileUpdates'));
  assert.ok(viewer.includes('personaMetaReviewResult'));
  assert.ok(viewer.includes('/viewer/persona-observation/meta-updates/review'));
  assert.ok(viewer.includes('Persona Meta Review'));
  assert.ok(viewer.includes('Persona Observation'));
  assert.ok(viewer.includes('/viewer/browser-trace-api'));
  assert.ok(viewer.includes('/viewer/browser-trace-api/fetcher-proposals'));
  assert.ok(viewer.includes('browserTraceAPICandidates'));
  assert.ok(viewer.includes('browserTraceAPIArtifacts'));
  assert.ok(viewer.includes('Browser Trace API'));
  assert.ok(viewer.includes('fetcher proposals'));
  assert.ok(viewer.includes('/viewer/complexity-hotspots'));
  assert.ok(viewer.includes('complexityHotspots'));
  assert.ok(viewer.includes('Complexity Hotspots'));
  assert.ok(viewer.includes('/viewer/superagent'));
  assert.ok(viewer.includes('superAgentRuns'));
  assert.ok(viewer.includes('superAgentRunQueue'));
  assert.ok(viewer.includes('SuperAgent Harness'));
  assert.ok(opsJs.includes('run queue'));
  assert.ok(viewer.includes('/viewer/knowledge-memory'));
  assert.ok(viewer.includes('/viewer/knowledge-memory/review'));
  assert.ok(viewer.includes('detail_type'));
  assert.ok(viewer.includes('reviewKnowledgeMemoryItem'));
  assert.ok(viewer.includes('Review / Promote Comparison'));
  assert.ok(viewer.includes('Review Result'));
  assert.ok(viewer.includes('knowledgePersonalArchive'));
  assert.ok(viewer.includes('Knowledge Memory'));
  assert.ok(viewer.includes('vault updates'));
  assert.ok(viewer.includes('approval pending'));
  assert.ok(viewer.includes('artifacts'));
  assert.ok(viewer.includes('gate_logs'));
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

test('viewer renders knowledge memory ledger inside memory tab', () => {
  const memoryJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/memory.js', 'utf8');
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
function esc(s) { return String(s || '').replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function short(s, n) { const v = String(s || ''); return v.length > n ? v.slice(0, n) + '...' : v; }
function fdt(s) { return String(s || '-'); }
const state = {memory: {knowledgeMemory: {
  personal_archive: [{entry_id: 'pa_1', title: 'BIO original', user_id: 'ren', source_ref: 'stg_personal', original_text: 'raw bio', compressed_summary: 'bio digest', protected: true, review_status: 'protected', security_warnings: ['prompt-like text']}],
  creative_knowledge: [{item_id: 'ck_1', title: '映画知識', source_id: 'src_movie', summary: 'movie digest'}],
  news_knowledge: [{item_id: 'news_1', topic: 'tech news', source_url: 'https://example.com/news', meta: {security_warnings: ['warn']}}],
  daily_intake_rules: [{rule_id: 'rule_1', title: 'daily tech', enabled: true}],
  temporal_markers: [{marker_id: 'tm_1', summary: 'one week memory'}],
  dream_runs: [{run_id: 'dream_1', topic: 'dream consolidation', review_status: 'pending'}],
}, sourceRegistryStaging: [
  {id: 'stg_personal', validation_status: 'pending'},
  {id: 'stg_movie', source_id: 'src_movie', validation_status: 'pending'},
], knowledgeMemoryDetail: {detail_type: 'personal_archive', id: 'pa_1', item: {entry_id: 'pa_1', source_ref: 'stg_personal', original_text: 'raw bio', compressed_summary: 'bio digest', protected: true, review_status: 'protected', security_warnings: ['prompt-like text']}}}};
` + sourceBetween(memoryJs, 'function knowledgeMemoryID', 'function refreshMemoryEvents') + `
renderKnowledgeMemoryLedger();
globalThis.__body = document.getElementById('knowledgeMemoryBody').innerHTML;
globalThis.__detail = document.getElementById('knowledgeMemoryDetail').innerHTML;
globalThis.__personal = document.getElementById('knowledgePersonalCount').textContent;
globalThis.__source = document.getElementById('knowledgeSourceCount').textContent;
globalThis.__dream = document.getElementById('knowledgeDreamCount').textContent;
`;
  const context = vm.createContext({document});
  vm.runInContext(source, context);

  assert.equal(context.__personal, '1');
  assert.equal(context.__source, '4');
  assert.equal(context.__dream, '1');
  assert.match(context.__body, /personal_archive/);
  assert.match(context.__body, /creative_knowledge/);
  assert.match(context.__body, /daily_intake_rule/);
  assert.match(context.__body, /original/);
  assert.match(context.__body, /protected/);
  assert.match(context.__body, /compressed/);
  assert.match(context.__body, /warning/);
  assert.match(context.__body, /stg_movie/);
  assert.match(context.__body, /Detail/);
  assert.match(context.__detail, /Original \/ Protected/);
  assert.match(context.__detail, /Compressed \/ Summary/);
  assert.match(context.__detail, /Warning \/ Review/);
  assert.match(context.__detail, /Review \/ Promote Comparison/);
  assert.match(context.__detail, /raw bio/);
  assert.match(context.__detail, /bio digest/);
  assert.match(context.__detail, /warnings=1/);
  assert.match(context.__detail, /promote_blockers=warnings, protected_original, related_staging_not_validated/);
  assert.match(context.__detail, /related_validated=0/);
  assert.match(context.__detail, /prompt-like text/);
  assert.match(context.__detail, /Related Source Registry Staging/);
  assert.match(context.__detail, /stg_personal/);
});

test('viewer renders source registry staging related knowledge column', () => {
  const memoryJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/memory.js', 'utf8');
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
function esc(s) { return String(s || '').replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function short(s, n) { const v = String(s || ''); return v.length > n ? v.slice(0, n) + '...' : v; }
function fdt(s) { return String(s || '-'); }
const state = {memory: {
  knowledgeMemory: {
    creative_knowledge: [{item_id: 'ck_1', title: '映画知識', source_id: 'src_movie', summary: 'movie digest'}],
    news_knowledge: [{item_id: 'news_1', topic: 'tech news', source_url: 'https://example.com/news'}],
  },
  sourceRegistryStaging: [
    {id: 'stg_movie', source_id: 'src_movie', validation_status: 'pending', kind: 'external_fetch', namespace: 'kb:creative', summary_draft: 'movie source'},
    {id: 'stg_news', source_url: 'https://example.com/news', validation_status: 'pending', kind: 'external_fetch', namespace: 'kb:news', summary_draft: 'news source'},
  ],
}};
` + sourceBetween(memoryJs, 'function knowledgeMemoryID', 'function setSourceRegistryStagingStatus') + `
renderSourceRegistryStaging();
globalThis.__body = document.getElementById('sourceRegistryStagingBody').innerHTML;
`;
  const context = vm.createContext({document, validateSourceRegistryStaging() {}, promoteSourceRegistryStaging() {}});
  vm.runInContext(source, context);

  assert.match(context.__body, /creative_knowledge:ck_1/);
  assert.match(context.__body, /news_knowledge:news_1/);
});

test('viewer renders source registry warning run status', () => {
  const memoryJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/memory.js', 'utf8');
  const elements = new Map();
  const document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, new FakeElement(id));
      return elements.get(id);
    },
  };
  const source = `
function esc(s) { return String(s || '').replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
const state = {memory: {sourceRegistryLastRun: {result: {Staged: 1, Validated: 1, Warnings: 2}}}};
` + sourceBetween(memoryJs, 'function renderSourceRegistryRunStatus', 'function runSourceRegistryEntry') + `
renderSourceRegistryRunStatus();
globalThis.__status = document.getElementById('sourceRegistryRunStatus').innerHTML;
`;
  const context = vm.createContext({document});
  vm.runInContext(source, context);

  assert.match(context.__status, /Source Registry run/);
  assert.match(context.__status, /warnings=2/);
  assert.match(context.__status, /badge warn/);
});

test('viewer renders revenue drilldown graph lines from dashboard summary', () => {
  const opsJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/ops.js', 'utf8');
  const elements = new Map();
  const document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, new FakeElement(id));
      return elements.get(id);
    },
  };
  const source = `
function esc(s) { return String(s || ''); }
function escAttr(s) { return String(s || ''); }
function short(s, n) { const v = String(s || ''); return v.length > n ? v.slice(0, n) + '...' : v; }
function ftime(s) { return String(s || '-'); }
function stateClass(s) { return String(s || ''); }
function sandboxField(obj, snake, pascal) {
  if (!obj) return undefined;
  if (Object.prototype.hasOwnProperty.call(obj, snake)) return obj[snake];
  if (Object.prototype.hasOwnProperty.call(obj, pascal)) return obj[pascal];
  return undefined;
}
const state = {ops: {
  revenueSummary: {
    total_revenue_amount: 3000,
    paid_customer_count: 2,
    pending_decision_count: 1,
    channel_draft_count: 1,
    kpi_trend: [
      {date: '2026-05-17', revenue_amount: 1000, post_count: 2, voice_count: 1},
      {date: '2026-05-18', revenue_amount: 3000, post_count: 3, voice_count: 2},
    ],
    product_sales: [{product_id: 'prod_1', product_name: '低単価商品', revenue_amount: 3000, sales_count: 2}],
    customer_voice_types: [{voice_type: 'blocker', count: 3}],
  },
  revenueHumanDecisions: [{decision_id: 'dec_1', decision_type: 'external_publish', approval_status: 'pending', gate_status: 'needs_review'}],
  revenueChannelDrafts: [{draft_id: 'draft_1', approval_status: 'pending'}],
}};
` + sourceBetween(opsJs, 'function revenueOpsCard', 'async function reviewRevenueHumanDecision') + `
renderRevenueDrilldown();
globalThis.__drilldown = document.getElementById('revenueDrilldownResult').textContent;
`;
  const context = vm.createContext({document});
  vm.runInContext(source, context);

  assert.match(context.__drilldown, /Revenue Drilldown/);
  assert.match(context.__drilldown, /KPI trend graph/);
  assert.match(context.__drilldown, /2026-05-18/);
  assert.match(context.__drilldown, /Product sales graph/);
  assert.match(context.__drilldown, /低単価商品/);
  assert.match(context.__drilldown, /Customer voice graph/);
  assert.match(context.__drilldown, /blocker/);
  assert.match(context.__drilldown, /Decision drilldown/);
  assert.match(context.__drilldown, /dec_1/);
});

test('viewer runtime cards prefer live llm ops status over local config labels', () => {
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
function stateClass(s) { return 'state-' + s; }
var state = {
  ops: {
    localLLM: {
      enabled: true,
      provider: 'local_openai',
      chat_base_url: 'http://192.168.1.31:8081',
      worker_base_url: 'http://192.168.1.31:8082',
      heavy_base_url: 'http://192.168.1.31:8082',
      wild_base_url: 'http://192.168.1.31:8081',
      chat_model: 'Chat',
      worker_model: 'Worker',
      heavy_model: 'Worker',
      wild_model: 'Chat',
    },
    llmStatus: {
      roles: {
        Chat: {health_ok: true, halted: false},
        Worker: {health_ok: false, halted: true},
        Heavy: {health_ok: true, halted: false},
        Wild: {health_ok: false, halted: true},
      },
      memory: {
        llm_by_role: {
          Chat: {role: 'Chat', model: '/models/gemma', port: 8081, pid: 30289, rss_mib: 707.47},
          Worker: {role: 'Worker', model: '/models/qwen-vl', port: 8082, pid: null, rss_mib: null},
          Heavy: {role: 'Heavy', model: '/models/qwen-heavy', port: 8083, pid: 46923, rss_mib: 49971.38},
          Wild: {role: 'Wild', model: '/models/qwen-wild', port: 8084, pid: null, rss_mib: null},
        },
      },
    },
  },
};
` + sourceBetween(js, 'function normState', 'function fmt') +
sourceBetween(opsJs, 'function renderLocalLLMRuntimeConfig', 'function setLlmOpsStatusPre') + `
renderLocalLLMRuntimeConfig();
globalThis.__runtime = document.getElementById('llmRuntimeConfigCards').innerHTML;
`;
  const context = vm.createContext({document});
  vm.runInContext(source, context);

  assert.ok(context.__runtime.includes('Worker'));
  assert.ok(context.__runtime.includes('Heavy'));
  assert.ok(context.__runtime.includes('halted'), 'Worker should not be shown as running when llm-ops marks it halted');
  assert.ok(context.__runtime.includes('/models/qwen-heavy'), 'Heavy should use live status model, not stale local config label');
  assert.ok(context.__runtime.includes('http://192.168.1.31:8083'), 'Heavy should use live status port');
});

test('viewer chat route alias builds llm switch request fields', () => {
  const timelineJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/timeline.js', 'utf8');
  const store = new Map();
  const context = vm.createContext({
    document: {querySelectorAll: () => []},
    localStorage: {
      getItem: (key) => store.get(key) || null,
      setItem: (key, value) => store.set(key, String(value)),
      removeItem: (key) => store.delete(key),
    },
  });
  vm.runInContext(timelineJs, context);

  vm.runInContext("localStorage.setItem('chatRouteAlias.selected', 'heavy')", context);
  const heavyReq = JSON.parse(vm.runInContext("JSON.stringify(buildViewerSendRequest('原因を調べて'))", context));
  assert.deepEqual(heavyReq, {
    message: '原因を調べて',
    model_alias: 'Heavy',
    base_url: 'http://127.0.0.1:8083',
    model: 'Heavy',
    route_prefix: '/analyze',
  });

  const explicitReq = JSON.parse(vm.runInContext("JSON.stringify(buildViewerSendRequest('/wild 物語にして'))", context));
  assert.deepEqual(explicitReq, {message: '/wild 物語にして'});
});

test('viewer chat route aliases follow runtime local llm config', () => {
  const timelineJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/timeline.js', 'utf8');
  const store = new Map();
  const context = vm.createContext({
    document: {querySelectorAll: () => []},
    localStorage: {
      getItem: (key) => store.get(key) || null,
      setItem: (key, value) => store.set(key, String(value)),
      removeItem: (key) => store.delete(key),
    },
  });
  vm.runInContext(timelineJs, context);

  vm.runInContext(`syncChatRouteAliasesFromRuntimeConfig({
    enabled: true,
    worker_base_url: 'http://192.168.1.31:8082',
    worker_model: 'WorkerRuntime',
    heavy_base_url: 'http://192.168.1.31:8083',
    heavy_model: 'HeavyRuntime',
    wild_base_url: 'http://192.168.1.31:8084',
    wild_model: 'WildRuntime'
  })`, context);
  vm.runInContext("localStorage.setItem('chatRouteAlias.selected', 'heavy')", context);
  const heavyReq = JSON.parse(vm.runInContext("JSON.stringify(buildViewerSendRequest('原因を調べて'))", context));
  assert.deepEqual(heavyReq, {
    message: '原因を調べて',
    model_alias: 'Heavy',
    base_url: 'http://192.168.1.31:8083',
    model: 'HeavyRuntime',
    route_prefix: '/analyze',
  });
});

test('viewer starts selected llm before sending alias request', async () => {
  const timelineJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/timeline.js', 'utf8');
  const calls = [];
  const context = vm.createContext({
    document: {querySelectorAll: () => []},
    localStorage: {getItem: () => null, setItem: () => {}, removeItem: () => {}},
    fetch: async (url, opts = {}) => {
      calls.push({url, opts});
      if (url === '/viewer/llm-ops/health') {
        return {
          ok: true,
          json: async () => ({status: 'ok', daemon: 'llm-mgmt'}),
          text: async () => '{"status":"ok","daemon":"llm-mgmt"}',
        };
      }
      if (url === '/viewer/llm-ops/status') {
        return {
          ok: true,
          json: async () => ({roles: {Chat: {health_ok: true}, Heavy: {health_ok: false}}}),
        };
      }
      if (url === '/viewer/llm-ops/stop') {
        return {ok: true, text: async () => '{"stopped":["Worker","Wild"],"halted":true}'};
      }
      if (url === '/viewer/llm-ops/start') {
        return {ok: true, text: async () => '{"ok_all":true}'};
      }
      throw new Error('unexpected fetch: ' + url);
    },
    refreshLlmOpsStatus: () => {},
  });
  vm.runInContext(timelineJs, context);

  await vm.runInContext(`ensureViewerLLMReadyForRequest({
    message: '原因を調べて',
    model_alias: 'Heavy',
    base_url: 'http://127.0.0.1:8083',
    model: 'Heavy',
    route_prefix: '/analyze'
  })`, context);

  assert.equal(calls.length, 4);
  assert.equal(calls[0].url, '/viewer/llm-ops/health');
  assert.equal(calls[1].url, '/viewer/llm-ops/status');
  assert.equal(calls[2].url, '/viewer/llm-ops/stop');
  assert.equal(calls[2].opts.method, 'POST');
  assert.deepEqual(JSON.parse(calls[2].opts.body), {roles: ['Worker', 'Wild']});
  assert.equal(calls[3].url, '/viewer/llm-ops/start');
  assert.equal(calls[3].opts.method, 'POST');
  assert.deepEqual(JSON.parse(calls[3].opts.body), {selection: 'Heavy'});
});

test('viewer stops Worker and Heavy before starting Wild', async () => {
  const timelineJs = fs.readFileSync('internal/adapter/viewer/assets/js/tabs/timeline.js', 'utf8');
  const calls = [];
  const context = vm.createContext({
    document: {querySelectorAll: () => []},
    localStorage: {getItem: () => null, setItem: () => {}, removeItem: () => {}},
    fetch: async (url, opts = {}) => {
      calls.push({url, opts});
      if (url === '/viewer/llm-ops/health') {
        return {ok: true, json: async () => ({status: 'ok'}), text: async () => '{"status":"ok"}'};
      }
      if (url === '/viewer/llm-ops/status') {
        return {ok: true, json: async () => ({roles: {Chat: {health_ok: true}, Worker: {health_ok: true}, Heavy: {health_ok: false}, Wild: {health_ok: false}}})};
      }
      if (url === '/viewer/llm-ops/stop') {
        return {ok: true, text: async () => '{"stopped":["Worker","Heavy"],"halted":true}'};
      }
      if (url === '/viewer/llm-ops/start') {
        return {ok: true, text: async () => '{"ok_all":true}'};
      }
      throw new Error('unexpected fetch: ' + url);
    },
    refreshLlmOpsStatus: () => {},
  });
  vm.runInContext(timelineJs, context);

  await vm.runInContext(`ensureViewerLLMReadyForRequest({
    message: '物語にして',
    model_alias: 'Wild',
    base_url: 'http://127.0.0.1:8084',
    model: 'Wild',
    route_prefix: '/wild'
  })`, context);

  assert.equal(calls[0].url, '/viewer/llm-ops/health');
  assert.equal(calls[1].url, '/viewer/llm-ops/status');
  assert.equal(calls[2].url, '/viewer/llm-ops/stop');
  assert.deepEqual(JSON.parse(calls[2].opts.body), {roles: ['Worker', 'Heavy']});
  assert.equal(calls[3].url, '/viewer/llm-ops/start');
  assert.deepEqual(JSON.parse(calls[3].opts.body), {selection: 'Wild'});
});
