// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const html=fs.readFileSync('internal/server/web/index.html','utf8');
 const source=html.slice(html.indexOf('let bulkIndexBusy=false;'),html.indexOf('/* Orphans (D156)'));
 const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
 const page=await browser.newPage({viewport:{width:320,height:800}});
 for(const lang of ['en','ru']){
  const catalog=JSON.parse(fs.readFileSync('internal/server/web/i18n/'+lang+'.json','utf8'));
  await page.setContent('<style>'+fs.readFileSync('internal/server/web/app.css','utf8')+'</style><div style="width:280px"><div id="bulkIndexes"></div></div><button id="bulkIndexStop"></button><div id="bulkIndexConfirm"></div><dialog id="bulkIndexDialog"></dialog>');
  await page.evaluate(({source,catalog})=>{
   window.$=id=>document.getElementById(id);window.tx=(key,args={})=>String(catalog[key]||key).replace(/\{(\w+)\}/g,(_,k)=>args[k]??k);window.esc=window.escAttr=s=>String(s);window.cfgInfo={canDelete:true};
   window.dicts=[{id:'one',source:true,dbSize:1,caps:{Contains:true,FTS:false}}];window.calls=[];window.setIndexStatus=s=>{window.status=s};window.runIngest=async(id,opts)=>{calls.push({id,opts});return true};window.loadDicts=async()=>{};window.lastScope=window.lastQuery=null;window.wudictI18n={errorText:s=>s};window.dictLabel=d=>d.id;
   eval(source+';renderBulkIndexes();');
  },{source,catalog});
  assert.deepEqual(await page.locator('select').evaluateAll(nodes=>nodes.map(n=>n.value)),['keep','keep','keep']);
  assert.equal(await page.locator('[data-bulk-start]').isDisabled(),true);
  await page.locator('[data-bulk-feat="contains"]').selectOption('delete');
  await page.locator('[data-bulk-feat="fts"]').selectOption('create');
  assert.equal(await page.evaluate(()=>calls.length),0);
  await page.locator('[data-bulk-start]').click();assert.equal(await page.evaluate(()=>calls.length),0);
  await page.locator('[data-bulk-go]').click();await page.waitForFunction(()=>calls.length===2&&!document.getElementById('bulkIndexStop').offsetParent);
  const calls=await page.evaluate(()=>window.calls);assert.equal(calls[0].opts.contains,0);assert.equal(calls[1].opts.fts,1);
  await page.locator('[data-bulk-feat="base"]').selectOption('delete');assert.equal(await page.locator('[data-bulk-feat="fts"]').isDisabled(),true);
  assert.equal(await page.locator('[data-bulk-feat="fts"]').inputValue(),'keep');
  await page.locator('[data-bulk-feat="base"]').selectOption('update');
  await page.locator('[data-bulk-start]').click();await page.locator('[data-bulk-go]').click();await page.waitForFunction(()=>calls.length===3&&document.getElementById('bulkIndexStop').hidden);
  assert.equal(await page.evaluate(()=>calls[2].opts.rebuild),true);
  await page.locator('[data-bulk-feat="fts"]').selectOption('update');
  await page.locator('[data-bulk-start]').click();assert.equal(await page.locator('#bulkIndexDialog').isVisible(),false);
  await page.locator('[data-bulk-feat="fts"]').selectOption('recreate');
  await page.locator('[data-bulk-start]').click();await page.locator('[data-bulk-go]').click();await page.waitForFunction(()=>calls.length===5&&document.getElementById('bulkIndexStop').hidden);
  assert.deepEqual(await page.evaluate(()=>calls.slice(3).map(c=>c.opts.fts)),[0,1]);
 }
 await browser.close();console.log('EN/RU dropdown defaults, deferred start, missing/existing/all selection and rebuild plans passed');
})().catch(e=>{console.error(e);process.exit(1)});
