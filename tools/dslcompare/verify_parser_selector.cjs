// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
  const origin=process.argv[2]||'http://127.0.0.1:6910';
  const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
  const page=await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  try{
    for(const language of ['en','ru']){
      await page.request.put(origin+'/api/language',{data:{language}});
      for(const width of [1100,390,320]){
        await page.setViewportSize({width,height:1000});
        await page.goto(origin);
        await page.locator('#panelBtn').click();await page.locator('#editDictSettings').click();
        for(const mode of ['original','gd','both']){
          await page.locator(`#dslParserMode [data-parser="${mode}"]`).click();
          await page.waitForFunction(mode=>!dslSaving&&selectedDSLParser()===mode,mode);
          assert.equal(await page.locator('#panelList .pd').count(),mode==='both'?2:1);
          assert.equal(await page.locator('[data-bulk-action]').count(),6);
          assert.equal(await page.evaluate(()=>bulkIndexCandidates('selected','base',true).length),mode==='both'?2:1);
          for(const card of await page.locator('#panelList .pd').all()){
            assert.equal(await card.locator('.dsl-index-row').count(),mode==='both'?2:1);
          }
          assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
        }
      }
    }
    assert.deepEqual(errors,[]);
    console.log('Global parser selection, filtered cards/index rows and EN/RU 320/390/1100 layouts passed.');
  }finally{
    await page.request.put(origin+'/api/dsl-mode',{data:{mode:'both',global:true}});
    await page.request.put(origin+'/api/language',{data:{language:'en'}});
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1});
