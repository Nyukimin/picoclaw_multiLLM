// Memory tab module: memory snapshots, layers, source registry, and recall traces.
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

function renderSourceRegistryRunStatus() {
  const el = document.getElementById('sourceRegistryRunStatus');
  if (!el) return;
  const run = state.memory.sourceRegistryLastRun || null;
  if (!run || !run.result) {
    el.innerHTML = '';
    return;
  }
  const result = run.result;
  const warnings = Number(result.Warnings ?? result.warnings ?? 0);
  const parts = [
    'staged=' + esc(String(result.Staged ?? result.staged ?? 0)),
    'validated=' + esc(String(result.Validated ?? result.validated ?? 0)),
    'warnings=' + esc(String(warnings)),
  ];
  const cls = warnings > 0 ? 'badge warn' : 'badge';
  el.innerHTML = '<span class="' + cls + '">Source Registry run: ' + parts.join(' / ') + '</span>';
}

function runSourceRegistryEntry(sourceID) {
  const id = String(sourceID || '').trim();
  if (!id) return;
  fetch('/viewer/source-registry?action=run&source_id=' + encodeURIComponent(id), {
    method: 'POST',
  }).then((r) => {
    if (!r.ok) throw new Error('source registry run failed');
    return r.json();
  }).then((data) => {
    state.memory.sourceRegistryLastRun = data;
    renderSourceRegistryRunStatus();
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
