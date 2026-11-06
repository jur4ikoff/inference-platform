const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const script = fs.readFileSync(__dirname + '/proxy_ui.js', 'utf8');

test('deployments use separate IndexedDB and localStorage and route API requests', async () => {
  const entries = new Map();
  const storage = {
    get length() { return entries.size; },
    key(i) { return [...entries.keys()][i] ?? null; },
    getItem(k) { return entries.get(String(k)) ?? null; },
    setItem(k, v) { entries.set(String(k), String(v)); },
    removeItem(k) { entries.delete(String(k)); }
  };
  const databases = new Set();
  const opened = [];
  function create(project, name, root = "") {
    const factory = { open(n) { databases.add(n); opened.push(n); return n; },
      deleteDatabase(n) { databases.delete(n); },
      async databases() { return [...databases].map(name => ({ name, version: 1 })); } };
    const requests = [];
    class FakeXHR { open(method, url) { this.url = url; } }
    class FakeRequest {}
    const location = new URL(`http://localhost:8082${root}/${project}/${name}/`);
    const window = { __modelProxyPrefix: `${root}/${project}/${name}/`, indexedDB: factory, localStorage: storage, fetch: async url => requests.push(url) };
    const context = { window, location, URL, XMLHttpRequest: FakeXHR, Request: FakeRequest };
    vm.runInNewContext(script, context);
    return { window, factory, requests, FakeXHR };
  }
  const a = create('ml', 'qwen');
  const b = create('test', 'qwen');
  a.factory.open('chats'); b.factory.open('chats');
  assert.notEqual(opened[0], opened[1]);
  assert.equal((await a.factory.databases()).length, 1);
  a.window.localStorage.setItem('theme', 'dark');
  b.window.localStorage.setItem('theme', 'light');
  a.window.localStorage.clear();
  assert.equal(b.window.localStorage.getItem('theme'), 'light');
  await a.window.fetch('/v1/chat/completions');
  assert.equal(a.requests[0], 'http://localhost:8082/ml/qwen/v1/chat/completions');
  await b.window.fetch('https://example.com/data');
  assert.equal(b.requests[0], 'https://example.com/data');
  const xhr = new a.FakeXHR(); xhr.open('GET', '/props');
  assert.equal(xhr.url, 'http://localhost:8082/ml/qwen/props');
});

test('NGINX inference prefix is retained in API calls and storage namespace', async () => {
 const entries = new Map();
 const storage = {get length(){return entries.size}, key(i){return [...entries.keys()][i] ?? null}, getItem(k){return entries.get(k) ?? null}, setItem(k,v){entries.set(k,String(v))}, removeItem(k){entries.delete(k)}};
 const opened = [];
 const factory = {open(n){opened.push(n)}, deleteDatabase(){}};
 const calls = [];
 const window = {__modelProxyPrefix:'/inference/ml/qwen/', indexedDB:factory, localStorage:storage, fetch:async url => calls.push(url)};
 class FakeXHR {open(){}}; class FakeRequest {}
 vm.runInNewContext(script, {window, location:new URL('http://localhost:8000/inference/ml/qwen/'), URL, XMLHttpRequest:FakeXHR, Request:FakeRequest});
 factory.open('chats'); await window.fetch('/v1/models');
 assert.equal(opened[0], 'inference:ml:qwen:chats');
 assert.equal(calls[0], 'http://localhost:8000/inference/ml/qwen/v1/models');
});
