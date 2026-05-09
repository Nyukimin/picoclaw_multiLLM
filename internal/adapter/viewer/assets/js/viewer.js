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
  sessions: {},
  jobs: {},
  evidence: [],
  evidenceSummary: {status: {}, error_kind: {}},
  evidenceOrder: [],
  selectedEvidenceJobID: '',
  selectedEvidenceItem: null,
  selectedEvidenceFocus: '',
  evidenceSortDesc: true,
  pendingEvidenceJobID: '',
  memory: {
    snapshot: {memory: [], news: [], digests: [], knowledge: []},
    layers: {l0: [], l1: [], l2: [], l3: []},
    events: [],
    searchCache: [],
    sourceRegistry: [],
    traces: [],
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
    lastMioReport: null,
    latestJobID: '',
    latestRoute: '',
    latestError: null,
    llmOpsEnabled: false,
    llmStatus: null,
    llmStatusError: '',
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
  currentCharacterId: '',
  currentText: '',
  currentDisplayText: '',
  currentSessionId: '',
  currentChunkIndex: -1,
  currentUtteranceId: '',
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
  bubbleKind: '',
  active: false,
  chunkKeys: new Set(),
};
const idleTTSSpeech = {
  el: null,
  textEl: null,
  characterId: '',
  sessionId: '',
  bubbleKind: '',
  active: false,
  chunkKeys: new Set(),
};

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

function isIdleChatSessionId(sessionId) {
  return String(sessionId || '').trim().indexOf('idle-') === 0;
}

function setCentralTTSSpeechText(characterId, text, sessionId, chunkIndex, utteranceId) {
  const target = isIdleChatSessionId(sessionId) ? 'idle' : 'central';
  setTTSSpeechText(target, characterId, text, sessionId, chunkIndex, utteranceId);
}

function setTTSSpeechText(target, characterId, text, sessionId, chunkIndex, utteranceId) {
  const normalizedText = String(text || '').trim();
  const speech = target === 'idle' ? idleTTSSpeech : centralTTSSpeech;
  const container = target === 'idle' ? idleLiveLog : chat;
  if (!normalizedText) {
    resetTTSSpeechBubble(speech);
    return;
  }
  if (!container) return;

  const id = String(characterId || '').trim().toLowerCase();
  const sid = String(sessionId || '').trim();
  const normalizedChunkIndex = Number.isFinite(chunkIndex) ? chunkIndex : -1;
  const bubbleKind = ttsBubbleKind(speech, normalizedText, sid, normalizedChunkIndex);
  const f = ag(id || 'mio');
  const key = String(utteranceId || '') || (sid + ':' + String(normalizedChunkIndex >= 0 ? normalizedChunkIndex : speech.chunkKeys.size));
  if (!speech.el || speech.characterId !== id || speech.bubbleKind !== bubbleKind || shouldStartNewTTSBubble(speech, normalizedChunkIndex, key)) {
    if (speech.el) speech.el.classList.remove('tts-current');
    const el = document.createElement('div');
    const idleClass = target === 'idle' ? ' idle-live-item idle-kind-tts idle-kind-' + bubbleKind : '';
    const kindLabel = target === 'idle' ? '<span class="idle-kind">' + (bubbleKind === 'topic' ? 'Topic' : 'Speech') + '</span>' : '';
    el.className = 'msg tts-current' + (id === 'shiro' ? ' shiro' : '') + idleClass;
    el.innerHTML =
      '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
      '<div class="mb"><div class="mh">' +
        kindLabel +
        '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
        '<span class="tm">' + ftime(new Date().toISOString()) + '</span>' +
      '</div><div class="mc"></div></div>';
    speech.el = el;
    speech.textEl = el.querySelector('.mc');
    speech.characterId = id;
    speech.sessionId = sid;
    speech.bubbleKind = bubbleKind;
    speech.active = true;
    speech.chunkKeys = new Set();
    if (target === 'central') {
      const em = document.getElementById('empty');
      if (em) em.remove();
    } else {
      removeIdleLiveEmpty();
    }
    container.appendChild(el);
    trimTimelineNodesFor(container, MAX_TIMELINE_NODES);
  } else {
    speech.el.classList.add('tts-current');
    speech.el.classList.toggle('shiro', id === 'shiro');
    speech.sessionId = sid;
    speech.active = true;
  }
  if (speech.chunkKeys.has(key)) {
    return;
  }
  speech.chunkKeys.add(key);
  if (speech.textEl) {
    const current = String(speech.textEl.textContent || '');
    speech.textEl.textContent = appendCentralTTSText(current, normalizedText);
    speech.textEl.dataset.raw = speech.textEl.textContent;
  }
  if (target === 'central') scrollToBottom();
  else idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
}

function resetCentralTTSSpeechBubble() {
  resetTTSSpeechBubble(centralTTSSpeech);
  resetTTSSpeechBubble(idleTTSSpeech);
}

function resetTTSSpeechBubble(speech) {
  if (speech.el) speech.el.classList.remove('tts-current');
  speech.active = false;
}

function shouldStartNewTTSBubble(speech, chunkIndex, key) {
  if (!speech.el) return true;
  if (!speech.textEl || !String(speech.textEl.textContent || '').trim()) return false;
  if (speech.chunkKeys.has(key)) return false;
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

function ttsBubbleKind(speech, text, sessionId, chunkIndex) {
  const s = String(text || '').trim();
  if (/^今日のお題です[、。！？!?]?/.test(s)) return 'topic';
  if (chunkIndex > 0 && speech.bubbleKind === 'topic' && speech.sessionId === String(sessionId || '').trim()) {
    return 'topic';
  }
  return 'speech';
}

let timelineAutoFollow = true;
let timelineUserInteracting = false;
let timelineInteractionTimer = null;
let suppressTimelineScroll = false;
let derivedDirty = false;
let activeViewerTab = 'timeline';
let sttControlsReady = false;

const tabs = Array.from(document.querySelectorAll('.tab-btn'));
const panels = {
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
  activeViewerTab = tab;
  tabs.forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
  Object.keys(panels).forEach((k) => panels[k].classList.toggle('active', k === tab));
  updateLatestButton();
  if (sttControlsReady) {
    if (tab === 'idlechat' && sttState.isRecording) stopSTT();
    updateSTTInputIndicators();
  }
  if (tab === 'timeline' && timelineAutoFollow) scrollToBottom(true);
}
tabs.forEach((btn) => btn.addEventListener('click', () => switchTab(btn.dataset.tab)));

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

function renderSystem() {
  const body = document.getElementById('systemBody');
  if (!body) return;
  body.innerHTML = '';
  const list = state.logs.filter(matchesSystemFilters).slice().reverse();
  if (list.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="8" class="small">No system events</td>';
    body.appendChild(tr);
    return;
  }
  list.forEach((ev) => {
    const raw = String(ev.content || '');
    const linePayload = JSON.stringify({
      timestamp: ev.timestamp || '',
      type: ev.type || '',
      from: ev.from || '',
      to: ev.to || '',
      route: ev.route || '',
      job_id: ev.job_id || '',
      content: raw,
    }, null, 2);
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(ev.timestamp)) + '</td>' +
      '<td>' + esc(ev.type || '-') + '</td>' +
      '<td>' + esc(agName(ev.from || '-')) + '</td>' +
      '<td>' + esc(agName(ev.to || '-')) + '</td>' +
      '<td>' + esc(ev.route || '-') + '</td>' +
      '<td class="code">' + esc(ev.job_id || '-') + '</td>' +
      '<td><div class="sys-content" data-raw="' + esc(raw) + '">' + esc(raw || '-') + '</div></td>' +
      '<td><div class="sys-actions">' +
        '<button class="ctl-btn" onclick="copyTextPayload(this, ' + escAttr(JSON.stringify(raw)) + ')">Text</button>' +
        '<button class="ctl-btn" onclick="copyTextPayload(this, ' + escAttr(JSON.stringify(linePayload)) + ')">Row</button>' +
      '</div></td>';
    body.appendChild(tr);
  });
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
if (newsPackRefreshBtn) newsPackRefreshBtn.addEventListener('click', refreshNewsPack);
if (newsPackCategory) newsPackCategory.addEventListener('keydown', (e) => { if (e.key === 'Enter') refreshNewsPack(); });

function setIdleState(mode, manual, active) {
  const currentMode = String(mode || '');
  if (currentMode === 'forecast') {
    idleStateEl.textContent = 'Forecast: ' + (active ? 'talking' : 'ready');
    idleStateEl.className = active ? 'idle-on' : 'idle-off';
    return;
  }
  if (currentMode === 'story') {
    idleStateEl.textContent = 'Story: ' + (active ? 'talking' : 'ready');
    idleStateEl.className = active ? 'idle-on' : 'idle-off';
    return;
  }
  if (currentMode === 'story-simple') {
    idleStateEl.textContent = 'Story(簡易): ' + (active ? 'talking' : 'ready');
    idleStateEl.className = active ? 'idle-on' : 'idle-off';
    return;
  }
  if (active) {
    idleStateEl.textContent = 'IdleChat: talking';
    idleStateEl.className = 'idle-on';
    return;
  }
  if (manual) {
    idleStateEl.textContent = 'IdleChat: on';
    idleStateEl.className = 'idle-on';
    return;
  }
  idleStateEl.textContent = 'IdleChat: off';
  idleStateEl.className = 'idle-off';
}

function setIdleSelectedMode(mode) {
  const next = mode === 'forecast'
    ? 'forecast'
    : (mode === 'story-simple' ? 'story-simple' : 'manual');
  state.idleChat.selectedMode = next;
  localStorage.setItem('idlechat.selectedMode', next);
  if (idleModeNormalBtn) idleModeNormalBtn.classList.toggle('is-selected', next === 'manual');
  if (idleModeForecastBtn) idleModeForecastBtn.classList.toggle('is-selected', next === 'forecast');
  if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.classList.toggle('is-selected', next === 'story-simple');
}

function setBadge(el, enabled) {
  el.textContent = enabled ? 'on' : 'off';
  el.className = 'badge ' + (enabled ? 'state-running' : 'state-offline');
}

function stripIdleTopicCategory(text) {
  return String(text || '').replace(/^今日のお題(?:（[^）]+）)*[:：]\s*/, '今日のお題：').trim();
}

function normalizeViewerDisplayText(text) {
  return stripIdleTopicCategory(text);
}

function setIdleSelectedView(view) {
  const next = (view === 'summary' || view === 'history') ? view : 'live';
  state.idleChat.selectedView = next;
  localStorage.setItem('idlechat.selectedView', next);
  idleSubtabs.forEach((btn) => {
    const active = btn.dataset.idleView === next;
    btn.classList.toggle('active', active);
    btn.setAttribute('aria-selected', active ? 'true' : 'false');
    btn.tabIndex = active ? 0 : -1;
  });
  idleSubviews.forEach((viewEl) => {
    const expectedID = 'idleView' + next.charAt(0).toUpperCase() + next.slice(1);
    viewEl.classList.toggle('active', viewEl.id === expectedID);
  });
}

function renderIdleChat() {
  const manualEl = document.getElementById('idleManual');
  const activeEl = document.getElementById('idleActive');
  const topicEl = document.getElementById('idleTopicNow');
  const body = document.getElementById('idlechatBody');
  if (!manualEl || !activeEl || !topicEl || !body) return;

  setBadge(manualEl, state.idleChat.manualMode);
  setBadge(activeEl, state.idleChat.chatActive);
  topicEl.textContent = stripIdleTopicCategory(state.idleChat.currentTopic) || '-';

  body.innerHTML = '';
  const rows = state.idleChat.history || [];
  renderIdleSummaryReview(rows);
  if (rows.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="7" class="small">No idleChat summaries yet</td>';
    body.appendChild(tr);
    return;
  }

  if (state.idleChat.openIndex >= rows.length) state.idleChat.openIndex = -1;

  rows.forEach((r, rowIndex) => {
    const tr = document.createElement('tr');
    const isOpen = state.idleChat.openIndex === rowIndex;
    const strategy = r.strategy || r.category || '-';
    const isForecast = strategy === 'forecast';
    if (isForecast) tr.style.borderLeft = '3px solid rgba(59,130,246,.5)';
    tr.innerHTML =
      '<td><button class="ctl-btn idle-open idle-title-btn" data-idx="' + esc(String(rowIndex)) + '"><span class="idle-arrow ' + (isOpen ? 'open' : '') + '">&#9654;</span><span>' + esc(stripIdleTopicCategory(r.title || '-') || '-') + '</span></button></td>' +
      '<td>' + esc(stripIdleTopicCategory(r.topic || '-') || '-') + '</td>' +
      '<td>' + esc(String(r.turns || 0)) + '</td>' +
      '<td>' + esc(r.loop_restarted ? 'yes' : 'no') + '</td>' +
      '<td>' + esc(fdt(r.started_at)) + '</td>' +
      '<td>' + esc(fdt(r.ended_at)) + '</td>' +
      '<td>' + esc(short(r.summary || '-', 200)) + '</td>';
    body.appendChild(tr);

    if (isOpen) {
      const exp = document.createElement('tr');
      exp.className = 'idle-expand';
      const transcript = Array.isArray(r.transcript) ? r.transcript : [];
      if (transcript.length === 0) {
        exp.innerHTML = '<td colspan="7"><div class="idle-actions"><button class="ctl-btn idle-copy-chat" data-idx="' + esc(String(rowIndex)) + '">Copy Chat</button></div><div class="idle-empty">Transcript not available</div></td>';
      } else {
        const items = transcript.map((line) => {
          const idx = String(line || '').indexOf(':');
          let speaker = 'agent';
          let content = String(line || '');
          if (idx > 0) {
            speaker = content.slice(0, idx).trim();
            content = content.slice(idx + 1).trim();
          }
          const info = ag(speaker);
          return '<div class="idle-bubble"><div class="idle-meta" style="color:' + info.c + '">' + info.e + ' ' + info.l + '</div><div>' + esc(content || '-') + '</div></div>';
        }).join('');
        exp.innerHTML = '<td colspan="7"><div class="idle-actions"><button class="ctl-btn idle-copy-chat" data-idx="' + esc(String(rowIndex)) + '">Copy Chat</button></div><div class="idle-transcript">' + items + '</div></td>';
      }
      body.appendChild(exp);
    }
  });

  body.querySelectorAll('.idle-open').forEach((btn) => {
    btn.addEventListener('click', () => {
      const next = Number(btn.dataset.idx || '-1');
      state.idleChat.openIndex = (state.idleChat.openIndex === next) ? -1 : next;
      renderIdleChat();
    });
  });
  body.querySelectorAll('.idle-copy-chat').forEach((btn) => {
    btn.addEventListener('click', () => {
      const idx = Number(btn.dataset.idx || '-1');
      const row = rows[idx];
      if (!row) {
        showToast('Copy failed', 'error');
        return;
      }
      copyTextPayload(btn, formatIdleChatTranscript(row));
    });
  });
}

function renderIdleSummaryReview(rows) {
  const listEl = document.getElementById('idleSummaryList');
  const detailEl = document.getElementById('idleSummaryDetail');
  if (!listEl || !detailEl) return;

  listEl.innerHTML = '';
  detailEl.innerHTML = '';
  if (!Array.isArray(rows) || rows.length === 0) {
    state.idleChat.selectedSummaryIndex = 0;
    listEl.innerHTML = '<div class="idle-empty">No summaries yet</div>';
    detailEl.innerHTML = '<div class="idle-empty">Select a summary</div>';
    return;
  }

  if (state.idleChat.selectedSummaryIndex < 0 || state.idleChat.selectedSummaryIndex >= rows.length) {
    state.idleChat.selectedSummaryIndex = 0;
  }
  const selectedIndex = state.idleChat.selectedSummaryIndex;
  rows.forEach((r, idx) => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'idle-review-item' + (idx === selectedIndex ? ' active' : '');
    btn.dataset.idx = String(idx);
    btn.innerHTML =
      '<div class="idle-review-title">' + esc(stripIdleTopicCategory(r.title || r.topic || '-') || '-') + '</div>' +
      '<div class="idle-review-meta">' + esc(fdt(r.ended_at || r.started_at)) + ' · turns ' + esc(String(r.turns || 0)) + '</div>';
    btn.addEventListener('click', () => {
      state.idleChat.selectedSummaryIndex = idx;
      renderIdleSummaryReview(rows);
    });
    listEl.appendChild(btn);
  });

  const row = rows[selectedIndex] || {};
  const sections = [
    {title: 'Summary', text: row.summary || '-'},
    {title: 'Quality Review', text: row.quality_review || '-'},
    {title: 'Prompt Guidance', text: row.prompt_guidance || '-'},
    {title: 'Transcript', text: formatIdleTranscriptOnly(row)},
  ];
  detailEl.innerHTML =
    '<div class="idle-actions" style="padding:0;justify-content:flex-start">' +
      '<button class="ctl-btn" id="idleSummaryCopy">Copy Review</button>' +
    '</div>' +
    '<h4>' + esc(stripIdleTopicCategory(row.title || '-') || '-') + '</h4>' +
    '<div class="idle-review-kv">' +
      '<span>Topic: ' + esc(stripIdleTopicCategory(row.topic || '-') || '-') + '</span>' +
      '<span>Strategy: ' + esc(String(row.strategy || row.category || '-')) + '</span>' +
      '<span>Turns: ' + esc(String(row.turns || 0)) + '</span>' +
      '<span>Ended: ' + esc(fdt(row.ended_at)) + '</span>' +
      (row.loop_restarted ? '<span>Loop Restart: yes' + (row.loop_reason ? ' / ' + esc(row.loop_reason) : '') + '</span>' : '') +
    '</div>' +
    sections.map((s) => (
      '<div class="idle-review-section">' +
        '<h5>' + esc(s.title) + '</h5>' +
        '<div class="idle-review-text">' + fmt(s.text || '-') + '</div>' +
      '</div>'
    )).join('');

  const copyBtn = document.getElementById('idleSummaryCopy');
  if (copyBtn) {
    copyBtn.addEventListener('click', () => copyTextPayload(copyBtn, formatIdleChatTranscript(row)));
  }
}

