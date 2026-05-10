// Ops tab module: LLM runtime and memory management UI.
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
  state.ops.localLLM = cfg && cfg.local_llm ? cfg.local_llm : null;
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
  renderLocalLLMRuntimeConfig();
  if (enabled) refreshLlmOpsStatus();
  else {
    state.ops.llmStatus = null;
    state.ops.llmStatusError = configured ? 'LLM_OPS_TOKEN missing' : 'llm_ops disabled';
    renderLlmMemoryStatus();
    setLlmOpsStatusPre(state.ops.llmStatusError);
  }
}

function renderLocalLLMRuntimeConfig() {
  const el = document.getElementById('llmRuntimeConfigCards');
  if (!el) return;
  const localLLM = state.ops.localLLM || {};
  if (!localLLM.enabled) {
    el.innerHTML = '<div class="debug-empty">local_llm disabled</div>';
    return;
  }
  const rows = [
    {role: 'Chat', model: localLLM.chat_model, url: localLLM.chat_base_url, state: 'running'},
    {role: 'Worker', model: localLLM.worker_model, url: localLLM.worker_base_url, state: 'running'},
    {role: 'Wild', model: localLLM.wild_model, url: localLLM.wild_base_url, state: sameLocalLLMEndpoint(localLLM.wild_base_url, localLLM.chat_base_url, localLLM.wild_model, localLLM.chat_model) ? 'shared' : 'running'},
  ].filter((row) => row.model || row.url);
  const params = [
    localLLM.provider ? 'provider=' + localLLM.provider : '',
    localLLM.timeout_sec ? 'timeout=' + localLLM.timeout_sec + 's' : '',
    localLLM.global_concurrency ? 'global=' + localLLM.global_concurrency : '',
    localLLM.model_concurrency ? 'model=' + localLLM.model_concurrency : '',
  ].filter(Boolean).join(' · ');
  el.innerHTML = rows.map((row) => (
    '<div class="llm-runtime-card">' +
      '<div class="ops-card-title">' + esc(row.role) + '<span class="badge ' + (row.state === 'shared' ? 'state-thinking' : 'state-running') + '">' + esc(row.state) + '</span></div>' +
      '<div class="llm-runtime-model">' + esc(row.model || '-') + '</div>' +
      '<div class="llm-runtime-url">' + esc(row.url || '-') + '/v1/chat/completions</div>' +
    '</div>'
  )).join('') + (params ? '<div class="ops-sub">' + esc(params) + '</div>' : '');
}

