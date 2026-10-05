/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */
(() => {
  const t = (key, params) => window.wudictI18n.t(key, params);
  const style = document.createElement('style');
  style.textContent = `#serverWorkNotice{position:fixed;bottom:var(--wd-vv-bottom,0px);left:8px;right:8px;z-index:9999;box-sizing:border-box;padding:8px 10px calc(8px + env(safe-area-inset-bottom,0px));border:1px solid #d5ae70;border-radius:8px;background:repeating-linear-gradient(135deg,#fff1d9 0px,#fff1d9 8px,#f8e6c7 8px,#f8e6c7 16px);color:#483c2b;font:14px sans-serif;display:flex;align-items:center;gap:10px;letter-spacing:0}#serverWorkNotice[hidden]{display:none}#serverWorkNotice span{flex:1;min-width:0}#serverWorkNotice button{flex:none;font:inherit;color:inherit;padding:3px 9px;border:1px solid #c9b28d;border-radius:6px;background:#fff1d9}html.server-work-active #workNotice{display:none!important}`;
  document.head.append(style);
  style.textContent += `html.server-work-active body>.card{margin-bottom:var(--workh,0px)}@media(max-width:600px){html.server-work-active dialog.group-dialog.panel-card{bottom:calc(1em + var(--wd-inset-bottom,0px) + var(--workh,0px))}}`;
  const box = document.createElement('div');
  box.id = 'serverWorkNotice'; box.hidden = true; box.setAttribute('role', 'status');
  const text = document.createElement('span'), button = document.createElement('button');
  button.type = 'button'; box.append(text, button); document.body.append(box);
  let status = null, pending = false;
  const locked = new Set();
  function lockControls(s) {
    const ids=new Set(s.busyDicts||[]),any=s.exclusive||ids.size>0;
    const targets=new Set();
    const mark=selector=>document.querySelectorAll(selector).forEach(el=>targets.add(el));
    if(any)mark('#bulkIndexes button,#bulkIndexes select,#clearDatabaseGo,#rescanActions input,#rescanActions select,#reindexGo,.pd .rm,.pd [data-rm="all"],.pd [data-rm="source"],#addRow,#rows input,#rows button,#save,#indexDefaults input,#choose,#fetch,#install,#file,#url,#howtoCopy,#howtoBack');
    document.querySelectorAll('.pd [data-feat],.pd [data-delete-index],.pd [data-rm="index"],.upgrade').forEach(el=>{
      const id=el.dataset.target||el.dataset.id||el.closest('.pd')?.dataset.id;
      if(s.exclusive||ids.has(id))targets.add(el);
    });
    for(const el of locked)if(!targets.has(el)){el.removeAttribute('data-work-locked');el.inert=false;locked.delete(el);}
    for(const el of targets){el.setAttribute('data-work-locked','');el.inert=true;locked.add(el);}
  }
  style.textContent+='[data-work-locked]{opacity:.45!important;pointer-events:none!important;cursor:default!important}';
  document.addEventListener('click',event=>{if(event.target.closest('[data-work-locked]')){event.preventDefault();event.stopImmediatePropagation();}},true);
  new MutationObserver(()=>{if(status)lockControls(status);}).observe(document.body,{childList:true,subtree:true});
  // A modal dialog lives above ordinary z-index layers. Keep the footer inside
  // the most recently opened modal so it remains visible and its Stop is usable.
  const dialogs = [];
  function place() {
    const parent = dialogs.filter(d => d.open && d.isConnected).at(-1) || document.body;
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
    lockControls(s);
    box.hidden = !s.running;
    document.documentElement.classList.toggle('server-work-active', s.running);
    if (s.running) {
      const action = t(s.action === 'remove' ? 'dictUI.removing' : s.action === 'update' ? 'database.actionUpdate' : 'dictUI.indexing');
      const indexes = (s.indexes || []).map(f => t(f === 'index' ? 'dictUI.indexPlain' : f === 'contains' ? 'dictUI.contains' : 'dictUI.fts')).join(', ');
      const count = s.currentTotal > 0 ? ` ${s.currentDone.toLocaleString()}/${s.currentTotal.toLocaleString()} (${Math.min(100,Math.floor(s.currentDone*100/s.currentTotal))}%)` : s.currentDone > 0 ? ` ${s.currentDone.toLocaleString()}` : '';
      text.textContent = s.id==='folder-setup' ? t('pages.savingFolders') : `${Math.min(s.done+1,s.total)}/${s.total} · ${s.current || ''} — ${action} ${indexes}${count}`;
      if(s.id==='rescan-indexes' && s.stage!=='dictionary')text.textContent=t(s.stage==='cleanup'?'database.cleaning':'database.working');
      if((s.busyDicts||[]).length>1)text.textContent=t('dictUI.activeJobs',{number:s.busyDicts.length})+' · '+text.textContent;
      button.disabled = s.stopRequested || pending;
      button.textContent = t(button.disabled ? 'dictUI.bulkStopping' : 'dictUI.stop');
    }
    place();
    if (s.running) document.documentElement.style.setProperty('--workh', `${box.getBoundingClientRect().height + 8}px`);
    else if (!document.getElementById('workNotice') || document.getElementById('workNotice').hidden) document.documentElement.style.setProperty('--workh','0px');
    window.dispatchEvent(new CustomEvent('wudict-work', {detail:{...s,text:text.textContent}}));
  }
  async function refresh(id = '') {
    const r = await fetch('/api/index-work'+(id?'?id='+encodeURIComponent(id):''));
    if (!r.ok) throw new Error(await r.text());
    const s = await r.json(); render(s); return s;
  }
  async function stop() {
    if (!status?.running || pending) return;
    pending = true; render(status);
    try { const r = await fetch('/api/index-work?id=all', {method:'DELETE'}); if (!r.ok) throw new Error(await r.text()); render(await r.json()); }
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
    const r = await fetch('/api/index-work'+(single?'?single=1':''), {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(operations)});
    if (!r.ok) throw new Error(await r.text());
    const started=await r.json();render(started);
    return wait(started.id);
  }};
  async function poll() {
    try { await refresh(); } catch { if (status?.running) text.textContent = t('dictUI.reconnecting'); }
    setTimeout(poll, 1000);
  }
  new ResizeObserver(() => {if(status?.running)document.documentElement.style.setProperty('--workh',`${box.getBoundingClientRect().height+8}px`);}).observe(box);
  poll();
})();