function formatIdleTranscriptOnly(row) {
  const transcript = Array.isArray(row && row.transcript) ? row.transcript : [];
  if (transcript.length === 0) return '(not available)';
  return transcript.map((line) => String(line || '')).join('\n');
}

function formatIdleChatTranscript(row) {
  const lines = [];
  lines.push('Title: ' + String(row && row.title || '-'));
  lines.push('Topic: ' + String(row && row.topic || '-'));
  lines.push('Strategy: ' + String(row && (row.strategy || row.category) || '-'));
  lines.push('Turns: ' + String(row && row.turns || 0));
  lines.push('Started: ' + String(row && row.started_at || ''));
  lines.push('Ended: ' + String(row && row.ended_at || ''));
  lines.push('');
  lines.push('Summary:');
  lines.push(String(row && row.summary || '-'));
  if (row && row.quality_review) {
    lines.push('');
    lines.push('Quality Review:');
    lines.push(String(row.quality_review || '-'));
  }
  if (row && row.prompt_guidance) {
    lines.push('');
    lines.push('Prompt Guidance:');
    lines.push(String(row.prompt_guidance || '-'));
  }
  lines.push('');
  lines.push('Transcript:');
  const transcript = Array.isArray(row && row.transcript) ? row.transcript : [];
  if (transcript.length === 0) {
    lines.push('(not available)');
  } else {
    transcript.forEach((line) => lines.push(String(line || '')));
  }
  return lines.join('\n');
}

async function refreshIdleStatus() {
  try {
    const r = await fetch('/viewer/idlechat/status');
    if (!r.ok) {
      idleStartBtn.disabled = true;
      if (idleModeNormalBtn) idleModeNormalBtn.disabled = true;
      if (idleModeForecastBtn) idleModeForecastBtn.disabled = true;
      if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.disabled = true;
      idleStopBtn.disabled = true;
      setIdleState('', false, false);
      state.idleChat.mode = '';
      state.idleChat.manualMode = false;
      state.idleChat.chatActive = false;
      state.idleChat.currentTopic = '';
      renderIdleChat();
      return;
    }
    const d = await r.json();
    setIdleState(d.mode || '', !!d.manual_mode, !!d.chat_active);
    idleStartBtn.disabled = !!d.manual_mode || !!d.chat_active;
    if (idleModeNormalBtn) idleModeNormalBtn.disabled = !!d.chat_active;
    if (idleModeForecastBtn) idleModeForecastBtn.disabled = !!d.chat_active;
    if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.disabled = !!d.chat_active;
    idleStopBtn.disabled = !d.manual_mode && !d.chat_active;
    state.idleChat.mode = d.mode || '';
    state.idleChat.manualMode = !!d.manual_mode;
    state.idleChat.chatActive = !!d.chat_active;
    state.idleChat.currentTopic = d.current_topic || '';
    renderIdleChat();
  } catch (_) {
    idleStartBtn.disabled = true;
    if (idleModeNormalBtn) idleModeNormalBtn.disabled = true;
    if (idleModeForecastBtn) idleModeForecastBtn.disabled = true;
    if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.disabled = true;
    idleStopBtn.disabled = true;
    setIdleState('', false, false);
    state.idleChat.mode = '';
    state.idleChat.manualMode = false;
    state.idleChat.chatActive = false;
    state.idleChat.currentTopic = '';
    renderIdleChat();
  }
}

async function refreshIdleLogs() {
  try {
    const r = await fetch('/viewer/idlechat/logs?limit=20');
    if (!r.ok) return;
    const d = await r.json();
    state.idleChat.mode = d.mode || '';
    state.idleChat.manualMode = !!d.manual_mode;
    state.idleChat.chatActive = !!d.chat_active;
    state.idleChat.currentTopic = d.current_topic || '';
    state.idleChat.history = Array.isArray(d.history) ? d.history : [];
    renderIdleChat();
  } catch (_) {
  }
}

async function controlIdle(path) {
  const btns = [idleStartBtn, idleModeNormalBtn, idleModeForecastBtn, idleModeStorySimpleBtn, idleStopBtn].filter(Boolean);
  btns.forEach((b) => { b.disabled = true; });
  try {
    const r = await fetch(path, {method: 'POST'});
    if (!r.ok) throw new Error('idlechat control failed');
  } catch (err) {
    console.error(err);
  } finally {
    await refreshIdleStatus();
  }
}

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

