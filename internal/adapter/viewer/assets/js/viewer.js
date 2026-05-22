'use strict';
const BT = String.fromCharCode(96);
const A = {
  user:   {c:'#94a3b8', l:'れん',  en:'Ren',   e:'\u{1f464}'},
  mio:    {c:'#f472b6', l:'みお',  en:'Mio',   e:'\u{1f338}'},
  shiro:  {c:'#22d3ee', l:'しろ',  en:'Shiro', e:'\u26a1'},
  coder1: {c:'#fb923c', l:'あか',  en:'Aka',   e:'\u{1f534}'},
  coder2: {c:'#818cf8', l:'あお',  en:'Ao',    e:'\u{1f535}'},
  coder3: {c:'#a78bfa', l:'ぎん',  en:'Gin',   e:'\u{1f7e3}'},
  coder4: {c:'#facc15', l:'きん',  en:'Kin',   e:'\u{1f7e1}'},
  system: {c:'#475569', l:'System', en:'System', e:'\u2699\ufe0f'},
};
const RC = {
  CHAT:'#f472b6', OPS:'#22d3ee', CODE:'#fb923c',
  CODE1:'#fb923c', CODE2:'#818cf8', CODE3:'#a78bfa', CODE4:'#facc15',
  PLAN:'#4ade80', ANALYZE:'#fbbf24', RESEARCH:'#34d399',
  IDLECHAT:'#a78bfa',
};
const AGENTS = ['mio', 'shiro', 'coder1', 'coder2', 'coder3', 'coder4'];
const ROLE_TARGETS = [
  {id:'mio', role:'Chat', alias:'Chat', use:'会話テンポ / ルミナ人格 / 音声UI'},
  {id:'shiro', role:'Worker', alias:'Worker', use:'実務処理 / 要約 / RAG / 画像解析'},
  {id:'coder1', role:'Wild', alias:'Wild', use:'物語生成 / 画像プロンプト / 雰囲気抽出'},
  {id:'coder2', role:'Coder', alias:'Worker', use:'実装 / 検証 / 差分整理'},
  {id:'coder3', role:'Coder', alias:'Worker', use:'実装 / 調査 / テスト補助'},
  {id:'coder4', role:'Coder', alias:'Worker', use:'実装 / レビュー / 仕上げ'},
];
const OFFLINE_MS = 120000;
const MAX_LOGS = 500;
const MAX_TIMELINE_NODES = 400;
const MAX_SEEN_EVENTS = 4000;
const PROGRESS_RECENT_EVENTS = 8;
const PROGRESS_DONE_LIMIT = 10;
const seenEventKeys = new Set();
const seenEventQueue = [];

function ag(n) { return A[(n || '').toLowerCase()] || A.system; }
function agName(n) {
  const info = ag(n);
  return info.l || info.en || String(n || '-');
}
function ftime(ts) {
  try { return new Date(ts).toLocaleTimeString('ja-JP', {hour:'2-digit', minute:'2-digit', second:'2-digit', timeZone:'Asia/Tokyo'}); }
  catch (_) { return ''; }
}
function fdt(ts) {
  try { return new Date(ts).toLocaleString('ja-JP', {hour12:false, timeZone:'Asia/Tokyo'}); }
  catch (_) { return ''; }
}
function esc(s) {
  const d = document.createElement('div');
  d.textContent = String(s || '');
  return d.innerHTML;
}
function short(s, n) {
  const v = String(s || '');
  return v.length > n ? v.slice(0, n) + '...' : v;
}
function normState(s) {
  if (s === 'idle' || s === 'thinking' || s === 'running' || s === 'error' || s === 'offline' || s === 'unavailable') return s;
  return 'idle';
}
function fmt(s) {
  let h = esc(s);
  const cbRe = new RegExp(BT+'{3}(\\w*)\\n([\\s\\S]*?)'+BT+'{3}', 'g');
  h = h.replace(cbRe, '<pre><code>$2</code></pre>');
  const icRe = new RegExp(BT+'([^'+BT+' ]+)'+BT, 'g');
  h = h.replace(icRe, '<code>$1</code>');
  h = h.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  h = h.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  h = h.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  return h;
}

function eventKey(ev) {
  return [
    ev.type || '',
    ev.from || '',
    ev.to || '',
    ev.route || '',
    ev.job_id || '',
    ev.session_id || '',
    ev.channel || '',
    ev.chat_id || '',
    ev.timestamp || '',
    ev.content || '',
  ].join('|');
}

function rememberEventKey(key) {
  seenEventKeys.add(key);
  seenEventQueue.push(key);
  while (seenEventQueue.length > MAX_SEEN_EVENTS) {
    const old = seenEventQueue.shift();
    if (!old) break;
    seenEventKeys.delete(old);
  }
}

const state = {
  logs: [],
  homeSendError: '',
  viewerAttachmentError: '',
  viewerStatusFetchError: '',
  sessions: {},
  jobs: {},
  evidence: [],
  evidenceSummary: {status: {}, error_kind: {}},
  evidenceFetchError: '',
  evidenceSummaryFetchError: '',
  verificationReports: [],
  verificationSummary: {status: {}, trigger_level: {}},
  verificationFetchError: '',
  verificationSummaryFetchError: '',
  evidenceOrder: [],
  selectedEvidenceJobID: '',
  selectedEvidenceItem: null,
  selectedEvidenceFocus: '',
  evidenceSortDesc: true,
  pendingEvidenceJobID: '',
  memory: {
    snapshot: {memory: [], news: [], digests: [], knowledge: []},
    memorySnapshotFetchError: '',
    layers: {l0: [], l1: [], l2: [], l3: []},
    events: [],
    searchCache: [],
    memoryEventsFetchError: '',
    sourceRegistry: [],
    sourceRegistryStaging: [],
    sourceRegistryFetchError: '',
    sourceRegistryStagingFetchError: '',
    knowledgeMemory: {
      personal_archive: [],
      creative_knowledge: [],
      news_knowledge: [],
      daily_intake_rules: [],
      temporal_markers: [],
      dream_runs: [],
    },
    knowledgeMemoryFetchError: '',
    knowledgeMemoryDetail: null,
    knowledgeMemoryReviewResult: null,
    traces: [],
    recallTraceFetchError: '',
    newsPackFetchError: '',
    selectedNewsIndex: 0,
  },
  agents: {},
  idleChat: {
    selectedMode: localStorage.getItem('idlechat.selectedMode') || 'manual',
    selectedView: localStorage.getItem('idlechat.selectedView') || 'live',
    mode: '',
    manualMode: false,
    chatActive: false,
    currentTopic: '',
    history: [],
    openIndex: -1,
    selectedSummaryIndex: 0,
  },
  openTasks: {},
  progressOpenJobs: {},
  ops: {
    persistedLogs: [],
    opsLogsFetchError: '',
    toolHarnessEvents: [],
    toolHarnessFetchError: '',
    dciTraces: [],
    dciFetchError: '',
    dciLastResult: null,
    sandboxes: [],
    sandboxArtifacts: [],
    sandboxPromotions: [],
    sandboxDecisions: [],
    sandboxGateLogs: [],
    sandboxFetchError: '',
    sandboxPromotionPreviewResult: null,
    skillManifests: [],
    skillTriggerLogs: [],
    skillChangeLogs: [],
    contributionGateLogs: [],
    skillExternalPRSubmitRecords: [],
    skillExternalPRAdapter: '',
    skillExternalPRAdapterConfigured: false,
    skillExternalPRHumanApprovalRequired: true,
    skillGovernanceFetchError: '',
    workstreams: [],
    workstreamGoals: [],
    workstreamArtifacts: [],
    workstreamAnnotations: [],
    workstreamSteering: [],
    workstreamHeartbeats: [],
    workstreamVaultUpdates: [],
    workstreamFetchError: '',
    workstreamVaultPreviewResult: null,
    workstreamVaultReviewResult: null,
    revenueMarketResearch: [],
    revenueSNSPostMetrics: [],
    revenueProducts: [],
    revenueCustomerVoices: [],
    revenueEvents: [],
    revenueHumanDecisions: [],
    revenueDailyRoutineReports: [],
    revenueChannelDrafts: [],
    revenueExternalSendApplyRecords: [],
    revenueExternalChannelAdapter: '',
    revenueExternalChannelAdapterConfigured: false,
    revenueExternalSendHumanApprovalRequired: true,
    revenueSummary: null,
    revenueFetchError: '',
    revenueDecisionReviewResult: null,
    personaDiscomfortLogs: [],
    personaTriggerLogs: [],
    personaCanonicalResponseLogs: [],
    personaObservationLogs: [],
    personaMetaProfileUpdates: [],
    personaMetaReviewResult: null,
    personaInterfaceSessions: [],
    personaObservationFetchError: '',
    browserTraceRuns: [],
    browserTraceAPICandidates: [],
    browserTraceAPISchemas: [],
    browserTraceAPICoverageReports: [],
    browserTraceAPIArtifacts: [],
    browserTraceAPIFetchError: '',
    browserTraceAPIFetcherProposalResult: null,
    complexityScans: [],
    complexityHotspots: [],
    complexityEvidence: [],
    complexityReports: [],
    complexityFetchError: '',
    aiWorkflowEvents: [],
    aiWorkflowProjectMemoryIndexes: [],
    aiWorkflowWorktreeRegistries: [],
    aiWorkflowCommandRegistries: [],
    aiWorkflowContextUsages: [],
    aiWorkflowContextBudgetPolicy: null,
    aiWorkflowFetchError: '',
    superAgentRuns: [],
    superAgentSubagentTasks: [],
    superAgentContextPacks: [],
    superAgentMessageChannels: [],
    superAgentTraceEvents: [],
    superAgentRunQueue: [],
    superAgentRuntimeConfig: null,
    superAgentFetchError: '',
    heavyWorkerRuntimeDiagnostics: null,
    heavyWorkerRuntimeDiagnosticsFetchError: '',
    knowledgePersonalArchive: [],
    knowledgeCreativeItems: [],
    knowledgeNewsItems: [],
    knowledgeDailyIntakeRules: [],
    knowledgeTemporalMarkers: [],
    knowledgeDreamRuns: [],
    knowledgeMemoryFetchError: '',
    knowledgeMemoryDetail: null,
    runtimeBlockedRoutes: [],
    lastMioReport: null,
    latestJobID: '',
    latestRoute: '',
    latestError: null,
    llmOpsEnabled: false,
    localLLM: null,
    runtimeReadiness: null,
    runtimeConfigFetchError: '',
    runtimeSTTBaseURL: '',
    runtimeSTTStreamURL: '',
    runtimeTTSBaseURL: '',
    runtimeDebugSystemFetchError: '',
    llmStatus: null,
    llmStatusError: '',
    runtimeHealth: null,
    runtimeHealthError: '',
  },
  debug: {
    gpu: null,
    audio: null,
    sttTrace: [],
    thinkTrace: [],
  },
};
AGENTS.forEach((id) => {
  state.agents[id] = {
    id,
    state: 'offline',
    reason: '',
    route: '-',
    lastEvent: '-',
    peer: '-',
    jobID: '-',
    sessionID: 'viewer',
    preview: '-',
    updatedAt: '',
  };
  state.openTasks[id] = {};
});

let msgCount = 0;
const mainEl = document.querySelector('main');
const chat = document.getElementById('chat');
const idleLiveLog = document.getElementById('idleLiveLog');
const ctr = document.getElementById('ctr');
const cnt = document.getElementById('cnt');
const latestBtn = document.getElementById('latestBtn');
const toastEl = document.getElementById('toast');
const thinkingBubbles = {};
const ttsPlayback = {
  queue: [],
  audio: null,
  playing: false,
  audioEnabled: true,
  unlocked: false,
  blocked: false,
  audioError: '',
  currentCharacterId: '',
  currentText: '',
  currentDisplayText: '',
  currentSessionId: '',
  currentChunkIndex: -1,
  currentUtteranceId: '',
  currentResponseId: '',
  currentShown: false,
  fallbackActive: false,
  fallbackTimer: null,
  seq: 0,
};
const lipSyncMioEl = document.getElementById('lipSyncMio');
const lipSyncShiroEl = document.getElementById('lipSyncShiro');
const lipSyncActors = {
  mio: lipSyncMioEl,
  shiro: lipSyncShiroEl,
};
const ttsNowPlayingEl = document.getElementById('ttsNowPlaying');
const ttsNowPlayingTextEl = document.getElementById('ttsNowPlayingText');
const centralTTSSpeech = {
  el: null,
  textEl: null,
  characterId: '',
  sessionId: '',
  responseId: '',
  bubbleKind: '',
  active: false,
  preRendered: false,
  chunkKeys: new Set(),
};
const idleTTSSpeech = {
  el: null,
  textEl: null,
  characterId: '',
  sessionId: '',
  responseId: '',
  bubbleKind: '',
  active: false,
  preRendered: false,
  chunkKeys: new Set(),
};
const idlePendingMessages = new Map();
let idleLiveTopicKey = '';
const idleLiveRenderedLog = [];
if (typeof window !== 'undefined') window.__idleLiveRenderedLog = idleLiveRenderedLog;
const IDLE_MESSAGE_FALLBACK_MS = 10000;

function setLipSyncSpeaking(characterId, speaking) {
  const id = String(characterId || '').trim().toLowerCase();
  const el = lipSyncActors[id];
  if (!el) return;
  const openSrc = String(el.dataset.open || '').trim();
  const closedSrc = String(el.dataset.closed || '').trim();
  el.src = speaking ? openSrc : closedSrc;
}

function clearLipSyncSpeaking() {
  setLipSyncSpeaking('mio', false);
  setLipSyncSpeaking('shiro', false);
}

function setNowPlayingText(characterId, text) {
  if (!ttsNowPlayingEl || !ttsNowPlayingTextEl) return;
  const normalizedText = String(text || '').trim();
  ttsNowPlayingEl.classList.remove('mio', 'shiro');
  if (!normalizedText) {
    ttsNowPlayingEl.classList.add('hidden');
    ttsNowPlayingTextEl.textContent = '';
    return;
  }
  const id = String(characterId || '').trim().toLowerCase();
  if (id === 'mio' || id === 'shiro') ttsNowPlayingEl.classList.add(id);
  ttsNowPlayingTextEl.textContent = normalizedText;
  ttsNowPlayingEl.classList.remove('hidden');
}

function describeTTSAudioError(err) {
  if (!err) return 'unknown audio playback error';
  const name = String(err.name || '').trim();
  const message = String(err.message || err).trim();
  return [name, message].filter(Boolean).join(': ') || 'unknown audio playback error';
}

function setTTSAudioError(err) {
  const message = describeTTSAudioError(err);
  ttsPlayback.audioError = message;
  setNowPlayingText('', 'TTS audio unavailable: ' + message);
}

function clearTTSAudioError() {
  ttsPlayback.audioError = '';
}

function isIdleChatSessionId(sessionId) {
  const sid = String(sessionId || '').trim();
  return sid.indexOf('idle-') === 0 || sid.indexOf('forecast-') === 0 || sid.indexOf('story-') === 0 || sid.indexOf('story-simple-') === 0;
}

function setCentralTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId, responseId) {
  const target = isIdleChatSessionId(sessionId) ? 'idle' : 'central';
  setTTSSpeechText(target, characterId, text, sessionId, chunkIndex, utteranceId, responseId);
}

function setTTSSpeechText(target, characterId, text, sessionId, chunkIndex, utteranceId, responseId) {
  if (target === 'idle') {
    renderIdleTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId, responseId);
    return;
  }
  renderChatTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId, responseId);
}

function renderChatTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId, responseId) {
  const normalizedText = String(text || '').trim();
  if (!normalizedText) {
    resetTTSSpeechBubble(centralTTSSpeech);
    return;
  }
  if (!chat) return;

  const id = String(characterId || '').trim().toLowerCase();
  const sid = String(sessionId || '').trim();
  const normalizedChunkIndex = Number.isFinite(chunkIndex) ? chunkIndex : -1;
  const rid = String(responseId || '').trim();
  const speech = centralTTSSpeech;
  const bubbleKind = ttsBubbleKind(speech, normalizedText, sid, normalizedChunkIndex, id);
  const f = ag(id || 'mio');
  const key = String(utteranceId || '') || (sid + ':' + String(normalizedChunkIndex >= 0 ? normalizedChunkIndex : speech.chunkKeys.size));
  if (!speech.el || speech.characterId !== id || speech.bubbleKind !== bubbleKind || shouldStartNewTTSBubble(speech, normalizedChunkIndex, key, rid)) {
    if (speech.el) speech.el.classList.remove('tts-current');
    const el = document.createElement('div');
    el.className = 'msg tts-current' + (id === 'shiro' ? ' shiro' : '');
    el.innerHTML =
      '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
      '<div class="mb"><div class="mh">' +
        '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
        '<span class="tm">' + ftime(new Date().toISOString()) + '</span>' +
      '</div><div class="mc"></div></div>';
    speech.el = el;
    speech.textEl = el.querySelector('.mc');
    speech.characterId = id;
    speech.sessionId = sid;
    speech.responseId = rid;
    speech.bubbleKind = bubbleKind;
    speech.active = true;
    speech.preRendered = false;
    speech.chunkKeys = new Set();
    const em = document.getElementById('empty');
    if (em) em.remove();
    chat.appendChild(el);
    trimTimelineNodesFor(chat, MAX_TIMELINE_NODES);
  } else {
    speech.el.classList.add('tts-current');
    speech.el.classList.toggle('shiro', id === 'shiro');
    speech.sessionId = sid;
    if (rid) speech.responseId = rid;
    speech.active = true;
  }
  if (speech.chunkKeys.has(key)) {
    return;
  }
  speech.chunkKeys.add(key);
  if (speech.textEl) {
    const current = String(speech.textEl.textContent || '');
    speech.textEl.textContent = speech.preRendered ? current : appendCentralTTSText(current, normalizedText);
    speech.textEl.dataset.raw = speech.textEl.textContent;
  }
  scrollToBottom();
}

function renderIdleTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId, responseId) {
  const normalizedText = String(text || '').trim();
  if (!normalizedText) {
    resetTTSSpeechBubble(idleTTSSpeech);
    return;
  }
  if (!idleLiveLog) return;

  const id = String(characterId || '').trim().toLowerCase();
  const sid = String(sessionId || '').trim();
  const normalizedChunkIndex = Number.isFinite(chunkIndex) ? chunkIndex : -1;
  const rid = String(responseId || '').trim();
  const speech = idleTTSSpeech;
  const bubbleKind = ttsBubbleKind(speech, normalizedText, sid, normalizedChunkIndex, id);
  const f = ag(id || 'mio');
  const key = String(utteranceId || '') || (sid + ':' + String(normalizedChunkIndex >= 0 ? normalizedChunkIndex : speech.chunkKeys.size));
  if (!speech.el || speech.characterId !== id || speech.bubbleKind !== bubbleKind || shouldStartNewTTSBubble(speech, normalizedChunkIndex, key, rid)) {
    if (speech.el) speech.el.classList.remove('tts-current');
    const rendered = consumeIdlePendingMessage(sid, id);
    const el = rendered && rendered.el ? rendered.el : document.createElement('div');
    if (rendered && rendered.el) {
      el.classList.add('tts-current');
      el.classList.add('idle-kind-tts');
      el.classList.add('idle-kind-' + bubbleKind);
      el.classList.toggle('shiro', id === 'shiro');
    } else {
      el.className = 'msg tts-current' + (id === 'shiro' ? ' shiro' : '') + ' idle-live-item idle-kind-tts idle-kind-' + bubbleKind;
      el.innerHTML =
        '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
        '<div class="mb"><div class="mh">' +
          '<span class="idle-kind">' + (bubbleKind === 'topic' ? 'Topic' : 'Speech') + '</span>' +
          '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
          '<span class="tm">' + ftime(new Date().toISOString()) + '</span>' +
        '</div><div class="mc"></div></div>';
    }
    speech.el = el;
    speech.textEl = el.querySelector('.mc');
    speech.characterId = id;
    speech.sessionId = sid;
    speech.responseId = rid;
    speech.bubbleKind = bubbleKind;
    speech.active = true;
    speech.preRendered = !!(rendered && rendered.el);
    speech.chunkKeys = new Set();
    removeIdleLiveEmpty();
    if (!(rendered && rendered.el)) idleLiveLog.appendChild(el);
    trimTimelineNodesFor(idleLiveLog, MAX_TIMELINE_NODES);
  } else {
    speech.el.classList.add('tts-current');
    speech.el.classList.toggle('shiro', id === 'shiro');
    speech.sessionId = sid;
    if (rid) speech.responseId = rid;
    speech.active = true;
  }
  if (speech.chunkKeys.has(key)) {
    return;
  }
  speech.chunkKeys.add(key);
  if (speech.textEl) {
    const current = String(speech.textEl.textContent || '');
    speech.textEl.textContent = speech.preRendered ? current : appendCentralTTSText(current, normalizedText);
    speech.textEl.dataset.raw = speech.textEl.textContent;
  }
  idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
}

