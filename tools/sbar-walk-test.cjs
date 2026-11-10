// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// The status bar's "where am I" rule, checked without a browser. The two walks
// share one function, and what it answers after a jump of OURS (the aim it
// recorded) and after the reader's own scroll (geometry) is the whole
// difference between an arrow that moves on and one that lifts the same place
// into view again.
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const web = path.join(__dirname, '../internal/server/web');
const main = fs.readFileSync(path.join(web, 'index.html'), 'utf8');
const source = main.match(/function sbarAt\(list,line\)\{[\s\S]*?\n}/);
assert.ok(source, 'sbarAt(list,line) is not in the page');
const context = {sbarAimed: null};
vm.runInNewContext(source[0], context);
const item = top => ({getBoundingClientRect: () => ({top})});
const list = (...tops) => tops.map(item);

// The first item is the reader's before anything has crossed the line: an
// answer starts under the bar, so at the top of the page the first place is the
// one in front of them.
assert.equal(context.sbarAt(list(600, 900), 300), 0);
// Geometry, when the reader moved: the last item above the line.
assert.equal(context.sbarAt(list(0, 300, 600), 500), 1);
// An aim of OUR last jump outranks geometry - the page settles a little after
// the landing, so a purely geometric answer would name the place it was jumped
// away from.
const aimed = list(0, 300, 600);
context.sbarAimed = aimed[2];
assert.equal(context.sbarAt(aimed, 500), 2);
// An aim the OTHER walk recorded is not in this list and must be ignored: the
// two arrows answer for their own lists, so the answer falls back to geometry.
context.sbarAimed = {getBoundingClientRect: () => ({top: 0})};
assert.equal(context.sbarAt(aimed, 500), 1);
// A step crosses a dictionary boundary: the walk is one flat list of headwords,
// so the answer names the article in the next section with nothing special
// about the boundary.
assert.equal(context.sbarAt(list(0, 100, 400, 700), 650), 2);
// Nothing to walk.
assert.equal(context.sbarAt([], 0), -1);
// The landing line comes from the summary of the article's OWN section, never
// from a constant: a long dictionary name wraps its header onto two lines, and a
// one-line assumption puts the headword under the header it is meant to stand
// beneath.
const lineFn = main.match(/function sbarArticleLine\(dt\)\{[\s\S]*?\n}/);
assert.ok(lineFn, 'sbarArticleLine(dt) is not in the page');
const lineContext = {sbarLine: () => 8};
vm.runInNewContext(lineFn[0], lineContext);
const withHeader = height => ({
  closest: () => ({querySelector: () => ({getBoundingClientRect: () => ({height})})})
});
lineContext.getComputedStyle = () => ({top: '48px'});
assert.equal(lineContext.sbarArticleLine(withHeader(62)), 110); // two lines
assert.equal(lineContext.sbarArticleLine(withHeader(30)), 78);  // one line

console.log('sbar walk: aim, geometry, boundary, empty list and the header line pass');
