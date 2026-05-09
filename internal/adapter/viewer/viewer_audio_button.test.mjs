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
  const js = fs.readFileSync('internal/adapter/viewer/assets/js/viewer.js', 'utf8');
  const start = js.indexOf('const ttsPlayback = {');
  const end = js.indexOf('let sending = false;');
  assert.ok(start > 0, 'ttsPlayback block not found');
  assert.ok(end > start, 'audio handler block end not found');
  const source = js.slice(start, end) + `
globalThis.__viewerAudioHarness = {
  ttsPlayback,
  updateAudioButton,
  enqueueTTSAudio,
  toggleTTSAudio,
  setCentralTTSSpeechText,
  chatAudioSync,
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
    idleLiveLog: document.getElementById('idleLiveLog'),
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

test('live mode audio button mirrors state and unlocks audio', async () => {
  const {harness, elements} = loadAudioHarness();
  const audioBtn = elements.get('audioBtn');
  const liveAudioBtn = elements.get('liveAudioBtn');

  harness.updateAudioButton();
  assert.equal(audioBtn.getAttribute('aria-label'), '音声を有効化');
  assert.equal(liveAudioBtn.getAttribute('aria-label'), '音声を有効化');

  await liveAudioBtn.click();

  assert.equal(harness.ttsPlayback.audioEnabled, true);
  assert.equal(harness.ttsPlayback.unlocked, true);
  assert.equal(audioBtn.getAttribute('aria-label'), '音声は有効です');
  assert.equal(liveAudioBtn.getAttribute('aria-label'), '音声は有効です');
  assert.ok(liveAudioBtn.classList.contains('ready'));
});

test('tts chunk is shown when audio play resolves even if media events are missed', async () => {
  const {harness, elements} = loadAudioHarness();

  harness.enqueueTTSAudio('/audio/tail.wav', 'mio', 'session-tail', 'default', 7, '末尾の音声です。', '末尾の表示です。', '', 'tail-7');
  await Promise.resolve();

  assert.equal(harness.ttsPlayback.playing, true);
  assert.equal(elements.get('chat').children.at(-1)._mc.textContent, '末尾の表示です。');
});

test('idlechat waits for two chunks before starting audio sync', async () => {
  const {harness, elements} = loadAudioHarness();

  harness.enqueueTTSAudio('/audio/idle-0.wav', 'mio', 'idle-session-1', 'default', 0, '最初です。', '最初です。', '', 'idle-0');
  await Promise.resolve();

  assert.equal(harness.ttsPlayback.playing, false);
  assert.equal(harness.ttsPlayback.queue.length, 1);
  assert.equal(elements.get('idleLiveLog').children.length, 0);

  harness.enqueueTTSAudio('/audio/idle-1.wav', 'mio', 'idle-session-1', 'default', 1, '次です。', '次です。', '', 'idle-1');
  await Promise.resolve();

  assert.equal(harness.ttsPlayback.playing, true);
  assert.equal(elements.get('idleLiveLog').children.at(-1)._mc.textContent, '最初です。');
});

test('idlechat starts a single buffered chunk after session completed', async () => {
  const {harness, elements} = loadAudioHarness();

  harness.enqueueTTSAudio('/audio/idle-only.wav', 'shiro', 'idle-session-done', 'default', 0, '一つだけです。', '一つだけです。', '', 'idle-only');
  await Promise.resolve();
  assert.equal(harness.ttsPlayback.playing, false);
  assert.equal(elements.get('idleLiveLog').children.length, 0);

  harness.chatAudioSync.markSessionCompleted('idle-session-done');
  await Promise.resolve();

  assert.equal(harness.ttsPlayback.playing, true);
  assert.equal(elements.get('idleLiveLog').children.at(-1)._mc.textContent, '一つだけです。');
});

test('central chat starts a new bubble after current tts speech is cleared', () => {
  const {harness, elements} = loadAudioHarness();
  const chat = elements.get('chat');

  harness.setCentralTTSSpeechText('mio', '最初の発話です。', 'session-1', 0, 'u1');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('mio', '次の発話です。', 'session-1', 0, 'u2');

  assert.equal(chat.children.length, 2);
  assert.equal(chat.children[0]._mc.textContent, '最初の発話です。');
  assert.equal(chat.children[1]._mc.textContent, '次の発話です。');
});

test('central chat separates adjacent tts chunks inside one bubble', () => {
  const {harness, elements} = loadAudioHarness();

  harness.setCentralTTSSpeechText('shiro', '前半です。', 'session-1', 0, 'u1');
  harness.setCentralTTSSpeechText('shiro', '後半です。', 'session-1', 1, 'u2');

  assert.equal(elements.get('chat').children.at(-1)._mc.textContent, '前半です。 後半です。');
});

test('central chat keeps same speaker speech chunks in one bubble after audio clears', () => {
  const {harness, elements} = loadAudioHarness();
  const chat = elements.get('chat');

  harness.setCentralTTSSpeechText('shiro', '前半です。', 'session-1', 0, 'speech-0');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('shiro', '後半です。', 'session-1', 1, 'speech-1');

  assert.equal(chat.children.length, 1);
  assert.equal(chat.children[0]._mc.textContent, '前半です。 後半です。');
});

test('central chat splits same speaker speech when response id changes', () => {
  const {harness, elements} = loadAudioHarness();
  const chat = elements.get('chat');

  harness.setCentralTTSSpeechText('mio', 'ひとつめです。', 'idle-response-boundary', 0, 'chunk-0', 'idle-response-boundary:0000');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('mio', 'ふたつめです。', 'idle-response-boundary', 1, 'chunk-1', 'idle-response-boundary:0001');

  assert.equal(chat.children.length, 0);
  assert.equal(elements.get('idleLiveLog').children.length, 2);
  assert.equal(elements.get('idleLiveLog').children[0]._mc.textContent, 'ひとつめです。');
  assert.equal(elements.get('idleLiveLog').children[1]._mc.textContent, 'ふたつめです。');
});

test('central chat splits when speaker changes even inside chunk sequence', () => {
  const {harness, elements} = loadAudioHarness();
  const chat = elements.get('chat');

  harness.setCentralTTSSpeechText('mio', 'みおの発話です。', 'session-1', 0, 'mio-0');
  harness.setCentralTTSSpeechText('shiro', 'しろの発話です。', 'session-1', 1, 'shiro-1');

  assert.equal(chat.children.length, 2);
  assert.equal(chat.children[0]._mc.textContent, 'みおの発話です。');
  assert.equal(chat.children[1]._mc.textContent, 'しろの発話です。');
});

test('central chat keeps topic announcement chunks in one bubble after audio clears', () => {
  const {harness, elements} = loadAudioHarness();
  const idleLiveLog = elements.get('idleLiveLog');

  harness.setCentralTTSSpeechText('mio', '今日のお題です、', 'idle-topic-1', 0, 'topic-0');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('mio', '記憶と風景の関係です！', 'idle-topic-1', 1, 'topic-1');

  assert.equal(elements.get('chat').children.length, 0);
  assert.equal(idleLiveLog.children.length, 1);
  assert.equal(idleLiveLog.children[0]._mc.textContent, '今日のお題です、記憶と風景の関係です！');
});

test('central chat starts speech bubble after topic announcement completes', () => {
  const {harness, elements} = loadAudioHarness();
  const idleLiveLog = elements.get('idleLiveLog');

  harness.setCentralTTSSpeechText('mio', '今日のお題です、', 'idle-topic-1', 0, 'topic-0');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('mio', '記憶と風景の関係です！', 'idle-topic-1', 1, 'topic-1');
  harness.setCentralTTSSpeechText('', '');
  harness.setCentralTTSSpeechText('mio', 'それ、少し切ない入口だね。', 'idle-topic-1', 0, 'speech-0');

  assert.equal(elements.get('chat').children.length, 0);
  assert.equal(idleLiveLog.children.length, 2);
  assert.equal(idleLiveLog.children[0]._mc.textContent, '今日のお題です、記憶と風景の関係です！');
  assert.equal(idleLiveLog.children[1]._mc.textContent, 'それ、少し切ない入口だね。');
});
