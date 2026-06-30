// Chat Timeline tab module: normal chat message rendering.
const DEFAULT_CHAT_ROUTE_ALIASES = {
  chatworker: {label: 'ChatWorker', baseURL: 'http://127.0.0.1:8082', model: 'ChatWorker', routePrefix: '/chatworker'},
  worker: {label: 'Worker', baseURL: 'http://127.0.0.1:8082', model: 'Worker', routePrefix: '/ops'},
  heavy: {label: 'Heavy', baseURL: 'http://127.0.0.1:8083', model: 'Heavy', routePrefix: '/analyze'},
  wild: {label: 'Wild', baseURL: 'http://127.0.0.1:8084', model: 'Wild', routePrefix: '/wild'},
};
let CHAT_ROUTE_ALIASES = {...DEFAULT_CHAT_ROUTE_ALIASES};
const CHAT_ROUTE_ALIAS_STORAGE_KEY = 'chatRouteAlias.selected';
const CHAT_RECIPIENT_LLM_SELECTIONS = {mio: 'Chat', shiro: 'ChatWorker', kuro: 'Heavy', midori: 'Wild'};

function syncChatRouteAliasesFromRuntimeConfig(localLLM) {
  if (!localLLM || !localLLM.enabled) {
    CHAT_ROUTE_ALIASES = {...DEFAULT_CHAT_ROUTE_ALIASES};
    return;
  }
  CHAT_ROUTE_ALIASES = {
    chatworker: {
      ...DEFAULT_CHAT_ROUTE_ALIASES.chatworker,
      baseURL: localLLM.worker_base_url || DEFAULT_CHAT_ROUTE_ALIASES.chatworker.baseURL,
      model: localLLM.chat_worker_model || DEFAULT_CHAT_ROUTE_ALIASES.chatworker.model,
    },
    worker: {
      ...DEFAULT_CHAT_ROUTE_ALIASES.worker,
      baseURL: localLLM.worker_base_url || DEFAULT_CHAT_ROUTE_ALIASES.worker.baseURL,
      model: localLLM.worker_model || DEFAULT_CHAT_ROUTE_ALIASES.worker.model,
    },
    heavy: {
      ...DEFAULT_CHAT_ROUTE_ALIASES.heavy,
      baseURL: localLLM.heavy_base_url || DEFAULT_CHAT_ROUTE_ALIASES.heavy.baseURL,
      model: localLLM.heavy_model || DEFAULT_CHAT_ROUTE_ALIASES.heavy.model,
    },
    wild: {
      ...DEFAULT_CHAT_ROUTE_ALIASES.wild,
      baseURL: localLLM.wild_base_url || DEFAULT_CHAT_ROUTE_ALIASES.wild.baseURL,
      model: localLLM.wild_model || DEFAULT_CHAT_ROUTE_ALIASES.wild.model,
    },
  };
}

function selectedChatRouteAlias() {
  localStorage.removeItem(CHAT_ROUTE_ALIAS_STORAGE_KEY);
  return '';
}

function isExplicitRouteMessage(message) {
  return /^\/(ops|wild|heavy|chatworker|chat-worker|worker-chat|code|code1|code2|code3|code4|plan|analyze|research|chat)(\s|$)/.test(String(message || '').trim());
}

function selectChatRouteAlias(alias) {
  localStorage.removeItem(CHAT_ROUTE_ALIAS_STORAGE_KEY);
  syncChatRouteAliasButtons();
}

function syncChatRouteAliasButtons() {
  const selected = selectedChatRouteAlias();
  document.querySelectorAll('[data-chat-route]').forEach((btn) => {
    const active = btn.dataset.chatRoute === selected;
    btn.classList.toggle('active', active);
    btn.setAttribute('aria-pressed', active ? 'true' : 'false');
  });
}

function bindChatRouteAliasButtons() {
  document.querySelectorAll('[data-chat-route]').forEach((btn) => {
    btn.addEventListener('click', () => selectChatRouteAlias(btn.dataset.chatRoute || ''));
  });
  syncChatRouteAliasButtons();
}

function applyChatRouteAliasToMessage(message) {
  const trimmed = String(message || '').trim();
  if (!trimmed || isExplicitRouteMessage(trimmed)) return trimmed;
  const selected = selectedChatRouteAlias();
  const alias = selected ? CHAT_ROUTE_ALIASES[selected] : null;
  return alias ? alias.routePrefix + ' ' + trimmed : trimmed;
}