function resetCentralTTSSpeechBubble() {
  resetTTSSpeechBubble(centralTTSSpeech);
  resetTTSSpeechBubble(idleTTSSpeech);
}

function resetTTSSpeechBubble(speech) {
  if (speech.el) speech.el.classList.remove('tts-current');
  speech.active = false;
  speech.preRendered = false;
}

function shouldStartNewTTSBubble(speech, chunkIndex, key, responseId) {
  if (!speech.el) return true;
  if (!speech.textEl || !String(speech.textEl.textContent || '').trim()) return false;
  if (speech.chunkKeys.has(key)) return false;
  if (responseId && speech.responseId && responseId !== speech.responseId) return true;
  if (chunkIndex === 0) return true;
  if (!speech.active && chunkIndex < 1) return true;
  return false;
}

function appendCentralTTSText(current, next) {
  const left = String(current || '');
  const right = String(next || '').trim();
  if (!left) return right;
  if (!right) return left;
  if (/\s$/.test(left) || /^\s/.test(next)) return left + right;
  if (/[、]$/.test(left)) return left + right;
  if (/[「『（(［\[]$/.test(left) || /^[、。！？!?）」』）)\]］]/.test(right)) return left + right;
  return left + ' ' + right;
}

function ttsBubbleKind(speech, text, sessionId, chunkIndex, characterId) {
  const s = String(text || '').trim();
  if (/^今日のお題です[、。！？!?]?/.test(s)) return 'topic';
  if (chunkIndex > 0 && speech.bubbleKind === 'topic' && speech.sessionId === String(sessionId || '').trim() && speech.characterId === String(characterId || '').trim().toLowerCase()) {
    return 'topic';
  }
  return 'speech';
}

let timelineAutoFollow = true;
let timelineUserInteracting = false;
let timelineInteractionTimer = null;
let suppressTimelineScroll = false;
let derivedDirty = false;
let activeViewerTab = 'home';
let sttControlsReady = false;

const tabs = Array.from(document.querySelectorAll('.tab-btn'));
const themeButtons = Array.from(document.querySelectorAll('.theme-btn'));
const mobilePanelSelect = document.getElementById('mobilePanelSelect');
const mobilePanelPrev = document.getElementById('mobilePanelPrev');
const mobilePanelNext = document.getElementById('mobilePanelNext');
const panels = {
  home: document.getElementById('panel-home'),
  develop: document.getElementById('panel-develop'),
  instructions: document.getElementById('panel-instructions'),
  reports: document.getElementById('panel-reports'),
  ops: document.getElementById('panel-ops'),
  overview: document.getElementById('panel-overview'),
  roles: document.getElementById('panel-roles'),
  progress: document.getElementById('panel-progress'),
  timeline: document.getElementById('panel-timeline'),
  system: document.getElementById('panel-system'),
  memory: document.getElementById('panel-memory'),
  'news-pack': document.getElementById('panel-news-pack'),
  idlechat: document.getElementById('panel-idlechat'),
  sessions: document.getElementById('panel-sessions'),
  jobs: document.getElementById('panel-jobs'),
};

function applyViewerTheme(theme) {
  const selected = (theme === 'classic' || theme === 'compact') ? theme : 'modern';
  const body = document.body;
  if (body && body.classList) {
    body.classList.remove('theme-classic', 'theme-modern', 'theme-compact');
    body.classList.add('theme-' + selected);
  }
  themeButtons.forEach((btn) => btn.classList.toggle('active', btn.dataset.theme === selected));
  try { localStorage.setItem('viewer.theme', selected); } catch (_) {}
}

function savedViewerTheme() {
  try { return localStorage.getItem('viewer.theme') || 'modern'; }
  catch (_) { return 'modern'; }
}

applyViewerTheme(savedViewerTheme());
themeButtons.forEach((btn) => btn.addEventListener('click', () => applyViewerTheme(btn.dataset.theme)));

const fltType = document.getElementById('fltType');
const fltAgent = document.getElementById('fltAgent');
const fltRoute = document.getElementById('fltRoute');
const fltJob = document.getElementById('fltJob');
const fltText = document.getElementById('fltText');
const sysPreset = document.getElementById('sysPreset');
const sysType = document.getElementById('sysType');
const sysText = document.getElementById('sysText');
const memorySession = document.getElementById('memorySession');
const memoryNamespace = document.getElementById('memoryNamespace');
const memoryCategory = document.getElementById('memoryCategory');
const memoryDomain = document.getElementById('memoryDomain');
const memoryEventNamespace = document.getElementById('memoryEventNamespace');
const memoryPromoteKind = document.getElementById('memoryPromoteKind');
const memoryPromoteID = document.getElementById('memoryPromoteID');
const memoryRefreshBtn = document.getElementById('memoryRefreshBtn');
const roleFilter = document.getElementById('roleFilter');
const sourceRegistrySaveBtn = document.getElementById('sourceRegistrySaveBtn');
const sourceRegistryExportBtn = document.getElementById('sourceRegistryExportBtn');
const sourceRegistryImportBtn = document.getElementById('sourceRegistryImportBtn');
const sourceRegistryStagingRefreshBtn = document.getElementById('sourceRegistryStagingRefreshBtn');
const sourceRegistryYAML = document.getElementById('sourceRegistryYAML');
const newsPackCategory = document.getElementById('newsPackCategory');
const newsPackRefreshBtn = document.getElementById('newsPackRefreshBtn');
const idleStartBtn = document.getElementById('idleStart');
const idleModeNormalBtn = document.getElementById('idleModeNormal');
const idleModeForecastBtn = document.getElementById('idleModeForecast');
const idleModeStorySimpleBtn = document.getElementById('idleModeStorySimple');
const idleStopBtn = document.getElementById('idleStop');
const idleStateEl = document.getElementById('idleState');
const idleSubtabs = Array.from(document.querySelectorAll('.idle-subtab'));
const idleSubviews = Array.from(document.querySelectorAll('.idle-subview'));
const audioBtn = document.getElementById('audioBtn');
const liveAudioBtn = document.getElementById('liveAudioBtn');
const eviStatus = document.getElementById('eviStatus');
const eviErrorKind = document.getElementById('eviErrorKind');
const eviPrev = document.getElementById('eviPrev');
const eviNext = document.getElementById('eviNext');
const eviPos = document.getElementById('eviPos');
const eviCopy = document.getElementById('eviCopy');
const eviCopySummary = document.getElementById('eviCopySummary');
const eviSort = document.getElementById('eviSort');

function switchTab(tab) {
  if (!panels[tab]) return;
  activeViewerTab = tab;
  document.body.dataset.viewerTab = tab;
  tabs.forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
  Object.keys(panels).forEach((k) => panels[k].classList.toggle('active', k === tab));
  if (mobilePanelSelect && mobilePanelSelect.value !== tab) mobilePanelSelect.value = tab;
  const activeTab = tabs.find((b) => b.dataset.tab === tab);
  if (activeTab && typeof activeTab.scrollIntoView === 'function') {
    activeTab.scrollIntoView({block: 'nearest', inline: 'center'});
  }
  updateLatestButton();
  if (sttControlsReady) {
    updateSTTInputIndicators();
  }
  if (tab === 'timeline' && timelineAutoFollow) scrollToBottom(true);
  renderDeskViews();
}
tabs.forEach((btn) => btn.addEventListener('click', () => switchTab(btn.dataset.tab)));
document.body.dataset.viewerTab = activeViewerTab;

function switchAdjacentPanel(delta) {
  const names = tabs.map((btn) => btn.dataset.tab).filter((name) => panels[name]);
  if (!names.length) return;
  const current = names.includes(activeViewerTab) ? activeViewerTab : names[0];
  const nextIndex = (names.indexOf(current) + delta + names.length) % names.length;
  switchTab(names[nextIndex]);
}

if (mobilePanelSelect) {
  mobilePanelSelect.addEventListener('change', () => switchTab(mobilePanelSelect.value));
  mobilePanelSelect.value = activeViewerTab;
}
if (mobilePanelPrev) mobilePanelPrev.addEventListener('click', () => switchAdjacentPanel(-1));
if (mobilePanelNext) mobilePanelNext.addEventListener('click', () => switchAdjacentPanel(1));

function matchesFilters(ev) {
  if (isSystemEvent(ev)) return false;
  if (fltType.value && ev.type !== fltType.value) return false;
  if (fltAgent.value && ev.from !== fltAgent.value && ev.to !== fltAgent.value) return false;
  if (fltRoute.value && (ev.route || '') !== fltRoute.value) return false;
  if (fltJob.value && !(ev.job_id || '').toLowerCase().includes(fltJob.value.toLowerCase())) return false;
  if (fltText.value && !(ev.content || '').toLowerCase().includes(fltText.value.toLowerCase())) return false;
  return true;
}
function isSystemEvent(ev) {
  if (!ev) return false;
  if (ev.type === 'routing.decision' || ev.type === 'entry.stage' || ev.type === 'tts.audio_chunk') return true;
  if (ev.type === 'agent.dispatch' || ev.type === 'mailbox.sent' || ev.type === 'mailbox.waiting' || ev.type === 'mailbox.received' || ev.type === 'mailbox.error' || ev.type === 'agent.error') return true;
  if (ev.type === 'agent.start' || ev.type === 'agent.note' || ev.type === 'agent.response') {
    const to = String(ev.to || '').toLowerCase();
    if (to !== 'user') return true;
  }
  if ((ev.from || '').toLowerCase() === 'system' || (ev.to || '').toLowerCase() === 'system') return true;
  if ((ev.from || '').toLowerCase() === 'tts') return true;
  return false;
}
function resetTimeline() {
  chat.innerHTML = '';
  Object.keys(thinkingBubbles).forEach((k) => delete thinkingBubbles[k]);
  msgCount = 0;
  state.logs.forEach((ev) => addMsgToTimeline(ev));
}
[fltType, fltAgent, fltRoute].forEach((el) => el.addEventListener('change', resetTimeline));
[fltJob, fltText].forEach((el) => el.addEventListener('input', resetTimeline));

function matchesSystemFilters(ev) {
  if (!isSystemEvent(ev)) return false;
  const preset = sysPreset ? sysPreset.value : 'all';
  if (preset === 'no-tts-audio' && ev.type === 'tts.audio_chunk') return false;
  if (preset === 'tts-only' && (ev.from || '').toLowerCase() !== 'tts' && ev.type !== 'tts.audio_chunk') return false;
  if (preset === 'tts-audio-only' && ev.type !== 'tts.audio_chunk') return false;
  if (sysType && sysType.value && ev.type !== sysType.value) return false;
  if (sysText && sysText.value && !(ev.content || '').toLowerCase().includes(sysText.value.toLowerCase())) return false;
  return true;
}


if (sysPreset) sysPreset.addEventListener('change', renderSystem);
if (sysType) sysType.addEventListener('change', renderSystem);
if (sysText) sysText.addEventListener('input', renderSystem);
if (roleFilter) roleFilter.addEventListener('change', renderRoleSelector);
if (memoryRefreshBtn) memoryRefreshBtn.addEventListener('click', refreshMemorySnapshot);
if (memorySession) memorySession.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshMemorySnapshot(); });
if (memoryNamespace) memoryNamespace.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshMemorySnapshot(); });
if (memoryCategory) memoryCategory.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshMemorySnapshot(); });
if (memoryDomain) memoryDomain.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshMemorySnapshot(); });
if (memoryEventNamespace) memoryEventNamespace.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshMemoryEvents(); });
if (sourceRegistrySaveBtn) sourceRegistrySaveBtn.addEventListener('click', saveSourceRegistryEntry);
if (sourceRegistryExportBtn) sourceRegistryExportBtn.addEventListener('click', exportSourceRegistryYAML);
if (sourceRegistryImportBtn) sourceRegistryImportBtn.addEventListener('click', importSourceRegistryYAML);
if (sourceRegistryStagingRefreshBtn) sourceRegistryStagingRefreshBtn.addEventListener('click', refreshSourceRegistryStaging);
if (newsPackRefreshBtn) newsPackRefreshBtn.addEventListener('click', refreshNewsPack);
if (newsPackCategory) newsPackCategory.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshNewsPack(); });


idleStartBtn.addEventListener('click', () => {
  const path = state.idleChat.selectedMode === 'forecast'
    ? '/viewer/idlechat/forecast'
    : (state.idleChat.selectedMode === 'story-simple'
      ? '/viewer/idlechat/story-simple'
      : '/viewer/idlechat/start');
  controlIdle(path);
});
if (idleModeNormalBtn) idleModeNormalBtn.addEventListener('click', () => setIdleSelectedMode('manual'));
if (idleModeForecastBtn) idleModeForecastBtn.addEventListener('click', () => setIdleSelectedMode('forecast'));
if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.addEventListener('click', () => setIdleSelectedMode('story-simple'));
idleStopBtn.addEventListener('click', () => controlIdle('/viewer/idlechat/stop'));
idleSubtabs.forEach((btn) => {
  btn.addEventListener('click', () => setIdleSelectedView(btn.dataset.idleView || 'live'));
});

function stateClass(s) { return 'state-' + normState(s); }

function num(v) {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

function pct(part, total) {
  const p = num(part);
  const t = num(total);
  if (t <= 0) return 0;
  return Math.max(0, Math.min(100, (p / t) * 100));
}

function fmtGiB(value) {
  const n = num(value);
  if (n <= 0) return '-';
  return n.toFixed(n >= 10 ? 1 : 2) + ' GiB';
}

function fmtGiBFromMiB(value) {
  const n = num(value);
  if (n <= 0) return '-';
  return fmtGiB(n / 1024);
}

function fmtBytesAsGiB(bytes) {
  const n = num(bytes);
  if (n <= 0) return '-';
  return fmtGiB(n / 1073741824);
}

function bump() {
  msgCount++;
  cnt.textContent = msgCount;
  ctr.style.display = 'flex';
  scrollToBottom();
}

function pushDebugTrace(kind, payload) {
  const bucket = kind === 'stt' ? state.debug.sttTrace : state.debug.thinkTrace;
  bucket.push(payload);
  if (bucket.length > 40) bucket.shift();
}

function renderDebugPanels() {
  const gpuEl = document.getElementById('debugGpuSummary');
  const sttEl = document.getElementById('debugSttTrace');
  const thinkEl = document.getElementById('debugThinkTrace');
  if (!gpuEl || !sttEl || !thinkEl) return;

  const g = state.debug.gpu;
  const a = state.debug.audio;
  if (!g || !g.available) {
    const note = g && g.note ? g.note : 'GPU情報を取得できません';
    gpuEl.innerHTML = '<span class="badge state-offline">unavailable</span> ' + esc(note);
  } else {
    gpuEl.innerHTML =
      '<div class="row"><span>total/used/free</span><span>' + esc(String(g.total_mb || 0)) + ' / ' + esc(String(g.used_mb || 0)) + ' / ' + esc(String(g.free_mb || 0)) + ' MB</span></div>' +
      '<div class="row"><span>LLM</span><span class="badge state-running">' + esc(String(g.llm_used_mb || 0)) + ' MB</span></div>' +
      '<div class="row"><span>STT</span><span class="badge state-thinking">' + esc(String(g.stt_used_mb || 0)) + ' MB</span></div>' +
      '<div class="row"><span>TTS</span><span class="badge state-idle">' + esc(String(g.tts_used_mb || 0)) + ' MB</span></div>' +
      '<div class="row"><span>other</span><span class="badge state-offline">' + esc(String(g.other_used_mb || 0)) + ' MB</span></div>';
  }
  if (a) {
    gpuEl.innerHTML +=
      '<div class="row"><span>STT health</span><span class="badge ' + (a.stt_ok ? 'state-idle' : 'state-error') + '">' + (a.stt_ok ? 'ok' : 'ng') + '</span></div>' +
      '<div class="ops-sub">stt=' + esc(a.stt_base_url || '-') + '\n' + esc(a.stt_health || '-') + '</div>' +
      '<div class="row"><span>TTS live/ready</span><span class="badge ' + ((a.tts_live_ok && a.tts_ready_ok) ? 'state-idle' : 'state-error') + '">' + ((a.tts_live_ok ? 'live:ok' : 'live:ng') + ' / ' + (a.tts_ready_ok ? 'ready:ok' : 'ready:ng')) + '</span></div>' +
      '<div class="ops-sub">tts=' + esc(a.tts_base_url || '-') + '\n/live ' + esc(a.tts_live || '-') + '\n/ready ' + esc(a.tts_ready || '-') + '</div>';
    if (a.last_error) {
      gpuEl.innerHTML += '<div class="ops-sub">error: ' + esc(a.last_error) + '</div>';
    }
  }

  const sttList = state.debug.sttTrace.slice().reverse();
  if (sttList.length === 0) {
    sttEl.innerHTML = '<div class="debug-empty">まだSTTイベントがありません</div>';
  } else {
    sttEl.innerHTML = sttList.map((item) => (
      '<div class="debug-item">' +
        '<div class="debug-meta">' + esc(item.time || '-') + ' · ' + esc(item.step || '-') + '</div>' +
        '<div>' + esc(item.text || '-') + '</div>' +
      '</div>'
    )).join('');
  }

  const thinkList = state.debug.thinkTrace.slice().reverse();
  if (thinkList.length === 0) {
    thinkEl.innerHTML = '<div class="debug-empty">まだthinkingイベントがありません</div>';
  } else {
    thinkEl.innerHTML = thinkList.map((item) => (
      '<div class="debug-item">' +
        '<div class="debug-meta">' + esc(item.time || '-') + ' · ' + esc(item.agent || '-') + ' · ' + esc(item.job || '-') + '</div>' +
        '<div>' + esc(item.text || '-') + '</div>' +
      '</div>'
    )).join('');
  }
}

function refreshDebugSystem() {
  fetch('/viewer/debug/system')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'debug system fetch failed'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.debug.gpu = data && data.gpu ? data.gpu : null;
      state.debug.audio = data && data.audio ? data.audio : null;
      renderDebugPanels();
    })
    .catch((err) => {
      console.error(err);
      state.debug.gpu = {available: false, note: String(err && err.message ? err.message : err)};
      state.debug.audio = null;
      renderDebugPanels();
    });
}

function trimTimelineNodes() {
  trimTimelineNodesFor(chat, MAX_TIMELINE_NODES);
}

function trimTimelineNodesFor(container, maxNodes) {
  if (!container) return;
  while (container.childElementCount > maxNodes) {
    const first = container.firstElementChild;
    if (!first) break;
    first.remove();
  }
}


function isTTSSyncedSpeaker(agentID) {
  const id = String(agentID || '').trim().toLowerCase();
  return id === 'mio' || id === 'shiro';
}

function addThinkingStart(ev) {
  if (!matchesFilters(ev)) return;
  const jid = ev.job_id || '_';
  if (thinkingBubbles[jid]) return;
  const f = ag(ev.from);
  const el = document.createElement('div');
  el.className = 'msg thinking';
  const textEl = document.createElement('div');
  textEl.className = 'mc';
  textEl.innerHTML = '<span class="dots"><span></span><span></span><span></span></span>';
  el.innerHTML =
    '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
    '<div class="mb"><div class="mh">' +
      '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
      '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
    '</div></div>';
  el.querySelector('.mb').appendChild(textEl);
  chat.appendChild(el);
  trimTimelineNodes();
  thinkingBubbles[jid] = {el: el, textEl: textEl, raw: '', waiting: true};
  scrollToBottom();
}

function addThinking(ev) {
  if (!matchesFilters(ev)) return;
  const jid = ev.job_id || '_';
  let b = thinkingBubbles[jid];
  if (!b) {
    const f = ag(ev.from);
    const el = document.createElement('div');
    el.className = 'msg thinking';
    const textEl = document.createElement('div');
    textEl.className = 'mc';
    el.innerHTML =
      '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
      '<div class="mb"><div class="mh">' +
        '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
        '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
      '</div></div>';
    el.querySelector('.mb').appendChild(textEl);
    chat.appendChild(el);
    trimTimelineNodes();
    b = {el: el, textEl: textEl, raw: '', waiting: false};
    thinkingBubbles[jid] = b;
  }
  if (b.waiting) {
    b.waiting = false;
    b.textEl.innerHTML = '';
  }
  b.raw += normalizeViewerDisplayText(ev.content || '');
  b.textEl.textContent = b.raw;
  scrollToBottom();
}

