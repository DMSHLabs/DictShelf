/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */


// Group membership never writes disabled, prefOrder or the search selector.
let userGroups=[], selectedGroup="all", groupSaving=false, groupShowAllPreferred=false;
let configuredGroupFilters=[];
let groupAvailableFilter="all",groupAvailableDicts=[];
let newGroupSelectedFilter=null,groupFilterMode="",groupFilterShowAll=false,groupFilterCollapsed=new Set();
function availableMatchesFilter(d){
  const filters=d.filters||[];
  return groupAvailableFilter==="all"||(groupAvailableFilter==="uncategorized"?filters.length===0:filters.some(g=>JSON.stringify([g.f,g.v])===groupAvailableFilter));
}
function appendAvailableFilter(rows,available){
  groupAvailableDicts=available;
  if(groupAvailableFilter!=='all'&&!available.some(availableMatchesFilter))groupAvailableFilter='all';
  const bar=document.createElement('div');bar.className='group-available-filter';
  const title=document.createElement('span');title.textContent=tx('dictUI.availableFilter');
  const button=document.createElement('button');button.type='button';button.id='groupAvailableFilter';button.className='btn';
  button.textContent=filterChoiceLabel(groupAvailableFilter);button.disabled=groupSaving;
  button.onclick=()=>openGroupFilterDialog('available');
  bar.append(title,button);rows.append(bar);
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
function groupFilters(){
  const found=new Map();
  for(const d of orderedDicts())for(const g of d.filters||[])found.set(JSON.stringify([g.f,g.v]),g);
  for(const g of configuredGroupFilters)if(!found.has(JSON.stringify([g.f,g.v])))found.set(JSON.stringify([g.f,g.v]),g);
  return [...found].sort((a,b)=>a[1].fo-b[1].fo||a[1].f.localeCompare(b[1].f)||a[1].vl.localeCompare(b[1].vl));
}
async function loadConfiguredGroupFilters(){
  try{
    const response=await fetch('/api/groups');
    if(!response.ok)return;
    const data=await response.json();
    configuredGroupFilters=(data.facets||[]).flatMap((f,fo)=>(f.groups||[]).map(g=>({f:f.id,v:g.id,fl:f.label,vl:g.label,fo:fo+3})));
  }catch(error){console.warn('could not load filters:',error)}
}
function filterChoiceLabel(value){
  if(value==='all')return tx('panel.allDictionaries');
  if(value==='uncategorized')return tx('dictUI.uncategorized');
  const g=groupFilters().find(([key])=>key===value)?.[1];
  return g?window.wudictI18n.facetLabels(g).vl:tx('dictUI.chooseFilter');
}
function filterCounts(ordered){
  const counts=new Map();
  for(const d of ordered){
    for(const key of new Set((d.filters||[]).map(g=>JSON.stringify([g.f,g.v]))))counts.set(key,(counts.get(key)||0)+1);
  }
  return counts;
}
function filterChoiceRow(label,key,count,checked,onClick){
  const row=document.createElement('button');row.type='button';row.className='group-filter-choice';
  row.setAttribute('role','radio');row.setAttribute('aria-checked',String(checked));
  const name=document.createElement('span');name.textContent=label;
  const end=document.createElement('span');end.className='group-filter-choice-end';
  if(count!==null){const number=document.createElement('span');number.className='group-filter-count';number.textContent=String(count);end.append(number)}
  const radio=document.createElement('span');radio.className='group-filter-radio';radio.setAttribute('aria-hidden','true');end.append(radio);
  row.append(name,end);row.onclick=onClick;return row;
}
function renderGroupFilterDialog(){
  const host=$('groupFilterChoices'),toggleHost=$('groupFilterToggle');host.replaceChildren();toggleHost.replaceChildren();
  const available=groupFilterMode==='available';
  const ordered=available?groupAvailableDicts:orderedDicts();
  const counts=filterCounts(ordered);
  const linked=userGroups.find(g=>g.id===selectedGroup);
  const chosen=groupFilterMode==='new'?newGroupSelectedFilter:linked?.filter||linked?.selectedFilter;
  const selected=available?groupAvailableFilter:chosen?JSON.stringify([chosen.facet,chosen.value]):'';
  if(available){
    host.append(filterChoiceRow(tx('panel.allDictionaries'),'all',null,selected==='all',()=>chooseGroupFilter('all')));
  }else{
    const toggle=document.createElement('label');toggle.className='group-filter-toggle';
    const check=document.createElement('input');check.type='checkbox';check.checked=groupFilterShowAll;
    check.onchange=()=>{groupFilterShowAll=check.checked;renderGroupFilterDialog()};
    toggle.append(check,document.createTextNode(tx('dictUI.showAllFilters')));toggleHost.append(toggle);
  }
  const sections=new Map();
  for(const [key,g] of groupFilters()){
    const count=counts.get(key)||0;
    if(!count&&!(!available&&groupFilterShowAll))continue;
    const labels=window.wudictI18n.facetLabels(g);
    let details=sections.get(g.f);
    if(!details){
      details=document.createElement('details');details.className='group-filter-section';details.open=!groupFilterCollapsed.has(g.f);
      details.addEventListener('toggle',()=>{
        if(details.open)groupFilterCollapsed.delete(g.f);else groupFilterCollapsed.add(g.f);
      });
      const summary=document.createElement('summary');summary.textContent=labels.fl;details.append(summary);
      sections.set(g.f,details);host.append(details);
    }
    details.append(filterChoiceRow(labels.vl,key,count,selected===key,()=>chooseGroupFilter(key)));
  }
  if(available&&ordered.some(d=>!(d.filters||[]).length)){
    const count=ordered.filter(d=>!(d.filters||[]).length).length;
    host.append(filterChoiceRow(tx('dictUI.uncategorized'),'uncategorized',count,selected==='uncategorized',()=>chooseGroupFilter('uncategorized')));
  }
}
function openGroupFilterDialog(mode){
  groupFilterMode=mode;groupFilterShowAll=false;groupFilterCollapsed=new Set();
  renderGroupFilterDialog();$('groupFilterDialog').showModal();
}
function chooseGroupFilter(key){
  $('groupFilterDialog').close();
  if(groupFilterMode==='available'){
    groupAvailableFilter=key;renderGroupRows();return;
  }
  const [facet,value]=JSON.parse(key),filter={facet,value};
  if(groupFilterMode==='new'){
    newGroupSelectedFilter=filter;$('newGroupFromFilter').checked=true;
    $('newGroupFilter').disabled=false;$('newGroupLink').disabled=false;
    $('newGroupFilter').textContent=filterChoiceLabel(key);
    if(!$('groupName').value.trim())$('groupName').value=filterChoiceLabel(key);
  }else saveGroupLink(filter);
}
$('cancelGroupFilter').onclick=()=>$('groupFilterDialog').close();
$('groupFilterDialog').addEventListener('close',()=>{
  if(groupFilterMode==='new'&&!newGroupSelectedFilter){
    $('newGroupFromFilter').checked=false;$('newGroupFilter').disabled=true;$('newGroupLink').disabled=true;
  }
});
function renderGroupLink(group){
  const controls=$("groupLinkControls");controls.hidden=group.readonly;
  if(group.readonly)return;
  $("groupLink").checked=!!group.filter;
  $("groupLink").disabled=groupSaving;
  $("groupLinkFilter").disabled=groupSaving;
  const selected=group.filter||group.selectedFilter;
  $("groupLinkFilter").textContent=selected?filterChoiceLabel(JSON.stringify([selected.facet,selected.value])):tx('dictUI.chooseFilter');
}
function groupHint(group,showAll,ordered){
  const members=new Set(group.members);
  const inside=ordered.filter(d=>members.has(d.id)).length;
  const outside=ordered.length-inside;
  return group.readonly?tx("dictUI.dragAll")
    :!showAll?(inside?tx("dictUI.dragGroup"):tx("dictUI.emptyGroup"))
    :inside?"":outside?tx("dictUI.addMembers"):tx("dictUI.noMembers");
}
function updateGroupSplit(){
  const editor=$('groupEditor'),lists=$('groupLists');
  if(!editor.open||!lists.classList.contains('editing'))return;
  const dialog=editor.getBoundingClientRect(),content=lists.getBoundingClientRect();
  lists.style.setProperty('--group-top-limit',Math.max(0,Math.floor(dialog.top+dialog.height/2-content.top))+'px');
}
window.addEventListener('resize',updateGroupSplit);
window.visualViewport?.addEventListener('resize',updateGroupSplit);
function renderGroupRows(){
  const group=userGroups.find(g=>g.id===selectedGroup), rows=$("groupRows"),otherRows=$('groupOtherRows');
  rows.replaceChildren();otherRows.replaceChildren();$('groupAvailableFilterHost').replaceChildren();
  $('groupLists').classList.remove('editing');$('groupAvailable').hidden=true;
  if(!group)return;
  renderGroupLink(group);
  const locked=group.readonly||!!group.filter;
  $('deleteGroup').hidden=group.readonly;
  $('deleteGroup').disabled=groupSaving;
  const showAll=$("groupShowAll");
  showAll.disabled=locked;
  showAll.checked=!locked&&groupShowAllPreferred;
  showAll.parentElement.classList.toggle("group-control-disabled",locked);
  const ordered=orderedDicts(), members=new Set(group.readonly?ordered.map(d=>d.id):group.members);
  const inside=group.readonly?ordered:orderedGroupDicts(group,ordered);
  const outside=locked?[]:ordered.filter(d=>!members.has(d.id));
  const filtering=showAll.checked&&!locked;
  $('groupLists').classList.toggle('editing',filtering);
  $('groupAvailable').hidden=!filtering;
  if(filtering)appendAvailableFilter($('groupAvailableFilterHost'),outside);
  const filteredOutside=filtering?outside.filter(availableMatchesFilter):outside;
  $('groupAddAll').disabled=groupSaving||groupAvailableFilter==='all'||!filteredOutside.length;
  $("groupHint").textContent=group.filter?tx("dictUI.linkedHint"):groupHint(group,showAll.checked,ordered);
  for(const d of filtering?inside.concat(filteredOutside):inside){
    const label=document.createElement(!locked&&showAll.checked?"label":"div");label.className="group-row";
    label.dataset.dict=d.id;
    const name=document.createElement("span");name.textContent=dictLabel(d);
    if(d.unavailable)name.append(" — ",tx("dictUI.dslUnavailable"));
    if(group.readonly||group.filter||!showAll.checked){
      const grip=document.createElement("button");grip.type="button";grip.className="group-grip";
      grip.textContent="≡";grip.setAttribute("aria-label",tx("dictUI.dragNamed",{name:dictLabel(d)}));
      label.append(grip,name);rows.append(label);continue;
    }
    const checkbox=document.createElement("input");checkbox.type="checkbox";checkbox.dataset.dict=d.id;checkbox.checked=members.has(d.id);checkbox.disabled=group.readonly||groupSaving;
    label.append(checkbox,name);(members.has(d.id)?rows:otherRows).append(label);
    checkbox.addEventListener("change",async()=>{
      const member=checkbox.checked,top=rows.scrollTop,otherTop=otherRows.scrollTop;groupSaving=true;$("groupError").textContent="";
      $("closeGroups").disabled=true;$("groupSelect").disabled=true;showAll.disabled=true;
      $('groupLists').querySelectorAll("input").forEach(input=>input.disabled=true);
      try{
        await groupRequest("/api/user-groups/member","PUT",{group:group.id,dict:d.id,member});
        group.members=group.members.filter(id=>id!==d.id);if(member)group.members.push(d.id);
        refreshLivePicker();if(group.id===pickerGroup&&$("q").value.trim())doSearch();
      }catch(error){$("groupError").textContent=error.message}
      finally{
        groupSaving=false;$("closeGroups").disabled=false;$("groupSelect").disabled=false;showAll.disabled=group.readonly;
        renderGroupRows();
        rows.scrollTop=top;otherRows.scrollTop=otherTop;
      }
    });
  }
  if(filtering&&!filteredOutside.length){const empty=document.createElement('p');empty.className='group-filter-empty';empty.textContent=tx('dictUI.filterEmpty');otherRows.append(empty)}
  requestAnimationFrame(updateGroupSplit);
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
    await loadConfiguredGroupFilters();
    userGroups=await groupRequest("/api/user-groups","GET");refreshLivePicker();
    if(!userGroups.some(g=>g.id===selectedGroup)){selectedGroup="all";groupShowAllPreferred=false}
    renderGroupOptions();renderGroupRows();$("groupSelect").disabled=false;
  }catch(error){$("groupError").textContent=error.message;$("groupHint").textContent=""}
  finally{button.disabled=false}
});
$("closeGroups").onclick=()=>$("groupEditor").close();
$("groupEditor").addEventListener("close",()=>$("editGroups").focus());
$("groupEditor").addEventListener("cancel",event=>{if(groupSaving)event.preventDefault()});
$("groupShowAll").onchange=()=>{groupShowAllPreferred=$("groupShowAll").checked;renderGroupRows()};
$('deleteGroup').onclick=()=>{
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||group.readonly||groupSaving)return;
  $('deleteGroupQuestion').textContent=tx('dictUI.deleteGroupConfirm',{name:group.name});
  $('deleteGroupError').textContent='';$('deleteGroupDialog').showModal();
};
$('deleteGroupCancel').onclick=()=>$('deleteGroupDialog').close();
$('deleteGroupDialog').addEventListener('cancel',event=>{if(groupSaving)event.preventDefault()});
$('deleteGroupGo').onclick=async()=>{
  const id=selectedGroup;
  if(groupSaving||id==='all')return;
  groupSaving=true;$('deleteGroupGo').disabled=true;$('deleteGroupCancel').disabled=true;
  $('deleteGroupError').textContent='';
  try{
    await groupRequest('/api/user-groups','DELETE',{group:id});
    userGroups=await groupRequest('/api/user-groups','GET');
    selectedGroup='all';groupShowAllPreferred=false;
    if(pickerGroup===id){pickerGroup='all';localStorage.setItem('wudict_picker_group','all');paintPickerGroup();if($('q').value.trim())doSearch()}
    renderGroupOptions();renderGroupRows();refreshLivePicker();$('deleteGroupDialog').close();
  }catch(error){$('deleteGroupError').textContent=error.message}
  finally{groupSaving=false;$('deleteGroupGo').disabled=false;$('deleteGroupCancel').disabled=false}
};
$('groupAddAll').onclick=async()=>{
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||group.readonly||group.filter||groupSaving||groupAvailableFilter==='all')return;
  const members=new Set(group.members);
  const ids=orderedDicts().filter(d=>!members.has(d.id)&&availableMatchesFilter(d)).map(d=>d.id);
  if(!ids.length)return;
  groupSaving=true;$('groupError').textContent='';
  $('closeGroups').disabled=true;$('groupSelect').disabled=true;$('groupShowAll').disabled=true;$('groupAddAll').disabled=true;
  $('groupOtherRows').querySelectorAll('input').forEach(input=>input.disabled=true);
  try{
    await groupRequest('/api/user-groups/members','PUT',{group:group.id,members:ids});
    userGroups=await groupRequest('/api/user-groups','GET');
    renderGroupOptions();refreshLivePicker();
    if(group.id===pickerGroup&&$('q').value.trim())doSearch();
  }catch(error){$('groupError').textContent=error.message}
  finally{
    groupSaving=false;$('closeGroups').disabled=false;$('groupSelect').disabled=false;
    renderGroupRows();
  }
};
function confirmGroupLinkReplacement(){
  const dialog=$("linkGroupDialog");
  dialog.returnValue="";
  return new Promise(resolve=>{
    dialog.addEventListener("close",()=>resolve(dialog.returnValue==="link"),{once:true});
    dialog.showModal();
  });
}
$("linkGroupGo").onclick=()=>$("linkGroupDialog").close("link");
$("linkGroupCancel").onclick=()=>$("linkGroupDialog").close("cancel");
async function saveGroupLink(filter){
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||group.readonly||groupSaving)return;
  if(filter&&!group.filter&&(group.order?.length||group.members.length)&&!await confirmGroupLinkReplacement()){
    renderGroupLink(group);
    return;
  }
  groupSaving=true;$("groupError").textContent="";
  try{
    await groupRequest("/api/user-groups/link","PUT",{group:group.id,filter});
    userGroups=await groupRequest("/api/user-groups","GET");
    if(!filter)groupShowAllPreferred=false;
    renderGroupOptions();renderGroupRows();refreshLivePicker();
    if(group.id===pickerGroup&&$("q").value.trim())doSearch();
  }catch(error){$("groupError").textContent=error.message;renderGroupLink(group)}
  finally{groupSaving=false;renderGroupRows()}
}
$("groupLink").onchange=()=>{
  if($("groupLink").checked){
    const group=userGroups.find(g=>g.id===selectedGroup),selected=group?.selectedFilter||group?.filter;
    if(selected)saveGroupLink(selected);
    else{$("groupLink").checked=false;openGroupFilterDialog('link')}
  }
  else saveGroupLink(null);
};
$("groupLinkFilter").onclick=()=>openGroupFilterDialog('link');
$("newGroupFromFilter").onchange=()=>{
  const on=$("newGroupFromFilter").checked;
  if(on)openGroupFilterDialog('new');
  else{newGroupSelectedFilter=null;$("newGroupFilter").disabled=true;$("newGroupFilter").textContent=tx('dictUI.chooseFilter');$("newGroupLink").checked=false;$("newGroupLink").disabled=true}
};
$("newGroupFilter").onclick=()=>openGroupFilterDialog('new');
$("groupSelect").onchange=()=>{
  if($("groupSelect").value==="new"){
    $("groupSelect").value=selectedGroup;$("groupName").value="";$("newGroupError").textContent="";
    newGroupSelectedFilter=null;$("newGroupFromFilter").checked=false;$("newGroupLink").checked=false;
    $("newGroupFromFilter").disabled=!groupFilters().length;
    $("newGroupFilter").textContent=tx('dictUI.chooseFilter');
    $("newGroupFilter").disabled=true;$("newGroupLink").disabled=true;
    $("newGroupDialog").showModal();$("groupName").focus();
  }else{selectedGroup=$("groupSelect").value;groupShowAllPreferred=false;$("groupError").textContent="";renderGroupRows()}
};
$("cancelGroup").onclick=()=>$("newGroupDialog").close();
$("newGroupDialog").addEventListener("close",()=>$("groupSelect").focus());
$("newGroupForm").onsubmit=async event=>{
  event.preventDefault();if($("createGroup").disabled)return;
  $("createGroup").disabled=true;$("cancelGroup").disabled=true;$("newGroupError").textContent="";
  try{
    const filter=$("newGroupFromFilter").checked?newGroupSelectedFilter:null;
    const group=await groupRequest("/api/user-groups","POST",{name:$("groupName").value,filter,linked:!!filter&&$("newGroupLink").checked});
    userGroups.push(group);selectedGroup=group.id;groupShowAllPreferred=false;renderGroupOptions();renderGroupRows();refreshLivePicker();$("newGroupDialog").close();
  }catch(error){$("newGroupError").textContent=error.message}
  finally{$("createGroup").disabled=false;$("cancelGroup").disabled=false}
};
$("newGroupDialog").addEventListener("cancel",event=>{if($("createGroup").disabled)event.preventDefault()});
// Promise.all resolves to an array: passing it to loadDicts would enable
// rescan and wait for index maintenance instead of listing dictionaries.
Promise.all([loadPrefs(),loadConfig(),loadUserCSS(),loadPickerGroups()])
  .then(()=>loadDicts()).then(ok=>{if(ok)applyURL()}).catch(error=>bootFailed(error.message));
// The preset layers are independent of the boot chain: their app halves are
// server-injected <link>s and their article halves join the article sheet
// whenever this answer lands, which is why this is fire-and-forget rather
// than a gate on the first search.
presetsLoad();
