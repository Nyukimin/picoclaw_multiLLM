// IdleChat tab module: mode controls, subviews, history, and summary review.
function removeIdleLiveEmpty() {
  if (!idleLiveLog) return;
  const empty = idleLiveLog.querySelector('.idle-live-empty');
  if (empty) empty.remove();
}

function clearIdleLiveTimelineForTopic(ev) {
  if (!idleLiveLog || !isIdleTopicEvent(ev)) return;
  const key = idleTopicKey(ev);
  if (!key || key === idleLiveTopicKey) return;
  idleLiveTopicKey = key;
  idlePendingMessages.clear();
  resetTTSSpeechBubble(idleTTSSpeech);
  if (typeof idleLiveLog.replaceChildren === 'function') idleLiveLog.replaceChildren();
  else {
    idleLiveLog.innerHTML = '';
    if (Array.isArray(idleLiveLog.children)) idleLiveLog.children.length = 0;
  }
}

function idleTopicKey(ev) {
  const sid = String((ev && (ev.session_id || ev.chat_id)) || '').trim();
  const content = normalizeViewerDisplayText((ev && ev.content) || '').trim();
  return sid + '|' + content;
}

function recordIdleLiveRendered(kind, ev, text) {
  idleLiveRenderedLog.push({
    kind,
    from: String((ev && ev.from) || ''),
    to: String((ev && ev.to) || ''),
    session_id: String((ev && (ev.session_id || ev.chat_id)) || ''),
    content: String(text || ''),
    timestamp: String((ev && ev.timestamp) || new Date().toISOString()),
  });
  while (idleLiveRenderedLog.length > 200) idleLiveRenderedLog.shift();
}

function idlePendingQueue(sessionId) {
  const sid = String(sessionId || '').trim() || 'idlechat';
  if (!idlePendingMessages.has(sid)) idlePendingMessages.set(sid, []);
  return idlePendingMessages.get(sid);
}

function queueIdleMessageForTTS(ev) {
  if (!ev || ev.type !== 'idlechat.message') return;
  const sid = String(ev.session_id || ev.chat_id || '').trim() || 'idlechat';
  const el = appendIdleLiveMessageEvent(ev);
  const item = {
    ev,
    el,
    from: String(ev.from || '').trim().toLowerCase(),
    consumed: false,
    timer: null,
  };
  item.timer = setTimeout(() => {
    item.consumed = true;
    pruneIdlePendingQueue(sid);
  }, IDLE_MESSAGE_FALLBACK_MS);
  idlePendingQueue(sid).push(item);
}

function consumeIdlePendingMessage(sessionId, characterId) {
  const sid = String(sessionId || '').trim() || 'idlechat';
  const queue = idlePendingMessages.get(sid);
  if (!queue || queue.length === 0) return;
  const id = String(characterId || '').trim().toLowerCase();
  let idx = queue.findIndex((item) => !item.consumed && (!id || item.from === id));
  if (idx < 0) idx = queue.findIndex((item) => !item.consumed);
  if (idx < 0) return;
  const item = queue[idx];
  item.consumed = true;
  if (item.timer) clearTimeout(item.timer);
  queue.splice(idx, 1);
  if (queue.length === 0) idlePendingMessages.delete(sid);
  return item;
}

function pruneIdlePendingQueue(sessionId) {
  const sid = String(sessionId || '').trim() || 'idlechat';
  const queue = idlePendingMessages.get(sid);
  if (!queue) return;
  const kept = queue.filter((item) => !item.consumed);
  if (kept.length === 0) idlePendingMessages.delete(sid);
  else idlePendingMessages.set(sid, kept);
}

function addIdleMsgToTimeline(ev) {
  if (!idleLiveLog || !ev || ev.type !== 'idlechat.message') return;
  clearIdleLiveTimelineForTopic(ev);
  queueIdleMessageForTTS(ev);
}

function appendIdleLiveMessageEvent(ev) {
  if (!idleLiveLog || !ev || ev.type !== 'idlechat.message') return null;
  removeIdleLiveEmpty();

  const f = ag(ev.from);
  const t = ev.to ? ag(ev.to) : null;
  const dir = t && ev.to ? '<span class="dir">→ ' + t.e + ' ' + t.l + '</span>' : '';
  const displayContent = normalizeViewerDisplayText(ev.content);
  const rawBlock = idleRawResponseBlock(ev, displayContent);
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
    '<div class="mc">' + fmt(displayContent) + rawBlock + '</div></div>';
  el.querySelector('.mc').dataset.raw = ev.content || '';
  idleLiveLog.appendChild(el);
  recordIdleLiveRendered(kind, ev, displayContent);
  trimTimelineNodesFor(idleLiveLog, MAX_TIMELINE_NODES);
  idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
  return el;
}

