// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {chromium}=require('playwright');

(async()=>{
  const origin=process.argv[2]||'http://127.0.0.1:6910';
  const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
  const page=await browser.newPage();
  const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  const config=async()=>await (await page.request.get(origin+'/api/config')).json();
  const dicts=async()=> (await (await page.request.get(origin+'/api/dicts')).text()).trim().split('\n').map(JSON.parse).filter(x=>x.t==='dict').map(x=>x.dict);
  const output=path.join(__dirname,'results','index-removal-preview');
  fs.mkdirSync(output,{recursive:true});
  let copied='';
  try{
    const cfg=await config();
    const root=cfg.roots[0].path;
    assert.ok(root.includes('index-removal-preview'),'use only isolated preview, never real dictionaries');
    await page.request.put(origin+'/api/dsl-defaults',{data:{original:{index:true},gd:{index:false}}});
    for(const language of ['en','ru']){
      await page.request.put(origin+'/api/language',{data:{language}});
      for(const width of [1100,390,320]){
        await page.setViewportSize({width,height:900});
        await page.goto(origin+'/setup');
        await page.waitForFunction(()=>document.querySelector('#rows input'));
        assert.equal(await page.locator('#dslOriginalIndex').isChecked(),true);
        assert.equal(await page.locator('#dslGDIndex').isChecked(),false);
        assert.equal(await page.locator('#dslGDContains').isDisabled(),true);
        for(const field of await page.locator('#dslDefaults input').all()){
          const rect=await field.boundingBox();
          assert.equal(rect.width,20);assert.equal(rect.height,20);
        }
        const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
        assert.equal(overflow,false,`${language} ${width} overflow`);
        await page.screenshot({path:path.join(output,`defaults-${language}-${width}.png`)});
      }
    }
    await page.locator('#dslOriginalIndex').uncheck();
    assert.equal(await page.locator('#dslDefaultsError').isVisible(),true);
    assert.equal((await config()).dslDefaults.original.index,true,'invalid selection must not be saved');
    await page.locator('#dslGDIndex').check();
    await page.locator('#dslGDContains').check();
    await page.locator('#dslGDFullText').check();
    await page.waitForFunction(()=>dslDefaultsWrite.then(ok=>ok));
    const defaults=(await config()).dslDefaults;
    assert.equal(defaults.original.index,false);
    assert.equal(defaults.gd.contains,true);
    assert.equal(defaults.gd.fullText,true);
    copied=path.join(root,`defaults-new-${Date.now()}.dsl`);
    fs.copyFileSync(path.join(__dirname,'fixtures','enhancer.dsl'),copied);
    await page.request.get(origin+'/api/rescan');
    let added=[];
    for(let i=0;i<100;i++){
      added=(await dicts()).filter(d=>d.dsl?.source===copied);
      if(added.length===2&&added.find(d=>d.dsl.variant==='gd').caps?.FTS)break;
      await new Promise(resolve=>setTimeout(resolve,100));
    }
    assert.equal(added.length,2);
    const gd=added.find(d=>d.dsl.variant==='gd'),original=added.find(d=>d.dsl.variant==='original');
    assert.equal(gd.caps.Contains,true);assert.equal(gd.caps.FTS,true);
    assert.equal(original.dbSize||0,0);assert.equal(original.unavailable,true);
    assert.equal(gd.dsl.mode,'gd');
    await page.reload();
    await page.waitForFunction(()=>document.querySelector('#dslGDFullText').checked);
    assert.deepEqual(errors,[]);
    console.log('DSL defaults, validation, persistence, automatic GD preparation and EN/RU 320/390/1100 layouts passed.');
  }finally{
    if(copied){
      for(const d of (await dicts()).filter(d=>d.dsl?.source===copied)){
        await page.request.delete(origin+`/api/library?dict=${encodeURIComponent(d.id)}&prepared=1&source=0`);
      }
      fs.rmSync(copied,{force:true});
      await page.request.get(origin+'/api/rescan');
    }
    await page.request.put(origin+'/api/dsl-defaults',{data:{original:{index:true},gd:{index:false}}});
    await page.request.put(origin+'/api/language',{data:{language:'en'}});
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1});