function fmtMiB(value) {
  const n = num(value);
  if (n <= 0) return '-';
  return n.toFixed(n >= 1024 ? 0 : 1) + ' MiB';
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
      if (!r.ok) throw new Error('debug system fetch failed');
      return r.json();
    })
    .then((data) => {
      state.debug.gpu = data && data.gpu ? data.gpu : null;
      state.debug.audio = data && data.audio ? data.audio : null;
      renderDebugPanels();
    })
    .catch((err) => {
      console.error(err);
      state.debug.gpu = {available: false, note: 'fetch failed'};
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

function removeIdleLiveEmpty() {
  if (!idleLiveLog) return;
  const empty = idleLiveLog.querySelector('.idle-live-empty');
  if (empty) empty.remove();
}

function addMsgToTimeline(ev) {
  if (!matchesFilters(ev)) return;
  if (ev.type === 'idlechat.summary') return;
  if (ev.type === 'idlechat.message') return;
  if (ev.type !== 'message.received' && ev.type !== 'idlechat.message' && (ev.from || '').toLowerCase() !== 'mio') return;

  const em = document.getElementById('empty');
  if (em) em.remove();

  if (ev.type === 'routing.decision') return;
  if (ev.type === 'agent.start') { addThinkingStart(ev); return; }
  if (ev.type === 'agent.thinking') { addThinking(ev); return; }
  if (ev.type === 'agent.response') { removeThinking(ev.job_id); }
  if (ev.type === 'agent.response' && (ev.to || '').toLowerCase() !== 'user') return;
  if (ev.type === 'agent.response' && isTTSSyncedSpeaker(ev.from)) return;
  if (ev.type === 'idlechat.message' && isTTSSyncedSpeaker(ev.from)) return;
  if (ev.type === 'agent.note' && (ev.to || '').toLowerCase() !== 'user') return;
  if (ev.type === 'message.received' && (ev.from || '').toLowerCase() !== 'user') return;

  const f = ag(ev.from);
  const t = ev.to ? ag(ev.to) : null;
  const dir = t && ev.to ? '<span class="dir">→ ' + t.e + ' ' + t.l + '</span>' : '';
  const displayContent = normalizeViewerDisplayText(ev.content);
  const el = document.createElement('div');
  el.className = 'msg';
  el.innerHTML =
    '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
    '<div class="mb"><div class="mh">' +
      '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' + dir +
      '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
    '</div><button class="cp" onclick="copyMsg(this)">Copy</button>' +
    '<div class="mc">' + fmt(displayContent) + '</div></div>';
  el.querySelector('.mc').dataset.raw = ev.content || '';
  chat.appendChild(el);
  trimTimelineNodes();
  bump();
}

function addIdleMsgToTimeline(ev) {
  if (!idleLiveLog || !ev || ev.type !== 'idlechat.message') return;
  if (isTTSSyncedSpeaker(ev.from)) return;
  removeIdleLiveEmpty();

  const f = ag(ev.from);
  const t = ev.to ? ag(ev.to) : null;
  const dir = t && ev.to ? '<span class="dir">→ ' + t.e + ' ' + t.l + '</span>' : '';
  const displayContent = normalizeViewerDisplayText(ev.content);
  const kind = isIdleTopicEvent(ev) ? 'topic' : 'speech';
  const el = document.createElement('div');
  el.className = 'msg idle-live-item idle-kind-' + kind;
  el.innerHTML =
    '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
    '<div class="mb"><div class="mh">' +
      '<span class="idle-kind">' + (kind === 'topic' ? 'Topic' : 'Speech') + '</span>' +
      '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' + dir +
      '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
    '</div><button class="cp" onclick="copyMsg(this)">Copy</button>' +
    '<div class="mc">' + fmt(displayContent) + '</div></div>';
  el.querySelector('.mc').dataset.raw = ev.content || '';
  idleLiveLog.appendChild(el);
  trimTimelineNodesFor(idleLiveLog, MAX_TIMELINE_NODES);
  idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
}

function addIdleSummaryToTimeline(ev) {
  if (!idleLiveLog || !ev || ev.type !== 'idlechat.summary') return;
  removeIdleLiveEmpty();

  const f = ag(ev.from || 'shiro');
  const displayContent = normalizeViewerDisplayText(ev.content);
  const el = document.createElement('div');
  el.className = 'msg idle-live-item idle-kind-summary';
  el.innerHTML =
    '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
    '<div class="mb"><div class="mh">' +
      '<span class="idle-kind">Summary</span>' +
      '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' +
      '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
    '</div><button class="cp" onclick="copyMsg(this)">Copy</button>' +
    '<div class="mc">' + fmt(displayContent) + '</div></div>';
  el.querySelector('.mc').dataset.raw = ev.content || '';
  idleLiveLog.appendChild(el);
  trimTimelineNodesFor(idleLiveLog, MAX_TIMELINE_NODES);
  idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
}

function isIdleTopicEvent(ev) {
  const content = String((ev && ev.content) || '').trim();
  return String((ev && ev.from) || '').toLowerCase() === 'user' &&
    String((ev && ev.to) || '').toLowerCase() === 'mio' &&
    /^今日のお題/.test(content);
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
    btn.classList.add('ok');
    setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('ok'); }, 1200);
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
    btn.classList.add('ok');
    showToast('Copied to clipboard', 'success');
    setTimeout(() => {
      btn.textContent = old;
      btn.classList.remove('ok');
    }, 1200);
  }).catch((err) => {
    console.error(err);
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

function renderOverview() {
  const cards = document.getElementById('agentCards');
  const body = document.getElementById('overviewBody');
  cards.innerHTML = '';
  body.innerHTML = '';

  AGENTS.forEach((id) => {
    const s = state.agents[id];
    const info = ag(id);
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML =
      '<h4>' + info.e + ' ' + info.l + '</h4>' +
      '<div class="row"><span>State</span><span class="badge ' + stateClass(s.state) + '">' + esc(s.state) + '</span></div>' +
      '<div class="row"><span>Reason</span><span>' + esc(s.reason || '-') + '</span></div>' +
      '<div class="row"><span>Route</span><span>' + esc(s.route || '-') + '</span></div>' +
      '<div class="row"><span>Open</span><span>' + esc(String(Object.keys(state.openTasks[id] || {}).length)) + '</span></div>' +
      '<div class="row"><span>Job</span><span class="code">' + esc(s.jobID || '-') + '</span></div>' +
      '<div class="row"><span>Updated</span><span>' + esc(ftime(s.updatedAt)) + '</span></div>';
    cards.appendChild(card);

    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + info.e + ' ' + info.l + '</td>' +
      '<td><span class="badge ' + stateClass(s.state) + '">' + esc(s.state) + '</span></td>' +
      '<td>' + esc(s.reason || '-') + '</td>' +
      '<td>' + esc(s.route || '-') + '</td>' +
      '<td>' + esc(s.lastEvent || '-') + '</td>' +
      '<td>' + esc(agName(s.peer || '-')) + '</td>' +
      '<td class="code">' + esc(s.jobID || '-') + '</td>' +
      '<td>' + esc(ftime(s.updatedAt)) + '</td>' +
      '<td>' + esc((s.preview || '-') + ' | open: ' + openTaskSummary(id)) + '</td>';
    body.appendChild(tr);
  });
}

function selectedRoleTargetID() {
  return localStorage.getItem('roleSelector.selectedTarget') || '';
}

function selectRoleTarget(id) {
  localStorage.setItem('roleSelector.selectedTarget', String(id || ''));
  renderRoleSelector();
}

function applyRoleTargetToMessage(message) {
  const trimmed = String(message || '').trim();
  if (!trimmed) return '';
  if (/^\/(ops|wild|code|code1|code2|code3|code4)(\s|$)/.test(trimmed)) return trimmed;
  const selected = ROLE_TARGETS.find((target) => target.id === selectedRoleTargetID());
  if (!selected || selected.id === 'mio') return trimmed;
  if (selected.id === 'shiro') return '/ops ' + trimmed;
  if (selected.id === 'coder1') return '/wild ' + trimmed;
  if (selected.id === 'coder2') return '/code2 ' + trimmed;
  if (selected.id === 'coder3') return '/code3 ' + trimmed;
  if (selected.id === 'coder4') return '/code4 ' + trimmed;
  return trimmed;
}

function roleTargetSummary(target) {
  const info = ag(target.id);
  const agent = state.agents[target.id] || {};
  return info.e + ' ' + info.l + ' / ' + target.role + ' / ' + (agent.state || 'offline');
}

function renderRoleSelector() {
  const cards = document.getElementById('roleSelectorCards');
  const body = document.getElementById('roleSelectorBody');
  const detail = document.getElementById('roleSelectorDetail');
  if (!cards || !body || !detail) return;
  const filter = roleFilter ? String(roleFilter.value || '') : '';
  const selectedID = selectedRoleTargetID();
  const targets = ROLE_TARGETS.filter((target) => !filter || target.role === filter);
  cards.innerHTML = '';
  body.innerHTML = '';

  ROLE_TARGETS.forEach((target) => {
    const info = ag(target.id);
    const agent = state.agents[target.id] || {};
    const card = document.createElement('div');
    card.className = 'card' + (selectedID === target.id ? ' evi-selected' : '');
    card.innerHTML =
      '<h4>' + info.e + ' ' + info.l + '</h4>' +
      '<div class="row"><span>Role</span><span>' + esc(target.role) + '</span></div>' +
      '<div class="row"><span>Alias</span><span>' + esc(target.alias) + '</span></div>' +
      '<div class="row"><span>State</span><span class="badge ' + stateClass(agent.state || 'offline') + '">' + esc(agent.state || 'offline') + '</span></div>' +
      '<div class="ops-sub">' + esc(target.use) + '</div>';
    card.addEventListener('click', () => selectRoleTarget(target.id));
    cards.appendChild(card);
  });

  if (targets.length === 0) {
    body.innerHTML = '<tr><td colspan="7" class="small">No role targets</td></tr>';
  } else {
    targets.forEach((target) => {
      const info = ag(target.id);
      const agent = state.agents[target.id] || {};
      const tr = document.createElement('tr');
      if (selectedID === target.id) tr.classList.add('evi-selected');
      tr.innerHTML =
        '<td>' + info.e + ' ' + esc(info.l) + '</td>' +
        '<td>' + esc(target.role) + '</td>' +
        '<td>' + esc(target.alias) + '</td>' +
        '<td>' + esc(target.use) + '</td>' +
        '<td><span class="badge ' + stateClass(agent.state || 'offline') + '">' + esc(agent.state || 'offline') + '</span></td>' +
        '<td>' + esc(agent.route || '-') + '</td>' +
        '<td><button class="ctl-btn" onclick="selectRoleTarget(&quot;' + esc(target.id) + '&quot;)">Select</button></td>';
      tr.addEventListener('click', (evt) => {
        if (evt.target && evt.target.tagName === 'BUTTON') return;
        selectRoleTarget(target.id);
      });
      body.appendChild(tr);
    });
  }

  const selected = ROLE_TARGETS.find((target) => target.id === selectedID);
  if (!selected) {
    detail.innerHTML = '<h4>Selected Target</h4><div class="small">No role selected</div>';
    return;
  }
  const agent = state.agents[selected.id] || {};
  detail.innerHTML =
    '<h4>' + esc(roleTargetSummary(selected)) + '</h4>' +
    '<div class="row"><span>Model Alias</span><span>' + esc(selected.alias) + '</span></div>' +
    '<div class="row"><span>Route</span><span>' + esc(agent.route || '-') + '</span></div>' +
    '<div class="row"><span>Job</span><span class="code">' + esc(agent.jobID || '-') + '</span></div>' +
    '<div class="ops-sub">' + esc(selected.use) + '</div>';
}

function latestOpsEventBy(fn) {
  const list = Array.isArray(state.ops.persistedLogs) ? state.ops.persistedLogs : [];
  for (let i = 0; i < list.length; i++) {
    const ev = list[i];
    if (fn(ev)) return ev;
  }
  return null;
}

function renderOps() {
  const cards = document.getElementById('opsCards');
  const focusBody = document.getElementById('opsFocusBody');
  const feedBody = document.getElementById('opsFeedBody');
  if (!cards || !focusBody || !feedBody) return;
  cards.innerHTML = '';
  focusBody.innerHTML = '';
  feedBody.innerHTML = '';

  const persisted = Array.isArray(state.ops.persistedLogs) ? state.ops.persistedLogs : [];
  const runningJobs = Object.values(state.jobs).filter((j) => String(j.status || '') !== 'done');
  const lastMio = state.ops.lastMioReport || latestOpsEventBy((ev) => String(ev.from || '').toLowerCase() === 'mio' && String(ev.to || '').toLowerCase() === 'user');
  const latestJobID = state.ops.latestJobID || ((persisted[0] && persisted[0].job_id) || '-');
  const latestRoute = state.ops.latestRoute || ((persisted[0] && persisted[0].route) || '-');
  const latestError = state.ops.latestError || latestOpsEventBy((ev) => {
    const t = String(ev.type || '').toLowerCase();
    return t === 'agent.error' || t === 'mailbox.error' || t === 'worker.classified_failure';
  });
  const activeAgents = AGENTS.filter((id) => {
    const s = state.agents[id];
    return s && s.state !== 'offline';
  });

  [
    {title: 'Latest Job', big: latestJobID || '-', sub: 'route: ' + (latestRoute || '-')},
    {
      title: 'Mio Last Report',
      big: lastMio ? short(lastMio.content || '-', 48) : '-',
      sub: lastMio ? ('time: ' + fdt(lastMio.timestamp) + '\njob: ' + (lastMio.job_id || '-')) : 'Mio からの最終報告はまだありません',
    },
    {
      title: 'Running Jobs',
      big: String(runningJobs.length),
      sub: runningJobs.slice(0, 3).map((j) => (j.id || '-') + ' · ' + (j.route || '-') + ' · ' + (j.status || '-')).join('\n') || '進行中ジョブなし',
    },
    {
      title: 'Last Error',
      big: latestError ? short(latestError.type || '-', 24) : 'none',
      sub: latestError ? (short(latestError.content || '-', 120) + '\njob: ' + (latestError.job_id || '-')) : '直近の失敗イベントなし',
    },
    {
      title: 'Active Agents',
      big: String(activeAgents.length),
      sub: activeAgents.map((id) => agName(id) + ' · ' + (state.agents[id].state || '-')).join('\n') || '全員 offline',
    },
  ].forEach((item) => {
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML =
      '<div class="ops-card-title">' + esc(item.title) + '</div>' +
      '<div class="ops-big">' + esc(item.big) + '</div>' +
      '<div class="ops-sub">' + esc(item.sub) + '</div>';
    cards.appendChild(card);
  });

  [
    {label: '最新 route', value: latestRoute || '-'},
    {label: '最新 persisted event', value: persisted[0] ? ((persisted[0].type || '-') + ' @ ' + fdt(persisted[0].timestamp)) : '-'},
    {label: 'Mio job', value: state.agents.mio && state.agents.mio.jobID ? state.agents.mio.jobID : '-'},
    {label: 'Worker job', value: state.agents.shiro && state.agents.shiro.jobID ? state.agents.shiro.jobID : '-'},
  ].forEach((row) => {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td>' + esc(row.label) + '</td><td>' + esc(row.value) + '</td>';
    focusBody.appendChild(tr);
  });

  const feed = persisted.filter((ev) => {
    const t = String(ev.type || '');
    return t === 'message.received' || t === 'routing.decision' || t === 'agent.dispatch' || t === 'agent.start' || t === 'agent.note' || t === 'agent.response' || t === 'mailbox.waiting' || t === 'mailbox.received' || t === 'mailbox.error' || t === 'agent.error' || t === 'worker.classified_failure';
  }).slice(0, 20);
  if (feed.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No operator events yet</td>';
    feedBody.appendChild(tr);
    return;
  }
  feed.forEach((ev) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(ev.timestamp)) + '</td>' +
      '<td>' + esc(ev.type || '-') + '</td>' +
      '<td>' + esc(agName(ev.from || '-')) + ' → ' + esc(agName(ev.to || '-')) + '</td>' +
      '<td class="code">' + esc(ev.job_id || '-') + '</td>' +
      '<td>' + esc(ev.route || '-') + '</td>' +
      '<td>' + esc(short(ev.content || '-', 140)) + '</td>';
    feedBody.appendChild(tr);
  });
}

