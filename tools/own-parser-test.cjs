// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const main = fs.readFileSync(path.join(__dirname, '../internal/server/web/index.html'), 'utf8');
assert.doesNotMatch(main, /ensureOwnDSLParser|selectedDSLParser|matchesDSLParser|\/api\/dsl-mode/);
const context = {};
vm.runInNewContext(main.match(/function dictLabel\(d\)\{[^\n]+/)[0], context);
assert.equal(context.dictLabel({name:'Dictionary GD'}), 'Dictionary GD');
assert.doesNotMatch(fs.readFileSync('internal/server/web/group-editor.js','utf8'), /matchesDSLParser/);
assert.doesNotMatch(fs.readFileSync('internal/server/web/browse.html','utf8'), /dsl.variant|dsl\?\.parser/);
console.log('Single parser: no selection requests or hidden variants in settings, groups and Browse.');
