// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

(function(){
"use strict";
const foldersDialogRoot=document.getElementById("foldersDialog")||document;
const foldersDialogEmbedded=foldersDialogRoot!==document;
const wudictDialogMode=foldersDialogEmbedded||new URLSearchParams(location.search).get("wudict_dialog")==="1";
let foldersDialogReturnFocus=null;
function setupNavigation(path){const target=new URL(path,location.href);const params=new URLSearchParams(location.search);if(params.get("wudict_webview2")==="1")target.searchParams.set("wudict_webview2","1");if(wudictDialogMode)target.searchParams.set("wudict_dialog","1");return target.pathname+target.search+target.hash}
if(foldersDialogEmbedded){
  window.wudictFoldersDialogEmbedded=true;
  let folderPicked=null,archivePicked=null;
  if(typeof window.wudictChooseFolder==="function"){
    window.wudictPickFolder=callback=>{folderPicked=callback;return window.wudictChooseFolder()};
    document.documentElement.setAttribute("data-folder-picker","");
  }
  if(typeof window.wudictChooseArchive==="function")window.wudictPickArchive=callback=>{archivePicked=callback;window.wudictChooseArchive()};
  window.addEventListener("message",event=>{
    if(event.origin!==location.origin)return;
    if(event.data?.type==="wudict-folder-picked"){const callback=folderPicked;folderPicked=null;if(callback&&typeof event.data.path==="string")callback(event.data.path)}
    else if(event.data?.type==="wudict-archive-picked"){const callback=archivePicked;archivePicked=null;if(callback&&typeof event.data.path==="string")callback(event.data.path)}
  });
}
const $=id=>foldersDialogEmbedded?foldersDialogRoot.querySelector("#"+id):document.getElementById(id);
const tx=window.wudictI18n.t;
const nativeSetupCall=wudictDialogMode&&typeof parent.wudictNativeCall==="function"
  ? parent.wudictNativeCall.bind(parent) : window.wudictNativeCall;
function browserHTTP(url,options){
  if(window.wudictAndroidGo)throw new Error("Android setup operation is not connected to the Go bridge");
  return fetch(url,options);
}
function androidSetupCall(action,argument){
  const result=JSON.parse(window.wudictAndroidGo.setup(action,argument));
  if(!result.ok)throw new Error(result.error||"Folder setup failed");
  return result.value;
}
function androidIntakeCall(action,argument=""){
  const result=JSON.parse(window.wudictAndroidGo.intake(action,argument));
  if(!result.ok)throw new Error(result.error||"Archive import failed");
  return result.value;
}
let indexDefaultsWrite=Promise.resolve(true);
function syncIndexDefaults(){
  $("newIndexIndex").checked=true;
  $("newIndexIndex").disabled=true;
  $("indexDefaultsError").hidden=true;
  return true;
}
function saveIndexDefaults(){
  if(!syncIndexDefaults())return Promise.resolve(false);
  const body={index:true,contains:$("newIndexContains").checked,fullText:$("newIndexFullText").checked};
  indexDefaultsWrite=indexDefaultsWrite.then(async()=>{
    try{
      if(window.wudictAndroidGo)androidSetupCall("indexDefaults",JSON.stringify(body));
      else if(typeof nativeSetupCall==="function")await nativeSetupCall("indexDefaults",JSON.stringify(body));
      else {
        const r=await browserHTTP("/api/index-defaults",{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
        if(!r.ok)throw new Error(await r.text());
      }
      return true;
    }catch(e){
      $("indexDefaultsError").hidden=false;
      $("indexDefaultsError").textContent=tx("pages.indexDefaultsFailed",{error:window.wudictI18n.errorText(e.message)});
      return false;
    }
  });
  return indexDefaultsWrite;
}
$("indexDefaults").addEventListener("change",()=>{saveIndexDefaults()});
let timer=null,lastOK=false;
let validationController=null,validationRun=0;

let setupNoticeTaskID=0;
let setupNoticeMessage="",setupNoticeStop=null;
function setSetupNotice(text,stop){
  setupNoticeMessage=text||"";setupNoticeStop=typeof stop==="function"?stop:null;
  if(!window.wudictTasks)return;
  if(!setupNoticeMessage){if(setupNoticeTaskID)window.wudictTasks.finish(setupNoticeTaskID);setupNoticeTaskID=0;return}
  if(setupNoticeTaskID)window.wudictTasks.update(setupNoticeTaskID,{message:setupNoticeMessage,stop:setupNoticeStop});
  else setupNoticeTaskID=window.wudictTasks.start("setup",setupNoticeMessage,{stop:setupNoticeStop});
}
window.addEventListener("wudict-tasks-ready",()=>{if(setupNoticeMessage)setSetupNotice(setupNoticeMessage,setupNoticeStop)});

const fmtN=n=>window.wudictI18n.number(n);
const esc=s=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
const el=(tag,cls,txt)=>{const e=document.createElement(tag);if(cls)e.className=cls;
  if(txt!==undefined)e.textContent=txt;return e};

/* One row per folder. Each validates itself as you type (its own status sits
   at the end of the row, so three folders never argue over one message line);
   the shared line below reports only the total, which is what the Save button
   depends on. */
function addRow(value){
  const row=document.createElement("div");row.className="frow";
  row.innerHTML=`<input placeholder="${esc(tx("pages.pathPlaceholder"))}" spellcheck="false">
    <button type="button" class="pick" title="${esc(tx("pages.chooseFolder"))}">📁</button><span class="brk"></span>
    <span class="st"></span><button type="button" class="rm" title="${esc(tx("pages.removeFolder"))}">✕</button>`;
  const input=row.querySelector("input");
  if(value)input.value=value;
  input.addEventListener("input",()=>{clearTimeout(timer);timer=setTimeout(validateAll,350)});
  // The host's folder dialog (see .pick above): it answers later, with a path
  // or not at all, and the row it fills is the one whose 📁 was pressed.
  row.querySelector(".pick").addEventListener("click",()=>{
    if(typeof window.wudictPickFolder!=="function")return;
    window.wudictPickFolder(p=>{
      if(typeof p!=="string"||!(p=p.trim()))return;
      input.value=p;
      clearTimeout(timer);timer=setTimeout(validateAll,350);
    }).catch(e=>{
      const st=row.querySelector(".st");st.className="st err";
      st.textContent="✕ "+window.wudictI18n.errorText(e.message);
    });
  });
  input.addEventListener("keydown",e=>{if(e.key==="Enter"&&lastOK)save()});
  row.querySelector(".rm").addEventListener("click",()=>{
    row.remove();if(!$("rows").children.length)addRow("");syncRemoveButtons();validateAll();
  });
  $("rows").appendChild(row);
  syncRemoveButtons();
  return input;
}
// the ✕ on a lone row would only ever clear it — hide it until there are two
function syncRemoveButtons(){
  const rows=[...$("rows").children];
  for(const r of rows)r.querySelector(".rm").style.visibility=rows.length>1?"visible":"hidden";
}
function rowInputs(){return [...foldersDialogRoot.querySelectorAll("#rows input")]}
function paths(){return rowInputs().map(i=>i.value.trim()).filter(Boolean)}

async function validateAll(){
  const run=++validationRun;
  if(validationController)validationController.abort();
  const controller=new AbortController();
  validationController=controller;
  const stop=()=>{
    controller.abort();
    if(validationRun!==run)return;
    validationRun++;
    validationController=null;
    setSetupNotice("");
  };
  const inputs=rowInputs();
  let total=0,bad=0,checked=0;
  const folderTotal=inputs.filter(input=>input.value.trim()).length;
  let folderDone=0;
  const reportFolder=()=>{
    folderDone++;
    setSetupNotice(tx("pages.checkingFoldersProgress",{number:folderDone,total:folderTotal}),stop);
  };
  if(folderTotal)setSetupNotice(tx("pages.checkingFoldersProgress",{number:0,total:folderTotal}),stop);
  await Promise.all(inputs.map(async input=>{
    const st=input.parentElement.querySelector(".st");
    const p=input.value.trim();
    if(!p){st.className="st";st.textContent="";return}
    checked++;
    let j;
    try{ j=window.wudictAndroidGo
      ? androidSetupCall("folderSetup","path="+encodeURIComponent(p))
      : typeof nativeSetupCall==="function"
      ? await nativeSetupCall("folderSetup","path="+encodeURIComponent(p),undefined,controller.signal)
      : await (await browserHTTP("/api/setup?path="+encodeURIComponent(p),{signal:controller.signal})).json() }
    catch(e){ if(controller.signal.aborted||run!==validationRun)return;st.className="st err";st.textContent=tx("pages.unreachable");bad++;reportFolder();return }
    if(run!==validationRun)return;
    if(j.error){st.className="st err";st.textContent="✕ "+window.wudictI18n.errorText(j.error);bad++;reportFolder();return}
    if(!j.found){st.className="st err";st.textContent=tx("pages.noneFound");bad++;reportFolder();return}
    st.className="st ok";st.textContent=tx("pages.found",{number:fmtN(j.found)});
    total+=j.found;
    reportFolder();
  }));
  if(run!==validationRun)return;
  // folders may overlap, so the honest total is the server's deduplicated
  // count, not the sum of the rows
  const list=paths();
  if(list.length>1&&total>0){
    try{
      const q=list.map(p=>"path="+encodeURIComponent(p)).join("&");
      const agg=window.wudictAndroidGo
        ? androidSetupCall("folderSetup",q)
        : typeof nativeSetupCall==="function"
        ? await nativeSetupCall("folderSetup",q,undefined,controller.signal)
        : await (await browserHTTP("/api/setup?"+q,{signal:controller.signal})).json();
      if(typeof agg.found==="number")total=agg.found;
    }catch(e){if(controller.signal.aborted||run!==validationRun)return}
  }
  if(run!==validationRun)return;
  syncAct(total,checked,bad);
  if(validationController===controller)validationController=null;
  setSetupNotice("");
}

/* The footer reports dictionaries found in the selected folders. */
function syncAct(total,checked,bad){
  lastOK=total>0;
  const msg=$("msg");
  if(!checked){msg.textContent="";msg.className=""}
  else if(total>0){
    msg.className="ok";
    const parts=[];
    if(total>0)parts.push(tx("panel.dictionaryCount",{count:total,number:fmtN(total)})+
      (checked>1?tx("pages.inFolders",{number:checked}):""));
    msg.textContent="✓ "+parts.join(" + ");
  }else{msg.className="err";msg.textContent=tx("pages.noDictionaries")}
  $("save").disabled=total<=0;
  $("save").textContent=checked>1?tx("pages.useFolders"):tx("pages.useFolder");
}

/* One act for both sources. Folders and the library are not alternatives —
   they are two dimensions of one library — so the page commits once: a second
   button would post without the paths, silently discarding whatever folders
   had been typed above it. */
async function save(){
  if(!await saveIndexDefaults())return;
  const q=paths().map(p=>"path="+encodeURIComponent(p)).join("&");
  if(!q)return;
  if(validationController){validationRun++;validationController.abort();validationController=null}
  let diagnostics="";
  try{if(localStorage.getItem("wudict_folder_diagnostics")==="1")diagnostics="&diagnostics=1"}catch(_){}
  const url="/api/setup?"+q+"&save=1&async=1"+diagnostics;
  $("save").disabled=true;
  let j;
  try{
    if(window.wudictAndroidGo){
      androidSetupCall("folderSetup",q+"&save=1"+diagnostics);
      const finished=await window.wudictWork.wait('folder-setup');
      j=finished.result||{error:finished.error};
    }else if(typeof nativeSetupCall==="function"){
      await nativeSetupCall("folderSetup",q+"&save=1"+diagnostics);
      const finished=await window.wudictWork.wait('folder-setup');
      j=finished.result||{error:finished.error};
    }else{
      const response=await browserHTTP(url);
      j=await response.json();
      if(response.status===202){const finished=await window.wudictWork.wait('folder-setup');j=finished.result||{error:finished.error};}
    }
  }
  catch(e){$("msg").className="err";$("msg").textContent="✕ "+window.wudictI18n.errorText(e.message);validateAll();return}
  if(j.error){$("msg").className="err";$("msg").textContent="✕ "+window.wudictI18n.errorText(j.error);$("save").disabled=false;return}
  if(j.saved){
    if(foldersDialogEmbedded){foldersDialogRoot.close();window.dispatchEvent(new CustomEvent("wudict-folders-saved"))}
    else if(wudictDialogMode)parent.postMessage({type:"wudict-workspace-close",changed:"setup"},location.origin);
    else location.href=setupNavigation("/");
  }
}

/* Importing an archive (/api/intake). Two acts: the file is read and reports
   what it holds, then the user says which of those to install. Nothing is
   written between the two — the archive is only listed, never unpacked — so
   changing your mind costs nothing and the Cancel below is a real cancel.

   Everything the engine needs to do this (staging folders, extraction order,
   what is copied where) stays here (D102): the page asks for a file and shows
   dictionaries. */
let impTimer=null,impBusy=false;
// decimal, the rule of index.html's mb and the server's logx.Size
const sizeMB=n=>n<1e3?n+" B":n<1e6?Math.round(n/1e3)+" kB":n<1e7?(n/1e6).toFixed(1)+" MB":n<1e9?Math.round(n/1e6)+" MB":(n/1e9).toFixed(1)+" GB";

function impSay(text,cls){const m=$("impMsg");m.textContent=text;m.className=cls||""}

async function impGET(){
  try{ return window.wudictAndroidGo?androidIntakeCall("status") : typeof nativeSetupCall==="function"
    ? await nativeSetupCall("intakeStatus") : await (await browserHTTP("/api/intake")).json() }
  catch(e){ return null }
}

/* A browser-selected or dropped archive. Windows sends bounded chunks through
   the native bridge; browser and Android keep the streaming HTTP upload. */
async function impSend(file){
  if(!file||impBusy)return;
  impBusy=true;
  impSay(tx("pages.reading",{name:file.name}));
  $("found").classList.add("hidden");
  let r,j;
  try{
    if(window.wudictAndroidGo)j=await impSendAndroidFile(file);
    else if(typeof nativeSetupCall==="function")j=await impSendNativeFile(file);
    else{
      r=await browserHTTP("/api/intake?name="+encodeURIComponent(file.name),{method:"POST",body:file});
      j=await r.json();
    }
  }catch(e){ impBusy=false; impSay(tx("pages.readFailed",{name:file.name}),"err"); return }
  impBusy=false;
  if(r&&!r.ok||j.error){impSay("✕ "+(window.wudictI18n.errorText(j&&j.error)||tx("pages.cannotImport")),"err");return}
  showCandidates(j);
}

function intakeChunkBase64(buffer){
  const bytes=new Uint8Array(buffer);let binary="";
  for(let offset=0;offset<bytes.length;offset+=32768)
    binary+=String.fromCharCode(...bytes.subarray(offset,Math.min(offset+32768,bytes.length)));
  return btoa(binary);
}
async function impSendNativeFile(file){
  const started=await nativeSetupCall("intakeFileStart",JSON.stringify({name:file.name,size:file.size}));
  const id=started.id,chunkSize=4*1024*1024;
  try{
    for(let offset=0;offset<file.size;offset+=chunkSize){
      const end=Math.min(offset+chunkSize,file.size),data=intakeChunkBase64(await file.slice(offset,end).arrayBuffer());
      await nativeSetupCall("intakeFileChunk",JSON.stringify({id,data}));
      impSay(tx("pages.reading",{name:file.name})+" · "+Math.round(end/file.size*100)+"%");
    }
    return await nativeSetupCall("intakeFileFinish",id);
  }catch(error){
    await nativeSetupCall("intakeFileCancel",id).catch(()=>{});
    throw error;
  }
}

async function impSendAndroidFile(file){
  const started=androidIntakeCall("fileStart",JSON.stringify({name:file.name,size:file.size}));
  const id=started.id,chunkSize=4*1024*1024;
  try{
    for(let offset=0;offset<file.size;offset+=chunkSize){
      const end=Math.min(offset+chunkSize,file.size),data=intakeChunkBase64(await file.slice(offset,end).arrayBuffer());
      androidIntakeCall("fileChunk",JSON.stringify({id,data}));
      impSay(tx("pages.reading",{name:file.name})+" · "+Math.round(end/file.size*100)+"%");
    }
    return androidIntakeCall("fileFinish",id);
  }catch(error){
    try{androidIntakeCall("fileCancel",id)}catch(_){}
    throw error;
  }
}

async function impSendPath(path){
  if(!path||impBusy)return;
  impBusy=true;
  impSay(tx("pages.reading",{name:path.split(/[\\/]/).pop()}));
  $("found").classList.add("hidden");
  try{
    const j=await nativeSetupCall("intakePath",path);
    impBusy=false;
    if(j.error){impSay("✕ "+window.wudictI18n.errorText(j.error),"err");return}
    showCandidates(j);
  }catch(e){impBusy=false;impSay("✕ "+(window.wudictI18n.errorText(e.message)||tx("pages.cannotImport")),"err")}
}

/* A link, fetched by the server rather than by this page. Not a download
   started here and uploaded back: the server is the one that can resume a
   broken transfer, and on Android this page dies whenever the device rotates
   while the download does not. Which sites are offered is the server's
   setting; a refusal arrives as its own sentence. */
async function impFetch(){
  const u=$("url").value.trim();
  if(!u||impBusy)return;
  impBusy=true;
  impSay(tx("pages.connecting"));
  $("found").classList.add("hidden");
  let r,j;
  try{
    if(window.wudictAndroidGo)j=androidIntakeCall("url",u);
    else if(typeof nativeSetupCall==="function")j=await nativeSetupCall("intakeURL",u);
    else {r=await browserHTTP("/api/intake?url="+encodeURIComponent(u),{method:"POST"});j=await r.json()}
  }catch(e){ impBusy=false; impSay("✕ "+(window.wudictI18n.errorText(e.message)||tx("pages.cannotOpenMark")),"err"); return }
  impBusy=false;
  if(r&&!r.ok||j.error){impSay("✕ "+(window.wudictI18n.errorText(j&&j.error)||tx("pages.cannotOpen")),"err");return}
  $("url").value="";
  impRender(j);
  impPoll();
}

/* How many file names a row shows before it counts instead: a StarDict set is
   four files and a resource folder, and the list must not become the row. The
   same cut as the phone's, so the two surfaces say the same sentence. */
const FILES_SHOWN=3;
function fileList(files){
  if(!files||!files.length)return "";
  const head=files.slice(0,FILES_SHOWN);
  if(files.length>head.length)head.push(tx("pages.more",{number:files.length-head.length}));
  return head.join(" + ");
}

function showCandidates(j){
  const box=$("cands");box.textContent="";
  let usable=0;
  // A missing companion stops being a refusal when the same companion was
  // found beside the file and is about to be downloaded: what the user is
  // choosing is the state AFTER the extras, not the state on disk now. The
  // server re-checks once they have landed, so a download that fails still
  // refuses honestly rather than installing half a dictionary.
  const extras=j.extras||[];
  const coming=m=>extras.some(x=>x.need==="needed"&&x.name.toLowerCase().endsWith(m.toLowerCase()));
  (j.candidates||[]).forEach((c,i)=>{
    const miss=c.missing||[];
    const ok=!miss.length;
    const soon=!ok&&miss.every(coming);
    const row=el("label","row"+(ok||soon?"":" no"));
    const cb=el("input");cb.type="checkbox";cb.value=i;
    // Already installed and identical: left unticked, because the likeliest
    // reason to be here is the OTHER dictionary in the same archive. Ticking
    // it is still allowed — repairing a folder is a real reason — and then it
    // is replaced like any other.
    // Ticked is "overwrite", unticked is "skip": the same file starts skipped,
    // a different one - almost always a newer edition - starts overwriting,
    // and the row says what it replaces and where.
    cb.checked=(ok||soon)&&!c.unchanged;cb.disabled=!ok&&!soon;
    row.appendChild(cb);
    row.appendChild(el("span","nm",c.name));
    // The files, not the format: "OED.mdx + OED.mdd" already says the format
    // and says the thing a bare title cannot — WHICH files this is, and that
    // there are two of them (D137).
    row.appendChild(el("span","fl",fileList(c.files)));
    // The size only when it is a cost about to be paid. A dictionary that is
    // already installed, or that cannot be installed at all, is carrying a
    // state instead, and that is what the rest of the row is for.
    if(ok&&(!c.existing||c.stale)&&!(c.elsewhere&&c.unchanged))row.appendChild(el("span","n",sizeMB(c.size||0)));
    // When the site says the file last changed: beside "replaces your copy",
    // it is how a newer edition tells itself apart from the one installed.
    if(c.date)row.appendChild(el("span","n",c.date));
    // Named, not merely refused: the user can go and fetch the missing file,
    // and cannot if they are not told which one it is.
    if(soon)row.appendChild(el("span","why",tx("pages.needsFound",{files:miss.join(", ")})));
    else if(!ok)row.appendChild(el("span","why",tx("pages.needs",{files:miss.join(", ")})));
    // A folder of that name holding no dictionary — one a removal left behind,
    // or one the user made — is taken over like any other, and said so. What it
    // is not is an installation: calling it one told the user a dictionary they
    // had just removed was still there (D137).
    else if(c.stale)row.appendChild(el("span","have",tx("pages.takesFolder")));
    else if(c.existing)row.appendChild(el("span","have",c.unchanged?tx("pages.alreadyInstalled"):tx("pages.replaces")));
    // In another folder of the library, from wherever it came: named, so the
    // user can tell "the same file" from "another edition or format".
    else if(c.elsewhere)row.appendChild(el("span","have",
      c.unchanged?tx("pages.alreadyIn",{where:c.elsewhere}):tx("pages.replacesIn",{where:c.elsewhere})));
    if(ok||soon)usable++;
    box.appendChild(row);
  });
  showExtras(j);
  // A collection's files are downloaded one by one; there is no one file to name.
  $("srcName").textContent=j.collection?tx("pages.downloadedFiles"):(j.source||tx("pages.file"));
  // The question is only asked when it is one: with IMPORT_KEEP set to keep or
  // delete it has already been answered, and asking again would make the
  // setting a suggestion.
  $("srcKeepWrap").style.display=j.keep==="ask"?"":"none";
  // Offered only when there is something to be asked about. Replacing is the
  // default because two of the same dictionary means two of every result.
  // Overwrite all / skip all, offered only where there is a choice to make
  // in bulk: two or more rows that can be ticked.
  $("candAll").classList.toggle("hidden",usable<2);
  $("found").classList.remove("hidden");
  impSay(usable?"":tx("pages.nothingInstallable"),usable?"":"err");
  $("install").disabled=!usable;
}

/* Companion files found beside a downloaded one, offered rather than taken.
   The site publishes a dictionary as several links; the server looked for the
   others by name and found these. Ticked by default because a dictionary
   without its media is a dictionary with no images or sound — but ticked, not
   downloaded, because on a phone these are hundreds of megabytes of somebody
   else's data allowance. */
function showExtras(j){
  const box=$("extras");box.textContent="";
  const list=j.extras||[];
  box.classList.toggle("hidden",!list.length);
  if(!list.length)return;
  box.appendChild(el("p","head",
    j.collection?tx("pages.collectionExtras"):tx("pages.extraFiles",{number:list.length})));
  list.forEach((x,i)=>{
    const row=el("label","row");
    const cb=el("input");cb.type="checkbox";cb.value=i;cb.checked=true;
    row.appendChild(cb);
    row.appendChild(el("span","nm",x.name));
    if(x.size)row.appendChild(el("span","n",sizeMB(x.size)));
    const why=x.need==="needed"?tx("pages.needed")
             :x.need==="media"?tx("pages.media")
                              :tx("pages.optional");
    row.appendChild(el("span","need"+(x.need==="needed"?" req":""),why));
    box.appendChild(row);
  });
}

async function impInstall(){
  if(!await saveIndexDefaults())return;
  const pick=[...foldersDialogRoot.querySelectorAll("#cands input:checked")].map(i=>+i.value);
  const extras=[...foldersDialogRoot.querySelectorAll("#extras input:checked")].map(i=>+i.value);
  if(!pick.length)return;
  $("install").disabled=true;
  let r,j;
  try{
    const body=JSON.stringify({pick:pick,keep:$("srcKeep").checked,extras:extras});
    if(window.wudictAndroidGo)j=androidIntakeCall("confirm",body);
    else if(typeof nativeSetupCall==="function")j=await nativeSetupCall("intakeConfirm",body);
    else {r=await browserHTTP("/api/intake?confirm=1",{method:"POST",
      headers:{"Content-Type":"application/json"},body});j=await r.json()}
  }catch(e){ impSay("✕ "+(window.wudictI18n.errorText(e.message)||tx("pages.cannotStartMark")),"err"); return }
  if(r&&!r.ok||j.error){impSay("✕ "+(window.wudictI18n.errorText(j&&j.error)||tx("pages.cannotStart")),"err");
    $("install").disabled=false;return}
  $("found").classList.add("hidden");
  impPoll();
}

/* Polled, not streamed: the extraction runs in the server and outlives this
   page, so the page's job is to ask what happened — including after a reload,
   which is why start() asks once with no job of its own. */
function impPoll(){
  clearTimeout(impTimer);
  impTimer=setTimeout(async()=>{
    const j=await impGET();
    if(!j)return impPoll();
    impRender(j);
    if(j.state==="installing"||j.state==="downloading")impPoll();
  },700);
}

function impRender(j){
  if(j.state==="downloading"){
    // No percentage when the server declared no length — a made-up one that
    // sticks is worse than a line that only says work is happening.
    const pct=j.total>0?Math.min(99,Math.floor(j.done*100/j.total))+"%":sizeMB(j.done||0);
    // The file first, the host after it: "which of several files is arriving"
    // is what a person watching a multi-gigabyte companion download wants to
    // read, and the name is only known once the server has answered — until
    // then the host alone is the honest line.
    const what=j.source?(j.host?tx("pages.downloadFileHost",{name:j.source,host:j.host}):tx("pages.downloadFile",{name:j.source})):tx("pages.downloadHost",{host:j.host||tx("pages.site")});
    impSay(what+"… "+pct);
    $("found").classList.add("hidden");
    return;
  }
  if(j.state==="installing"){
    const pct=j.total>0?Math.min(99,Math.floor(j.done*100/j.total)):0;
    impSay(tx("pages.installing",{percent:pct}));
    $("found").classList.add("hidden");
    return;
  }
  if(j.state==="done"){
    const n=(j.installed||[]).length;
    // A collection is many answers: what did not arrive is said beside what
    // did, rather than lost behind a tick.
    const also=j.error?tx("pages.alsoFailed",{detail:window.wudictI18n.errorText(j.error)}):"";
    impSay((n?tx("pages.installedNames",{names:j.installed.join(", ")}):tx("pages.finished"))+also,n||!j.error?"ok":"err");
    // The folder now holds more than it did, and the count under it is what
    // the Save button is enabled from.
    validateAll();
    return;
  }
  if(j.state==="error"){impSay("✕ "+(window.wudictI18n.errorText(j.error)||tx("pages.importFailed")),"err");return}
  if(j.state==="ready"){showCandidates(j);return}
}

async function impCancel(){
  clearTimeout(impTimer);
  try{if(window.wudictAndroidGo)androidIntakeCall("cancel");
    else if(typeof nativeSetupCall==="function")await nativeSetupCall("intakeCancel");
    else await browserHTTP("/api/intake",{method:"DELETE"})}catch(e){}
  $("found").classList.add("hidden");
  impSay("");
}

/* Copies the guide into the folder above (POST /api/howto); the copy then
   stands in for the built-in guide, and the server has already rescanned. */
async function howtoBack(e){
  e.preventDefault();
  const msg=$("howtoBackRow");
  try{
    if(window.wudictAndroidGo)androidSetupCall("howto","restore");
    else if(typeof nativeSetupCall==="function")await nativeSetupCall("howto","restore");
    else {const r=await browserHTTP("/api/howto?restore=1",{method:"POST"});
      const j=await r.json().catch(()=>({}));if(!r.ok)throw new Error(j.error||"HTTP "+r.status)}
    msg.textContent=tx("pages.howtoRestored");
  }catch(e){msg.textContent="✕ "+window.wudictI18n.errorText(e.message||tx("pages.notAnswering"))}
}

async function howtoCopy(e){
  e.preventDefault();
  try{
    let j;
    if(window.wudictAndroidGo)j=androidSetupCall("howto","copy");
    else if(typeof nativeSetupCall==="function")j=await nativeSetupCall("howto","copy");
    else {const r=await browserHTTP("/api/howto",{method:"POST"});j=await r.json().catch(()=>({}));
      if(!r.ok)throw new Error(j.error||"HTTP "+r.status)}
    impSay(tx("pages.howtoAdded",{path:j.path}),"ok");
  }catch(e){impSay("✕ "+window.wudictI18n.errorText(e.message||tx("pages.notAnswering")),"err")}
}

let intakeWired=false;
function wireIntake(dir){
  if(!dir)return; // nowhere to install into yet: the folder above comes first
  $("addDir").textContent=dir;
  $("addSection").classList.remove("hidden");
  if(intakeWired)return;
  intakeWired=true;
  $("choose").addEventListener("click",()=>{
    if(typeof nativeSetupCall==="function"&&typeof window.wudictPickArchive==="function"){
      window.wudictPickArchive(impSendPath);
      return;
    }
    $("file").click();
  });
  $("file").addEventListener("change",()=>{const f=$("file").files[0];$("file").value="";impSend(f)});
  $("fetch").addEventListener("click",impFetch);
  $("url").addEventListener("keydown",e=>{if(e.key==="Enter"){e.preventDefault();impFetch()}});
  // A one-line field deletes the line breaks of a pasted list, which fuses
  // its links into one that is none of them. They become spaces instead, and
  // the server reads every link out of what arrives.
  $("url").addEventListener("paste",e=>{
    const t=e.clipboardData&&e.clipboardData.getData("text");
    if(!t||!/[\r\n]/.test(t))return;
    e.preventDefault();
    const f=$("url"),a=f.selectionStart??f.value.length,b=f.selectionEnd??a;
    // "#" lines (a list's title, its comments) are dropped first: joined onto
    // one line, the first of them would make the whole line a comment.
    const v=t.split(/\r?\n|\r/).filter(l=>!/^\s*#/.test(l)).join(" ").replace(/\s+/g," ").trim();
    f.value=f.value.slice(0,a)+v+f.value.slice(b);
    f.setSelectionRange(a+v.length,a+v.length);
  });
  $("install").addEventListener("click",impInstall);
  for(const [id,on] of [["candAllOn",true],["candAllOff",false]])$(id).addEventListener("click",e=>{
    e.preventDefault();
    for(const cb of foldersDialogRoot.querySelectorAll("#cands input:not(:disabled)"))cb.checked=on;
  });
  $("dropJob").addEventListener("click",impCancel);
  $("howtoCopy").addEventListener("click",howtoCopy);
  const d=$("drop");
  // dragover must be cancelled or the browser navigates to the file instead,
  // which on a drop of a 2 GB zip is a download of it.
  for(const ev of ["dragenter","dragover"])d.addEventListener(ev,e=>{
    e.preventDefault();d.classList.add("over")});
  for(const ev of ["dragleave","drop"])d.addEventListener(ev,()=>d.classList.remove("over"));
  d.addEventListener("drop",e=>{
    e.preventDefault();
    const f=e.dataTransfer&&e.dataTransfer.files&&e.dataTransfer.files[0];
    if(f)impSend(f);
  });
}

$("addRow").addEventListener("click",()=>{addRow("").focus()});
$("save").addEventListener("click",save);

/* Pre-fill with the folders in use: this page is also the folder EDITOR, not
   only the first-run chooser, so it must show what is configured now. When a
   flag or an environment variable set DICT_DIR, saving to wudict.toml cannot
   take effect — say so instead of silently doing nothing next launch. */
async function start(){
  $("rows").replaceChildren();
  $("origin").classList.add("hidden");
  $("cfgProblems").classList.add("hidden");
  $("howtoBackRow").classList.add("hidden");
  let cfg=null,migration=null;
  try{
    if(window.wudictAndroidGo){
      [cfg,migration]=await Promise.all([
        Promise.resolve().then(()=>JSON.parse(window.wudictAndroidGo.config())),
        Promise.resolve().then(()=>androidSetupCall("setup","migration=status"))
      ]);
    }else if(typeof nativeSetupCall==="function"){
      [cfg,migration]=await Promise.all([
        nativeSetupCall("config"),nativeSetupCall("setup","migration=status")
      ]);
    }else{
      const [configResponse,migrationResponse]=await Promise.all([
        browserHTTP("/api/config"),browserHTTP("/api/setup?migration=status")
      ]);
      cfg=await configResponse.json();migration=await migrationResponse.json();
    }
  }catch(e){try{cfg=window.wudictAndroidGo?JSON.parse(window.wudictAndroidGo.config()):typeof nativeSetupCall==="function"
    ? await nativeSetupCall("config") : await (await browserHTTP("/api/config")).json()}catch(_){}}
  if(migration&&migration.available&&!foldersDialogEmbedded)await showMigrationDialog(migration.forked);
  for(const [feature,suffix] of [["contains","Contains"],["fullText","FullText"]]){
    $("newIndex"+suffix).checked=!!cfg?.indexDefaults?.[feature];
  }
  syncIndexDefaults();
  await saveIndexDefaults();
  const dirs=(cfg&&cfg.roots||[]).map(r=>r.path);
  if(dirs.length)dirs.forEach(d=>addRow(d)); else addRow("");
  /* howtoRemoved is a fact about the host: it stays false while the builtin
     guide is not wired, so this row never appears - kept whole, like the
     markup and howtoBack, for a host that offers the guide. */
  if(cfg&&cfg.howtoRemoved){
    $("howtoBackRow").classList.remove("hidden");
    $("howtoBack").onclick=howtoBack;
  }
  if(cfg&&cfg.dictDirEditable===false){
    const how=cfg.dictDirOrigin==="flag"?"--dict-dir":"DICT_DIR";
    $("origin").textContent=tx("pages.origin",{source:how});
    $("origin").classList.remove("hidden");
  }
  // Configuration lines that set nothing: a misspelt key is otherwise a
  // setting that silently does not exist.
  if(cfg&&cfg.configProblems&&cfg.configProblems.length){
    const p=$("cfgProblems");
    const configName=cfg.configPath?cfg.configPath.replace(/^.*[\\/]/,""):"dictshelf.toml";
    p.textContent="⚠ "+configName+(cfg.configPath?" ("+cfg.configPath+")":"")+": "+cfg.configProblems.join("; ");
    p.classList.remove("hidden");
  }
  if(dirs.length)validateAll(); else rowInputs()[0].focus();
  // The importer needs somewhere to install into, and a job may already be
  // running from before this page was loaded.
  const st=await impGET();
  wireIntake(st&&st.dir||dirs[0]||"");
  if(st&&st.state){impRender(st);
    if(st.state==="installing"||st.state==="downloading")impPoll()}
}

function showMigrationDialog(forked){
  const intro=$("migrationIntro"),actions=$("migrationActions"),shade=$("migrationShade");
  intro.textContent=tx(forked?"pages.migrationForked":"pages.migrationUpstream");
  const choices=forked?
    [["copy","pages.migrationCopy"],["move","pages.migrationMove"],["new","pages.migrationNew"]]:
    [["import","pages.migrationImport"],["new","pages.migrationNew"]];
  return new Promise(resolve=>{
    for(const [choice,key] of choices){
      const button=document.createElement("button");
      button.type="button";button.className=choice==="new"?"btn ghost":"btn";
      button.textContent=tx(key);
      button.addEventListener("click",()=>{
        if(choice==="new"){shade.hidden=true;resolve();return}
        applyMigration(choice,button);
      });
      actions.appendChild(button);
    }
    shade.hidden=false;
    actions.querySelector("button")?.focus();
  });
}

async function applyMigration(choice,button){
  const buttons=[...$("migrationActions").querySelectorAll("button")],error=$("migrationError");
  buttons.forEach(b=>b.disabled=true);error.hidden=true;
  $("migrationIntro").textContent=tx("pages.migrationProgress");
  setSetupNotice(tx("pages.migrationProgress"));
  try{
    if(window.wudictAndroidGo)androidSetupCall("setup","migration="+encodeURIComponent(choice));
    else if(typeof nativeSetupCall==="function")await nativeSetupCall("setup","migration="+encodeURIComponent(choice));
    else {
      const response=await browserHTTP("/api/setup?migration="+encodeURIComponent(choice),{method:"POST"});
      const result=await response.json();
      if(!response.ok||result.error)throw new Error(result.error||response.statusText);
    }
    location.assign(setupNavigation("/setup"));
  }catch(e){
    error.textContent=window.wudictI18n.errorText(e.message);error.hidden=false;
    buttons.forEach(b=>b.disabled=false);button.focus();setSetupNotice("");
  }
}
// Prepared data follows the selected source folders; it is no longer offered
// as a second, independently retained collection.
if(!foldersDialogEmbedded)start();

/* The ✕ closes the page the way the back gesture does: no navigation to "/"
   (which would load the app from scratch and stack an entry the back gesture
   would then walk back into this page), just history. A first run — where the
   server serves THIS page for "/" — has nothing behind it, and offers no ✕. */
if(foldersDialogEmbedded){
  $("closePage").onclick=()=>foldersDialogRoot.close();
  foldersDialogRoot.addEventListener("close",()=>{if(validationController){validationController.abort();validationController=null;validationRun++}foldersDialogReturnFocus?.focus()});
  foldersDialogRoot.addEventListener("cancel",()=>{if(validationController){validationController.abort();validationController=null;validationRun++}});
  window.openFoldersDialog=link=>{foldersDialogReturnFocus=link;foldersDialogRoot.showModal();start()};
}
else if(wudictDialogMode){
  $("closePage").onclick=()=>parent.postMessage({type:"wudict-workspace-close"},location.origin);
  addEventListener("message",event=>{
    if(event.origin===location.origin&&event.source===parent&&event.data?.type==="wudict-workspace-close-request")
      parent.postMessage({type:"wudict-workspace-close"},location.origin);
  });
}
else if(history.length>1)$("closePage").onclick=()=>history.back();
else $("closePage").hidden=true;
})();