function renderSessions() {
  const body = document.getElementById('sessionsBody');
  body.innerHTML = '';
  const list = Object.values(state.sessions).sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));

  if (list.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="8" class="small">No session data yet</td>';
    body.appendChild(tr);
    return;
  }

  list.forEach((s) => {
    const agents = Object.keys(s.agents).filter((x) => AGENTS.includes(x) || x === 'user').map((x) => agName(x)).join(', ') || '-';
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td class="code">' + esc(s.id) + '</td>' +
      '<td>' + esc(s.channel || '-') + '</td>' +
      '<td class="code">' + esc(s.chatID || '-') + '</td>' +
      '<td>' + esc(String(s.count)) + '</td>' +
      '<td>' + esc(s.lastRoute || '-') + '</td>' +
      '<td>' + esc(short(s.lastUserMessage || '-', 80)) + '</td>' +
      '<td>' + esc(agents) + '</td>' +
      '<td>' + esc(fdt(s.updatedAt)) + '</td>';
    body.appendChild(tr);
  });
}

function renderJobs() {
  const body = document.getElementById('jobsBody');
  body.innerHTML = '';
  const list = Object.values(state.jobs).sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));

  if (list.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="8" class="small">No job data yet</td>';
    body.appendChild(tr);
    return;
  }

  list.forEach((j) => {
    const st = j.status === 'error' ? 'error' : (j.status === 'done' ? 'idle' : 'running');
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td class="code">' + esc(j.id) + '</td>' +
      '<td>' + esc(j.route || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(st) + '">' + esc(j.status) + '</span></td>' +
      '<td>' + esc(agName(j.from || '-') + ' -> ' + agName(j.to || '-')) + '</td>' +
      '<td>' + esc(fdt(j.startedAt)) + '</td>' +
      '<td>' + esc(fdt(j.updatedAt)) + '</td>' +
      '<td>' + esc(String(j.events || 0)) + '</td>' +
      '<td>' + esc(short(j.preview || '-', 90)) + '</td>';
    body.appendChild(tr);
  });
}

function progressPhaseLabel(phase) {
  const p = String(phase || 'received');
  if (p === 'received') return 'received';
  if (p === 'routing') return 'routing';
  if (p === 'delegating') return 'delegating';
  if (p === 'chatting') return 'chatting';
  if (p === 'delegated_to_worker') return 'to worker';
  if (p === 'delegated_to_coder') return 'to coder';
  if (p === 'queued') return 'queued';
  if (p === 'waiting') return 'waiting';
  if (p === 'processing') return 'processing';
  if (p === 'worker_verifying') return 'verifying';
  if (p === 'retrying') return 'retrying';
  if (p === 'reporting') return 'reporting';
  if (p === 'done') return 'done';
  if (p === 'error') return 'error';
  return p;
}

function classifyJobPhase(ev, current) {
  const from = String(ev.from || '').toLowerCase();
  const to = String(ev.to || '').toLowerCase();
  const content = String(ev.content || '');
  if (ev.type === 'message.received') return {phase: 'received', owner: 'mio'};
  if (ev.type === 'routing.decision') return {phase: 'routing', owner: current.owner || 'mio'};
  if (ev.type === 'agent.dispatch') return {phase: 'delegating', owner: to || current.owner};
  if (ev.type === 'agent.thinking') return {phase: 'chatting', owner: from || current.owner};
  if (ev.type === 'mailbox.sent') return {phase: 'queued', owner: to || current.owner};
  if (ev.type === 'mailbox.waiting') return {phase: 'waiting', owner: to || current.owner};
  if (ev.type === 'mailbox.received') return {phase: 'processing', owner: from || current.owner};
  if (ev.type === 'worker.retry_request') return {phase: 'retrying', owner: to || 'coder1'};
  if (ev.type === 'worker.classified_failure') return {phase: 'retrying', owner: from || 'shiro'};
  if (ev.type === 'agent.error' || ev.type === 'mailbox.error') return {phase: 'error', owner: from || current.owner};
  if (ev.type === 'agent.start') {
    if (to === 'shiro') {
      if (content.indexOf('Worker実行') >= 0 || content.indexOf('Patch') >= 0 || content.indexOf('整形') >= 0) {
        return {phase: 'worker_verifying', owner: 'shiro'};
      }
      return {phase: 'delegated_to_worker', owner: 'shiro'};
    }
    if (to.indexOf('coder') === 0) return {phase: 'delegated_to_coder', owner: to};
    if (to === 'mio') return {phase: 'reporting', owner: 'mio'};
  }
  if (ev.type === 'agent.response') {
    if (from === 'mio' && to === 'user') {
      const lower = content.toLowerCase();
      return {phase: (lower.indexOf('error') >= 0 || lower.indexOf('失敗') >= 0) ? 'error' : 'done', owner: 'mio'};
    }
    if (from === 'shiro' && to === 'mio') return {phase: 'reporting', owner: 'mio'};
    if (from.indexOf('coder') === 0 && to === 'shiro') return {phase: 'worker_verifying', owner: 'shiro'};
  }
  return {phase: current.phase || 'received', owner: current.owner || '-'};
}

function deriveProgressData() {
  const jobs = {};
  state.logs.forEach((ev) => {
    const jid = String(ev.job_id || '');
    if (!jid) return;
    const current = jobs[jid] || {
      jobID: jid,
      route: ev.route || '-',
      phase: 'received',
      owner: 'mio',
      retryCount: 0,
      failureKind: '',
      failureReason: '',
      latestSummary: '',
      latestChatReport: '',
      latestWorkerNote: '',
      latestCoderNote: '',
      status: 'running',
      updatedAt: ev.timestamp || '',
      startedAt: ev.timestamp || '',
      recentEvents: [],
    };

    current.route = ev.route || current.route;
    current.updatedAt = ev.timestamp || current.updatedAt;
    if (!current.startedAt) current.startedAt = ev.timestamp || '';
    if (ev.content) current.latestSummary = short(ev.content, 120);

    const phased = classifyJobPhase(ev, current);
    current.phase = phased.phase;
    current.owner = phased.owner;

    if (ev.type === 'worker.retry_request') {
      const m = String(ev.content || '').match(/retry=(\d+)/);
      const retry = m ? Number(m[1]) : (current.retryCount + 1);
      if (retry > current.retryCount) current.retryCount = retry;
    }
    if (ev.type === 'worker.classified_failure') {
      const raw = String(ev.content || '');
      const idx = raw.indexOf(':');
      current.failureKind = idx > 0 ? raw.slice(0, idx).trim() : raw.trim();
      current.failureReason = idx > 0 ? raw.slice(idx + 1).trim() : raw.trim();
      current.status = 'error';
    }
    if (ev.type === 'agent.error' || ev.type === 'mailbox.error') {
      current.failureKind = ev.type;
      current.failureReason = String(ev.content || '').trim();
      current.status = 'error';
    }
    if (ev.type === 'agent.note') {
      if (String(ev.from || '').toLowerCase() === 'mio' && String(ev.to || '').toLowerCase() === 'user') current.latestChatReport = ev.content || current.latestChatReport;
      if (String(ev.from || '').toLowerCase() === 'shiro') current.latestWorkerNote = ev.content || current.latestWorkerNote;
      if (String(ev.from || '').toLowerCase().indexOf('coder') === 0) current.latestCoderNote = ev.content || current.latestCoderNote;
    }
    if ((ev.type === 'agent.error' || ev.type === 'mailbox.error') && String(ev.from || '').toLowerCase().indexOf('coder') === 0) {
      current.latestCoderNote = ev.content || current.latestCoderNote;
    }
    if (ev.type === 'agent.response' && String(ev.from || '').toLowerCase() === 'mio' && String(ev.to || '').toLowerCase() === 'user') {
      current.latestChatReport = ev.content || current.latestChatReport;
      current.status = current.phase === 'error' ? 'error' : 'done';
    } else if (ev.type === 'agent.response' && current.status !== 'error') {
      current.status = (current.phase === 'done') ? 'done' : 'running';
    }

    if (ev.type === 'agent.start' || ev.type === 'agent.dispatch' || ev.type === 'agent.note' || ev.type === 'agent.response' || ev.type === 'agent.error' || ev.type === 'mailbox.sent' || ev.type === 'mailbox.waiting' || ev.type === 'mailbox.received' || ev.type === 'mailbox.error' || ev.type === 'worker.retry_request' || ev.type === 'worker.classified_failure' || ev.type === 'message.received' || ev.type === 'routing.decision') {
      current.recentEvents.push({
        timestamp: ev.timestamp || '',
        type: ev.type || '',
        from: ev.from || '',
        to: ev.to || '',
        content: short(ev.content || '', 200),
      });
      if (current.recentEvents.length > PROGRESS_RECENT_EVENTS) current.recentEvents.shift();
    }

    jobs[jid] = current;
  });

  const list = Object.values(jobs).sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));
  const running = list.filter((j) => j.status !== 'done');
  const done = list.filter((j) => j.status === 'done').slice(0, PROGRESS_DONE_LIMIT);
  const filtered = running.concat(done);

  const agents = {};
  AGENTS.forEach((id) => {
    const base = state.agents[id] || {};
    const related = filtered.filter((j) => j.owner === id || String(base.jobID || '') === String(j.jobID || '')).sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''));
    const top = related[0] || null;
    agents[id] = {
      id: id,
      state: base.state || 'offline',
      jobID: top ? top.jobID : (base.jobID || '-'),
      phase: top ? top.phase : '-',
      retryCount: top ? top.retryCount : 0,
      failureKind: top ? top.failureKind : '',
      latestSummary: top ? top.latestSummary : (base.preview || '-'),
      updatedAt: top ? top.updatedAt : (base.updatedAt || ''),
      openCount: Object.keys(state.openTasks[id] || {}).length,
    };
  });

  return {jobs: filtered, agents: agents};
}

function toggleProgressJob(jobID) {
  state.progressOpenJobs[jobID] = !state.progressOpenJobs[jobID];
  renderProgress();
}
window.toggleProgressJob = toggleProgressJob;

function renderProgressDetail(job) {
  const items = Array.isArray(job.recentEvents) ? job.recentEvents.slice().reverse() : [];
  const logs = items.length === 0
    ? '<div class="progress-empty">No recent events</div>'
    : items.map((item) => (
      '<div class="progress-log-item">' +
        '<div class="progress-log-meta">' + esc(ftime(item.timestamp)) + ' · ' + esc(item.type) + ' · ' + esc(agName(item.from || '-')) + ' → ' + esc(agName(item.to || '-')) + '</div>' +
        '<div>' + esc(item.content || '-') + '</div>' +
      '</div>'
    )).join('');
  return '' +
    '<div class="progress-detail">' +
      '<div class="progress-section"><b>Chat Report</b><div style="margin-top:6px;line-height:1.5">' + esc(job.latestChatReport || '-') + '</div></div>' +
      '<div class="progress-section"><b>Worker Note</b><div style="margin-top:6px;line-height:1.5">' + esc(job.latestWorkerNote || '-') + '</div></div>' +
      '<div class="progress-section"><b>Coder Note</b><div style="margin-top:6px;line-height:1.5">' + esc(job.latestCoderNote || '-') + '</div></div>' +
      '<div class="progress-section"><b>Failure</b><div style="margin-top:6px;line-height:1.5">' + esc(job.failureReason || '-') + '</div></div>' +
      '<div class="progress-section"><b>Recent Events</b><div class="progress-log" style="margin-top:6px">' + logs + '</div></div>' +
    '</div>';
}

function renderProgress() {
  const data = deriveProgressData();
  const agentCards = document.getElementById('progressAgentCards');
  const body = document.getElementById('progressBody');
  const agentSummary = document.getElementById('progressAgentSummary');
  const jobSummary = document.getElementById('progressJobSummary');
  if (!agentCards || !body || !agentSummary || !jobSummary) return;

  agentCards.innerHTML = '';
  body.innerHTML = '';

  const agentValues = Object.values(data.agents);
  const activeAgents = agentValues.filter((a) => a.state !== 'offline').length;
  agentSummary.textContent = 'active: ' + String(activeAgents) + ' / ' + String(agentValues.length);
  jobSummary.textContent = 'showing: ' + String(data.jobs.length) + ' jobs';

  agentValues.forEach((a) => {
    const info = ag(a.id);
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML =
      '<h4>' + info.e + ' ' + info.l + '</h4>' +
      '<div class="row"><span>State</span><span class="badge ' + stateClass(a.state) + '">' + esc(a.state) + '</span></div>' +
      '<div class="row"><span>Phase</span><span class="phase-badge">' + esc(progressPhaseLabel(a.phase)) + '</span></div>' +
      '<div class="row"><span>Job</span><span class="code">' + esc(a.jobID || '-') + '</span></div>' +
      '<div class="row"><span>Retry</span><span>' + esc(String(a.retryCount || 0)) + '</span></div>' +
      '<div class="row"><span>Failure</span><span>' + esc(a.failureKind || '-') + '</span></div>' +
      '<div class="row"><span>Open</span><span>' + esc(String(a.openCount || 0)) + '</span></div>' +
      '<div class="row"><span>Updated</span><span>' + esc(ftime(a.updatedAt)) + '</span></div>' +
      '<div class="small" style="margin-top:8px;line-height:1.5">' + esc(short(a.latestSummary || '-', 120)) + '</div>';
    agentCards.appendChild(card);
  });

  if (data.jobs.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="10" class="small">No progress data yet</td>';
    body.appendChild(tr);
    return;
  }

  data.jobs.forEach((j) => {
    const st = j.status === 'error' ? 'error' : (j.status === 'done' ? 'idle' : 'running');
    const open = !!state.progressOpenJobs[j.jobID];
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td class="code">' + esc(j.jobID) + '</td>' +
      '<td>' + esc(j.route || '-') + '</td>' +
      '<td><span class="phase-badge">' + esc(progressPhaseLabel(j.phase)) + '</span></td>' +
      '<td>' + esc(agName(j.owner || '-')) + '</td>' +
      '<td>' + esc(String(j.retryCount || 0)) + '</td>' +
      '<td><span class="badge ' + errorKindClass(j.failureKind || '') + '">' + esc(j.failureKind || '-') + '</span></td>' +
      '<td><span class="badge ' + stateClass(st) + '">' + esc(j.status || '-') + '</span></td>' +
      '<td>' + esc(fdt(j.updatedAt)) + '</td>' +
      '<td>' + esc(short(j.latestSummary || '-', 120)) + '</td>' +
      '<td><button class="ctl-btn" onclick="toggleProgressJob(\'' + esc(j.jobID) + '\')">' + (open ? 'Hide' : 'Open') + '</button></td>';
    body.appendChild(tr);

    if (open) {
      const exp = document.createElement('tr');
      exp.className = 'progress-expand';
      exp.innerHTML = '<td colspan="10">' + renderProgressDetail(j) + '</td>';
      body.appendChild(exp);
    }
  });
}

