// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const fs=require('node:fs');const path=require('node:path');const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{const b=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});const p=await b.newPage({viewport:{width:1170,height:950}});const css=fs.readFileSync('internal/server/web/presets/gd/article-style.css','utf8').replace(/@font-face\s*\{[^}]*\}/g,'');const sharedCSS=fs.readFileSync('internal/server/web/examples.css','utf8');const js=fs.readFileSync('internal/server/web/examples.js','utf8');const paper='data:image/jpeg;base64,'+fs.readFileSync('android/app/src/main/assets/backgrounds/paper_01.jpg').toString('base64');
const fixture='<p class="wu-m" style="--wd-m:1">1) горшок; котелок; кастрюля</p><p id="generated" class="wu-m wu-ex" style="--wd-m:2">cooking pot — горшок / котелок для варки пищи</p><p id="author" class="wu-m" style="--wd-m:2">♦ <span class="wu-ex">pots and pans — кухонная посуда / утварь</span></p><p id="mixed" class="wu-m" style="--wd-m:2"><span class="wu-ex">pot of jam</span> — банка варенья</p><p id="inline" class="wu-m" style="--wd-m:1">2) банка: <span class="wu-ex">pot of jam</span></p><p id="optional" class="wu-m wu-sec wu-ex" style="--wd-m:2"><span class="wu-lang">They put a plant in the pot.</span></p>';
await p.setContent('<style>body{margin:0;display:flex;font-family:Arial}section{width:390px;box-sizing:border-box;padding:15px;min-height:950px}h2{font-size:18px;font-weight:400}.dark{background:#20252b;color:#e8e3db}.light{background:#fff;color:#292929}.paper{background-image:url('+paper+');color:#332b22}</style><div id="out" style="display:contents"></div>');
await p.evaluate(({css,sharedCSS,fixture})=>{for(const theme of ['light','dark','paper']){const section=document.createElement('section');section.className=theme;section.innerHTML='<h2>'+({light:'Светлый фон',dark:'Тёмный фон',paper:'Фоновая картинка'}[theme])+'</h2>';for(const gd of [true,false]){const h=document.createElement('div');h.className='article';const r=h.attachShadow({mode:'open'});r.innerHTML='<style>:host{display:block;font:20px/1.618 Arial;color:inherit}.wu-m{margin:0;padding-left:calc(var(--wd-m,0)*1em)}.wu-ex{color:#4682b4}'+css+sharedCSS+'</style><h3>'+ (gd?'GD':'Original')+'</h3><div class="'+(gd?'wu-gd':'ordinary')+'">'+fixture+'</div>';section.append(h)}document.querySelector('#out').append(section)}},{css,sharedCSS,fixture});
const before=await p.evaluate(()=>[...document.querySelectorAll('.article')].map(n=>n.shadowRoot.textContent));await p.addScriptTag({content:js});await p.addScriptTag({content:js});const report=await p.evaluate(()=>[...document.querySelectorAll('.article')].map(n=>{const r=n.shadowRoot;return {gd:!!r.querySelector('.wu-gd'),text:r.textContent,marked:r.querySelectorAll('.wu-example-block').length,bullets:r.querySelectorAll('.wu-example-bullet').length,inlineMarked:r.querySelector('#inline').classList.contains('wu-example-block'),mixedHidden:r.querySelector('#mixed').classList.contains('wu-xonly'),bg:getComputedStyle(r.querySelector('#generated')).backgroundColor,color:getComputedStyle(r.querySelector('#generated')).color,optionalColor:getComputedStyle(r.querySelector('#optional .wu-lang')).color,authorPseudo:getComputedStyle(r.querySelector('#author'),'::before').content}}));report.forEach((r,i)=>{assert.equal(r.text,before[i]);assert.equal(r.marked,3);assert.equal(r.bullets,1);assert.equal(r.inlineMarked,false);assert.equal(r.mixedHidden,false);if(r.gd){assert.equal(r.authorPseudo,String.fromCharCode(34,34));assert.equal(r.optionalColor,r.color)}});const hideCSS=fs.readFileSync('internal/server/web/presets/examples/hide_examples_article.css','utf8');
const folding=await p.evaluate(hideCSS=>[...document.querySelectorAll('.article')].map(host=>{const r=host.shadowRoot;const style=document.createElement('style');style.textContent=hideCSS;r.append(style);const data={gd:!!r.querySelector('.wu-gd'),blockHidden:getComputedStyle(r.querySelector('#generated')).display==='none',inlineHidden:getComputedStyle(r.querySelector('#inline .wu-ex')).display==='none',mixedHidden:getComputedStyle(r.querySelector('#mixed .wu-ex')).display==='none',translationVisible:getComputedStyle(r.querySelector('#mixed')).display!=='none',inlineMarker:getComputedStyle(r.querySelector('#inline .wu-ex'),'::before').content,inlineBackground:getComputedStyle(r.querySelector('#inline .wu-ex')).backgroundColor};style.remove();data.restored=getComputedStyle(r.querySelector('#inline .wu-ex')).display!=='none';return data}),hideCSS);
for(const r of folding){assert.equal(r.blockHidden,false);assert.equal(r.inlineHidden,false);assert.equal(r.mixedHidden,false);assert.equal(r.translationVisible,true);assert.equal(r.restored,true);assert.equal(r.inlineMarker,'none');if(r.gd)assert.notEqual(r.inlineBackground,'rgba(0, 0, 0, 0)')}
const markers=['•','‣','⁃','⁌','⁍','∙','·','▪','▫','■','□','●','○','◆','♦','◇','◊','★','☆','☐','☑','☒','❥','❧','➔','➜','➤','➢','→','⇒','*','+','-','–','—'];
const optionalState=await p.evaluate(hideCSS=>[...document.querySelectorAll('.article')].map(host=>{const r=host.shadowRoot;const style=document.createElement('style');style.textContent=hideCSS;r.append(style);const hidden=getComputedStyle(r.querySelector('#optional')).display==='none';style.remove();return hidden}),hideCSS);
assert(optionalState.every(Boolean));
await p.evaluate(markers=>{const host=document.createElement('div');host.className='article';const r=host.attachShadow({mode:'open'});r.innerHTML='<div class="wu-gd">'+markers.map(m=>'<p class="wu-m">'+m+' <a class="wu-audio" href="#">audio</a> <span class="wu-ex">Example.</span></p>').join('')+'</div>';document.querySelector('#out').append(host)},markers);
await p.addScriptTag({content:js});
const markerState=await p.evaluate(()=>{const r=[...document.querySelectorAll('.article')].at(-1).shadowRoot;return [...r.querySelectorAll('p')].map(n=>({block:n.classList.contains('wu-example-block'),hidden:n.classList.contains('wu-xonly'),markers:n.querySelectorAll('.wu-example-bullet').length}))});
for(const r of markerState){assert.equal(r.block,true);assert.equal(r.hidden,false);assert.equal(r.markers,1)}
console.log('Inline folding/restore and '+markers.length+' marker variants passed');
const main=fs.readFileSync('internal/server/web/index.html','utf8');
const extraCSSFunction=main.match(/function extraExampleCSS\(\)\{[\s\S]*?\n\}/)[0];
const extraApplyFunction=main.match(/function applyHideUnmarkedExamples\(on,persist=true\)\{[\s\S]*?\n\}/)[0];
const extraRow=main.match(/<label id="examplesExtraRow"[^\n]+/)[0];
await p.evaluate(({extraApplyFunction,extraRow})=>{
  const row=document.createElement('div');row.innerHTML=extraRow;document.body.append(row);
  window.$=id=>document.getElementById(id);window.articleRefresh=()=>{};window.preferenceSaves=0;window.savePrefs=()=>window.preferenceSaves++;
  window.eval(extraApplyFunction);applyHideUnmarkedExamples(true,false);
  if(!document.getElementById('hideUnmarkedExamples').checked||window.preferenceSaves!==0)throw new Error('Loaded preference not restored');
  applyHideUnmarkedExamples(false);
  if(document.getElementById('hideUnmarkedExamples').checked||window.preferenceSaves!==1)throw new Error('Changed preference not saved');
  row.remove();
},{extraApplyFunction,extraRow});
await p.evaluate(({extraCSSFunction,hideCSS})=>{
  window.STYLE_OFF=false;window.hideUnmarkedExamples=false;window.PRESETS={enabled:['hide_examples']};
  window.eval(extraCSSFunction);
  window.testHideCSS=hideCSS;
  const h=document.createElement('div');h.id='extra-examples-test';h.className='article';
  const r=h.attachShadow({mode:'open'});
  r.innerHTML='<div class="wu-gd" data-wu-examples="2"><p id="whole">◆ <a class="wu-audio">sound</a><span class="wu-ex">example</span><img alt="picture"></p><p id="mixed"><span class="wu-ex">example</span> translation</p><p id="link"><a>definition</a><span class="wu-ex">example</span></p><p><span class="wu-sec">optional</span></p></div>';
  document.querySelector('#out').append(h);
},{extraCSSFunction,hideCSS});
await p.addScriptTag({content:js});
const extra=await p.evaluate(()=>{
  const h=document.querySelector('#extra-examples-test'),r=h.shadowRoot,style=document.createElement('style');r.append(style);
  const states=[];
  for(const [hide,on] of [[true,false],[true,true],[false,true],[true,true]]){
    window.PRESETS.enabled=hide?['hide_examples']:[];window.hideUnmarkedExamples=on;
    style.textContent=(hide?window.testHideCSS:'')+extraExampleCSS();
    states.push({whole:getComputedStyle(r.querySelector('#whole')).display==='none',fragment:getComputedStyle(r.querySelector('#mixed .wu-ex')).display==='none',mixed:getComputedStyle(r.querySelector('#mixed')).display!=='none',link:getComputedStyle(r.querySelector('#link')).display!=='none',optional:getComputedStyle(r.querySelector('.wu-sec')).display==='none'});
  }
  h.remove();return states;
});
assert.deepEqual(extra.map(s=>s.whole),[false,true,false,true]);
assert.deepEqual(extra.map(s=>s.fragment),[false,true,false,true]);
assert.deepEqual(extra.map(s=>s.optional),[true,true,false,true]);
assert(extra.every(s=>s.mixed&&s.link));
console.log('Optional [ex] folding: v2 fallback, whole paragraph/media, mixed text, definition links and Show/Hide passed');
await p.evaluate(()=>[...document.querySelectorAll('.article')].at(-1).remove());
for(const size of [15,20,30])for(const lineHeight of [1.3,1.618,2]){
 const geometry=await p.evaluate(({size,lineHeight})=>[...document.querySelectorAll('.article')].filter(h=>h.shadowRoot.querySelector('.wu-gd')).map(h=>{h.style.fontSize=size+'px';const el=h.shadowRoot.querySelector('#generated');el.style.fontSize=size+'px';el.style.lineHeight=String(lineHeight);const style=getComputedStyle(el);const marker=getComputedStyle(el,'::before');return {size:parseFloat(style.fontSize),line:parseFloat(style.lineHeight),padding:parseFloat(style.paddingTop),top:parseFloat(marker.top),width:parseFloat(marker.width),height:parseFloat(marker.height)}}),{size,lineHeight});
 for(const g of geometry){assert(Math.abs(g.width-g.size*.5)<.05);assert(Math.abs(g.height-g.size*.7)<.05);assert(Math.abs(g.top-g.padding-g.line/2)<.05)}
}
await p.evaluate(()=>{for(const h of document.querySelectorAll('.article')){h.style.removeProperty('font-size');const el=h.shadowRoot.querySelector('#generated');if(el){el.style.removeProperty('font-size');el.style.removeProperty('line-height')}}});
console.log('Diamond size/first-line centre passed at 15/20/30px and three line heights');
await p.evaluate(()=>{const host=document.createElement('div');host.id='prepared-gd-test';host.className='article';const r=host.attachShadow({mode:'open'});r.innerHTML='<div class="wu-gd" data-wu-examples="3"><p class="wu-ex wu-example-block wu-xonly">Prepared example.</p><p>Definition <span class="wu-ex wu-inline-example wu-xonly">Inline example.</span></p></div>';r.savedHTML=r.innerHTML;r.scanCalls=0;const original=r.querySelectorAll.bind(r);r.querySelectorAll=(...args)=>{r.scanCalls++;return original(...args)};document.querySelector('#out').append(host)});
await p.addScriptTag({content:js});
const skip=await p.evaluate(()=>{const h=document.querySelector('#prepared-gd-test');const r=h.shadowRoot;const result={calls:r.scanCalls,unchanged:r.innerHTML===r.savedHTML};h.remove();return result});assert.equal(skip.calls,0);assert.equal(skip.unchanged,true);console.log('Prepared GD bypass passed: no paragraph/example scans');
for(const preset of ['sepia_article.css','background_image_article.css']){
 const reset=fs.readFileSync('internal/server/web/presets/background/'+preset,'utf8');
 await p.evaluate(({sharedCSS,reset})=>{const host=document.createElement('div');host.id='original-background-test';host.className='article';const r=host.attachShadow({mode:'open'});r.innerHTML='<style>:host{display:block;color:#332b22;font:20px/1.618 Arial}.wu-m{margin:0}'+sharedCSS+reset+'</style><p class="wu-m"><span class="wu-ex">A lobster pot.</span></p><p>Definition <span class="wu-ex">Inline example.</span></p>';document.querySelector('#out').append(host)},{sharedCSS,reset});
 await p.addScriptTag({content:js});
 const colors=await p.evaluate(()=>{const host=document.querySelector('#original-background-test');const r=host.shadowRoot;const result=[r.querySelector('.wu-example-block'),r.querySelector('.wu-inline-example')].map(n=>getComputedStyle(n).backgroundColor);host.remove();return result});
 for(const color of colors)assert.notEqual(color,'rgba(0, 0, 0, 0)',preset);
}
console.log('Original direct-root background survives sepia/image presets');
const compactCSS=fs.readFileSync('internal/server/web/presets/compact/compact_article.css','utf8');
await p.setViewportSize({width:390,height:950});
for(const size of [15,20,30]){
 await p.evaluate(({sharedCSS,compactCSS,size})=>{const host=document.createElement('div');host.id='compact-example-test';const r=host.attachShadow({mode:'open'});r.innerHTML='<style>:host{display:block;font:'+size+'px/1.618 Arial}'+sharedCSS+compactCSS+'</style><p class="wu-example-block">A lobster pot.</p>';document.body.append(host)},{sharedCSS,compactCSS,size});
 const geometry=await p.evaluate(()=>{const host=document.querySelector('#compact-example-test');const n=host.shadowRoot.querySelector('p');const style=getComputedStyle(n),marker=getComputedStyle(n,'::before');const range=document.createRange();range.selectNodeContents(n);const result={textLeft:range.getBoundingClientRect().left-n.getBoundingClientRect().left,markerRight:parseFloat(marker.left)+parseFloat(marker.width),padding:parseFloat(style.paddingInlineStart)};host.remove();return result});
 assert.ok(geometry.textLeft>geometry.markerRight,JSON.stringify({size,...geometry}));
 assert.ok(Math.abs(geometry.padding-size*1.1)<.1);
}
console.log('Compact keeps example text clear of the diamond at 15/20/30px');
await p.screenshot({path:'tools/dslcompare/results/gd-examples-preview.png',fullPage:true});console.log(JSON.stringify(report.map(({text,...r})=>r)));await b.close()})().catch(e=>{console.error(e);process.exit(1)});
