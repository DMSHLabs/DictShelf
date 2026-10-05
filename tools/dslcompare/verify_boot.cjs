// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
  const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
  try{
    for(const locale of ['en','ru']){
      const page=await browser.newPage();
      const errors=[],requests=[];
      page.on('pageerror',error=>errors.push(error.message));
      page.on('request',request=>requests.push(new URL(request.url()).pathname));
      // A startup rescan must never reach the server or mutate the library.
      await page.route('**/api/rescan*',route=>route.abort());
      const base=process.env.WUDICT_BASE_URL||'http://127.0.0.1:16989';
      // Run only against a throwaway server: language is a saved preference.
      const response=await page.request.put(base+'/api/language',{data:{language:locale}});
      assert.ok(response.ok());
      await page.goto(base+'/');
      await page.waitForFunction(()=>phase==='ready',null,{timeout:20000});
      assert.deepEqual(errors,[]);
      assert.ok(requests.includes('/api/dicts'));
      assert.ok(!requests.includes('/api/rescan'),'startup requested index maintenance');
      await page.close();
    }
    console.log('EN/RU full-page startup lists dictionaries without rescan');
  }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
