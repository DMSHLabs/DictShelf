// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Run against the isolated enhancer fixture, never the user's dictionary library.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');

(async () => {
  const origin = process.argv[2] || 'http://127.0.0.1:6908';
  const output = path.join(__dirname, 'results', 'enhancer-preview');
  const browser = await chromium.launch({headless:true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? {executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH} : {})});
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  async function settled() {
    await page.waitForFunction(() => !document.querySelector('#dslAll button').disabled);
  }
  async function mode(value) {
    await page.locator(`[data-dsl-all="${value}"]`).click();
    await settled();
    await page.waitForFunction(value => {
      const b=document.querySelector(`[data-dsl-all="${value}"]`);
      return b.getAttribute('aria-pressed')==='true';
    }, value);
  }
  async function counts(visible, hidden) {
    assert.equal(await page.locator('#panelList .pd').count(),2);
    assert.equal(await page.locator('#panelList .dsl-unavailable').count(),hidden);
    assert.equal(await page.locator('#dict option:not([value="all"])').count(),visible);
  }
  try {
    const reports=[];
    for(const language of ['en','ru']) {
      await page.request.put(origin+'/api/language',{data:{language}});
      for(const width of [1100,390,320]) {
        await page.setViewportSize({width,height:900});
        await page.goto(origin);
        await page.locator('#q').fill('demo');
        await page.locator('#q').press('Enter');
        await page.locator('details.dict').first().waitFor();
        await page.locator('#panelBtn').click();
        await page.locator('#editDictSettings').click();
        await settled();
        await mode('original'); await counts(1,1);
        // A hidden GD card can bring its variant back without deleting either card.
        await page.locator('.dsl-unavailable [data-dsl-mode="gd"]').click();
        await settled(); await counts(2,0);
        await page.locator('.pd').first().locator('[data-dsl-mode="original"]').click();
        await settled(); await counts(1,1);
        assert.equal(await page.locator('.pd [data-dsl-mode="gd"]:disabled:checked').count(),2);
        await page.locator('#closeDictSettings').click();
        await page.locator('#closePanel').click();
        await page.waitForFunction(() => document.querySelectorAll('details.dict').length===1);
        await page.reload();
        await page.locator('#panelBtn').click();
        await page.locator('#editDictSettings').click();
        await settled(); await counts(1,1);
        await mode('both'); await counts(2,0);
        if(language==='en'&&width===1100){
          await page.route('**/api/dsl-mode',route=>route.fulfill({status:500,body:'test write failure'}),{times:1});
          await page.locator('.pd').first().locator('[data-dsl-mode="gd"]').click();
          await settled(); await counts(2,0);
          assert.equal(await page.locator('.dsl-choices input:checked').count(),4);
        }
        const layout=await page.locator('#dictSettings').evaluate(d => {
          const bad=[...d.querySelectorAll('.dsl-all button,.dsl-choices label')].filter(el=>{
            const r=el.getBoundingClientRect(),box=d.getBoundingClientRect();
            return r.left<box.left||r.right>box.right||el.scrollWidth>el.clientWidth+1;
          });
          return {overflow:d.scrollWidth>d.clientWidth+1,bad:bad.map(el=>el.textContent)};
        });
        assert.deepEqual(layout,{overflow:false,bad:[]});
        await page.screenshot({path:path.join(output,`modes-${language}-${width}.png`)});
        reports.push({language,width,layout});
      }
    }
    assert.deepEqual(errors,[]);
    fs.writeFileSync(path.join(output,'modes-browser.json'),JSON.stringify(reports,null,2));
    console.log(JSON.stringify(reports));
  } finally {
    await page.request.put(origin+'/api/dsl-mode',{data:{mode:'both'}});
    await page.request.put(origin+'/api/language',{data:{language:'en'}});
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1});
