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
  bindDCISearchControls();

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
    toolHarnessOpsCard(),
    dciOpsCard(),
    sandboxOpsCard(),
    skillGovernanceOpsCard(),
    workstreamOpsCard(),
    revenueOpsCard(),
    personaObservationOpsCard(),
    browserTraceAPIOpsCard(),
    complexityHotspotOpsCard(),
    superAgentOpsCard(),
    heavyWorkerRuntimeOpsCard(),
    knowledgeMemoryOpsCard(),
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
  renderKnowledgeMemoryDetailFocus(focusBody);

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
  renderToolHarnessEvents();
  renderDCITraces();
  renderDCISearchResult();
  renderSandboxStatus();
  renderWorkstreamVaultReviews();
  renderRevenueHumanDecisions();
  renderRevenueChannelDrafts();
  renderRevenueDrilldown();
  renderPersonaMetaReviews();
}

function toolHarnessField(ev, snake, pascal) {
  if (!ev) return undefined;
  if (Object.prototype.hasOwnProperty.call(ev, snake)) return ev[snake];
  if (Object.prototype.hasOwnProperty.call(ev, pascal)) return ev[pascal];
  return undefined;
}

function toolHarnessRepairSummary(repair) {
  if (!repair) return '-';
  const kind = toolHarnessField(repair, 'type', 'Type') || '-';
  const path = toolHarnessField(repair, 'path', 'Path');
  const p = Array.isArray(path) ? path.join('.') : String(path || '');
  return p ? kind + ':' + p : kind;
}

function toolHarnessOpsCard() {
  const events = Array.isArray(state.ops.toolHarnessEvents) ? state.ops.toolHarnessEvents : [];
  const repaired = events.filter((ev) => String(toolHarnessField(ev, 'validation_status', 'ValidationStatus') || '') === 'repaired').length;
  const latest = events[0] || null;
  const latestStatus = latest ? String(toolHarnessField(latest, 'validation_status', 'ValidationStatus') || '-') : '-';
  const latestTool = latest ? String(toolHarnessField(latest, 'tool_name', 'ToolName') || '-') : '-';
  return {
    title: 'Tool Harness',
    big: String(repaired) + '/' + String(events.length),
    sub: latest ? ('latest: ' + latestTool + ' · ' + latestStatus) : 'mediation event なし',
  };
}

function renderToolHarnessEvents() {
  const body = document.getElementById('toolHarnessBody');
  if (!body) return;
  body.innerHTML = '';
  const events = Array.isArray(state.ops.toolHarnessEvents) ? state.ops.toolHarnessEvents : [];
  if (events.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No tool mediation events yet</td>';
    body.appendChild(tr);
    return;
  }
  events.slice(0, 30).forEach((ev) => {
    const repairs = toolHarnessField(ev, 'repairs_applied', 'Repairs') || [];
    const defaults = toolHarnessField(ev, 'relation_defaults_applied', 'RelationDefaults') || [];
    const status = String(toolHarnessField(ev, 'validation_status', 'ValidationStatus') || '-');
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(toolHarnessField(ev, 'created_at', 'CreatedAt'))) + '</td>' +
      '<td>' + esc(toolHarnessField(ev, 'tool_name', 'ToolName') || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(status) + '">' + esc(status) + '</span></td>' +
      '<td class="code">' + esc(Array.isArray(repairs) && repairs.length ? repairs.map(toolHarnessRepairSummary).join(', ') : '-') + '</td>' +
      '<td class="code">' + esc(Array.isArray(defaults) && defaults.length ? defaults.map((d) => String(toolHarnessField(d, 'field', 'Field') || '-') + '=' + String(toolHarnessField(d, 'value', 'Value'))).join(', ') : '-') + '</td>' +
      '<td class="code">' + esc(short(toolHarnessField(ev, 'raw_input_hash', 'RawInputHash') || '-', 32)) + '</td>';
    body.appendChild(tr);
  });
}

function dciField(trace, snake, pascal) {
  if (!trace) return undefined;
  if (Object.prototype.hasOwnProperty.call(trace, snake)) return trace[snake];
  if (Object.prototype.hasOwnProperty.call(trace, pascal)) return trace[pascal];
  return undefined;
}

function dciOpsCard() {
  const traces = Array.isArray(state.ops.dciTraces) ? state.ops.dciTraces : [];
  const latest = traces[0] || null;
  const evidenceCount = traces.reduce((sum, trace) => sum + Number(dciField(trace, 'final_evidence_count', 'FinalEvidenceCount') || 0), 0);
  return {
    title: 'DCI Trace',
    big: String(evidenceCount) + '/' + String(traces.length),
    sub: latest ? ('latest: ' + short(dciField(latest, 'user_query', 'UserQuery') || '-', 72)) : 'Search Trace なし',
  };
}

function renderDCITraces() {
  const body = document.getElementById('dciTraceBody');
  if (!body) return;
  body.innerHTML = '';
  const traces = Array.isArray(state.ops.dciTraces) ? state.ops.dciTraces : [];
  if (traces.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No DCI search traces yet</td>';
    body.appendChild(tr);
    return;
  }
  traces.slice(0, 20).forEach((trace) => {
    const scope = dciField(trace, 'corpus_scope', 'CorpusScope') || [];
    const status = String(dciField(trace, 'status', 'Status') || '-');
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(dciField(trace, 'ended_at', 'EndedAt') || dciField(trace, 'started_at', 'StartedAt'))) + '</td>' +
      '<td class="code">' + esc(short(dciField(trace, 'event_id', 'EventID') || '-', 32)) + '</td>' +
      '<td>' + esc(short(dciField(trace, 'user_query', 'UserQuery') || '-', 90)) + '</td>' +
      '<td>' + esc(String(dciField(trace, 'final_evidence_count', 'FinalEvidenceCount') || 0)) + '</td>' +
      '<td><span class="badge ' + stateClass(status) + '">' + esc(status) + '</span></td>' +
      '<td class="code">' + esc(Array.isArray(scope) ? scope.join(', ') : String(scope || '-')) + '</td>';
    body.appendChild(tr);
  });
}

function renderDCISearchResult() {
  const el = document.getElementById('dciSearchResult');
  if (!el) return;
  const result = state.ops.dciLastResult || null;
  if (!result) {
    el.textContent = 'DCI manual search result: -';
    return;
  }
  const pack = result.pack || result.Pack || {};
  const trace = result.trace || result.Trace || {};
  const evidence = pack.evidence || pack.Evidence || [];
  const lines = [
    'event: ' + String(pack.event_id || pack.EventID || trace.event_id || trace.EventID || '-'),
    'query: ' + String(pack.query || pack.Query || '-'),
    'status: ' + String(trace.status || trace.Status || '-'),
    'evidence: ' + String(Array.isArray(evidence) ? evidence.length : 0),
  ];
  if (Array.isArray(evidence) && evidence.length) {
    evidence.slice(0, 3).forEach((ev, idx) => {
      const file = ev.file_path || ev.FilePath || '-';
      const line = ev.line_start || ev.LineStart || '-';
      const snippet = ev.snippet || ev.Snippet || '-';
      lines.push(String(idx + 1) + '. ' + file + ':' + line + ' ' + short(snippet, 160));
    });
  }
  el.textContent = lines.join('\n');
}

function sandboxField(item, snake, pascal) {
  if (!item) return undefined;
  if (Object.prototype.hasOwnProperty.call(item, snake)) return item[snake];
  if (Object.prototype.hasOwnProperty.call(item, pascal)) return item[pascal];
  return undefined;
}

function sandboxOpsCard() {
  const sandboxes = Array.isArray(state.ops.sandboxes) ? state.ops.sandboxes : [];
  const artifacts = Array.isArray(state.ops.sandboxArtifacts) ? state.ops.sandboxArtifacts : [];
  const promotions = Array.isArray(state.ops.sandboxPromotions) ? state.ops.sandboxPromotions : [];
  const decisions = Array.isArray(state.ops.sandboxDecisions) ? state.ops.sandboxDecisions : [];
  const logs = Array.isArray(state.ops.sandboxGateLogs) ? state.ops.sandboxGateLogs : [];
  const blocked = decisions.filter((d) => String(sandboxField(d, 'status', 'Status') || '') !== 'approve').length;
  const latestLog = logs[0] || null;
  return {
    title: 'Sandbox Gate',
    big: String(promotions.length) + '/' + String(sandboxes.length),
    sub: sandboxes.length || promotions.length || artifacts.length ? ('artifacts: ' + String(artifacts.length) + '\nblocked/needs review: ' + String(blocked) + '\nlatest log: ' + String(sandboxField(latestLog, 'gate_status', 'GateStatus') || '-')) : 'sandbox record なし',
  };
}