function sameLocalLLMEndpoint(urlA, urlB, modelA, modelB) {
  return String(urlA || '').replace(/\/+$/, '') === String(urlB || '').replace(/\/+$/, '') &&
    String(modelA || '') === String(modelB || '');
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
  const processListsEl = document.getElementById('llmMemoryProcessLists');
  const rolesEl = document.getElementById('llmMemoryRoles');
  if (!cards || !systemBar || !rolesEl) return;

  const status = state.ops.llmStatus || {};
  const localLLM = state.ops.localLLM || {};
  const memory = status.memory || {};
  const system = memory.system || {};
  const byRole = memory.llm_by_role || {};
  const totalGiB = num(system.total_gib) || (num(system.total_bytes) / 1073741824);
  const usedGiB = num(system.used_gib) || (num(system.used_bytes) / 1073741824);
  const freeGiB = num(system.free_gib) || (num(system.free_bytes) / 1073741824);
  const availableGiB = memoryGiB(system, ['available', 'available_for_llm', 'safe_available_for_llm']);
  const swapUsedGiB = memoryGiB(system, ['swap_used', 'swap.used', 'swap_used_for_llm']);
  const compressedGiB = memoryGiB(system, ['compressed', 'compressed_memory']);
  const fileCacheGiB = memoryGiB(system, ['file_cache', 'cache', 'cached']);
  const wiredGiB = memoryGiB(system, ['wired', 'wired_memory']);
  const availableForLLMGiB = memoryGiB(system, ['available_for_llm']);
  const usedForLLMGiB = memoryGiB(system, ['used_for_llm']);
  const safeAvailableForLLMGiB = memoryGiB(system, ['safe_available_for_llm']);
  const safetyMarginGiB = memoryGiB(system, ['llm_safety_margin']);
  const usedPct = pct(usedGiB, totalGiB);
  const freePct = pct(freeGiB, totalGiB);
  const chatRSSMiB = roleRSSMiB(byRole.Chat);
  const workerRSSMiB = roleRSSMiB(byRole.Worker);

  cards.innerHTML = [
    {title: 'Total RAM', big: fmtGiB(totalGiB), sub: system.total_bytes ? fmtBytesAsGiB(system.total_bytes) : 'memory.system.total_gib', indicator: memoryIndicator('none')},
    {title: 'Used RAM', big: fmtGiB(usedGiB), sub: usedPct.toFixed(1) + '% used', indicator: memoryIndicatorForUsedPct(usedPct)},
    {title: 'Free RAM', big: fmtGiB(freeGiB), sub: freePct.toFixed(1) + '% free', indicator: memoryIndicatorForFreePct(freePct)},
    {title: 'Available RAM', big: fmtReportedGiB(availableGiB), sub: memorySourceLabel(system, ['available', 'available_for_llm', 'safe_available_for_llm']), indicator: memoryIndicatorForAvailable(availableGiB)},
    {title: 'Swap Used', big: fmtReportedGiB(swapUsedGiB), sub: memorySourceLabel(system, ['swap_used', 'swap.used', 'swap_used_for_llm']), indicator: memoryIndicatorForSwap(swapUsedGiB)},
    {title: 'Memory Pressure', big: fmtMemoryPressure(system), sub: memorySourceLabel(system, ['memory_pressure', 'pressure', 'memory_pressure_percent']), indicator: memoryIndicatorForPressure(system)},
    {title: 'Compressed', big: fmtReportedGiB(compressedGiB), sub: memorySourceLabel(system, ['compressed', 'compressed_memory']), indicator: memoryIndicatorForCompressed(compressedGiB, totalGiB)},
    {title: 'File Cache', big: fmtReportedGiB(fileCacheGiB), sub: memorySourceLabel(system, ['file_cache', 'cache', 'cached']), indicator: memoryIndicator('none')},
    {title: 'Wired', big: fmtReportedGiB(wiredGiB), sub: memorySourceLabel(system, ['wired', 'wired_memory']), indicator: memoryIndicatorForWired(wiredGiB, totalGiB)},
    {title: 'Available for LLM', big: fmtReportedGiB(availableForLLMGiB), sub: memorySourceLabel(system, ['available_for_llm']), indicator: memoryIndicatorForAvailable(availableForLLMGiB)},
    {title: 'Used for LLM', big: fmtReportedGiB(usedForLLMGiB), sub: memorySourceLabel(system, ['used_for_llm']), indicator: memoryIndicatorForUsedPct(pct(usedForLLMGiB, totalGiB))},
    {title: 'Safe Available', big: fmtReportedGiB(safeAvailableForLLMGiB), sub: memorySourceLabel(system, ['safe_available_for_llm']), indicator: memoryIndicatorForSafeAvailable(safeAvailableForLLMGiB)},
    {title: 'Safety Margin', big: fmtReportedGiB(safetyMarginGiB), sub: memorySourceLabel(system, ['llm_safety_margin']), indicator: memoryIndicator('none')},
    {title: 'Chat RSS', big: fmtGiBFromMiB(chatRSSMiB), sub: rolePIDLabel(byRole.Chat)},
    {title: 'Worker RSS', big: fmtGiBFromMiB(workerRSSMiB), sub: rolePIDLabel(byRole.Worker)},
  ].map((item) => (
    '<div class="llm-memory-card">' +
      '<div class="ops-card-title"><span>' + esc(item.title) + '</span>' + renderMemoryIndicator(item.indicator) + '</div>' +
      '<div class="ops-big">' + esc(item.big) + '</div>' +
      '<div class="ops-sub">' + esc(item.sub) + '</div>' +
    '</div>'
  )).join('');

  const barFill = systemBar.querySelector('span');
  if (barFill) barFill.style.width = usedPct.toFixed(1) + '%';
  systemBar.title = 'Used ' + usedPct.toFixed(1) + '% / Free ' + freePct.toFixed(1) + '%';

  if (processListsEl) {
    processListsEl.innerHTML =
      renderMemoryProcessList('Top Memory Processes', memoryList(memory, system, ['top_memory_processes', 'top_processes', 'processes'])) +
      renderMemoryProcessList('Model Processes', memoryList(memory, system, ['model_processes', 'llm_processes', 'models']));
  }

  const roles = Object.keys(byRole).sort((a, b) => {
    const order = {Chat: 0, Worker: 1};
    return (order[a] ?? 50) - (order[b] ?? 50) || a.localeCompare(b);
  });
  if (roles.length === 0) {
    const fallback = renderLocalLLMFallback(localLLM, state.ops.llmStatusError);
    rolesEl.innerHTML = fallback || (state.ops.llmStatusError
      ? '<div class="debug-empty">' + esc(state.ops.llmStatusError) + '</div>'
      : '<div class="debug-empty">memory.llm_by_role is empty</div>');
    return;
  }
  rolesEl.innerHTML = roles.map((role) => {
    const info = byRole[role] || {};
    const rssMiB = roleRSSMiB(info);
    const rssPct = pct(rssMiB, totalGiB * 1024);
    const st = llmRoleMemoryState(role, info);
    const pid = info.pid == null ? 'stopped' : 'pid ' + String(info.pid);
    return '<div class="llm-role-memory-item">' +
      '<div class="llm-role-memory-head">' +
        '<div><div class="llm-role-memory-title">' + esc(role) + '</div><div class="llm-role-memory-meta">' + esc(pid) + ' · ' + esc(fmtGiBFromMiB(rssMiB)) + ' RSS</div></div>' +
        '<span class="badge ' + stateClass(st) + '">' + esc(st) + '</span>' +
      '</div>' +
      '<div class="llm-role-memory-bar" title="' + escAttr(rssPct.toFixed(2) + '% of system RAM') + '"><span style="width:' + escAttr(rssPct.toFixed(2)) + '%"></span></div>' +
    '</div>';
  }).join('');
}

