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
    await page.request.put(origin+'/api/dsl-mode',{data:{mode:'both'}});
    const reports=[];
    for(const language of ['en','ru']) {
      await page.request.put(origin+'/api/language',{data:{language}});
      for(const width of [1100,390,320]) {
        await page.setViewportSize({width,height:1000});
        await page.goto(origin);
        await search();
        await settings();
        assert.equal(await page.locator('[data-delete-index]:enabled').count(),4);
        const first=page.locator('#panelList .pd').first();
        await first.locator('[data-delete-index="gd"]').click();
        await first.locator('.rmno').click();
        assert.equal(await first.locator('[data-delete-index]').count(),2);
        const layout=await page.locator('#dictSettings').evaluate(d=>{
          const box=d.getBoundingClientRect();
          const bad=[...d.querySelectorAll('.dsl-index-remove,.dsl-index-row button')].filter(el=>{
            const r=el.getBoundingClientRect();
            return r.left<box.left||r.right>box.right||el.scrollWidth>el.clientWidth+1;
          });
          return {overflow:d.scrollWidth>d.clientWidth+1,bad:bad.map(el=>el.textContent)};
        });
        assert.deepEqual(layout,{overflow:false,bad:[]});
        await page.locator('#dictSettingsScroll').evaluate(el=>el.scrollTop=0);
        await page.screenshot({path:path.join(output,`remove-${language}-${width}.png`)});
        reports.push({language,width,layout});
      }
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
      await card.locator('.rmgo').click();
      await page.waitForFunction(variant=>document.querySelectorAll(`[data-delete-index="${variant}"]:disabled`).length===2,variant);
      assert.equal(fs.existsSync(target.textDB),false);
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
      await page.waitForFunction(variant=>document.querySelectorAll(`[data-delete-index="${variant}"]:disabled`).length===2,variant);
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
