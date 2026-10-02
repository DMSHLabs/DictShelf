// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Use only the isolated enhancer fixture, never the user's dictionary library.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');

(async () => {
  const origin = process.argv[2] || 'http://127.0.0.1:6910';
  const output = path.join(__dirname, 'results', 'index-removal-preview');
  fs.mkdirSync(output, {recursive:true});
  const browser = await chromium.launch({headless:true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? {executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH} : {})});
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  async function dictionaries() {
    const response = await page.request.get(origin+'/api/dicts');
    return (await response.text()).trim().split('\n').map(JSON.parse)
      .filter(x=>x.t==='dict').map(x=>x.dict).filter(x=>x.dsl);
  }
  async function search() {
    await page.locator('#q').fill('demo');
    await page.locator('#q').press('Enter');
    await page.waitForFunction(() => document.querySelectorAll('details.dict').length===2);
  }
  async function settings() {
    await page.locator('#panelBtn').click();
    await page.locator('#editDictSettings').click();
    await page.locator('#panelList .pd').first().waitFor();
    for (const card of await page.locator('#panelList .pd').all()) {
      await card.locator('details.prov > summary').click();
    }
  }
  try {
    console.log('Checking index settings layouts and operations...');
    await page.request.put(origin+'/api/dsl-mode',{data:{mode:'both'}});
    const reports=[];
    for(const language of ['en','ru']) {
      await page.request.put(origin+'/api/language',{data:{language}});
      for(const width of [1100,390,320]) {
        await page.setViewportSize({width,height:1000});
        await page.goto(origin);
        await search();
        await settings();
        assert.equal(await page.locator('[data-bulk-action]').count(),6);
        const rows=page.locator('#panelList .pd').first().locator('.dsl-index-row');
        assert.equal(await rows.count(),2);
        for(const row of await rows.all()){
          assert.equal(await row.locator('[data-feat="contains"]').count(),1);
          assert.equal(await row.locator('[data-feat="fts"]').count(),1);
          const target=await row.locator('[data-feat="fts"]').getAttribute('data-target');
          assert.equal(await row.locator('a.browse').getAttribute('href'),'/browse?dict='+target);
        }
        assert.equal(await page.locator('[data-delete-index]:enabled').count(),4);
        const first=page.locator('#panelList .pd').first();
        await first.locator('[data-delete-index="gd"]').click();
        await first.locator('.rmno').click();
        assert.equal(await first.locator('[data-delete-index]').count(),2);
        const layout=await page.locator('#dictSettings').evaluate(d=>{
          const box=d.getBoundingClientRect();
          const bad=[...d.querySelectorAll('.bulk-index-row button,.dsl-index-row button')].filter(el=>{
            const r=el.getBoundingClientRect();
            return r.left<box.left||r.right>box.right||el.scrollWidth>el.clientWidth+1;
          });
          return {overflow:d.scrollWidth>d.clientWidth+1,bad:bad.map(el=>el.textContent)};
        });
        assert.deepEqual(layout,{overflow:false,bad:[]});
        await page.locator('#dictSettingsScroll').evaluate(el=>el.scrollTop=0);
        await page.screenshot({path:path.join(output,`remove-${language}-${width}.png`)});
        reports.push({language,width,layout});
        if(language==='en'&&width===1100){
          const feature=first.locator('[data-feat="fts"]').first();
          const before=await first.locator('.dsl-feature-line').first().boundingBox();
          await page.evaluate(()=>{
            window.EventSource=class {
              addEventListener(name,callback){
                if(name==='progress')setTimeout(()=>callback({data:JSON.stringify({done:35000,total:0})}),100);
              }
              close(){}
            };
          });
          await feature.click();
          await page.waitForFunction(()=>document.querySelector('.dsl-index-tools .busy')?.textContent.includes('35,000'));
          const after=await first.locator('.dsl-feature-line').first().boundingBox();
          assert.equal(after.x,before.x);
        }
      }
    }
    // Hold two simulated jobs open to inspect queued and active card fields.
    await page.request.put(origin+'/api/language',{data:{language:'en'}});
    await page.reload();await settings();
    await page.evaluate(()=>{
      window.realEventSource=window.EventSource;
      window.bulkFakeSources=[];
      window.EventSource=class {
        constructor(url){this.url=url;this.listeners={};window.bulkFakeSources.push(this)}
        addEventListener(name,callback){this.listeners[name]=callback}
        close(){}
      };
    });
    await page.locator('[data-bulk-action="create"][data-bulk-scope="selected"][data-bulk-feat="fts"]').click();
    assert.equal(await page.locator('#bulkIndexDialog').evaluate(el=>el.open),true);
    const popup=await page.locator('#bulkIndexDialog').boundingBox();
    assert.ok(popup.x>=31&&popup.width<=420,'confirmation leaves visible space beside it');
    assert.ok(popup.height<page.viewportSize().height*.8,'confirmation must not stretch vertically');
    assert.equal(await page.locator('#bulkIndexDialog').evaluate(el=>getComputedStyle(el,'::backdrop').backgroundColor),'rgba(0, 0, 0, 0)');
    await page.locator('[data-bulk-cancel]').click();
    assert.equal(await page.evaluate(()=>window.bulkFakeSources.length),0);
    await page.locator('[data-bulk-action="create"][data-bulk-scope="selected"][data-bulk-feat="fts"]').click();
    await page.locator('[data-bulk-go]').click();
    assert.equal(await page.locator('#bulkIndexDialog').evaluate(el=>el.open),false);
    await page.waitForFunction(()=>window.bulkFakeSources.length===1);
    const active=await page.evaluate(()=>new URL(window.bulkFakeSources[0].url,location.href).searchParams.get('dict'));
    await page.evaluate(()=>window.bulkFakeSources[0].listeners.progress({data:JSON.stringify({done:35000,total:70000})}));
    assert.match(await page.locator('#bulkIndexConfirm').textContent(),/35,000\/70,000/);
    await page.waitForFunction(()=>document.querySelector('#statusMsg').textContent.includes('35,000/70,000'));
    const activeFields=page.locator(`.pd [data-feat="fts"][data-target="${active}"]`);
    assert.equal(await activeFields.count(),2);
    for(const field of await activeFields.all())assert.match(await field.textContent(),/35,000\/70,000/);
    const queuedFields=page.locator(`.pd [data-feat="fts"]:not([data-target="${active}"])`);
    for(const field of await queuedFields.all())assert.match(await field.textContent(),/preparing/i);
    await page.evaluate(()=>window.bulkFakeSources[0].listeners.done({data:'{}'}));
    await page.waitForFunction(()=>window.bulkFakeSources.length===2);
    await page.evaluate(()=>window.bulkFakeSources[1].listeners.done({data:'{}'}));
    await page.waitForFunction(()=>!bulkIndexBusy&&!document.querySelector('[data-bulk-action]').disabled);
    await page.evaluate(()=>{window.bulkFakeSources=[]});
    console.log('Checking safe stop after the active dictionary...');
    await page.locator('[data-bulk-action="create"][data-bulk-scope="selected"][data-bulk-feat="fts"]').click();
    await page.locator('[data-bulk-go]').click();
    await page.waitForFunction(()=>window.bulkFakeSources.length===1);
    await page.locator('#bulkIndexStop').click();
    assert.equal(await page.locator('#bulkIndexStop').isDisabled(),true);
    assert.match(await page.locator('#bulkIndexStop').textContent(),/finishing the current dictionary/);
    assert.equal(await page.evaluate(()=>bulkIndexBusy),true);
    await page.evaluate(()=>window.bulkFakeSources[0].listeners.done({data:'{}'}));
    await page.waitForFunction(()=>!bulkIndexBusy&&!document.querySelector('[data-bulk-action]').disabled);
    assert.equal(await page.evaluate(()=>window.bulkFakeSources.length),1);
    assert.match(await page.locator('#bulkIndexConfirm').textContent(),/Stopped\. Updated 1 dictionaries/);
    await page.evaluate(()=>{window.EventSource=window.realEventSource});
    async function bulk(action,scope,feat){
      const mode=scope==='all'?'both':scope;
      await page.locator(`#dslParserMode [data-parser="${mode}"]`).click();
      await page.waitForFunction(mode=>!dslSaving&&selectedDSLParser()===mode,mode);
      await page.locator(`[data-bulk-action="${action}"][data-bulk-scope="selected"][data-bulk-feat="${feat}"]`).click();
      await page.locator('[data-bulk-go]').click();
      await page.waitForFunction(()=>!bulkIndexBusy&&!document.querySelector('[data-bulk-action]').disabled);
    }
    await bulk('create','gd','fts');
    assert.equal((await dictionaries()).find(d=>d.dsl.variant==='gd').caps.FTS,true);
    assert.equal((await dictionaries()).find(d=>d.dsl.variant==='original').caps.FTS,false);
    await bulk('delete','gd','fts');
    await bulk('create','all','contains');
    assert.ok((await dictionaries()).every(d=>d.caps.Contains));
    await bulk('delete','original','contains');
    assert.equal((await dictionaries()).find(d=>d.dsl.variant==='gd').caps.Contains,true);
    assert.equal((await dictionaries()).find(d=>d.dsl.variant==='original').caps.Contains,false);
    await bulk('delete','all','base');
    assert.ok((await dictionaries()).every(d=>d.unavailable&&!d.dbSize));
    assert.equal(await page.locator('.dsl-browse-line .browse').count(),0);
    assert.equal(await page.locator('.dsl-index-note').count(),0);
    assert.equal(await page.locator('.dsl-index-warning').count(),2);
    await bulk('create','all','base');
    assert.ok((await dictionaries()).every(d=>d.dbSize));
    for(const card of await page.locator('.pd').all()){
      if(!await card.locator('details.prov').evaluate(el=>el.open))await card.locator('details.prov > summary').click();
    }
    const before=await dictionaries();
    assert.equal(before.length,2);
    assert.equal(path.basename(before[0].dsl.source),'demo.dsl');
    const source=before[0].dsl.source;
    const sourceBody=fs.readFileSync(source);
    for(const variant of ['gd','original']) {
      const current=await dictionaries();
      const target=current.find(x=>x.dsl.variant===variant);
      const other=current.find(x=>x.dsl.variant!==variant);
      const otherBody=fs.readFileSync(other.textDB);
      const otherStamp=fs.statSync(other.textDB).mtimeMs;
      // Delete the other variant from the opposite card, not its own card.
      const card=page.locator(`.pd[data-id="${other.id}"]`);
      await card.locator(`[data-delete-index="${variant}"]`).click();
      assert.equal(await card.locator('.rmgo').getAttribute('data-target'),target.id);
      assert.equal(await card.locator('.rmgo').textContent(),'Delete');
      assert.equal(await card.locator('.rmno').textContent(),'Cancel');
      const question=await card.locator('.rmq').textContent();
      assert.equal(question.includes('full-text'),!!target.caps.FTS);
      assert.equal(question.includes('contains'),!!target.caps.Contains);
      assert.equal(question.includes('packed media'),!!(target.mediaDB||target.mediaSize));
      await card.locator('.rmgo').click();
      await page.waitForFunction(variant=>document.querySelectorAll(`[data-delete-index="${variant}"]`).length===0,variant);
      assert.equal(fs.existsSync(target.textDB),false);
      assert.equal(await card.locator('.dsl-browse-line .browse').count(),1);
      assert.equal(await card.locator('.dsl-index-note').count(),1);
      assert.equal(await card.locator('.dsl-index-warning').count(),0);
      assert.deepEqual(fs.readFileSync(source),sourceBody);
      assert.deepEqual(fs.readFileSync(other.textDB),otherBody);
      assert.equal(fs.statSync(other.textDB).mtimeMs,otherStamp);
      await page.locator('#closeDictSettings').click();
      await page.locator('#closePanel').click();
      await page.locator('#q').press('Enter');
      await page.waitForFunction(() => document.querySelectorAll('details.dict').length===1);
      assert.equal(fs.existsSync(target.textDB),false);
      await page.reload();
      await settings();
      assert.equal(await page.locator(`#dict option[value="${target.id}"]`).count(),0);
      const full=page.locator(`.pd[data-id="${other.id}"] [data-feat="fts"][data-target="${target.id}"]`);
      await full.click();
      await page.waitForFunction(id=>document.querySelector(`.pd [data-feat="fts"][data-target="${id}"]`)?.getAttribute('aria-pressed')==='true',target.id);
      const rebuilt=(await dictionaries()).find(x=>x.id===target.id);
      assert.ok(rebuilt.dbSize>0);
      assert.equal(rebuilt.caps.FTS,true);
      assert.deepEqual(fs.readFileSync(other.textDB),otherBody);
      await page.locator(`.pd[data-id="${other.id}"] [data-feat="fts"][data-target="${target.id}"]`).click();
      await page.waitForFunction(id=>document.querySelector(`.pd [data-feat="fts"][data-target="${id}"]`)?.getAttribute('aria-pressed')==='false',target.id);
      assert.equal((await dictionaries()).find(x=>x.id===target.id).caps.FTS,false);
      for (const c of await page.locator('#panelList .pd').all()) {
        const disclosure=c.locator('details.prov');
        if(!await disclosure.evaluate(el=>el.open))await disclosure.locator('summary').click();
      }
    }
    for(const variant of ['gd','original']) {
      const card=page.locator('.pd').first();
      await card.locator(`[data-delete-index="${variant}"]`).click();
      await card.locator('.rmgo').click();
      await page.waitForFunction(variant=>document.querySelectorAll(`[data-delete-index="${variant}"]`).length===0,variant);
      for(const c of await page.locator('#panelList .pd').all()) {
        const disclosure=c.locator('details.prov');
        if(!await disclosure.evaluate(el=>el.open))await disclosure.locator('summary').click();
      }
    }
    assert.equal(await page.locator('.dsl-index-warning').count(),2);
    await page.reload();
    await settings();
    assert.equal(await page.locator('#dict option:not([value="all"])').count(),0);
    assert.equal(await page.locator('.dsl-index-warning').count(),2);
    // Restore the isolated fixture for subsequent mode/layout tests.
    for(const variant of ['original','gd']) {
      const target=(await dictionaries()).find(x=>x.dsl.variant===variant);
      await page.locator(`.pd [data-feat="base"][data-target="${target.id}"]`).first().click();
      await page.waitForFunction(id=>!document.querySelector(`.pd [data-feat="base"][data-target="${id}"]`),target.id);
    }
    assert.deepEqual(errors,[]);
    fs.writeFileSync(path.join(output,'browser.json'),JSON.stringify(reports,null,2));
    console.log('Index removal, persistence, search exclusion, explicit rebuild, independent full text and EN/RU layouts passed.');
  } finally {
    await page.request.put(origin+'/api/language',{data:{language:'en'}});
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1});