function memoryField(obj, names) {
  for (const name of names) {
    const parts = String(name).split('.');
    let cur = obj;
    for (const part of parts) {
      if (!cur || typeof cur !== 'object' || !(part in cur)) {
        cur = undefined;
        break;
      }
      cur = cur[part];
    }
    if (cur !== undefined && cur !== null && cur !== '') return {name, value: cur};
  }
  return {name: names[0], value: null};
}

function memoryGiB(system, bases) {
  const gibNames = bases.map((base) => base + '_gib');
  const byteNames = bases.map((base) => base + '_bytes');
  const gib = memoryField(system, gibNames);
  if (gib.value !== null) return num(gib.value);
  const bytes = memoryField(system, byteNames);
  if (bytes.value !== null) return num(bytes.value) / 1073741824;
  return null;
}

function fmtReportedGiB(value) {
  if (value === null || value === undefined || value === '') return '-';
  const n = Number(value);
  if (!Number.isFinite(n) || n < 0) return '-';
  return n.toFixed(n >= 10 ? 1 : 2) + ' GiB';
}

function memorySourceLabel(system, bases) {
  const names = bases.flatMap((base) => [base + '_gib', base + '_bytes', base]);
  const found = memoryField(system, names);
  if (found.value === null) return 'not reported';
  return 'memory.system.' + found.name;
}

function fmtMemoryPressure(system) {
  const found = memoryField(system, ['memory_pressure', 'pressure', 'memory_pressure_percent']);
  if (found.value === null) return '-';
  if (typeof found.value === 'number') {
    return found.name.endsWith('_percent') ? found.value.toFixed(1) + '%' : String(found.value);
  }
  return String(found.value);
}

function memoryIndicator(level, label) {
  const normalized = level || 'unknown';
  const labels = {ok: 'OK', warn: 'WARN', danger: 'DANGER', unknown: 'UNKNOWN', none: ''};
  const classes = {ok: 'running', warn: 'thinking', danger: 'error', unknown: 'offline', none: ''};
  return {level: normalized, label: label || labels[normalized] || 'UNKNOWN', state: classes[normalized] || 'offline'};
}

function renderMemoryIndicator(indicator) {
  if (!indicator || indicator.level === 'none') return '';
  return '<span class="llm-memory-indicator state-' + escAttr(indicator.state) + '">' + esc(indicator.label) + '</span>';
}