function removeThinking(jid) {
  const key = jid || '_';
  const b = thinkingBubbles[key];
  if (!b) return;
  b.el.remove();
  delete thinkingBubbles[key];
}

function addSys(ev) {
  const el = document.createElement('div');
  el.className = 'sys';
  const rc = RC[ev.route] || '#94a3b8';
  const displayContent = normalizeViewerDisplayText(ev.content || '');
  el.innerHTML =
    '<div class="sl"></div>' +
    '<div class="sb">⚙️ <span class="rt" style="color:' + rc + '">' +
      esc(ev.route || '?') + '</span> ' + esc(displayContent) + '</div>' +
    '<div class="sl"></div>';
  chat.appendChild(el);
  trimTimelineNodes();
  bump();
}

function isTimelineActive() {
  return !!(panels.timeline && panels.timeline.classList.contains('active'));
}

function isTimelineNearBottom() {
  return (mainEl.scrollHeight - mainEl.scrollTop - mainEl.clientHeight) <= 120;
}

function updateLatestButton() {
  if (!latestBtn) return;
  latestBtn.classList.toggle('show', isTimelineActive() && !timelineAutoFollow);
}

function noteTimelineInteraction() {
  if (!isTimelineActive()) return;
  timelineUserInteracting = true;
  if (timelineInteractionTimer) clearTimeout(timelineInteractionTimer);
  timelineInteractionTimer = setTimeout(() => {
    timelineUserInteracting = false;
  }, 1200);
}

function setTimelineAutoFollow(enabled) {
  timelineAutoFollow = !!enabled;
  updateLatestButton();
}

mainEl.addEventListener('wheel', noteTimelineInteraction, {passive:true});
mainEl.addEventListener('touchstart', noteTimelineInteraction, {passive:true});
mainEl.addEventListener('touchmove', noteTimelineInteraction, {passive:true});
mainEl.addEventListener('pointerdown', noteTimelineInteraction, {passive:true});
mainEl.addEventListener('scroll', () => {
  if (!isTimelineActive() || suppressTimelineScroll) return;
  const nearBottom = isTimelineNearBottom();
  if (timelineUserInteracting && !nearBottom) {
    setTimelineAutoFollow(false);
    return;
  }
  if (nearBottom) setTimelineAutoFollow(true);
});

function scrollToBottom(force) {
  if (!isTimelineActive()) return;
  if (!force && !timelineAutoFollow) return;
  suppressTimelineScroll = true;
  mainEl.scrollTop = mainEl.scrollHeight;
  requestAnimationFrame(() => {
    suppressTimelineScroll = false;
    if (isTimelineNearBottom()) setTimelineAutoFollow(true);
  });
}

if (latestBtn) {
  latestBtn.addEventListener('click', () => {
    setTimelineAutoFollow(true);
    scrollToBottom(true);
  });
}

function copyMsg(btn) {
  const mc = btn.parentElement.querySelector('.mc');
  const text = mc.dataset.raw || mc.textContent;
  writeClipboardText(text).then(() => {
    btn.textContent = 'OK';
    btn.title = '';
    btn.classList.add('ok');
    setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('ok'); }, 1200);
  }).catch((err) => {
    console.error(err);
    const message = 'Copy unavailable: ' + String(err && err.message ? err.message : err);
    btn.textContent = message;
    btn.title = message;
    showToast('Copy failed', 'error');
  });
}
window.copyMsg = copyMsg;

function escAttr(s) {
  return String(s || '')
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function writeClipboardText(text) {
  const value = String(text || '');
  if (navigator.clipboard && window.isSecureContext) {
    return navigator.clipboard.writeText(value);
  }
  return new Promise((resolve, reject) => {
    const ta = document.createElement('textarea');
    ta.value = value;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.top = '-1000px';
    ta.style.left = '-1000px';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    let ok = false;
    try {
      ok = document.execCommand('copy');
    } catch (err) {
      document.body.removeChild(ta);
      reject(err);
      return;
    }
    document.body.removeChild(ta);
    if (ok) resolve();
    else reject(new Error('copy command failed'));
  });
}

function copyTextPayload(btn, payload) {
  writeClipboardText(String(payload || '')).then(() => {
    const old = btn.textContent;
    btn.textContent = 'Copied';
    btn.title = '';
    btn.classList.add('ok');
    showToast('Copied to clipboard', 'success');
    setTimeout(() => {
      btn.textContent = old;
      btn.classList.remove('ok');
    }, 1200);
  }).catch((err) => {
    console.error(err);
    const message = 'Copy unavailable: ' + String(err && err.message ? err.message : err);
    btn.textContent = message;
    btn.title = message;
    showToast('Copy failed', 'error');
  });
}
window.copyTextPayload = copyTextPayload;

function upsertSession(ev) {
  const sid = ev.session_id || 'viewer';
  let s = state.sessions[sid];
  if (!s) {
    s = {id: sid, channel: '-', chatID: '-', count: 0, lastRoute: '-', lastUserMessage: '-', agents: {}, updatedAt: ''};
    state.sessions[sid] = s;
  }
  s.count++;
  if (ev.channel) s.channel = ev.channel;
  if (ev.chat_id) s.chatID = ev.chat_id;
  if (ev.route) s.lastRoute = ev.route;
  if (ev.from === 'user' && ev.content) s.lastUserMessage = ev.content;
  if (ev.from) s.agents[ev.from] = true;
  if (ev.to) s.agents[ev.to] = true;
  s.updatedAt = ev.timestamp;
}

function upsertJob(ev) {
  const jid = ev.job_id || '-';
  if (jid === '-') return;
  let j = state.jobs[jid];
  if (!j) {
    j = {id: jid, route: '-', status: 'running', from: '-', to: '-', startedAt: ev.timestamp, updatedAt: ev.timestamp, events: 0, preview: ''};
    state.jobs[jid] = j;
  }
  j.events++;
  j.updatedAt = ev.timestamp;
  if (ev.route) j.route = ev.route;
  if (ev.from) j.from = ev.from;
  if (ev.to) j.to = ev.to;
  if (ev.content) j.preview = ev.content;
  if (ev.type === 'agent.response') {
    const c = (ev.content || '').toLowerCase();
    j.status = (c.includes('error') || c.includes('失敗')) ? 'error' : 'done';
  }
}

function touchAgent(agentID, patch) {
  if (!state.agents[agentID]) return;
  Object.assign(state.agents[agentID], patch);
}

function applyMonitorStatusSnapshot(payload) {
  const status = payload && payload.status ? payload.status : payload;
  if (!status) return;
  state.viewerStatusFetchError = '';
  const items = [];
  if (status.chat) items.push(status.chat);
  if (status.worker) items.push(status.worker);
  if (status.coders && Array.isArray(status.coders.items)) {
    status.coders.items.forEach((item) => items.push(item));
  }
  items.forEach((item) => {
    const id = String(item.id || item.agent_id || '').toLowerCase();
    if (!AGENTS.includes(id)) return;
    touchAgent(id, {
      state: item.state || item.status || state.agents[id].state,
      reason: item.reason || '',
      route: item.route || '-',
      lastEvent: item.last_event || '-',
      preview: item.preview || '-',
      updatedAt: item.updated_at || '',
      jobID: item.job_id || '-',
    });
  });
  renderOverview();
  renderRoleSelector();
  renderProgress();
}

function addOpenTask(owner, ev) {
  if (!AGENTS.includes(owner)) return;
  const jid = ev.job_id || '';
  if (!jid) return;
  state.openTasks[owner][jid] = {
    jobID: jid,
    route: ev.route || '-',
    text: short(ev.content || '-', 80),
    updatedAt: ev.timestamp || new Date().toISOString(),
  };
}

function doneOpenTask(owner, jid) {
  if (!AGENTS.includes(owner) || !jid) return;
  delete state.openTasks[owner][jid];
}

function openTaskSummary(agentID) {
  const m = state.openTasks[agentID] || {};
  const list = Object.values(m).sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));
  if (list.length === 0) return '-';
  if (list.length === 1) return list[0].text || list[0].route || list[0].jobID;
  return short((list[0].text || list[0].route || list[0].jobID) + ' / +' + String(list.length - 1), 90);
}

function updateAgents(ev) {
  const ts = ev.timestamp || new Date().toISOString();
  const route = ev.route || '-';
  const jid = ev.job_id || '-';

  if (ev.type === 'message.received') {
    touchAgent('mio', {state: 'running', route, lastEvent: ev.type, peer: ev.from || '-', preview: short(ev.content, 80), updatedAt: ts, jobID: jid});
    return;
  }
  if (ev.type === 'routing.decision') {
    touchAgent('mio', {state: 'running', reason: '', route, lastEvent: ev.type, peer: '-', preview: short(ev.content, 80), updatedAt: ts, jobID: jid});
    return;
  }

  const from = (ev.from || '').toLowerCase();
  const to = (ev.to || '').toLowerCase();

  if (AGENTS.includes(from)) {
    let s = 'running';
    if (ev.type === 'agent.thinking') s = 'thinking';
    if (ev.type === 'agent.response') {
      const c = (ev.content || '').toLowerCase();
      s = (c.includes('error') || c.includes('失敗')) ? 'error' : 'idle';
    }
    touchAgent(from, {
      state: s,
      reason: '',
      route,
      lastEvent: ev.type,
      peer: to || '-',
      preview: short(ev.content, 80),
      updatedAt: ts,
      jobID: jid,
    });
  }

  if (ev.type === 'agent.start' && AGENTS.includes(to)) {
    addOpenTask(to, ev);
    touchAgent(to, {
      state: 'running',
      reason: '',
      route,
      lastEvent: ev.type,
      peer: from || '-',
      preview: short(ev.content, 80),
      updatedAt: ts,
      jobID: jid,
    });
  }
  if (ev.type === 'agent.response' && AGENTS.includes(from)) {
    doneOpenTask(from, jid);
  }
  if (ev.type === 'agent.response' && to === 'mio') {
    touchAgent('mio', {
      state: 'idle',
      reason: '',
      route,
      lastEvent: ev.type,
      peer: from || '-',
      preview: short(ev.content, 80),
      updatedAt: ts,
      jobID: jid,
    });
  }
}


function renderEvidence() {
  const body = document.getElementById('evidenceBody');
  body.innerHTML = '';
  const fetchError = evidenceFetchErrorMessage();
  if (fetchError) {
    state.evidenceOrder = [];
    if (state.selectedEvidenceJobID) {
      state.selectedEvidenceJobID = '';
      state.selectedEvidenceItem = null;
      syncEvidenceQuery('');
    }
    const detail = document.getElementById('evidenceDetail');
    if (detail) detail.textContent = 'No selection';
    updateEvidenceNav();
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="11" class="small">Evidence / verification unavailable: ' + esc(fetchError) + '</td>';
    body.appendChild(tr);
    return;
  }
  const statusFilter = (eviStatus && eviStatus.value) ? eviStatus.value : '';
  const kindFilter = (eviErrorKind && eviErrorKind.value) ? eviErrorKind.value : '';
  const list = combinedEvidenceList().filter((r) => {
    if (statusFilter && String(r.status || '') !== statusFilter) return false;
    if (kindFilter && String(r.error_kind || '') !== kindFilter) return false;
    return true;
  }).sort((a, b) => {
    const ta = Date.parse(a.finished_at || a.created_at || 0) || 0;
    const tb = Date.parse(b.finished_at || b.created_at || 0) || 0;
    return state.evidenceSortDesc ? (tb - ta) : (ta - tb);
  });
  state.evidenceOrder = list.map((r) => String(r.job_id || '')).filter((id) => id !== '');
  if (state.selectedEvidenceJobID && state.evidenceOrder.indexOf(state.selectedEvidenceJobID) < 0) {
    state.selectedEvidenceJobID = '';
    state.selectedEvidenceItem = null;
    syncEvidenceQuery('');
    const detail = document.getElementById('evidenceDetail');
    if (detail) detail.textContent = 'No selection';
  }
  updateEvidenceNav();

  if (list.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="11" class="small">No execution evidence yet</td>';
    body.appendChild(tr);
    return;
  }

  list.forEach((r) => {
    const isVerificationReport = r._kind === 'verification_report';
    const st = isVerificationReport ? verificationStatusClass(r.status) : ((r.status === 'failed') ? 'error' : (r.status === 'passed' ? 'idle' : 'running'));
    const ek = String(r.error_kind || '');
    const stepsCount = Array.isArray(r.steps) ? r.steps.length : 0;
    const verifyCount = isVerificationReport ? Number(r.claim_count || 0) : (Array.isArray(r.verification) ? r.verification.length : 0);
    const latestVerify = isVerificationReport ? latestVerificationReportLink(r.job_id || '', r.status) : latestVerificationLink(r.job_id || '', r.verification);
    const tr = document.createElement('tr');
    if ((r.job_id || '') === (state.selectedEvidenceJobID || '')) tr.classList.add('evi-selected');
    tr.innerHTML =
      '<td class="code">' + esc((isVerificationReport ? 'verification_report:' : 'execution_report:') + (r.job_id || '-')) + '</td>' +
      '<td class="code">' + esc(r.job_id || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(st) + '">' + esc(r.status || '-') + '</span></td>' +
      '<td><span class="badge ' + (isVerificationReport ? 'state-thinking' : errorKindClass(ek)) + '">' + esc(ek || r.trigger_level || '-') + '</span></td>' +
      '<td>' + latestVerify + '</td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'steps\', event)">' + esc(String(stepsCount)) + '</button></td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'verification\', event)">' + esc(String(verifyCount)) + '</button></td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'\', event)">' + esc(short(r.goal || r.route || '-', 90)) + '</button></td>' +
      '<td>' + esc(String(r.attempt_count || 0)) + '</td>' +
      '<td>' + esc(String(r.repair_count || 0)) + '</td>' +
      '<td>' + esc(fdt(r.finished_at)) + '</td>' +
      '<td><button class="ctl-btn" onclick="openEvidence(\'' + esc(r.job_id || '') + '\')">View</button></td>';
    tr.style.cursor = 'pointer';
    tr.addEventListener('click', function(evt) {
      const t = evt.target;
      if (t && t.tagName === 'BUTTON') return;
      openEvidence(r.job_id || '');
    });
    body.appendChild(tr);
  });
}

function combinedEvidenceList() {
  const out = Array.isArray(state.evidence) ? state.evidence.slice() : [];
  const seenJobs = new Set(out.map((r) => String(r.job_id || '')).filter((id) => id !== ''));
  (state.verificationReports || []).forEach((r) => {
    const jobID = String(r.job_id || '');
    if (!jobID || seenJobs.has(jobID)) return;
    out.push(Object.assign({_kind: 'verification_report'}, r));
  });
  return out;
}

function evidenceFetchErrorMessage() {
  const parts = [];
  if (state.evidenceFetchError) parts.push('evidence: ' + state.evidenceFetchError);
  if (state.verificationFetchError) parts.push('verification: ' + state.verificationFetchError);
  return parts.join('; ');
}

function evidenceSummaryFetchErrorMessage() {
  const parts = [];
  if (state.evidenceSummaryFetchError) parts.push('evidence summary: ' + state.evidenceSummaryFetchError);
  if (state.verificationSummaryFetchError) parts.push('verification summary: ' + state.verificationSummaryFetchError);
  return parts.join('; ');
}

function renderEvidenceSummary() {
  const root = document.getElementById('evidenceSummaryCards');
  if (!root) return;
  const fetchError = evidenceSummaryFetchErrorMessage();
  if (fetchError) {
    root.innerHTML = '' +
      '<div class="card"><h4>Evidence Total</h4><div style="font-size:22px;font-weight:700">unavailable</div><div class="small">evidence summary unavailable: ' + esc(fetchError) + '</div></div>' +
      '<div class="card"><h4>Verification Reports</h4><div style="font-size:22px;font-weight:700">unavailable</div><div class="small">verification summary unavailable: ' + esc(fetchError) + '</div><div class="small">blocked: execution evidence state unreadable</div></div>';
    return;
  }
  const s = state.evidenceSummary || {};
  const st = s.status || {};
  const ek = s.error_kind || {};
  const vs = (state.verificationSummary || {}).status || {};
  const vl = (state.verificationSummary || {}).trigger_level || {};
  const total = (st.passed || 0) + (st.failed || 0) + (st.other || 0);
  const verifyTotal = (vs.verified || 0) + (vs.weakly_supported || 0) + (vs.unsupported || 0) + (vs.conflict || 0) + (vs.not_checked || 0);
  root.innerHTML = '' +
    '<div class="card"><h4>Evidence Total</h4><div style="font-size:22px;font-weight:700">' + esc(String(total)) + '</div></div>' +
    '<div class="card"><h4>Status</h4>' +
      '<div class="row"><span>passed</span><span class="badge state-idle">' + esc(String(st.passed || 0)) + '</span></div>' +
      '<div class="row"><span>failed</span><span class="badge state-error">' + esc(String(st.failed || 0)) + '</span></div>' +
      '<div class="row"><span>other</span><span class="badge state-offline">' + esc(String(st.other || 0)) + '</span></div>' +
    '</div>' +
    '<div class="card"><h4>Error Kind</h4>' +
      '<div class="row"><span>apply</span><span class="badge state-running">' + esc(String(ek.apply || 0)) + '</span></div>' +
      '<div class="row"><span>verify</span><span class="badge state-error">' + esc(String(ek.verify || 0)) + '</span></div>' +
      '<div class="row"><span>repair</span><span class="badge state-thinking">' + esc(String(ek.repair || 0)) + '</span></div>' +
      '<div class="row"><span>none</span><span class="badge state-offline">' + esc(String(ek.none || 0)) + '</span></div>' +
    '</div>' +
    '<div class="card"><h4>Verification Reports</h4>' +
      '<div class="row"><span>total</span><span class="badge state-running">' + esc(String(verifyTotal)) + '</span></div>' +
      '<div class="row"><span>verified</span><span class="badge state-idle">' + esc(String(vs.verified || 0)) + '</span></div>' +
      '<div class="row"><span>weak</span><span class="badge state-thinking">' + esc(String(vs.weakly_supported || 0)) + '</span></div>' +
      '<div class="row"><span>unsupported</span><span class="badge state-error">' + esc(String(vs.unsupported || 0)) + '</span></div>' +
      '<div class="row"><span>conflict</span><span class="badge state-error">' + esc(String(vs.conflict || 0)) + '</span></div>' +
      '<div class="row"><span>high</span><span class="badge state-error">' + esc(String(vl.high || 0)) + '</span></div>' +
    '</div>';
}




function refreshDerivedViews() {
  renderDeskViews();
  renderOps();
  if (typeof renderToolHarnessEvents === 'function') renderToolHarnessEvents();
  if (typeof renderDCITraces === 'function') renderDCITraces();
  renderDebugPanels();
  renderOverview();
  renderRoleSelector();
  renderProgress();
  renderSystem();
  renderSessions();
  renderJobs();
  renderEvidence();
  renderMemorySnapshot();
  renderMemoryEvents();
  renderRecallTraces();
}

function renderDeskViews() {
  if (typeof renderHomeDesk === 'function') renderHomeDesk();
  if (typeof renderDevelopDesk === 'function') renderDevelopDesk();
  if (typeof renderInstructionsDesk === 'function') renderInstructionsDesk();
  if (typeof renderReportsDesk === 'function') renderReportsDesk();
}

function refreshOpsData() {
  fetch('/viewer/logs?scope=persisted&limit=40')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'ops logs unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      const items = Array.isArray(data.items) ? data.items : [];
      state.ops.opsLogsFetchError = '';
      state.ops.persistedLogs = items;
      state.ops.lastMioReport = items.find((ev) => String(ev.from || '').toLowerCase() === 'mio' && String(ev.to || '').toLowerCase() === 'user') || null;
      state.ops.latestJobID = items[0] ? (items[0].job_id || '') : '';
      state.ops.latestRoute = items[0] ? (items[0].route || '') : '';
      state.ops.latestError = items.find((ev) => {
        const t = String(ev.type || '').toLowerCase();
        return t === 'agent.error' || t === 'mailbox.error' || t === 'worker.classified_failure';
      }) || null;
      renderOps();
      renderDeskViews();
    })
    .catch((err) => {
      state.ops.opsLogsFetchError = String(err && err.message ? err.message : err);
      state.ops.persistedLogs = [];
      state.ops.lastMioReport = null;
      state.ops.latestJobID = '';
      state.ops.latestRoute = '';
      state.ops.latestError = null;
      renderOps();
      renderDeskViews();
      console.error(err);
    });
}

