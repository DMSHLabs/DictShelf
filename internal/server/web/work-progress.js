/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */
(() => {
  if (document.documentElement.hasAttribute('data-workspace-dialog') &&
      window.parent !== window && window.parent.wudictWork) {
    window.wudictWork = window.parent.wudictWork;
    window.wudictTasks = window.parent.wudictTasks;
    return;
  }
  const t = (key, params) => window.wudictI18n.t(key, params);
  const style = document.createElement('style');
  style.textContent = `#serverWorkNotice{position:fixed;bottom:var(--wd-vv-bottom,0px);left:8px;right:8px;z-index:9999;box-sizing:border-box;padding:8px 10px calc(8px + env(safe-area-inset-bottom,0px));display:flex;align-items:center;gap:8px;flex-wrap:wrap;color:var(--fg,var(--fg-main,#222));font:14px sans-serif;letter-spacing:0}#serverWorkNotice[hidden]{display:none}#serverWorkNotice .work-spinner{box-sizing:border-box;flex:none;width:16px;height:16px;border:2px solid color-mix(in srgb,var(--accent,#865000) 28%,transparent);border-top-color:var(--accent,#865000);border-radius:50%;animation:work-spin .8s linear infinite}#serverWorkNotice span{flex:1 1 220px;min-width:0;overflow-wrap:anywhere}#serverWorkNotice button{flex:0 1 auto;max-width:100%;white-space:normal;overflow-wrap:anywhere;font:inherit;color:var(--label,var(--fg,#222));padding:3px 9px;border:1px solid var(--line,#b9a98e);border-radius:8px;background:var(--bg-card,var(--bg,#fff));appearance:none;-webkit-appearance:none}#serverWorkNotice button:disabled{opacity:.8}html.server-work-active #workNotice{display:none!important}@keyframes work-spin{to{transform:rotate(360deg)}}@media(prefers-reduced-motion:reduce){#serverWorkNotice .work-spinner{animation:none;border-color:var(--accent,#865000)}}`;
  style.textContent += `#serverWorkNotice{margin:0 6px 6px;border:1px solid var(--accent,var(--line,#b9a98e));border-radius:10px;background-color:var(--paper-bg,var(--bg,#fff));background-image:repeating-linear-gradient(135deg,color-mix(in srgb,var(--accent,#865000) 13%,transparent) 0px,color-mix(in srgb,var(--accent,#865000) 13%,transparent) 7px,transparent 7px,transparent 15px);box-shadow:var(--shadow,0 2px 10px #0003)}#serverWorkBacking{position:fixed;left:0;right:0;bottom:var(--wd-vv-bottom,0px);height:var(--workh,0px);z-index:9998;background-color:var(--paper-bg,var(--bg,#fff));background-image:var(--paper-bk-image,none);background-size:100vw 100vh;background-position:center bottom;background-repeat:no-repeat;pointer-events:none}#serverWorkBacking[hidden]{display:none}`;
  style.textContent += `#serverWorkNotice{flex-wrap:wrap}#serverWorkNotice span{flex:1 1 220px;overflow-wrap:anywhere}#serverWorkNotice button{flex:0 1 auto;max-width:100%;white-space:normal;overflow-wrap:anywhere}`;
  style.textContent += `html.server-work-active body>.card{margin-bottom:var(--workh,0px)}html.server-work-active #panel.show{bottom:var(--workh,0px)}@media(max-width:600px){html.server-work-active dialog.group-dialog.panel-card{bottom:calc(16px + var(--wd-inset-bottom,0px) + var(--workh,0px))}}`;
  // Browse is a full-height flex workspace. Keep its one shared task panel in
  // normal flow at the bottom so it takes space from the word pane instead of
  // covering the page controls or the list. The fixed backing is only for
  // surfaces where the notice floats over the app.
  style.textContent += `html.server-work-active body.workspace-browse>#serverWorkNotice{position:static;inset:auto;flex:none;width:auto;margin:0 6px 6px}body.workspace-browse>#serverWorkBacking{display:none!important}`;
  style.textContent += `@media(min-width:601px){html.server-work-active dialog:is(#groupEditor,#dictSettings,#webDictionaryPicker):has(> #serverWorkNotice){box-sizing:border-box;padding-bottom:var(--workh,0px)}}html.server-work-active .mask-dock{bottom:var(--workh,0px)}`;
  style.textContent += `html.server-work-active #bulkIndexStatus,html.server-work-active #reindexLine,html.server-work-active #clearDatabaseStatus,html.server-work-active #clearDatabaseProgress{display:none!important}`;
  style.textContent += `[data-work-locked]{opacity:.45!important;pointer-events:none!important;cursor:default!important}`;
  style.textContent += `#serverWorkNotice .work-summary{display:flex;align-items:center;gap:8px;flex:1 1 100%;min-width:0}#serverWorkNotice .work-toggle{flex:none;width:1.8em;padding:0!important;font-size:1.2em;line-height:1.2}#serverWorkNotice .work-summary-text{flex:1 1 auto;min-width:0;overflow-wrap:anywhere}#serverWorkNotice .work-pulse,#serverWorkNotice .work-task-progress{height:4px;overflow:hidden;border-radius:9px;background:color-mix(in srgb,var(--accent,#865000) 18%,transparent)}#serverWorkNotice .work-pulse{flex:1 1 100%;min-width:100%;margin:0 0 0 0}#serverWorkNotice .work-pulse::after{content:"";display:block;width:42%;height:100%;background:linear-gradient(90deg,transparent,color-mix(in srgb,var(--accent,#865000) 72%,transparent),transparent);animation:work-pulse 1.8s ease-in-out infinite}#serverWorkNotice .work-task-list{flex:1 1 100%;min-width:0;max-height:min(32vh,280px);overflow-y:auto;overscroll-behavior:contain}#serverWorkNotice .work-task-row{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:4px 8px;padding:5px 0;border-top:1px solid color-mix(in srgb,var(--line,#b9a98e) 60%,transparent)}#serverWorkNotice .work-task-label{min-width:0;overflow-wrap:anywhere}#serverWorkNotice .work-task-progress{grid-column:1/-1}#serverWorkNotice .work-task-progress>span{display:block;height:100%;background:var(--accent,#865000)}#serverWorkNotice .work-task-stop{grid-column:2;grid-row:1;font-size:.9em;padding:1px 7px}@keyframes work-pulse{0%{transform:translateX(-110%)}100%{transform:translateX(240%)}}@media(prefers-reduced-motion:reduce){#serverWorkNotice .work-pulse::after{animation:none;width:100%;background:color-mix(in srgb,var(--accent,#865000) 38%,transparent)}}`;
  style.textContent += `#serverWorkNotice .work-task-progress>span[data-indeterminate]{width:42%!important;opacity:1!important;background:linear-gradient(90deg,transparent,color-mix(in srgb,var(--accent,#865000) 72%,transparent),transparent)!important;animation:work-pulse 1.8s ease-in-out infinite}@media(prefers-reduced-motion:reduce){#serverWorkNotice .work-task-progress>span[data-indeterminate]{width:100%!important;animation:none;background:color-mix(in srgb,var(--accent,#865000) 38%,transparent)!important}}`;
  style.textContent += `#serverWorkNotice .work-single-progress{display:block;flex:1 1 100%;width:100%;height:12px;box-sizing:border-box;overflow:hidden;border-radius:9px;background:color-mix(in srgb,var(--accent,#865000) 18%,transparent)}#serverWorkNotice .work-single-progress[hidden]{display:none}#serverWorkNotice .work-single-progress>span{display:block;height:100%;background:var(--accent,#865000);transition:width .12s linear}`;
  style.textContent += `#serverWorkNotice .work-summary{display:grid;grid-template-columns:max-content minmax(0,1fr);align-items:center;gap:8px;width:100%;box-sizing:border-box}#serverWorkNotice .work-toggle{grid-column:1;grid-row:1;box-sizing:border-box;width:max-content;max-width:100%;min-width:0;flex:none;white-space:normal;text-align:center;overflow-wrap:anywhere;padding-inline:8px!important}#serverWorkNotice .work-summary-text{grid-column:2;grid-row:1}#serverWorkNotice .work-pulse-row{grid-column:1/-1;grid-row:2;display:flex;align-items:center;gap:8px;min-width:0;width:100%}#serverWorkNotice .work-pulse{flex:1 1 auto;width:auto;min-width:0;max-width:none;height:12px;box-sizing:border-box;margin:0}#serverWorkNotice .work-pulse::after{width:42%}#serverWorkNotice .work-stop-all{flex:none;white-space:nowrap;overflow-wrap:normal}#serverWorkNotice .work-task-list{display:block;flex:0 1 100%;align-self:stretch;width:100%;height:auto;min-height:1.5em;box-sizing:border-box}#serverWorkNotice .work-task-label{grid-column:1/-1;display:flex;justify-content:flex-start;align-items:baseline;gap:2px 6px;flex-wrap:wrap;min-width:0}#serverWorkNotice .work-task-name{flex:0 1 auto;min-width:0;white-space:normal;overflow-wrap:anywhere}#serverWorkNotice .work-task-count{flex:none;white-space:nowrap;font-variant-numeric:tabular-nums}#serverWorkNotice .work-task-progress{grid-column:1;grid-row:2;height:12px;width:100%;box-sizing:border-box}#serverWorkNotice .work-task-stop{grid-column:2;grid-row:2;flex:none;white-space:nowrap;overflow-wrap:normal;font-size:.9em;padding:1px 7px}#serverWorkNotice .work-task-progress>span[data-indeterminate]{width:42%!important}`;
  style.textContent += `#serverWorkNotice .work-summary[hidden],#serverWorkNotice .work-task-list[hidden]{display:none!important}`;
  document.head.append(style);
  const backing = document.createElement('div');
  backing.id = 'serverWorkBacking'; backing.hidden = true; backing.setAttribute('aria-hidden','true');
  const box = document.createElement('div');
  box.id = 'serverWorkNotice'; box.hidden = true; box.setAttribute('role', 'status');
  const spinner = document.createElement('span'), text = document.createElement('span'), button = document.createElement('button');
  const summary = document.createElement('div'), toggle = document.createElement('button'), summaryText = document.createElement('span');
  const pulseRow = document.createElement('div'), pulse = document.createElement('div'), stopAll = document.createElement('button'), taskList = document.createElement('div');
  const singleProgress=document.createElement('div'),singleProgressBar=document.createElement('span');
  spinner.className = 'work-spinner'; spinner.setAttribute('aria-hidden','true');
  button.type = 'button';
  summary.className='work-summary';summary.hidden=true;
  toggle.type='button';toggle.className='work-toggle';toggle.setAttribute('aria-expanded','false');
  summaryText.className='work-summary-text';pulseRow.className='work-pulse-row';pulse.className='work-pulse';pulse.setAttribute('aria-hidden','true');
  stopAll.type='button';stopAll.className='work-stop-all';
  singleProgress.className='work-single-progress';singleProgress.hidden=true;singleProgress.setAttribute('role','progressbar');singleProgress.setAttribute('aria-valuemin','0');singleProgressBar.style.width='0%';singleProgress.append(singleProgressBar);
  taskList.className='work-task-list';taskList.hidden=true;
  pulseRow.append(pulse,stopAll);summary.append(toggle,summaryText,pulseRow);box.append(spinner,text,button,singleProgress,summary,taskList);document.body.append(backing,box);
  let status = null, pending = false, starting = false, refreshSequence = 0, waiting = 0;
  const tasks=new Map(),renderedProgress=new Map();let nextTaskID=0,expanded=false,indexTaskID=0,renderQueued=false;
  function taskProgress(task){
    const values=[];
    if(task.total>0)values.push(Math.max(0,Math.min(100,task.done*100/task.total)));
    for(const child of task.children||[]){
      const percent=Number.isFinite(child.percent)?child.percent:child.total>0?child.done*100/child.total:null;
      if(Number.isFinite(percent))values.push(Math.max(0,Math.min(100,percent)));
    }
    return values;
  }
  function taskProgressKey(task){return taskProgress(task).length+':'+(task.children||[]).map(child=>child.label+':'+(child.active??'')).join('\u0000')}
  function shouldRenderTaskProgress(task){
    const now=performance.now(),values=taskProgress(task),key=taskProgressKey(task);
    const previous=renderedProgress.get(task.id);
    if(!previous||previous.key!==key)return true;
    if(values.length)return values.some((value,index)=>value-previous.values[index]>=1||value>=100&&previous.values[index]<100);
    return now-previous.at>=200;
  }
  function rememberTaskProgress(task){renderedProgress.set(task.id,{at:performance.now(),values:taskProgress(task),key:taskProgressKey(task)})}
  function scheduleTaskRender(){
    if(renderQueued)return;
    renderQueued=true;
    requestAnimationFrame(()=>{renderQueued=false;renderTasks()});
  }
  const taskTypeKeys={indexing:'work.task.indexing',wildcards:'work.task.wildcards',browse:'work.task.browse',rescan:'work.task.rescan',setup:'work.task.setup'};
  function taskStart(type,label,options={}){
    type=String(type||'work').slice(0,64);
    let id;
    do{id=(nextTaskID+1)>>>0;nextTaskID=id}while(id===0||tasks.has(id));
    tasks.set(id,{id,type,label:String(label||''),message:'',done:Math.max(0,Number(options.done)||0),total:Math.max(0,Number(options.total)||0),children:Array.isArray(options.children)?options.children.map(child=>typeof child==='string'?{label:child}:child).filter(child=>child&&child.label).map(child=>({label:String(child.label),done:Math.max(0,Number(child.done)||0),total:Math.max(0,Number(child.total)||0),percent:Number.isFinite(child.percent)?Math.max(0,Math.min(100,Number(child.percent))):null,active:typeof child.active==='boolean'?child.active:undefined,stop:typeof child.stop==='function'?child.stop:null,stopRequested:!!child.stopRequested})):[],stop:options.stop||null,stopRequested:!!options.stopRequested});
    rememberTaskProgress(tasks.get(id));renderTasks();return id;
  }
  function taskUpdate(id,patch={}){
    const task=tasks.get(Number(id));if(!task||!patch||typeof patch!=='object')return false;
    for(const key of ['type','label','message'])if(Object.hasOwn(patch,key))task[key]=String(patch[key]||'');
    if(Object.hasOwn(patch,'children'))task.children=Array.isArray(patch.children)?patch.children.map(child=>typeof child==='string'?{label:child}:child).filter(child=>child&&child.label).map(child=>({label:String(child.label),done:Math.max(0,Number(child.done)||0),total:Math.max(0,Number(child.total)||0),percent:Number.isFinite(child.percent)?Math.max(0,Math.min(100,Number(child.percent))):null,active:typeof child.active==='boolean'?child.active:undefined,stop:typeof child.stop==='function'?child.stop:null,stopRequested:!!child.stopRequested})):[];
    if(Object.hasOwn(patch,'done'))task.done=Math.max(0,Number(patch.done)||0);
    if(Object.hasOwn(patch,'total'))task.total=Math.max(0,Number(patch.total)||0);
    if(Object.hasOwn(patch,'stopRequested'))task.stopRequested=!!patch.stopRequested;
    if(Object.hasOwn(patch,'stop'))task.stop=typeof patch.stop==='function'?patch.stop:null;
    if(shouldRenderTaskProgress(task)){rememberTaskProgress(task);scheduleTaskRender()}return true;
  }
  function taskFinish(id){id=Number(id);const removed=tasks.delete(id);renderedProgress.delete(id);if(removed)renderTasks();return removed}
  function taskFinishMatching(type,label){
    let removed=false;
    for(const [id,task] of tasks)if(task.type===type&&(task.label===label||task.message===label)){tasks.delete(id);renderedProgress.delete(id);removed=true}
    if(removed)renderTasks();
    return removed;
  }
  function taskName(type){const key=taskTypeKeys[type];return key?t(key):type}
  function taskText(task){return task.message||task.label||taskName(task.type)}
  async function taskStop(id){
    const task=tasks.get(id);if(!task?.stop||task.stopRequested)return;
    task.stopRequested=true;renderTasks();
    try{await task.stop(id)}catch(_){task.stopRequested=false;renderTasks()}
  }
  async function taskStopChild(task){
    if(!task?.stop||task.stopRequested)return;
    task.stopRequested=true;renderTasks();
    try{await task.stop()}catch(_){task.stopRequested=false;renderTasks()}
  }
  async function taskStopAll(active){
    const stoppable=[...new Map(active.filter(task=>task.stop&&!task.stopRequested).map(task=>[task.id,task])).values()];
    if(!stoppable.length)return;
    stoppable.forEach(task=>task.stopRequested=true);renderTasks();
    const results=await Promise.allSettled(stoppable.map(task=>task.stop(task.id)));
    if(results.some(result=>result.status==='rejected'))stoppable.forEach((task,index)=>{if(results[index]?.status==='rejected')task.stopRequested=false});
    renderTasks();
  }
  function renderTasks(){
    const active=[...tasks.values()],running=active.length>0;
    let detailCount=0,trackedChildren=0,activeChildren=0;
    for(const task of active){
      if(task.children?.length){detailCount+=task.children.length;for(const child of task.children)if(typeof child.active==='boolean'){trackedChildren++;if(child.active)activeChildren++}}
      else detailCount++;
    }
    // Wildcards is an aggregate operation even when its child rows are between
    // updates (or only one dictionary is selected). Keep its disclosure stable
    // so a progress refresh cannot replace the Show tasks control mid-click.
    const multiple=detailCount>1||active.some(task=>task.type==='wildcards');
    box.hidden=!running;backing.hidden=!running;
    document.documentElement.classList.toggle('server-work-active',running);
    const single=active.length===1&&!multiple;box.classList.toggle('work-multiple',multiple&&running);
    spinner.hidden=!single;text.hidden=!single;button.hidden=!single||!active[0]?.stop;
    const singleBrowse=single&&active[0].type==='browse'&&active[0].total>0;
    singleProgress.hidden=!singleBrowse;
    if(singleBrowse){
      const task=active[0],percent=Math.max(0,Math.min(100,task.done*100/task.total));
      singleProgressBar.style.width=`${percent}%`;
      singleProgress.setAttribute('aria-valuemax',String(task.total));
      singleProgress.setAttribute('aria-valuenow',String(Math.min(task.done,task.total)));
      singleProgress.setAttribute('aria-label',`${window.wudictI18n.number(task.done)} ${t('dictUI.progressOf',{total:window.wudictI18n.number(task.total)})}`);
    }
    summary.hidden=!multiple;taskList.hidden=!multiple||!expanded;
    if(single){
      const task=active[0];text.textContent=taskText(task);button.disabled=task.stopRequested||(task.id===indexTaskID&&(pending||starting));
      button.textContent=t(button.disabled?'dictUI.bulkStopping':'dictUI.stop');button.onclick=()=>taskStop(task.id);
    }else if(multiple){
      const counts=new Map();for(const task of active)counts.set(task.type,(counts.get(task.type)||0)+1);
      const activeCount=trackedChildren?activeChildren:detailCount;
      summaryText.textContent=active.length===1?`${taskName(active[0].type)} · ${window.wudictI18n.number(activeCount)}`:[...counts].map(([type,count])=>`${taskName(type)} (${window.wudictI18n.number(count)})`).join(', ');
      toggle.hidden=false;toggle.textContent=(expanded?'▾ ':'▸ ')+t(expanded?'work.collapse':'work.expand');toggle.setAttribute('aria-expanded',String(expanded));
      toggle.setAttribute('aria-label',t(expanded?'work.collapse':'work.expand'));
      const stoppable=active.filter(task=>task.stop);
      const singleParent=active.length===1&&stoppable.length===1;
      stopAll.hidden=stoppable.length===0;
      stopAll.disabled=stoppable.length===0||stoppable.every(task=>task.stopRequested||(task.id===indexTaskID&&(pending||starting)));
      stopAll.textContent=t(stoppable.some(task=>task.stopRequested)?'dictUI.bulkStopping':singleParent?'dictUI.stop':'work.stopAll');
      stopAll.onclick=()=>singleParent?taskStop(active[0].id):taskStopAll(active);
      if(expanded){
        const details=active.flatMap(task=>task.children?.length?task.children.map(child=>({...child,parent:task})): [{...task,parent:task}]);
        taskList.replaceChildren(...details.map(task=>{
        const row=document.createElement('div');row.className='work-task-row';
        const label=document.createElement('span');label.className='work-task-label';
        const name=document.createElement('span');name.className='work-task-name';name.textContent=task.label||taskText(task.parent);label.append(name);
        if(Number.isFinite(task.percent)){const count=document.createElement('span');count.className='work-task-count';count.textContent=`${Math.round(task.percent)}%`;label.append(count)}
        else if(task.total>0||task.done>0){const count=document.createElement('span');count.className='work-task-count';count.textContent=task.total>0?`${window.wudictI18n.number(task.done)} ${t('dictUI.progressOf',{total:window.wudictI18n.number(task.total)})}`:window.wudictI18n.number(task.done);label.append(count)}
        row.append(label);
        const track=document.createElement('div');track.className='work-task-progress';
        const bar=document.createElement('span');bar.style.width=Number.isFinite(task.percent)?`${task.percent}%`:task.total>0?`${Math.max(0,Math.min(100,task.done*100/task.total))}%`:'100%';
        if(!Number.isFinite(task.percent)&&task.total<=0)bar.dataset.indeterminate='';track.append(bar);row.append(track);
        if(task.stop){const stop=document.createElement('button');stop.type='button';stop.className='work-task-stop';stop.textContent=t(task.stopRequested?'dictUI.bulkStopping':'dictUI.stop');stop.disabled=task.stopRequested;stop.onclick=()=>taskStopChild(task);row.append(stop)}
        return row;
        }));
      }
    }
    if(!running)expanded=false;
    if(!running&&taskList.childElementCount)taskList.replaceChildren();
    place();
    if(!running&&(!document.getElementById('workNotice')||document.getElementById('workNotice').hidden))document.documentElement.style.setProperty('--workh','0px');
    window.dispatchEvent(new CustomEvent('wudict-work-tasks',{detail:{tasks:active.map(({id,type,label,message,done,total})=>({id,type,label,message,done,total}))}}));
  }
  toggle.onclick=()=>{expanded=!expanded;renderTasks()};
  window.wudictTasks={start:taskStart,update:taskUpdate,finish:taskFinish,finishMatching:taskFinishMatching,activeCount:()=>tasks.size};
  window.dispatchEvent(new CustomEvent('wudict-tasks-ready'));
  const cacheKey='wudict-active-work';
  async function request(url, options={}) {
    if(window.wudictAndroidGo)throw new Error('Android work operation is not connected to the Go bridge');
    const controller=new AbortController();
    const timer=setTimeout(()=>controller.abort(),10000);
    try {return await fetch(url,{...options,signal:controller.signal});}
    finally {clearTimeout(timer);}
  }
  function androidIndexWork(action,id='',body='') {
    const result=JSON.parse(window.wudictAndroidGo.indexWork(action,id,body));
    if(result.status<200||result.status>=300)throw new Error(result.body?.error||'index operation failed');
    return result.body;
  }
  function nativeWorkCall(method,argument=''){
    const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),10000);
    return window.wudictNativeCall(method,argument,undefined,controller.signal).finally(()=>clearTimeout(timer));
  }
  const locked = new Set();
  function lockControls(s) {
    const ids=new Set([...(s.busyDicts||[]),...(s.pendingDicts||[])]),any=s.exclusive||ids.size>0;
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
    if (s.running) {
      const action = t(s.action === 'remove' ? 'dictUI.removing' : s.action === 'update' ? 'database.actionUpdate' : 'dictUI.indexing');
      const indexes = (s.indexes || []).map(f => t(f === 'index' ? 'dictUI.indexPlain' : f === 'contains' ? 'dictUI.contains' : 'dictUI.fts')).join(', ');
      const count = s.currentTotal > 0 ? ` ${window.wudictI18n.number(s.currentDone)} ${t('dictUI.progressOf',{total:window.wudictI18n.number(s.currentTotal)})}` : s.currentDone > 0 ? ` ${window.wudictI18n.number(s.currentDone)}` : '';
      const queueCount = s.id==='bulk-indexes' || s.id==='initial-index-queue' || s.id==='rescan-indexes' || s.id==='reindex' || s.id==='setup-work';
      const position = queueCount && s.total > 1 ? `${Math.min(s.done+1,s.total)}/${s.total} · ` : '';
      let message = s.id==='starting' ? t('database.working') : s.id==='folder-setup' ? t('pages.savingFolders') : `${position}${s.current || ''} — ${action} ${indexes}${count}`;
      if(s.id==='rescan-indexes' && s.stage!=='dictionary')message=t(s.stage==='cleanup'?'database.cleaning':'database.working');
      if((s.busyDicts||[]).length>1)message=t('dictUI.activeJobs',{number:s.busyDicts.length})+' · '+message;
      const taskType=s.id==='folder-setup'?'setup':s.id==='rescan-indexes'?'rescan':'indexing';
      if(!indexTaskID)indexTaskID=taskStart(taskType,message,{stop});
      taskUpdate(indexTaskID,{type:taskType,message,done:s.currentDone||0,total:s.currentTotal||0,stopRequested:!!s.stopRequested,stop:s.id==='starting'?null:stop});
    } else if(indexTaskID) {
      taskFinish(indexTaskID);indexTaskID=0;
    }
    place();
    window.dispatchEvent(new CustomEvent('wudict-work', {detail:{...s,taskID:indexTaskID,text:s.running?(tasks.get(indexTaskID)?.message||''):''}}));
  }
  async function refresh(id = '', shouldRender = true) {
    const sequence = ++refreshSequence;
    if (window.wudictAndroidGo) {
      const s=androidIndexWork('get',id||'');
      if(shouldRender&&sequence===refreshSequence&&(!starting||s.running))render(s);
      return s;
    }
    if (typeof window.wudictNativeCall === 'function') {
      const s = await nativeWorkCall('indexWork', id || '');
      if(shouldRender&&sequence===refreshSequence&&(!starting||s.running))render(s);
      return s;
    }
    const r = await request('/api/index-work'+(id?'?id='+encodeURIComponent(id):''));
    if (!r.ok) throw new Error(await r.text());
    const s = await r.json();
    if(shouldRender&&sequence===refreshSequence&&(!starting||s.running))render(s);
    return s;
  }
  async function stop() {
    if (!status?.running || pending) return;
    pending = true; render(status);
    try {
      if(window.wudictAndroidGo)render(androidIndexWork('stop'));
      else if(typeof window.wudictNativeCall==='function')render(await nativeWorkCall('stopIndexWork'));
      else { const r = await request('/api/index-work?id=all', {method:'DELETE'}); if (!r.ok) throw new Error(await r.text()); render(await r.json()); }
    }
    finally { pending = false; }
  }
  async function stopDictionary(jobID,dictID){
    if(!status?.running||!jobID||!dictID)throw new Error('dictionary operation is no longer active');
    let result;
    if(window.wudictAndroidGo)result=androidIndexWork('stopDictionary',jobID,dictID);
    else if(typeof window.wudictNativeCall==='function')result=await nativeWorkCall('stopIndexWork',JSON.stringify({id:jobID,dict:dictID}));
    else{
      const query=new URLSearchParams({id:jobID,dict:dictID});
      const response=await request('/api/index-work?'+query.toString(),{method:'DELETE'});
      if(!response.ok)throw new Error(await response.text());
      result=await response.json();
    }
    render(result);return result;
  }
  button.onclick = () => stop().catch(() => {button.disabled=false;});
  async function wait(id) {
    waiting++;
    try { for (;;) {
      await new Promise(resolve => setTimeout(resolve, 700));
      let s; try {
        s = await refresh(id,id!=='folder-setup');
        if(id==='folder-setup')await refresh();
      } catch { continue; }
      if (!s.running) return s;
    }} finally { waiting--; }
  }
  async function complete(id) {
    // A terminal response belongs to this exact job. Clear its banner
    // immediately, then refresh the aggregate status in case another job is
    // already active.
    if(status?.id===id)render({...status,running:false,exclusive:false,busyDicts:[]});
    return refresh();
  }
  window.wudictWork = {stop, stopDictionary, refresh, wait, complete, async start(operations, single=false) {
    starting=true;
    render({id:'starting',running:true,exclusive:!single,busyDicts:single?[operations[0].dict]:[]});
    let started;
    try{
    const body=JSON.stringify(operations);
    if(window.wudictAndroidGo)started=androidIndexWork(single?'startSingle':'start','',body);
    else if(typeof window.wudictNativeCall==='function')started=await nativeWorkCall(single?'startSingleIndexWork':'startIndexWork',body);
    else {
      const r = await request('/api/index-work'+(single?'?single=1':''), {method:'POST',headers:{'Content-Type':'application/json'},body});
      if (!r.ok) throw new Error(await r.text());
      started=await r.json();
    }
    }catch(error){starting=false;await refresh().catch(()=>render({running:false,exclusive:false,busyDicts:[]}));throw error;}
    starting=false;render(started);
    const result=await wait(started.id);
    // `wait` has the terminal status for this exact job. Apply it directly so
    // a newer or delayed background poll cannot leave this job's panel behind.
    render({...result,running:false,exclusive:false,busyDicts:[]});
    await complete(started.id).catch(()=>{});
    return result;
  }};
  async function poll() {
    // The queue waiter owns progress polling while start() is active. Two
    // competing poll loops can continually supersede each other's replies.
    try { if(!waiting)await refresh(); } catch { if (status?.running) text.textContent = t('dictUI.reconnecting'); }
    setTimeout(poll, 1000);
  }
  new ResizeObserver(entries => {
    if(!status?.running)return;
    const entry=entries[entries.length-1];
    const borderBox=entry?.borderBoxSize;
    const height=borderBox?(Array.isArray(borderBox)?borderBox[0]?.blockSize:borderBox.blockSize):box.getBoundingClientRect().height;
    if(Number.isFinite(height))document.documentElement.style.setProperty('--workh',`${height+8}px`);
  }).observe(box);
  // Record whether window-resize events are waiting behind work on the WebView
  // main thread. Collapse a drag into one diagnostic record after it settles.
  let resizeBurst=null,resizeTimer=0;
  window.addEventListener('resize',event=>{
    if(!status?.running||typeof window.browseDiagnosticsEnabled!=='function'||!window.browseDiagnosticsEnabled())return;
    const now=performance.now(),eventTime=Number(event.timeStamp)||now,handlerDelay=Math.max(0,now-eventTime);
    if(!resizeBurst)resizeBurst={started:now,count:0,maxHandlerDelay:0,maxFrameDelay:0,maxPaintDelay:0};
    const burst=resizeBurst;burst.count++;burst.maxHandlerDelay=Math.max(burst.maxHandlerDelay,handlerDelay);
    requestAnimationFrame(()=>{
      const frameDelay=Math.max(0,performance.now()-eventTime);burst.maxFrameDelay=Math.max(burst.maxFrameDelay,frameDelay);
      requestAnimationFrame(()=>{burst.maxPaintDelay=Math.max(burst.maxPaintDelay,Math.max(0,performance.now()-eventTime))});
    });
    clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{
      if(resizeBurst!==burst)return;resizeBurst=null;
      const send=window.sendBrowseDiagnostic;
      if(typeof send==='function')send({event:'work_resize',durationMs:Math.round(performance.now()-burst.started),words:window.innerWidth,letters:window.innerHeight,active:burst.count,completed:burst.maxHandlerDelay,dictionaries:burst.maxFrameDelay,readMs:burst.maxPaintDelay,progressEvents:tasks.size});
    },350);
  },{passive:true});
  try {const previous=JSON.parse(sessionStorage.getItem(cacheKey));if(previous?.running)render(previous);}catch{}
  poll();
})();
