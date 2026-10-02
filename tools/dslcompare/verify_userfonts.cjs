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
  const uploaded=[];
  const faces=['Regular','Bold','Italic','BoldItalic'];
  try{
    const cfg=await (await page.request.get(origin+'/api/config')).json();
    assert.ok(cfg.roots[0].path.includes('index-removal-preview'),'isolated fixture only');
    const files=await (await page.request.get(origin+'/api/files')).json();
    assert.ok(!(files.files||[]).some(f=>/^wugd_/.test(f.name)),'do not overwrite user font files');
    const font=fs.readFileSync(path.join(__dirname,'../../internal/server/web/fonts/Quivira.otf'));
    for(const face of faces){
      const name=`wugd_${face}.otf`;
      const response=await page.request.post(origin+'/api/files?name='+name,{data:font,headers:{'Content-Type':'font/otf'}});
      assert.equal(response.status(),200);uploaded.push(name);
    }
    await page.goto(origin);
    const loaded=await page.evaluate(async()=>{
      const styles=['13px','bold 13px','italic 13px','italic bold 13px'];
      return Promise.all(styles.map(async style=>(await document.fonts.load(style+' "WuGD Arial"')).length));
    });
    assert.deepEqual(loaded,[1,1,1,1]);
    console.log('All four uploaded OTF variants loaded through Custom CSS Files.');
  }finally{
    for(const name of uploaded)await page.request.delete(origin+'/api/files?name='+name);
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1});
