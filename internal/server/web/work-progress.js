/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */
(() => {
  const t = (key, params) => window.wudictI18n.t(key, params);
  const style = document.createElement('style');
  style.textContent = `#serverWorkNotice{position:fixed;bottom:var(--wd-vv-bottom,0px);left:8px;right:8px;z-index:9999;box-sizing:border-box;padding:8px 10px calc(8px + env(safe-area-inset-bottom,0px));display:flex;align-items:center;gap:8px;flex-wrap:wrap;color:var(--fg,var(--fg-main,#222));font:14px sans-serif;letter-spacing:0}#serverWorkNotice[hidden]{display:none}#serverWorkNotice .work-spinner{box-sizing:border-box;flex:none;width:16px;height:16px;border:2px solid color-mix(in srgb,var(--accent,#865000) 28%,transparent);border-top-color:var(--accent,#865000);border-radius:50%;animation:work-spin .8s linear infinite}#serverWorkNotice span{flex:1 1 220px;min-width:0;overflow-wrap:anywhere}#serverWorkNotice button{flex:0 1 auto;max-width:100%;white-space:normal;overflow-wrap:anywhere;font:inherit;color:inherit;padding:3px 9px;border:1px solid var(--line,#b9a98e);border-radius:6px;background:var(--paper-bg,var(--card,var(--bg,#fff)))}#serverWorkNotice button:disabled{opacity:.8}html.server-work-active #workNotice{display:none!important}@keyframes work-spin{to{transform:rotate(360deg)}}@media(prefers-reduced-motion:reduce){#serverWorkNotice .work-spinner{animation:none;border-color:var(--accent,#865000)}}`;
  style.textContent += `#serverWorkNotice{margin:0 6px 6px;border:1px solid var(--accent,var(--line,#b9a98e));border-radius:10px;background-color:var(--paper-bg,var(--bg,#fff));background-image:repeating-linear-gradient(135deg,color-mix(in srgb,var(--accent,#865000) 13%,transparent) 0px,color-mix(in srgb,var(--accent,#865000) 13%,transparent) 7px,transparent 7px,transparent 15px);box-shadow:var(--shadow,0 2px 10px #0003)}#serverWorkBacking{position:fixed;left:0;right:0;bottom:var(--wd-vv-bottom,0px);height:var(--workh,0px);z-index:9998;background-color:var(--paper-bg,var(--bg,#fff));background-image:var(--paper-bk-image,none);background-size:100vw 100vh;background-position:center bottom;background-repeat:no-repeat;pointer-events:none}#serverWorkBacking[hidden]{display:none}`;
  style.textContent += `#serverWorkNotice{flex-wrap:wrap}#serverWorkNotice span{flex:1 1 220px;overflow-wrap:anywhere}#serverWorkNotice button{flex:0 1 auto;max-width:100%;white-space:normal;overflow-wrap:anywhere}`;
  style.textContent += `html.server-work-active body>.card{margin-bottom:var(--workh,0px)}@media(max-width:600px){html.server-work-active dialog.group-dialog.panel-card{bottom:calc(1em + var(--wd-inset-bottom,0px) + var(--workh,0px))}}`;
  style.textContent += `html.server-work-active #bulkIndexStatus,html.server-work-active #reindexLine,html.server-work-active #clearDatabaseStatus,html.server-work-active #clearDatabaseProgress{display:none!important}`;
  style.textContent += `[data-work-locked]{opacity:.45!important;pointer-events:none!important;cursor:default!important}`;
  document.head.append(style);
  const backing = document.createElement('div');
  backing.id = 'serverWorkBacking'; backing.hidden = true; backing.setAttribute('aria-hidden','true');
  const box = document.createElement('div');
  box.id = 'serverWorkNotice'; box.hidden = true; box.setAttribute('role', 'status');
  const spinner = document.createElement('span'), text = document.createElement('span'), button = document.createElement('button');
  spinner.className = 'work-spinner'; spinner.setAttribute('aria-hidden','true');
  button.type = 'button'; box.append(spinner, text, button); document.body.append(backing, box);
  let status = null, pending = false, starting = false;
  const cacheKey='wudict-active-work';
  async function request(url, options={}) {
    const controller=new AbortController();
    const timer=setTimeout(()=>controller.abort(),10000);
    try {return await fetch(url,{...options,signal:controller.signal});}
    finally {clearTimeout(timer);}
  }
  const locked = new Set();
  function lockControls(s) {
    const ids=new Set(s.busyDicts||[]),any=s.exclusive||ids.size>0;
    const targets=new Set();
    const mark=selector=>document.querySelectorAll(selector).forEach(el=>targets.add(el));
    if(any)mark('#bulkIndexes button,#bulkIndexes select,#clearDatabaseGo,#clearDatabaseDialog input,#rescanActions button,#rescanActions input,#rescanActions select,#reindexGo,.pd .rm,.pd [data-rm="all"],.pd [data-rm="source"],#addRow,#rows input,#rows button,#save,#indexDefaults input,#choose,#fetch,#install,#file,#url,#howtoCopy,#howtoBack');
    document.querySelectorAll('.pd [data-feat],.pd [data-delete-index],.pd [data-rm="index"],.upgrade').forEach(el=>{
      const id=el.dataset.target||el.dataset.id||el.closest('.pd')?.dataset.id;
      if(s.exclusive||ids.has(id))targets.add(el);
    });
    for(const el of locked)if(!targets.has(el)){el.removeAttribute('data-work-locked');el.inert=false;locked.delete(el);}
    for(const el of targets){el.setAttribute('data-work-locked','');el.inert=true;locked.add(el);}
  }
  document.addEventListener('click',event=>{if(event.target.closest('[data-work-locked]')){event.preventDefault();event.stopImmediatePropagation();}},true);
  new MutationObserver(()=>{if(status)lockControls(status);}).observe(document.body,{childList:true,subtree:true});
  // A modal dialog lives above ordinary z-index layers. Keep the footer inside
  // the most recently opened modal so it remains visible and its Stop is usable.
  const dialogs = [];
  function place() {
    const parent = dialogs.filter(d => d.open && d.isConnected).at(-1) || document.body;
    if (backing.parentNode !== parent) parent.append(backing);
    if (box.parentNode !== parent) parent.append(box);
    const local = document.getElementById('workNotice');
    if (local && local.parentNode !== parent) parent.append(local);
  }
  new MutationObserver(records => {
    for (const record of records) {
      const d = record.target;
      if (d.tagName !== 'DIALOG') continue;
      const i = dialogs.indexOf(d); if (i >= 0) dialogs.splice(i, 1);
      if (d.open) dialogs.push(d);
    }
    place();
  }).observe(document.body, {subtree:true, attributes:true, attributeFilter:['open']});
  function render(s) {
    status = s;
    try {if(s.running)sessionStorage.setItem(cacheKey,JSON.stringify(s));else sessionStorage.removeItem(cacheKey);}catch{}
    lockControls(s);
    box.hidden = !s.running;
    backing.hidden = !s.running;
    document.documentElement.classList.toggle('server-work-active', s.running);
    if (s.running) {
      const action = t(s.action === 'remove' ? 'dictUI.removing' : s.action === 'update' ? 'database.actionUpdate' : 'dictUI.indexing');
      const indexes = (s.indexes || []).map(f => t(f === 'index' ? 'dictUI.indexPlain' : f === 'contains' ? 'dictUI.contains' : 'dictUI.fts')).join(', ');
      const count = s.currentTotal > 0 ? ` ${window.wudictI18n.number(s.currentDone)} ${t('dictUI.progressOf',{total:window.wudictI18n.number(s.currentTotal)})}` : s.currentDone > 0 ? ` ${window.wudictI18n.number(s.currentDone)}` : '';
      text.textContent = s.id==='starting' ? t('database.working') : s.id==='folder-setup' ? t('pages.savingFolders') : `${Math.min(s.done+1,s.total)}/${s.total} · ${s.current || ''} — ${action} ${indexes}${count}`;
      if(s.id==='rescan-indexes' && s.stage!=='dictionary')text.textContent=t(s.stage==='cleanup'?'database.cleaning':'database.working');
      if((s.busyDicts||[]).length>1)text.textContent=t('dictUI.activeJobs',{number:s.busyDicts.length})+' · '+text.textContent;
      button.disabled = s.stopRequested || pending || starting;
      button.hidden = s.id==='starting';
      button.textContent = t(button.disabled ? 'dictUI.bulkStopping' : 'dictUI.stop');
    }
    place();
    if (s.running) document.documentElement.style.setProperty('--workh', `${box.getBoundingClientRect().height + 8}px`);
    else if (!document.getElementById('workNotice') || document.getElementById('workNotice').hidden) document.documentElement.style.setProperty('--workh','0px');
    window.dispatchEvent(new CustomEvent('wudict-work', {detail:{...s,text:text.textContent}}));
  }
  async function refresh(id = '') {
    const r = await request('/api/index-work'+(id?'?id='+encodeURIComponent(id):''));
    if (!r.ok) throw new Error(await r.text());
    const s = await r.json(); if(!starting||s.running)render(s); return s;
  }
  async function stop() {
    if (!status?.running || pending) return;
    pending = true; render(status);
    try { const r = await request('/api/index-work?id=all', {method:'DELETE'}); if (!r.ok) throw new Error(await r.text()); render(await r.json()); }
    finally { pending = false; }
  }
  button.onclick = () => stop().catch(() => {button.disabled=false;});
  async function wait(id) {
    for (;;) {
      await new Promise(resolve => setTimeout(resolve, 700));
      let s; try { s = await refresh(id); } catch { continue; }
      if (!s.running) return s;
    }
  }
  window.wudictWork = {stop, refresh, wait, async start(operations, single=false) {
    starting=true;
    render({id:'starting',running:true,exclusive:!single,busyDicts:single?[operations[0].dict]:[]});
    let started;
    try{
    const r = await fetch('/api/index-work'+(single?'?single=1':''), {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(operations)});
    if (!r.ok) throw new Error(await r.text());
    started=await r.json();
    }catch(error){starting=false;await refresh().catch(()=>render({running:false,exclusive:false,busyDicts:[]}));throw error;}
    starting=false;render(started);
    return wait(started.id);
  }};
  async function poll() {
    try { await refresh(); } catch { if (status?.running) text.textContent = t('dictUI.reconnecting'); }
    setTimeout(poll, 1000);
  }
  new ResizeObserver(() => {if(status?.running)document.documentElement.style.setProperty('--workh',`${box.getBoundingClientRect().height+8}px`);}).observe(box);
  try {const previous=JSON.parse(sessionStorage.getItem(cacheKey));if(previous?.running)render(previous);}catch{}
  poll();
})();
