// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const source=fs.readFileSync('internal/server/web/group-editor.js','utf8').split('async function saveGroupOrder')[0];
 const b=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});const p=await b.newPage({viewport:{width:320,height:800}});
 for(const lang of ['en','ru']){
  await p.setContent('<style>'+fs.readFileSync('internal/server/web/group-editor.css','utf8')+'</style><label><input id="groupShowAll" type="checkbox"></label><div id="groupRows"></div><div id="groupHint"></div><div id="groupError"></div><button id="closeGroups"></button><select id="groupSelect"></select><input id="q">');
  const catalog=JSON.parse(fs.readFileSync('internal/server/web/i18n/'+lang+'.json','utf8'));
  await p.evaluate(({source,catalog})=>{
   window.$=id=>document.getElementById(id);window.tx=key=>catalog[key]||key;Object.defineProperty(window,'localStorage',{value:{getItem:()=>null},configurable:true});window.wudictI18n={facetLabels:g=>({fl:g.fl,vl:g.vl})};window.requests=[];window.fetch=async(url,opts)=>{requests.push({url,body:JSON.parse(opts.body)});return {ok:true,json:async()=>({})}};
   const ox={f:'pub',fl:'Publisher',fo:4,v:'oxford',vl:'Oxford'};
   window.dicts=[{id:'member',name:'Current member',filters:[]},{id:'ox',name:'Oxford',filters:[ox]},{id:'other',name:'Other',filters:[{...ox,v:'collins',vl:'Collins'}]},{id:'none',name:'Unclassified',filters:[]}];window.orderedDicts=()=>dicts;window.orderedGroupDicts=(g,ds)=>ds.filter(d=>g.members.includes(d.id));window.dictLabel=d=>d.name;window.refreshLivePicker=()=>{};
   eval(source+';userGroups=[{id:"custom",members:["member"]}];selectedGroup="custom";groupShowAllPreferred=true;renderGroupRows();');
  },{source,catalog});
  assert.equal(await p.locator('#groupAvailableFilter').inputValue(),'all');assert.equal(await p.locator('.group-row').count(),4);
  await p.locator('#groupAvailableFilter').selectOption(JSON.stringify(['pub','oxford']));assert.deepEqual(await p.locator('.group-row').evaluateAll(ns=>ns.map(n=>n.dataset.dict)),['member','ox']);
  await p.locator('[data-dict="ox"] input').check();await p.waitForFunction(()=>document.querySelector('[data-dict="ox"] input')?.checked&&!document.querySelector('#closeGroups').disabled);
  assert.equal(await p.locator('#groupAvailableFilter').inputValue(),'all');
  assert.equal(await p.locator('#groupAvailableFilter option').evaluateAll(ns=>ns.some(n=>n.value===JSON.stringify(['pub','oxford']))),false);
  assert.equal(await p.evaluate(()=>requests[0].url),'/api/user-groups/member');
  await p.locator('#groupAvailableFilter').selectOption('uncategorized');assert.deepEqual(await p.locator('.group-row').evaluateAll(ns=>ns.map(n=>n.dataset.dict)),['member','ox','none']);
  assert.ok(await p.evaluate(()=>document.querySelector('[data-dict="ox"]').compareDocumentPosition(document.querySelector('#groupAvailableFilter'))&Node.DOCUMENT_POSITION_FOLLOWING));
  await p.locator('[data-dict="none"] input').check();await p.waitForFunction(()=>!document.querySelector('#closeGroups').disabled&&document.querySelector('[data-dict="none"] input')?.checked);
  assert.equal(await p.locator('#groupAvailableFilter option[value="uncategorized"]').count(),0);
  assert.equal(await p.locator('#groupAvailableFilter').inputValue(),'all');
 }
 await b.close();console.log('EN/RU outside-only filtering, uncategorized, membership moves and filter retention passed');
})().catch(e=>{console.error(e);process.exit(1)});
