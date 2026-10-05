// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

const assert = require('node:assert/strict');
const fs = require('node:fs');
const {chromium} = require('playwright');
(async () => {
  const browser = await chromium.launch({headless:true, channel:process.env.PLAYWRIGHT_CHANNEL || undefined});
  try {
    for (const width of [390, 1100]) {
      const page = await browser.newPage({viewport:{width,height:800}});
      await page.setContent('<main>Dictionary article</main><div id="bulkIndexes"><button id="bulk">Start</button></div><button id="addRow">Add folder</button><button id="save">Save folders</button><button id="clearDatabaseGo">Rescan</button><div class="pd" data-id="alpha"><button data-feat="fts" id="alpha">FTS</button><span class="rm" id="remove">Remove dictionary</span></div><div class="pd" data-id="beta"><button data-feat="fts" id="beta">FTS</button></div><dialog id="settings" class="group-dialog panel-card">Settings</dialog><dialog id="nested" class="group-dialog panel-card">Confirmation</dialog>');
      const dark=width===1100;
      await page.addStyleTag({content:dark?':root{--fg:#d8d5d0;--accent:#e69a33;--line:#33363b;--paper-bg:#191a1c}':':root{--fg:#2b2a28;--accent:#e08600;--line:#e8e4dd;--paper-bg:#fbfaf8}'});
      await page.addStyleTag({content:fs.readFileSync('internal/server/web/group-editor.css','utf8')});
      await page.evaluate(() => {
        window.wudictI18n={t:k=>k==='dictUI.bulkStopping'?'Stopping: finishing the current dictionary…':k,number:n=>n.toLocaleString('en-US')};
        window.work={id:'bulk-indexes',running:true,exclusive:true,busyDicts:[],total:30,done:4,current:'Longman DOCE',action:'update',indexes:['index'],currentDone:5000,currentTotal:10000};
        window.fetch=async(url,opts)=>{
          if(opts?.method==='DELETE')window.work.stopRequested=true;
          return {ok:true,json:async()=>window.work};
        };
      });
      await page.addScriptTag({content:fs.readFileSync('internal/server/web/work-progress.js','utf8')});
      await page.waitForSelector('#serverWorkNotice:not([hidden])');
      assert(await page.evaluate(dark=>{
        const box=document.getElementById('serverWorkNotice'),backing=document.getElementById('serverWorkBacking');
        const style=getComputedStyle(box),backStyle=getComputedStyle(backing);
        return !backing.hidden&&backing.parentNode===box.parentNode&&Number(backStyle.zIndex)<Number(style.zIndex)
          &&style.borderTopWidth==='1px'&&style.borderTopStyle==='solid'
          &&style.borderTopColor===(dark?'rgb(230, 154, 51)':'rgb(224, 134, 0)')
          &&style.color===(dark?'rgb(216, 213, 208)':'rgb(43, 42, 40)')
          &&style.backgroundImage.includes('repeating-linear-gradient');
      },dark), 'footer must show its framed, theme-colored stripes above the opaque backing');
      assert(await page.evaluate(()=>['bulk','alpha','beta','remove','save','addRow','clearDatabaseGo'].every(id=>document.getElementById(id).inert)), 'bulk work locks all mutation controls');
      await page.evaluate(async()=>{window.work.exclusive=false;window.work.busyDicts=['alpha'];await window.wudictWork.refresh();});
      assert(await page.evaluate(()=>document.getElementById('alpha').inert&&!document.getElementById('beta').inert&&['bulk','remove','save','addRow','clearDatabaseGo'].every(id=>document.getElementById(id).inert)), 'individual work leaves other dictionaries available but locks bulk/folders/removal');
      for (const id of ['settings','nested']) {
        await page.evaluate(id=>document.getElementById(id).showModal(),id);
        await page.waitForFunction(id=>document.getElementById('serverWorkNotice').parentElement.id===id,id);
        const bounds=await page.locator('#serverWorkNotice').boundingBox();
        assert(bounds.y+bounds.height<=801 && bounds.y>650, 'footer must remain at viewport bottom');
        assert(await page.evaluate(()=>{
          const button=document.querySelector('#serverWorkNotice button'), r=button.getBoundingClientRect();
          return document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)===button;
        }), 'modal must not obscure Stop or clip it');
      }
      await page.locator('#serverWorkNotice button').click();
      await page.waitForFunction(()=>document.querySelector('#serverWorkNotice button').disabled);
      const layout=await page.evaluate(()=>{
        const box=document.getElementById('serverWorkNotice');
        return {height:box.getBoundingClientRect().height,textWidth:box.querySelector('span:not(.work-spinner)').getBoundingClientRect().width};
      });
      assert(layout.textWidth>200&&layout.height<180, 'long Stop label must not squeeze progress into a narrow column');
      await page.evaluate(()=>document.getElementById('nested').close());
      await page.waitForFunction(()=>document.getElementById('serverWorkNotice').parentElement.id==='settings');
      await page.evaluate(async()=>{window.work.running=false;window.work.busyDicts=[];window.work.exclusive=false;await window.wudictWork.refresh();});
      assert(await page.locator('#serverWorkNotice').isHidden(), 'completed work must hide Stop and footer');
      assert(await page.evaluate(()=>!document.getElementById('save').inert&&!document.getElementById('alpha').inert), 'completion releases locks');
      await page.evaluate(()=>{
        const original=window.fetch;
        window.fetch=async(url,opts)=>{if(opts?.method==='POST'){await new Promise(resolve=>setTimeout(resolve,250));return {ok:true,json:async()=>window.work};}return original(url,opts);};
        window.startResult=window.wudictWork.start([{dict:'alpha',feature:'base',action:'recreate'}]);
      });
      assert(await page.locator('#serverWorkNotice').isVisible(), 'starting must show footer before POST responds');
      await page.evaluate(()=>window.startResult);
      await page.close();
    }
    console.log('Global progress remains visible and Stop usable over nested modal dialogs, desktop and mobile.');
  } finally {await browser.close();}
})().catch(err=>{console.error(err);process.exitCode=1;});
