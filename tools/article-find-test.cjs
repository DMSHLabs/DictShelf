// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

// Run against an isolated preview server; fixtures replace only the browser DOM.
const assert = require('node:assert/strict');
const {chromium} = require('playwright');
const origin = process.argv[2] || 'http://127.0.0.1:6915';
(async () => {
  const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
  const errors = [];
  try {
    const page = await browser.newPage();
    page.on('pageerror', e => errors.push(e.message));
    async function fixture() {
      await page.evaluate(() => {
        window.wudictArticleFind.stop();
        document.getElementById('out').replaceChildren();
        const out = document.getElementById('out');
        for (let i=0;i<2;i++) {
          const section = document.createElement('details');section.className='dict';section.open=i===0;
          section.innerHTML='<summary>Dictionary '+i+'</summary><dl><dd></dd></dl>';
          out.append(section);
          const host=document.createElement('div');host.className='article';
          section.querySelector('dd').append(host);
          host.attachShadow({mode:'open'}).innerHTML=i===0
            ? '<style>.wu-xonly{display:none}p{margin:30px 0}</style><p>Cat cat cats catalog educate.</p><p>take <b>care</b> take&nbsp;\n<i>care</i> take<br>care</p><p>Ёж ёжик a+b[1]</p><p class="wu-xonly" id="hidden-one">hidden cat</p><p class="wu-xonly" id="hidden-two">hidden cat again</p><p style="display:none">invisible cat</p>'
            : '<p>cat in second dictionary</p>';
        }
        const frame=document.createElement('iframe');frame.className='article';frame.setAttribute('sandbox','allow-scripts allow-same-origin');
        frame.srcdoc='<p>cat in iframe</p><script>window.fixtureReady=true;<\/script>';
        out.querySelector('dd').append(frame);
      });
      await page.waitForFunction(() => document.querySelector('iframe.article')?.contentDocument?.body?.textContent.includes('cat in iframe'));
    }
    async function search(query,mode='contains',matchCase=false,examples=false,all=true) {
      if (!await page.locator('#articleFindDialog').evaluate(e=>e.open)) await page.locator('#sbarFind').click();
      await page.locator('#articleFindQuery').fill(query);
      await page.locator('#articleFindMode').selectOption(mode);
      await page.locator('#articleFindCase').setChecked(matchCase);
      await page.locator('#articleFindExamples').setChecked(examples);
      await page.locator('#articleFindAll').setChecked(all);
      await page.locator('#articleFindDialogNext').click();
    }
    const total = () => page.locator('#articleFindCount').textContent();
    for (const language of ['en','ru']) {
      await page.request.put(origin+'/api/language',{data:{language}});
      for (const width of [320,390,1100]) {
        await page.setViewportSize({width,height:700});
        await page.goto(origin+'/?q=find');
        await page.locator('.article').first().waitFor();
        await page.waitForFunction(() => phase === 'ready' && !livePickerSearching);
        await fixture();
        await page.locator('#sbarFind').click();
        assert.equal(await page.locator('#articleFindDialog').evaluate(e=>e.scrollWidth>e.clientWidth),false,`${language}/${width} dialog overflow`);
        assert.equal(await page.locator('#sbar').evaluate(e=>e.scrollWidth>e.clientWidth),false,`${language}/${width} bar overflow`);
        await page.screenshot({path:`tools/dslcompare/results/article-find-preview/dialog-${language}-${width}.png`});
        await search('cat');assert.match(await total(),/7$/);
        await search('cat','prefix');assert.match(await total(),/6$/);
        await search('cat','exact');assert.match(await total(),/4$/);
        await search('cat','exact',true);assert.match(await total(),/3$/);
        await search('"take care"','exact');assert.match(await total(),/3$/);
        assert.equal(await page.evaluate(()=>document.querySelector('.article').shadowRoot.querySelector('b').textContent),'care');
        await search('ёж','exact');assert.match(await total(),/1$/);
        await search('ёж','prefix');assert.match(await total(),/2$/);
        await search('a+b[1]');assert.match(await total(),/1$/);
        await search('hidden','contains',false,false);
        assert.match(await total(),language==='ru'?/Совпадений нет/:/No matches/);
        await page.locator('#sbarFind').click();
        assert.equal(await page.locator('#articleFindHiddenHint').isVisible(),true);
        await search('hidden','contains',false,true);
        assert.match(await total(),/2$/);
        const visibleExamples = () => page.evaluate(()=>[...document.querySelector('.article').shadowRoot.querySelectorAll('.wu-xonly')].filter(e=>getComputedStyle(e).display!=='none').map(e=>e.id));
        assert.deepEqual(await visibleExamples(),['hidden-one']);
        await page.locator('#articleFindNext').click();assert.deepEqual(await visibleExamples(),['hidden-two']);
        await page.locator('#articleFindNext').click();assert.deepEqual(await visibleExamples(),['hidden-one']);
        assert.match(await total(),language==='ru'?/С начала/:/From the beginning/);
        await page.locator('#articleFindPrev').click();assert.deepEqual(await visibleExamples(),['hidden-two']);
        await page.locator('#articleFindStop').click();assert.deepEqual(await visibleExamples(),[]);
        await search('cat','exact',false,false,false);
        assert.equal(await page.evaluate(()=>CSS.highlights.get('wudict-find-all').size),0);
        assert.equal(await page.evaluate(()=>CSS.highlights.get('wudict-find-current').size),1);
        for(let i=0;i<4;i++) await page.locator('#articleFindNext').click();
        assert.equal(await page.locator('details.dict').nth(1).evaluate(e=>e.open),true);
        await page.evaluate(()=>{
          const host=document.createElement('div');host.className='article';
          host.attachShadow({mode:'open'}).innerHTML='<p>cat arriving later</p>';
          document.querySelector('dd').append(host);
        });
        await page.waitForFunction(()=>/5$/.test(document.getElementById('articleFindCount').textContent));
        assert.equal(await page.locator('#articleFindStrip').evaluate(e=>e.scrollWidth>e.clientWidth),false);
        await page.screenshot({path:`tools/dslcompare/results/article-find-preview/${language}-${width}.png`});
        await page.evaluate(async()=>{document.getElementById('q').value='find';await doSearch()});
        assert.equal(await page.locator('#articleFindStrip').isVisible(),false);
        assert.equal(await page.evaluate(()=>CSS.highlights.has('wudict-find-current')),false);
        await page.locator('#sbarFind').click();
        assert.equal(await page.locator('#articleFindQuery').inputValue(),'cat');
        await page.locator('#articleFindClose').click();
        await page.locator('#articleFindStrip').waitFor({state:'visible'});
        await page.evaluate(()=>sbarSet(false));
        assert.equal(await page.locator('#articleFindStrip').evaluate(e=>Math.round(e.getBoundingClientRect().bottom)),700);
        await page.evaluate(()=>sbarSet(true));
        await page.keyboard.press('Control+f');
        assert.equal(await page.locator('#articleFindDialog').isVisible(),true);
        await page.locator('#articleFindQuery').fill('Fixture');
        await page.locator('#articleFindQuery').press('Enter');
        assert.match(await total(),/1$/);
        await page.locator('#articleFindStop').click();
        await page.reload();
        await page.waitForFunction(() => phase === 'ready' && !livePickerSearching);
        await page.locator('#sbarFind').click();
        assert.equal(await page.locator('#articleFindQuery').inputValue(),'Fixture');
        await page.locator('#articleFindClose').click();
      }
    }
    await fixture();
    await page.evaluate(()=>Object.defineProperty(CSS,'highlights',{value:undefined}));
    await search('cat','exact');
    assert.equal(await page.evaluate(()=>document.getSelection().toString().toLowerCase()),'cat');
    await page.locator('#articleFindStop').click();
    assert.equal(await page.evaluate(()=>document.getSelection().toString()),'');
    assert.deepEqual(errors,[]);
    console.log('Article find: modes, case, formatted phrases, iframes, hidden examples, wrap, cleanup, streaming, EN/RU 320/390/1100 passed.');
  } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1});