function renderSandboxStatus() {
  const body = document.getElementById('sandboxBody');
  if (!body) return;
  body.innerHTML = '';
  const sandboxes = Array.isArray(state.ops.sandboxes) ? state.ops.sandboxes : [];
  const promotions = Array.isArray(state.ops.sandboxPromotions) ? state.ops.sandboxPromotions : [];
  const decisions = Array.isArray(state.ops.sandboxDecisions) ? state.ops.sandboxDecisions : [];
  if (sandboxes.length === 0 && promotions.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="7" class="small">No sandbox or promotion records yet</td>';
    body.appendChild(tr);
    renderSandboxPromotionPreviewResult();
    return;
  }
  const rows = Math.max(sandboxes.length, promotions.length);
  for (let i = 0; i < rows; i++) {
    const sandbox = sandboxes[i] || {};
    const promotion = promotions[i] || {};
    const decision = decisions[i] || {};
    const sandboxStatus = String(sandboxField(sandbox, 'status', 'Status') || '-');
    const gate = String(sandboxField(decision, 'status', 'Status') || '-');
    const promotionID = String(sandboxField(promotion, 'promotion_id', 'PromotionID') || '');
    const previewPayload = promotionID ? encodeURIComponent(JSON.stringify(promotion)) : '';
    const preview = promotionID
      ? '<button class="ctl-btn sandbox-promotion-preview" type="button" data-promotion="' + escAttr(previewPayload) + '">Preview</button>'
      : '-';
    const manualReview = promotionID
      ? ' <button class="ctl-btn sandbox-promotion-manual-review" type="button" data-promotion="' + escAttr(previewPayload) + '">Manual Review</button>'
      : '';
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td class="code">' + esc(sandboxField(sandbox, 'sandbox_id', 'SandboxID') || sandboxField(promotion, 'sandbox_id', 'SandboxID') || '-') + '</td>' +
      '<td>' + esc(sandboxField(sandbox, 'type', 'Type') || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(sandboxStatus) + '">' + esc(sandboxStatus) + '</span></td>' +
      '<td class="code">' + esc(short(sandboxField(sandbox, 'path', 'Path') || '-', 90)) + '</td>' +
      '<td class="code">' + esc(promotionID || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(gate) + '">' + esc(gate) + '</span></td>' +
      '<td>' + preview + manualReview + '</td>';
    body.appendChild(tr);
  }
  body.querySelectorAll('.sandbox-promotion-preview').forEach((btn) => {
    btn.addEventListener('click', () => {
      previewSandboxPromotion(btn.getAttribute('data-promotion') || '');
    });
  });
  body.querySelectorAll('.sandbox-promotion-manual-review').forEach((btn) => {
    btn.addEventListener('click', () => {
      requestSandboxPromotionManualReview(btn.getAttribute('data-promotion') || '');
    });
  });
  renderSandboxPromotionPreviewResult();
}

function renderSandboxPromotionPreviewResult() {
  const el = document.getElementById('sandboxPromotionPreviewResult');
  if (!el) return;
  const preview = state.ops.sandboxPromotionPreviewResult || null;
  if (!preview) {
    el.textContent = 'sandbox promotion diff preview: -';
    return;
  }
  const reviewResult = state.ops.sandboxPromotionManualReviewResult || null;
  el.textContent = formatSandboxPromotionDiffPreview(preview) + (reviewResult ? '\n\nmanual review workflow:\n' + JSON.stringify(reviewResult, null, 2) : '');
}

function formatSandboxPromotionDiffPreview(preview) {
  if (!preview || preview.status === 'failed') {
    return 'sandbox promotion diff preview:\n' + JSON.stringify(preview || {}, null, 2);
  }
  const item = preview.preview || preview;
  const files = Array.isArray(sandboxField(item, 'files', 'Files')) ? sandboxField(item, 'files', 'Files') : [];
  const fileCount = sandboxField(item, 'file_count', 'FileCount');
  const added = sandboxField(item, 'added_lines', 'AddedLines');
  const removed = sandboxField(item, 'removed_lines', 'RemovedLines');
  const riskFlags = sandboxDiffRiskFlags(item);
  const manualReview = Boolean(sandboxField(item, 'requires_manual_review', 'RequiresManualReview'));
  const lines = [
    'sandbox promotion diff preview:',
    'status: ' + String(sandboxField(item, 'status', 'Status') || '-'),
    'files: ' + String(fileCount == null ? files.length : fileCount) + ' added: ' + String(added == null ? '-' : added) + ' removed: ' + String(removed == null ? '-' : removed),
    'manual review: ' + String(manualReview ? 'required' : 'not required'),
    'risk flags: ' + (riskFlags.length ? riskFlags.join(', ') : '-'),
  ];
  files.slice(0, 6).forEach((file, idx) => {
    const fileRiskFlags = sandboxDiffRiskFlags(file);
    const fileManualReview = Boolean(sandboxField(file, 'requires_manual_review', 'RequiresManualReview'));
    lines.push('');
    lines.push('file ' + String(idx + 1) + ': ' + String(sandboxField(file, 'path', 'Path') || '-'));
    lines.push('added: ' + String(sandboxField(file, 'added_lines', 'AddedLines') || 0) + ' removed: ' + String(sandboxField(file, 'removed_lines', 'RemovedLines') || 0) + ' hunks: ' + String(sandboxField(file, 'hunk_count', 'HunkCount') || 0));
    if (fileManualReview || fileRiskFlags.length) {
      lines.push('review: required risk flags: ' + (fileRiskFlags.length ? fileRiskFlags.join(', ') : '-'));
    }
    const hunks = Array.isArray(sandboxField(file, 'hunks', 'Hunks')) ? sandboxField(file, 'hunks', 'Hunks') : [];
    hunks.slice(0, 2).forEach((hunk) => {
      lines.push('@@ -' + String(sandboxField(hunk, 'old_start', 'OldStart') || 0) + ',' + String(sandboxField(hunk, 'old_count', 'OldCount') || 0) + ' +' + String(sandboxField(hunk, 'new_start', 'NewStart') || 0) + ',' + String(sandboxField(hunk, 'new_count', 'NewCount') || 0) + ' @@');
      lines.push(twoColumnDiffRows(sandboxField(hunk, 'rows', 'Rows') || [], 58, 18));
    });
  });
  if (files.length > 6) lines.push('\n... +' + String(files.length - 6) + ' more files');
  return lines.join('\n');
}

function sandboxDiffRiskFlags(item) {
  const flags = sandboxField(item, 'risk_flags', 'RiskFlags');
  return Array.isArray(flags) ? flags.map((flag) => String(flag)).filter(Boolean) : [];
}

function twoColumnDiffRows(rows, width, maxRows) {
  const items = Array.isArray(rows) ? rows.slice(0, maxRows || 18) : [];
  const out = [
    padPreviewCell('old', width) + ' | ' + padPreviewCell('new', width),
    repeatChar('-', width) + '-+-' + repeatChar('-', width),
  ];
  items.forEach((row) => {
    const op = String(sandboxField(row, 'op', 'Op') || '');
    const oldNo = sandboxField(row, 'old_line', 'OldLine');
    const newNo = sandboxField(row, 'new_line', 'NewLine');
    const oldText = String(sandboxField(row, 'old_text', 'OldText') || '');
    const newText = String(sandboxField(row, 'new_text', 'NewText') || '');
    const left = (oldNo ? String(oldNo).padStart(4, ' ') + ' ' : '     ') + (op === 'removed' ? '- ' : '  ') + oldText;
    const right = (newNo ? String(newNo).padStart(4, ' ') + ' ' : '     ') + (op === 'added' ? '+ ' : '  ') + newText;
    out.push(padPreviewCell(left, width) + ' | ' + padPreviewCell(right, width));
  });
  if (Array.isArray(rows) && rows.length > items.length) {
    out.push('... +' + String(rows.length - items.length) + ' more rows');
  }
  return out.join('\n');
}

function skillGovernanceOpsCard() {
  const manifests = Array.isArray(state.ops.skillManifests) ? state.ops.skillManifests : [];
  const triggers = Array.isArray(state.ops.skillTriggerLogs) ? state.ops.skillTriggerLogs : [];
  const contributions = Array.isArray(state.ops.contributionGateLogs) ? state.ops.contributionGateLogs : [];
  const transcripts = Array.isArray(state.ops.coderTranscripts) ? state.ops.coderTranscripts : [];
  const blocked = contributions.filter((item) => String(sandboxField(item, 'gate_status', 'GateStatus') || '') === 'blocked').length;
  const missed = triggers.filter((item) => String(sandboxField(item, 'status', 'Status') || '') === 'missed').length;
  const latest = triggers[0] || null;
  const warning = missed > 0 ? '\nWARNING: skill_trigger_missed requires review' : '';
  return {
    title: 'Skill Governance',
    big: String(triggers.length) + '/' + String(manifests.length),
    sub: manifests.length || triggers.length || contributions.length || transcripts.length ? ('missed triggers: ' + String(missed) + '\nblocked contributions: ' + String(blocked) + '\ncoder transcripts: ' + String(transcripts.length) + '\nlatest skill: ' + String(sandboxField(latest, 'skill_id', 'SkillID') || '-') + warning) : 'skill manifest なし',
  };
}

function workstreamOpsCard() {
  const workstreams = Array.isArray(state.ops.workstreams) ? state.ops.workstreams : [];
  const goals = Array.isArray(state.ops.workstreamGoals) ? state.ops.workstreamGoals : [];
  const artifacts = Array.isArray(state.ops.workstreamArtifacts) ? state.ops.workstreamArtifacts : [];
  const annotations = Array.isArray(state.ops.workstreamAnnotations) ? state.ops.workstreamAnnotations : [];
  const steering = Array.isArray(state.ops.workstreamSteering) ? state.ops.workstreamSteering : [];
  const heartbeats = Array.isArray(state.ops.workstreamHeartbeats) ? state.ops.workstreamHeartbeats : [];
  const vaultUpdates = latestWorkstreamVaultUpdates(Array.isArray(state.ops.workstreamVaultUpdates) ? state.ops.workstreamVaultUpdates : []);
  const activeGoals = goals.filter((item) => String(sandboxField(item, 'status', 'Status') || '') === 'active').length;
  const activeHeartbeats = heartbeats.filter((item) => String(sandboxField(item, 'status', 'Status') || '') === 'active').length;
  const approvalPending = vaultUpdates.filter((item) => String(sandboxField(item, 'review_status', 'ReviewStatus') || '') === 'pending').length;
  const latest = workstreams[0] || null;
  return {
    title: 'Workstreams',
    big: String(goals.length) + '/' + String(workstreams.length),
    sub: workstreams.length || goals.length || artifacts.length || annotations.length || steering.length || heartbeats.length || vaultUpdates.length ? ('active goals: ' + String(activeGoals) + ' active heartbeats: ' + String(activeHeartbeats) + '\napproval pending: ' + String(approvalPending) + ' vault updates: ' + String(vaultUpdates.length) + '\nartifacts: ' + String(artifacts.length) + ' annotations: ' + String(annotations.length) + ' steering: ' + String(steering.length) + '\nlatest: ' + String(sandboxField(latest, 'name', 'Name') || '-')) : 'workstream record なし',
  };
}

function latestWorkstreamVaultUpdates(items) {
  const seen = new Set();
  const out = [];
  items.forEach((item) => {
    const id = String(sandboxField(item, 'update_id', 'UpdateID') || '');
    const key = id || JSON.stringify(item);
    if (seen.has(key)) return;
    seen.add(key);
    out.push(item);
  });
  return out;
}

function renderWorkstreamVaultReviews() {
  const body = document.getElementById('workstreamVaultReviewBody');
  if (!body) return;
  body.innerHTML = '';
  const updates = latestWorkstreamVaultUpdates(Array.isArray(state.ops.workstreamVaultUpdates) ? state.ops.workstreamVaultUpdates : []);
  if (updates.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No workstream vault updates yet</td>';
    body.appendChild(tr);
    renderWorkstreamVaultReviewResult();
    return;
  }
  updates.slice(0, 20).forEach((item) => {
    const updateID = String(sandboxField(item, 'update_id', 'UpdateID') || '');
    const review = String(sandboxField(item, 'review_status', 'ReviewStatus') || '-');
    const proposed = String(sandboxField(item, 'proposed_content', 'ProposedContent') || '');
    const pending = review === 'pending';
    const payload = encodeURIComponent(JSON.stringify(item));
    const preview = updateID
      ? '<button class="ctl-btn workstream-vault-preview" type="button" data-update="' + escAttr(payload) + '">Preview</button> '
      : '';
    const actions = preview + (pending && updateID
      ? '<button class="ctl-btn workstream-vault-review" type="button" data-update="' + escAttr(payload) + '" data-review-status="approved">Approve</button> <button class="ctl-btn workstream-vault-review" type="button" data-update="' + escAttr(payload) + '" data-review-status="rejected">Reject</button>'
      : '<span class="small">-</span>');
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(sandboxField(item, 'created_at', 'CreatedAt'))) + '</td>' +
      '<td class="code">' + esc(updateID || '-') + '</td>' +
      '<td class="code">' + esc(short(sandboxField(item, 'file_path', 'FilePath') || '-', 80)) + '</td>' +
      '<td><span class="badge ' + stateClass(review) + '">' + esc(review) + '</span></td>' +
      '<td class="code">' + esc(proposed ? short(proposed, 120) : '-') + '</td>' +
      '<td>' + actions + '</td>';
    body.appendChild(tr);
  });
  body.querySelectorAll('.workstream-vault-review').forEach((btn) => {
    btn.addEventListener('click', () => {
      reviewWorkstreamVaultUpdate(btn.getAttribute('data-update') || '', btn.getAttribute('data-review-status') || '');
    });
  });
  body.querySelectorAll('.workstream-vault-preview').forEach((btn) => {
    btn.addEventListener('click', () => {
      previewWorkstreamVaultUpdate(btn.getAttribute('data-update') || '');
    });
  });
  renderWorkstreamVaultReviewResult();
}

function renderWorkstreamVaultReviewResult() {
  const el = document.getElementById('workstreamVaultReviewResult');
  if (!el) return;
  const result = state.ops.workstreamVaultReviewResult || null;
  const preview = state.ops.workstreamVaultPreviewResult || null;
  if (!result && !preview) {
    el.textContent = 'workstream vault review: -';
    return;
  }
  el.textContent = 'review:\n' + (result ? JSON.stringify(result, null, 2) : '-') + (preview ? '\n\npreview:\n' + formatWorkstreamVaultPreview(preview) : '');
}

function formatWorkstreamVaultPreview(preview) {
  if (!preview || preview.status === 'failed') {
    return JSON.stringify(preview || {}, null, 2);
  }
  const item = preview.preview || preview;
  const current = String(sandboxField(item, 'current_content', 'CurrentContent') || '');
  const proposed = String(sandboxField(item, 'proposed_content', 'ProposedContent') || '');
  const filePath = String(sandboxField(item, 'file_path', 'FilePath') || '-');
  const currentMissing = Boolean(sandboxField(item, 'current_missing', 'CurrentMissing'));
  const added = sandboxField(item, 'added_lines', 'AddedLines');
  const removed = sandboxField(item, 'removed_lines', 'RemovedLines');
  const diff = String(sandboxField(item, 'unified_diff', 'UnifiedDiff') || '');
  return [
    'preview side-by-side:',
    'file: ' + filePath + (currentMissing ? ' (new file)' : ''),
    'added: ' + String(added == null ? '-' : added) + ' removed: ' + String(removed == null ? '-' : removed),
    '',
    twoColumnText('current', current, 'proposed', proposed, 58, 16),
    '',
    'compact unified diff:',
    diff || '-',
  ].join('\n');
}

function twoColumnText(leftTitle, leftText, rightTitle, rightText, width, maxLines) {
  const left = splitPreviewLines(leftText, maxLines);
  const right = splitPreviewLines(rightText, maxLines);
  const count = Math.max(left.length, right.length, 1);
  const border = repeatChar('-', width) + '-+-' + repeatChar('-', width);
  const rows = [
    padPreviewCell(leftTitle, width) + ' | ' + padPreviewCell(rightTitle, width),
    border,
  ];
  for (let i = 0; i < count; i++) {
    rows.push(padPreviewCell(left[i] || '', width) + ' | ' + padPreviewCell(right[i] || '', width));
  }
  return rows.join('\n');
}

function splitPreviewLines(text, maxLines) {
  const lines = String(text || '').split(/\r?\n/);
  const limit = maxLines || 16;
  const out = lines.slice(0, limit).map((line) => line.length > 56 ? line.slice(0, 53) + '...' : line);
  if (lines.length > limit) {
    out.push('... +' + String(lines.length - limit) + ' more lines');
  }
  if (out.length === 0) out.push('');
  return out;
}

function padPreviewCell(text, width) {
  const value = String(text || '');
  if (value.length >= width) return value.slice(0, width);
  return value + repeatChar(' ', width - value.length);
}

function repeatChar(ch, count) {
  return new Array(Math.max(0, count) + 1).join(ch);
}

async function previewWorkstreamVaultUpdate(encodedUpdate) {
  if (!encodedUpdate) return;
  let payload = {};
  try {
    payload = JSON.parse(decodeURIComponent(encodedUpdate));
  } catch (_) {
    state.ops.workstreamVaultPreviewResult = {status: 'failed', error: 'invalid workstream vault update payload'};
    renderWorkstreamVaultReviews();
    return;
  }
  try {
    const res = await fetch('/viewer/workstreams/vault-updates/preview', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    state.ops.workstreamVaultPreviewResult = res.ok ? data : {status: 'failed', http_status: res.status, response: data};
  } catch (err) {
    state.ops.workstreamVaultPreviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderWorkstreamVaultReviews();
}

async function previewSandboxPromotion(encodedPromotion) {
  if (!encodedPromotion) return;
  let payload = {};
  try {
    payload = JSON.parse(decodeURIComponent(encodedPromotion));
  } catch (_) {
    state.ops.sandboxPromotionPreviewResult = {status: 'failed', error: 'invalid sandbox promotion payload'};
    renderSandboxPromotionPreviewResult();
    return;
  }
  try {
    const res = await fetch('/viewer/sandbox/promotions/preview', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    state.ops.sandboxPromotionPreviewResult = res.ok ? data : {status: 'failed', http_status: res.status, response: data};
  } catch (err) {
    state.ops.sandboxPromotionPreviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderSandboxPromotionPreviewResult();
}

async function requestSandboxPromotionManualReview(encodedPromotion) {
  if (!encodedPromotion) return;
  let promotion = {};
  try {
    promotion = JSON.parse(decodeURIComponent(encodedPromotion));
  } catch (_) {
    state.ops.sandboxPromotionManualReviewResult = {status: 'failed', error: 'invalid sandbox promotion payload'};
    renderSandboxPromotionPreviewResult();
    return;
  }
  try {
    const res = await fetch('/viewer/sandbox/promotions/manual-review', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        promotion,
        workstream_id: sandboxField(promotion, 'workstream_id', 'WorkstreamID') || '',
      }),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    state.ops.sandboxPromotionManualReviewResult = res.ok ? data : {status: 'failed', http_status: res.status, response: data};
    if (res.ok && data && data.preview) {
      state.ops.sandboxPromotionPreviewResult = {preview: data.preview};
    }
  } catch (err) {
    state.ops.sandboxPromotionManualReviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderSandboxPromotionPreviewResult();
}

async function reviewWorkstreamVaultUpdate(encodedUpdate, reviewStatus) {
  if (!encodedUpdate || (reviewStatus !== 'approved' && reviewStatus !== 'rejected')) return;
  let payload = {};
  try {
    payload = JSON.parse(decodeURIComponent(encodedUpdate));
  } catch (_) {
    state.ops.workstreamVaultReviewResult = {status: 'failed', error: 'invalid workstream vault update payload'};
    renderWorkstreamVaultReviews();
    return;
  }
  payload.review_status = reviewStatus;
  try {
    const res = await fetch('/viewer/workstreams/vault-updates/review', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    if (!res.ok) {
      state.ops.workstreamVaultReviewResult = {status: 'failed', http_status: res.status, response: data};
    } else {
      state.ops.workstreamVaultReviewResult = data;
      refreshWorkstreamData();
    }
  } catch (err) {
    state.ops.workstreamVaultReviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderWorkstreamVaultReviews();
}

function revenueOpsCard() {
  const market = Array.isArray(state.ops.revenueMarketResearch) ? state.ops.revenueMarketResearch : [];
  const posts = Array.isArray(state.ops.revenueSNSPostMetrics) ? state.ops.revenueSNSPostMetrics : [];
  const products = Array.isArray(state.ops.revenueProducts) ? state.ops.revenueProducts : [];
  const voices = Array.isArray(state.ops.revenueCustomerVoices) ? state.ops.revenueCustomerVoices : [];
  const events = Array.isArray(state.ops.revenueEvents) ? state.ops.revenueEvents : [];
  const decisions = latestRevenueHumanDecisions(Array.isArray(state.ops.revenueHumanDecisions) ? state.ops.revenueHumanDecisions : []);
  const dailyReports = Array.isArray(state.ops.revenueDailyRoutineReports) ? state.ops.revenueDailyRoutineReports : [];
  const channelDrafts = Array.isArray(state.ops.revenueChannelDrafts) ? state.ops.revenueChannelDrafts : [];
  const summary = state.ops.revenueSummary && typeof state.ops.revenueSummary === 'object' ? state.ops.revenueSummary : null;
  const paid = events.filter((item) => Number(sandboxField(item, 'amount', 'Amount') || 0) > 0).length;
  const usableVoices = voices.filter((item) => Boolean(sandboxField(item, 'usable_for_marketing', 'UsableForMarketing'))).length;
  const pendingDecisions = decisions.filter((item) => String(sandboxField(item, 'approval_status', 'ApprovalStatus') || '') === 'pending' || String(sandboxField(item, 'gate_status', 'GateStatus') || '') === 'needs_review').length;
  const totalRevenue = summary ? Number(sandboxField(summary, 'total_revenue_amount', 'TotalRevenueAmount') || 0) : events.reduce((sum, item) => sum + Math.max(0, Number(sandboxField(item, 'amount', 'Amount') || 0)), 0);
  const paidCustomers = summary ? Number(sandboxField(summary, 'paid_customer_count', 'PaidCustomerCount') || 0) : 0;
  const trend = summary && Array.isArray(sandboxField(summary, 'kpi_trend', 'KPITrend')) ? sandboxField(summary, 'kpi_trend', 'KPITrend') : [];
  const productSales = summary && Array.isArray(sandboxField(summary, 'product_sales', 'ProductSales')) ? sandboxField(summary, 'product_sales', 'ProductSales') : [];
  const voiceTypes = summary && Array.isArray(sandboxField(summary, 'customer_voice_types', 'CustomerVoiceTypes')) ? sandboxField(summary, 'customer_voice_types', 'CustomerVoiceTypes') : [];
  const topProduct = productSales[0] || null;
  const topVoiceType = voiceTypes[0] || null;
  const latestTrend = trend.length ? trend[trend.length - 1] : null;
  const latest = channelDrafts[0] || dailyReports[0] || products[0] || market[0] || null;
  const channelDraftCount = summary ? Number(sandboxField(summary, 'channel_draft_count', 'ChannelDraftCount') || 0) : channelDrafts.length;
  return {
    title: 'Revenue',
    big: String(events.length) + '/' + String(products.length),
    sub: market.length || posts.length || products.length || voices.length || events.length || decisions.length || dailyReports.length || channelDrafts.length ? ('paid events: ' + String(summary ? sandboxField(summary, 'paid_event_count', 'PaidEventCount') : paid) + ' market: ' + String(summary ? sandboxField(summary, 'market_research_count', 'MarketResearchCount') : market.length) + '\nrevenue: ' + String(totalRevenue) + ' paid customers: ' + String(paidCustomers) + '\nvoices usable: ' + String(summary ? sandboxField(summary, 'usable_voice_count', 'UsableVoiceCount') : usableVoices) + '/' + String(summary ? sandboxField(summary, 'customer_voice_count', 'CustomerVoiceCount') : voices.length) + ' posts: ' + String(summary ? sandboxField(summary, 'sns_post_count', 'SNSPostCount') : posts.length) + '\ndaily reports: ' + String(summary ? sandboxField(summary, 'daily_report_count', 'DailyReportCount') : dailyReports.length) + ' channel drafts: ' + String(channelDraftCount) + ' human decisions pending: ' + String(summary ? sandboxField(summary, 'pending_decision_count', 'PendingDecisionCount') : pendingDecisions) + '/' + String(decisions.length) + '\ntrend days: ' + String(trend.length) + ' latest revenue: ' + String(latestTrend ? sandboxField(latestTrend, 'revenue_amount', 'RevenueAmount') : '-') + '\ntop product: ' + String(topProduct ? (sandboxField(topProduct, 'product_name', 'ProductName') || sandboxField(topProduct, 'product_id', 'ProductID')) : '-') + ' voices top: ' + String(topVoiceType ? sandboxField(topVoiceType, 'voice_type', 'VoiceType') : '-') + '\nlatest: ' + String(sandboxField(latest, 'draft_id', 'DraftID') || sandboxField(latest, 'report_id', 'ReportID') || sandboxField(latest, 'product_name', 'ProductName') || sandboxField(latest, 'theme', 'Theme') || '-')) : 'revenue record なし',
  };
}

function latestRevenueHumanDecisions(items) {
  const seen = new Set();
  const out = [];
  items.forEach((item) => {
    const id = String(sandboxField(item, 'decision_id', 'DecisionID') || '');
    const key = id || JSON.stringify(item);
    if (seen.has(key)) return;
    seen.add(key);
    out.push(item);
  });
  return out;
}

function renderRevenueHumanDecisions() {
  const body = document.getElementById('revenueDecisionBody');
  if (!body) return;
  body.innerHTML = '';
  const decisions = latestRevenueHumanDecisions(Array.isArray(state.ops.revenueHumanDecisions) ? state.ops.revenueHumanDecisions : []);
  if (decisions.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No revenue human decisions yet</td>';
    body.appendChild(tr);
    renderRevenueDecisionResult();
    return;
  }
  decisions.slice(0, 20).forEach((item) => {
    const decisionID = String(sandboxField(item, 'decision_id', 'DecisionID') || '');
    const approval = String(sandboxField(item, 'approval_status', 'ApprovalStatus') || '-');
    const gate = String(sandboxField(item, 'gate_status', 'GateStatus') || '-');
    const needsReview = approval === 'pending' || gate === 'needs_review';
    const actions = needsReview && decisionID
      ? '<button class="ctl-btn revenue-decision-review" type="button" data-decision-id="' + escAttr(decisionID) + '" data-approval-status="approved">Approve</button> <button class="ctl-btn revenue-decision-review" type="button" data-decision-id="' + escAttr(decisionID) + '" data-approval-status="rejected">Reject</button>'
      : '<span class="small">-</span>';
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(sandboxField(item, 'created_at', 'CreatedAt'))) + '</td>' +
      '<td class="code">' + esc(decisionID || '-') + '</td>' +
      '<td>' + esc(sandboxField(item, 'decision_type', 'DecisionType') || '-') + '</td>' +
      '<td><span class="badge ' + stateClass(gate) + '">' + esc(approval + ' / ' + gate) + '</span></td>' +
      '<td>' + esc(short(sandboxField(item, 'description', 'Description') || '-', 120)) + '</td>' +
      '<td>' + actions + '</td>';
    body.appendChild(tr);
  });
  body.querySelectorAll('.revenue-decision-review').forEach((btn) => {
    btn.addEventListener('click', () => {
      reviewRevenueHumanDecision(btn.getAttribute('data-decision-id') || '', btn.getAttribute('data-approval-status') || '');
    });
  });
  renderRevenueDecisionResult();
}

function renderRevenueDecisionResult() {
  const el = document.getElementById('revenueDecisionResult');
  if (!el) return;
  const result = state.ops.revenueDecisionReviewResult || null;
  if (!result) {
    el.textContent = 'revenue decision review: -';
    return;
  }
  el.textContent = JSON.stringify(result, null, 2);
}

function renderRevenueChannelDrafts() {
  const body = document.getElementById('revenueChannelDraftBody');
  if (!body) return;
  body.innerHTML = '';
  const drafts = Array.isArray(state.ops.revenueChannelDrafts) ? state.ops.revenueChannelDrafts : [];
  if (drafts.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="7" class="small">No revenue channel drafts yet</td>';
    body.appendChild(tr);
    renderRevenueChannelDraftResult();
    return;
  }
  drafts.slice(0, 20).forEach((item) => {
    const draftID = String(sandboxField(item, 'draft_id', 'DraftID') || '');
    const channel = String(sandboxField(item, 'channel', 'Channel') || '-');
    const approval = String(sandboxField(item, 'approval_status', 'ApprovalStatus') || '-');
    const externalSend = Boolean(sandboxField(item, 'external_send_applied', 'ExternalSendApplied'));
    const sendState = externalSend ? 'sent unexpectedly' : 'draft only';
    const source = sandboxField(item, 'source_report_id', 'SourceReportID') || sandboxField(item, 'source_workstream_id', 'SourceWorkstreamID') || '-';
    const subject = sandboxField(item, 'subject', 'Subject') || '';
    const bodyText = sandboxField(item, 'body', 'Body') || '';
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(sandboxField(item, 'created_at', 'CreatedAt'))) + '</td>' +
      '<td class="code">' + esc(draftID || '-') + '</td>' +
      '<td>' + esc(channel) + '</td>' +
      '<td><span class="badge ' + stateClass(approval) + '">' + esc(approval) + '</span></td>' +
      '<td class="code">' + esc(short(source, 42)) + '</td>' +
      '<td>' + esc(short((subject ? subject + ' / ' : '') + bodyText, 160)) + '</td>' +
      '<td><span class="badge ' + stateClass(externalSend ? 'failed' : 'pending') + '">' + esc(sendState) + '</span></td>';
    body.appendChild(tr);
  });
  renderRevenueChannelDraftResult();
}

function renderRevenueChannelDraftResult() {
  const el = document.getElementById('revenueChannelDraftResult');
  if (!el) return;
  const drafts = Array.isArray(state.ops.revenueChannelDrafts) ? state.ops.revenueChannelDrafts : [];
  const externalSent = drafts.filter((item) => Boolean(sandboxField(item, 'external_send_applied', 'ExternalSendApplied'))).length;
  const pending = drafts.filter((item) => String(sandboxField(item, 'approval_status', 'ApprovalStatus') || '') === 'pending').length;
  el.textContent = 'revenue channel drafts: ' + String(drafts.length) + ' total / ' + String(pending) + ' pending / ' + String(externalSent) + ' external_send_applied';
}

function revenueBar(value, max) {
  const n = Math.max(0, Number(value || 0));
  const limit = Math.max(1, Number(max || 1));
  const width = Math.max(1, Math.min(20, Math.round((n / limit) * 20)));
  return '#'.repeat(width) + '.'.repeat(Math.max(0, 20 - width));
}

function revenueDrilldownLines() {
  const summary = state.ops.revenueSummary && typeof state.ops.revenueSummary === 'object' ? state.ops.revenueSummary : {};
  const trend = Array.isArray(sandboxField(summary, 'kpi_trend', 'KPITrend')) ? sandboxField(summary, 'kpi_trend', 'KPITrend') : [];
  const productSales = Array.isArray(sandboxField(summary, 'product_sales', 'ProductSales')) ? sandboxField(summary, 'product_sales', 'ProductSales') : [];
  const voiceTypes = Array.isArray(sandboxField(summary, 'customer_voice_types', 'CustomerVoiceTypes')) ? sandboxField(summary, 'customer_voice_types', 'CustomerVoiceTypes') : [];
  const decisions = latestRevenueHumanDecisions(Array.isArray(state.ops.revenueHumanDecisions) ? state.ops.revenueHumanDecisions : []);
  const drafts = Array.isArray(state.ops.revenueChannelDrafts) ? state.ops.revenueChannelDrafts : [];
  const maxRevenue = Math.max(1, ...trend.map((item) => Number(sandboxField(item, 'revenue_amount', 'RevenueAmount') || 0)));
  const maxProductRevenue = Math.max(1, ...productSales.map((item) => Number(sandboxField(item, 'revenue_amount', 'RevenueAmount') || 0)));
  const maxVoiceCount = Math.max(1, ...voiceTypes.map((item) => Number(sandboxField(item, 'count', 'Count') || 0)));
  const lines = [
    'Revenue Drilldown',
    'summary: revenue=' + String(sandboxField(summary, 'total_revenue_amount', 'TotalRevenueAmount') || 0) +
      ' paid_customers=' + String(sandboxField(summary, 'paid_customer_count', 'PaidCustomerCount') || 0) +
      ' pending_decisions=' + String(sandboxField(summary, 'pending_decision_count', 'PendingDecisionCount') || 0) +
      ' channel_drafts=' + String(sandboxField(summary, 'channel_draft_count', 'ChannelDraftCount') || drafts.length),
    '',
    'KPI trend graph:',
  ];
  if (trend.length === 0) {
    lines.push('-');
  } else {
    trend.slice(-14).forEach((item) => {
      const day = sandboxField(item, 'date', 'Date') || '-';
      const revenue = Number(sandboxField(item, 'revenue_amount', 'RevenueAmount') || 0);
      const posts = Number(sandboxField(item, 'post_count', 'PostCount') || 0);
      const voices = Number(sandboxField(item, 'voice_count', 'VoiceCount') || 0);
      lines.push(day + ' |' + revenueBar(revenue, maxRevenue) + '| revenue=' + revenue + ' posts=' + posts + ' voices=' + voices);
    });
  }
  lines.push('', 'Product sales graph:');
  if (productSales.length === 0) {
    lines.push('-');
  } else {
    productSales.slice(0, 10).forEach((item) => {
      const name = sandboxField(item, 'product_name', 'ProductName') || sandboxField(item, 'product_id', 'ProductID') || '-';
      const revenue = Number(sandboxField(item, 'revenue_amount', 'RevenueAmount') || 0);
      const sales = Number(sandboxField(item, 'sales_count', 'SalesCount') || 0);
      lines.push(short(name, 28) + ' |' + revenueBar(revenue, maxProductRevenue) + '| revenue=' + revenue + ' sales=' + sales);
    });
  }
  lines.push('', 'Customer voice graph:');
  if (voiceTypes.length === 0) {
    lines.push('-');
  } else {
    voiceTypes.slice(0, 10).forEach((item) => {
      const voiceType = sandboxField(item, 'voice_type', 'VoiceType') || '-';
      const count = Number(sandboxField(item, 'count', 'Count') || 0);
      lines.push(short(voiceType, 28) + ' |' + revenueBar(count, maxVoiceCount) + '| count=' + count);
    });
  }
  lines.push('', 'Decision drilldown:');
  if (decisions.length === 0) {
    lines.push('-');
  } else {
    decisions.slice(0, 8).forEach((item) => {
      lines.push(
        String(sandboxField(item, 'decision_id', 'DecisionID') || '-') +
        ' / ' + String(sandboxField(item, 'decision_type', 'DecisionType') || '-') +
        ' / ' + String(sandboxField(item, 'approval_status', 'ApprovalStatus') || '-') +
        ' / ' + String(sandboxField(item, 'gate_status', 'GateStatus') || '-')
      );
    });
  }
  return lines;
}

function renderRevenueDrilldown() {
  const el = document.getElementById('revenueDrilldownResult');
  if (!el) return;
  el.textContent = revenueDrilldownLines().join('\n');
}

async function reviewRevenueHumanDecision(decisionID, approvalStatus) {
  if (!decisionID || (approvalStatus !== 'approved' && approvalStatus !== 'rejected')) return;
  const payload = {decision_id: decisionID, approval_status: approvalStatus};
  try {
    const res = await fetch('/viewer/revenue/human-decision-gate/review', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    if (!res.ok) {
      state.ops.revenueDecisionReviewResult = {status: 'failed', http_status: res.status, response: data};
    } else {
      state.ops.revenueDecisionReviewResult = data;
      refreshRevenueData();
    }
  } catch (err) {
    state.ops.revenueDecisionReviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderRevenueHumanDecisions();
}

function personaObservationOpsCard() {
  const discomforts = Array.isArray(state.ops.personaDiscomfortLogs) ? state.ops.personaDiscomfortLogs : [];
  const triggers = Array.isArray(state.ops.personaTriggerLogs) ? state.ops.personaTriggerLogs : [];
  const canonicals = Array.isArray(state.ops.personaCanonicalResponseLogs) ? state.ops.personaCanonicalResponseLogs : [];
  const observations = Array.isArray(state.ops.personaObservationLogs) ? state.ops.personaObservationLogs : [];
  const metaUpdates = latestPersonaMetaProfileUpdates(Array.isArray(state.ops.personaMetaProfileUpdates) ? state.ops.personaMetaProfileUpdates : []);
  const sessions = Array.isArray(state.ops.personaInterfaceSessions) ? state.ops.personaInterfaceSessions : [];
  const pending = observations.filter((item) => String(sandboxField(item, 'review_status', 'ReviewStatus') || '') === 'pending').length;
  const pendingMeta = metaUpdates.filter((item) => String(sandboxField(item, 'review_status', 'ReviewStatus') || '') === 'pending').length;
  const latest = discomforts[0] || observations[0] || metaUpdates[0] || triggers[0] || null;
  return {
    title: 'Persona Observation',
    big: String(observations.length) + '/' + String(discomforts.length),
    sub: discomforts.length || triggers.length || canonicals.length || observations.length || metaUpdates.length || sessions.length ? ('review pending: ' + String(pending) + ' meta pending: ' + String(pendingMeta) + '\ncanonical: ' + String(canonicals.length) + ' sessions: ' + String(sessions.length) + '\nlatest: ' + String(sandboxField(latest, 'character_id', 'CharacterID') || sandboxField(latest, 'target_id', 'TargetID') || '-')) : 'persona observation record なし',
  };
}

function latestPersonaMetaProfileUpdates(items) {
  const seen = new Set();
  const out = [];
  items.forEach((item) => {
    const id = String(sandboxField(item, 'update_id', 'UpdateID') || '');
    const key = id || JSON.stringify(item);
    if (seen.has(key)) return;
    seen.add(key);
    out.push(item);
  });
  return out;
}

function renderPersonaMetaReviews() {
  const body = document.getElementById('personaMetaReviewBody');
  if (!body) return;
  body.innerHTML = '';
  const updates = latestPersonaMetaProfileUpdates(Array.isArray(state.ops.personaMetaProfileUpdates) ? state.ops.personaMetaProfileUpdates : []);
  if (updates.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td colspan="6" class="small">No persona meta updates yet</td>';
    body.appendChild(tr);
    renderPersonaMetaReviewResult();
    return;
  }
  updates.slice(0, 20).forEach((item) => {
    const updateID = String(sandboxField(item, 'update_id', 'UpdateID') || '');
    const review = String(sandboxField(item, 'review_status', 'ReviewStatus') || '-');
    const target = String(sandboxField(item, 'target_id', 'TargetID') || '-');
    const observer = String(sandboxField(item, 'observer_id', 'ObserverID') || '-');
    const proposed = String(sandboxField(item, 'proposed_content', 'ProposedContent') || '');
    const pending = review === 'pending';
    const payload = encodeURIComponent(JSON.stringify(item));
    const actions = pending && updateID
      ? '<button class="ctl-btn persona-meta-review" type="button" data-update="' + escAttr(payload) + '" data-review-status="approved">Approve</button> <button class="ctl-btn persona-meta-review" type="button" data-update="' + escAttr(payload) + '" data-review-status="rejected">Reject</button>'
      : '<span class="small">-</span>';
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(ftime(sandboxField(item, 'created_at', 'CreatedAt'))) + '</td>' +
      '<td class="code">' + esc(updateID || '-') + '</td>' +
      '<td>' + esc(observer + ' → ' + target) + '</td>' +
      '<td><span class="badge ' + stateClass(review) + '">' + esc(review) + '</span></td>' +
      '<td class="code">' + esc(proposed ? short(proposed, 120) : '-') + '</td>' +
      '<td>' + actions + '</td>';
    body.appendChild(tr);
  });
  body.querySelectorAll('.persona-meta-review').forEach((btn) => {
    btn.addEventListener('click', () => {
      reviewPersonaMetaUpdate(btn.getAttribute('data-update') || '', btn.getAttribute('data-review-status') || '');
    });
  });
  renderPersonaMetaReviewResult();
}

function renderPersonaMetaReviewResult() {
  const el = document.getElementById('personaMetaReviewResult');
  if (!el) return;
  const result = state.ops.personaMetaReviewResult || null;
  if (!result) {
    el.textContent = 'persona meta review: -';
    return;
  }
  el.textContent = JSON.stringify(result, null, 2);
}

async function reviewPersonaMetaUpdate(encodedUpdate, reviewStatus) {
  if (!encodedUpdate || (reviewStatus !== 'approved' && reviewStatus !== 'rejected')) return;
  let payload = {};
  try {
    payload = JSON.parse(decodeURIComponent(encodedUpdate));
  } catch (_) {
    state.ops.personaMetaReviewResult = {status: 'failed', error: 'invalid persona meta update payload'};
    renderPersonaMetaReviews();
    return;
  }
  payload.review_status = reviewStatus;
  try {
    const res = await fetch('/viewer/persona-observation/meta-updates/review', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    if (!res.ok) {
      state.ops.personaMetaReviewResult = {status: 'failed', http_status: res.status, response: data};
    } else {
      state.ops.personaMetaReviewResult = data;
      refreshPersonaObservationData();
    }
  } catch (err) {
    state.ops.personaMetaReviewResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderPersonaMetaReviews();
}

function browserTraceAPIOpsCard() {
  const runs = Array.isArray(state.ops.browserTraceRuns) ? state.ops.browserTraceRuns : [];
  const candidates = Array.isArray(state.ops.browserTraceAPICandidates) ? state.ops.browserTraceAPICandidates : [];
  const schemas = Array.isArray(state.ops.browserTraceAPISchemas) ? state.ops.browserTraceAPISchemas : [];
  const coverage = Array.isArray(state.ops.browserTraceAPICoverageReports) ? state.ops.browserTraceAPICoverageReports : [];
  const artifacts = Array.isArray(state.ops.browserTraceAPIArtifacts) ? state.ops.browserTraceAPIArtifacts : [];
  const auth = candidates.filter((item) => Boolean(sandboxField(item, 'auth_required', 'AuthRequired'))).length;
  const fetcherProposals = artifacts.filter((item) => String(sandboxField(item, 'artifact_type', 'Type') || '') === 'fetcher_proposal').length;
  const latest = candidates[0] || runs[0] || null;
  return {
    title: 'Browser Trace API',
    big: String(candidates.length) + '/' + String(runs.length),
    sub: runs.length || candidates.length || schemas.length || coverage.length || artifacts.length ? ('auth candidates: ' + String(auth) + ' schemas: ' + String(schemas.length) + '\ncoverage reports: ' + String(coverage.length) + ' fetcher proposals: ' + String(fetcherProposals) + '\nlatest: ' + String(sandboxField(latest, 'path_template', 'PathTemplate') || sandboxField(latest, 'trace_run_id', 'TraceRunID') || '-')) : 'browser trace api record なし',
  };
}

async function requestBrowserTraceAPIFetcherProposal(candidateID, workstreamID) {
  if (!candidateID) return;
  try {
    const res = await fetch('/viewer/browser-trace-api/fetcher-proposals', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        candidate_id: candidateID,
        workstream_id: workstreamID || '',
        human_approved: true,
      }),
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : {};
    } catch (_) {
      data = {raw: text};
    }
    state.ops.browserTraceAPIFetcherProposalResult = res.ok ? data : {status: 'failed', http_status: res.status, response: data};
  } catch (err) {
    state.ops.browserTraceAPIFetcherProposalResult = {status: 'failed', error: String(err && err.message ? err.message : err)};
  }
  renderOps();
}

function complexityHotspotOpsCard() {
  const scans = Array.isArray(state.ops.complexityScans) ? state.ops.complexityScans : [];
  const hotspots = Array.isArray(state.ops.complexityHotspots) ? state.ops.complexityHotspots : [];
  const evidence = Array.isArray(state.ops.complexityEvidence) ? state.ops.complexityEvidence : [];
  const highRisk = hotspots.filter((item) => String(sandboxField(item, 'risk_level', 'RiskLevel') || '') === 'high').length;
  const latest = hotspots[0] || scans[0] || null;
  return {
    title: 'Complexity Hotspots',
    big: String(hotspots.length) + '/' + String(scans.length),
    sub: scans.length || hotspots.length || evidence.length ? ('high risk: ' + String(highRisk) + ' evidence: ' + String(evidence.length) + '\nlatest: ' + String(sandboxField(latest, 'hotspot_type', 'HotspotType') || sandboxField(latest, 'scan_id', 'ScanID') || '-') + '\nmode: report-only') : 'complexity hotspot record なし',
  };
}

function superAgentOpsCard() {
  const runs = Array.isArray(state.ops.superAgentRuns) ? state.ops.superAgentRuns : [];
  const tasks = Array.isArray(state.ops.superAgentSubagentTasks) ? state.ops.superAgentSubagentTasks : [];
  const contexts = Array.isArray(state.ops.superAgentContextPacks) ? state.ops.superAgentContextPacks : [];
  const channels = Array.isArray(state.ops.superAgentMessageChannels) ? state.ops.superAgentMessageChannels : [];
  const events = Array.isArray(state.ops.superAgentTraceEvents) ? state.ops.superAgentTraceEvents : [];
  const queue = Array.isArray(state.ops.superAgentRunQueue) ? state.ops.superAgentRunQueue : [];
  const running = runs.filter((item) => String(sandboxField(item, 'status', 'Status') || '') === 'running').length;
  const queued = queue.filter((item) => String(sandboxField(item, 'status', 'Status') || '') === 'queued').length;
  const latest = runs[0] || tasks[0] || events[0] || null;
  return {
    title: 'SuperAgent Harness',
    big: String(runs.length) + '/' + String(tasks.length),
    sub: runs.length || tasks.length || contexts.length || channels.length || events.length || queue.length ? ('running: ' + String(running) + ' context packs: ' + String(contexts.length) + '\nchannels: ' + String(channels.length) + ' trace events: ' + String(events.length) + '\nrun queue: ' + String(queue.length) + ' queued: ' + String(queued) + '\nlatest: ' + String(sandboxField(latest, 'run_id', 'RunID') || sandboxField(latest, 'subagent_id', 'SubagentID') || '-')) : 'superagent harness record なし',
  };
}

function heavyWorkerRuntimeOpsCard() {
  const diag = state.ops.heavyWorkerRuntimeDiagnostics || null;
  if (!diag) {
    return {
      title: 'Heavy Runtime',
      big: '-',
      sub: 'runtime diagnostics 未取得',
    };
  }
  const ops = diag.llm_ops || {};
  const live = ops.live_available ? 'live' : 'config';
  const stateInfo = ops.role_state || {};
  const memory = ops.memory || {};
  const health = stateInfo.health_ok === true ? 'healthy' : (stateInfo.health_ok === false ? 'unhealthy' : live);
  const model = memory.model || diag.model || '-';
  const pid = memory.pid == null ? 'pid -' : ('pid ' + String(memory.pid));
  const base = memory.port ? replaceURLPort(diag.base_url, memory.port) : (diag.base_url || '-');
  const error = ops.error ? '\n' + String(ops.error) : '';
  return {
    title: 'Heavy Runtime',
    big: health,
    sub: 'route: ' + (diag.route || 'ANALYZE') + ' ' + (diag.route_prefix || '/analyze') +
      '\n' + String(model) + ' · ' + pid +
      '\n' + String(base) + error,
  };
}

function knowledgeMemoryOpsCard() {
  const personal = Array.isArray(state.ops.knowledgePersonalArchive) ? state.ops.knowledgePersonalArchive : [];
  const creative = Array.isArray(state.ops.knowledgeCreativeItems) ? state.ops.knowledgeCreativeItems : [];
  const news = Array.isArray(state.ops.knowledgeNewsItems) ? state.ops.knowledgeNewsItems : [];
  const intake = Array.isArray(state.ops.knowledgeDailyIntakeRules) ? state.ops.knowledgeDailyIntakeRules : [];
  const temporal = Array.isArray(state.ops.knowledgeTemporalMarkers) ? state.ops.knowledgeTemporalMarkers : [];
  const dreams = Array.isArray(state.ops.knowledgeDreamRuns) ? state.ops.knowledgeDreamRuns : [];
  const pendingDreams = dreams.filter((item) => String(sandboxField(item, 'review_status', 'ReviewStatus') || '') === 'pending').length;
  const latest = personal[0] || creative[0] || news[0] || intake[0] || null;
  return {
    title: 'Knowledge Memory',
    big: String(personal.length) + '/' + String(creative.length),
    sub: personal.length || creative.length || news.length || intake.length || temporal.length || dreams.length ? ('daily intake: ' + String(intake.length) + ' news: ' + String(news.length) + '\ntemporal: ' + String(temporal.length) + ' dream pending: ' + String(pendingDreams) + '\nlatest: ' + String(sandboxField(latest, 'title', 'Title') || sandboxField(latest, 'topic', 'Topic') || sandboxField(latest, 'entry_id', 'EntryID') || '-')) : 'knowledge memory record なし',
  };
}

function renderKnowledgeMemoryDetailFocus(focusBody) {
  const candidates = knowledgeMemoryDetailCandidates();
  const controls = candidates.map((candidate) => {
    return '<button class="ctl-btn" onclick="fetchKnowledgeMemoryDetail(&quot;' + esc(candidate.type) + '&quot;,&quot;' + esc(candidate.id) + '&quot;)">' + esc(candidate.label) + '</button>';
  }).join(' ');
  const detail = state.ops.knowledgeMemoryDetail || null;
  const tr = document.createElement('tr');
  tr.innerHTML = '<td>Knowledge Memory Detail</td><td>' + (controls || '<span class="small">detail候補なし</span>') + '</td>';
  focusBody.appendChild(tr);
  const detailRow = document.createElement('tr');
  detailRow.innerHTML = '<td>Knowledge Detail Result</td><td class="code">' + esc(detail ? JSON.stringify(detail, null, 2) : '-') + '</td>';
  focusBody.appendChild(detailRow);
}

function knowledgeMemoryDetailCandidates() {
  const out = [];
  const personal = Array.isArray(state.ops.knowledgePersonalArchive) ? state.ops.knowledgePersonalArchive : [];
  const creative = Array.isArray(state.ops.knowledgeCreativeItems) ? state.ops.knowledgeCreativeItems : [];
  const news = Array.isArray(state.ops.knowledgeNewsItems) ? state.ops.knowledgeNewsItems : [];
  const intake = Array.isArray(state.ops.knowledgeDailyIntakeRules) ? state.ops.knowledgeDailyIntakeRules : [];
  const temporal = Array.isArray(state.ops.knowledgeTemporalMarkers) ? state.ops.knowledgeTemporalMarkers : [];
  const dreams = Array.isArray(state.ops.knowledgeDreamRuns) ? state.ops.knowledgeDreamRuns : [];
  if (personal[0]) out.push({type: 'personal_archive', id: String(sandboxField(personal[0], 'entry_id', 'EntryID') || ''), label: 'Personal'});
  if (creative[0]) out.push({type: 'creative_knowledge', id: String(sandboxField(creative[0], 'item_id', 'ItemID') || ''), label: 'Creative'});
  if (news[0]) out.push({type: 'news_knowledge', id: String(sandboxField(news[0], 'item_id', 'ItemID') || ''), label: 'News'});
  if (intake[0]) out.push({type: 'daily_intake_rule', id: String(sandboxField(intake[0], 'rule_id', 'RuleID') || ''), label: 'Intake'});
  if (temporal[0]) out.push({type: 'temporal_marker', id: String(sandboxField(temporal[0], 'marker_id', 'MarkerID') || ''), label: 'Temporal'});
  if (dreams[0]) out.push({type: 'dream_run', id: String(sandboxField(dreams[0], 'run_id', 'RunID') || ''), label: 'Dream'});
  return out.filter((candidate) => candidate.id);
}

function fetchKnowledgeMemoryDetail(detailType, id) {
  const type = String(detailType || '').trim();
  const detailID = String(id || '').trim();
  if (!type || !detailID) return;
  fetch('/viewer/knowledge-memory?detail_type=' + encodeURIComponent(type) + '&id=' + encodeURIComponent(detailID) + '&limit=100')
    .then((r) => {
      if (!r.ok) throw new Error('knowledge memory detail fetch failed');
      return r.json();
    })
    .then((data) => {
      state.ops.knowledgeMemoryDetail = data;
      renderOps();
    })
    .catch((err) => {
      state.ops.knowledgeMemoryDetail = {error: String(err && err.message ? err.message : err), detail_type: type, id: detailID};
      renderOps();
      console.error(err);
    });
}

let dciSearchBound = false;
function bindDCISearchControls() {
  if (dciSearchBound) return;
  dciSearchBound = true;
  const input = document.getElementById('dciSearchInput');
  const button = document.getElementById('dciSearchBtn');
  if (!input || !button) return;
  const run = () => {
    const query = String(input.value || '').trim();
    if (!query) {
      state.ops.dciLastResult = {pack: {query: ''}, trace: {status: 'query_required'}};
      renderDCISearchResult();
      return;
    }
    button.disabled = true;
    fetch('/viewer/dci/search', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({query}),
    })
      .then((r) => {
        if (!r.ok) throw new Error('dci search failed');
        return r.json();
      })
      .then((data) => {
        state.ops.dciLastResult = data;
        renderDCISearchResult();
        refreshDCIData();
      })
      .catch((err) => {
        state.ops.dciLastResult = {pack: {query}, trace: {status: 'failed', error_message: String(err && err.message ? err.message : err)}};
        renderDCISearchResult();
      })
      .finally(() => {
        button.disabled = false;
      });
  };
  button.addEventListener('click', run);
  input.addEventListener('keydown', (ev) => {
    if (ev.key === 'Enter') run();
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
  if (typeof syncChatRouteAliasesFromRuntimeConfig === 'function') {
    syncChatRouteAliasesFromRuntimeConfig(state.ops.localLLM);
  }
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
    llmRuntimeRoleRow('Chat', localLLM.chat_model, localLLM.chat_base_url, ''),
    llmRuntimeRoleRow('Worker', localLLM.worker_model, localLLM.worker_base_url, ''),
    llmRuntimeRoleRow('Heavy', localLLM.heavy_model, localLLM.heavy_base_url,
      sameLocalLLMEndpoint(localLLM.heavy_base_url, localLLM.worker_base_url, localLLM.heavy_model, localLLM.worker_model) ? 'shared' : ''),
    llmRuntimeRoleRow('Wild', localLLM.wild_model, localLLM.wild_base_url,
      sameLocalLLMEndpoint(localLLM.wild_base_url, localLLM.chat_base_url, localLLM.wild_model, localLLM.chat_model) ? 'shared' : ''),
  ].filter((row) => row.model || row.url);
  const params = [
    localLLM.provider ? 'provider=' + localLLM.provider : '',
    localLLM.timeout_sec ? 'timeout=' + localLLM.timeout_sec + 's' : '',
    localLLM.global_concurrency ? 'global=' + localLLM.global_concurrency : '',
    localLLM.model_concurrency ? 'model=' + localLLM.model_concurrency : '',
  ].filter(Boolean).join(' · ');
  el.innerHTML = rows.map((row) => (
    '<div class="llm-runtime-card">' +
      '<div class="ops-card-title">' + esc(row.role) + '<span class="badge ' + stateClass(row.stateClass || row.state) + '">' + esc(row.state) + '</span></div>' +
      '<div class="llm-runtime-model">' + esc(row.model || '-') + '</div>' +
      '<div class="llm-runtime-url">' + esc(row.url || '-') + '/v1/chat/completions</div>' +
      (row.meta ? '<div class="ops-sub">' + esc(row.meta) + '</div>' : '') +
    '</div>'
  )).join('') + (params ? '<div class="ops-sub">' + esc(params) + '</div>' : '');
}

function llmRuntimeRoleRow(role, configModel, configURL, configuredState) {
  const status = state.ops.llmStatus || {};
  const roleState = status.roles && status.roles[role] ? status.roles[role] : null;
  const memoryRole = status.memory && status.memory.llm_by_role && status.memory.llm_by_role[role]
    ? status.memory.llm_by_role[role]
    : null;
  const livePort = memoryRole && memoryRole.port != null ? Number(memoryRole.port) : null;
  const liveURL = Number.isFinite(livePort) ? replaceURLPort(configURL, livePort) : '';
  const liveModel = memoryRole && memoryRole.model ? String(memoryRole.model) : '';
  const pid = memoryRole && memoryRole.pid != null ? 'pid ' + String(memoryRole.pid) : '';
  let runtimeState = configuredState || 'configured';
  let stateClassName = configuredState === 'shared' ? 'thinking' : 'offline';

  if (roleState) {
    if (roleState.halted) {
      runtimeState = 'halted';
      stateClassName = 'error';
    } else if (roleState.health_ok === false) {
      runtimeState = 'unhealthy';
      stateClassName = 'error';
    } else if (roleState.health_ok === true || pid) {
      runtimeState = 'running';
      stateClassName = 'running';
    }
  } else if (memoryRole && memoryRole.pid != null) {
    runtimeState = 'running';
    stateClassName = 'running';
  }

  return {
    role,
    model: liveModel || configModel,
    url: liveURL || configURL,
    state: runtimeState,
    stateClass: stateClassName,
    meta: pid,
  };
}

function replaceURLPort(rawURL, port) {
  const text = String(rawURL || '').trim();
  if (!text) return 'http://127.0.0.1:' + String(port);
  try {
    const parsed = new URL(text);
    parsed.port = String(port);
    return parsed.toString().replace(/\/+$/, '');
  } catch (_) {
    return text.replace(/:\d+(\/.*)?$/, ':' + String(port));
  }
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
