/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */


// Group membership never writes disabled, prefOrder or the search selector.
let userGroups=[], selectedGroup="all", groupSaving=false, groupShowAllPreferred=false;
let configuredGroupFilters=[];
let groupAvailableFilter="all",groupAvailableDicts=[];
let groupActivePanel=null;
let newGroupSelectedFilter=null,groupFilterMode="",groupFilterShowAll=false,groupFilterCollapsed=new Set();
let groupEditorMode="",groupDraftMembers=new Set(),groupDraftFilter=null,groupDraftLinked=false,groupSavedEditPreference=false,groupDraftChooseForLink=false;
let groupMemberDraft=null;
let groupOrderSelection=null,groupOrderLongPress=null,groupOrderSuppressClick=false;
function availableMatchesFilter(d){
  const filters=d.filters||[];
  return groupAvailableFilter==="all"||(groupAvailableFilter==="uncategorized"?filters.length===0:filters.some(g=>JSON.stringify([g.f,g.v])===groupAvailableFilter));
}
function availableSearchText(){return $('groupAvailableSearch').value.trim().toLocaleLowerCase()}
function availableMatchesSearch(d){return dictLabel(d).toLocaleLowerCase().includes(availableSearchText())}
function highlightGroupSearch(row,query){
  const name=row.querySelector('.group-dictionary-name');if(!name)return;
  const text=name.dataset.fullText||name.textContent;name.dataset.fullText=text;
  const value=query.trim();if(!value){name.textContent=text;return}
  const lower=text.toLocaleLowerCase(),needle=value.toLocaleLowerCase();let from=0,at;
  const parts=[];
  while((at=lower.indexOf(needle,from))!==-1){
    if(at>from)parts.push(document.createTextNode(text.slice(from,at)));
    const mark=document.createElement('mark');mark.className='group-search-match';mark.textContent=text.slice(at,at+value.length);parts.push(mark);from=at+value.length;
  }
  if(from===0){name.textContent=text;return}
  if(from<text.length)parts.push(document.createTextNode(text.slice(from)));
  name.replaceChildren(...parts);
}
function updateMemberSearch(){
  const rows=$('groupRows'),search=$('groupMembersSearch').value.trim().toLocaleLowerCase();
  let visible=0;
  for(const row of rows.querySelectorAll('.group-row')){
    row.hidden=!row.dataset.search.includes(search);
    highlightGroupSearch(row,search);
    if(!row.hidden)visible++;
  }
  rows.querySelector('.group-filter-empty')?.remove();
  if(!visible&&search){
    const empty=document.createElement('p');empty.className='group-filter-empty';
    empty.textContent=tx('dictUI.memberSearchEmpty');rows.append(empty);
  }
  requestAnimationFrame(updateGroupSplit);
}
function updateAvailableSearch(){
  const rows=$('groupOtherRows'),search=availableSearchText();
  let visible=0;
  for(const row of rows.querySelectorAll('.group-row')){
    row.hidden=!row.dataset.search.includes(search);
    highlightGroupSearch(row,search);
    if(!row.hidden)visible++;
  }
  rows.querySelector('.group-filter-empty')?.remove();
  if(!visible){
    const empty=document.createElement('p');empty.className='group-filter-empty';
    empty.textContent=tx(search?'dictUI.searchEmpty':'dictUI.filterEmpty');rows.append(empty);
  }
  $('groupAddAll').disabled=groupSaving||groupAvailableFilter==='all'||!visible;
  requestAnimationFrame(updateGroupSplit);
}
$('groupAvailableSearch').addEventListener('input',updateAvailableSearch);
$('groupMembersSearch').addEventListener('input',updateMemberSearch);
for(const [searchId,clearId] of [['groupAvailableSearch','groupAvailableSearchClear'],['groupMembersSearch','groupMembersSearchClear']]){
  const search=$(searchId),clear=$(clearId);
  search.addEventListener('input',()=>{clear.hidden=!search.value});
  clear.addEventListener('pointerdown',event=>event.preventDefault());
  clear.addEventListener('click',()=>{
    if(!search.value)return;
    search.value='';search.dispatchEvent(new Event('input',{bubbles:true}));search.focus();
  });
}
for(const id of ['groupAvailableSearch','groupMembersSearch'])$(id).addEventListener('keydown',event=>{
  if(event.key==='Enter'){event.preventDefault();event.currentTarget.blur()}
});
function groupSearchKeyboardOpen(){
  const height=window.visualViewport?.height||window.innerHeight;
  return ['groupAvailableSearch','groupMembersSearch'].some(id=>document.activeElement===$(id))&&height<groupEditorFullViewportHeight-100;
}
function updateGroupKeyboardToggle(){
  for(const [buttonId,searchId] of [['groupSearchDone','groupAvailableSearch'],['groupMembersKeyboard','groupMembersSearch']]){
    const button=$(buttonId),open=groupSearchKeyboardOpen()&&document.activeElement===$(searchId);
    button.dataset.keyboard=open?'hide':'show';
    button.setAttribute('aria-label',tx(open?'dictUI.hideKeyboard':'dictUI.showKeyboard'));
    button.title=button.getAttribute('aria-label');
  }
}
for(const [buttonId,searchId] of [['groupSearchDone','groupAvailableSearch'],['groupMembersKeyboard','groupMembersSearch']]){
  let wasOpen=null;
  $(buttonId).addEventListener('pointerdown',()=>{wasOpen=groupSearchKeyboardOpen()&&document.activeElement===$(searchId)});
  $(buttonId).onclick=()=>{
    const search=$(searchId),open=wasOpen??(groupSearchKeyboardOpen()&&document.activeElement===search);
    wasOpen=null;
    if(open)search.blur();
    else{search.blur();search.focus()}
    requestAnimationFrame(updateGroupKeyboardToggle);
  };
}
function activateGroupPanel(panel){
  if(groupEditorMode&&panel!=='controls')return;
  groupActivePanel=panel;
  $('groupControls').classList.toggle('active',panel==='controls');
  $('groupMembers').classList.toggle('active',panel==='members');
  $('groupAvailable').classList.toggle('active',panel==='available');
  requestAnimationFrame(updateGroupSplit);
}
for(const [panelId,panel] of [['groupControls','controls'],['groupMembers','members'],['groupAvailable','available']]){
  $(panelId).addEventListener('pointerdown',()=>activateGroupPanel(panel));
}
for(const [searchId,panel] of [['groupMembersSearch','members'],['groupAvailableSearch','available']]){
  $(searchId).addEventListener('focus',()=>activateGroupPanel(panel));
  $(searchId).addEventListener('blur',()=>requestAnimationFrame(updateGroupSplit));
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
  select.value=selectedGroup;
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
  const chosen=groupFilterMode==='draft'?groupDraftFilter:groupFilterMode==='new'?newGroupSelectedFilter:linked?.filter||linked?.selectedFilter;
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
  $('groupFilterDialog').returnValue='';renderGroupFilterDialog();$('groupFilterDialog').showModal();
}
function chooseGroupFilter(key){
  $('groupFilterDialog').close('selected');
  if(groupFilterMode==='available'){
    groupAvailableFilter=key;renderGroupRows();return;
  }
  const [facet,value]=JSON.parse(key),filter={facet,value};
  if(groupFilterMode==='draft'){
    groupDraftFilter=filter;if(groupDraftChooseForLink)groupDraftLinked=true;groupDraftChooseForLink=false;
    if(groupEditorMode==='new')groupDraftMembers=new Set(orderedDicts().filter(d=>(d.filters||[]).some(g=>g.f===facet&&g.v===value)).map(d=>d.id));
    $('groupLink').checked=groupDraftLinked;$('groupLinkFilter').textContent=filterChoiceLabel(key);
    if(!$('groupNameInline').value.trim())$('groupNameInline').value=filterChoiceLabel(key);
    renderGroupRows();return;
  }
  if(groupFilterMode==='new'){
    newGroupSelectedFilter=filter;$('newGroupFromFilter').checked=true;
    $('newGroupFilter').disabled=false;$('newGroupLink').disabled=false;
    $('newGroupFilter').textContent=filterChoiceLabel(key);
    if(!$('groupName').value.trim())$('groupName').value=filterChoiceLabel(key);
  }else saveGroupLink(filter);
}
$('cancelGroupFilter').onclick=()=>$('groupFilterDialog').close();
$('closeGroupFilter').onclick=()=>$('groupFilterDialog').close();
$('groupFilterDialog').addEventListener('close',()=>{
  if(groupFilterMode==='link'&&$('groupFilterDialog').returnValue!=='selected'){
    const group=userGroups.find(g=>g.id===selectedGroup);if(group)renderGroupLink(group);
  }
  if(groupFilterMode==='draft'&&groupDraftChooseForLink){groupDraftChooseForLink=false;$('groupLink').checked=false}
  if(groupFilterMode==='new'&&!newGroupSelectedFilter){
    $('newGroupFromFilter').checked=false;$('newGroupFilter').disabled=true;$('newGroupLink').disabled=true;
  }
});
function renderGroupLink(group){
  const controls=$("groupLinkControls");controls.hidden=group.readonly;
  if(group.readonly)return;
  const linked=groupEditorMode==='rename'?groupDraftLinked:!!group.filter;
  $("groupLink").checked=linked;
  $("groupLink").disabled=groupSaving||groupEditorMode==='rename';
  $("groupLinkFilter").disabled=groupSaving||groupEditorMode==='rename'||!linked;
  const selected=groupEditorMode==='rename'?groupDraftFilter:group.filter||group.selectedFilter;
  $("groupLinkFilter").textContent=selected?filterChoiceLabel(JSON.stringify([selected.facet,selected.value])):tx('dictUI.chooseFilter');
  $('groupLinkFilterHost').classList.toggle('can-link',!linked&&!selected&&!groupSaving&&groupEditorMode!=='rename');
}
let groupEditorFullViewportHeight=0;
function updateGroupSplit(){
  const editor=$('groupEditor'),lists=$('groupLists');
  editor.classList.remove('compact-controls');lists.classList.remove('hide-members','members-sized');
  const members=$('groupMembers'),available=$('groupAvailable');
  members.classList.remove('search-collapsed');available.classList.remove('search-collapsed');
  if(!editor.open)return;
  const viewportHeight=window.visualViewport?.height||window.innerHeight;
  if(!['groupAvailableSearch','groupMembersSearch'].some(id=>document.activeElement===$(id)))groupEditorFullViewportHeight=viewportHeight;
  else groupEditorFullViewportHeight=Math.max(groupEditorFullViewportHeight,viewportHeight);
  updateGroupKeyboardToggle();
  if(!lists.classList.contains('editing')){
    if(groupActivePanel!=='members'&&$('groupRows').clientHeight<4*groupRowHeight('groupRows'))members.classList.add('search-collapsed');
    return;
  }
  const memberRows=$('groupRows'),availableRows=$('groupOtherRows');
  const memberChrome=groupPanelChromeHeight(members,memberRows);
  const availableChrome=groupPanelChromeHeight(available,availableRows);
  const memberNatural=memberChrome+groupRowsContentHeight(memberRows);
  const gap=parseFloat(getComputedStyle(lists).rowGap)||0;
  if(groupActivePanel==='members'){
    const listsHeight=lists.clientHeight;
    const availableRow=groupRowsCapacityHeight(availableRows,1);
    const fullLimit=listsHeight-gap-availableChrome-availableRow;
    if(memberNatural>fullLimit)available.classList.add('search-collapsed');
    const availableReserve=groupPanelChromeHeight(available,availableRows,available.classList.contains('search-collapsed'))+availableRow;
    const memberHeight=Math.max(0,Math.min(memberNatural,listsHeight-gap-availableReserve));
    lists.style.setProperty('--group-members-height',Math.ceil(memberHeight)+'px');
    lists.classList.add('members-sized');
  }else if(groupActivePanel==='available'){
    editor.classList.add('compact-controls');
    const listsHeight=lists.clientHeight;
    const memberMin=groupPanelChromeHeight(members,memberRows,true)+groupRowsCapacityHeight(memberRows,3);
    const availableMin=availableChrome+groupRowsCapacityHeight(availableRows,5);
    if(memberMin+availableMin+gap<=listsHeight){
      const memberHeight=Math.max(memberMin,Math.min(memberNatural,listsHeight-availableMin-gap));
      lists.style.setProperty('--group-members-height',Math.ceil(memberHeight)+'px');
      lists.classList.add('members-sized');
    }else lists.classList.add('hide-members');
  }
  if(groupActivePanel!=='members'&&$('groupRows').clientHeight<4*groupRowHeight('groupRows'))members.classList.add('search-collapsed');
  if(groupActivePanel!=='available'&&$('groupOtherRows').clientHeight<4*groupRowHeight('groupOtherRows'))available.classList.add('search-collapsed');
}
function groupRowHeight(id){return Math.max(44,$(id).querySelector('.group-row:not([hidden])')?.getBoundingClientRect().height||0)}
function groupPanelChromeHeight(panel,rows,withoutSearch=false){
  const style=getComputedStyle(panel);
  let height=['borderTopWidth','borderBottomWidth','paddingTop','paddingBottom'].reduce((sum,key)=>sum+(parseFloat(style[key])||0),0);
  for(const child of panel.children){
    if(child===rows||child.hidden||(withoutSearch&&child.classList.contains('group-search-bar')))continue;
    const childStyle=getComputedStyle(child);
    if(childStyle.display==='none')continue;
    height+=child.getBoundingClientRect().height+(parseFloat(childStyle.marginTop)||0)+(parseFloat(childStyle.marginBottom)||0);
  }
  return height;
}
function groupRowsContentHeight(rows){
  let height=0;
  for(const child of rows.children){
    if(child.hidden)continue;
    const style=getComputedStyle(child);
    height+=child.getBoundingClientRect().height+(parseFloat(style.marginTop)||0)+(parseFloat(style.marginBottom)||0);
  }
  return height;
}
function groupRowsCapacityHeight(rows,count){
  const heights=[...rows.querySelectorAll('.group-row:not([hidden])')].slice(0,count).map(row=>row.getBoundingClientRect().height);
  const fallback=Math.max(44,heights.at(-1)||0);
  return heights.reduce((sum,height)=>sum+height,0)+(count-heights.length)*fallback;
}
window.addEventListener('resize',updateGroupSplit);
window.visualViewport?.addEventListener('resize',updateGroupSplit);
function groupMemberDraftDirty(){
  if(!groupMemberDraft)return false;
  return groupMemberDraft.baseMembers.length!==groupMemberDraft.members.size||groupMemberDraft.baseMembers.some(id=>!groupMemberDraft.members.has(id))||groupMemberDraft.baseOrder.length!==groupMemberDraft.order.length||groupMemberDraft.baseOrder.some((id,i)=>groupMemberDraft.order[i]!==id);
}
function visibleGroupOrder(group){
  return group.readonly?orderedIds():orderedGroupDicts(group,orderedDicts()).map(d=>d.id);
}
function movedGroupSelection(order,selection,direction){
  const picked=order.filter(id=>selection.has(id));
  if(!picked.length)return order;
  const remaining=order.filter(id=>!selection.has(id));
  const first=order.indexOf(picked[0]),last=order.indexOf(picked.at(-1));
  const at=direction==='top'?0:direction==='bottom'?remaining.length
    :direction==='up'?Math.max(0,first-1):Math.min(remaining.length,last-picked.length+2);
  remaining.splice(at,0,...picked);
  return remaining;
}
function updateGroupOrderToolbar(group,inside,filtering,nameMode){
  if(groupOrderSelection&&(groupOrderSelection.groupId!==group.id||filtering||nameMode))groupOrderSelection=null;
  const visible=!filtering&&!nameMode&&inside.length>0;
  const selecting=visible&&!!groupOrderSelection;
  $('groupOrderToolbar').hidden=!visible;
  $('groupOrderHint').hidden=selecting;
  $('groupOrderControls').hidden=!selecting;
  $('groupMembers').classList.toggle('order-selecting',selecting);
  if(selecting){
    const order=visibleGroupOrder(group),selected=groupOrderSelection.ids;
    for(const id of [...selected])if(!order.includes(id))selected.delete(id);
    $('groupOrderCount').textContent=String(selected.size);
    for(const [direction,button] of [['top','groupOrderTop'],['up','groupOrderUp'],['down','groupOrderDown'],['bottom','groupOrderBottom']]){
      $(button).disabled=groupSaving||!selected.size||movedGroupSelection(order,selected,direction).every((id,i)=>id===order[i]);
    }
    $('groupOrderExit').disabled=groupSaving;
  }
  return selecting;
}
function beginGroupMemberDraft(group){
  if(!group||group.readonly||group.filter)return;
  const members=[...group.members],order=[...(group.order||group.members)];
  groupMemberDraft={groupId:group.id,baseMembers:members,members:new Set(members),baseOrder:order,order};
}
function confirmDiscardGroupDraft(onDiscard){
  const dialog=$("discardGroupDraftDialog");
  dialog.returnValue="";
  dialog.addEventListener("close",()=>{if(dialog.returnValue==="discard")onDiscard()},{once:true});
  dialog.showModal();
}
function renderGroupRows(){
  let group=userGroups.find(g=>g.id===selectedGroup);
  if(groupEditorMode==='new')group={id:'__draft__',name:$('groupNameInline').value,members:[...groupDraftMembers],order:[...groupDraftMembers],readonly:false,filter:groupDraftLinked&&groupDraftFilter?{facet:groupDraftFilter.facet,value:groupDraftFilter.value}:null,selectedFilter:groupDraftFilter?{facet:groupDraftFilter.facet,value:groupDraftFilter.value}:null};
  const rows=$("groupRows"),otherRows=$('groupOtherRows');
  rows.replaceChildren();otherRows.replaceChildren();$('groupAvailableFilterHost').replaceChildren();
  $('groupLists').classList.remove('editing');$('groupAvailable').hidden=true;
  if(!group)return;
  const nameMode=!!groupEditorMode;
  $('groupSelect').hidden=nameMode;$('groupNameInline').hidden=!nameMode;
  $('groupNameCancel').hidden=!nameMode;$('groupNameApply').hidden=!nameMode;$('groupNewAction').hidden=nameMode;
  $('closeGroups').setAttribute('aria-label',tx(nameMode?'panel.cancel':'dictUI.closeGroups'));
  $('closeGroups').title=$('closeGroups').getAttribute('aria-label');
  $('groupEditor').classList.toggle('name-mode',nameMode);
  $('groupMembers').inert=nameMode;$('groupAvailable').inert=nameMode;
  $('groupRenameAction').hidden=nameMode;$('groupRenameAction').disabled=group.readonly||groupSaving;
  $('deleteGroup').hidden=group.readonly||nameMode;
  $('groupNameInline').disabled=groupSaving;
  renderGroupLink(group);
  const locked=group.readonly||!!group.filter;
  $('deleteGroup').disabled=groupSaving;
  const showAll={checked:!locked&&(nameMode||groupShowAllPreferred)};
  const ordered=orderedDicts();
  const members=groupMemberDraft?.groupId===group.id?groupMemberDraft.members:new Set(group.readonly?ordered.map(d=>d.id):group.members);
  const displayGroup=groupMemberDraft?.groupId===group.id?{...group,members:[...members],order:groupMemberDraft.order}:group;
  const inside=group.readonly?ordered:orderedGroupDicts(displayGroup,ordered);
  const outside=locked?[]:ordered.filter(d=>!members.has(d.id));
  const filtering=showAll.checked&&!locked;
  const selecting=updateGroupOrderToolbar(group,inside,filtering,nameMode);
  const showEmptyHint=!nameMode&&!filtering&&!group.readonly&&!group.filter&&members.size===0;
  $('groupLists').classList.toggle('editing',filtering);
  $('groupEditor').classList.toggle('edit-members',filtering&&!nameMode);
  $('groupMemberDraftActions').hidden=!!(locked||nameMode||selecting);
  $('groupMemberHint').hidden=!filtering||nameMode;
  $('groupEditToggle').hidden=filtering;$('groupEditToggle').disabled=groupSaving;
  $('groupMemberApply').hidden=!filtering;$('groupMemberCancel').hidden=!filtering;
  $('groupMemberApply').disabled=groupSaving||!groupMemberDraftDirty();
  $('groupMemberCancel').disabled=groupSaving;
  $('groupAvailable').hidden=!filtering;
  $('groupAvailableSearch').disabled=groupSaving;
  $('groupMembersSearch').disabled=groupSaving;
  if(filtering)appendAvailableFilter($('groupAvailableFilterHost'),outside);
  const filteredOutside=filtering?outside.filter(availableMatchesFilter):outside;
  $("groupHint").hidden=true;
  $("groupHint").textContent='';
  $('groupEmptyHint').hidden=!showEmptyHint;
  $('groupEmptyHintText').textContent=showEmptyHint?tx('dictUI.emptyGroup'):'';
  for(const d of filtering?inside.concat(filteredOutside):inside){
    const label=document.createElement(selecting||(!locked&&showAll.checked)?"label":"div");label.className="group-row";
    label.dataset.dict=d.id;label.dataset.search=dictLabel(d).toLocaleLowerCase();
    const name=document.createElement("span");name.className='group-dictionary-name';name.textContent=dictLabel(d);
    if(d.unavailable)name.append(" — ",tx("dictUI.dslUnavailable"));
    if(selecting){
      const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.className='group-order-select';
      checkbox.checked=groupOrderSelection.ids.has(d.id);checkbox.disabled=groupSaving;
      checkbox.setAttribute('aria-label',tx('dictUI.selectNamed',{name:dictLabel(d)}));
      checkbox.addEventListener('change',()=>{
        if(checkbox.checked)groupOrderSelection.ids.add(d.id);else groupOrderSelection.ids.delete(d.id);
        updateGroupOrderToolbar(group,inside,false,false);
      });
      label.append(checkbox,name);rows.append(label);continue;
    }
    if(group.readonly||group.filter||!showAll.checked){
      const grip=document.createElement("button");grip.type="button";grip.className="group-grip";
      grip.textContent="≡";grip.setAttribute("aria-label",tx("dictUI.dragNamed",{name:dictLabel(d)}));
      label.append(grip,name);rows.append(label);continue;
    }
    const checkbox=document.createElement("input");checkbox.type="checkbox";checkbox.dataset.dict=d.id;checkbox.checked=members.has(d.id);checkbox.disabled=group.readonly||groupSaving;
    label.append(checkbox,name);(members.has(d.id)?rows:otherRows).append(label);
    checkbox.addEventListener("change",async()=>{
      if(groupEditorMode==='new'){
        if(checkbox.checked)groupDraftMembers.add(d.id);else groupDraftMembers.delete(d.id);
        renderGroupRows();return;
      }
      if(groupMemberDraft?.groupId===group.id){
        if(checkbox.checked){groupMemberDraft.members.add(d.id);groupMemberDraft.order.push(d.id)}else{groupMemberDraft.members.delete(d.id);groupMemberDraft.order=groupMemberDraft.order.filter(id=>id!==d.id)}
        renderGroupRows();return;
      }
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
  updateMemberSearch();
  if(filtering)updateAvailableSearch();
  requestAnimationFrame(updateGroupSplit);
}
async function saveGroupOrder(ids,focusId){
  if(groupSaving)return;
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||(!group.readonly&&groupShowAllPreferred&&!groupMemberDraft))return;
  const visible=new Set(ids), reordered=ids.slice();
  // Replace visible slots only; temporarily unavailable dictionaries keep membership and position.
  ids=(groupMemberDraft?.groupId===group.id?groupMemberDraft.order:(group.readonly?orderedIds():group.members)).map(id=>visible.has(id)?reordered.shift():id);
  if(groupMemberDraft?.groupId===group.id){groupMemberDraft.order=ids;renderGroupRows();return}
  const rows=$("groupRows"),top=rows.scrollTop;
  groupSaving=true;$("groupError").textContent="";
  $("closeGroups").disabled=true;$("groupSelect").disabled=true;
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
  const current=(groupMemberDraft?.groupId===group.id?groupMemberDraft.order:(group.readonly?orderedIds():group.members)).filter(id=>visible.has(id));
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
  const id=event.target.closest(".group-row").dataset.dict,ids=(groupMemberDraft?.groupId===group.id?groupMemberDraft.order:(group.readonly?orderedIds():group.members)).filter(id=>visible.has(id)),from=ids.indexOf(id);
  const to=from+(event.key==="ArrowUp"?-1:1);
  if(from<0||to<0||to>=ids.length)return;
  event.preventDefault();ids.splice(from,1);ids.splice(to,0,id);saveGroupOrder(ids,id);
});
function cancelGroupOrderLongPress(){
  if(groupOrderLongPress)clearTimeout(groupOrderLongPress.timer);
  groupOrderLongPress=null;
}
$('groupRows').addEventListener('pointerdown',event=>{
  if(groupOrderSelection)groupOrderSuppressClick=false;
  const name=event.target.closest('.group-dictionary-name');
  if(!name||event.button!==0||groupSaving||groupOrderSelection||$('groupOrderToolbar').hidden)return;
  const row=name.closest('.group-row');if(!row)return;
  cancelGroupOrderLongPress();
  const press={pointerId:event.pointerId,x:event.clientX,y:event.clientY,timer:null};
  press.timer=setTimeout(()=>{
    if(groupOrderLongPress!==press)return;
    groupOrderLongPress=null;
    const group=userGroups.find(g=>g.id===selectedGroup);
    if(!group||groupSaving||!visibleGroupOrder(group).includes(row.dataset.dict))return;
    groupOrderSelection={groupId:group.id,ids:new Set([row.dataset.dict])};
    groupOrderSuppressClick=true;
    const rows=$('groupRows'),top=rows.scrollTop;
    renderGroupRows();rows.scrollTop=top;
  },550);
  groupOrderLongPress=press;
});
$('groupRows').addEventListener('pointermove',event=>{
  const press=groupOrderLongPress;
  if(press&&event.pointerId===press.pointerId&&Math.hypot(event.clientX-press.x,event.clientY-press.y)>10)cancelGroupOrderLongPress();
});
for(const type of ['pointerup','pointercancel','pointerleave'])$('groupRows').addEventListener(type,cancelGroupOrderLongPress);
$('groupRows').addEventListener('contextmenu',event=>{
  if(!$('groupOrderToolbar').hidden&&event.target.closest('.group-dictionary-name'))event.preventDefault();
});
$('groupRows').addEventListener('click',event=>{
  if(groupOrderSuppressClick){event.preventDefault();event.stopPropagation();groupOrderSuppressClick=false}
},true);
for(const [id,direction] of [['groupOrderTop','top'],['groupOrderUp','up'],['groupOrderDown','down'],['groupOrderBottom','bottom']]){
  $(id).addEventListener('click',()=>{
    const group=userGroups.find(g=>g.id===selectedGroup);
    if(!group||!groupOrderSelection||groupOrderSelection.groupId!==group.id||groupSaving)return;
    const order=visibleGroupOrder(group),next=movedGroupSelection(order,groupOrderSelection.ids,direction);
    if(next.some((value,i)=>value!==order[i]))saveGroupOrder(next);
  });
}
$('groupOrderExit').addEventListener('click',()=>{
  const rows=$('groupRows'),top=rows.scrollTop;
  groupOrderSelection=null;renderGroupRows();rows.scrollTop=top;
});
function matchGroupFontsToSettings(){
  const size=getComputedStyle($('editGroups')).fontSize;
  for(const id of ['groupEditor','newGroupDialog','groupFilterDialog','linkGroupDialog','deleteGroupDialog','discardGroupDraftDialog'])$(id).style.fontSize=size;
  $('groupEditor').style.setProperty('--group-action-font-size',size);
}
$("editGroups").addEventListener("click",async()=>{
  matchGroupFontsToSettings();
  cancelGroupOrderLongPress();groupOrderSelection=null;groupOrderSuppressClick=false;
  groupAvailableFilter="all";
  groupActivePanel=null;$('groupControls').classList.remove('active');$('groupMembers').classList.remove('active');$('groupAvailable').classList.remove('active');
  groupEditorFullViewportHeight=0;
  const button=$("editGroups");button.disabled=true;
  $("groupError").textContent="";$("groupHint").hidden=false;$("groupHint").textContent=tx("panel.loading");$("groupRows").replaceChildren();
  $("groupSelect").disabled=true;$("groupEditor").showModal();
  try{
    await loadConfiguredGroupFilters();
    userGroups=await groupRequest("/api/user-groups","GET");refreshLivePicker();
    if(!userGroups.some(g=>g.id===selectedGroup)){selectedGroup="all";groupShowAllPreferred=false}
    renderGroupOptions();renderGroupRows();$("groupSelect").disabled=false;
  }catch(error){$("groupError").textContent=error.message;$("groupHint").hidden=true;$("groupHint").textContent=""}
  finally{button.disabled=false}
});
$("closeGroups").onclick=()=>{
  if(groupEditorMode){cancelGroupNameEdit();return}
  if(groupMemberDraftDirty()){confirmDiscardGroupDraft(()=>{groupMemberDraft=null;$("groupEditor").close()});return}
  groupMemberDraft=null;$("groupEditor").close();
};
$("groupEditor").addEventListener("close",()=>{
  cancelGroupOrderLongPress();groupOrderSelection=null;groupOrderSuppressClick=false;
  if(groupEditorMode){groupEditorMode='';groupShowAllPreferred=groupSavedEditPreference;groupDraftMembers.clear();groupDraftFilter=null;groupDraftLinked=false;$('groupNameInline').value='';}
  $('editGroups').focus();
});
$("groupEditor").addEventListener("cancel",event=>{
  if(groupSaving){event.preventDefault();return}
  if(groupEditorMode){event.preventDefault();cancelGroupNameEdit();return}
  if(groupMemberDraftDirty()){event.preventDefault();confirmDiscardGroupDraft(()=>{groupMemberDraft=null;$('groupEditor').close()});return}
  groupMemberDraft=null;
});
function startGroupMemberDraft(){
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||group.readonly||group.filter||groupSaving)return;
  beginGroupMemberDraft(group);groupShowAllPreferred=true;groupActivePanel='members';
  $('groupMembers').classList.add('active');$('groupControls').classList.remove('active');renderGroupRows();
}
$('groupEditToggle').onclick=startGroupMemberDraft;
async function applyGroupMemberDraft(){
  const group=userGroups.find(g=>g.id===selectedGroup),draft=groupMemberDraft;
  if(!group||!draft||draft.groupId!==group.id||groupSaving)return;
  groupSaving=true;$('groupError').textContent='';$('groupMemberApply').disabled=true;$('groupMemberCancel').disabled=true;
  try{
    await groupRequest('/api/user-groups/membership','PUT',{group:group.id,members:draft.order.filter(id=>draft.members.has(id))});
    userGroups=await groupRequest('/api/user-groups','GET');groupMemberDraft=null;groupShowAllPreferred=false;
    refreshLivePicker();if(group.id===pickerGroup&&$('q').value.trim())doSearch();renderGroupOptions();renderGroupRows();
  }catch(error){$('groupError').textContent=error.message}
  finally{groupSaving=false;renderGroupRows()}
}
function closeGroupMemberDraft(){
  if(!groupMemberDraft||groupSaving)return;
  const discard=()=>{groupMemberDraft=null;groupShowAllPreferred=false;groupActivePanel=null;$('groupMembers').classList.remove('active');renderGroupRows()};
  if(groupMemberDraftDirty())confirmDiscardGroupDraft(discard);else discard();
}
$('groupMemberApply').onclick=applyGroupMemberDraft;
$('groupMemberCancel').onclick=closeGroupMemberDraft;
$('discardGroupDraftGo').onclick=()=>$('discardGroupDraftDialog').close('discard');
$('discardGroupDraftKeep').onclick=()=>$('discardGroupDraftDialog').close('keep');
$('discardGroupDraftDialog').addEventListener('cancel',event=>{event.preventDefault();$('discardGroupDraftDialog').close('keep')});
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
  const members=groupMemberDraft?.groupId===group.id?groupMemberDraft.members:new Set(group.members);
  const ids=orderedDicts().filter(d=>!members.has(d.id)&&availableMatchesFilter(d)&&availableMatchesSearch(d)).map(d=>d.id);
  if(!ids.length)return;
  if(groupMemberDraft?.groupId===group.id){for(const id of ids){groupMemberDraft.members.add(id);groupMemberDraft.order.push(id)}renderGroupRows();return}
  groupSaving=true;$('groupError').textContent='';
  $('closeGroups').disabled=true;$('groupSelect').disabled=true;$('groupAddAll').disabled=true;
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
  groupSaving=true;$("groupError").textContent="";
  $('groupLink').disabled=true;$('groupLinkFilter').disabled=true;$('groupLinkFilterHost').classList.remove('can-link');
  try{
    await groupRequest("/api/user-groups/link","PUT",{group:group.id,filter});
    userGroups=await groupRequest("/api/user-groups","GET");
    if(!filter)groupShowAllPreferred=false;
    renderGroupOptions();renderGroupRows();refreshLivePicker();
    if(group.id===pickerGroup&&$("q").value.trim())doSearch();
  }catch(error){$("groupError").textContent=error.message;renderGroupLink(group)}
  finally{groupSaving=false;renderGroupRows()}
}
async function beginGroupLinkSelection(){
  const group=userGroups.find(g=>g.id===selectedGroup);
  if(!group||group.readonly||group.filter||groupSaving)return;
  renderGroupLink(group);
  if((group.order?.length||group.members.length)&&!await confirmGroupLinkReplacement())return;
  if(groupSaving||selectedGroup!==group.id)return;
  const remembered=group.selectedFilter;
  if(remembered&&groupFilters().some(([key])=>key===JSON.stringify([remembered.facet,remembered.value]))){
    saveGroupLink(remembered);return;
  }
  openGroupFilterDialog('link');
}
$("groupLink").onchange=()=>{
  if(groupEditorMode){
    if($('groupLink').checked){
      if(groupDraftFilter)groupDraftLinked=true;
      else{groupDraftChooseForLink=true;$('groupLink').checked=false;openGroupFilterDialog('draft')}
    }else groupDraftLinked=false;
    renderGroupRows();return;
  }
  if($("groupLink").checked)beginGroupLinkSelection();
  else saveGroupLink(null);
};
$('groupLinkFilterHost').onclick=()=>{
  if(!$('groupLinkFilterHost').classList.contains('can-link'))return;
  $('groupLink').checked=true;$('groupLink').dispatchEvent(new Event('change',{bubbles:true}));
};
$("groupLinkFilter").onclick=()=>{
  if($('groupLinkFilter').disabled)return;
  openGroupFilterDialog(groupEditorMode?'draft':'link');
};
$("newGroupFromFilter").onchange=()=>{
  const on=$("newGroupFromFilter").checked;
  if(on)openGroupFilterDialog('new');
  else{newGroupSelectedFilter=null;$("newGroupFilter").disabled=true;$("newGroupFilter").textContent=tx('dictUI.chooseFilter');$("newGroupLink").checked=false;$("newGroupLink").disabled=true}
};
$("newGroupFilter").onclick=()=>openGroupFilterDialog('new');
function beginGroupNameEdit(mode){
  const group=userGroups.find(item=>item.id===selectedGroup);
  if(mode==='rename'&&(!group||group.readonly||groupSaving))return;
  groupSavedEditPreference=groupShowAllPreferred;groupShowAllPreferred=true;groupEditorMode=mode;
  if(mode==='new'){
    groupDraftMembers=new Set();groupDraftFilter=null;groupDraftLinked=false;
    $('groupNameInline').value='';$('groupLink').checked=false;$('groupLinkFilter').textContent=tx('dictUI.chooseFilter');
  }else{
    groupDraftFilter=group.filter||group.selectedFilter||null;groupDraftLinked=!!group.filter;
    $('groupNameInline').value=group.name;
  }
  for(const id of ['groupMembersSearch','groupAvailableSearch']){$(id).value='';$(id).dispatchEvent(new Event('input',{bubbles:true}))}
  groupActivePanel='controls';$('groupControls').classList.add('active');$('groupMembers').classList.remove('active');$('groupAvailable').classList.remove('active');
  $('groupError').textContent='';renderGroupRows();$('groupNameInline').focus();requestAnimationFrame(updateGroupSplit);
}
function cancelGroupNameEdit(){
  if(!groupEditorMode||groupSaving)return;
  groupEditorMode='';groupShowAllPreferred=groupSavedEditPreference;groupDraftMembers.clear();groupDraftFilter=null;groupDraftLinked=false;groupDraftChooseForLink=false;
  groupActivePanel=null;$('groupControls').classList.remove('active');$('groupMembers').classList.remove('active');$('groupAvailable').classList.remove('active');
  $('groupNameInline').value='';$('groupError').textContent='';renderGroupRows();$('groupSelect').focus();
}
async function saveGroupNameEdit(){
  if(!groupEditorMode||groupSaving)return;
  if(groupEditorMode==='rename'){
    const group=userGroups.find(item=>item.id===selectedGroup);
    if(groupDraftLinked&&!group?.filter&&(group?.order?.length||group?.members?.length)&&!await confirmGroupLinkReplacement())return;
  }
  groupSaving=true;$('groupError').textContent='';$('groupNameApply').disabled=true;$('groupNameCancel').disabled=true;$('closeGroups').disabled=true;
  try{
    if(groupEditorMode==='new'){
      const filter=groupDraftFilter?{facet:groupDraftFilter.facet,value:groupDraftFilter.value}:null;
      const body={name:$('groupNameInline').value,filter,linked:!!filter&&groupDraftLinked};
      if(!groupDraftLinked)body.members=[...groupDraftMembers];
      const group=await groupRequest('/api/user-groups','POST',body);selectedGroup=group.id;
    }else await groupRequest('/api/user-groups/name','PUT',{group:selectedGroup,name:$('groupNameInline').value,filter:groupDraftFilter,linked:groupDraftLinked});
    userGroups=await groupRequest('/api/user-groups','GET');
    groupEditorMode='';groupDraftMembers.clear();groupDraftFilter=null;groupDraftLinked=false;groupShowAllPreferred=false;
    groupActivePanel=null;$('groupControls').classList.remove('active');$('groupMembers').classList.remove('active');$('groupAvailable').classList.remove('active');
    renderGroupOptions();renderGroupRows();refreshLivePicker();
    if(selectedGroup===pickerGroup&&$('q').value.trim())doSearch();
  }catch(error){$('groupError').textContent=error.message}
  finally{groupSaving=false;$('groupNameApply').disabled=false;$('groupNameCancel').disabled=false;$('closeGroups').disabled=false;renderGroupRows()}
}
$('groupNewAction').onclick=()=>beginGroupNameEdit('new');
$('groupNameApply').onclick=saveGroupNameEdit;
$('groupRenameAction').onclick=()=>beginGroupNameEdit('rename');
$('groupNameCancel').onclick=cancelGroupNameEdit;
$('groupNameInline').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();saveGroupNameEdit()}else if(event.key==='Escape'){event.preventDefault();cancelGroupNameEdit()}});
$('groupSelect').onchange=()=>{
  const next=$('groupSelect').value;
  if(groupMemberDraftDirty()){const previous=selectedGroup;$('groupSelect').value=previous;confirmDiscardGroupDraft(()=>{groupMemberDraft=null;selectedGroup=next;$('groupSelect').value=next;groupSelectChanged()});return}
  groupMemberDraft=null;selectedGroup=next;groupSelectChanged();
};
function groupSelectChanged(){
  cancelGroupOrderLongPress();groupOrderSelection=null;groupOrderSuppressClick=false;
  groupShowAllPreferred=false;groupActivePanel=null;
  $('groupControls').classList.remove('active');$('groupMembers').classList.remove('active');$('groupAvailable').classList.remove('active');
  $('groupAvailableSearch').value='';$('groupMembersSearch').value='';$('groupError').textContent='';renderGroupRows();
}// Promise.all resolves to an array: passing it to loadDicts would enable
// rescan and wait for index maintenance instead of listing dictionaries.
Promise.all([loadPrefs(),loadConfig(),loadUserCSS(),loadPickerGroups()])
  .then(()=>loadDicts()).then(ok=>{if(ok)applyURL()}).catch(error=>bootFailed(error.message));
// The preset layers are independent of the boot chain: their app halves are
// server-injected <link>s and their article halves join the article sheet
// whenever this answer lands, which is why this is fire-and-forget rather
// than a gate on the first search.
presetsLoad();
