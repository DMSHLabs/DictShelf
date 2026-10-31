// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Checks the setup page's index-defaults block: its markup in setup.html and
// the save path that now lives in setup.js. The two split apart when the setup
// logic moved out of inline <script> blocks, so each is read from the file that
// owns it rather than assuming setup.html holds both.

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');

const read = name => fs.readFileSync(path.join(__dirname, '../internal/server/web', name), 'utf8');
const html = read('setup.html');
const script = read('setup.js');

// Compile every piece of code that ships: setup.html's inline scripts and the
// whole of setup.js (compiled, never run - only syntax is under test here).
for (const [, attrs, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
  if (!attrs.includes('src=')) new vm.Script(body, {filename: 'setup.html'});
}
new vm.Script(script, {filename: 'setup.js'});

const section = html.match(/<section id="indexDefaults">([\s\S]*?)<\/section>/)[1];
assert.equal([...section.matchAll(/type="checkbox"/g)].length, 3);
assert.match(section, /id="newIndexIndex" checked disabled/);
assert.doesNotMatch(section, /dslGD|pages.dslParser|dictUI.dslOriginal/);

const fields = Object.fromEntries(['newIndexIndex', 'newIndexContains', 'newIndexFullText', 'indexDefaultsError'].map(id => [id, {}]));
fields.newIndexContains.checked = true;
fields.newIndexFullText.checked = false;
const writes = [];
const context = {
  $: id => fields[id],
  tx: key => key,
  window: {wudictI18n: {errorText: s => s}},
  // The save path calls browserHTTP, not fetch directly (setup.js wraps fetch
  // to guard the Android bridge), so the stub is the wrapper the code uses.
  browserHTTP: async (url, options) => {
    writes.push({url, body: JSON.parse(options.body)});
    return {ok: true, text: async () => ''};
  }
};
const source = ['syncIndexDefaults', 'saveIndexDefaults']
  .map(name => script.match(new RegExp('function ' + name + '\\(\\)\\{[\\s\\S]*?\\n}'))[0])
  .join('\n');
vm.runInNewContext('let indexDefaultsWrite=Promise.resolve(true);\n' + source, context);
(async () => {
  assert.equal(await context.saveIndexDefaults(), true);
  assert.equal(fields.newIndexIndex.checked, true);
  assert.equal(fields.newIndexIndex.disabled, true);
  assert.deepEqual(writes, [{url: '/api/index-defaults', body: {index: true, contains: true, fullText: false}}]);
  console.log('Folder index defaults: markup, syntax and saving passed.');
})().catch(error => {console.error(error); process.exitCode = 1;});
