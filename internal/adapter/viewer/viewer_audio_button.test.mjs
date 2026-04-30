import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

class FakeClassList {
  constructor() {
    this.values = new Set();
  }
  add(...names) {
    names.forEach((name) => this.values.add(name));
  }
  remove(...names) {
    names.forEach((name) => this.values.delete(name));
  }
  contains(name) {
    return this.values.has(name);
  }
  toggle(name, force) {
    const enabled = force === undefined ? !this.values.has(name) : Boolean(force);
    if (enabled) this.values.add(name);
    else this.values.delete(name);
    return enabled;
  }
}

class FakeElement {
  constructor(id = '') {
    this.id = id;
    this.children = [];
    this.classList = new FakeClassList();
    this.dataset = {};
    this.style = {};
    this.attributes = {};
    this.listeners = {};
    this.textContent = '';
    this.innerHTML = '';
    this.title = '';
  }
  set className(value) {
    this._className = String(value || '');
    this.classList = new FakeClassList();
    this._className.split(/\s+/).filter(Boolean).forEach((name) => this.classList.add(name));
  }
  get className() {
    return this._className || '';
  }
  appendChild(child) {
    this.children.push(child);
    return child;
  }
  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }
  getAttribute(name) {
    return this.attributes[name] || '';
  }
  addEventListener(type, fn) {
    this.listeners[type] = fn;
  }
  querySelector(selector) {
    if (selector === '.mc') {
      if (!this._mc) this._mc = new FakeElement('mc');
      return this._mc;
    }
    return null;
  }
  click() {
    if (this.listeners.click) return this.listeners.click({preventDefault() {}});
  }
  remove() {}
  removeAttribute(name) {
    delete this.attributes[name];
  }
  scrollIntoView() {}
}

class FakeAudio {
  constructor() {
    this.listeners = {};
    this.dataset = {};
    this.readyState = 4;
    this.currentTime = 0;
    this.muted = false;
    this.preload = '';
    this.src = '';
    this.paused = true;
  }
  addEventListener(type, fn) {
    this.listeners[type] = fn;
  }
  play() {
    this.paused = false;
    return Promise.resolve();
  }
  pause() {
    this.paused = true;
  }
  load() {}
  removeAttribute(name) {
    if (name === 'src') this.src = '';
  }
}

function loadAudioHarness() {
  const html = fs.readFileSync('internal/adapter/viewer/viewer.html', 'utf8');
  const start = html.indexOf('const ttsPlayback = {');
  const end = html.indexOf('let sending = false;');
  assert.ok(start > 0, 'ttsPlayback block not found');
  assert.ok(end > start, 'audio handler block end not found');
  const source = html.slice(start, end) + `
globalThis.__viewerAudioHarness = {
  ttsPlayback,
  updateAudioButton,
  enqueueTTSAudio,
};
`;

  const elements = new Map();
  const document = {
    createElement: (tag) => new FakeElement(tag),
    addEventListener: () => {},
    getElementById: (id) => {
      if (!elements.has(id)) elements.set(id, new FakeElement(id));
      return elements.get(id);
    },
    querySelector: () => new FakeElement('main'),
    querySelectorAll: () => [],
  };
  const timers = [];
  const context = {
    document,
    console: {error() {}},
    window: {addEventListener() {}, location: {protocol: 'http:'}, history: null},
    HTMLMediaElement: {HAVE_CURRENT_DATA: 2},
    Audio: FakeAudio,
    MAX_TIMELINE_NODES: 400,
    mainEl: document.querySelector('main'),
    chat: document.getElementById('chat'),
    ctr: document.getElementById('ctr'),
    cnt: document.getElementById('cnt'),
    latestBtn: document.getElementById('latestBtn'),
    toastEl: document.getElementById('toast'),
    thinkingBubbles: {},
    setTimeout: (fn, _ms) => {
      timers.push(fn);
      return timers.length;
    },
    setInterval: () => 0,
    clearTimeout: () => {},
    clearLipSyncSpeaking() {},
    setLipSyncSpeaking() {},
    scrollToBottom() {},
    ftime: () => '12:00:00',
    ag: (id) => ({c: id === 'shiro' ? '#22d3ee' : '#f472b6', l: id || 'mio', e: ''}),
  };
  vm.createContext(context);
  vm.runInContext(source, context);
  return {harness: context.__viewerAudioHarness, elements, timers};
}

test('speaker button can turn ready audio off without stopping central chat fallback', async () => {
  const {harness, elements, timers} = loadAudioHarness();
  const audioBtn = elements.get('audioBtn');

  harness.ttsPlayback.unlocked = true;
  harness.ttsPlayback.blocked = false;
  harness.updateAudioButton();
  assert.equal(audioBtn.getAttribute('aria-label'), '音声は有効です');

  await audioBtn.click();

  assert.equal(harness.ttsPlayback.audioEnabled, false);
  assert.equal(harness.ttsPlayback.playing, false);
  assert.equal(audioBtn.getAttribute('aria-label'), '音声はOFFです。タップしてON');
  assert.equal(audioBtn.textContent, '🔇');

  harness.enqueueTTSAudio('/audio/a.wav', 'mio', 'session-1', 'default', 0, 'speech', '中央表示です。', '', 'u1');
  assert.equal(harness.ttsPlayback.queue.length, 0);
  assert.equal(harness.ttsPlayback.fallbackActive, true);
  assert.equal(elements.get('chat').children.at(-1)._mc.textContent, '中央表示です。');

  timers.shift()();
  assert.equal(harness.ttsPlayback.fallbackActive, false);
});
