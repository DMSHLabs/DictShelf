// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
// Run against the isolated preview fixture, not the user's library.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');

(async () => {
  const origin = process.argv[2] || 'http://127.0.0.1:6907';
  const output = path.join(__dirname, 'results', 'enhancer-preview');
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {}) });
  try {
    const page = await browser.newPage();
    const failures = [];
    page.on('response', r => {
      const optional=r.status()===404&&/^\/files\/wugd_(Regular|Bold|Italic|BoldItalic)\.(ttf|otf)$/.test(new URL(r.url()).pathname);
      if(r.status()>=400&&!optional)failures.push(`${r.status()} ${r.url()}`);
    });
    await page.goto(origin);
    await page.locator('#q').fill('demo');
    await page.locator('#q').press('Enter');
    await page.locator('.wu-gd').waitFor({ state: 'attached' });
    await page.evaluate(() => {
      const host = [...document.querySelectorAll('.article')].find(el => el.shadowRoot?.querySelector('.wu-gd'));
      const section = host.closest('details');
      if (section) section.open = true;
    });
    const state = await page.evaluate(async () => {
      await document.fonts.load('17px "WuGD Phonetic"');
      await Promise.allSettled(['13px','bold 13px','italic 13px','italic bold 13px'].map(style=>document.fonts.load(style+' "WuGD Arial"')));
      const host = [...document.querySelectorAll('.article')].find(el => el.shadowRoot?.querySelector('.wu-gd'));
      const root = host.shadowRoot;
      const marks = [...root.querySelectorAll('.wu-xonly')].map(p => p.textContent.trim());
      const optional = [...root.querySelectorAll('p')].find(p => p.textContent === 'Optional example.');
      return { marks, bodyColor: getComputedStyle(root.querySelector('.wu-gd')).color, family: getComputedStyle(root.querySelector('.wu-ipa')).fontFamily,
        optionalStyle: { fontStyle: getComputedStyle(optional).fontStyle, color: getComputedStyle(optional).color },
        fonts: [...document.fonts].filter(f => f.family.startsWith('WuGD')).map(f => ({ family: f.family, status: f.status })),
        buttons: root.querySelectorAll('button,.gde-hwbtn').length };
    });
    assert.equal(state.marks.length, 2);
    assert(state.marks.includes('Optional example.'));
    assert(state.family.includes('WuGD Phonetic'));
    assert.deepEqual(state.optionalStyle, { fontStyle: 'italic', color: state.bodyColor });
    assert(state.fonts.some(f => f.family.includes('Phonetic') && f.status === 'loaded'));
    assert.equal(state.buttons, 0);
    // Settings must affect already-prepared GD articles, including v4 imports.
    for(const size of [15,24])for(const weight of [400,500,700]){
      await page.evaluate(({size,weight})=>{applyFS(size,false);applyFW(weight,false)}, {size,weight});
      const typography=await page.evaluate(()=>{
        const root=[...document.querySelectorAll('.article')].find(el=>el.shadowRoot?.querySelector('.wu-gd')).shadowRoot;
        const read=selector=>{const css=getComputedStyle(root.querySelector(selector));return {size:parseFloat(css.fontSize),weight:css.fontWeight}};
        return {body:read('.wu-gd'),ipa:read('.wu-ipa'),bold:read('b')};
      });
      assert.equal(typography.body.size,size);
      assert.equal(typography.body.weight,String(weight));
      assert.ok(Math.abs(typography.ipa.size-size*1.307692)<0.01);
      assert.equal(typography.ipa.weight,String(weight));
      assert.ok(Number(typography.bold.weight)>=700);
    }
    await page.evaluate(()=>{applyFS(15,false);applyFW(400,false)});
    // Exercise the existing commands, not an injected replacement toggle.
    await page.locator('#examplesOff').evaluate(button => button.click());
    await page.waitForFunction(() => {
      const host = [...document.querySelectorAll('.article')].find(el => el.shadowRoot?.querySelector('.wu-gd'));
      return [...host.shadowRoot.querySelectorAll('.wu-xonly')].every(p => getComputedStyle(p).display === 'none');
    });
    const fold = await page.evaluate(() => {
      const root = [...document.querySelectorAll('.article')].find(el => el.shadowRoot?.querySelector('.wu-gd')).shadowRoot;
      const mixed = [...root.querySelectorAll('p')].find(p => p.textContent.includes('Translation stays visible'));
      return { hidden: true, retained: getComputedStyle(mixed).display !== 'none' };
    });
    await page.locator('#examplesOn').evaluate(button => button.click());
    await page.waitForFunction(() => {
      const root = [...document.querySelectorAll('.article')].find(el => el.shadowRoot?.querySelector('.wu-gd')).shadowRoot;
      return [...root.querySelectorAll('.wu-xonly')].every(p => getComputedStyle(p).display !== 'none');
    });
    fold.restored = true;
    assert.deepEqual(fold, { hidden: true, retained: true, restored: true });
    for (const width of [1100, 390]) {
      await page.setViewportSize({ width, height: 850 });
      await page.screenshot({ path: path.join(output, `preview-${width}.png`), fullPage: true });
    }
    assert.deepEqual(failures, []);
    const report = { state, fold, failures };
    fs.writeFileSync(path.join(output, 'browser.json'), JSON.stringify(report, null, 2));
    console.log(JSON.stringify(report, null, 2));
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
