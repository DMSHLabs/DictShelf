// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('internal/server/web/index.html','utf8');
const extract=name=>source.match(new RegExp('function '+name+'\\([^\\n]*\\{[\\s\\S]*?\\n\\}'))[0];
(async()=>{
 const streams=[];
 const context={dicts:[{id:'d',outdated:['reader']}],dictLabel:()=> 'Dictionary',tx:s=>s,esc:s=>s,
  setIndexStatus:()=>{},loadDicts:async()=>{},lastQuery:'',
  EventSource:class {constructor(url){this.url=url;this.handlers={};streams.push(this)}addEventListener(name,fn){this.handlers[name]=fn}close(){this.closed=true}}};
 vm.createContext(context);
 vm.runInContext(source.match(/const following=new Set\(\);/)[0]+'\n'+extract('runIngest'),context);
 const label=()=>({dataset:{},classList:{add(){},remove(){}},style:{}});
 const first=context.runIngest('d',{fts:1,quiet:true},label());
 assert.equal(streams[0].url,'/api/ingest?dict=d&fts=1');
 await context.runIngest('d',{},label());assert.equal(streams.length,1);
 await streams[0].handlers.done();assert.equal(await first,true);
 const retry=context.runIngest('d',{quiet:true},label());assert.equal(streams.length,2);
 streams[1].handlers.error({});assert.equal(await retry,false);
 const third=context.runIngest('d',{quiet:true},label());assert.equal(streams.length,3);
 await streams[2].handlers.done();await third;
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