function memoryIndicatorForUsedPct(value) {
  if (value == null || !Number.isFinite(Number(value))) return memoryIndicator('unknown');
  if (value >= 95) return memoryIndicator('danger');
  if (value >= 90) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForFreePct(value) {
  if (value == null || !Number.isFinite(Number(value))) return memoryIndicator('unknown');
  if (value <= 3) return memoryIndicator('danger');
  if (value <= 8) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForAvailable(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return memoryIndicator('unknown');
  if (n < 4) return memoryIndicator('danger');
  if (n < 8) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForSafeAvailable(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return memoryIndicator('unknown');
  if (n < 2) return memoryIndicator('danger');
  if (n < 4) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForSwap(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return memoryIndicator('unknown');
  if (n >= 4) return memoryIndicator('danger');
  if (n >= 1) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForCompressed(value, totalGiB) {
  const n = Number(value);
  const total = Number(totalGiB);
  if (!Number.isFinite(n) || !Number.isFinite(total) || total <= 0) return memoryIndicator('unknown');
  const ratio = (n / total) * 100;
  if (ratio >= 10) return memoryIndicator('danger');
  if (ratio >= 5) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForWired(value, totalGiB) {
  const n = Number(value);
  const total = Number(totalGiB);
  if (!Number.isFinite(n) || !Number.isFinite(total) || total <= 0) return memoryIndicator('unknown');
  const ratio = (n / total) * 100;
  if (ratio >= 50) return memoryIndicator('danger');
  if (ratio >= 35) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryIndicatorForPressure(system) {
  const text = String(memoryField(system, ['memory_pressure', 'pressure']).value || '').toLowerCase();
  if (text.includes('critical')) return memoryIndicator('danger');
  if (text.includes('warn')) return memoryIndicator('warn');
  if (text.includes('normal') || text.includes('ok')) return memoryIndicator('ok');
  const pctValue = memoryField(system, ['memory_pressure_percent']).value;
  const n = Number(pctValue);
  if (!Number.isFinite(n)) return memoryIndicator('unknown');
  if (n >= 98) return memoryIndicator('danger');
  if (n >= 90) return memoryIndicator('warn');
  return memoryIndicator('ok');
}

function memoryList(memory, system, names) {
  const fromMemory = memoryField(memory, names);
  if (Array.isArray(fromMemory.value)) return fromMemory.value;
  const fromSystem = memoryField(system, names);
  if (Array.isArray(fromSystem.value)) return fromSystem.value;
  return [];
}

function renderMemoryProcessList(title, rows) {
  const items = Array.isArray(rows) ? rows : [];
  const body = items.length ? items.slice(0, 8).map(renderMemoryProcessRow).join('') : '<div class="ops-sub">not reported</div>';
  return '<div class="llm-memory-process-list">' +
    '<div class="ops-card-title">' + esc(title) + '</div>' +
    body +
  '</div>';
}

function renderMemoryProcessRow(row) {
  if (row == null || typeof row !== 'object') {
    return '<div class="llm-memory-process-row"><span class="llm-memory-process-name">' + esc(String(row || '-')) + '</span><span class="llm-memory-process-meta">-</span></div>';
  }
  const name = row.name || row.command || row.process || row.model || row.role || ('pid ' + (row.pid == null ? '-' : row.pid));
  const rss = row.rss_gib != null ? fmtGiB(row.rss_gib) : fmtGiBFromMiB(row.rss_mib != null ? row.rss_mib : (num(row.rss_bytes) / 1048576));
  const pid = row.pid == null ? '' : 'pid ' + row.pid;
  const meta = [rss, pid].filter((part) => part && part !== '-').join(' · ') || '-';
  return '<div class="llm-memory-process-row"><span class="llm-memory-process-name">' + esc(name) + '</span><span class="llm-memory-process-meta">' + esc(meta) + '</span></div>';
}

function renderLocalLLMFallback(localLLM, errorText) {
  if (!localLLM || !localLLM.enabled) return '';
  const rows = [
    {role: 'Chat', model: localLLM.chat_model, url: localLLM.chat_base_url},
    {role: 'Worker', model: localLLM.worker_model, url: localLLM.worker_base_url},
    {role: 'Wild', model: localLLM.wild_model, url: localLLM.wild_base_url},
  ].filter((row) => row.model || row.url);
  if (!rows.length) return '';
  const note = errorText
    ? '<div class="debug-empty">' + esc(errorText) + '<div class="ops-sub">Mac管理APIが未到達のため、メモリ値は取得できません。推論API設定のみ表示しています。</div></div>'
    : '';
  const params = [
    localLLM.provider ? 'provider=' + localLLM.provider : '',
    localLLM.timeout_sec ? 'timeout=' + localLLM.timeout_sec + 's' : '',
    localLLM.global_concurrency ? 'global=' + localLLM.global_concurrency : '',
    localLLM.model_concurrency ? 'model=' + localLLM.model_concurrency : '',
  ].filter(Boolean).join(' · ');
  return note + rows.map((row) => (
    '<div class="llm-role-memory-item">' +
      '<div class="llm-role-memory-head">' +
        '<div><div class="llm-role-memory-title">' + esc(row.role) + '</div>' +
        '<div class="llm-role-memory-meta">' + esc(row.model || '-') + '</div>' +
        '<div class="ops-sub">' + esc(row.url || '-') + '</div></div>' +
        '<span class="badge state-offline">ops api down</span>' +
      '</div>' +
    '</div>'
  )).join('') + (params ? '<div class="ops-sub">' + esc(params) + '</div>' : '');
}

function roleRSSMiB(info) {
  if (!info) return 0;
  return num(info.rss_mib) || (num(info.rss_bytes) / 1048576);
}

function rolePIDLabel(info) {
  if (!info || info.pid == null) return 'stopped';
  return 'pid ' + String(info.pid);
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
