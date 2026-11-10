// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// The group editor's two lists share one window, and when they split it evenly
// is a decision a person can be surprised by: the working list must keep the
// room it needs, and even halves are for two long lists, not for a long one
// beside two dictionaries. Checked without a browser - the rule is a count.
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const web = path.join(__dirname, '../internal/server/web');
const source = fs.readFileSync(path.join(web, 'group-editor.js'), 'utf8');
const rule = source.match(/function groupSplitEqual\(activeCount,inactiveCount\)\{[\s\S]*?\n}/);
assert.ok(rule, 'groupSplitEqual(activeCount,inactiveCount) is not in the editor');
const context = {};
vm.runInNewContext(rule[0], context);
// The reported window: the library has 43 dictionaries, the group two.
assert.equal(context.groupSplitEqual(43, 2), false);
assert.equal(context.groupSplitEqual(2, 43), false);
// Eight is the line, and "eight or more" includes eight.
assert.equal(context.groupSplitEqual(8, 8), true);
assert.equal(context.groupSplitEqual(8, 7), false);
assert.equal(context.groupSplitEqual(7, 8), false);
assert.equal(context.groupSplitEqual(0, 0), false);
console.log('group split: even halves only for two long lists');