function refreshToolHarnessData() {
  fetch('/viewer/tool-harness/recent?limit=30')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'tool harness unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.toolHarnessFetchError = '';
      state.ops.toolHarnessEvents = Array.isArray(data.items) ? data.items : [];
      if (typeof renderToolHarnessEvents === 'function') renderToolHarnessEvents();
      renderOps();
    })
    .catch((err) => {
      state.ops.toolHarnessFetchError = String(err && err.message ? err.message : err);
      state.ops.toolHarnessEvents = [];
      if (typeof renderToolHarnessEvents === 'function') renderToolHarnessEvents();
      renderOps();
      console.error(err);
    });
}

function refreshDCIData() {
  fetch('/viewer/dci/recent?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'dci trace unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.dciFetchError = '';
      state.ops.dciTraces = Array.isArray(data.items) ? data.items : [];
      if (typeof renderDCITraces === 'function') renderDCITraces();
      renderOps();
    })
    .catch((err) => {
      state.ops.dciFetchError = String(err && err.message ? err.message : err);
      state.ops.dciTraces = [];
      if (typeof renderDCITraces === 'function') renderDCITraces();
      renderOps();
      console.error(err);
    });
}

function refreshSandboxData() {
  fetch('/viewer/sandbox?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.sandboxFetchError = '';
      state.ops.sandboxes = Array.isArray(data.sandboxes) ? data.sandboxes : [];
      state.ops.sandboxArtifacts = Array.isArray(data.artifacts) ? data.artifacts : [];
      state.ops.sandboxPromotions = Array.isArray(data.promotions) ? data.promotions : [];
      state.ops.sandboxDecisions = Array.isArray(data.decisions) ? data.decisions : [];
      state.ops.sandboxGateLogs = Array.isArray(data.gate_logs) ? data.gate_logs : [];
      if (typeof renderSandboxStatus === 'function') renderSandboxStatus();
      renderOps();
    })
    .catch((err) => {
      state.ops.sandboxFetchError = String(err && err.message ? err.message : err);
      state.ops.sandboxes = [];
      state.ops.sandboxArtifacts = [];
      state.ops.sandboxPromotions = [];
      state.ops.sandboxDecisions = [];
      state.ops.sandboxGateLogs = [];
      if (typeof renderSandboxStatus === 'function') renderSandboxStatus();
      renderOps();
      console.error(err);
    });
}

function refreshSkillGovernanceData() {
  fetch('/viewer/skill-governance/recent?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.skillGovernanceFetchError = '';
      state.ops.skillManifests = Array.isArray(data.manifests) ? data.manifests : [];
      state.ops.skillTriggerLogs = Array.isArray(data.trigger_logs) ? data.trigger_logs : [];
      state.ops.skillChangeLogs = Array.isArray(data.change_logs) ? data.change_logs : [];
      state.ops.contributionGateLogs = Array.isArray(data.contributions) ? data.contributions : [];
      state.ops.skillExternalPRSubmitRecords = Array.isArray(data.external_pr_submit_records) ? data.external_pr_submit_records : [];
      state.ops.skillExternalPRAdapter = data.external_pr_adapter ? String(data.external_pr_adapter) : '';
      state.ops.skillExternalPRAdapterConfigured = Boolean(data.external_pr_adapter_configured);
      state.ops.skillExternalPRHumanApprovalRequired = data.human_approval_required_for_pr !== false;
      state.ops.coderTranscripts = Array.isArray(data.coder_transcripts) ? data.coder_transcripts : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.skillGovernanceFetchError = String(err && err.message ? err.message : err);
      state.ops.skillManifests = [];
      state.ops.skillTriggerLogs = [];
      state.ops.skillChangeLogs = [];
      state.ops.contributionGateLogs = [];
      state.ops.skillExternalPRSubmitRecords = [];
      state.ops.coderTranscripts = [];
      renderOps();
      console.error(err);
    });
}

function refreshWorkstreamData() {
  fetch('/viewer/workstreams?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.workstreamFetchError = '';
      state.ops.workstreams = Array.isArray(data.workstreams) ? data.workstreams : [];
      state.ops.workstreamGoals = Array.isArray(data.goals) ? data.goals : [];
      state.ops.workstreamArtifacts = Array.isArray(data.artifacts) ? data.artifacts : [];
      state.ops.workstreamAnnotations = Array.isArray(data.annotations) ? data.annotations : [];
      state.ops.workstreamSteering = Array.isArray(data.steering) ? data.steering : [];
      state.ops.workstreamHeartbeats = Array.isArray(data.heartbeats) ? data.heartbeats : [];
      state.ops.workstreamVaultUpdates = Array.isArray(data.vault_updates) ? data.vault_updates : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.workstreamFetchError = String(err && err.message ? err.message : err);
      state.ops.workstreams = [];
      state.ops.workstreamGoals = [];
      state.ops.workstreamArtifacts = [];
      state.ops.workstreamAnnotations = [];
      state.ops.workstreamSteering = [];
      state.ops.workstreamHeartbeats = [];
      state.ops.workstreamVaultUpdates = [];
      renderOps();
      console.error(err);
    });
}

function refreshRevenueData() {
  fetch('/viewer/revenue?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.revenueFetchError = '';
      state.ops.revenueMarketResearch = Array.isArray(data.market_research) ? data.market_research : [];
      state.ops.revenueSNSPostMetrics = Array.isArray(data.sns_post_metrics) ? data.sns_post_metrics : [];
      state.ops.revenueProducts = Array.isArray(data.products) ? data.products : [];
      state.ops.revenueCustomerVoices = Array.isArray(data.customer_voices) ? data.customer_voices : [];
      state.ops.revenueEvents = Array.isArray(data.revenue_events) ? data.revenue_events : [];
      state.ops.revenueHumanDecisions = Array.isArray(data.human_decisions) ? data.human_decisions : [];
      state.ops.revenueDailyRoutineReports = Array.isArray(data.daily_routine_reports) ? data.daily_routine_reports : [];
      state.ops.revenueChannelDrafts = Array.isArray(data.channel_drafts) ? data.channel_drafts : [];
      state.ops.revenueExternalSendApplyRecords = Array.isArray(data.external_send_apply_records) ? data.external_send_apply_records : [];
      state.ops.revenueExternalChannelAdapter = String(data.external_channel_adapter || '');
      state.ops.revenueExternalChannelAdapterConfigured = Boolean(data.external_channel_adapter_configured);
      state.ops.revenueExternalSendHumanApprovalRequired = Boolean(data.human_approval_required_for_external_send);
      state.ops.revenueSummary = data && data.summary && typeof data.summary === 'object' ? data.summary : null;
      renderOps();
    })
    .catch((err) => {
      state.ops.revenueFetchError = String(err && err.message ? err.message : err);
      state.ops.revenueMarketResearch = [];
      state.ops.revenueSNSPostMetrics = [];
      state.ops.revenueProducts = [];
      state.ops.revenueCustomerVoices = [];
      state.ops.revenueEvents = [];
      state.ops.revenueHumanDecisions = [];
      state.ops.revenueDailyRoutineReports = [];
      state.ops.revenueChannelDrafts = [];
      state.ops.revenueExternalSendApplyRecords = [];
      state.ops.revenueSummary = null;
      renderOps();
      console.error(err);
    });
}

function refreshPersonaObservationData() {
  fetch('/viewer/persona-observation?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'persona observation unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.personaObservationFetchError = '';
      state.ops.personaDiscomfortLogs = Array.isArray(data.discomfort_logs) ? data.discomfort_logs : [];
      state.ops.personaTriggerLogs = Array.isArray(data.trigger_logs) ? data.trigger_logs : [];
      state.ops.personaCanonicalResponseLogs = Array.isArray(data.canonical_response_logs) ? data.canonical_response_logs : [];
      state.ops.personaObservationLogs = Array.isArray(data.observation_logs) ? data.observation_logs : [];
      state.ops.personaMetaProfileUpdates = Array.isArray(data.meta_profile_updates) ? data.meta_profile_updates : [];
      state.ops.personaInterfaceSessions = Array.isArray(data.interface_sessions) ? data.interface_sessions : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.personaObservationFetchError = String(err && err.message ? err.message : err);
      state.ops.personaDiscomfortLogs = [];
      state.ops.personaTriggerLogs = [];
      state.ops.personaCanonicalResponseLogs = [];
      state.ops.personaObservationLogs = [];
      state.ops.personaMetaProfileUpdates = [];
      state.ops.personaInterfaceSessions = [];
      renderOps();
      console.error(err);
    });
}

function refreshBrowserTraceAPIData() {
  fetch('/viewer/browser-trace-api?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.browserTraceAPIFetchError = '';
      state.ops.browserTraceRuns = Array.isArray(data.trace_runs) ? data.trace_runs : [];
      state.ops.browserTraceAPICandidates = Array.isArray(data.api_candidates) ? data.api_candidates : [];
      state.ops.browserTraceAPISchemas = Array.isArray(data.api_schemas) ? data.api_schemas : [];
      state.ops.browserTraceAPICoverageReports = Array.isArray(data.coverage_reports) ? data.coverage_reports : [];
      state.ops.browserTraceAPIArtifacts = Array.isArray(data.api_artifacts) ? data.api_artifacts : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.browserTraceAPIFetchError = String(err && err.message ? err.message : err);
      state.ops.browserTraceRuns = [];
      state.ops.browserTraceAPICandidates = [];
      state.ops.browserTraceAPISchemas = [];
      state.ops.browserTraceAPICoverageReports = [];
      state.ops.browserTraceAPIArtifacts = [];
      renderOps();
      console.error(err);
    });
}

function refreshComplexityHotspotData() {
  fetch('/viewer/complexity-hotspots?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.complexityFetchError = '';
      state.ops.complexityScans = Array.isArray(data.scans) ? data.scans : [];
      state.ops.complexityHotspots = Array.isArray(data.hotspots) ? data.hotspots : [];
      state.ops.complexityEvidence = Array.isArray(data.evidence) ? data.evidence : [];
      state.ops.complexityReports = Array.isArray(data.reports) ? data.reports : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.complexityFetchError = String(err && err.message ? err.message : err);
      state.ops.complexityScans = [];
      state.ops.complexityHotspots = [];
      state.ops.complexityEvidence = [];
      state.ops.complexityReports = [];
      renderOps();
      console.error(err);
    });
}

function refreshSuperAgentData() {
  fetch('/viewer/superagent?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.superAgentFetchError = '';
      state.ops.superAgentRuns = Array.isArray(data.agent_runs) ? data.agent_runs : [];
      state.ops.superAgentSubagentTasks = Array.isArray(data.subagent_tasks) ? data.subagent_tasks : [];
      state.ops.superAgentContextPacks = Array.isArray(data.context_packs) ? data.context_packs : [];
      state.ops.superAgentMessageChannels = Array.isArray(data.message_channels) ? data.message_channels : [];
      state.ops.superAgentTraceEvents = Array.isArray(data.trace_events) ? data.trace_events : [];
      state.ops.superAgentRunQueue = Array.isArray(data.run_queue) ? data.run_queue : [];
      state.ops.superAgentRuntimeConfig = data.runtime_config && typeof data.runtime_config === 'object' ? data.runtime_config : null;
      renderOps();
    })
    .catch((err) => {
      state.ops.superAgentFetchError = String(err && err.message ? err.message : err);
      state.ops.superAgentRuns = [];
      state.ops.superAgentSubagentTasks = [];
      state.ops.superAgentContextPacks = [];
      state.ops.superAgentMessageChannels = [];
      state.ops.superAgentTraceEvents = [];
      state.ops.superAgentRunQueue = [];
      state.ops.superAgentRuntimeConfig = null;
      renderOps();
      console.error(err);
    });
}

function refreshAIWorkflowData() {
  fetch('/viewer/ai-workflow?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.aiWorkflowFetchError = '';
      state.ops.aiWorkflowEvents = Array.isArray(data.workflow_events) ? data.workflow_events : [];
      state.ops.aiWorkflowProjectMemoryIndexes = Array.isArray(data.project_memory_indexes) ? data.project_memory_indexes : [];
      state.ops.aiWorkflowWorktreeRegistries = Array.isArray(data.worktree_registries) ? data.worktree_registries : [];
      state.ops.aiWorkflowCommandRegistries = Array.isArray(data.command_registries) ? data.command_registries : [];
      state.ops.aiWorkflowContextUsages = Array.isArray(data.context_usages) ? data.context_usages : [];
      state.ops.aiWorkflowContextBudgetPolicy = data.context_budget_policy && typeof data.context_budget_policy === 'object' ? data.context_budget_policy : null;
      renderOps();
    })
    .catch((err) => {
      state.ops.aiWorkflowFetchError = String(err && err.message ? err.message : err);
      state.ops.aiWorkflowEvents = [];
      state.ops.aiWorkflowProjectMemoryIndexes = [];
      state.ops.aiWorkflowWorktreeRegistries = [];
      state.ops.aiWorkflowCommandRegistries = [];
      state.ops.aiWorkflowContextUsages = [];
      state.ops.aiWorkflowContextBudgetPolicy = null;
      renderOps();
      console.error(err);
    });
}

function refreshHeavyWorkerRuntimeDiagnostics() {
  fetch('/viewer/ai-workflow/heavy-worker/runtime-diagnostics', { cache: 'no-store' })
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'heavy worker runtime diagnostics unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.heavyWorkerRuntimeDiagnosticsFetchError = '';
      state.ops.heavyWorkerRuntimeDiagnostics = data || null;
      renderOps();
    })
    .catch((err) => {
      state.ops.heavyWorkerRuntimeDiagnosticsFetchError = String(err && err.message ? err.message : err);
      state.ops.heavyWorkerRuntimeDiagnostics = null;
      renderOps();
      console.error(err);
    });
}

function refreshKnowledgeMemoryData() {
  fetch('/viewer/knowledge-memory?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((body) => {
          throw new Error('HTTP ' + r.status + (body ? ': ' + body.trim() : ''));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.ops.knowledgeMemoryFetchError = '';
      state.ops.knowledgePersonalArchive = Array.isArray(data.personal_archive) ? data.personal_archive : [];
      state.ops.knowledgeCreativeItems = Array.isArray(data.creative_knowledge) ? data.creative_knowledge : [];
      state.ops.knowledgeNewsItems = Array.isArray(data.news_knowledge) ? data.news_knowledge : [];
      state.ops.knowledgeDailyIntakeRules = Array.isArray(data.daily_intake_rules) ? data.daily_intake_rules : [];
      state.ops.knowledgeTemporalMarkers = Array.isArray(data.temporal_markers) ? data.temporal_markers : [];
      state.ops.knowledgeDreamRuns = Array.isArray(data.dream_runs) ? data.dream_runs : [];
      renderOps();
    })
    .catch((err) => {
      state.ops.knowledgeMemoryFetchError = String(err && err.message ? err.message : err);
      state.ops.knowledgePersonalArchive = [];
      state.ops.knowledgeCreativeItems = [];
      state.ops.knowledgeNewsItems = [];
      state.ops.knowledgeDailyIntakeRules = [];
      state.ops.knowledgeTemporalMarkers = [];
      state.ops.knowledgeDreamRuns = [];
      state.ops.knowledgeMemoryDetail = null;
      renderOps();
      console.error(err);
    });
}

function refreshRuntimeBlockedRouteData() {
  const routes = [
    {label: 'Source Registry staging', path: '/viewer/source-registry?action=staging&limit=3'},
    {label: 'Memory Layers', path: '/viewer/memory/layers'},
    {label: 'Sandbox status', path: '/viewer/sandbox?limit=1'},
    {label: 'LLM Ops status', path: '/viewer/llm-ops/status'},
  ];
  Promise.all(routes.map((route) => {
    return fetch(route.path, {cache: 'no-store'})
      .then((r) => r.text().then((body) => ({
        label: route.label,
        path: route.path,
        status: r.status,
        ok: r.ok,
        body: body || '',
      })))
      .catch((err) => ({
        label: route.label,
        path: route.path,
        status: 0,
        ok: false,
        body: String(err && err.message ? err.message : err),
      }));
  }))
    .then((items) => {
      state.ops.runtimeBlockedRoutes = items;
      renderOps();
    })
    .catch((err) => console.error(err));
}

function refreshEvidence() {
  fetch('/viewer/evidence/recent?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'evidence unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.evidenceFetchError = '';
      state.evidence = Array.isArray(data.items) ? data.items : [];
      renderEvidence();
      renderDeskViews();
      if (state.pendingEvidenceJobID) {
        const want = state.pendingEvidenceJobID;
        const found = state.evidence.some((r) => String(r.job_id || '') === want);
        if (found) {
          state.pendingEvidenceJobID = '';
          openEvidence(want);
        } else {
          const detail = document.getElementById('evidenceDetail');
          if (detail) detail.innerHTML = '<span class="badge state-error">not found</span> job_id=' + esc(want);
          state.pendingEvidenceJobID = '';
          if (state.evidenceOrder.length > 0) {
            showToast('job_id not found, switched to newest evidence', 'error');
            openEvidence(state.evidenceOrder[0]);
          }
        }
      }
    })
    .catch((err) => {
      state.evidenceFetchError = String(err && err.message ? err.message : err);
      state.evidence = [];
      state.pendingEvidenceJobID = '';
      state.selectedEvidenceItem = null;
      renderEvidence();
      renderDeskViews();
      console.error(err);
    });
}

function refreshEvidenceSummary() {
  fetch('/viewer/evidence/summary')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'evidence summary unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.evidenceSummaryFetchError = '';
      state.evidenceSummary = data.summary || {status: {}, error_kind: {}};
      renderEvidenceSummary();
      renderDeskViews();
    })
    .catch((err) => {
      state.evidenceSummaryFetchError = String(err && err.message ? err.message : err);
      state.evidenceSummary = {status: {}, error_kind: {}};
      renderEvidenceSummary();
      renderDeskViews();
      console.error(err);
    });
}

function refreshVerification() {
  fetch('/viewer/verification/recent?limit=20')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'verification unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.verificationFetchError = '';
      state.verificationReports = Array.isArray(data.items) ? data.items : [];
      renderEvidence();
      renderDeskViews();
    })
    .catch((err) => {
      state.verificationFetchError = String(err && err.message ? err.message : err);
      state.verificationReports = [];
      renderEvidence();
      renderDeskViews();
      console.error(err);
    });
}

function refreshVerificationSummary() {
  fetch('/viewer/verification/summary')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'verification summary unavailable'));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.verificationSummaryFetchError = '';
      state.verificationSummary = data.summary || {status: {}, trigger_level: {}};
      renderEvidenceSummary();
      renderDeskViews();
    })
    .catch((err) => {
      state.verificationSummaryFetchError = String(err && err.message ? err.message : err);
      state.verificationSummary = {status: {}, trigger_level: {}};
      renderEvidenceSummary();
      renderDeskViews();
      console.error(err);
    });
}

function openEvidence(jobID) {
  if (!jobID) return;
  state.selectedEvidenceJobID = jobID;
  syncEvidenceQuery(jobID);
  renderEvidence();
  const hasVerificationOnly = (state.verificationReports || []).some((r) => String(r.job_id || '') === String(jobID)) &&
    !(state.evidence || []).some((r) => String(r.job_id || '') === String(jobID));
  const detailURL = hasVerificationOnly ? '/viewer/verification/detail?job_id=' : '/viewer/evidence/detail?job_id=';
  fetch(detailURL + encodeURIComponent(jobID))
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          const fallback = r.status === 404 ? 'evidence not found' : 'evidence detail fetch failed';
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || fallback));
        });
      }
      return r.json();
    })
    .then((data) => {
      state.selectedEvidenceItem = data.item || null;
      updateEvidenceNav();
      const el = document.getElementById('evidenceDetail');
      el.innerHTML = hasVerificationOnly ? renderVerificationReportDetail(state.selectedEvidenceItem || {}) : renderEvidenceDetail(state.selectedEvidenceItem || {});
      if (state.selectedEvidenceFocus) {
        scrollEvidenceFocus(state.selectedEvidenceFocus);
        state.selectedEvidenceFocus = '';
      }
    })
    .catch((err) => {
      console.error(err);
      state.selectedEvidenceItem = null;
      updateEvidenceNav();
      const el = document.getElementById('evidenceDetail');
      if (el) {
        const msg = String(err && err.message ? err.message : 'error');
        el.innerHTML = '<span class="badge state-error">' + esc(msg) + '</span> job_id=' + esc(jobID);
      }
      if (state.evidenceOrder.length > 0 && String(state.evidenceOrder[0]) !== String(jobID)) {
        showToast('evidence unavailable, switched to newest evidence', 'error');
        openEvidence(state.evidenceOrder[0]);
      }
    });
}
window.openEvidence = openEvidence;

