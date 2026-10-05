/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */


// Group membership never writes disabled, prefOrder or the search selector.
let userGroups=[], selectedGroup="all", groupSaving=false, groupShowAllPreferred=false;
let groupAvailableFilter="all";
function availableMatchesFilter(d){
  const filters=d.filters||[];
  return groupAvailableFilter==="all"||(groupAvailableFilter==="uncategorized"?filters.length===0:filters.some(g=>JSON.stringify([g.f,g.v])===groupAvailableFilter));
}
function appendAvailableFilter(rows,ordered){
  const bar=document.createElement('label');bar.className='group-available-filter';
  const title=document.createElement('span');title.textContent=tx('dictUI.availableFilter');
  const select=document.createElement('select');select.id='groupAvailableFilter';select.disabled=groupSaving;
  select.add(new Option(tx('panel.allDictionaries'),'all'));
  const categories=new Map();
  for(const d of ordered)for(const g of d.filters||[])categories.set(JSON.stringify([g.f,g.v]),g);
  let section=null,lastFacet=null;
  for(const [key,g] of [...categories].sort((a,b)=>a[1].fo-b[1].fo||a[1].f.localeCompare(b[1].f)||a[1].vl.localeCompare(b[1].vl))){
    const labels=window.wudictI18n.facetLabels(g);
    if(lastFacet!==g.f){section=document.createElement('optgroup');section.label=labels.fl;select.append(section);lastFacet=g.f}
    section.append(new Option(labels.vl,key));
  }
  if(ordered.some(d=>!(d.filters||[]).length))select.add(new Option(tx('dictUI.uncategorized'),'uncategorized'));
  if(![...select.options].some(option=>option.value===groupAvailableFilter))groupAvailableFilter='all';
  select.value=groupAvailableFilter;
  select.onchange=()=>{groupAvailableFilter=select.value;renderGroupRows();$('groupAvailableFilter').focus()};
  bar.append(title,select);rows.append(bar);
}
let pickerGroup=localStorage.getItem("wudict_picker_group")||"all";
async function loadPickerGroups(){
  try{userGroups=await groupRequest("/api/user-groups","GET");
    if(!userGroups.some(g=>g.id===pickerGroup))pickerGroup="all";
  }catch(e){console.warn("could not load dictionary groups:",e);pickerGroup="all"}
  // The panel's drop-down and the status bar's chip both name the group in
  // force, and this is the moment the names exist. The chip is painted from
  // what this call caches, because syncChips runs BEFORE this script has
  // declared userGroups at all (see scopeLabel in index.html).
  if(typeof paintPickerGroup==="function")paintPickerGroup();
}
// Translate known API validation messages only; unknown diagnostic details stay intact.
function groupErrorText(message){
  const keys=new Map([["Invalid group name","dictUI.invalidName"],["Enter a group name (1–100 characters, without control characters).","dictUI.nameLength"],["All Dictionaries is a reserved group name.","dictUI.reservedName"],["A group with this name already exists.","dictUI.duplicateName"],["Could not create group","dictUI.createFailed"],["Invalid membership","dictUI.invalidMember"],["All Dictionaries always contains every dictionary.","dictUI.allMembers"],["Dictionary no longer available","dictUI.dictMissing"],["Group not found","dictUI.groupMissing"],["Invalid group order","dictUI.invalidOrder"],["Group membership changed; reload the list","dictUI.reloadGroup"]]);
  return keys.has(message)?tx(keys.get(message)):message;
}
async function groupRequest(path,method,body){
  const response=await fetch(path,{method,headers:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body)});
  if(!response.ok)throw new Error(groupErrorText((await response.text()).trim()));
  return response.json();
}
function renderGroupOptions(){
  const select=$("groupSelect");select.replaceChildren();
  for(const group of userGroups)select.add(new Option(pickerGroupName(group.id),group.id));
  select.add(new Option(tx("dictUI.newGroup"),"new"));select.value=selectedGroup;
}
function groupHint(group,showAll,ordered){
  const members=new Set(group.members);
  const inside=ordered.filter(d=>members.has(d.id)).length;
  const outside=ordered.length-inside;
  return group.readonly?tx("dictUI.dragAll")
    :!showAll?(inside?tx("dictUI.dragGroup"):tx("dictUI.emptyGroup"))
    :inside?"":outside?tx("dictUI.addMembers"):tx("dictUI.noMembers");
}
function renderGroupRows(){
  const group=userGroups.find(g=>g.id===selectedGroup), rows=$("groupRows");rows.replaceChildren();
  if(!group)return;
  const showAll=$("groupShowAll");
  showAll.disabled=group.readonly;
  showAll.checked=group.readonly||groupShowAllPreferred;
  showAll.parentElement.classList.toggle("group-control-disabled",group.readonly);
  const ordered=orderedDicts(), members=new Set(group.readonly?ordered.map(d=>d.id):group.members);
  const inside=group.readonly?ordered:orderedGroupDicts(group,ordered);
  const outside=group.readonly?[]:ordered.filter(d=>!members.has(d.id));
  const filtering=showAll.checked&&!group.readonly;
  // Offer only categories containing dictionaries still available to add.
  const filterHost=document.createElement('div');
  if(filtering)appendAvailableFilter(filterHost,outside);
  const filteredOutside=filtering?outside.filter(availableMatchesFilter):outside;
  const visible=showAll.checked?inside.concat(filteredOutside):inside;
  $("groupHint").textContent=groupHint(group,showAll.checked,ordered);
  let filterInserted=false;
  for(const d of visible){
    if(filtering&&!members.has(d.id)&&!filterInserted){rows.append(...filterHost.childNodes);filterInserted=true}
    const label=document.createElement(!group.readonly&&showAll.checked?"label":"div");label.className="group-row";
    label.dataset.dict=d.id;
    const name=document.createElement("span");name.textContent=dictLabel(d);
    if(group.readonly||!showAll.checked){
      const grip=document.createElement("button");grip.type="button";grip.className="group-grip";
      grip.textContent="≡";grip.setAttribute("aria-label",tx("dictUI.dragNamed",{name:dictLabel(d)}));
      label.append(grip,name);rows.append(label);continue;
    }
    const checkbox=document.createElement("input");checkbox.type="checkbox";checkbox.dataset.dict=d.id;checkbox.checked=members.has(d.id);checkbox.disabled=group.readonly||groupSaving;
    label.append(checkbox,name);rows.append(label);
    checkbox.addEventListener("change",async()=>{
      const member=checkbox.checked,scrollTop=rows.scrollTop;groupSaving=true;$("groupError").textContent="";
      $("closeGroups").disabled=true;$("groupSelect").disabled=true;showAll.disabled=true;
      rows.querySelectorAll("input").forEach(input=>input.disabled=true);
      try{
        await groupRequest("/api/user-groups/member","PUT",{group:group.id,dict:d.id,member});
        group.members=group.members.filter(id=>id!==d.id);if(member)group.members.push(d.id);
        refreshLivePicker();if(group.id===pickerGroup&&$("q").value.trim())doSearch();
      }catch(error){$("groupError").textContent=error.message}
      finally{
        groupSaving=false;$("closeGroups").disabled=false;$("groupSelect").disabled=false;showAll.disabled=group.readonly;
        renderGroupRows();
        rows.scrollTop=scrollTop;
      }
    });
  }
  if(filtering&&!filterInserted)rows.append(...filterHost.childNodes);
  if(filtering&&!filteredOutside.length){const empty=document.createElement('p');empty.className='group-filter-empty';empty.textContent=tx('dictUI.filterEmpty');rows.append(empty)}
}
async function saveGroupOrder(ids,focusId){
  if(groupSaving)return;
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||(!group.readonly&&$("groupShowAll").checked))return;
  const visible=new Set(ids), reordered=ids.slice();
  // Replace visible slots only; temporarily unavailable dictionaries keep membership and position.
  ids=(group.readonly?orderedIds():group.members).map(id=>visible.has(id)?reordered.shift():id);
  const rows=$("groupRows"),top=rows.scrollTop;
  groupSaving=true;$("groupError").textContent="";
  $("closeGroups").disabled=true;$("groupSelect").disabled=true;$("groupShowAll").disabled=true;
  try{
    if(group.readonly){
      prefOrder=ids;savePrefs(true);refreshDictUI(true);
    }else{
      await groupRequest("/api/user-groups/order","PUT",{group:group.id,members:ids});
      group.members=ids;
    }
    refreshLivePicker();
    if(group.id===pickerGroup&&$("q").value.trim())doSearch();
  }catch(error){$("groupError").textContent=error.message}
  finally{
    groupSaving=false;$("closeGroups").disabled=false;$("groupSelect").disabled=false;
    renderGroupRows();rows.scrollTop=top;
    if(focusId){const grip=[...rows.querySelectorAll(".group-row")].find(row=>row.dataset.dict===focusId)?.querySelector(".group-grip");grip?.focus({preventScroll:true})}
  }
}
let groupDrag=null;
function clearGroupDrop(){
  $("groupRows").querySelectorAll(".dragging,.drop-before,.drop-after").forEach(row=>
    row.classList.remove("dragging","drop-before","drop-after"));
}
function updateGroupDrop(){
  if(!groupDrag)return;
  const drag=groupDrag,rows=$("groupRows");
  const row=document.elementFromPoint(drag.x,drag.y)?.closest(".group-row");
  rows.querySelectorAll(".drop-before,.drop-after").forEach(el=>el.classList.remove("drop-before","drop-after"));
  drag.target=null;
  if(!row||!rows.contains(row)||row.dataset.dict===drag.id)return;
  drag.target=row.dataset.dict;
  drag.after=drag.y>row.getBoundingClientRect().top+row.getBoundingClientRect().height/2;
  row.classList.add(drag.after?"drop-after":"drop-before");
}
function groupDragTick(){
  if(!groupDrag)return;
  const rows=$("groupRows"),rect=rows.getBoundingClientRect(),edge=44;
  if(groupDrag.y<rect.top+edge)rows.scrollTop-=Math.min(28,Math.max(0,(rect.top+edge-groupDrag.y)/5));
  else if(groupDrag.y>rect.bottom-edge)rows.scrollTop+=Math.min(28,Math.max(0,(groupDrag.y-rect.bottom+edge)/5));
  updateGroupDrop();
  groupDrag.raf=requestAnimationFrame(groupDragTick);
}
$("groupRows").addEventListener("pointerdown",event=>{
  const grip=event.target.closest(".group-grip");
  if(!grip||groupSaving||event.button!==0)return;
  event.preventDefault();
  const row=grip.closest(".group-row");
  groupDrag={id:row.dataset.dict,x:event.clientX,y:event.clientY,pointerId:event.pointerId,target:null,after:false,raf:null};
  row.classList.add("dragging");
  grip.setPointerCapture(event.pointerId);
  groupDrag.raf=requestAnimationFrame(groupDragTick);
});
$("groupRows").addEventListener("pointermove",event=>{
  if(!groupDrag||event.pointerId!==groupDrag.pointerId)return;
  event.preventDefault();groupDrag.x=event.clientX;groupDrag.y=event.clientY;updateGroupDrop();
});
function finishGroupDrag(event,cancel){
  if(!groupDrag||event.pointerId!==groupDrag.pointerId)return;
  groupDrag.x=event.clientX;groupDrag.y=event.clientY;updateGroupDrop();
  const drag=groupDrag;groupDrag=null;cancelAnimationFrame(drag.raf);clearGroupDrop();
  if(cancel||!drag.target)return;
  const group=userGroups.find(g=>g.id===selectedGroup);if(!group)return;
  const visible=new Set(orderedDicts().map(d=>d.id));
  const current=(group.readonly?orderedIds():group.members).filter(id=>visible.has(id));
  const ids=current.slice(),from=ids.indexOf(drag.id);if(from<0)return;
  ids.splice(from,1);
  let to=ids.indexOf(drag.target);if(to<0)return;
  if(drag.after)to++;
  ids.splice(to,0,drag.id);
  if(ids.some((id,i)=>id!==current[i]))saveGroupOrder(ids,drag.id);
}
$("groupRows").addEventListener("pointerup",event=>finishGroupDrag(event,false));
$("groupRows").addEventListener("pointercancel",event=>finishGroupDrag(event,true));
$("groupRows").addEventListener("keydown",event=>{
  if(!event.target.classList.contains("group-grip")||!(["ArrowUp","ArrowDown"].includes(event.key)))return;
  const group=userGroups.find(g=>g.id===selectedGroup);if(!group||groupSaving)return;
  const visible=new Set(orderedDicts().map(d=>d.id));
  const id=event.target.closest(".group-row").dataset.dict,ids=(group.readonly?orderedIds():group.members).filter(id=>visible.has(id)),from=ids.indexOf(id);
  const to=from+(event.key==="ArrowUp"?-1:1);
  if(from<0||to<0||to>=ids.length)return;
  event.preventDefault();ids.splice(from,1);ids.splice(to,0,id);saveGroupOrder(ids,id);
});
$("editGroups").addEventListener("click",async()=>{
  groupAvailableFilter="all";
  const button=$("editGroups");button.disabled=true;
  $("groupError").textContent="";$("groupHint").textContent=tx("panel.loading");$("groupRows").replaceChildren();
  $("groupSelect").disabled=true;$("groupEditor").showModal();
  try{
    userGroups=await groupRequest("/api/user-groups","GET");refreshLivePicker();
    if(!userGroups.some(g=>g.id===selectedGroup))selectedGroup="all";
    renderGroupOptions();renderGroupRows();$("groupSelect").disabled=false;
  }catch(error){$("groupError").textContent=error.message;$("groupHint").textContent=""}
  finally{button.disabled=false}
});
$("closeGroups").onclick=()=>$("groupEditor").close();
$("groupEditor").addEventListener("close",()=>$("editGroups").focus());
$("groupEditor").addEventListener("cancel",event=>{if(groupSaving)event.preventDefault()});
$("groupShowAll").onchange=()=>{groupShowAllPreferred=$("groupShowAll").checked;renderGroupRows()};
$("groupSelect").onchange=()=>{
  if($("groupSelect").value==="new"){
    $("groupSelect").value=selectedGroup;$("groupName").value="";$("newGroupError").textContent="";
    $("newGroupDialog").showModal();$("groupName").focus();
  }else{selectedGroup=$("groupSelect").value;$("groupError").textContent="";renderGroupRows()}
};
$("cancelGroup").onclick=()=>$("newGroupDialog").close();
$("newGroupDialog").addEventListener("close",()=>$("groupSelect").focus());
$("newGroupForm").onsubmit=async event=>{
  event.preventDefault();if($("createGroup").disabled)return;
  $("createGroup").disabled=true;$("cancelGroup").disabled=true;$("newGroupError").textContent="";
  try{
    const group=await groupRequest("/api/user-groups","POST",{name:$("groupName").value});
    userGroups.push(group);selectedGroup=group.id;renderGroupOptions();renderGroupRows();refreshLivePicker();$("newGroupDialog").close();
  }catch(error){$("newGroupError").textContent=error.message}
  finally{$("createGroup").disabled=false;$("cancelGroup").disabled=false}
};
$("newGroupDialog").addEventListener("cancel",event=>{if($("createGroup").disabled)event.preventDefault()});
// Promise.all resolves to an array: passing it to loadDicts would enable
// rescan and wait for index maintenance instead of listing dictionaries.
Promise.all([loadPrefs(),loadConfig(),loadUserCSS(),loadPickerGroups()])
  .then(()=>loadDicts()).then(applyURL).catch(error=>bootFailed(error.message));
// The preset layers are independent of the boot chain: their app halves are
// server-injected <link>s and their article halves join the article sheet
// whenever this answer lands, which is why this is fire-and-forget rather
// than a gate on the first search.
presetsLoad();
