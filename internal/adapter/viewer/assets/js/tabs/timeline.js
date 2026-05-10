// Chat Timeline tab module: normal chat message rendering.
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