function idleRawResponseBlock(ev, displayContent) {
  if (!ev || isIdleTopicEvent(ev)) return '';
  const raw = String(ev.raw_content || ev.rawContent || '').trim();
  if (!raw) return '';
  return '<div class="idle-raw-response">' +
    '<div class="idle-raw-label">編集前（テストモード）</div>' +
    '<div class="idle-raw-text">' + fmt(raw) + '</div>' +
  '</div>';
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
  recordIdleLiveRendered('summary', ev, displayContent);
  trimTimelineNodesFor(idleLiveLog, MAX_TIMELINE_NODES);
  idleLiveLog.scrollTop = idleLiveLog.scrollHeight;
}

function isIdleTopicEvent(ev) {
  const content = String((ev && ev.content) || '').trim();
  return String((ev && ev.from) || '').toLowerCase() === 'user' &&
    String((ev && ev.to) || '').toLowerCase() === 'mio' &&
    (/^今日のお題/.test(content) || /^お題は[、,:：]/.test(content));
}

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
  const idleErrors = [
    state.idleChat.statusError,
    state.idleChat.logsError,
    state.idleChat.controlError,
  ].map((err) => String(err || '').trim()).filter(Boolean);
  idleErrors.forEach((err) => {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="7" class="small">' + esc(err) + '</td>';
    body.appendChild(tr);
  });
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
      const text = await r.text();
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
      state.idleChat.history = [];
      state.idleChat.statusError = 'IdleChat status unavailable: HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'idlechat status unavailable');
      renderIdleChat();
      return;
    }
    const d = await r.json();
    state.idleChat.statusError = '';
    setIdleState(d.mode || '', !!d.manual_mode, !!d.chat_active);
    idleStartBtn.disabled = !!d.manual_mode || !!d.chat_active;
    if (idleModeNormalBtn) idleModeNormalBtn.disabled = !!d.chat_active;
    if (idleModeForecastBtn) idleModeForecastBtn.disabled = !!d.chat_active;
    if (idleModeStorySimpleBtn) idleModeStorySimpleBtn.disabled = !!d.chat_active;
    idleStopBtn.disabled = !d.manual_mode && !d.chat_active;
    state.idleChat.mode = d.mode || '';
    state.idleChat.manualMode = !!d.manual_mode;
    state.idleChat.chatActive = !!d.chat_active;
    if (state.idleChat.chatActive) state.idleChat.interrupted = false;
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
    state.idleChat.history = [];
    state.idleChat.statusError = 'IdleChat status unavailable: ' + String(_ && _.message ? _.message : _);
    renderIdleChat();
  }
}

async function refreshIdleLogs() {
  try {
    const r = await fetch('/viewer/idlechat/logs?limit=20');
    if (!r.ok) {
      const text = await r.text();
      state.idleChat.history = [];
      state.idleChat.logsError = 'IdleChat logs unavailable: HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'idlechat logs unavailable');
      renderIdleChat();
      return;
    }
    const d = await r.json();
    state.idleChat.logsError = '';
    state.idleChat.mode = d.mode || '';
    state.idleChat.manualMode = !!d.manual_mode;
    state.idleChat.chatActive = !!d.chat_active;
    if (state.idleChat.chatActive) state.idleChat.interrupted = false;
    state.idleChat.currentTopic = d.current_topic || '';
    state.idleChat.history = Array.isArray(d.history) ? d.history : [];
    renderIdleChat();
  } catch (err) {
    state.idleChat.history = [];
    state.idleChat.logsError = 'IdleChat logs unavailable: ' + String(err && err.message ? err.message : err);
    renderIdleChat();
  }
}

async function controlIdle(path) {
  const btns = [idleStartBtn, idleModeNormalBtn, idleModeForecastBtn, idleModeStorySimpleBtn, idleStopBtn].filter(Boolean);
  btns.forEach((b) => { b.disabled = true; });
  try {
    const r = await fetch(path, {method: 'POST'});
    if (!r.ok) {
      const text = await r.text();
      throw new Error('HTTP ' + String(r.status) + ': ' + (text || r.statusText || 'idlechat control failed'));
    }
    state.idleChat.controlError = '';
  } catch (err) {
    state.idleChat.controlError = 'IdleChat control unavailable: ' + String(err && err.message ? err.message : err);
    console.error(err);
  } finally {
    await refreshIdleStatus();
  }
}
