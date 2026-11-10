// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// The star in a search result is a toggle whose requests run in the background,
// so the reader can press it faster than the server answers. What must never
// happen is two requests in flight for one article, or a press ignored because
// the previous request was still out. Checked without a browser: the queue is a
// function of two maps.
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const web = path.join(__dirname, '../internal/server/web');
const source = fs.readFileSync(path.join(web, 'index.html'), 'utf8');
const rule = source.match(/function favouriteToggleQueue\(\{[^}]*\}\)\{[\s\S]*?\n}/);
assert.ok(rule, 'favouriteToggleQueue is not in the page');
const load = () => {
  const context = {};
  vm.runInNewContext(rule[0], context);
  return context.favouriteToggleQueue;
};

// Every write is held open until the test releases it, which is what a slow
// server looks like to the star.
const harness = () => {
  const calls = [];
  const waiting = [];
  const queue = load()({
    saved: () => false,
    write: (id, want) => {
      calls.push([id, want]);
      return new Promise((resolve, reject) => waiting.push({ resolve, reject }));
    },
    onChange: () => {},
    onError: error => {
      throw error;
    },
  });
  const settle = result => {
    const next = waiting.shift();
    assert.ok(next, 'a write was settled that was never sent');
    if (result instanceof Error) next.reject(result);
    else next.resolve();
  };
  return { queue, calls, settle, waiting };
};

(async () => {
  // Press, unpress, press again while the first request is still out: the reader
  // ends with the article in the collection, and the server hears one request.
  {
    const { queue, calls, settle } = harness();
    const first = queue.toggle('c');
    const second = queue.toggle('c');
    const third = queue.toggle('c');
    settle();
    await Promise.all([first, second, third]);
    assert.deepEqual(calls, [['c', true]], 'three presses became more than one request');
    assert.equal(queue.current('c'), true);
  }
  // Press and unpress: the last wish is the server's state, so a second request
  // follows the first - in order, never beside it.
  {
    const { queue, calls, settle } = harness();
    const first = queue.toggle('c');
    const second = queue.toggle('c');
    settle();
    await new Promise(resolve => setTimeout(resolve, 0));
    assert.deepEqual(calls, [['c', true], ['c', false]], 'the second wish was not sent');
    settle();
    await Promise.all([first, second]);
    assert.equal(queue.current('c'), false);
  }
  // A refused request puts the star back where the server says it is, and does
  // not leave the queue believing the change happened.
  {
    let refused = 0;
    const calls = [];
    const context = {};
    vm.runInNewContext(rule[0], context);
    const queue = context.favouriteToggleQueue({
      saved: () => false,
      write: (id, want) => {
        calls.push([id, want]);
        return Promise.reject(new Error('no'));
      },
      onChange: () => {},
      onError: () => {
        refused++;
      },
    });
    await queue.toggle('c');
    assert.deepEqual(calls, [['c', true]]);
    assert.equal(refused, 1);
    assert.equal(queue.current('c'), false);
  }
  console.log('favourite toggle: one request at a time, the last wish wins');
})().catch(error => {
  console.error(error.message);
  process.exit(1);
});