function openEvidenceWithFocus(jobID, focus, evt) {
  if (evt && typeof evt.stopPropagation === 'function') evt.stopPropagation();
  state.selectedEvidenceFocus = String(focus || '');
  openEvidence(jobID);
}
window.openEvidenceWithFocus = openEvidenceWithFocus;

function updateEvidenceNav() {
  const order = state.evidenceOrder || [];
  const cur = String(state.selectedEvidenceJobID || '');
  const idx = order.indexOf(cur);
  if (eviPos) {
    if (order.length === 0 || idx < 0) eviPos.textContent = '- / -';
    else eviPos.textContent = String(idx + 1) + ' / ' + String(order.length);
  }
  if (eviPrev) eviPrev.disabled = !(idx > 0);
  if (eviNext) eviNext.disabled = !(idx >= 0 && idx < order.length - 1);
  if (eviCopy) eviCopy.disabled = !state.selectedEvidenceItem;
  if (eviCopySummary) eviCopySummary.disabled = !state.selectedEvidenceItem;
}

function openEvidenceAdjacent(delta) {
  const order = state.evidenceOrder || [];
  const cur = String(state.selectedEvidenceJobID || '');
  const idx = order.indexOf(cur);
  if (idx < 0) return;
  const next = idx + delta;
  if (next < 0 || next >= order.length) return;
  openEvidence(order[next]);
}

if (eviPrev) eviPrev.addEventListener('click', () => openEvidenceAdjacent(-1));
if (eviNext) eviNext.addEventListener('click', () => openEvidenceAdjacent(1));

function setEvidenceCopyFailure(btn, label, err) {
  if (!btn) return;
  const message = label + ' unavailable: ' + String(err && err.message ? err.message : err);
  btn.textContent = message;
  btn.title = message;
  showToast(label + ' failed', 'error');
}

if (eviCopy) eviCopy.addEventListener('click', () => {
  if (!state.selectedEvidenceItem) return;
  const text = JSON.stringify(state.selectedEvidenceItem, null, 2);
  writeClipboardText(text).then(() => {
    const old = eviCopy.textContent;
    eviCopy.textContent = 'Copied';
    eviCopy.title = '';
    showToast('Copied evidence JSON', 'success');
    setTimeout(() => { eviCopy.textContent = old; }, 1200);
  }).catch((err) => {
    console.error(err);
    setEvidenceCopyFailure(eviCopy, 'Evidence JSON copy', err);
  });
});
if (eviCopySummary) eviCopySummary.addEventListener('click', () => {
  if (!state.selectedEvidenceItem) return;
  const summary = buildEvidenceSummary(state.selectedEvidenceItem);
  writeClipboardText(summary).then(() => {
    const old = eviCopySummary.textContent;
    eviCopySummary.textContent = 'Copied';
    eviCopySummary.title = '';
    showToast('Copied evidence summary', 'success');
    setTimeout(() => { eviCopySummary.textContent = old; }, 1200);
  }).catch((err) => {
    console.error(err);
    setEvidenceCopyFailure(eviCopySummary, 'Evidence summary copy', err);
  });
});

function errorKindClass(kind) {
  const k = String(kind || '').toLowerCase();
  if (k === 'apply') return 'state-running';
  if (k === 'verify') return 'state-error';
  if (k === 'repair') return 'state-thinking';
  return 'state-offline';
}

function renderEvidenceDetail(item) {
  const steps = Array.isArray(item.steps) ? item.steps : [];
  const verification = Array.isArray(item.verification) ? item.verification : [];
  const statusClass = item.status === 'failed' ? 'state-error' : (item.status === 'passed' ? 'state-idle' : 'state-running');
  const errText = item.error ? esc(item.error) : '-';
  const stepHTML = steps.length > 0 ? steps.map((s, i) => (String(i + 1) + '. ' + esc(s))).join('<br>') : '-';
  const verifyHTML = verification.length > 0 ? verification.map((v, i) => (String(i + 1) + '. ' + renderVerificationLine(v))).join('<br>') : '-';
  return '' +
    '<div class="row"><span>Job ID</span><span class="code">' + esc(item.job_id || '-') + '</span></div>' +
    '<div class="row"><span>Status</span><span class="badge ' + statusClass + '">' + esc(item.status || '-') + '</span></div>' +
    '<div class="row"><span>Error Kind</span><span class="badge ' + errorKindClass(item.error_kind || '') + '">' + esc(item.error_kind || '-') + '</span></div>' +
    '<div class="row"><span>Goal</span><span>' + esc(item.goal || '-') + '</span></div>' +
    '<div class="row"><span>Attempt Count</span><span>' + esc(String(item.attempt_count || 0)) + '</span></div>' +
    '<div class="row"><span>Repair Count</span><span>' + esc(String(item.repair_count || 0)) + '</span></div>' +
    '<div id="evidenceSectionSteps" style="margin-top:8px"><b>Steps</b><div style="margin-top:4px;line-height:1.5">' + stepHTML + '</div></div>' +
    '<div id="evidenceSectionVerification" style="margin-top:8px"><b>Verification</b><div style="margin-top:4px;line-height:1.5">' + verifyHTML + '</div></div>' +
    '<div style="margin-top:8px"><b>Error</b><div style="margin-top:4px;line-height:1.5">' + errText + '</div></div>' +
    '<div style="margin-top:8px" class="small">Finished: ' + esc(fdt(item.finished_at)) + '</div>';
}

function renderVerificationLine(v) {
  const line = String(v || '');
  const lower = line.toLowerCase();
  if (lower.includes('verify:passed')) {
    return '<span class="badge state-idle">passed</span> ' + esc(line);
  }
  if (lower.includes('verify:failed')) {
    return '<span class="badge state-error">failed</span> ' + esc(line);
  }
  if (lower.includes('verify:error') || lower.includes('repair:error')) {
    return '<span class="badge state-error">error</span> ' + esc(line);
  }
  return '<span class="badge state-offline">note</span> ' + esc(line);
}

function renderVerificationReportDetail(item) {
  const claims = Array.isArray(item.claims) ? item.claims : [];
  const evidence = Array.isArray(item.evidence) ? item.evidence : [];
  const questions = Array.isArray(item.questions) ? item.questions : [];
  const status = String(item.status || '-');
  const claimHTML = claims.length > 0 ? claims.map((c, i) => {
    return String(i + 1) + '. <span class="badge ' + stateClass(verificationStatusClass(c.status)) + '">' + esc(c.status || '-') + '</span> ' + esc(c.text || '-') + ' <span class="small">' + esc(c.reason || '') + '</span>';
  }).join('<br>') : '-';
  const evidenceHTML = evidence.length > 0 ? evidence.map((ev, i) => {
    const support = ev.conflicts ? 'conflict' : (ev.supports ? 'support' : 'ref');
    const cls = ev.conflicts ? 'state-error' : (ev.supports ? 'state-idle' : 'state-offline');
    return String(i + 1) + '. <span class="badge ' + cls + '">' + esc(support) + '</span> ' + esc(ev.source_type || '-') + ':' + esc(ev.source_id || '-') + ' <span class="small">' + esc(ev.note || '') + '</span>';
  }).join('<br>') : '-';
  const questionHTML = questions.length > 0 ? questions.map((q, i) => String(i + 1) + '. ' + esc(q.query || '-')).join('<br>') : '-';
  return '' +
    '<div class="row"><span>Job ID</span><span class="code">' + esc(item.job_id || '-') + '</span></div>' +
    '<div class="row"><span>Status</span><span class="badge ' + stateClass(verificationStatusClass(status)) + '">' + esc(status) + '</span></div>' +
    '<div class="row"><span>Trigger</span><span class="badge state-thinking">' + esc(item.trigger_level || '-') + '</span></div>' +
    '<div class="row"><span>Route</span><span>' + esc(item.route || '-') + '</span></div>' +
    '<div class="row"><span>Counts</span><span>' + esc('claims=' + String(item.claim_count || 0) + ' verified=' + String(item.verified_count || 0) + ' weak=' + String(item.weak_count || 0) + ' unsupported=' + String(item.unsupported_count || 0) + ' conflict=' + String(item.conflict_count || 0)) + '</span></div>' +
    '<div id="evidenceSectionVerification" style="margin-top:8px"><b>Claims</b><div style="margin-top:4px;line-height:1.5">' + claimHTML + '</div></div>' +
    '<div style="margin-top:8px"><b>Evidence Refs</b><div style="margin-top:4px;line-height:1.5">' + evidenceHTML + '</div></div>' +
    '<div style="margin-top:8px"><b>Verification Questions</b><div style="margin-top:4px;line-height:1.5">' + questionHTML + '</div></div>' +
    '<div style="margin-top:8px" class="small">Created: ' + esc(fdt(item.created_at)) + '</div>';
}

function verificationStatusClass(status) {
  const s = String(status || '').toLowerCase();
  if (s === 'verified') return 'idle';
  if (s === 'weakly_supported' || s === 'not_checked') return 'thinking';
  if (s === 'unsupported' || s === 'conflict') return 'error';
  return 'offline';
}

function latestVerificationReportLink(jobID, status) {
  const cls = stateClass(verificationStatusClass(status));
  const badge = '<span class="badge ' + cls + '">' + esc(status || '-') + '</span>';
  if (!jobID) return badge;
  return '<button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(jobID) + '\', \'verification\', event)">' + badge + '</button>';
}

function latestVerificationBadge(list) {
  const arr = Array.isArray(list) ? list : [];
  if (arr.length === 0) return '<span class="badge state-offline">-</span>';
  const line = String(arr[arr.length - 1] || '').toLowerCase();
  if (line.includes('verify:passed')) return '<span class="badge state-idle">passed</span>';
  if (line.includes('verify:failed')) return '<span class="badge state-error">failed</span>';
  if (line.includes('verify:error') || line.includes('repair:error')) return '<span class="badge state-error">error</span>';
  return '<span class="badge state-offline">note</span>';
}

function latestVerificationLink(jobID, list) {
  const badge = latestVerificationBadge(list);
  if (!jobID) return badge;
  return '<button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(jobID) + '\', \'verification\', event)">' + badge + '</button>';
}

function latestVerificationLabel(list) {
  const arr = Array.isArray(list) ? list : [];
  if (arr.length === 0) return '-';
  const line = String(arr[arr.length - 1] || '').toLowerCase();
  if (line.includes('verify:passed')) return 'passed';
  if (line.includes('verify:failed')) return 'failed';
  if (line.includes('verify:error') || line.includes('repair:error')) return 'error';
  return 'note';
}

function buildEvidenceSummary(item) {
  const parts = [
    'job_id=' + String(item.job_id || '-'),
    'status=' + String(item.status || '-'),
    'error_kind=' + String(item.error_kind || '-'),
    'latest_verify=' + latestVerificationLabel(item.verification),
    'attempt_count=' + String(item.attempt_count || 0),
    'repair_count=' + String(item.repair_count || 0),
  ];
  if (item.error) parts.push('error=' + String(item.error));
  return parts.join(' | ');
}

function syncEvidenceQuery(jobID) {
  if (!window.history || !window.history.replaceState) return;
  const u = new URL(window.location.href);
  if (jobID) u.searchParams.set('job_id', String(jobID));
  else u.searchParams.delete('job_id');
  window.history.replaceState(null, '', u.toString());
}

function initTabFromQuery() {
  try {
    const u = new URL(window.location.href);
    const tab = (u.searchParams.get('tab') || '').trim().toLowerCase();
    if (tab && panels[tab]) {
      switchTab(tab);
    }
  } catch (_) {}
}

function initLiveMode() {
  try {
    const u = new URL(window.location.href);
    if (u.searchParams.get('mode') !== 'live') return false;
    document.body.classList.add('live-mode');
    switchTab('timeline');
    // ライブモードではIdleChat状態をポーリングしてトピックバーを更新
    setInterval(async () => {
      const topicEl = document.getElementById('liveTopicText');
      try {
        const r = await fetch('/viewer/idlechat/status');
        if (!r.ok) {
          const text = await r.text();
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'idlechat status unavailable'));
        }
        const d = await r.json();
        if (topicEl) {
          topicEl.textContent = d.current_topic || '-';
        }
      } catch (err) {
        if (topicEl) {
          topicEl.textContent = 'IdleChat status unavailable: ' + String(err && err.message ? err.message : err);
        }
      }
    }, 5000);
    return true;
  } catch (_) { return false; }
}

function initEvidenceFromQuery() {
  try {
    const u = new URL(window.location.href);
    const q = (u.searchParams.get('job_id') || '').trim();
    if (q) {
      state.pendingEvidenceJobID = q;
      switchTab('jobs');
    }
  } catch (_) {}
}

function scrollEvidenceFocus(focus) {
  const f = String(focus || '').toLowerCase();
  let id = '';
  if (f === 'verification') id = 'evidenceSectionVerification';
  if (f === 'steps') id = 'evidenceSectionSteps';
  if (!id) return;
  const el = document.getElementById(id);
  if (!el) return;
  el.scrollIntoView({behavior: 'smooth', block: 'center'});
}

let toastTimer = null;
let toastLastMsg = '';
let toastLastKind = '';
function showToast(msg, kind) {
  if (!toastEl) return;
  const text = String(msg || '');
  const k = (kind === 'success' || kind === 'error') ? kind : 'info';
  const same = toastEl.classList.contains('show') && toastLastMsg === text && toastLastKind === k;
  toastEl.textContent = text;
  toastEl.classList.remove('info', 'success', 'error');
  toastEl.classList.add(k);
  if (!same) {
    toastEl.classList.remove('show');
    // Force style flush so quick consecutive different messages animate reliably.
    void toastEl.offsetWidth;
    toastEl.classList.add('show');
  }
  toastLastMsg = text;
  toastLastKind = k;
  if (toastTimer) clearTimeout(toastTimer);
  let ms = 1800;
  if (k === 'success') ms = 1200;
  if (k === 'error') ms = 2600;
  toastTimer = setTimeout(() => {
    toastEl.classList.remove('show');
    toastLastMsg = '';
    toastLastKind = '';
  }, ms);
}

if (toastEl) {
  toastEl.addEventListener('click', () => {
    if (toastTimer) clearTimeout(toastTimer);
    toastEl.classList.remove('show');
    toastLastMsg = '';
    toastLastKind = '';
  });
}
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  if (!toastEl || !toastEl.classList.contains('show')) return;
  if (toastTimer) clearTimeout(toastTimer);
  toastEl.classList.remove('show');
  toastLastMsg = '';
  toastLastKind = '';
});

if (eviStatus) eviStatus.addEventListener('change', renderEvidence);
if (eviErrorKind) eviErrorKind.addEventListener('change', renderEvidence);
if (eviSort) eviSort.addEventListener('click', () => {
  state.evidenceSortDesc = !state.evidenceSortDesc;
  eviSort.textContent = state.evidenceSortDesc ? 'finished: newest' : 'finished: oldest';
  renderEvidence();
});

function degradeOfflineStates() {
  const now = Date.now();
  AGENTS.forEach((id) => {
    const s = state.agents[id];
    if (!s.updatedAt) return;
    if (s.state === 'unavailable') return;
    const diff = now - Date.parse(s.updatedAt);
    if (diff > OFFLINE_MS && s.state !== 'offline') s.state = 'offline';
  });
  renderOverview();
  renderRoleSelector();
  renderProgress();
}
setInterval(degradeOfflineStates, 5000);

function refreshViewerStatus() {
  fetch('/viewer/status')
    .then((r) => {
      if (!r.ok) {
        return r.text().then((text) => {
          throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'viewer status unavailable'));
        });
      }
      return r.json();
    })
    .then((payload) => applyMonitorStatusSnapshot(payload))
    .catch((err) => {
      const message = String(err && err.message ? err.message : err);
      state.viewerStatusFetchError = message;
      AGENTS.forEach((id) => {
        touchAgent(id, {
          state: 'unavailable',
          reason: 'viewer status unavailable: ' + message,
          route: '-',
          lastEvent: 'viewer status fetch failed',
          peer: '-',
          jobID: '-',
          preview: 'viewer status unavailable',
          updatedAt: new Date().toISOString(),
        });
      });
      renderOverview();
      renderRoleSelector();
      renderProgress();
      console.error(err);
    });
}

function ingestEvent(ev) {
  const key = eventKey(ev);
  if (seenEventKeys.has(key)) return;
  rememberEventKey(key);

  state.logs.push(ev);
  if (state.logs.length > MAX_LOGS) state.logs.shift();
  upsertSession(ev);
  upsertJob(ev);
  updateAgents(ev);
  addMsgToTimeline(ev);
  addIdleMsgToTimeline(ev);
  addIdleSummaryToTimeline(ev);
  if (ev.type === 'agent.thinking') {
    pushDebugTrace('think', {
      time: ftime(ev.timestamp),
      agent: agName(ev.from || '-'),
      job: ev.job_id || '-',
      text: short(ev.content || '', 240),
    });
  }
  handleTTSAudioEvent(ev);
  derivedDirty = true;
}

function handleTTSAudioEvent(ev) {
  chatAudioSync.handleEvent(ev);
}

function resolveTTSPlaybackURL(audioURL, audioPath) {
  const url = String(audioURL || '').trim();
  if (window.location.protocol === 'https:' && url.startsWith('http://') && audioPath) {
    return '/viewer/tts/audio?path=' + encodeURIComponent(audioPath);
  }
  if (url) return url;
  if (audioPath) return '/viewer/tts/audio?path=' + encodeURIComponent(audioPath);
  return '';
}

