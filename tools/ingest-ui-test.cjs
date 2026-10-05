// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('internal/server/web/index.html','utf8');
const extract=name=>source.match(new RegExp('function '+name+'\\([^\\n]*\\{[\\s\\S]*?\\n\\}'))[0];
(async()=>{
 const streams=[];
 const stopButton={};
 const context={dicts:[{id:'d',outdated:['reader']}],dictLabel:()=> 'Dictionary',tx:s=>s,esc:s=>s,
  $:()=>stopButton,setGlobalStop:()=>{},clearGlobalStop:()=>{},requestIndexStop:()=>{},bulkIndexBusy:false,
  setIndexStatus:()=>{},loadDicts:async()=>{},lastQuery:'',
  wudictI18n:{errorText:s=>s},
  EventSource:class {static CLOSED=2;constructor(url){this.url=url;this.handlers={};this.readyState=1;streams.push(this)}addEventListener(name,fn){this.handlers[name]=fn}close(){this.closed=true;this.readyState=2}}};
 context.window=context;
 vm.createContext(context);
 vm.runInContext(source.match(/const following=new Set\(\);/)[0]+'\n'+extract('runIngest'),context);
 const label=()=>({dataset:{},classList:{add(){},remove(){}},style:{}});
 const chip=label();
 const first=context.runIngest('d',{fts:1,quiet:true},chip);
 assert.equal(streams[0].url,'/api/ingest?dict=d&fts=1');
 await context.runIngest('d',{},label());assert.equal(streams.length,1);
 await streams[0].handlers.done();assert.equal(await first,true);
 assert.equal(chip.dataset.busy,undefined);
 // Recreate uses the same chip for removing and then building the index.
 const removal=context.runIngest('d',{fts:0,quiet:true},chip);
 assert.equal(streams[1].url,'/api/ingest?dict=d&fts=0');
 await streams[1].handlers.done();assert.equal(await removal,true);
 const recreation=context.runIngest('d',{fts:1,quiet:true},chip);
 assert.equal(streams[2].url,'/api/ingest?dict=d&fts=1');
 await streams[2].handlers.done();assert.equal(await recreation,true);
 const retry=context.runIngest('d',{quiet:true},chip);assert.equal(streams.length,4);
 streams[3].handlers.error({});assert.equal(streams[3].closed,undefined,'transient disconnect must reconnect');
 streams[3].handlers.error({data:JSON.stringify({error:'test failure'})});assert.equal(await retry,false);
 assert.equal(chip.dataset.busy,undefined);
 const third=context.runIngest('d',{quiet:true},chip);assert.equal(streams.length,5);
 await streams[4].handlers.stopped();assert.equal(await third,true);
 assert.equal(chip.dataset.busy,undefined);
 const box={hidden:true,innerHTML:''};context.$=()=>box;context.escAttr=s=>s;
 vm.runInContext('let reindexSt=null;\n'+extract('renderReindex'),context);
 context.renderReindex();
 assert.equal(box.hidden,false);assert.match(box.innerHTML,/id="reindexGo"/);
 context.dicts=[];context.renderReindex();assert.equal(box.hidden,true);
 // Exercise the call at the end of the real panel render with its empty-list branch.
 let render=extract('renderPanel');
 context.dicts=[];
 let calls=0;context.renderReindex=()=>calls++;
 context.$=()=>({classList:{contains:()=>false},scrollTop:0,querySelector:()=>null,innerHTML:''});
 // The panel has substantial unrelated DOM rendering; run its unchanged final block.
 const tail=render.slice(render.indexOf('  if(scroller)scroller.scrollTop=top;'),render.lastIndexOf('\n}'));
 vm.runInContext('const scroller=null,top=0,el={};\n'+tail,context);
 assert.equal(calls,1,'panel refresh must render rebuild offer');
 console.log('Index requests, duplicate suppression, success/error retries and rebuild refresh pass.');
})().catch(e=>{console.error(e);process.exitCode=1});
