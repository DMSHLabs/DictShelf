/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */
(function(){
"use strict";
const root=document.getElementById("filtersDialog");
if(!root)return;
const tx=window.wudictI18n.t;
const $=id=>root.querySelector("#filtersDialog"+id);
const esc=s=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
const nativeCall=typeof window.wudictNativeCall==="function"?window.wudictNativeCall.bind(window):null;
let saved="",facets=[],writable=false,unusable=false,busy=false,loaded=false,returnFocus=null;
const dirty=()=>$("Text").value!==saved;
function say(text,cls){$("Message").className=cls||"";$("Message").textContent=text}
function edited(before,after){
  const was=new Map();
  for(const f of before)for(const g of f.groups)was.set(f.id+"\n"+g.id,g.items.join("\n"));
  const out=[];
  for(const f of after)for(const g of f.groups)if(was.get(f.id+"\n"+g.id)!==g.items.join("\n"))out.push({f:f.id,g});
  return out;
}
async function unmatched(changed){
  if(!changed.length)return [];
  const have=new Set();
  try{
    if(window.wudictAndroidGo){
      const id=window.wudictAndroidGo.startDictStream();
      if(id<=0)throw new Error("Dictionary list unavailable");
      try{
        for(;;){
          const packet=JSON.parse(window.wudictAndroidGo.pollDictStream(id));
          for(const o of packet.messages||[])if(o.t==="dict"&&o.dict)
            for(const g of o.dict.filters||o.dict.groups||[])have.add(g.f+"\n"+g.v);
          if(packet.done){if(packet.error)throw new Error(packet.error);break}
          await new Promise(resolve=>setTimeout(resolve,16));
        }
      }finally{window.wudictAndroidGo.cancelDictStream(id)}
    }else if(nativeCall)await nativeCall("dicts","",o=>{if(o.t==="dict")for(const g of o.dict.filters||o.dict.groups||[])have.add(g.f+"\n"+g.v)});
    else{
      const t=await(await fetch("/api/dicts")).text();
      for(const line of t.split("\n")){if(!line.trim())continue;const o=JSON.parse(line);if(o.t==="dict")for(const g of o.dict.filters||o.dict.groups||[])have.add(g.f+"\n"+g.v)}
    }
  }catch(_){return []}
  return changed.filter(c=>!have.has(c.f+"\n"+c.g.id)).map(c=>c.g.label);
}
function problems(j){
  $("Problems").innerHTML=j.problems.map(p=>"<li>"+(p.line?tx("filters.line",{number:p.line})+": ":"")+esc(p.msg)+(p.text?" — <code>"+esc(p.text)+"</code>":"")+"</li>").join("");
  if(j.unusable)return tx("filters.fallback");
  const n=j.problems.reduce((sum,p)=>sum+(p.more||1),0);
  return n?tx("filters.problems",{number:n}):"";
}
function show(j,keepText){
  writable=j.writable;unusable=j.unusable;facets=j.facets;
  if(!keepText){$("Text").value=j.text;saved=$("Text").value}
  $("Save").disabled=!writable;
  $("Reset").classList.toggle("hidden",!j.custom||!writable);
  $("File").textContent=writable?tx(j.custom?"filters.savedIn":"filters.createFile",{file:j.file}):tx("filters.noFolder");
  return problems(j);
}
async function call(method,body,query){
  if(window.wudictAndroidGo){
    const action=method==="GET"?"get":method==="PUT"?"save":"reset";
    const argument=method==="PUT"?JSON.stringify({text:body||"",replace:new URLSearchParams(query||"").get("replace")==="1"}):"";
    const result=JSON.parse(window.wudictAndroidGo.groups(action,argument));
    if(!result.ok)throw new Error(result.error||"Could not edit filters");
    return result.value;
  }
  if(nativeCall){
    if(method==="GET")return nativeCall("groups");
    if(method==="PUT")return nativeCall("saveGroups",JSON.stringify({text:body||"",replace:new URLSearchParams(query||"").get("replace")==="1"}));
    if(method==="DELETE")return nativeCall("resetGroups");
  }
  const r=await fetch("/api/groups"+(query||""),{method,body,headers:body==null?{}:{"Content-Type":"text/plain; charset=utf-8"}});
  if(!r.ok){let why="HTTP "+r.status;try{const j=await r.json();if(j&&j.error)why=j.error}catch(_){}throw new Error(why)}
  return r.json();
}
async function load(){
  try{const p=show(await call("GET"));loaded=true;if(p)say(p,"err")}
  catch(e){say(tx("filters.unreachable",{error:e.message}),"err")}
}
async function save(){
  if(!writable||busy)return;
  if(unusable&&!confirm(tx("filters.replace")))return;
  busy=true;$("Save").disabled=true;say(tx("filters.saving"));
  const sent=$("Text").value,before=facets;
  try{
    const j=await call("PUT",sent,unusable?"?replace=1":"");
    const typed=$("Text").value!==sent,p=show(j,typed);saved=typed?sent:$("Text").value;
    const none=await unmatched(edited(before,j.facets)),parts=[tx("filters.saved")];
    if(p)parts.push(p);if(none.length)parts.push(tx("filters.unmatched",{names:none.join(", ")}));
    say(parts.join(" "),p||none.length?"err":"ok");
  }catch(e){say(tx("filters.saveFailed",{error:e.message}),"err")}
  finally{busy=false;$("Save").disabled=!writable}
}
function open(link){
  returnFocus=link;
  root.showModal();
  if(!loaded)load();else if(!dirty())load();
}
function closeEditor(){
  if(dirty()&&!confirm(tx("filters.leave")))return;
  saved=$("Text").value;root.close();
}
$("Save").addEventListener("click",save);
$("Reset").addEventListener("click",async()=>{
  if(busy||!confirm(tx("filters.resetConfirm")))return;
  try{show(await call("DELETE"));say(tx("filters.restored"),"ok")}
  catch(e){say(tx("filters.resetFailed",{error:e.message}),"err")}
});
$("Text").addEventListener("input",()=>{if($("Message").className!=="err")say("")});
$("Close").addEventListener("click",closeEditor);
root.addEventListener("cancel",event=>{event.preventDefault();closeEditor()});
root.addEventListener("close",()=>returnFocus?.focus());
document.addEventListener("keydown",e=>{
  if(!root.open)return;
  if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==="s"){e.preventDefault();save()}
});
window.openFiltersDialog=open;
})();