function createChatAudioSync() {
  const state = ttsPlayback;
  const completedSessions = new Set();
  const seenUtterances = new Set();

  const module = {
    state,
    audio: {
      ensure: ensureAudioInternal,
      enqueue: enqueueAudioChunkInternal,
      playNext: playNextInternal,
      disable: disableAudioInternal,
      unlock: unlockAudioInternal,
    },
    text: {
      show: showChunkTextInternal,
      clear: clearTextInternal,
      fallback: showFallbackChunkInternal,
    },
    lipSync: {
      start: startLipSyncInternal,
      stop: stopLipSyncInternal,
      clear: clearLipSyncSpeaking,
    },
    handleEvent,
    enqueueAudio: enqueueAudioChunkInternal,
    enqueueDisplayFallback: enqueueDisplayFallbackInternal,
    markAudioStarted,
    markSessionCompleted,
    resetCurrent: resetCurrentInternal,
    startTextFallback: startTextFallbackInternal,
    playNext: playNextInternal,
    disableAudio: disableAudioInternal,
    unlockAudio: unlockAudioInternal,
    ensureAudio: ensureAudioInternal,
    showFallbackChunk: showFallbackChunkInternal,
  };
  return module;

  function normalizeEvent(ev) {
    if (ev && ev.type === 'tts.session_completed') {
      try {
        const payload = JSON.parse(ev.content || '{}');
        return {
          eventType: 'session_completed',
          sessionId: String(payload.session_id || ev.session_id || '').trim(),
        };
      } catch (_) {
        return {
          eventType: 'session_completed',
          sessionId: String(ev.session_id || '').trim(),
        };
      }
    }
    if (!ev || ev.type !== 'tts.audio_chunk') return null;
    let payload = null;
    try {
      payload = JSON.parse(ev.content || '{}');
    } catch (_) {
      return null;
    }
    let url = String(payload.audio_url || '').trim();
    const audioPath = String(payload.audio_path || '').trim();
    const sessionId = String(payload.session_id || '').trim();
    const track = String(payload.track || payload.track_id || 'default').trim() || 'default';
    const chunkIndexRaw = Number(payload.chunk_index);
    const chunkIndex = Number.isFinite(chunkIndexRaw) ? Math.floor(chunkIndexRaw) : -1;
    const characterId = String(payload.character_id || payload.speaker || '').trim().toLowerCase();
    const text = String(payload.text || payload.speech_text || '').trim();
    const displayText = String(payload.display_text || payload.viewer_text || payload.text || '').trim();
    const responseId = String(payload.response_id || '').trim();
    const utteranceId = String(payload.utterance_id || '').trim() || (sessionId + ':' + String(chunkIndex));
    const mode = isIdleChatSessionId(sessionId) ? 'idlechat' : 'chat';
    url = resolveTTSPlaybackURL(url, audioPath);

    // Rewrite known remote URL patterns to browser-fetchable paths.
    const cacheMatch = url.match(/^http:\/\/192\.168\.1\.36:(8765|8766)\/cache-(?:a|b)\/([^/?#]+\.wav)$/i);
    if (cacheMatch) {
      url = `http://192.168.1.36:${cacheMatch[1]}/audio/${cacheMatch[2]}`;
    }
    return {
      eventType: 'chunk',
      url,
      audioPath,
      characterId,
      sessionId,
      track,
      chunkIndex,
      text,
      displayText,
      responseId,
      utteranceId,
      displayOnly: !url,
      mode,
    };
  }

  function handleEvent(ev) {
    const chunk = normalizeEvent(ev);
    if (!chunk) return;
    if (chunk.eventType === 'session_completed') {
      markSessionCompleted(chunk.sessionId);
      return;
    }
    if (!chunk.url) {
      if (chunk.displayText) enqueueDisplayFallbackInternal(chunk);
      return;
    }
    enqueueAudioChunkInternal(chunk);
  }

  function normalizeChunk(chunk) {
    const normalized = {
      url: String((chunk && chunk.url) || ''),
      characterId: String((chunk && chunk.characterId) || '').trim().toLowerCase(),
      sessionId: String((chunk && chunk.sessionId) || '').trim(),
      track: String((chunk && chunk.track) || 'default').trim() || 'default',
      chunkIndex: Number.isFinite(chunk && chunk.chunkIndex) ? chunk.chunkIndex : -1,
      text: String((chunk && chunk.text) || ''),
      displayText: String((chunk && (chunk.displayText || chunk.text)) || ''),
      responseId: String((chunk && chunk.responseId) || ''),
      utteranceId: String((chunk && chunk.utteranceId) || ''),
      displayOnly: Boolean(chunk && chunk.displayOnly),
      mode: String((chunk && chunk.mode) || '').trim(),
    };
    if (!normalized.mode) normalized.mode = isIdleChatSessionId(normalized.sessionId) ? 'idlechat' : 'chat';
    if (!normalized.utteranceId) {
      normalized.utteranceId = normalized.sessionId + ':' + String(normalized.chunkIndex >= 0 ? normalized.chunkIndex : state.seq + 1);
    }
    return normalized;
  }

  function enqueueAudioChunkInternal(chunk) {
    enqueueChunkInternal(Object.assign(normalizeChunk(chunk), {displayOnly: false}));
    playNextInternal();
  }

  function enqueueDisplayFallbackInternal(chunk) {
    enqueueChunkInternal(Object.assign(normalizeChunk(chunk), {url: '', displayOnly: true}));
    startTextFallbackInternal();
  }

  function enqueueChunkInternal(chunk) {
    if (chunk.utteranceId && seenUtterances.has(chunk.utteranceId)) return;
    if (chunk.utteranceId) seenUtterances.add(chunk.utteranceId);
    chunk.seq = ++state.seq;
    state.queue.push(chunk);
    sortQueue();
  }

  function sortQueue() {
    state.queue.sort((a, b) => {
      const ap = a.mode === 'idlechat' ? 1 : 0;
      const bp = b.mode === 'idlechat' ? 1 : 0;
      if (ap !== bp) return ap - bp;
      const aKey = `${a.sessionId}|${a.track}`;
      const bKey = `${b.sessionId}|${b.track}`;
      if (aKey === bKey && a.chunkIndex >= 0 && b.chunkIndex >= 0 && a.chunkIndex !== b.chunkIndex) {
        return a.chunkIndex - b.chunkIndex;
      }
      return a.seq - b.seq;
    });
  }

  function markSessionCompleted(sessionId) {
    const sid = String(sessionId || '').trim();
    if (!sid) return;
    completedSessions.add(sid);
    playNextInternal();
  }

  function canStartChunk(chunk) {
    if (!chunk) return false;
    if (chunk.mode !== 'idlechat') return true;
    const buffered = state.queue.filter((item) => item && item.mode === 'idlechat' && item.sessionId === chunk.sessionId && !item.displayOnly).length;
    return buffered >= 2 || completedSessions.has(chunk.sessionId);
  }

  function resetCurrentInternal() {
    stopLipSyncInternal(state.currentCharacterId);
    state.playing = false;
    state.currentCharacterId = '';
    state.currentText = '';
    state.currentDisplayText = '';
    state.currentSessionId = '';
    state.currentChunkIndex = -1;
    state.currentUtteranceId = '';
    state.currentResponseId = '';
    state.currentShown = false;
    setNowPlayingText('', '');
    clearTextInternal();
  }

  function disableAudioInternal() {
    if (state.playing && !state.currentShown && state.currentDisplayText) {
      showChunkTextInternal({
        characterId: state.currentCharacterId,
        displayText: state.currentDisplayText,
        text: state.currentText,
        sessionId: state.currentSessionId,
        chunkIndex: state.currentChunkIndex,
        utteranceId: state.currentUtteranceId,
        responseId: state.currentResponseId,
      });
    }
    if (state.audio) {
      try { state.audio.pause(); } catch (_) {}
      try { state.audio.removeAttribute('src'); } catch (_) {}
      try { state.audio.load(); } catch (_) {}
    }
    state.audioEnabled = false;
    state.unlocked = false;
    state.blocked = false;
    clearTTSAudioError();
    resetCurrentInternal();
    updateAudioButton();
    startTextFallbackInternal();
  }

  async function unlockAudioInternal() {
    state.audioEnabled = true;
    const audio = ensureAudioInternal();
    try {
      audio.pause();
      audio.muted = true;
      audio.src = 'data:audio/wav;base64,UklGRigAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQQAAAAA';
      await audio.play();
      audio.pause();
      audio.currentTime = 0;
      audio.removeAttribute('src');
      audio.load();
      audio.muted = false;
      state.unlocked = true;
      state.blocked = false;
      clearTTSAudioError();
      updateAudioButton();
      playNextInternal();
    } catch (err) {
      state.unlocked = false;
      state.blocked = true;
      setTTSAudioError(err);
      updateAudioButton();
      startTextFallbackInternal();
      console.error('tts audio unlock failed', err);
    }
  }

  function ensureAudioInternal() {
    if (!state.audio) {
      state.audio = new Audio();
      state.audio.preload = 'auto';
      prepareMobileInlineAudio(state.audio);
      state.audio.addEventListener('playing', markAudioStarted);
      state.audio.addEventListener('timeupdate', markAudioStarted);
      state.audio.addEventListener('ended', function() {
        resetCurrentInternal();
        playNextInternal();
      });
      state.audio.addEventListener('error', function() {
        resetCurrentInternal();
        playNextInternal();
      });
    }
    return state.audio;
  }

  function markAudioStarted() {
    if (!state.playing) return;
    if (state.currentShown) return;
    const audio = state.audio;
    if (audio && audio.currentTime <= 0 && audio.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) return;
    state.currentShown = true;
    startLipSyncInternal(state.currentCharacterId);
    setNowPlayingText(state.currentCharacterId, state.currentText);
    showChunkTextInternal({
      characterId: state.currentCharacterId,
      displayText: state.currentDisplayText,
      text: state.currentText,
      sessionId: state.currentSessionId,
      chunkIndex: state.currentChunkIndex,
      utteranceId: state.currentUtteranceId,
      responseId: state.currentResponseId,
    });
  }

  function showFallbackChunkInternal(item) {
    if (!item) return;
    showChunkTextInternal(item);
  }

  function showChunkTextInternal(item) {
    setCentralTTSSpeechText(
      String((item && item.characterId) || ''),
      String((item && (item.displayText || item.text)) || ''),
      String((item && item.sessionId) || ''),
      Number.isFinite(item && item.chunkIndex) ? item.chunkIndex : -1,
      String((item && item.utteranceId) || ''),
      String((item && item.responseId) || '')
    );
  }

  function clearTextInternal() {
    setCentralTTSSpeechText('', '');
  }

  function startLipSyncInternal(characterId) {
    setLipSyncSpeaking(characterId, true);
  }

  function stopLipSyncInternal(characterId) {
    setLipSyncSpeaking(characterId, false);
  }

  function startTextFallbackInternal() {
    if (state.playing || state.fallbackActive) return;
    if (state.queue.length === 0) {
      clearTextInternal();
      return;
    }
    const head = state.queue[0];
    if (state.audioEnabled && !state.blocked && !(head && head.displayOnly)) return;
    const next = state.queue.shift();
    state.fallbackActive = true;
    showFallbackChunkInternal(next);
    if (state.fallbackTimer) clearTimeout(state.fallbackTimer);
    state.fallbackTimer = setTimeout(function() {
      state.fallbackTimer = null;
      state.fallbackActive = false;
      const nextHead = state.queue[0];
      if (state.blocked || (nextHead && nextHead.displayOnly)) {
        startTextFallbackInternal();
        return;
      }
      playNextInternal();
    }, ttsDisplayDelay(next));
  }

  function playNextInternal() {
    if (state.playing) return;
    if (state.fallbackActive) return;
    if (!state.audioEnabled) {
      startTextFallbackInternal();
      return;
    }
    if (state.queue.length > 0 && state.queue[0].displayOnly) {
      startTextFallbackInternal();
      return;
    }
    if (state.blocked) {
      startTextFallbackInternal();
      return;
    }
    if (state.queue.length === 0) {
      module.lipSync.clear();
      setNowPlayingText('', '');
      clearTextInternal();
      return;
    }
    const next = state.queue[0];
    if (!canStartChunk(next)) return;
    state.queue.shift();
    const audio = ensureAudioInternal();
    state.playing = true;
    state.currentCharacterId = String((next && next.characterId) || '');
    state.currentText = String((next && next.text) || '');
    state.currentDisplayText = String((next && next.displayText) || state.currentText || '');
    state.currentSessionId = String((next && next.sessionId) || '');
    state.currentChunkIndex = Number.isFinite(next && next.chunkIndex) ? next.chunkIndex : -1;
    state.currentUtteranceId = String((next && next.utteranceId) || '');
    state.currentResponseId = String((next && next.responseId) || '');
    state.currentShown = false;
    audio.dataset.characterId = state.currentCharacterId;
    audio.src = String((next && next.url) || '');
    audio.play().then(function() {
      markAudioStarted();
      state.audioEnabled = true;
      state.unlocked = true;
      state.blocked = false;
      clearTTSAudioError();
      updateAudioButton();
    }).catch(function(err) {
      resetCurrentInternal();
      if (isAutoplayBlockedError(err)) {
        state.blocked = true;
        state.unlocked = false;
        state.queue.unshift(next);
      } else {
        state.blocked = false;
        showFallbackChunkInternal(next);
      }
      setTTSAudioError(err);
      console.error('tts audio play failed', err);
      updateAudioButton();
      if (state.blocked) startTextFallbackInternal();
      else playNextInternal();
    });
  }
}

const chatAudioSync = createChatAudioSync();

function updateAudioButton() {
  const status = !ttsPlayback.audioEnabled ? 'off' : (ttsPlayback.blocked ? 'blocked' : (ttsPlayback.unlocked ? 'ready' : ''));
  [audioBtn, liveAudioBtn].forEach(function(btn) {
    if (!btn) return;
    btn.classList.remove('ready', 'blocked', 'off');
    if (status) btn.classList.add(status);
    btn.textContent = ttsPlayback.audioEnabled ? '🔊' : '🔇';
    if (!ttsPlayback.audioEnabled) {
      btn.title = '音声はOFFです。タップしてON';
      btn.setAttribute('aria-label', '音声はOFFです。タップしてON');
    } else if (ttsPlayback.blocked) {
      const detail = ttsPlayback.audioError ? ' / ' + ttsPlayback.audioError : '';
      btn.title = '音声がブロックされています。タップして再許可' + detail;
      btn.setAttribute('aria-label', '音声がブロックされています。タップして再許可' + detail);
    } else if (ttsPlayback.unlocked) {
      btn.title = '音声は有効です';
      btn.setAttribute('aria-label', '音声は有効です');
    } else {
      btn.title = '音声を有効化';
      btn.setAttribute('aria-label', '音声を有効化');
    }
  });
}

function bindTTSAudioButton(btn) {
  if (btn) {
    btn.addEventListener('click', toggleTTSAudio);
  }
}

function resetCurrentTTSAudioState() {
  chatAudioSync.resetCurrent();
}

function disableTTSAudio() {
  chatAudioSync.disableAudio();
}

async function unlockTTSAudio() {
  await chatAudioSync.unlockAudio();
}

async function toggleTTSAudio() {
  if (ttsPlayback.audioEnabled && ttsPlayback.unlocked && !ttsPlayback.blocked) {
    disableTTSAudio();
    return;
  }
  await unlockTTSAudio();
}

function ensureTTSAudio() {
  return chatAudioSync.ensureAudio();
}

function isAutoplayBlockedError(err) {
  if (!err) return false;
  const name = String(err.name || '').trim();
  const msg = String(err.message || '').toLowerCase();
  if (name === 'NotAllowedError') return true;
  return msg.includes('user didn\'t interact') || msg.includes('notallowederror');
}

function isMobileControlViewport() {
  if (typeof window === 'undefined') return false;
  if (window.matchMedia && window.matchMedia('(max-width: 640px)').matches) return true;
  return Boolean(navigator.maxTouchPoints && window.innerWidth <= 900);
}

function prepareMobileInlineAudio(audio) {
  if (!audio) return;
  audio.playsInline = true;
  audio.setAttribute('playsinline', '');
  audio.setAttribute('webkit-playsinline', '');
}

function ensureVoiceChatForMobileControl() {
  if (isVoiceChatAllowed()) return true;
  if (!isMobileControlViewport()) return false;
  switchTab('timeline');
  return isVoiceChatAllowed();
}

function markTTSAudioStarted() {
  chatAudioSync.markAudioStarted();
}

function ttsDisplayDelay(item) {
  const text = String((item && (item.displayText || item.text)) || '');
  const len = Array.from(text).length;
  const punctuationPause = /[。！？!?]$/.test(text.trim()) ? 280 : 0;
  return Math.max(900, Math.min(3400, 520 + (len * 85) + punctuationPause));
}

function showTTSFallbackChunk(item) {
  chatAudioSync.showFallbackChunk(item);
}

function startTTSTextFallback() {
  chatAudioSync.startTextFallback();
}

function enqueueTTSAudio(url, characterId, sessionId, track, chunkIndex, text, displayText, responseId, utteranceId) {
  chatAudioSync.enqueueAudio({
    url: url,
    characterId: characterId || '',
    sessionId: sessionId || '',
    track: track || 'default',
    chunkIndex: Number.isFinite(chunkIndex) ? chunkIndex : -1,
    text: text || '',
    displayText: displayText || text || '',
    responseId: responseId || '',
    utteranceId: utteranceId || '',
  });
}

function enqueueTTSDisplayFallback(characterId, sessionId, track, chunkIndex, text, displayText, responseId, utteranceId) {
  chatAudioSync.enqueueDisplayFallback({
    url: '',
    characterId: characterId || '',
    sessionId: sessionId || '',
    track: track || 'default',
    chunkIndex: Number.isFinite(chunkIndex) ? chunkIndex : -1,
    text: text || '',
    displayText: displayText || text || '',
    responseId: responseId || '',
    utteranceId: utteranceId || '',
  });
}

function playNextTTSAudio() {
  chatAudioSync.playNext();
}

const inp = document.getElementById('inp');
const sendBtn = document.getElementById('sendBtn');
const attachBtn = document.getElementById('attachBtn');
const cameraBtn = document.getElementById('cameraBtn');
const attachInput = document.getElementById('attachInput');
const cameraInput = document.getElementById('cameraInput');
const attachmentTray = document.getElementById('attachmentTray');
bindTTSAudioButton(audioBtn);
bindTTSAudioButton(liveAudioBtn);
updateAudioButton();
let sending = false;
let viewerAttachments = [];
function autoResize() {
  inp.style.height = 'auto';
  inp.style.height = Math.min(inp.scrollHeight, 120) + 'px';
}
inp.addEventListener('input', autoResize);
inp.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    send();
  }
});
sendBtn.addEventListener('click', send);
if (attachBtn && attachInput) attachBtn.addEventListener('click', () => attachInput.click());
if (cameraBtn && cameraInput) cameraBtn.addEventListener('click', () => cameraInput.click());
if (attachInput) attachInput.addEventListener('change', () => addViewerAttachments(attachInput.files, attachInput));
if (cameraInput) cameraInput.addEventListener('change', () => addViewerAttachments(cameraInput.files, cameraInput));

function addViewerAttachments(files, input) {
  state.viewerAttachmentError = '';
  Array.from(files || []).forEach((file) => {
    if (!viewerAttachmentAccepted(file)) {
      const name = String(file && file.name ? file.name : 'attachment');
      state.viewerAttachmentError = 'Attachment unavailable: unsupported file type: ' + name;
      showToast('未対応の添付形式です', 'error');
      return;
    }
    state.viewerAttachmentError = '';
    viewerAttachments.push(file);
  });
  if (input) input.value = '';
  renderAttachmentTray();
}

function viewerAttachmentAccepted(file) {
  const type = String(file && file.type || '').toLowerCase();
  const name = String(file && file.name || '').toLowerCase();
  return type.startsWith('image/') || type === 'application/pdf' || type.startsWith('text/') ||
    /\.(txt|md|json|csv|yaml|yml)$/.test(name);
}

function renderAttachmentTray() {
  if (!attachmentTray) return;
  attachmentTray.innerHTML = '';
  attachmentTray.classList.toggle('has-items', viewerAttachments.length > 0 || Boolean(state.viewerAttachmentError));
  if (state.viewerAttachmentError) {
    const err = document.createElement('span');
    err.className = 'attachment-chip attachment-error';
    err.textContent = state.viewerAttachmentError;
    attachmentTray.appendChild(err);
  }
  viewerAttachments.forEach((file, index) => {
    const chip = document.createElement('span');
    chip.className = 'attachment-chip';
    const name = document.createElement('span');
    name.className = 'name';
    name.textContent = file.name || 'attachment';
    const size = document.createElement('span');
    size.className = 'size';
    size.textContent = formatAttachmentSize(file.size || 0);
    const remove = document.createElement('button');
    remove.className = 'attachment-remove';
    remove.type = 'button';
    remove.title = '添付を外す';
    remove.textContent = '×';
    remove.addEventListener('click', () => {
      viewerAttachments.splice(index, 1);
      renderAttachmentTray();
    });
    chip.append(name, size, remove);
    attachmentTray.appendChild(chip);
  });
}

function formatAttachmentSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return Math.round(bytes / 1024) + ' KiB';
  return Math.round(bytes / (1024 * 1024)) + ' MiB';
}

function send() {
  const text = inp.value.trim();
  const message = text;
  const attachments = viewerAttachments.slice();
  if ((!text && attachments.length === 0) || sending) return;
  sending = true;
  sendBtn.disabled = true;
  inp.disabled = true;
  if (attachBtn) attachBtn.disabled = true;
  if (cameraBtn) cameraBtn.disabled = true;

  const sendPromise = attachments.length > 0 ? sendViewerMessage(message, attachments) : sendViewerMessage(message);
  sendPromise
  .then(() => {
    inp.value = '';
    viewerAttachments = [];
    renderAttachmentTray();
    autoResize();
  })
  .catch((err) => {
    const message = 'Viewer send unavailable: ' + String(err && err.message ? err.message : err);
    addMsgToTimeline({
      type: 'agent.response',
      from: 'mio',
      to: 'user',
      timestamp: new Date().toISOString(),
      content: message,
    });
    console.error(err);
  })
  .finally(() => {
    sending = false;
    sendBtn.disabled = false;
    inp.disabled = false;
    if (attachBtn) attachBtn.disabled = false;
    if (cameraBtn) cameraBtn.disabled = false;
    inp.focus();
  });
}

async function sendViewerMessage(message, attachments = []) {
  const body = buildViewerSendRequest(message);
  if (!body.message && (!attachments || attachments.length === 0)) throw new Error('message or attachment is required');
  await ensureViewerLLMReadyForRequest(body);
  let request;
  if (attachments && attachments.length > 0) {
    const form = new FormData();
    Object.entries(body).forEach(([key, value]) => form.append(key, value || ''));
    attachments.forEach((file) => form.append('attachments[]', file, file.name));
    request = {method: 'POST', body: form};
  } else {
    request = {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    };
  }
  const r = await fetch('/viewer/send', request);
  if (!r.ok) {
    const text = await r.text();
    throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'send failed'));
  }
  return {ok: true};
}

