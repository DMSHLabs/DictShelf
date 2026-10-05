// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const main=fs.readFileSync('internal/server/web/index.html','utf8');
const groups=fs.readFileSync('internal/server/web/group-editor.js','utf8');
(async()=>{
  const rows={scrollTop:12}, controls={};let sent;
  const context={dicts:[{id:'a',dsl:{parser:'original',variant:'original'}},{id:'hidden',dsl:{variant:'gd'}},{id:'b'}],cfgInfo:null,
    userGroups:[{id:'group',members:['a','hidden','b']}],selectedGroup:'group',pickerGroup:'all',groupSaving:false,
    $:id=>id==='groupRows'?rows:controls[id]||(controls[id]={checked:false,value:''}),
    groupRequest:async(url,method,body)=>{sent=body},
    orderedIds:()=>['a','hidden','b'],refreshLivePicker:()=>{},renderGroupRows:()=>{}};
  vm.createContext(context);
  vm.runInContext(groups.match(/async function saveGroupOrder\(ids,focusId\)\{[\s\S]*?\n\}/)[0],context);
  await context.saveGroupOrder(['b','hidden']);
  assert.equal(sent.members.join(','),'a,b,hidden');
  assert.equal(context.userGroups[0].members.join(','),'a,b,hidden');
  console.log('Group ordering preserves temporarily unavailable dictionary memberships.');
})().catch(e=>{console.error(e);process.exitCode=1});
