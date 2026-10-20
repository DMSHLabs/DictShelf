// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '../internal/server/web/index.html'), 'utf8');
const start = html.indexOf('function nativeSearchRequest(query)');
const end = html.indexOf('function dictName(id)', start);
assert.ok(start >= 0 && end > start, 'search adapter not found');
const calls = [];
const context = {
  URLSearchParams, TextDecoder, JSON,
  window: {
    wudictI18n: { errorText: value => value },
    wudictNativeCall: async (method, argument, onMessage, signal) => {
      calls.push({ method, argument, onMessage, signal });
      onMessage({ t: 'hit', name: 'Dictionary', results: [{ word: 'corazón' }] });
    }
  },
  fetch: () => { throw new Error('native lookup attempted an HTTP request'); }
};
vm.createContext(context);
vm.runInContext(html.slice(start, end), context);

async function main() {
  const encode = query => JSON.parse(context.nativeSearchRequest(query));
  assert.deepEqual(encode('q=coraz%C3%B3n+muscular&mode=fts&dict=b%2Ca&n=25&hl=YES'), {
    q: 'corazón muscular', mode: 'fts', format: '', dict: 'b,a', n: 25, hl: true
  });
  for (const [value, limit] of [['', 0], ['invalid', 0], ['1.5', 0], ['-1', 0],
    ['%2B12', 12], ['501', 500], ['9223372036854775807', 500], ['9223372036854775808', 0]]) {
    assert.equal(encode('q=word&n=' + value).n, limit, 'limit ' + value);
  }
  for (const value of ['', '0', 'FALSE', 'no', 'off', '%20false%20']) {
    assert.equal(encode('q=word&hl=' + value).hl, false, 'highlight ' + value);
  }
  const signal = new AbortController().signal;
  const received = [];
  await context.fetchStream('/api/search?q=coraz%C3%B3n&mode=exact&n=1', message => received.push(message), signal);
  assert.equal(calls[0].method, 'search');
  assert.equal(JSON.parse(calls[0].argument).q, 'corazón');
  assert.equal(calls[0].signal, signal, 'cancellation signal lost');
  assert.equal(received[0].results[0].word, 'corazón');
  await context.fetchStream('/api/dicts', () => {});
  assert.equal(calls[1].method, 'dicts');
  const plan = '{"new":{"index":true}}';
  await context.fetchStream('/api/rescan?stream=1', () => {}, undefined, { method: 'POST', body: plan });
  assert.equal(calls[2].method, 'rescanIndexes');
  assert.equal(calls[2].argument, plan, 'rescan payload changed');
  const cancelled = Object.assign(new Error('cancelled'), { name: 'AbortError' });
  context.window.wudictNativeCall = async () => { throw cancelled; };
  await assert.rejects(context.fetchStream('/api/search?q=word', () => {}, signal), error => error === cancelled);

  // An ordinary browser retains its HTTP stream and incremental decoding.
  delete context.window.wudictNativeCall;
  const bytes = new TextEncoder().encode('{"t":"begin"}\n{"t":"hit","name":"corazón"}\n{"t":"end"}\n');
  const accentedByte = bytes.indexOf(0xc3);
  let chunks = [bytes.slice(0, accentedByte + 1), bytes.slice(accentedByte + 1)];
  context.fetch = async (url, options) => {
    assert.equal(url, '/api/search?q=word');
    assert.equal(options.signal, signal);
    return { ok: true, body: { getReader: () => ({ read: async () => chunks.length
      ? { value: chunks.shift(), done: false } : { done: true } }) } };
  };
  const browser = [];
  await context.fetchStream('/api/search?q=word', message => browser.push(message), signal);
  assert.deepEqual(browser.map(message => message.t), ['begin', 'hit', 'end']);
  assert.equal(browser[1].name, 'corazón');
  console.log('PASS: typed native search, limits/highlighting, progressive events, cancellation, and browser HTTP fallback.');
}

main().catch(error => { console.error(error); process.exitCode = 1; });
