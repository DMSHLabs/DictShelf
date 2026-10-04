// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const fs=require('node:fs'),assert=require('node:assert/strict');const {chromium}=require('playwright');
(async()=>{
 const b=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
 for(const lang of ['en','ru']){
  const p=await b.newPage({viewport:{width:390,height:850}});const catalog=JSON.parse(fs.readFileSync('internal/server/web/i18n/'+lang+'.json','utf8'));
  await p.addInitScript(({catalog})=>{window.wudictI18n={t:(k,a={})=>String(catalog[k]||k).replace(/\{(\w+)\}/g,(_,v)=>a[v]??v)};window.confirm=()=>true},{catalog});
  const facets=[{id:'pub',groups:[{id:'mine',label:'Mine',items:['oxford']}]}];let saved=false;
  await p.route('http://test/**',async route=>{const url=route.request().url();if(url==='http://test/groups'){let html=fs.readFileSync('internal/server/web/groups.html','utf8').replace(/\{\{T:([^}]+)\}\}/g,(_,k)=>catalog[k]||k).replace('{{I18N}}','').replace('{{PRESETS}}','');await route.fulfill({contentType:'text/html',body:html})}else if(url.endsWith('/api/groups')){saved ||= route.request().method()==='PUT';await route.fulfill({json:{text:'Mine = oxford',custom:saved,writable:true,file:'groups.ini',unusable:false,problems:[],facets:saved?facets:[]}})}else if(url.endsWith('/api/dicts'))await route.fulfill({body:JSON.stringify({t:'dict',dict:{filters:[{f:'pub',v:'mine'}]}})+'\n'});else if(url.includes('/assets/setup.css'))await route.fulfill({contentType:'text/css',body:fs.readFileSync('internal/server/web/setup.css','utf8')});else await route.fulfill({body:''})});
  const errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto('http://test/groups');await p.waitForFunction(()=>document.querySelector('#text').value.length>0);
  assert.equal(await p.locator('h1').textContent(),catalog['filters.title']);await p.locator('#save').click();await p.waitForFunction(()=>document.querySelector('#msg').className==='ok');assert.equal(await p.locator('#msg').textContent(),catalog['filters.saved']);assert.deepEqual(errors,[]);
  for(const color of ['#e0cba3','#243344'])for(const image of [false,true]){
   const colors=await p.evaluate(({color,image})=>{window.wudictShellBackground(color,image);return {body:getComputedStyle(document.body).backgroundColor,card:getComputedStyle(document.querySelector('.card')).backgroundColor,input:getComputedStyle(document.querySelector('textarea')).backgroundColor,button:getComputedStyle(document.querySelector('#save')).backgroundColor,text:getComputedStyle(document.querySelector('textarea')).color}},{color,image});
   assert.equal(colors.body,image?'rgba(0, 0, 0, 0)':color==='#e0cba3'?'rgb(224, 203, 163)':'rgb(36, 51, 68)');assert.equal(colors.card,'rgba(255, 255, 255, 0.14)');assert.equal(colors.input,'rgba(255, 255, 255, 0.18)');assert.equal(colors.button,colors.input);assert.equal(colors.text,color==='#e0cba3'?'rgb(43, 42, 40)':'rgb(244, 241, 235)');
  }
  await p.close();
 }
 await b.close();console.log('EN/RU filter editor loads/saves and validates matches from fork filters');
})().catch(e=>{console.error(e);process.exit(1)});
