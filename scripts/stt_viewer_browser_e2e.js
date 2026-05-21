#!/usr/bin/env node
const { chromium } = require('playwright');
const path = require('path');

function parseArgs(argv) {
  const args = {
    url: 'http://127.0.0.1:18790/viewer?tab=timeline',
    wav: 'tmp/client_stt_input_latest.wav',
    realMic: false,
    requireFinal: true,
    requireSend: true,
    speakMs: 0,
    partialTimeoutMs: 30000,
    finalTimeoutMs: 70000,
    headless: true,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const a = argv[i];
    if (a === '--url') args.url = argv[++i];
    else if (a === '--wav') args.wav = argv[++i];
    else if (a === '--real-mic') args.realMic = true;
    else if (a === '--no-require-final') args.requireFinal = false;
    else if (a === '--no-require-send') args.requireSend = false;
    else if (a === '--speak-ms') args.speakMs = Number(argv[++i]) || 0;
    else if (a === '--partial-timeout-ms') args.partialTimeoutMs = Number(argv[++i]) || args.partialTimeoutMs;
    else if (a === '--final-timeout-ms') args.finalTimeoutMs = Number(argv[++i]) || args.finalTimeoutMs;
    else if (a === '--headed') args.headless = false;
    else if (a === '--headless') args.headless = true;
    else throw new Error(`unknown arg: ${a}`);
  }
  return args;
}

function originOf(url) {
  const u = new URL(url);
  return `${u.protocol}//${u.host}`;
}

async function waitOrNull(page, fn, timeout) {
  try {
    await page.waitForFunction(fn, null, { timeout });
    return true;
  } catch (_) {
    return false;
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const repo = process.cwd();
  const launchArgs = ['--use-fake-ui-for-media-stream', '--autoplay-policy=no-user-gesture-required'];
  if (!args.realMic) {
    launchArgs.push('--use-fake-device-for-media-stream');
    launchArgs.push(`--use-file-for-fake-audio-capture=${path.resolve(repo, args.wav)}`);
  }

  const browser = await chromium.launch({ headless: args.headless, args: launchArgs });
  const page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
  await page.context().grantPermissions(['microphone'], { origin: originOf(args.url) });

  const wsSent = [];
  const wsRecv = [];
  const consoleLines = [];
  page.on('websocket', ws => {
    ws.on('framesent', frame => wsSent.push(frame.payload));
    ws.on('framereceived', frame => wsRecv.push(frame.payload));
  });
  page.on('console', msg => consoleLines.push(`${msg.type()}: ${msg.text()}`));
  page.on('pageerror', err => consoleLines.push(`pageerror: ${err.message}`));

  let sendBody = '';
  await page.route('**/viewer/send', route => {
    sendBody = route.request().postData() || '';
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });

  await page.goto(args.url, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('body[data-viewer-tab="timeline"]', { timeout: 10000 });
  await page.click('#micBtn');
  await page.waitForFunction(() => document.querySelector('#micState')?.textContent?.includes('on'), null, { timeout: 10000 });
  const sawPartial = await waitOrNull(page, () => (document.querySelector('#sttCaption')?.textContent || '').includes('暫定字幕:'), args.partialTimeoutMs);
  const partialCaption = await page.textContent('#sttCaption').catch(() => '');

  if (args.realMic && args.speakMs <= 0) {
    await page.pause();
  } else if (args.speakMs > 0) {
    await page.waitForTimeout(args.speakMs);
  }

  await page.click('#micBtn');
  const sawFinal = await waitOrNull(page, () => (document.querySelector('#sttCaption')?.textContent || '').includes('確定字幕:'), args.finalTimeoutMs);
  const finalCaption = await page.textContent('#sttCaption').catch(() => '');
  const deadline = Date.now() + 15000;
  while (!sendBody && Date.now() < deadline) await page.waitForTimeout(200);

  const sentText = wsSent.filter(p => typeof p === 'string');
  const recvText = wsRecv.filter(p => typeof p === 'string').join('\n');
  const result = {
    ok: true,
    url: args.url,
    real_mic: args.realMic,
    saw_partial: sawPartial,
    saw_final: sawFinal,
    partial_caption: partialCaption,
    final_caption: finalCaption,
    sent_start: sentText.some(p => p.includes('"type":"start"')),
    sent_stop: sentText.some(p => p.includes('"type":"stop"')),
    sent_binary: wsSent.some(p => typeof p !== 'string'),
    recv_partial: recvText.includes('"type":"partial"') || recvText.includes('"type":"draft"') || sawPartial,
    recv_final: recvText.includes('"type":"final"'),
    chat_send_observed: Boolean(sendBody),
    send_message: sendBody ? JSON.parse(sendBody).message || '' : '',
    session: await page.textContent('#sttSessionState').catch(() => ''),
    mic: await page.textContent('#micState').catch(() => ''),
    conn: await page.textContent('#sttConnState').catch(() => ''),
    recent_console: consoleLines.slice(-20),
  };

  const failures = [];
  if (!result.sent_start) failures.push('missing start control');
  if (!result.sent_binary) failures.push('missing binary PCM chunks');
  if (!result.sent_stop) failures.push('missing stop control');
  if (args.requireFinal && !result.recv_final) failures.push('missing final event');
  if (args.requireFinal && !result.saw_final) failures.push('missing final caption');
  if (args.requireSend && !result.chat_send_observed) failures.push('missing /viewer/send');
  if (args.requireSend && !String(result.send_message || '').trim()) failures.push('empty send message');
  if (failures.length > 0) {
    result.ok = false;
    result.failures = failures;
  }

  console.log(JSON.stringify(result, null, 2));
  await browser.close();
  process.exit(result.ok ? 0 : 2);
}

main().catch(err => {
  console.error(err && err.stack ? err.stack : err);
  process.exit(1);
});