function buildViewerStatusSnapshot() {
  const items = {};
  AGENTS.forEach((id) => {
    const current = state.agents[id] || {};
    items[id] = {
      state: current.state || 'offline',
      reason: current.reason || '',
      route: current.route || '-',
      updated_at: current.updatedAt || '',
      preview: current.preview || '',
    };
  });
  const jobs = Object.values(state.jobs || {});
  return {
    generated_at: new Date().toISOString(),
    timeline_event_count: state.logs.length,
    job_count: jobs.length,
    running_job_count: jobs.filter((j) => String(j.status || '') !== 'done').length,
    agents: items,
  };
}

function registerWebMCPTools() {
  const modelContext = navigator && navigator.modelContext;
  if (!modelContext || typeof modelContext.registerTool !== 'function') {
    console.info('[WebMCP] navigator.modelContext is unavailable on this browser/page');
    return;
  }

  const toolSignalController = new AbortController();
  window.addEventListener('pagehide', () => toolSignalController.abort(), {once: true});

  try {
    modelContext.registerTool({
      name: 'viewer.get_status',
      title: 'Get Viewer Status',
      description: 'Return RenCrow viewer status including agent states and running jobs.',
      inputSchema: {
        type: 'object',
        properties: {},
        additionalProperties: false,
      },
      annotations: {
        readOnlyHint: true,
      },
      execute: async function() {
        return buildViewerStatusSnapshot();
      },
    }, {signal: toolSignalController.signal});

    modelContext.registerTool({
      name: 'viewer.send_message',
      title: 'Send Viewer Message',
      description: 'Send a user message to RenCrow via viewer send endpoint.',
      inputSchema: {
        type: 'object',
        properties: {
          message: {
            type: 'string',
            description: 'Message body to send to RenCrow.',
            minLength: 1,
          },
        },
        required: ['message'],
        additionalProperties: false,
      },
      execute: async function(input) {
        const msg = input && typeof input.message === 'string' ? input.message : '';
        await sendViewerMessage(msg);
        return {
          ok: true,
          message_preview: short(msg, 120),
        };
      },
    }, {signal: toolSignalController.signal});

    console.info('[WebMCP] tools registered: viewer.get_status, viewer.send_message');
  } catch (err) {
    console.error('[WebMCP] tool registration failed', err);
  }
}

function connect() {
  const es = new EventSource('/viewer/events');
  const dot = document.getElementById('dot');
  const stxt = document.getElementById('stxt');
  es.onopen = () => {
    dot.className = 'dot';
    stxt.textContent = 'Connected';
  };
  es.onmessage = (e) => {
    try { ingestEvent(JSON.parse(e.data)); }
    catch (err) { console.error(err); }
  };
  es.onerror = () => {
    dot.className = 'dot off';
    stxt.textContent = 'Reconnecting...';
    es.close();
    setTimeout(connect, 3000);
  };
}

refreshDerivedViews();
if (typeof bindHomeDeskControls === 'function') bindHomeDeskControls();
if (typeof bindDevelopDeskControls === 'function') bindDevelopDeskControls();
if (typeof bindInstructionsDeskControls === 'function') bindInstructionsDeskControls();
if (typeof bindReportsDeskControls === 'function') bindReportsDeskControls();
renderIdleChat();
setIdleSelectedMode(state.idleChat.selectedMode);
setIdleSelectedView(state.idleChat.selectedView);
refreshIdleStatus();
refreshIdleLogs();
if (!initLiveMode()) {
  initTabFromQuery();
}
initEvidenceFromQuery();
refreshOpsData();
refreshToolHarnessData();
refreshDCIData();
refreshSandboxData();
refreshSkillGovernanceData();
refreshWorkstreamData();
refreshRevenueData();
refreshPersonaObservationData();
refreshBrowserTraceAPIData();
refreshComplexityHotspotData();
refreshAIWorkflowData();
refreshSuperAgentData();
refreshHeavyWorkerRuntimeDiagnostics();
refreshKnowledgeMemoryData();
refreshRuntimeBlockedRouteData();
refreshEvidence();
refreshEvidenceSummary();
refreshVerification();
refreshVerificationSummary();
refreshMemorySnapshot();
refreshRecallTraces();
refreshViewerStatus();
setInterval(() => {
  if (!derivedDirty) return;
  refreshDerivedViews();
  derivedDirty = false;
}, 500);
setInterval(refreshViewerStatus, 5000);
setInterval(refreshIdleStatus, 3000);
setInterval(refreshIdleLogs, 5000);
setInterval(refreshOpsData, 5000);
setInterval(refreshToolHarnessData, 5000);
setInterval(refreshDCIData, 5000);
setInterval(refreshSandboxData, 5000);
setInterval(refreshSkillGovernanceData, 5000);
setInterval(refreshWorkstreamData, 5000);
setInterval(refreshRevenueData, 5000);
setInterval(refreshPersonaObservationData, 5000);
setInterval(refreshBrowserTraceAPIData, 5000);
setInterval(refreshComplexityHotspotData, 5000);
setInterval(refreshAIWorkflowData, 5000);
setInterval(refreshSuperAgentData, 5000);
setInterval(refreshHeavyWorkerRuntimeDiagnostics, 5000);
setInterval(refreshKnowledgeMemoryData, 5000);
setInterval(refreshRuntimeBlockedRouteData, 5000);
setInterval(refreshEvidence, 5000);
setInterval(refreshEvidenceSummary, 5000);
setInterval(refreshVerification, 5000);
setInterval(refreshVerificationSummary, 5000);
setInterval(refreshMemorySnapshot, 15000);
setInterval(refreshRecallTraces, 15000);
setInterval(refreshDebugSystem, 5000);
setInterval(() => {
  const panel = document.getElementById('llmOpsPanel');
  if (panel && state.ops.llmOpsEnabled) refreshLlmOpsStatus();
}, 5000);
refreshDebugSystem();
registerWebMCPTools();
connect();

