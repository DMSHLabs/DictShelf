// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

(function(){
"use strict";
const lemmaDialog=document.getElementById("lemmaDialog");
const lemmaDialogEmbedded=!!lemmaDialog;
const wudictDialogMode=lemmaDialogEmbedded||new URLSearchParams(location.search).get("wudict_dialog")==="1";
const lemmaDialogReturnFocus={link:null};
const lemmaDialogId=id=>"lemma"+id[0].toUpperCase()+id.slice(1);
const $=id=>lemmaDialogEmbedded?lemmaDialog.querySelector("#"+lemmaDialogId(id)):document.getElementById(id);
const tx=window.wudictI18n.t;
const esc=s=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
const nativeLemmaCall=wudictDialogMode&&typeof parent.wudictNativeCall==="function"
  ? parent.wudictNativeCall.bind(parent) : window.wudictNativeCall;
// decimal, the rule of index.html's mb and the server's logx.Size
const mb=n=>n<1e3?n+" B":n<1e6?Math.round(n/1e3)+" kB":n<1e7?(n/1e6).toFixed(1)+" MB":n<1e9?Math.round(n/1e6)+" MB":(n/1e9).toFixed(1)+" GB";

/* State lives in the Go process, not here: a download is a job it runs, so
   this page can be reloaded, rotated or closed and reopened without losing it.
   Everything drawn below comes from one GET, which is also what is polled. */
let polling=false,hasSnapshot=false;
const languageNames=typeof Intl.DisplayNames==="function"?new Intl.DisplayNames([window.wudictI18n.language],{type:"language"}):null;
function languageName(l){
  if(!languageNames)return l.name;
  try{const name=languageNames.of(l.code);return name&&name!==l.code?name:l.name}catch(_){return l.name}
}

async function load(){
  let j;
  try{ j=window.wudictAndroidGo
    ? androidLemmaCall("get","")
    : typeof nativeLemmaCall==="function"
    ? await nativeLemmaCall("lemmas")
    : await (await fetch("/api/lemmas")).json() }
  catch(e){ $("banner").className=(lemmaDialogEmbedded?"lemma-banner":"banner")+" warn";$("banner").textContent=tx("pages.cannotReach");return null }
  render(j);
  hasSnapshot=true;
  return j;
}

function showLoading(){
  const banner=$("banner");
  banner.className=(lemmaDialogEmbedded?"lemma-banner":"banner")+" loading";
  banner.replaceChildren();
  const spinner=document.createElement("span");spinner.className="spin";spinner.setAttribute("aria-hidden","true");
  banner.append(spinner,document.createTextNode(" "+tx("pages.loadingLemmas")));
}
function loadAfterPaint(){
  if(!hasSnapshot)showLoading();
  // Android's Go bridge call is synchronous. Yield through a paint first so
  // the window and its loading state appear before a cold catalogue request.
  requestAnimationFrame(()=>setTimeout(()=>{
    load().then(j=>{if(j&&j.languages.some(l=>l.state==="downloading"))poll()});
  },32));
}

async function changeLemma(method,code){
  if(window.wudictAndroidGo)return androidLemmaCall(method==="POST"?"install":"remove",code);
  if(typeof nativeLemmaCall==="function"){
    return nativeLemmaCall(method==="POST"?"installLemma":"removeLemma",code);
  }
  const response=await fetch("/api/lemmas?code="+encodeURIComponent(code),{method});
  if(!response.ok){
    const body=await response.json().catch(()=>({}));
    throw new Error(body.error||"HTTP "+response.status);
  }
  return response.json();
}

function androidLemmaCall(action,code){
  const result=JSON.parse(window.wudictAndroidGo.lemmas(action,code));
  if(!result.ok)throw new Error(result.error||"Lemmatization is unavailable");
  return result.value;
}

function render(j){
  const list=$("list");
  list.textContent="";
  let installed=0;
  for(const l of j.languages){
    if(l.installed)installed++;
    list.appendChild(rowFor(l,j));
  }
  $("summary").textContent=j.languages.length?tx("pages.installedTotal",{number:installed,total:j.languages.length}):tx("pages.installedCount",{number:installed});
  $("dir").textContent=tx("pages.installedIn",{path:j.dir});

  const b=$("banner");
  if(!j.enabled){
    b.className="banner warn";
    b.textContent=tx("pages.lemmaOff");
  }else if(j.error){
    b.className="banner warn";
    b.textContent=tx("pages.catalogueFailed",{error:window.wudictI18n.errorText(j.error)});
  }else{
    b.className="banner hidden";
  }

  $("shadow").className=j.shadowed&&j.shadowed.length?"banner warn":"banner warn hidden";
  if(j.shadowed&&j.shadowed.length)$("shadow").textContent=j.shadowed.join("; ");

  /* ODbL: this data may be redistributed only with attribution, and the page
     that installs it is where a user will look for that. */
  const lic=j.languages.find(l=>l.license);
  $("credit").innerHTML=lic
    ? esc(tx("pages.wordLists",{source:lic.source||"",license:lic.license}))+
      (j.attribution?' · <a href="'+esc(j.attribution)+'" target="_blank" rel="noopener">'+esc(tx("pages.attribution"))+'</a>':"")
    : "";
}

function rowFor(l,j){
  const row=document.createElement("div");
  row.className="lrow"+(l.installed?"":" off");
  const cb=document.createElement("input");
  cb.type="checkbox";cb.checked=l.installed;cb.id="lemma-cb-"+l.code;
  cb.setAttribute("aria-label",languageName(l));
  const busy=l.state==="downloading";
  // Built-in English cannot be "not installed", so its box has nothing to say.
  cb.disabled=busy||(l.builtin&&!l.localSize)||(!l.installed&&!l.catalogued);
  cb.addEventListener("change",()=>toggle(l,cb));

  const nm=document.createElement("label");
  nm.className="nm";nm.htmlFor=cb.id;
  nm.textContent=languageName(l)+" ("+l.code+")";

  const cost=document.createElement("span");
  cost.className="cost";
  const bits=[];
  if(l.size)bits.push(mb(l.size));
  else if(l.localSize)bits.push(mb(l.localSize));
  if(l.heapMB)bits.push(tx("pages.memory",{number:l.heapMB}));
  cost.textContent=bits.join(" · ");

  const st=document.createElement("span");
  st.className="st";
  if(busy){
    st.className="st busy";
    // Guarded rather than trusting the numbers: a catalogue with no declared
    // size, or a row read from an older build, must read "Downloading…" and
    // never "Downloading… NaN%".
    var pct=l.total>0?Math.floor(100*(l.done||0)/l.total):-1;
    st.textContent=pct>=0?tx("pages.downloadPercent",{percent:Math.min(pct,100)}):tx("pages.downloading");
  }else if(l.state==="error"){
    st.className="st err";st.textContent="✕ "+window.wudictI18n.errorText(l.error);
  }else if(l.builtin&&!l.localSize){
    st.className="st ok";st.textContent=tx("pages.builtin");
  }else if(l.installed&&l.mismatch){
    st.className="st";st.textContent=tx("pages.installedDiffers");
  }else if(l.installed&&!l.catalogued){
    st.className="st ok";st.textContent=tx("pages.installedLocal");
  }else if(l.installed){
    st.className="st ok";st.textContent=tx("pages.installed");
  }
  row.append(cb,nm,cost,st);
  return row;
}

/* Checking installs, unchecking removes. The box is disabled the moment it is
   clicked and re-enabled by the next poll that reports a terminal state, so a
   second click cannot start a second download - Go refuses one anyway,
   but the UI should not invite it. */
async function toggle(l,cb){
  cb.disabled=true;
  if(cb.checked){
    setStatus(cb,"busy",tx("pages.downloading"));
    try{
      await changeLemma("POST",l.code);
    }catch(e){ fail(cb,window.wudictI18n.errorText(e.message)||tx("pages.cannotReachShort"));return }
    poll();
    return;
  }
  const size=l.localSize?" ("+mb(l.localSize)+")":"";
  if(!confirm(tx("pages.removeLemmas",{language:languageName(l),size}))){
    cb.checked=true;cb.disabled=false;return;
  }
  setStatus(cb,"busy",tx("pages.removing"));
  try{
    await changeLemma("DELETE",l.code);
  }catch(e){ fail(cb,window.wudictI18n.errorText(e.message)||tx("pages.cannotReachShort"));return }
  load();
}

function setStatus(cb,cls,text){
  const st=cb.parentElement.querySelector(".st");
  st.className="st "+cls;st.textContent=text;
}
function fail(cb,msg){
  cb.checked=!cb.checked;cb.disabled=false;setStatus(cb,"err","✕ "+msg);
}

/* Polling, not a stream. A browser allows about six connections per origin and
   a download here can be one of several; a stream each would leave the app
   unable to search while they ran. It also means progress is server state:
   nothing is lost when the page goes away, which on Android it does on every
   rotation. */
async function poll(){
  if(polling)return;
  polling=true;
  for(;;){
    const j=await load();
    if(!j||!j.languages.some(l=>l.state==="downloading"))break;
    await new Promise(r=>setTimeout(r,800));
  }
  polling=false;
}

function openLemmaDialog(link){
  lemmaDialogReturnFocus.link=link||document.activeElement;
  lemmaDialog.showModal();
  loadAfterPaint();
}

if(lemmaDialogEmbedded){
  $("closePage").onclick=()=>lemmaDialog.close();
  lemmaDialog.addEventListener("close",()=>lemmaDialogReturnFocus.link?.focus());
  window.openLemmaDialog=openLemmaDialog;
}else loadAfterPaint();

/* The ✕ closes the page the way the back gesture does — no navigation to "/",
   which would load the app from scratch and stack an entry the back gesture
   would then walk back into this page. A first run (where "/" is served as
   /setup) has nothing behind it, and shows no ✕. */
if(wudictDialogMode&&!lemmaDialogEmbedded){
  $("closePage").onclick=()=>parent.postMessage({type:"wudict-workspace-close"},location.origin);
  addEventListener("message",event=>{
    if(event.origin!==location.origin||event.source!==parent)return;
    if(event.data?.type==="wudict-workspace-close-request")parent.postMessage({type:"wudict-workspace-close"},location.origin);
    else if(event.data?.type==="wudict-workspace-refresh")load();
  });
}
else if(history.length>1)$("closePage").onclick=()=>history.back();
else $("closePage").hidden=true;

})();
