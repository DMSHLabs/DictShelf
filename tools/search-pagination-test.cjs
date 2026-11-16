// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

const html=fs.readFileSync('internal/server/web/index.html','utf8');
const start=html.indexOf('function normalizedSearchPortionSize');
const end=html.indexOf('function searchPortionSize',start);
assert.ok(start>=0&&end>start,'search paging helpers not found');
const context={};
vm.createContext(context);
vm.runInContext(html.slice(start,end),context);

const size=context.searchPortionRequestSize;
const offset=context.searchPortionOffset;
assert.equal(size(0,5,20),5,'first block must keep its initial size');
assert.equal(size(5,5,20),20,'second block must use the configured size');
assert.equal(size(0,5,20),5,'returning to the first block must keep five articles');
assert.equal(size(25,5,20),20,'later blocks must use the configured size');
assert.equal(size(0,5,20,1),1,'single-article requests must stay at one');
assert.deepEqual([1,2,3].map(page=>offset(page,20,5)),[0,5,25],
  'block offsets must follow the five-then-configured-size sequence');
console.log('Search pagination: first block, later blocks, return navigation, and offsets passed.');