function selectedLabChatRecipient() {
  const body = typeof document !== 'undefined' ? document.body : null;
  if (!body || !body.classList || !body.classList.contains('lab-mode') || !body.classList.contains('lab-chat-mode')) return '';
  const raw = body.dataset && body.dataset.labSelectedPartner
    ? body.dataset.labSelectedPartner
    : (typeof getLabSelectedPartner === 'function' ? getLabSelectedPartner() : '');
  const actor = typeof normalizeLabActor === 'function'
    ? normalizeLabActor(raw)
    : String(raw || '').trim().toLowerCase();
  return /^(mio|shiro|kuro|midori)$/.test(actor) ? actor : '';
}

function normalizeChatRecipientTarget(value) {
  const target = String(value || '').trim().toLowerCase();
  if (target === 'chatworker' || target === 'worker-chat' || target === 'worker_chat') return 'shiro';
  if (target === 'heavy') return 'kuro';
  if (target === 'wild') return 'midori';
  return /^(mio|shiro|kuro|midori)$/.test(target) ? target : '';
}

function buildViewerSendRequest(message) {
  const trimmed = String(message || '').trim();
  selectedChatRouteAlias();
  if (!trimmed) return {message: ''};
  if (isExplicitRouteMessage(trimmed)) return {message: trimmed};

  const labRecipient = selectedLabChatRecipient();
  if (labRecipient) return {message: trimmed, to: labRecipient};

  const target = selectedChatTargetAgent();
  const recipient = normalizeChatRecipientTarget(target);
  if (recipient) return {message: trimmed, to: recipient};
  return {message: applyRoleTargetToMessage(trimmed)};
}

function viewerLLMStartSelectionForRequest(req) {
  const recipient = normalizeViewerLLMRecipientTarget(req && (req.to || req.recipient || req.target));
  if (recipient && CHAT_RECIPIENT_LLM_SELECTIONS[recipient]) return CHAT_RECIPIENT_LLM_SELECTIONS[recipient];
  const alias = String(req && req.model_alias ? req.model_alias : '').trim();
  if (alias === 'ChatWorker') return 'ChatWorker';
  return alias === 'Worker' || alias === 'Heavy' || alias === 'Wild' ? alias : '';
}

function viewerLLMStartSelectionForRecipient(recipient) {
  const normalized = normalizeViewerLLMRecipientTarget(recipient);
  return normalized && CHAT_RECIPIENT_LLM_SELECTIONS[normalized] ? CHAT_RECIPIENT_LLM_SELECTIONS[normalized] : '';
}

function normalizeViewerLLMRecipientTarget(value) {
  if (typeof normalizeChatRecipientTarget === 'function') return normalizeChatRecipientTarget(value);
  const target = String(value || '').trim().toLowerCase();
  if (target === 'chatworker' || target === 'worker-chat' || target === 'worker_chat') return 'shiro';
  if (target === 'heavy') return 'kuro';
  if (target === 'wild') return 'midori';
  return /^(mio|shiro|kuro|midori)$/.test(target) ? target : '';
}

function selectedChatTargetAgent() {
  const el = document.getElementById('chatTargetAgent');
  return el ? String(el.value || 'mio').trim().toLowerCase() : '';
}

function viewerLLMRoleInfo(status, role) {
  if (!status || !role) return null;
  if (status.roles && status.roles[role]) return status.roles[role];
  return status[role] || null;
}

function viewerLLMControlRoleForSelection(selection) {
  const value = String(selection || '').trim().toLowerCase();
  if (value === 'chatworker') return 'Worker';
  if (value === 'worker') return 'Worker';
  if (value === 'chat') return 'Chat';
  if (value === 'heavy') return 'Heavy';
  if (value === 'wild') return 'Wild';
  return String(selection || '').trim();
}

function viewerLLMRoleHealthy(status, role) {
  const controlRole = viewerLLMControlRoleForSelection(role);
  const info = viewerLLMRoleInfo(status, controlRole) || viewerLLMRoleInfo(status, role);
  if (!info) return false;
  if (info.halted === true) return false;
  return info.health_ok === true || info.status === 'ok' || info.health === 'ok';
}

function viewerLLMSelectionReady(status, selection) {
  return viewerLLMRoleHealthy(status, selection);
}