// === STT (Speech-to-Text) realtime PCM16 streaming ===
const sttState = {
  ws: null,
  audioContext: null,
  audioStream: null,
  scriptNode: null,
  isRecording: false,
  isStopping: false,
  keepSessionChannel: false,
  streamReady: false,
  sampleRate: 16000,
  inputSampleRate: 48000,
  chunkBuffer: [],
  chunkSamples: 1600,
  draftBuffer: [],        // Recent window for draft (1 second)
  draftTimer: null,
  finalWaitTimer: null,
  reconnectTimer: null,
  reconnecting: false,
  stopControlSent: false,
  finalReceived: false,
  captureLog: [],
  capturePCM: [],
  captureStartedAt: '',
  captureEndedAt: '',
  captureSessionID: '(unknown)',
  captureEventID: '',
  captureActionError: '',
  sentAudioSamples: 0,
  sentAudioBytes: 0,
  sentAudioFrames: 0,
  lastLoggedAudioSecond: 0,
  lastRecognitionText: '',
  lastRecognitionType: '',
  partialCaptionText: '',
  finalCaptionText: '',
  errorCaptionText: '',
  inputLevel: 0,
  voiceBridgeURL: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/stt`,
  sttBaseURL: '',
  runtimeConfigLoaded: false
};

const micBtn = document.getElementById('micBtn');
const micStateEl = document.getElementById('micState');
const sttConnStateEl = document.getElementById('sttConnState');
const sttSessionStateEl = document.getElementById('sttSessionState');
const sttCaptionEl = document.getElementById('sttCaption');
const sttCaptionLabelEl = document.getElementById('sttCaptionLabel');
const sttCaptionTextEl = document.getElementById('sttCaptionText');
const debugSttSessionEl = document.getElementById('debugSttSession');
const sttCaptureCopyBtn = document.getElementById('sttCaptureCopyBtn');
const sttCaptureDownloadBtn = document.getElementById('sttCaptureDownloadBtn');
const sttCaptureClearBtn = document.getElementById('sttCaptureClearBtn');
const sttSessionCopyBtn = document.getElementById('sttSessionCopyBtn');
const STT_FINAL_WAIT_TIMEOUT_MS = 90000;
const STT_STOP_TAIL_SILENCE_MS = 1000;
if (micBtn) {
  micBtn.addEventListener('click', toggleSTT);
}
if (sttCaptureCopyBtn) {
  sttCaptureCopyBtn.addEventListener('click', copySTTCaptureLog);
}
if (sttCaptureDownloadBtn) {
  sttCaptureDownloadBtn.addEventListener('click', downloadSTTCaptureLog);
}
if (sttCaptureClearBtn) {
  sttCaptureClearBtn.addEventListener('click', clearSTTCaptureLog);
}
if (sttSessionCopyBtn) {
  sttSessionCopyBtn.addEventListener('click', copySTTSessionID);
}
sttControlsReady = true;
updateSTTInputIndicators();
loadViewerRuntimeConfig();

function isVoiceChatAllowed() {
  return activeViewerTab === 'timeline' && !document.body.classList.contains('live-mode');
}

async function loadViewerRuntimeConfig() {
  try {
    const res = await fetch('/viewer/runtime-config', { cache: 'no-store' });
    if (!res.ok) {
      const text = await res.text();
      syncLLMOpsPanel(null, 'HTTP ' + String(res.status) + ': ' + (text || res.statusText || 'runtime config unavailable'));
      return;
    }
    const cfg = await res.json();
    if (cfg && cfg.stt_stream_url) {
      sttState.voiceBridgeURL = String(cfg.stt_stream_url).trim() || sttState.voiceBridgeURL;
    }
    if (cfg && cfg.stt_base_url) {
      sttState.sttBaseURL = String(cfg.stt_base_url).trim();
    }
    sttState.runtimeConfigLoaded = true;
    updateSTTInputIndicators();
    syncLLMOpsPanel(cfg, '');
    loadViewerDebugSystemSnapshot();
  } catch (err) {
    const message = String(err && err.message ? err.message : err);
    console.warn('[STT] runtime config unavailable:', err);
    syncLLMOpsPanel(null, message);
  }
}

async function loadViewerDebugSystemSnapshot() {
  if (typeof syncRuntimeDebugSystem !== 'function') return;
  try {
    const res = await fetch('/viewer/debug/system', { cache: 'no-store' });
    if (!res.ok) {
      const text = await res.text();
      syncRuntimeDebugSystem(null, 'HTTP ' + String(res.status) + ': ' + (text || res.statusText || 'debug system unavailable'));
      return;
    }
    syncRuntimeDebugSystem(await res.json(), '');
  } catch (err) {
    const message = String(err && err.message ? err.message : err);
    console.warn('[Runtime] debug system snapshot unavailable:', err);
    syncRuntimeDebugSystem(null, message);
  }
}

function recordSTTCaptureEvent(type, payload) {
  if (type !== 'speech_start' && type !== 'start' && type !== 'stop' && type !== 'draft' && type !== 'partial' && type !== 'final' && type !== 'progress' && type !== 'audio_sent' && type !== 'ready' && type !== 'closed' && type !== 'error' && type !== 'ws_open' && type !== 'ws_error' && type !== 'ws_close') return;
  const rawPayload = String(payload || '').trim();
  if (type === 'speech_start' || type === 'ready' || type === 'closed' || type === 'ws_open' || type === 'ws_close') {
    payload = '-';
  } else {
    if (!rawPayload) return;
    payload = rawPayload;
  }
  const nowISO = new Date().toISOString();
  if (!sttState.captureStartedAt) {
    sttState.captureStartedAt = nowISO;
  }
  sttState.captureEndedAt = nowISO;
  sttState.captureLog.push({
    time: ftime(nowISO),
    type,
    payload: String(payload || '-'),
  });
  if (sttState.captureLog.length > 200) {
    sttState.captureLog.shift();
  }
}

function renderSTTDebugPanelsSafely() {
  try {
    renderDebugPanels();
  } catch (err) {
    console.warn('[STT] Debug panel render skipped:', err && err.message ? err.message : err);
  }
}

function getSTTCaptureSummaryText() {
  const finals = sttState.captureLog
    .filter((item) => (item.type === 'final' || item.type === 'partial' || item.type === 'draft') && item.payload && item.payload !== '-')
    .map((item) => item.payload.trim().split(' / ')[0].trim())
    .filter(Boolean);
  return finals.length > 0 ? finals.slice(-3).join(' / ') : '-';
}

function buildSTTCaptureLogText() {
  const startedAt = sttState.captureStartedAt ? fdt(sttState.captureStartedAt) : '-';
  const endedAt = sttState.captureEndedAt ? fdt(sttState.captureEndedAt) : '-';
  const meta = [
    '# Client STT Log',
    'client_url: ' + window.location.href,
    'ws_url: ' + sttState.voiceBridgeURL,
    'test_time: ' + startedAt + ' ~ ' + endedAt,
    'session_id: ' + (sttState.captureSessionID || '(unknown)'),
    'event_id: ' + (sttState.captureEventID || '(unknown)'),
    'sent_audio: ' + formatSTTSentAudioSummary(),
    'spoken_text: ' + getSTTCaptureSummaryText(),
    '',
  ];
  const body = sttState.captureLog.slice().reverse().map((item) => {
    return `${item.time || '--:--:--'} · ${item.type || '-'}\n${item.payload || '-'}`;
  });
  if (body.length === 0) {
    body.push('NO_STT_EVENTS');
  }
  return meta.concat(body).join('\n');
}

function formatSTTSentAudioSummary() {
  const sampleRate = Number(sttState.sampleRate || 16000) || 16000;
  const seconds = sampleRate > 0 ? sttState.sentAudioSamples / sampleRate : 0;
  return `${seconds.toFixed(3)}s / ${sttState.sentAudioBytes} bytes / ${sttState.sentAudioFrames} frames`;
}

function formatSTTServerEventPayload(msg, fallbackText) {
  const parts = [];
  const text = String(fallbackText || '').trim();
  if (text) parts.push(text);
  if (msg && msg.seq !== undefined && msg.seq !== null) parts.push('seq=' + String(msg.seq));
  if (msg && msg.start_ms !== undefined && msg.end_ms !== undefined) {
    parts.push('range=' + String(msg.start_ms) + '-' + String(msg.end_ms) + 'ms');
  }
  if (msg && msg.duration !== undefined && msg.duration !== null) {
    parts.push('duration=' + String(msg.duration) + 's');
  }
  if (msg && msg.reason) parts.push('reason=' + String(msg.reason));
  return parts.join(' / ');
}

function describeSTTActionError(prefix, err) {
  return prefix + ': ' + String(err && err.message ? err.message : err);
}

function copySTTCaptureLog() {
  const text = buildSTTCaptureLogText();
  writeClipboardText(text).then(() => {
    sttState.captureActionError = '';
    updateSTTInputIndicators();
    showToast('STTログをコピーしました', 'success');
  }).catch((err) => {
    sttState.captureActionError = describeSTTActionError('STT log copy unavailable', err);
    updateSTTInputIndicators();
    console.error('[STT] copy failed:', err);
    showToast('STTログコピー失敗', 'error');
  });
}

function downloadSTTCaptureLog() {
  const text = buildSTTCaptureLogText();
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'client_stt_log.txt';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
  showToast('client_stt_log.txt を保存しました', 'success');
}

function clearSTTCaptureLog() {
  sttState.captureLog = [];
  sttState.capturePCM = [];
  sttState.captureStartedAt = '';
  sttState.captureEndedAt = '';
  sttState.partialCaptionText = '';
  sttState.finalCaptionText = '';
  sttState.errorCaptionText = '';
  updateSTTCaption();
  updateSTTInputIndicators();
  showToast('STTログをクリアしました', 'success');
}

function copySTTSessionID() {
  const sid = String(sttState.captureSessionID || '(unknown)').trim() || '(unknown)';
  if (sid === '(unknown)') {
    showToast('SessionID未受信です', 'error');
    return;
  }
  writeClipboardText(sid).then(() => {
    sttState.captureActionError = '';
    updateSTTInputIndicators();
    showToast('SessionIDをコピーしました', 'success');
  }).catch((err) => {
    sttState.captureActionError = describeSTTActionError('STT session copy unavailable', err);
    updateSTTInputIndicators();
    console.error('[STT] session_id copy failed:', err);
    showToast('SessionIDコピー失敗', 'error');
  });
}

async function persistSTTLogToServer(logText) {
  const res = await fetch('/viewer/stt/log', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({content: logText}),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error('HTTP ' + String(res.status) + ': ' + (text || res.statusText || 'stt log save failed'));
  }
}

async function persistSTTWavToServer(wavBuffer) {
  const res = await fetch('/viewer/stt/wav', {
    method: 'POST',
    headers: {'Content-Type': 'audio/wav'},
    body: wavBuffer,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error('HTTP ' + String(res.status) + ': ' + (text || res.statusText || 'stt wav save failed'));
  }
}

async function runSTTAutoTest() {
  const providerURL = buildSTTProviderURLForAutoTest();
  const res = await fetch('/viewer/stt/autotest', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      provider_url: providerURL,
      provider_rounds: 1,
      ws_rounds: 1,
      ws_wait: 8,
    }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error('HTTP ' + String(res.status) + ': ' + (text || res.statusText || 'stt autotest failed'));
  }
}

function buildSTTProviderURLForAutoTest() {
  const state = typeof sttState !== 'undefined' ? sttState : {};
  const base = String(state.sttBaseURL || '').trim().replace(/\/+$/, '');
  return base ? base + '/v1/audio/transcriptions' : '';
}

async function persistSTTArtifacts() {
  const logText = buildSTTCaptureLogText();
  await persistSTTLogToServer(logText);
  if (sttState.capturePCM.length > 0) {
    const wav = pcm16ToWav(new Int16Array(sttState.capturePCM));
    await persistSTTWavToServer(wav);
    await runSTTAutoTest();
  }
}

function updateSTTInputIndicators() {
  const voiceAllowed = isVoiceChatAllowed();
  const mobileControlAllowed = voiceAllowed || isMobileControlViewport();
  if (micBtn) {
    micBtn.classList.toggle('ready', !!sttState.isRecording);
    micBtn.classList.toggle('has-level', sttState.isRecording && sttState.inputLevel > 0);
    micBtn.style.setProperty('--mic-level-pct', `${Math.round(Math.max(0, Math.min(100, sttState.inputLevel)))}%`);
    micBtn.disabled = !mobileControlAllowed && !sttState.isRecording;
    micBtn.title = voiceAllowed
      ? (sttState.isRecording ? `音声入力中（入力レベル ${Math.round(sttState.inputLevel)}%・クリックで停止）` : '音声入力')
      : (mobileControlAllowed ? 'Chatに切り替えて音声入力' : '音声入力は通常チャットでのみ有効です');
  }
  if (micStateEl) {
    micStateEl.textContent = sttState.isRecording ? 'Mic: on' : 'Mic: off';
    micStateEl.className = 'stt-state' + (sttState.isRecording ? ' mic-on' : '');
  }
  if (sttConnStateEl) {
    let text = 'STT: off';
    let cls = 'stt-state conn-off';
    const ws = sttState.ws;
    if (sttState.isRecording) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        text = 'STT: connected';
        cls = 'stt-state conn-on';
      } else if (sttState.reconnecting || (ws && ws.readyState === WebSocket.CONNECTING)) {
        text = 'STT: reconnecting';
        cls = 'stt-state conn-reconnect';
      } else {
        text = 'STT: waiting';
        cls = 'stt-state conn-off';
      }
    } else if (ws && ws.readyState === WebSocket.OPEN) {
      text = 'STT: standby';
      cls = 'stt-state conn-on';
    } else if (sttState.reconnecting || (ws && ws.readyState === WebSocket.CONNECTING)) {
      text = 'STT: connecting';
      cls = 'stt-state conn-reconnect';
    }
    sttConnStateEl.textContent = text;
    sttConnStateEl.className = cls;
  }
  if (sttSessionStateEl) {
    const sid = String(sttState.captureSessionID || '(unknown)').trim() || '(unknown)';
    const actionError = String(sttState.captureActionError || '').trim();
    const suffix = actionError ? ' / ' + actionError : '';
    sttSessionStateEl.textContent = 'Session: ' + sid + suffix;
    sttSessionStateEl.title = 'Session: ' + sid + suffix;
    if (debugSttSessionEl) {
      debugSttSessionEl.textContent = 'Session: ' + sid + suffix;
    }
  }
}

function updateSTTCaption() {
  if (!sttCaptionEl) return;
  const finalText = String(sttState.finalCaptionText || '').trim();
  const partialText = String(sttState.partialCaptionText || '').trim();
  const errorText = String(sttState.errorCaptionText || '').trim();
  const setCaption = (label, text, cls) => {
    if (sttCaptionLabelEl) sttCaptionLabelEl.textContent = label || '暫定文字列';
    if (sttCaptionTextEl) sttCaptionTextEl.textContent = text || '-';
    sttCaptionEl.title = [label, text].filter(Boolean).join(': ');
    sttCaptionEl.className = cls || 'stt-caption';
  };
  if (errorText) {
    setCaption('STT error', errorText, 'stt-caption has-text error');
    return;
  }
  if (finalText) {
    setCaption('確定文字列', finalText, 'stt-caption has-text final');
    return;
  }
  if (partialText) {
    setCaption('暫定文字列', partialText, 'stt-caption has-text draft');
    return;
  }
  setCaption('暫定文字列', '', 'stt-caption');
}

function setSTTCaptionError(text) {
  sttState.errorCaptionText = String(text || 'unknown error').trim() || 'unknown error';
  sttState.partialCaptionText = '';
  sttState.finalCaptionText = '';
  updateSTTCaption();
}

async function toggleSTT() {
  if (sttState.isRecording) {
    stopSTT();
  } else {
    if (!ensureVoiceChatForMobileControl()) {
      showToast('音声入力は通常チャットでのみ有効です', 'error');
      return;
    }
    await startSTT();
  }
}

async function startSTT() {
  if (!ensureVoiceChatForMobileControl()) {
    showToast('音声入力は通常チャットでのみ有効です', 'error');
    return;
  }
  try {
    sttState.isStopping = false;
    sttState.captureLog = [];
    sttState.capturePCM = [];
    sttState.captureStartedAt = '';
    sttState.captureEndedAt = '';
    sttState.captureEventID = '';
    sttState.captureActionError = '';
    sttState.sentAudioSamples = 0;
    sttState.sentAudioBytes = 0;
    sttState.sentAudioFrames = 0;
    sttState.lastLoggedAudioSecond = 0;
    sttState.lastRecognitionText = '';
    sttState.lastRecognitionType = '';
    sttState.partialCaptionText = '';
    sttState.finalCaptionText = '';
    sttState.errorCaptionText = '';
    sttState.stopControlSent = false;
    sttState.finalReceived = false;
    clearSTTFinalWaitTimer();
    updateSTTCaption();
    updateSTTInputLevel(0);
    sttState.streamReady = false;
    if (!sttState.runtimeConfigLoaded) {
      await loadViewerRuntimeConfig();
    }
    sttState.audioStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        noiseSuppression: true,
        echoCancellation: true,
        autoGainControl: true
      }
    });
    sttState.audioContext = new (window.AudioContext || window.webkitAudioContext)();
    sttState.inputSampleRate = Math.round(sttState.audioContext.sampleRate || 48000);
    sttState.sampleRate = 16000;
    const source = sttState.audioContext.createMediaStreamSource(sttState.audioStream);

    // ScriptProcessor is enough here because we only need mono PCM16 chunks for STT.
    sttState.scriptNode = sttState.audioContext.createScriptProcessor(4096, 1, 1);
    source.connect(sttState.scriptNode);
    sttState.scriptNode.connect(sttState.audioContext.destination);

    sttState.scriptNode.onaudioprocess = (e) => {
      if (!sttState.isRecording) return;
      const pcm = e.inputBuffer.getChannelData(0);
      const pcm16 = resampleToPCM16(pcm, sttState.inputSampleRate || 48000, 16000);
      updateSTTInputLevel(calculateSTTInputLevel(pcm16));
      sttState.draftBuffer.push(...pcm16);
      sttState.capturePCM.push(...pcm16);
      sendSTTAudioChunk(pcm16);
      const maxDraftSamples = sttState.sampleRate;
      if (sttState.draftBuffer.length > maxDraftSamples) {
        sttState.draftBuffer = sttState.draftBuffer.slice(-maxDraftSamples);
      }
    };

    sttState.isRecording = true;
    updateSTTInputIndicators();
    connectSTTWebSocket();
  } catch (err) {
    sttState.captureActionError = describeSTTActionError('STT microphone start unavailable', err);
    updateSTTInputIndicators();
    console.error('[STT] Failed:', err);
    showToast('マイクアクセス拒否', 'error');
    stopSTT();
  }
}

function connectSTTWebSocket() {
  if (sttState.isStopping) return;
  if (!sttState.keepSessionChannel && !sttState.isRecording) return;
  if (sttState.ws && sttState.ws.readyState === WebSocket.OPEN) return;
  if (sttState.ws && sttState.ws.readyState === WebSocket.CONNECTING) return;
  sttState.ws = new WebSocket(sttState.voiceBridgeURL);
  sttState.ws.binaryType = 'arraybuffer';
  sttState.ws.onopen = () => {
    sttState.reconnecting = false;
    recordSTTCaptureEvent('ws_open', '');
    sendSTTStartControl();
    console.log('[STT] Connected - streaming PCM16 16kHz chunks');
    updateSTTInputIndicators();
  };
  sttState.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        const inp = document.getElementById('inp');
        if (msg.type) {
          const eventText = extractSTTMessageText(msg);
          pushDebugTrace('stt', {
            time: ftime(new Date().toISOString()),
            step: msg.type,
            text: short(eventText, 240),
          });
          if (msg.event_id && !sttState.captureEventID) {
            sttState.captureEventID = String(msg.event_id).trim();
          }
          if (msg.type === 'session_info' && msg.session_id) {
            sttState.captureSessionID = String(msg.session_id).trim() || '(unknown)';
            updateSTTInputIndicators();
          } else if (msg.session_id && sttState.captureSessionID === '(unknown)') {
            sttState.captureSessionID = String(msg.session_id).trim() || '(unknown)';
            updateSTTInputIndicators();
          } else if (msg.type === 'ready') {
            sttState.streamReady = true;
            if (msg.sample_rate) sttState.sampleRate = Number(msg.sample_rate) || sttState.sampleRate;
            updateSTTInputIndicators();
          } else if (msg.type === 'transcribing') {
            recordSTTCaptureEvent('progress', 'transcribing');
          } else if (msg.type === 'progress') {
            recordSTTCaptureEvent('progress', `${msg.duration || 0}s / ${msg.bytes || 0} bytes`);
          }
          if (msg.type !== 'progress') {
            recordSTTCaptureEvent(msg.type, formatSTTServerEventPayload(msg, eventText));
          }
          renderSTTDebugPanelsSafely();
        }
        if ((msg.type === 'draft' || msg.type === 'partial') && extractSTTMessageText(msg)) {
          const draftText = extractSTTMessageText(msg);
          sttState.lastRecognitionText = String(draftText || '').trim();
          sttState.lastRecognitionType = msg.type;
          sttState.partialCaptionText = sttState.lastRecognitionText;
          sttState.finalCaptionText = '';
          updateSTTCaption();
          console.log('[STT] Draft:', draftText);
        } else if (msg.type === 'final') {
          sttState.lastRecognitionText = String(msg.text || '').trim();
          sttState.lastRecognitionType = 'final';
          sttState.finalReceived = true;
          clearSTTFinalWaitTimer();
          sttState.finalCaptionText = sttState.lastRecognitionText;
          sttState.partialCaptionText = '';
          sttState.errorCaptionText = '';
          updateSTTCaption();
          console.log('[STT] Final:', msg.text);
          handleSTTFinalText(sttState.lastRecognitionText);
          // Clear buffer for next utterance (server-side VAD detected end)
          sttState.draftBuffer = [];
          if (sttState.isStopping && sttState.ws && sttState.ws.readyState === WebSocket.OPEN) {
            sttState.ws.close();
          }
        } else if (msg.type === 'reply_reset') {
          console.log('[STT] LLM reply starting...');
        } else if (msg.type === 'reply_delta' && msg.text) {
          console.log('[STT] LLM reply:', msg.text);
        } else if (msg.type === 'closed') {
          if (sttState.isStopping && sttState.ws && sttState.ws.readyState === WebSocket.OPEN) {
            sttState.ws.close();
          }
        } else if (msg.type === 'empty') {
          console.log('[STT] Empty result');
        } else if (msg.type === 'error') {
          const sttErrorText = extractSTTMessageText(msg) || 'unknown error';
          if (sttState.finalReceived) {
            recordSTTCaptureEvent('error', 'ignored after final: ' + sttErrorText);
            console.warn('[STT] Error ignored after final:', msg.error || msg.message);
            updateSTTInputIndicators();
            return;
          }
          sttState.captureActionError = describeSTTActionError('STT recognition unavailable', sttErrorText);
          setSTTCaptionError(sttErrorText);
          updateSTTInputIndicators();
          console.error('[STT] Error:', msg.error || msg.message);
          showToast('認識エラー', 'error');
        }
      } catch (err) {
        sttState.captureActionError = describeSTTActionError('STT message parse unavailable', err);
        setSTTCaptionError(sttState.captureActionError);
        updateSTTInputIndicators();
        console.error('[STT] Parse error:', err);
      }
  };
  sttState.ws.onerror = (event) => {
    recordSTTCaptureEvent('ws_error', event && event.message ? event.message : 'connection error');
    sttState.captureActionError = describeSTTActionError(
      'STT websocket unavailable',
      event && event.message ? event : 'connection error',
    );
    setSTTCaptionError(sttState.captureActionError);
    updateSTTInputIndicators();
    if (!sttState.isStopping && sttState.keepSessionChannel) scheduleSTTReconnect();
  };
  sttState.ws.onclose = () => {
    recordSTTCaptureEvent('ws_close', '');
    sttState.streamReady = false;
    updateSTTInputIndicators();
    if (sttState.isStopping) {
      completeSTTStop();
      return;
    }
    if (!sttState.isStopping && sttState.keepSessionChannel) scheduleSTTReconnect();
  };
}

function extractSTTMessageText(msg) {
  if (!msg) return '';
  if (msg.text) return String(msg.text);
  if (msg.message) return String(msg.message);
  if (typeof msg.error === 'string') return msg.error;
  if (msg.error && msg.error.message) return String(msg.error.message);
  if (msg.error_code) return String(msg.error_code);
  return '';
}

function resampleToPCM16(input, fromRate, toRate) {
  const sourceRate = Math.max(1, Number(fromRate) || 48000);
  const targetRate = Math.max(1, Number(toRate) || 16000);
  const outputLength = Math.max(1, Math.floor(input.length * targetRate / sourceRate));
  const output = new Int16Array(outputLength);
  const ratio = sourceRate / targetRate;
  for (let i = 0; i < outputLength; i++) {
    const pos = i * ratio;
    const left = Math.floor(pos);
    const right = Math.min(input.length - 1, left + 1);
    const frac = pos - left;
    const sample = (input[left] || 0) * (1 - frac) + (input[right] || 0) * frac;
    output[i] = Math.max(-32768, Math.min(32767, Math.round(sample * 32767)));
  }
  return output;
}

function calculateSTTInputLevel(pcm16) {
  if (!pcm16 || pcm16.length === 0) return 0;
  let sumSquares = 0;
  for (let i = 0; i < pcm16.length; i++) {
    const sample = Number(pcm16[i]) || 0;
    sumSquares += sample * sample;
  }
  const rms = Math.sqrt(sumSquares / pcm16.length);
  return Math.max(0, Math.min(100, Math.round((rms / 2400) * 100)));
}

function updateSTTInputLevel(level) {
  sttState.inputLevel = Math.max(0, Math.min(100, Number(level) || 0));
  if (!micBtn) return;
  micBtn.style.setProperty('--mic-level-pct', `${Math.round(sttState.inputLevel)}%`);
  micBtn.classList.toggle('has-level', sttState.isRecording && sttState.inputLevel > 0);
}

function sendSTTAudioChunk(pcm16) {
  if (!sttState.isRecording || !sttState.ws || sttState.ws.readyState !== WebSocket.OPEN) return;
  sttState.chunkBuffer.push(...pcm16);
  while (sttState.chunkBuffer.length >= sttState.chunkSamples) {
    const chunk = new Int16Array(sttState.chunkBuffer.slice(0, sttState.chunkSamples));
    sttState.chunkBuffer = sttState.chunkBuffer.slice(sttState.chunkSamples);
    sttState.ws.send(chunk.buffer);
    recordSTTAudioSent(chunk.length);
  }
}

function flushSTTAudioChunkBuffer() {
  if (!sttState.ws || sttState.ws.readyState !== WebSocket.OPEN || sttState.chunkBuffer.length === 0) return false;
  const chunk = new Int16Array(sttState.chunkBuffer);
  sttState.chunkBuffer = [];
  sttState.ws.send(chunk.buffer);
  recordSTTAudioSent(chunk.length);
  return true;
}

function recordSTTAudioSent(samples) {
  const count = Math.max(0, Number(samples) || 0);
  if (count <= 0) return;
  sttState.sentAudioSamples += count;
  sttState.sentAudioBytes += count * 2;
  sttState.sentAudioFrames += 1;
  const sampleRate = Number(sttState.sampleRate || 16000) || 16000;
  const seconds = sampleRate > 0 ? sttState.sentAudioSamples / sampleRate : 0;
  const wholeSecond = Math.floor(seconds);
  if (wholeSecond > sttState.lastLoggedAudioSecond) {
    sttState.lastLoggedAudioSecond = wholeSecond;
    recordSTTCaptureEvent('audio_sent', `${seconds.toFixed(3)}s / ${sttState.sentAudioBytes} bytes / frame ${sttState.sentAudioFrames}`);
  }
}

function sendSTTStopTailSilence() {
  if (!sttState.ws || sttState.ws.readyState !== WebSocket.OPEN) return false;
  const sampleRate = Number(sttState.sampleRate || 16000) || 16000;
  const totalSamples = Math.max(0, Math.round(sampleRate * STT_STOP_TAIL_SILENCE_MS / 1000));
  if (totalSamples <= 0) return false;
  const chunkSamples = Math.max(1, Number(sttState.chunkSamples || 1600) || 1600);
  for (let offset = 0; offset < totalSamples; offset += chunkSamples) {
    const size = Math.min(chunkSamples, totalSamples - offset);
    sttState.ws.send(new Int16Array(size).buffer);
  }
  recordSTTCaptureEvent('progress', `stop tail silence ${STT_STOP_TAIL_SILENCE_MS}ms`);
  return true;
}

function sendSTTStartControl() {
  if (!sttState.ws || sttState.ws.readyState !== WebSocket.OPEN) return false;
  const sampleRate = Number(sttState.sampleRate || 16000) || 16000;
  const control = {
    type: 'start',
    sample_rate: sampleRate,
    channels: 1,
    format: 'pcm_s16le',
  };
  sttState.ws.send(JSON.stringify(control));
  recordSTTCaptureEvent('start', `${sampleRate}Hz pcm_s16le mono`);
  return true;
}

function sendSTTStopControl() {
  if (!sttState.ws || sttState.ws.readyState !== WebSocket.OPEN) return false;
  if (sttState.stopControlSent) return true;
  sttState.ws.send(JSON.stringify({ type: 'stop' }));
  sttState.stopControlSent = true;
  recordSTTCaptureEvent('stop', 'requested');
  return true;
}

function clearSTTFinalWaitTimer() {
  if (!sttState.finalWaitTimer) return;
  clearTimeout(sttState.finalWaitTimer);
  sttState.finalWaitTimer = null;
}

function scheduleSTTFinalWaitTimeout() {
  clearSTTFinalWaitTimer();
  sttState.finalWaitTimer = setTimeout(() => {
    sttState.finalWaitTimer = null;
    if (!sttState.isStopping) return;
    sttState.captureActionError = describeSTTActionError('STT final unavailable', 'timed out waiting for final');
    setSTTCaptionError(sttState.captureActionError);
    recordSTTCaptureEvent('error', 'timed out waiting for final');
    updateSTTInputIndicators();
    if (sttState.ws && (sttState.ws.readyState === WebSocket.OPEN || sttState.ws.readyState === WebSocket.CONNECTING)) {
      sttState.ws.close();
      return;
    }
    completeSTTStop();
  }, STT_FINAL_WAIT_TIMEOUT_MS);
}

function handleSTTFinalText(text) {
  const finalText = String(text || '').trim();
  if (!finalText) return;
  if (!isVoiceChatAllowed()) {
    console.warn('[STT] Final ignored outside normal chat:', finalText);
    return;
  }
  const inp = document.getElementById('inp');
  if (inp) {
    inp.value = finalText;
    autoResize();
    inp.focus();
  }
  if (!sending) {
    send();
  }
  if (sttState.ws && sttState.ws.readyState === WebSocket.OPEN && !sttState.isRecording) {
    sttState.ws.close();
  }
}

function scheduleSTTReconnect() {
  if (sttState.reconnecting) return;
  sttState.reconnecting = true;
  updateSTTInputIndicators();
  if (sttState.reconnectTimer) clearTimeout(sttState.reconnectTimer);
  sttState.reconnectTimer = setTimeout(() => {
    sttState.reconnectTimer = null;
    if (sttState.isStopping || !sttState.keepSessionChannel) {
      sttState.reconnecting = false;
      updateSTTInputIndicators();
      return;
    }
    connectSTTWebSocket();
  }, 700);
}

function pcm16ToWav(pcmBuffer, sampleRate = sttState.sampleRate || 48000) {
  const numChannels = 1;
  const bitsPerSample = 16;
  const byteRate = sampleRate * numChannels * bitsPerSample / 8;
  const blockAlign = numChannels * bitsPerSample / 8;
  const dataSize = pcmBuffer.length * 2;
  const header = new ArrayBuffer(44);
  const view = new DataView(header);
  // RIFF
  view.setUint32(0, 0x52494646, false); // "RIFF"
  view.setUint32(4, 36 + dataSize, true);
  view.setUint32(8, 0x57415645, false); // "WAVE"
  // fmt
  view.setUint32(12, 0x666d7420, false); // "fmt "
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, numChannels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, byteRate, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, bitsPerSample, true);
  // data
  view.setUint32(36, 0x64617461, false); // "data"
  view.setUint32(40, dataSize, true);

  const wavBuffer = new ArrayBuffer(44 + dataSize);
  new Uint8Array(wavBuffer).set(new Uint8Array(header), 0);
  new Int16Array(wavBuffer, 44).set(pcmBuffer, 0);
  return wavBuffer;
}

function sendDraft() {
  // Deprecated: realtime STT now streams PCM16 binary chunks directly.
}

function stopSTT() {
  if (sttState.isStopping) return;
  sttState.isStopping = true;
  console.log('[STT] Stopping');
  sttState.isRecording = false;
  updateSTTInputLevel(0);

  if (sttState.draftTimer) sttState.draftTimer();
  if (sttState.reconnectTimer) {
    clearTimeout(sttState.reconnectTimer);
    sttState.reconnectTimer = null;
  }
  sttState.reconnecting = false;

  if (sttState.scriptNode) {
    sttState.scriptNode.disconnect();
    sttState.scriptNode = null;
  }
  if (sttState.audioContext) {
    sttState.audioContext.close();
    sttState.audioContext = null;
  }
  if (sttState.audioStream) {
    sttState.audioStream.getTracks().forEach(t => t.stop());
    sttState.audioStream = null;
  }
  if (sttState.finalReceived) {
    clearSTTFinalWaitTimer();
    sttState.chunkBuffer = [];
    if (sttState.ws && sttState.ws.readyState === WebSocket.OPEN) {
      sttState.ws.close();
      updateSTTInputIndicators();
      return;
    }
  }
  if (sttState.ws && sttState.ws.readyState === WebSocket.OPEN) {
    flushSTTAudioChunkBuffer();
    sendSTTStopTailSilence();
    sendSTTStopControl();
    scheduleSTTFinalWaitTimeout();
    updateSTTInputIndicators();
    return;
  }
  sttState.chunkBuffer = [];
  if (sttState.ws && sttState.ws.readyState === WebSocket.CONNECTING) {
    sttState.ws.close();
    scheduleSTTFinalWaitTimeout();
    updateSTTInputIndicators();
    return;
  }

  completeSTTStop();
}

function completeSTTStop() {
  if (!sttState.isStopping) return;
  clearSTTFinalWaitTimer();
  sttState.chunkBuffer = [];
  sttState.draftBuffer = [];
  sttState.stopControlSent = false;
  sttState.isStopping = false;
  updateSTTInputIndicators();
  persistSTTArtifacts().then(() => {
    showToast('STTログ/WAVを tmp に保存しました', 'success');
  }).catch((err) => {
    sttState.captureActionError = describeSTTActionError('STT artifact persistence unavailable', err);
    updateSTTInputIndicators();
    console.error('[STT] persist failed:', err);
    showToast('STT保存または自動テストに失敗', 'error');
  });
  console.log('[STT] Stopped');
}
