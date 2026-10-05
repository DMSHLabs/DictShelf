// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const html = fs.readFileSync(path.join(__dirname, '../internal/server/web/setup.html'), 'utf8');
for (const [, attrs, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
  if (!attrs.includes('src=')) new vm.Script(body);
}
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
  fetch: async (url, options) => {writes.push({url, body: JSON.parse(options.body)}); return {ok: true};}
};
vm.runInNewContext('let indexDefaultsWrite=Promise.resolve(true);\n' + ['syncDSLDefaults', 'saveIndexDefaults'].map(name => html.match(new RegExp('function ' + name + '\\(\\)\\{[\\s\\S]*?\\n}'))[0]).join('\n'), context);
(async () => {
  assert.equal(await context.saveIndexDefaults(), true);
  assert.equal(fields.newIndexIndex.checked, true);
  assert.equal(fields.newIndexIndex.disabled, true);
  assert.deepEqual(writes, [{url: '/api/index-defaults', body: {index: true, contains: true, fullText: false}}]);
  console.log('Folder index defaults: markup, syntax and saving passed.');
})().catch(error => {console.error(error); process.exitCode = 1;});