function viewerLLMLiveModelKeyForSelection(selection) {
  const value = String(selection || '').trim().toLowerCase();
  if (value === 'chat') return 'chat';
  if (value === 'chatworker') return 'chatworker';
  if (value === 'worker') return 'worker';
  if (value === 'heavy') return 'heavy';
  if (value === 'wild') return 'wild';
  return '';
}

function viewerLLMConnectionRefused(error) {
  const text = String(error || '').toLowerCase();
  return /connection refused|actively refused|could not connect|econnrefused|connectex/.test(text);
}

function viewerLocalLLMModelReadyOrDegraded(model) {
  if (!model) return false;
  if (model.loaded === true) return true;
  const status = String(model.status || '').trim().toLowerCase();
  if (status === 'ok' || status === 'ready' || status === 'live') return true;
  if (model.loaded_model || model.backend_model || model.default_model || model.model || model.id) return true;
  if (!model.error) return false;
  return !viewerLLMConnectionRefused(model.error);
}

async function ensureViewerLocalLLMSelectionReady(selection) {
  const key = viewerLLMLiveModelKeyForSelection(selection);
  if (!key) return false;
  if (key === 'chat') return true;

  let cfg;
  try {
    const res = await fetch('/viewer/runtime-config', {cache: 'no-store'});
    if (!res.ok) return false;
    cfg = await res.json();
  } catch (_) {
    return false;
  }
  if (!cfg) return false;
  if (cfg.llm_ops_configured && cfg.llm_ops_enabled === false) {
    throw new Error('llm ops proxy disabled: LLM_OPS_TOKEN missing');
  }
  if (cfg.llm_ops_configured) return false;

  const localLLM = cfg.local_llm || {};
  if (!localLLM.enabled) {
    throw new Error('llm ops proxy not configured: ' + String(selection || key));
  }
  const liveModels = localLLM.live_models || {};
  const model = liveModels[key];
  const detail = model && model.error ? ': ' + String(model.error) : '';
  if (viewerLocalLLMModelReadyOrDegraded(model)) {
    throw new Error('llm ops proxy not configured: ' + String(selection || key) + ' is only degraded/pending' + detail);
  }
  throw new Error('llm ops proxy not configured: ' + String(selection || key) + detail);
}

function formatViewerLLMOpsHTTPError(prefix, status, body) {
  const text = String(body || '').trim();
  return prefix + ': HTTP ' + String(status) + (text ? ': ' + text : '');
}

async function ensureViewerLLMReadyForRequest(req) {
  const selection = viewerLLMStartSelectionForRequest(req);
  return ensureViewerLLMSelectionReady(selection);
}

async function ensureViewerLLMReadyForRecipient(recipient) {
  const selection = viewerLLMStartSelectionForRecipient(recipient);
  return ensureViewerLLMSelectionReady(selection);
}

async function waitViewerLLMSelectionReady(selection, timeoutMs = 35000, pollMs = 500) {
  if (!selection || typeof setTimeout !== 'function') return;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, pollMs));
    const statusRes = await fetch('/viewer/llm-ops/status', {cache: 'no-store'});
    if (!statusRes.ok) continue;
    const status = await statusRes.json();
    if (viewerLLMSelectionReady(status, selection)) return;
  }
  throw new Error('llm ops model switch timed out: ' + selection);
}

async function ensureViewerLLMSelectionReady(selection) {
  if (!selection) return;
  if (await ensureViewerLocalLLMSelectionReady(selection)) return;

  const healthRes = await fetch('/viewer/llm-ops/health', {cache: 'no-store'});
  if (!healthRes.ok) {
    const body = await healthRes.text();
    throw new Error(formatViewerLLMOpsHTTPError('llm ops health failed', healthRes.status, body));
  }

  const statusRes = await fetch('/viewer/llm-ops/status', {cache: 'no-store'});
  if (!statusRes.ok) {
    const body = await statusRes.text();
    throw new Error(formatViewerLLMOpsHTTPError('llm ops status failed', statusRes.status, body));
  }
  const status = await statusRes.json();
  if (viewerLLMSelectionReady(status, selection)) return;

  const startRes = await fetch('/viewer/llm-ops/start', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({selection}),
  });
  const body = await startRes.text();
  if (!startRes.ok) {
    throw new Error(formatViewerLLMOpsHTTPError('llm ops start failed', startRes.status, body));
  }
  await waitViewerLLMSelectionReady(selection);
  if (typeof refreshLlmOpsStatus === 'function') refreshLlmOpsStatus();
}