function renderEvidence() {
  const body = document.getElementById('evidenceBody');
  body.innerHTML = '';
  const statusFilter = (eviStatus && eviStatus.value) ? eviStatus.value : '';
  const kindFilter = (eviErrorKind && eviErrorKind.value) ? eviErrorKind.value : '';
  const list = (state.evidence || []).filter((r) => {
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
    const st = (r.status === 'failed') ? 'error' : (r.status === 'passed' ? 'idle' : 'running');
    const ek = String(r.error_kind || '');
    const stepsCount = Array.isArray(r.steps) ? r.steps.length : 0;
    const verifyCount = Array.isArray(r.verification) ? r.verification.length : 0;
    const latestVerify = latestVerificationLink(r.job_id || '', r.verification);
    const tr = document.createElement('tr');
    if ((r.job_id || '') === (state.selectedEvidenceJobID || '')) tr.classList.add('evi-selected');
    tr.innerHTML =
      '<td class="code">' + esc('execution_report:' + (r.job_id || '-')) + '</td>' +
      '<td class="code">' + esc(r.job_id || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(st) + '">' + esc(r.status || '-') + '</span></td>' +
      '<td><span class="badge ' + errorKindClass(ek) + '">' + esc(ek || '-') + '</span></td>' +
      '<td>' + latestVerify + '</td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'steps\', event)">' + esc(String(stepsCount)) + '</button></td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'verification\', event)">' + esc(String(verifyCount)) + '</button></td>' +
      '<td><button class="ctl-btn" onclick="openEvidenceWithFocus(\'' + esc(r.job_id || '') + '\', \'\', event)">' + esc(short(r.goal || '-', 90)) + '</button></td>' +
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

function renderEvidenceSummary() {
  const root = document.getElementById('evidenceSummaryCards');
  if (!root) return;
  const s = state.evidenceSummary || {};
  const st = s.status || {};
  const ek = s.error_kind || {};
  const total = (st.passed || 0) + (st.failed || 0) + (st.other || 0);
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
    '</div>';
}

function renderMemorySnapshot() {
  const snap = state.memory.snapshot || {};
  const memory = Array.isArray(snap.memory) ? snap.memory : [];
  const news = Array.isArray(snap.news) ? snap.news : [];
  const digests = Array.isArray(snap.digests) ? snap.digests : [];
  const knowledge = Array.isArray(snap.knowledge) ? snap.knowledge : [];
  const memoryBody = document.getElementById('memoryBody');
  const newsBody = document.getElementById('newsPackBody');
  const digestBody = document.getElementById('digestBody');
  const memoryCount = document.getElementById('memoryCount');
  const newsPackCount = document.getElementById('newsPackCount');
  const digestCount = document.getElementById('digestCount');
  const knowledgeCount = document.getElementById('knowledgeCount');
  if (memoryCount) memoryCount.textContent = String(memory.length);
  if (newsPackCount) newsPackCount.textContent = String(news.length);
  if (digestCount) digestCount.textContent = String(digests.length);
  if (knowledgeCount) knowledgeCount.textContent = String(knowledge.length);
  if (memoryBody) {
    memoryBody.innerHTML = '';
    if (memory.length === 0) {
      memoryBody.innerHTML = '<tr><td colspan="7" class="small">No memory for selected namespace</td></tr>';
    } else {
      memory.forEach((m) => {
        const tr = document.createElement('tr');
        const id = esc(m.ID || m.id || '');
        tr.innerHTML =
          '<td>' + esc(m.Layer || m.layer || '-') + '</td>' +
          '<td><span class="badge state-idle">' + esc(m.MemoryState || m.memory_state || '-') + '</span></td>' +
          '<td class="code">' + esc(m.Namespace || m.namespace || '-') + '</td>' +
          '<td>' + esc(m.Speaker || m.speaker || '-') + '</td>' +
          '<td>' + esc(short(m.Message || m.message || '-', 180)) + '</td>' +
          '<td>' + esc(fdt(m.UpdatedAt || m.updated_at || m.CreatedAt || m.created_at)) + '</td>' +
          '<td>' +
            '<button class="ctl-btn" onclick="setMemoryState(&quot;' + id + '&quot;, &quot;candidate&quot;)">Candidate</button> ' +
            '<button class="ctl-btn" onclick="setMemoryState(&quot;' + id + '&quot;, &quot;confirmed&quot;)">Confirm</button> ' +
            '<button class="ctl-btn" onclick="promoteMemory(&quot;' + id + '&quot;)">Promote</button>' +
          '</td>';
        memoryBody.appendChild(tr);
      });
    }
  }
  if (newsBody) {
    newsBody.innerHTML = '';
    if (news.length === 0) {
      newsBody.innerHTML = '<tr><td colspan="5" class="small">No news pack items</td></tr>';
    } else {
      news.forEach((n) => {
        const tr = document.createElement('tr');
        const urls = n.SourceURL || n.source_url || '';
        tr.innerHTML =
          '<td>' + esc(fdt(n.PublishedAt || n.published_at || n.FetchedAt || n.fetched_at)) + '</td>' +
          '<td>' + esc(n.Category || n.category || '-') + '</td>' +
          '<td class="code">' + esc(short(urls || n.SourceID || n.source_id || '-', 80)) + '</td>' +
          '<td>' + esc(short(n.SummaryDraft || n.summary_draft || n.RawText || n.raw_text || '-', 180)) + '</td>' +
          '<td>' + esc((n.Keywords || n.keywords || []).join ? (n.Keywords || n.keywords || []).join(', ') : '-').replace(/,/g, ', ') + '</td>';
        newsBody.appendChild(tr);
      });
    }
  }
  if (digestBody) {
    digestBody.innerHTML = '';
    if (digests.length === 0) {
      digestBody.innerHTML = '<tr><td colspan="4" class="small">No daily digests</td></tr>';
    } else {
      digests.forEach((d) => {
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(d.DigestSlot || d.digest_slot || '-') + '</td>' +
          '<td>' + esc(d.Category || d.category || '-') + '</td>' +
          '<td>' + esc(short(d.DigestText || d.digest_text || '-', 220)) + '</td>' +
          '<td>' + esc(fdt(d.CreatedAt || d.created_at)) + '</td>';
        digestBody.appendChild(tr);
      });
    }
  }
  renderNewsPackPanel();
}

function newsItems() {
  const snap = state.memory.snapshot || {};
  return Array.isArray(snap.news) ? snap.news : [];
}

function newsDigests() {
  const snap = state.memory.snapshot || {};
  return Array.isArray(snap.digests) ? snap.digests : [];
}

function newsSourceURL(item) {
  return String((item && (item.SourceURL || item.source_url)) || '').trim();
}

function newsSourceID(item) {
  return String((item && (item.SourceID || item.source_id)) || '').trim();
}

function newsSummary(item) {
  return String((item && (item.SummaryDraft || item.summary_draft || item.RawText || item.raw_text)) || '-');
}

function newsKeywords(item) {
  const raw = item && (item.Keywords || item.keywords);
  return Array.isArray(raw) ? raw : [];
}

function newsUsageMatches(item) {
  const url = newsSourceURL(item);
  const sourceID = newsSourceID(item);
  if (!url && !sourceID) return [];
  const traces = Array.isArray(state.memory.traces) ? state.memory.traces : [];
  const matches = [];
  traces.forEach((trace) => {
    const items = Array.isArray(trace.Items || trace.items) ? (trace.Items || trace.items) : [];
    items.forEach((ri) => {
      const urls = ri.SourceURLs || ri.source_urls || [];
      const urlText = Array.isArray(urls) ? urls.join(' ') : String(urls || '');
      const summary = String(ri.Summary || ri.summary || '');
      if ((url && urlText.indexOf(url) >= 0) || (sourceID && summary.indexOf(sourceID) >= 0)) {
        matches.push({trace, item: ri, source: url || sourceID});
      }
    });
  });
  return matches;
}

function newsUsageCount(item) {
  return newsUsageMatches(item).length;
}

function newsRelatedMemoryMatches(item) {
  const related = [];
  newsUsageMatches(item).forEach((match) => {
    const traceItems = Array.isArray(match.trace.Items || match.trace.items) ? (match.trace.Items || match.trace.items) : [];
    traceItems.forEach((ri) => {
      const kind = String(ri.Kind || ri.kind || '');
      if (kind === 'search_cache') return;
      related.push({
        layer: ri.Layer || ri.layer || '-',
        kind: kind || '-',
        reason: ri.Reason || ri.reason || ri.Decision || ri.decision || '-',
        summary: ri.Summary || ri.summary || '-',
      });
    });
  });
  return related;
}

function renderNewsPackPanel() {
  const news = newsItems();
  const digests = newsDigests();
  const body = document.getElementById('newsPackPanelBody');
  const digestBody = document.getElementById('newsDigestPanelBody');
  const detail = document.getElementById('newsPackDetail');
  const usageBody = document.getElementById('newsUsageBody');
  const relatedBody = document.getElementById('newsRelatedMemoryBody');
  const newsCount = document.getElementById('newsPackPanelCount');
  const digestCount = document.getElementById('newsDigestPanelCount');
  const usageCountEl = document.getElementById('newsUsageCount');
  if (newsCount) newsCount.textContent = String(news.length);
  if (digestCount) digestCount.textContent = String(digests.length);
  const totalUsage = news.reduce((sum, item) => sum + newsUsageCount(item), 0);
  if (usageCountEl) usageCountEl.textContent = String(totalUsage);

  if (state.memory.selectedNewsIndex < 0 || state.memory.selectedNewsIndex >= news.length) {
    state.memory.selectedNewsIndex = 0;
  }
  const selected = news[state.memory.selectedNewsIndex] || null;

  if (body) {
    body.innerHTML = '';
    if (news.length === 0) {
      body.innerHTML = '<tr><td colspan="5" class="small">No news pack items</td></tr>';
    } else {
      news.forEach((item, idx) => {
        const tr = document.createElement('tr');
        if (idx === state.memory.selectedNewsIndex) tr.classList.add('evi-selected');
        const url = newsSourceURL(item);
        const source = url || newsSourceID(item) || '-';
        tr.innerHTML =
          '<td>' + esc(fdt(item.PublishedAt || item.published_at || item.FetchedAt || item.fetched_at)) + '</td>' +
          '<td>' + esc(item.Category || item.category || '-') + '</td>' +
          '<td class="code">' + esc(short(source, 90)) + '</td>' +
          '<td>' + esc(short(newsSummary(item), 220)) + '</td>' +
          '<td><button class="ctl-btn" onclick="selectNewsPackItem(' + idx + ')">' + esc(String(newsUsageCount(item))) + '</button></td>';
        tr.addEventListener('click', (evt) => {
          if (evt.target && evt.target.tagName === 'BUTTON') return;
          selectNewsPackItem(idx);
        });
        body.appendChild(tr);
      });
    }
  }

  if (detail) {
    if (!selected) {
      detail.innerHTML = '<h4>Source Detail</h4><div class="small">No news selected</div>';
    } else {
      const url = newsSourceURL(selected);
      const source = url || newsSourceID(selected) || '-';
      const link = url ? '<a href="' + esc(url) + '" target="_blank" rel="noopener noreferrer">' + esc(short(url, 130)) + '</a>' : esc(source);
      detail.innerHTML =
        '<h4>' + esc(short(newsSummary(selected), 90)) + '</h4>' +
        '<div class="row"><span>Category</span><span>' + esc(selected.Category || selected.category || '-') + '</span></div>' +
        '<div class="row"><span>Source</span><span class="code">' + link + '</span></div>' +
        '<div class="row"><span>Published</span><span>' + esc(fdt(selected.PublishedAt || selected.published_at || selected.FetchedAt || selected.fetched_at)) + '</span></div>' +
        '<div class="row"><span>Source ID</span><span class="code">' + esc(newsSourceID(selected) || '-') + '</span></div>' +
        '<div class="row"><span>Keywords</span><span>' + esc(newsKeywords(selected).join(', ') || '-') + '</span></div>' +
        '<div class="ops-sub">' + esc(newsSummary(selected)) + '</div>';
    }
  }

  if (digestBody) {
    digestBody.innerHTML = '';
    if (digests.length === 0) {
      digestBody.innerHTML = '<tr><td colspan="5" class="small">No daily digests</td></tr>';
    } else {
      digests.forEach((d) => {
        const ids = d.NewsIDs || d.news_ids || [];
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(d.DigestSlot || d.digest_slot || '-') + '</td>' +
          '<td>' + esc(d.Category || d.category || '-') + '</td>' +
          '<td>' + esc(short(d.DigestText || d.digest_text || '-', 260)) + '</td>' +
          '<td class="code">' + esc(short(Array.isArray(ids) ? ids.join(', ') : String(ids || '-'), 120)) + '</td>' +
          '<td>' + esc(fdt(d.CreatedAt || d.created_at)) + '</td>';
        digestBody.appendChild(tr);
      });
    }
  }

  if (usageBody) {
    usageBody.innerHTML = '';
    const matches = selected ? newsUsageMatches(selected) : [];
    if (matches.length === 0) {
      usageBody.innerHTML = '<tr><td colspan="4" class="small">No recall usage for selected news</td></tr>';
    } else {
      matches.forEach((match) => {
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td class="code">' + esc(match.trace.ResponseID || match.trace.response_id || '-') + '</td>' +
          '<td>' + esc(match.trace.Role || match.trace.role || '-') + '</td>' +
          '<td class="code">' + esc(short(match.source || '-', 120)) + '</td>' +
          '<td>' + esc(fdt(match.trace.CreatedAt || match.trace.created_at)) + '</td>';
        usageBody.appendChild(tr);
      });
    }
  }
  if (relatedBody) {
    relatedBody.innerHTML = '';
    const related = selected ? newsRelatedMemoryMatches(selected) : [];
    if (related.length === 0) {
      relatedBody.innerHTML = '<tr><td colspan="4" class="small">No related memory for selected news</td></tr>';
    } else {
      related.forEach((item) => {
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td class="code">' + esc(item.layer) + '</td>' +
          '<td>' + esc(item.kind) + '</td>' +
          '<td>' + esc(short(item.reason, 160)) + '</td>' +
          '<td>' + esc(short(item.summary, 220)) + '</td>';
        relatedBody.appendChild(tr);
      });
    }
  }
}

function selectNewsPackItem(idx) {
  state.memory.selectedNewsIndex = idx;
  renderNewsPackPanel();
}

function refreshNewsPack() {
  if (newsPackCategory && memoryCategory) {
    memoryCategory.value = newsPackCategory.value.trim();
  }
  const params = new URLSearchParams();
  params.set('limit', '30');
  if (newsPackCategory && newsPackCategory.value.trim()) params.set('category', newsPackCategory.value.trim());
  fetch('/viewer/memory/snapshot?' + params.toString())
    .then((r) => {
      if (!r.ok) throw new Error('news pack fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.snapshot = data || {};
      renderMemorySnapshot();
      renderNewsPackPanel();
      refreshRecallTraces();
    })
    .catch((err) => console.error(err));
}

function renderMemoryLayers() {
  const layers = state.memory.layers || {};
  const l0 = Array.isArray(layers.l0) ? layers.l0 : [];
  const l1 = Array.isArray(layers.l1) ? layers.l1 : [];
  const l2 = Array.isArray(layers.l2) ? layers.l2 : [];
  const l3 = Array.isArray(layers.l3) ? layers.l3 : [];
  const body = document.getElementById('memoryLayerBody');
  const l0Count = document.getElementById('memoryL0Count');
  const l2Count = document.getElementById('memoryL2Count');
  const l3Count = document.getElementById('memoryL3Count');
  if (l0Count) l0Count.textContent = String(l0.length);
  if (l2Count) l2Count.textContent = String(l2.length);
  if (l3Count) l3Count.textContent = String(l3.length);
  if (!body) return;
  body.innerHTML = '';
  const rows = [];
  const pushMemory = (layer, item) => {
    rows.push({
      layer,
      scope: item.Namespace || item.namespace || item.SessionID || item.session_id || '-',
      kind: item.MemoryState || item.memory_state || item.Speaker || item.speaker || '-',
      summary: item.Message || item.message || '-',
      updated: item.UpdatedAt || item.updated_at || item.CreatedAt || item.created_at || '',
    });
  };
  l0.forEach((item) => pushMemory('L0', item));
  l1.forEach((item) => pushMemory('L1', item));
  l2.forEach((item) => rows.push({
    layer: 'L2',
    scope: item.Domain || item.domain || '-',
    kind: 'thread_summary',
    summary: item.Summary || item.summary || '-',
    updated: item.EndTime || item.end_time || item.StartTime || item.start_time || '',
  }));
  l3.forEach((item) => pushMemory('L3', item));
  if (rows.length === 0) {
    body.innerHTML = '<tr><td colspan="5" class="small">No L0/L2/L3 memory layers</td></tr>';
    return;
  }
  rows.forEach((row) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(row.layer) + '</td>' +
      '<td class="code">' + esc(row.scope) + '</td>' +
      '<td>' + esc(row.kind) + '</td>' +
      '<td>' + esc(short(row.summary, 220)) + '</td>' +
      '<td>' + esc(fdt(row.updated)) + '</td>';
    body.appendChild(tr);
  });
}

function refreshMemoryLayers() {
  const params = new URLSearchParams();
  params.set('limit', '20');
  if (memorySession && memorySession.value.trim()) params.set('session_id', memorySession.value.trim());
  if (memoryNamespace && memoryNamespace.value.trim()) params.set('namespace', memoryNamespace.value.trim());
  if (memoryDomain && memoryDomain.value.trim()) params.set('domain', memoryDomain.value.trim());
  fetch('/viewer/memory/layers?' + params.toString())
    .then((r) => {
      if (!r.ok) throw new Error('memory layers fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.layers = data || {l0: [], l1: [], l2: [], l3: []};
      renderMemoryLayers();
    })
    .catch((err) => console.error(err));
}

function memoryEventNamespaceValue() {
  if (memoryEventNamespace && memoryEventNamespace.value.trim()) return memoryEventNamespace.value.trim();
  if (memoryNamespace && memoryNamespace.value.trim()) return memoryNamespace.value.trim();
  return 'kb:web';
}

function eventPayloadSummary(ev) {
  const payload = ev.Payload || ev.payload || {};
  try {
    return JSON.stringify(payload);
  } catch (_) {
    return String(payload || '-');
  }
}

function renderMemoryEvents() {
  const eventBody = document.getElementById('memoryEventBody');
  const cacheBody = document.getElementById('searchCacheBody');
  const eventCount = document.getElementById('memoryEventCount');
  const cacheCount = document.getElementById('searchCacheCount');
  const events = Array.isArray(state.memory.events) ? state.memory.events : [];
  const searchCache = Array.isArray(state.memory.searchCache) ? state.memory.searchCache : [];
  if (eventCount) eventCount.textContent = String(events.length);
  if (cacheCount) cacheCount.textContent = String(searchCache.length);
  if (eventBody) {
    eventBody.innerHTML = '';
    if (events.length === 0) {
      eventBody.innerHTML = '<tr><td colspan="5" class="small">No L1 event log entries</td></tr>';
    } else {
      events.forEach((ev) => {
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(fdt(ev.CreatedAt || ev.created_at)) + '</td>' +
          '<td class="code">' + esc(ev.Namespace || ev.namespace || '-') + '</td>' +
          '<td>' + esc(ev.EventType || ev.event_type || '-') + '</td>' +
          '<td>' + esc(ev.Source || ev.source || '-') + '</td>' +
          '<td class="code">' + esc(short(eventPayloadSummary(ev), 220)) + '</td>';
        eventBody.appendChild(tr);
      });
    }
  }
  if (cacheBody) {
    cacheBody.innerHTML = '';
    if (searchCache.length === 0) {
      cacheBody.innerHTML = '<tr><td colspan="5" class="small">No search cache entries</td></tr>';
    } else {
      searchCache.forEach((entry) => {
        const urls = entry.SourceURLs || entry.source_urls || [];
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(fdt(entry.RetrievedAt || entry.retrieved_at)) + '</td>' +
          '<td>' + esc(entry.Provider || entry.provider || '-') + '</td>' +
          '<td>' + esc(short(entry.RawQuery || entry.raw_query || entry.NormalizedQuery || entry.normalized_query || '-', 140)) + '</td>' +
          '<td>' + esc(fdt(entry.ExpiresAt || entry.expires_at)) + '</td>' +
          '<td class="code">' + esc(short(Array.isArray(urls) ? urls.join(', ') : String(urls || '-'), 160)) + '</td>';
        cacheBody.appendChild(tr);
      });
    }
  }
}

function refreshMemoryEvents() {
  const params = new URLSearchParams();
  params.set('limit', '20');
  params.set('namespace', memoryEventNamespaceValue());
  fetch('/viewer/memory/events?' + params.toString())
    .then((r) => {
      if (!r.ok) throw new Error('memory events fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.events = Array.isArray(data.events) ? data.events : [];
      state.memory.searchCache = Array.isArray(data.search_cache) ? data.search_cache : [];
      renderMemoryEvents();
    })
    .catch((err) => console.error(err));
}

function renderSourceRegistry() {
  const body = document.getElementById('sourceRegistryBody');
  if (!body) return;
  const entries = Array.isArray(state.memory.sourceRegistry) ? state.memory.sourceRegistry : [];
  body.innerHTML = '';
  if (entries.length === 0) {
    body.innerHTML = '<tr><td colspan="7" class="small">No source registry entries</td></tr>';
    return;
  }
  entries.forEach((s) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td class="code">' + esc(s.source_id || '-') + '</td>' +
      '<td>' + esc(s.kind || '-') + '</td>' +
      '<td>' + esc(String(s.trust_score ?? '-')) + '</td>' +
      '<td>' + esc(String(s.fetch_interval_sec || '-')) + '</td>' +
      '<td>' + esc((s.enabled ? 'enabled' : 'disabled') + (s.last_status ? ' / ' + s.last_status : '')) + '</td>' +
      '<td class="code">' + esc(short(s.url || '-', 110)) + '</td>' +
      '<td><button class="ctl-btn" onclick="runSourceRegistryEntry(&quot;' + esc(s.source_id || '') + '&quot;)">Run</button></td>';
    body.appendChild(tr);
  });
}

function runSourceRegistryEntry(sourceID) {
  const id = String(sourceID || '').trim();
  if (!id) return;
  fetch('/viewer/source-registry?action=run&source_id=' + encodeURIComponent(id), {
    method: 'POST',
  }).then((r) => {
    if (!r.ok) throw new Error('source registry run failed');
    return r.json();
  }).then(() => {
    refreshSourceRegistry();
    refreshMemorySnapshot();
  }).catch((err) => console.error(err));
}

function refreshSourceRegistry() {
  fetch('/viewer/source-registry')
    .then((r) => {
      if (!r.ok) throw new Error('source registry fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.sourceRegistry = Array.isArray(data.entries) ? data.entries : [];
      renderSourceRegistry();
    })
    .catch((err) => console.error(err));
}

function saveSourceRegistryEntry() {
  const payload = {
    source_id: document.getElementById('sourceRegistryID').value.trim(),
    url: document.getElementById('sourceRegistryURL').value.trim(),
    kind: document.getElementById('sourceRegistryKind').value,
    trust_score: Number(document.getElementById('sourceRegistryTrust').value || '0.5'),
    fetch_interval_sec: Number(document.getElementById('sourceRegistryInterval').value || '3600'),
    license_note: document.getElementById('sourceRegistryLicense').value.trim() || 'manual registration',
    enabled: document.getElementById('sourceRegistryEnabled').checked,
    meta: {},
  };
  const namespace = document.getElementById('sourceRegistryNamespace').value.trim();
  if (namespace) payload.meta.namespace = namespace;
  fetch('/viewer/source-registry', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload),
  }).then((r) => {
    if (!r.ok) throw new Error('source registry save failed');
    return r.json();
  }).then(() => refreshSourceRegistry())
    .catch((err) => console.error(err));
}

function exportSourceRegistryYAML() {
  fetch('/viewer/source-registry?format=yaml')
    .then((r) => {
      if (!r.ok) throw new Error('source registry export failed');
      return r.text();
    })
    .then((text) => {
      if (sourceRegistryYAML) sourceRegistryYAML.value = text;
    })
    .catch((err) => console.error(err));
}

function importSourceRegistryYAML() {
  if (!sourceRegistryYAML || !sourceRegistryYAML.value.trim()) return;
  fetch('/viewer/source-registry?format=yaml', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-yaml'},
    body: sourceRegistryYAML.value,
  }).then((r) => {
    if (!r.ok) throw new Error('source registry import failed');
    return r.json();
  }).then(() => refreshSourceRegistry())
    .catch((err) => console.error(err));
}

function refreshMemorySnapshot() {
  const params = new URLSearchParams();
  params.set('limit', '20');
  if (memoryNamespace && memoryNamespace.value.trim()) params.set('namespace', memoryNamespace.value.trim());
  if (memoryCategory && memoryCategory.value.trim()) params.set('category', memoryCategory.value.trim());
  if (memoryDomain && memoryDomain.value.trim()) params.set('domain', memoryDomain.value.trim());
  fetch('/viewer/memory/snapshot?' + params.toString())
    .then((r) => {
      if (!r.ok) throw new Error('memory snapshot fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.snapshot = data || {};
      renderMemorySnapshot();
      refreshMemoryLayers();
      refreshMemoryEvents();
      refreshSourceRegistry();
    })
    .catch((err) => console.error(err));
}

function postMemoryAction(url, payload) {
  return fetch(url, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload),
  }).then((r) => {
    if (!r.ok) throw new Error('memory action failed');
    return r.json();
  }).then(() => refreshMemorySnapshot())
    .catch((err) => console.error(err));
}

function setMemoryState(id, memoryState) {
  if (!id) return;
  postMemoryAction('/viewer/memory/state', {id, memory_state: memoryState});
}

function memoryPromotePayload(id) {
  const targetKind = memoryPromoteKind ? memoryPromoteKind.value.trim() : '';
  const targetID = memoryPromoteID ? memoryPromoteID.value.trim() : '';
  if (!id || !targetKind || !targetID) return null;
  return {
    id,
    target_kind: targetKind,
    target_id: targetID,
    promoted_by: 'viewer',
  };
}

function promoteMemory(id) {
  const payload = memoryPromotePayload(id);
  if (!payload) return;
  postMemoryAction('/viewer/memory/promote', payload);
}

function renderRecallTraces() {
  const body = document.getElementById('recallTraceBody');
  if (!body) return;
  const traces = Array.isArray(state.memory.traces) ? state.memory.traces : [];
  body.innerHTML = '';
  if (traces.length === 0) {
    body.innerHTML = '<tr><td colspan="9" class="small">No recall traces yet</td></tr>';
    return;
  }
  traces.forEach((trace) => {
    const items = Array.isArray(trace.Items || trace.items) ? (trace.Items || trace.items) : [];
    if (items.length === 0) {
      const tr = document.createElement('tr');
      tr.innerHTML =
        '<td class="code">' + esc(trace.ResponseID || trace.response_id || '-') + '</td>' +
        '<td>' + esc(trace.Role || trace.role || '-') + '</td>' +
        '<td colspan="6" class="small">No referenced recall items</td>' +
        '<td>' + esc(fdt(trace.CreatedAt || trace.created_at)) + '</td>';
      body.appendChild(tr);
      return;
    }
    items.forEach((item) => {
      const urls = item.SourceURLs || item.source_urls || [];
      const tr = document.createElement('tr');
      tr.innerHTML =
        '<td class="code">' + esc(trace.ResponseID || trace.response_id || '-') + '</td>' +
        '<td>' + esc(trace.Role || trace.role || '-') + '</td>' +
        '<td>' + esc(item.Layer || item.layer || '-') + '</td>' +
        '<td>' + esc(item.Kind || item.kind || '-') + '</td>' +
        '<td>' + esc(item.Decision || item.decision || '-') + '</td>' +
        '<td>' + esc(short(item.Reason || item.reason || '-', 140)) + '</td>' +
        '<td>' + esc(short(item.Summary || item.summary || item.Query || item.query || '-', 180)) + '</td>' +
        '<td class="code">' + esc(short(Array.isArray(urls) ? urls.join(', ') : String(urls || '-'), 100)) + '</td>' +
        '<td>' + esc(fdt(trace.CreatedAt || trace.created_at)) + '</td>';
      body.appendChild(tr);
    });
  });
}

function refreshRecallTraces() {
  const params = new URLSearchParams();
  params.set('limit', '20');
  fetch('/viewer/recall/traces?' + params.toString())
    .then((r) => {
      if (!r.ok) throw new Error('recall trace fetch failed');
      return r.json();
    })
    .then((data) => {
      state.memory.traces = Array.isArray(data.items) ? data.items : [];
      renderRecallTraces();
      renderNewsPackPanel();
    })
    .catch((err) => console.error(err));
}

function refreshDerivedViews() {
  renderOps();
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

function refreshOpsData() {
  fetch('/viewer/logs?scope=persisted&limit=40')
    .then((r) => {
      if (!r.ok) throw new Error('ops logs fetch failed');
      return r.json();
    })
    .then((data) => {
      const items = Array.isArray(data.items) ? data.items : [];
      state.ops.persistedLogs = items;
      state.ops.lastMioReport = items.find((ev) => String(ev.from || '').toLowerCase() === 'mio' && String(ev.to || '').toLowerCase() === 'user') || null;
      state.ops.latestJobID = items[0] ? (items[0].job_id || '') : '';
      state.ops.latestRoute = items[0] ? (items[0].route || '') : '';
      state.ops.latestError = items.find((ev) => {
        const t = String(ev.type || '').toLowerCase();
        return t === 'agent.error' || t === 'mailbox.error' || t === 'worker.classified_failure';
      }) || null;
      renderOps();
    })
    .catch((err) => console.error(err));
}

function refreshEvidence() {
  fetch('/viewer/evidence/recent?limit=20')
    .then((r) => {
      if (!r.ok) throw new Error('evidence fetch failed');
      return r.json();
    })
    .then((data) => {
      state.evidence = Array.isArray(data.items) ? data.items : [];
      renderEvidence();
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
    .catch((err) => console.error(err));
}

function refreshEvidenceSummary() {
  fetch('/viewer/evidence/summary')
    .then((r) => {
      if (!r.ok) throw new Error('evidence summary fetch failed');
      return r.json();
    })
    .then((data) => {
      state.evidenceSummary = data.summary || {status: {}, error_kind: {}};
      renderEvidenceSummary();
    })
    .catch((err) => console.error(err));
}

function openEvidence(jobID) {
  if (!jobID) return;
  state.selectedEvidenceJobID = jobID;
  syncEvidenceQuery(jobID);
  renderEvidence();
  fetch('/viewer/evidence/detail?job_id=' + encodeURIComponent(jobID))
    .then((r) => {
      if (!r.ok) {
        if (r.status === 404) throw new Error('evidence not found');
        throw new Error('evidence detail fetch failed');
      }
      return r.json();
    })
    .then((data) => {
      state.selectedEvidenceItem = data.item || null;
      updateEvidenceNav();
      const el = document.getElementById('evidenceDetail');
      el.innerHTML = renderEvidenceDetail(state.selectedEvidenceItem || {});
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
if (eviCopy) eviCopy.addEventListener('click', () => {
  if (!state.selectedEvidenceItem) return;
  const text = JSON.stringify(state.selectedEvidenceItem, null, 2);
  navigator.clipboard.writeText(text).then(() => {
    const old = eviCopy.textContent;
    eviCopy.textContent = 'Copied';
    showToast('Copied evidence JSON', 'success');
    setTimeout(() => { eviCopy.textContent = old; }, 1200);
  }).catch((err) => console.error(err));
});
if (eviCopySummary) eviCopySummary.addEventListener('click', () => {
  if (!state.selectedEvidenceItem) return;
  const summary = buildEvidenceSummary(state.selectedEvidenceItem);
  navigator.clipboard.writeText(summary).then(() => {
    const old = eviCopySummary.textContent;
    eviCopySummary.textContent = 'Copied';
    showToast('Copied evidence summary', 'success');
    setTimeout(() => { eviCopySummary.textContent = old; }, 1200);
  }).catch((err) => console.error(err));
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
      try {
        const r = await fetch('/viewer/idlechat/status');
        if (!r.ok) return;
        const d = await r.json();
        const topicEl = document.getElementById('liveTopicText');
        if (topicEl) {
          topicEl.textContent = d.current_topic || '-';
        }
      } catch (_) {}
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
      if (!r.ok) throw new Error('viewer status fetch failed');
      return r.json();
    })
    .then((payload) => applyMonitorStatusSnapshot(payload))
    .catch((err) => console.error(err));
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
      updateAudioButton();
      playNextInternal();
    } catch (err) {
      state.unlocked = false;
      state.blocked = true;
      updateAudioButton();
      startTextFallbackInternal();
      console.error('tts audio unlock failed', err);
    }
  }

  function ensureAudioInternal() {
    if (!state.audio) {
      state.audio = new Audio();
      state.audio.preload = 'auto';
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
      String((item && item.utteranceId) || '')
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
    state.currentShown = false;
    audio.dataset.characterId = state.currentCharacterId;
    audio.src = String((next && next.url) || '');
    audio.play().then(function() {
      markAudioStarted();
      state.audioEnabled = true;
      state.unlocked = true;
      state.blocked = false;
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
      btn.title = '音声がブロックされています。タップして再許可';
      btn.setAttribute('aria-label', '音声がブロックされています。タップして再許可');
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
bindTTSAudioButton(audioBtn);
bindTTSAudioButton(liveAudioBtn);
updateAudioButton();
let sending = false;
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

function send() {
  const text = inp.value.trim();
  if (!text || sending) return;
  sending = true;
  sendBtn.disabled = true;
  inp.disabled = true;

  sendViewerMessage(text)
  .then(() => {
    inp.value = '';
    autoResize();
  })
  .catch((err) => console.error(err))
  .finally(() => {
    sending = false;
    sendBtn.disabled = false;
    inp.disabled = false;
    inp.focus();
  });
}

async function sendViewerMessage(message) {
  const body = {message: applyRoleTargetToMessage(message)};
  if (!body.message) throw new Error('message is required');
  const r = await fetch('/viewer/send', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error('send failed');
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
refreshEvidence();
refreshEvidenceSummary();
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
setInterval(refreshEvidence, 5000);
setInterval(refreshEvidenceSummary, 5000);
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
  reconnectTimer: null,
  reconnecting: false,
  captureLog: [],
  capturePCM: [],
  captureStartedAt: '',
  captureEndedAt: '',
  captureSessionID: '(unknown)',
  voiceBridgeURL: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/stt`,
  runtimeConfigLoaded: false
};

const micBtn = document.getElementById('micBtn');
const micStateEl = document.getElementById('micState');
const sttConnStateEl = document.getElementById('sttConnState');
const sttSessionStateEl = document.getElementById('sttSessionState');
const debugSttSessionEl = document.getElementById('debugSttSession');
const sttCaptureCopyBtn = document.getElementById('sttCaptureCopyBtn');
const sttCaptureDownloadBtn = document.getElementById('sttCaptureDownloadBtn');
const sttCaptureClearBtn = document.getElementById('sttCaptureClearBtn');
const sttSessionCopyBtn = document.getElementById('sttSessionCopyBtn');
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

let llmOpsUIBound = false;
function bindLLMOpsButtons() {
  if (llmOpsUIBound) return;
  llmOpsUIBound = true;
  const refresh = document.getElementById('llmOpsRefreshBtn');
  const stopBtn = document.getElementById('llmOpsStopBtn');
  const restartBtn = document.getElementById('llmOpsRestartBtn');
  if (refresh) refresh.addEventListener('click', refreshLlmOpsStatus);
  if (stopBtn) stopBtn.addEventListener('click', llmOpsStopChatWorker);
  if (restartBtn) restartBtn.addEventListener('click', llmOpsRestartAllRoles);
}

function syncLLMOpsPanel(cfg) {
  const panel = document.getElementById('llmOpsPanel');
  if (!panel) return;
  const configured = Boolean(cfg && cfg.llm_ops_configured);
  const enabled = Boolean(cfg && cfg.llm_ops_enabled);
  const baseURL = cfg && cfg.llm_ops_base_url ? String(cfg.llm_ops_base_url) : '';
  state.ops.llmOpsEnabled = enabled;
  bindLLMOpsButtons();
  const configEl = document.getElementById('llmOpsConfigState');
  const refresh = document.getElementById('llmOpsRefreshBtn');
  const stopBtn = document.getElementById('llmOpsStopBtn');
  const restartBtn = document.getElementById('llmOpsRestartBtn');
  [refresh, stopBtn, restartBtn].forEach((btn) => {
    if (btn) btn.disabled = !enabled;
  });
  if (configEl) {
    if (enabled) {
      configEl.innerHTML = '<span class="badge state-running">enabled</span> ' + esc(baseURL || 'llm_ops configured');
    } else if (configured) {
      configEl.innerHTML = '<span class="badge state-error">token missing</span> ' + esc(baseURL || 'llm_ops configured') + '<div class="ops-sub">LLM_OPS_TOKEN が未設定のためViewerプロキシは無効です</div>';
    } else {
      configEl.innerHTML = '<span class="badge state-offline">disabled</span><div class="ops-sub">~/.picoclaw/config.yaml に llm_ops.enabled/base_url がありません</div>';
    }
  }
  if (enabled) refreshLlmOpsStatus();
  else {
    state.ops.llmStatus = null;
    state.ops.llmStatusError = configured ? 'LLM_OPS_TOKEN missing' : 'llm_ops disabled';
    renderLlmMemoryStatus();
    setLlmOpsStatusPre(state.ops.llmStatusError);
  }
}

function setLlmOpsStatusPre(text) {
  const el = document.getElementById('llmOpsStatusPre');
  if (el) el.textContent = text == null ? '' : String(text);
}

function llmRoleMemoryState(role, info) {
  if (!info || info.pid == null || info.rss_mib == null) return 'offline';
  const roleState = state.ops.llmStatus && state.ops.llmStatus.roles && state.ops.llmStatus.roles[role];
  if (roleState && roleState.halted) return 'error';
  if (roleState && roleState.health_ok === false) return 'error';
  return 'running';
}

function renderLlmMemoryStatus() {
  const cards = document.getElementById('llmMemoryCards');
  const systemBar = document.getElementById('llmMemorySystemBar');
  const rolesEl = document.getElementById('llmMemoryRoles');
  if (!cards || !systemBar || !rolesEl) return;

  const status = state.ops.llmStatus || {};
  const memory = status.memory || {};
  const system = memory.system || {};
  const byRole = memory.llm_by_role || {};
  const totalGiB = num(system.total_gib) || (num(system.total_bytes) / 1073741824);
  const usedGiB = num(system.used_gib) || (num(system.used_bytes) / 1073741824);
  const freeGiB = num(system.free_gib) || (num(system.free_bytes) / 1073741824);
  const usedPct = pct(usedGiB, totalGiB);
  const freePct = pct(freeGiB, totalGiB);
  const llmTotalMiB = Object.keys(byRole).reduce((sum, role) => sum + num(byRole[role] && byRole[role].rss_mib), 0);

  cards.innerHTML = [
    {title: 'Total RAM', big: fmtGiB(totalGiB), sub: system.total_bytes ? fmtBytesAsGiB(system.total_bytes) : 'memory.system.total_gib'},
    {title: 'Used RAM', big: fmtGiB(usedGiB), sub: usedPct.toFixed(1) + '% used'},
    {title: 'Free RAM', big: fmtGiB(freeGiB), sub: freePct.toFixed(1) + '% free'},
    {title: 'LLM RSS Total', big: fmtMiB(llmTotalMiB), sub: 'Chat / Worker process RSS'},
  ].map((item) => (
    '<div class="llm-memory-card">' +
      '<div class="ops-card-title">' + esc(item.title) + '</div>' +
      '<div class="ops-big">' + esc(item.big) + '</div>' +
      '<div class="ops-sub">' + esc(item.sub) + '</div>' +
    '</div>'
  )).join('');

  const barFill = systemBar.querySelector('span');
  if (barFill) barFill.style.width = usedPct.toFixed(1) + '%';
  systemBar.title = 'Used ' + usedPct.toFixed(1) + '% / Free ' + freePct.toFixed(1) + '%';

  const roles = Object.keys(byRole).sort((a, b) => {
    const order = {Chat: 0, Worker: 1};
    return (order[a] ?? 50) - (order[b] ?? 50) || a.localeCompare(b);
  });
  if (roles.length === 0) {
    rolesEl.innerHTML = state.ops.llmStatusError
      ? '<div class="debug-empty">' + esc(state.ops.llmStatusError) + '</div>'
      : '<div class="debug-empty">memory.llm_by_role is empty</div>';
    return;
  }
  rolesEl.innerHTML = roles.map((role) => {
    const info = byRole[role] || {};
    const rssMiB = num(info.rss_mib) || (num(info.rss_bytes) / 1048576);
    const rssPct = pct(rssMiB, totalGiB * 1024);
    const st = llmRoleMemoryState(role, info);
    const pid = info.pid == null ? 'stopped' : 'pid ' + String(info.pid);
    return '<div class="llm-role-memory-item">' +
      '<div class="llm-role-memory-head">' +
        '<div><div class="llm-role-memory-title">' + esc(role) + '</div><div class="llm-role-memory-meta">' + esc(pid) + ' · ' + esc(fmtMiB(rssMiB)) + ' RSS</div></div>' +
        '<span class="badge ' + stateClass(st) + '">' + esc(st) + '</span>' +
      '</div>' +
      '<div class="llm-role-memory-bar" title="' + escAttr(rssPct.toFixed(2) + '% of system RAM') + '"><span style="width:' + escAttr(rssPct.toFixed(2)) + '%"></span></div>' +
    '</div>';
  }).join('');
}

async function refreshLlmOpsStatus() {
  try {
    const res = await fetch('/viewer/llm-ops/status', { cache: 'no-store' });
    const body = await res.text();
    if (!res.ok) {
      state.ops.llmStatusError = 'HTTP ' + res.status;
      setLlmOpsStatusPre('HTTP ' + res.status + '\n' + body);
      renderLlmMemoryStatus();
      return;
    }
    try {
      state.ops.llmStatus = JSON.parse(body);
      state.ops.llmStatusError = '';
      renderLlmMemoryStatus();
      setLlmOpsStatusPre(JSON.stringify(state.ops.llmStatus, null, 2));
    } catch (parseErr) {
      state.ops.llmStatusError = String(parseErr);
      setLlmOpsStatusPre(body);
      renderLlmMemoryStatus();
    }
  } catch (err) {
    state.ops.llmStatusError = String(err);
    setLlmOpsStatusPre(String(err));
    renderLlmMemoryStatus();
  }
}

async function llmOpsStopChatWorker() {
  if (!confirm('MLX 上の Chat と Worker を停止しますか？（自動復旧しません／halted まで停止）')) return;
  try {
    const res = await fetch('/viewer/llm-ops/stop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ roles: ['Chat', 'Worker'] }),
    });
    const body = await res.text();
    setLlmOpsStatusPre((res.ok ? '' : 'HTTP ' + res.status + '\n') + body);
    await refreshLlmOpsStatus();
  } catch (err) {
    setLlmOpsStatusPre(String(err));
  }
}

async function llmOpsRestartAllRoles() {
  if (!confirm('管理対象ロールをすべて再起動しますか？')) return;
  try {
    const res = await fetch('/viewer/llm-ops/restart', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ roles: 'all' }),
    });
    const body = await res.text();
    setLlmOpsStatusPre((res.ok ? '' : 'HTTP ' + res.status + '\n') + body);
    await refreshLlmOpsStatus();
  } catch (err) {
    setLlmOpsStatusPre(String(err));
  }
}

async function loadViewerRuntimeConfig() {
  try {
    const res = await fetch('/viewer/runtime-config', { cache: 'no-store' });
    if (!res.ok) {
      syncLLMOpsPanel(null);
      return;
    }
    const cfg = await res.json();
    if (cfg && cfg.stt_stream_url) {
      sttState.voiceBridgeURL = String(cfg.stt_stream_url).trim() || sttState.voiceBridgeURL;
    }
    sttState.runtimeConfigLoaded = true;
    updateSTTInputIndicators();
    syncLLMOpsPanel(cfg);
  } catch (err) {
    console.warn('[STT] runtime config unavailable:', err);
    syncLLMOpsPanel(null);
  }
}

function recordSTTCaptureEvent(type, payload) {
  if (type !== 'speech_start' && type !== 'draft' && type !== 'final' && type !== 'progress' && type !== 'ready') return;
  const rawPayload = String(payload || '').trim();
  if (type === 'speech_start' || type === 'ready') {
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

function getSTTCaptureSummaryText() {
  const finals = sttState.captureLog
    .filter((item) => item.type === 'final' && item.payload && item.payload !== '-')
    .map((item) => item.payload.trim())
    .filter(Boolean);
  return finals.length > 0 ? finals.join(' / ') : '-';
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

function copySTTCaptureLog() {
  const text = buildSTTCaptureLogText();
  writeClipboardText(text).then(() => {
    showToast('STTログをコピーしました', 'success');
  }).catch((err) => {
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
    showToast('SessionIDをコピーしました', 'success');
  }).catch((err) => {
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
  if (!res.ok) throw new Error('stt log save failed');
}

async function persistSTTWavToServer(wavBuffer) {
  const res = await fetch('/viewer/stt/wav', {
    method: 'POST',
    headers: {'Content-Type': 'audio/wav'},
    body: wavBuffer,
  });
  if (!res.ok) throw new Error('stt wav save failed');
}

async function runSTTAutoTest() {
  const res = await fetch('/viewer/stt/autotest', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      provider_rounds: 1,
      ws_rounds: 1,
      ws_wait: 8,
    }),
  });
  if (!res.ok) throw new Error('stt autotest failed');
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
  if (micBtn) {
    micBtn.classList.toggle('ready', !!sttState.isRecording);
    micBtn.disabled = !voiceAllowed && !sttState.isRecording;
    micBtn.title = voiceAllowed
      ? (sttState.isRecording ? '音声入力中（クリックで停止）' : '音声入力')
      : '音声入力は通常チャットでのみ有効です';
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
    sttSessionStateEl.textContent = 'Session: ' + sid;
    sttSessionStateEl.title = 'Session: ' + sid;
    if (debugSttSessionEl) {
      debugSttSessionEl.textContent = 'Session: ' + sid;
    }
  }
}

async function toggleSTT() {
  if (sttState.isRecording) {
    stopSTT();
  } else {
    if (!isVoiceChatAllowed()) {
      showToast('音声入力は通常チャットでのみ有効です', 'error');
      return;
    }
    await startSTT();
  }
}

async function startSTT() {
  if (!isVoiceChatAllowed()) {
    showToast('音声入力は通常チャットでのみ有効です', 'error');
    return;
  }
  try {
    sttState.isStopping = false;
    sttState.captureLog = [];
    sttState.capturePCM = [];
    sttState.captureStartedAt = '';
    sttState.captureEndedAt = '';
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
    console.log('[STT] Connected - streaming PCM16 16kHz chunks');
    updateSTTInputIndicators();
  };
  sttState.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        const inp = document.getElementById('inp');
        if (msg.type) {
          pushDebugTrace('stt', {
            time: ftime(new Date().toISOString()),
            step: msg.type,
            text: short(String(msg.text || msg.error || ''), 240),
          });
          if (msg.type === 'session_info' && msg.session_id) {
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
          recordSTTCaptureEvent(msg.type, msg.text || '');
          renderDebugPanels();
        }
        if (msg.type === 'draft' && msg.text) {
          console.log('[STT] Draft:', msg.text);
        } else if (msg.type === 'final' && msg.text) {
          console.log('[STT] Final:', msg.text);
          handleSTTFinalText(msg.text);
          // Clear buffer for next utterance (server-side VAD detected end)
          sttState.draftBuffer = [];
        } else if (msg.type === 'reply_reset') {
          console.log('[STT] LLM reply starting...');
        } else if (msg.type === 'reply_delta' && msg.text) {
          console.log('[STT] LLM reply:', msg.text);
        } else if (msg.type === 'empty') {
          console.log('[STT] Empty result');
        } else if (msg.type === 'error') {
          console.error('[STT] Error:', msg.error);
          showToast('認識エラー', 'error');
        }
      } catch (err) {
        console.error('[STT] Parse error:', err);
      }
  };
  sttState.ws.onerror = () => {
    updateSTTInputIndicators();
    if (!sttState.isStopping && sttState.keepSessionChannel) scheduleSTTReconnect();
  };
  sttState.ws.onclose = () => {
    sttState.streamReady = false;
    updateSTTInputIndicators();
    if (!sttState.isStopping && sttState.keepSessionChannel) scheduleSTTReconnect();
  };
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

function sendSTTAudioChunk(pcm16) {
  if (!sttState.isRecording || !sttState.ws || sttState.ws.readyState !== WebSocket.OPEN) return;
  sttState.chunkBuffer.push(...pcm16);
  while (sttState.chunkBuffer.length >= sttState.chunkSamples) {
    const chunk = new Int16Array(sttState.chunkBuffer.slice(0, sttState.chunkSamples));
    sttState.chunkBuffer = sttState.chunkBuffer.slice(sttState.chunkSamples);
    sttState.ws.send(chunk.buffer);
  }
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

  if (sttState.draftTimer) sttState.draftTimer();
  if (sttState.reconnectTimer) {
    clearTimeout(sttState.reconnectTimer);
    sttState.reconnectTimer = null;
  }
  sttState.reconnecting = false;
  sttState.chunkBuffer = [];

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
  if (sttState.ws && sttState.ws.readyState === WebSocket.OPEN) {
    sttState.ws.close();
  }

  sttState.draftBuffer = [];
  updateSTTInputIndicators();
  sttState.isStopping = false;
  persistSTTArtifacts().then(() => {
    showToast('STTログ/WAVを tmp に保存しました', 'success');
  }).catch((err) => {
    console.error('[STT] persist failed:', err);
    showToast('STT保存または自動テストに失敗', 'error');
  });
  console.log('[STT] Stopped');
}
