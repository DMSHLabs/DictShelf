// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const java=fs.readFileSync('android/app/src/main/java/com/legbehindneck/wudict/Shell.java','utf8');
 const script=java.match(/DICTIONARY_PICKER_JS = """([\s\S]*?)""";/)[1];
 const b=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH});
 const p=await b.newPage();await p.setContent('<div id="bulkIndexes"></div>');
 await p.evaluate(()=>{window.calls=[];window.prompt=(name,data)=>{calls.push({name,data:JSON.parse(data)});return '1'};window.changes=0});
 await p.addScriptTag({content:script});
 for(let i=0;i<2;i++){
  await p.evaluate(()=>{document.querySelector('#bulkIndexes').innerHTML='<select><option value="keep">Не менять</option><option value="create">Создать недостающие</option><option disabled>Удалить</option></select>';document.querySelector('select').addEventListener('change',()=>changes++)});
  await p.waitForFunction(()=>document.querySelector('select').dataset.shellPicker==='1');
  await p.locator('select').evaluate(n=>n.dispatchEvent(new MouseEvent('click',{bubbles:true,detail:0})));
  await p.waitForFunction(count=>calls.length===count,i+1);
  assert.equal(await p.locator('select').inputValue(),'create');
 }
 const result=await p.evaluate(()=>({calls,changes}));assert.equal(result.changes,2);
 assert.ok(result.calls.every(c=>c.name==='wudict:dictionary-picker'&&c.data.kind==='bulkIndexAction'&&c.data.rows[2].disabled));
 await b.close();console.log('Dynamic bulk selects use the shared Android picker and retain change/disabled behavior');
})().catch(e=>{console.error(e);process.exit(1)});
