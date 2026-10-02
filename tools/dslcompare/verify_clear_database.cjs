// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert = require('node:assert/strict');
const { chromium } = require('playwright');
(async () => {
  const origin = process.argv[2] || 'http://127.0.0.1:6912';
  const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  try {
    const config = await (await page.request.get(origin+'/api/config')).json();
    assert(config.roots.every(r => /clear-database-preview[\\/]dictionaries$/.test(r.path)), 'Use only the isolated clear-database-preview fixture');
    for (const language of ['en','ru']) {
      await page.request.put(origin+'/api/language', {data:{language}});
      for (const width of [1100,390,320]) {
        await page.setViewportSize({width,height:700});
        await page.goto(origin+'/setup');
        assert.equal(await page.locator('#libSection').count(), 0);
        await page.locator('#rows input').first().waitFor();
        await page.goto(origin);
        await page.locator('#panelBtn').click();
        assert.equal(await page.locator('#clearDatabaseBtn').count(),0);
        // Opening the dialog must not wait for another configuration request.
        let openingConfigRequests=0;
        await page.route('**/api/config',route=>{openingConfigRequests++;return route.abort()});
        await page.locator('#rescanBtn').click();
        const dialog=page.locator('#clearDatabaseDialog');
        await dialog.waitFor({state:'visible'});
        assert.equal(openingConfigRequests,0);
        await page.unroute('**/api/config');
        assert.equal(await dialog.locator('input').count(),3);
        assert(await dialog.locator('input[name=index]').isChecked());
        assert(await dialog.locator('input[name=index]').isDisabled());
        assert.equal(await dialog.locator('[data-rescan-action=keep][aria-pressed=true]').count(),3);
        assert.equal(await dialog.locator('[data-rescan-action]').count(),8);
        assert.equal(await page.locator('#clearDatabaseGo').isDisabled(),false);
        assert.equal(await dialog.evaluate(e => e.scrollWidth > e.clientWidth),false);
        assert.equal(await dialog.evaluate(e => e.scrollTop),0);
        assert.equal(await dialog.evaluate(e => e.scrollHeight > e.clientHeight+1),false,`Dialog must fit one screen: ${language}/${width}`);
        assert.equal(await dialog.locator('#clearDatabaseClose').count(),1);
        assert.equal(await page.locator('#clearDatabaseTitle').evaluate(e=>e.firstChild.textContent.trim()),await page.locator('#rescanBtn').textContent());
        const rect = await dialog.boundingBox();
        assert(rect.x>=0 && rect.x+rect.width<=width+1);
        await page.screenshot({path:`tools/dslcompare/results/clear-database-preview/${language}-${width}.png`});
        await dialog.locator('[data-rescan-feature=index][data-rescan-action=recreate]').click();
        await dialog.locator('[data-rescan-feature=contains][data-rescan-action=delete]').click();
        await dialog.locator('[data-rescan-feature=fullText][data-rescan-action=update]').click();
        await page.locator('#clearDatabaseGo').click();
        await page.waitForFunction(() => !document.getElementById('clearDatabaseCancel').disabled);
        assert.equal(await page.locator('#clearDatabaseStatus').textContent(),language==='ru'?'Словари обновлены.':'Dictionaries updated.');
        await page.locator('#clearDatabaseCancel').click();
      }
    }
    assert.deepEqual(errors,[]);
    console.log('Unified rescan EN/RU 320/390/1100px, mandatory base, actions and rebuild passed.');
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode=1; });