function addMsgToTimeline(ev) {
  if (ev.type === 'agent.response') removeThinking(ev.job_id);
  if (ev.type === 'agent.thinking') { addThinking(ev); return; }
  if (ev.type === 'agent.start') { addThinkingStart(ev); return; }
  if (isCoordinationTraceEvent(ev)) { addCoordinationTraceToTimeline(ev); return; }

  if (!matchesFilters(ev)) return;
  if (ev.type === 'idlechat.summary') return;
  if (ev.type === 'idlechat.message') return;
  const timelineSpeaker = (ev.from || '').toLowerCase();
  if (ev.type !== 'message.received' && ev.type !== 'idlechat.message' && !['mio', 'chatworker', 'heavy', 'wild', 'shiro', 'kuro', 'midori'].includes(timelineSpeaker)) return;

  const em = document.getElementById('empty');
  if (em) em.remove();

  if (ev.type === 'routing.decision') return;
  if (ev.type === 'agent.response' && (ev.to || '').toLowerCase() !== 'user') return;
  if (ev.type === 'agent.response' && isTTSSyncedSpeaker(ev.from) && !isViewerLocalFailureMessage(ev)) return;
  if (ev.type === 'idlechat.message' && isTTSSyncedSpeaker(ev.from)) return;
  if (ev.type === 'agent.note' && (ev.to || '').toLowerCase() !== 'user') return;
  if (ev.type === 'message.received' && (ev.from || '').toLowerCase() !== 'user') return;

  const f = ag(ev.from);
  const t = ev.to ? ag(ev.to) : null;
  const dir = t && ev.to ? '<span class="dir">→ ' + t.e + ' ' + t.l + '</span>' : '';
  const displayContent = normalizeViewerDisplayText(ev.content);
  const from = String(ev.from || '').toLowerCase();
  const roleClass = from === 'user' ? ' user' : ' assistant';
  const el = document.createElement('div');
  el.className = 'msg' + roleClass;
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

function isCoordinationTraceEvent(ev) {
  const type = String(ev && ev.type ? ev.type : '');
  return type === 'agent.delegate' || type === 'agent.report' || type === 'worker.request' || type === 'worker.result';
}

function addCoordinationTraceToTimeline(ev) {
  if (!matchesCoordinationTraceFilters(ev)) return;
  const em = document.getElementById('empty');
  if (em) em.remove();
  const f = ag(ev.from);
  const t = ev.to ? ag(ev.to) : null;
  const dir = t && ev.to ? '<span class="dir">→ ' + t.e + ' ' + t.l + '</span>' : '';
  const meta = [ev.type || '', ev.route || '', ev.job_id || ''].filter(Boolean).join(' / ');
  const el = document.createElement('div');
  el.className = 'msg assistant coordination-trace';
  el.innerHTML =
    '<div class="av" style="background:' + f.c + '18;color:' + f.c + '">' + f.e + '</div>' +
    '<div class="mb"><div class="mh">' +
      '<span class="an" style="color:' + f.c + '">' + f.l + '</span>' + dir +
      '<span class="tm">' + ftime(ev.timestamp) + '</span>' +
    '</div><button class="cp" onclick="copyMsg(this)">Copy</button>' +
    '<div class="coord-meta">' + esc(meta || 'internal trace') + '</div>' +
    '<div class="mc">' + fmt(normalizeViewerDisplayText(ev.content || '')) + '</div></div>';
  el.querySelector('.mc').dataset.raw = ev.content || '';
  chat.appendChild(el);
  trimTimelineNodes();
  bump();
}

function matchesCoordinationTraceFilters(ev) {
  if (fltType.value && ev.type !== fltType.value) return false;
  if (fltAgent.value && ev.from !== fltAgent.value && ev.to !== fltAgent.value) return false;
  if (fltRoute.value && (ev.route || '') !== fltRoute.value) return false;
  if (fltJob.value && !(ev.job_id || '').toLowerCase().includes(fltJob.value.toLowerCase())) return false;
  if (fltText.value && !(ev.content || '').toLowerCase().includes(fltText.value.toLowerCase())) return false;
  return true;
}

function isViewerLocalFailureMessage(ev) {
  return String(ev && ev.content ? ev.content : '').startsWith('Viewer send unavailable:');
}

bindChatRouteAliasButtons();
