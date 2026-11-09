/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

// Find in article results with a server-owned cursor; the client displays targets.
(function () {
"use strict";
const $ = id => document.getElementById(id), t = window.wudictI18n.t;
const dialog = $("articleFindDialog"), strip = $("articleFindStrip");
const wordChar = /[\p{L}\p{M}\p{N}_]/u;
const findHistoryKey = "wudict_article_find_history_v1";
const skip = "script,style,noscript,template,button,input,select,textarea,audio,video,svg";
const block = "p,div,li,dt,dd,td,th,h1,h2,h3,h4,h5,h6,blockquote,pre,.wu-m";
let options = {query:"", mode:"contains", matchCase:false, all:true, examples:false};
try { Object.assign(options, JSON.parse(localStorage.getItem("wudict_article_find") || "{}")); } catch (_) {}
delete options.blocks; // Retire the former loaded-block-only preference.
function findHistoryLimit(){
  try { const raw=localStorage.getItem("wudict_history_length"); if(raw===null)return 20; const n=Number(raw); return Number.isInteger(n)&&n>=0&&n<=1000?n:20; }
  catch (_) { return 20; }
}
function loadFindHistory(){
  try {
    const rows=JSON.parse(localStorage.getItem(findHistoryKey)||"[]"),seen=new Set(),out=[];
    if(Array.isArray(rows))for(const value of rows){
      if(typeof value!=="string"||!value.trim())continue;
      const query=value.trim(),key=query.normalize("NFC").toLocaleLowerCase();
      if(seen.has(key))continue;
      seen.add(key);out.push(query);
      if(out.length>=1000)break;
    }
    return out.slice(0,findHistoryLimit());
  } catch (_) { return []; }
}
let findHistory=loadFindHistory(),suppressFindHistoryFocus=false;
let matches = [], index = -1, active = false, restored = [], docs = new Set(), timer, fullTextNavigation = null, fullTextGeneration = null, searchAnchor = null;
let remoteFind = null, remoteFindToken = 0, remoteFindRequestSeq = 0, remoteFindPending = false;
let settle = null;
const fallbackSelections = new Map();
const highlightCSS = "::highlight(wudict-find-all){background:#ffe082;color:#171717}::highlight(wudict-find-current){background:#ff9800;color:#171717;text-decoration:underline}";

function restoreExamples() {
  for (const [el, value, priority] of restored) {
    value ? el.style.setProperty("display", value, priority) : el.style.removeProperty("display");
  }
  restored = [];
}
function clearPaint() {
  for (const doc of docs) {
    doc.defaultView?.CSS?.highlights?.delete("wudict-find-all");
    doc.defaultView?.CSS?.highlights?.delete("wudict-find-current");
  }
  for (const [doc, range] of fallbackSelections) {
    const selection = doc.getSelection();
    if (selection?.rangeCount === 1 && selection.getRangeAt(0) === range) selection.removeAllRanges();
  }
  fallbackSelections.clear();
}
function rootFor(host) {
  try { return host.shadowRoot || host.contentDocument?.body; } catch (_) { return null; }
}
function save() {
  try { localStorage.setItem("wudict_article_find", JSON.stringify(options)); } catch (_) {}
}
function saveFindHistory(){
  try { localStorage.setItem(findHistoryKey,JSON.stringify(findHistory.slice(0,findHistoryLimit()))); } catch (_) {}
}
function recordFindHistory(query=options.query){
  const value=String(query||"").trim(),max=findHistoryLimit();
  if(!value||!max)return;
  const key=value.normalize("NFC").toLocaleLowerCase();
  findHistory=[value,...findHistory.filter(row=>row.normalize("NFC").toLocaleLowerCase()!==key)].slice(0,max);
  saveFindHistory();
}
function hideFindHistory(){
  const list=$("articleFindHistory");list.hidden=true;list.replaceChildren();
  $("articleFindQuery").setAttribute("aria-expanded","false");
}
function sizeFindHistory(){
  const list=$("articleFindHistory"),viewport=window.visualViewport;
  const viewportBottom=viewport?viewport.offsetTop+viewport.height:document.documentElement.clientHeight;
  const popupTop=$("articleFindQueryBox").getBoundingClientRect().bottom+4;
  list.style.maxHeight=Math.max(0,viewportBottom-popupTop-4)+"px";
}
function renderFindHistory(filter=$("articleFindQuery").value){
  const list=$("articleFindHistory"),query=String(filter||"").trim().normalize("NFC").toLocaleLowerCase();
  list.replaceChildren();
  if(!dialog.open||$("articleFindMode").value==="fts"||!findHistoryLimit()){hideFindHistory();return}
  const rows=findHistory.slice(0,findHistoryLimit()).filter(value=>!query||value.normalize("NFC").toLocaleLowerCase().includes(query));
  for(const value of rows){
    const button=document.createElement("button");button.type="button";button.setAttribute("role","option");button.textContent=value;
    button.addEventListener("click",()=>{
      const input=$("articleFindQuery");input.value=value;hideFindHistory();readOptions();index=-1;active=!!options.query;
      rebuild();recordFindHistory(value);suppressFindHistoryFocus=true;input.focus();suppressFindHistoryFocus=false;hideFindHistory();
    });
    list.appendChild(button);
  }
  sizeFindHistory();list.hidden=!rows.length;
  $("articleFindQuery").setAttribute("aria-expanded",String(!list.hidden));
}
function readOptions() {
  options = {query:$("articleFindQuery").value.trim(), mode:$("articleFindMode").value,
    matchCase:$("articleFindCase").checked, all:$("articleFindAll").checked,
    examples:$("articleFindExamples").checked};
  save();
}
function fillOptions() {
  $("articleFindQuery").value = options.query;
  // The article-text search is one of two things: a word anywhere (as a
  // substring or, with the checkbox, as whole tokens) or the live full-text
  // answer. Older saved modes collapse to the word reading.
  options.mode = options.mode === "word" ? "word" : options.mode === "fts" && fullTextNavigation ? "fts" : "contains";
  $("articleFindMode").value = options.mode;
  $("articleFindWholeWords").checked = options.mode === "word";
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
}
function open(preserveAnchor=false) {
  if (!preserveAnchor) searchAnchor = null;
  /* The strip is the live answer, and the dialog opens on the reading it shows:
     [A B] and [A ↔ B] are the dialog's own phrase and near readings, and the
     words reading is the walk itself. Filling the query but not starting a new
     search - the reader asked to see the mode, not to run it again. */
  let adopted = false;
  if (fullTextNavigation && !preserveAnchor) {
    const reading = fullTextNavigation.reading;
    const mode = reading === "phrase" || reading === "near" ? reading : "fts";
    options = {...options, mode, query: fullTextNavigation.query || options.query};
    adopted = true;
  }
  fillOptions();
  syncControls();
  active = options.mode !== "fts" && !!options.query;
  if (active && !searchAnchor && !adopted) rebuild();
  dialog.showModal();
  strip.hidden = true; measure();
  if (options.mode === "fts") $("articleFindMode").focus();
  else { $("articleFindQuery").focus(); $("articleFindQuery").select(); }
}
function syncControls() {
  syncRemoteFindTraceButton();
  const select = $("articleFindMode"), hasFTS = !!fullTextNavigation;
  const ftsOption=select.querySelector('option[value="fts"]');
  ftsOption.disabled=!hasFTS;ftsOption.hidden=!hasFTS;
  let fts = select.value === "fts";
  if(fts)hideFindHistory();
  if(fts&&!hasFTS){
    options={...options,mode:"contains"};select.value="contains";fts=false;save();
    if(dialog.open){active=!!options.query;if(active)rebuild()}
  }
  $("articleFindStop").disabled = fts && hasFTS;
  $("articleFindQuery").disabled = fts;
  for (const id of ["articleFindCase","articleFindAll","articleFindExamples"]) $(id).disabled = fts;
  if (fts) {
    if (fullTextNavigation) {
      options.query = fullTextNavigation.query || "";
      $("articleFindQuery").value = options.query;
    }
    const position = fullTextNavigation?.position || t("articleFind.noMatches");
    $("articleFindStatus").textContent = position;
    $("articleFindCount").textContent = position;
    $("articleFindCount").disabled = !(Number(fullTextNavigation?.total)>0);
    $("articleFindDialogPrev").disabled = !fullTextNavigation?.canPrevious;
    $("articleFindDialogNext").disabled = !fullTextNavigation?.canNext;
  } else if(remoteFind){
    const state=remoteFind.state,position=serverFindPosition();
    const display=remoteFind.error?"⚠ "+t("search.fullListUnavailable"):position;
    $("articleFindStatus").textContent=display;
    $("articleFindCount").textContent=display;
    $("articleFindCount").disabled=!state?.found;
    $("articleFindDialogPrev").disabled=remoteFindPending||!serverFindCanStep();
    $("articleFindDialogNext").disabled=remoteFindPending||!serverFindCanStep();
  } else {
    $("articleFindDialogPrev").disabled = !canStep(-1);
    $("articleFindDialogNext").disabled = !canStep(1);
  }
  $("articleFindScrollToMatch").disabled = fts ? !fullTextNavigation?.canRestore :
    remoteFind ? remoteFindPending || !remoteFind.target : !matches.length || index < 0;
  window.wudictThemedSelects?.articleFindMode?.sync();
}
function canStep(step){
  if(options.mode==="fts")return !!(step<0?fullTextNavigation?.canPrevious:fullTextNavigation?.canNext);
  if(remoteFind)return !remoteFindPending&&serverFindCanStep();
  return false;
}
/* The window near counts with is the reader's setting (Settings → Results, 1…50,
   ten by default), and the page mirrors the resolved number for anything outside
   its own scope as window.wudictNearWords. That mirror is what this reads: the
   hint under the mode states the same number, but it is a plural TEMPLATE whose
   {number} the page fills in, so reading the sentence back is only the fallback
   for a page that mirrors nothing (the check stand) - and one word apart is the
   last resort there, not a promise of anywhere. */
function nearWords(){
  const chosen=Number(window.wudictNearWords);
  if(Number.isFinite(chosen)&&chosen>0)return chosen;
  const quoted=String(t("search.modeHintNear")).match(/\d+/u);
  return quoted?Number(quoted[0]):1;
}
function ensureFindStyle(root) {
  if (!root || root.querySelector("style[data-wudict-find]")) return;
  const doc = root.ownerDocument || root;
  const style = doc.createElement("style"); style.dataset.wudictFind = "";
  style.textContent = highlightCSS; root.appendChild(style);
}
function paint() {
  syncRemoteFindTraceButton();
  clearPaint();
  // The ranges are painted by CSS Custom Highlight, whose rules must live in the
  // tree that carries the text: a shadow root, or a frame's body. The old client
  // scan injected them; it is gone, so the one live painter does it here.
  for (const hit of matches) ensureFindStyle(rootFor(hit.host));
  for (const doc of docs) {
    const win = doc.defaultView;
    if (!win?.CSS?.highlights || !win.Highlight) continue;
    const all = new win.Highlight(), current = new win.Highlight();
    if (options.all) for (const hit of matches) if (hit.range.startContainer.ownerDocument === doc) all.add(hit.range);
    if (index >= 0 && matches[index]?.range.startContainer.ownerDocument === doc) current.add(matches[index].range);
    win.CSS.highlights.set("wudict-find-all", all);
    win.CSS.highlights.set("wudict-find-current", current);
  }
  if(remoteFind&&options.mode!=="fts"){
    const display=remoteFind.error?"⚠ "+t("search.fullListUnavailable"):serverFindPosition();
    $("articleFindCount").textContent=display;
    $("articleFindCount").disabled=!remoteFind.state?.found;
    $("articleFindStatus").textContent=display;
    $("articleFindLabel").textContent=options.query;
    $("articleFindLabel").title=options.query;
    $("articleFindPrev").disabled=remoteFindPending||!serverFindCanStep();
    $("articleFindNext").disabled=remoteFindPending||!serverFindCanStep();
    $("articleFindDialogPrev").disabled=remoteFindPending||!serverFindCanStep();
    $("articleFindDialogNext").disabled=remoteFindPending||!serverFindCanStep();
    $("articleFindScrollToMatch").disabled=remoteFindPending||!remoteFind.target;
    return;
  }
}
function closeRemoteFind(){
  remoteFindToken++;remoteFindRequestSeq++;remoteFindPending=false;
  const previous=remoteFind;remoteFind=null;
  if(previous?.id)apiFetch("/api/search?"+new URLSearchParams({cacheAction:"navClose",cacheID:previous.id})).catch(()=>{});
}
function remoteFindID(){
  const bytes=new Uint8Array(16);
  try{crypto.getRandomValues(bytes)}catch(_){for(let i=0;i<bytes.length;i++)bytes[i]=Math.floor(Math.random()*256)}
  return [...bytes].map(value=>value.toString(16).padStart(2,"0")).join("");
}
let remoteFindDiagnosticRun=0;
let remoteFindTraceRun=0,remoteFindTraceActive=false,remoteFindTraceStop=false,remoteFindDiagnosticTail=Promise.resolve();
function remoteFindTraceEnabled(){return searchArticleDiagnosticsEnabled()||remoteFindTraceActive}
function syncRemoteFindTraceButton(){
  const button=$("articleFindTrace");
  if(!button)return;
  const fullText=options.mode==="fts"&&!!fullTextNavigation;
  const visible=fullText||!!remoteFind?.id&&options.mode!=="fts";
  button.hidden=false;
  button.textContent=remoteFindTraceActive?t("articleFind.traceStop"):t("articleFind.traceStart");
  button.setAttribute("aria-label",button.textContent);
  button.title=searchArticleDiagnosticsEnabled()?button.textContent:`${button.textContent} — ${t("articleFind.traceLogDisabled")}`;
  button.disabled=!visible||!remoteFindTraceActive&&(fullText?!fullTextNavigation?.canNext:!remoteFind?.state?.found||remoteFindPending);
}
function remoteFindDiagnostic(record,stage,fields={}){
  if(!remoteFindTraceEnabled())return Promise.resolve(false);
  const payload={event:"search_article",stage,runId:record?.diagnosticRun||remoteFindDiagnosticRun,
    query:String(options.query||"").replace(/[\r\n]/g," ").slice(0,1024),mode:options.mode,
    elapsedMs:Math.max(0,Math.round(performance.now()-(record?.diagnosticStarted||performance.now()))),...fields};
  remoteFindDiagnosticTail=remoteFindDiagnosticTail.then(()=>sendBrowseDiagnostic(payload)).catch(()=>false);
  return remoteFindDiagnosticTail;
}
function remoteFindContext(){
  const source=window.wudictArticleFindContext?.()||{};
  const details=[...$("out").querySelectorAll("details.dict[data-search-dict]")];
  const dictionaries=source.dictionaries?.length?source.dictionaries:details.map(det=>det.dataset.searchDict).filter(Boolean);
  const first=details[0];
  const orders=source.orders?.length?source.orders:details.map((_,index)=>index);
  return {dictionaries,orders,cacheCount:Number(source.cacheCount)||dictionaries.length,
    query:source.query??first?.dataset.searchQuery??"",mode:source.mode??first?.dataset.searchMode??"",
    ignore:!!source.ignoreDiacritics,firstSize:Number(first?.dataset.searchInitial)||5,
    pageSize:Number(first?.dataset.searchPageSize)||20,groupID:source.groupID||"",
    rungs:source.rungs?.length?source.rungs:details.map(det=>det.dataset.searchRung||"")};
}
function remoteFindSignature(context){
  return JSON.stringify([context.groupID,context.dictionaries,context.orders,context.cacheCount,context.query,context.mode,options.query,options.mode,
    context.ignore,context.rungs,options.matchCase,options.examples,nearWords()]);
}
function remoteFindURL(record,action,extra={}){
  return "/api/search?"+new URLSearchParams({cacheAction:action,cacheID:record.id,origin:location.origin,...extra});
}
async function remoteFindFetch(record,action,extra={}){
	const started=performance.now();
	const requestMeta={action,fromPosition:Number(record.state?.position)||0,fromTotal:Number(record.state?.total)||0,
    fromDict:record.state?.target?.dict||"",fromBlock:Number(record.state?.target?.blockStart)||0};
	const requestLogStarted=performance.now();
	await remoteFindDiagnostic(record,"navigation_request",{detail:`action=${action}; from=${requestMeta.fromPosition}/${requestMeta.fromTotal}; dict=${requestMeta.fromDict}; block=${requestMeta.fromBlock}; trace=${record.traceRun||0}`});
	const requestLogMs=Math.round(performance.now()-requestLogStarted);
	let response;
	try{response=await apiFetch(remoteFindURL(record,action,extra))}
	catch(error){await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),detail:`action=${action}; transport_error=${String(error).slice(0,700)}`});throw error}
  if(!response.ok){
    const error=await response.text();
    await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),detail:`action=${action}; status=${response.status}; error=${String(error).slice(0,700)}`});
    throw new Error(error||"article search failed");
  }
  const state=await response.json();
  await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),completed:Math.min(100000,state.total||0),
    detail:`action=${action}; found=${!!state.found}; complete=${!!state.complete}; position=${state.position||0}/${state.total||0}; dict=${state.target?.dict||""}; article=${state.target?.articleIndex??""}; block=${state.target?.blockStart??""}; occurrences=${state.target?.occurrences?.length||0}; diagnostic_request_ms=${requestLogMs}`});
  return state;
}
async function startRemoteFind(){
  if(options.mode==="fts")return;
  if(!options.query){closeRemoteFind();matches=[];index=-1;docs.clear();restoreExamples();clearPaint();syncControls();return}
  const context=remoteFindContext(),anchor=serverFindAnchor(),signature=remoteFindSignature(context)+JSON.stringify(anchor);
  if(remoteFind?.signature===signature){paint();return}
  closeRemoteFind();
  if(!context.dictionaries.length||!context.query||!context.mode){remoteFind={signature,state:{complete:true,total:0,found:false}};paint();return}
  const record={id:remoteFindID(),signature,state:null,pending:true,error:"",diagnosticRun:++remoteFindDiagnosticRun,diagnosticStarted:performance.now()};
  remoteFind=record;remoteFindPending=true;
  const token=++remoteFindToken;
  matches=[];index=-1;docs.clear();restoreExamples();clearPaint();paint();
  try{
    const size=Math.max(1,Math.min(500,context.pageSize)),firstSize=Math.max(1,Math.min(500,context.firstSize));
    const params={dict:context.dictionaries.join(","),cacheCount:String(context.cacheCount),
      cacheOrders:context.orders.join(","),q:options.query,mode:options.mode,
      resultQuery:context.query,resultMode:context.mode,ignoreDiacritics:context.ignore?"1":"0",
      matchCase:options.matchCase?"1":"0",examples:options.examples?"1":"0",nearWords:String(nearWords()),
      pageSize:String(size),firstSize:String(firstSize),prime:"0",groupID:context.groupID,rungs:context.rungs.join(",")};
    if(anchor){params.anchorDict=anchor.dict;params.anchorArticle=String(anchor.article);params.anchorText=anchor.text;
      params.anchorDirection=String(anchor.direction)}
    searchAnchor=null;
    const state=await remoteFindFetch(record,"navStart",params);
    if(token!==remoteFindToken||remoteFind!==record)return;
    record.state=state;record.pendingTarget=anchor&&state.found?state:null;record.anchorDirection=anchor?.direction||0;
    record.pending=false;record.error="";
  }catch(error){
    if(token!==remoteFindToken||remoteFind!==record)return;
    record.state={complete:true,total:0,found:false};record.pending=false;record.error=String(error?.message||error);
  }finally{
    if(token===remoteFindToken&&remoteFind===record){remoteFindPending=false;paint()}
  }
}
function serverFindCanStep(){
  const state=remoteFind?.state;
  return !!(state&&(!state.complete||Number(state.total)>1));
}
function serverFindPosition(){
  const state=remoteFind?.state;
  if(!state?.found)return state?.complete?t("articleFind.noMatches"):"";
  const total=Number(state.total)||0;
  return t("articleFind.position",{current:Number(state.position)||1,total:String(total)+(state.complete?"":"+")});
}
/* The server decided WHAT matched and WHERE, as a path to a text node plus
   offsets inside it (articleFindNodeSpan). The page resolves that path and sets
   a Range - it keeps no model of the article's text, because two models of the
   same article drift, and a drifted one paints the wrong words. Split() no
   longer decides anything here.

   Two guards, in order: the path must land on a text node whose text at those
   offsets is the match's own text, and when it does not - a frame's parser can
   hoist <link>/<style>/<meta> into a head the server never had, which shifts
   top-level child indices - the match is LOCATED by its text instead, walking
   the article's text nodes in order behind a cursor, so repeated words still
   land in the right places. A match neither way is skipped and reported: the
   reader sees fewer marks, never wrong ones. */
function foldFindText(value){
  const collapsed=String(value??"").replace(/\s+/gu," ").trim();
  return options.matchCase?collapsed:collapsed.toLocaleLowerCase();
}
function findTextNodes(root){
  const doc=root.ownerDocument||root,win=doc.defaultView,nodes=[];
  if(!win?.NodeFilter)return nodes;
  const walker=doc.createTreeWalker(root,win.NodeFilter.SHOW_TEXT);
  while(walker.nextNode()){
    const node=walker.currentNode,parent=node.parentElement;
    if(!node.data||!parent)continue;
    if(parent.closest("script,style,noscript,template,title"))continue;
    const css=win.getComputedStyle(parent);
    if(css.display==="none"||css.visibility==="hidden"||css.visibility==="collapse")continue;
    nodes.push(node);
  }
  return nodes;
}
function resolveNodePath(root,path){
  let node=root;
  for(const index of path||[]){
    node=node?.childNodes?.[index];
    if(!node)return null;
  }
  return node&&node.nodeType===3?node:null;
}
function rangeFromNodes(doc,first,firstStart,last,lastEnd){
  const range=doc.createRange();
  try{range.setStart(first,firstStart);range.setEnd(last,lastEnd)}catch(_){return null}
  return range;
}
// locateFindText finds one occurrence's text among the article's text nodes,
// starting at the node cursor: inside a single node first (the common case), then
// across a run of consecutive ones (a phrase the markup split).
function locateFindText(nodes,cursor,occurrence,doc){
  const want=foldFindText(occurrence.text);
  if(!want)return {range:null,cursor};
  for(let i=cursor;i<nodes.length;i++){
    const folded=foldFindText(nodes[i].data),at=folded.indexOf(want);
    if(at<0)continue;
    const range=rangeFromNodes(doc,nodes[i],at,nodes[i],at+want.length);
    if(range)return {range,cursor:i};
  }
  for(let i=cursor;i<nodes.length;i++){
    let joined="",spans=[];
    for(let j=i;j<nodes.length&&j<i+8;j++){
      const text=foldFindText(nodes[j].data);
      const at=joined.length?joined.length+1:0;
      joined=joined?joined+" "+text:text;
      spans.push({node:nodes[j],from:at,to:at+text.length});
      if(joined.length<want.length)continue;
      const found=joined.indexOf(want);
      if(found<0)continue;
      const end=found+want.length;
      const first=spans.find(span=>span.to>found),last=spans.find(span=>span.to>=end);
      if(!first||!last)continue;
      const range=rangeFromNodes(doc,first.node,found-first.from,last.node,end-last.from);
      if(range)return {range,cursor:j};
    }
  }
  return {range:null,cursor};
}
function serverFindRanges(root,occurrences){
  const doc=root.ownerDocument||root,win=doc.defaultView;
  if(!win)return [];
  const nodes=findTextNodes(root),out=[];
  let cursor=0,missed=0;
  for(const occurrence of occurrences||[]){
    const spans=occurrence?.spans||[];
    if(!spans.length){missed++;continue}
    const first=resolveNodePath(root,spans[0].path),last=resolveNodePath(root,spans[spans.length-1].path);
    let range=first&&last?rangeFromNodes(doc,first,spans[0].start,last,spans[spans.length-1].end):null;
    if(range&&foldFindText(range.toString())!==foldFindText(occurrence.text))range=null;
    if(!range){
      const located=locateFindText(nodes,cursor,occurrence,doc);
      range=located.range;cursor=located.cursor;
    }
    if(!range){missed++;continue}
    for(let i=cursor;i<nodes.length;i++){if(nodes[i]===range.startContainer||range.intersectsNode?.(nodes[i])){cursor=i;break}}
    out.push({range,example:range.startContainer.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")||null});
  }
  if(missed)window.wudictArticleFind?.traceEvent?.("navigation_trace_ranges",`occurrences=${(occurrences||[]).length}; painted=${out.length}; missed=${missed}`);
  return out;
}
/* The reader's own position, named the way the page CAN name it: a snippet of
   the article's text taken from the selection. The server finds that snippet in
   its own model of the same article, so the gesture does not depend on the two
   trees having the same node indices - a frame's parser moves <link>/<style>
   into a head the server never had, and every top-level index shifts with them.
   The snippet is folded the way the server folds it, so whitespace cannot part
   the two. */
function serverFindAnchor(){
  if(!searchAnchor||options.mode==="fts")return null;
  const host=searchAnchor.host,det=host?.closest("details.dict"),row=host?.closest("dd");
  if(!det||!row)return null;
  const rows=[...det.querySelectorAll(":scope>dl>dd")],articleOffset=rows.indexOf(row);
  if(articleOffset<0)return null;
  const range=searchAnchor.range;
  let node=range.startContainer;
  while(node&&node.nodeType!==3)node=node.firstChild;
  if(!node)return null;
  const text=foldFindText(node.data.slice(range.startOffset)).slice(0,64);
  if(!text)return null;
  return {dict:det.dataset.searchDict||"",article:(Number(det.dataset.searchPageStart)||0)+articleOffset,
    text,direction:searchAnchor.direction};
}
async function showRemoteFindTarget(target,state,token){
  if(!target)return false;
  let det=null;
  for(let attempt=0;attempt<50;attempt++){
    det=[...$("out").querySelectorAll("details.dict")].find(node=>node.dataset.searchDict===target.dict);
    if(det)break;
    if(token!==remoteFindToken)return false;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  if(!det)return false;
  const blockStart=Number(target.blockStart)||0;
  if((Number(det.dataset.searchPageStart)||0)!==blockStart){
    const replacement=await window.wudictRestoreArticleBlock?.(det,blockStart);
    if(!replacement||token!==remoteFindToken)return false;
    det=replacement;
  }
  det.open=true;
  const row=det.querySelectorAll(":scope>dl>dd")[Number(target.articleIndex)-blockStart];
  const host=row?.querySelector(".article");
  if(!host)return false;
  let found=[];
  for(let attempt=0;attempt<50;attempt++){
    const root=rootFor(host);
    if(root)found=serverFindRanges(root,target.occurrences);
    if(found.length||token!==remoteFindToken)break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  if(!found.length||token!==remoteFindToken)return false;
  matches=found.map(item=>({host,range:item.range,example:item.example}));
  docs=new Set(matches.map(item=>item.range.startContainer.ownerDocument));
  index=Math.max(0,Math.min(Number(target.matchIndex)||0,matches.length-1));
  remoteFind.state=state;remoteFind.target=target;
  dialog.close();strip.hidden=false;measure();
  restoreExamples();showExample(matches[index]);paint();
  settleOn(matches[index],det);
  return true;
}
async function moveRemoteFind(step){
  if(!remoteFind?.id||remoteFindPending)return false;
  remoteFindPending=true;const record=remoteFind,token=remoteFindToken,request=++remoteFindRequestSeq;
  try{
    if(record.pendingTarget){
      const target=record.pendingTarget;record.pendingTarget=null;
      if(step===record.anchorDirection){
        const paintStarted=performance.now();
        record.error=await showRemoteFindTarget(target.target,target,token)?"":t("search.fullListUnavailable");
        await remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${target.target?.dict||""}; article=${target.target?.articleIndex??""}; block=${target.target?.blockStart??""}; position=${target.position||0}/${target.total||0}; success=${!record.error}`});
        return !record.error;
      }
    }
    const state=await remoteFindFetch(record,step<0?"navPrevious":"navNext");
    if(request!==remoteFindRequestSeq||token!==remoteFindToken||remoteFind!==record)return;
    if(state.found){record.state=state;const paintStarted=performance.now();record.error=await showRemoteFindTarget(state.target,state,token)?"":t("search.fullListUnavailable");
      await remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${state.target?.dict||""}; article=${state.target?.articleIndex??""}; block=${state.target?.blockStart??""}; position=${state.position||0}/${state.total||0}; success=${!record.error}`})}
    else record.state={...record.state,...state};
    return !!state.found&&!record.error;
  }catch(error){record.error=String(error?.message||error);await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`action=move; error=${record.error.slice(0,700)}`});return false}
  finally{if(request===remoteFindRequestSeq&&token===remoteFindToken){remoteFindPending=false;paint()}}
}
async function runRemoteFindTrace(){
  const record=remoteFind;
  if(!record?.id||remoteFindPending||remoteFindTraceActive||!record.state?.found)return;
  remoteFindTraceActive=true;remoteFindTraceStop=false;record.traceRun=++remoteFindTraceRun;
  const started=performance.now(),startPosition=Number(record.state.position)||1;
  let steps=0,reason="end",previousPosition=startPosition;
  syncRemoteFindTraceButton();
  await remoteFindDiagnostic(record,"navigation_trace_start",{detail:`trace=${record.traceRun}; position=${startPosition}; total=${record.state.total||0}; dict=${record.state.target?.dict||""}; block=${record.state.target?.blockStart??""}`});
  try{
    while(!remoteFindTraceStop&&remoteFind===record&&steps<100000){
      if(record.state?.complete&&Number(record.state.position)>=Number(record.state.total)){reason="last_match";break}
      const stepStarted=performance.now(),from=Number(record.state?.position)||0;
      const moved=await moveRemoteFind(1);
      if(remoteFind!==record){reason="search_changed";break}
      if(!moved){reason=record.error?"error":"no_next_match";break}
      steps++;
      const state=record.state||{},position=Number(state.position)||0;
      await remoteFindDiagnostic(record,"navigation_trace_step",{completed:Math.min(100000,steps),total:Math.min(100000,state.total||0),detail:`trace=${record.traceRun}; step=${steps}; from=${from}; to=${position}; complete=${!!state.complete}; step_ms=${Math.round(performance.now()-stepStarted)}; dict=${state.target?.dict||""}; article=${state.target?.articleIndex??""}; block=${state.target?.blockStart??""}`});
      if(state.complete&&position<=previousPosition){
        reason="last_match";
        const restored=await moveRemoteFind(-1);
        if(restored)steps++;
        break;
      }
      previousPosition=position;
      await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
    }
    if(steps>=100000)reason="limit";
    if(remoteFindTraceStop)reason="stopped";
  }catch(error){reason="error";await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; error=${String(error?.message||error).slice(0,700)}`})}
  finally{
    await remoteFindDiagnostic(record,reason==="stopped"?"navigation_trace_stop":"navigation_trace_end",{completed:Math.min(100000,steps),total:Math.min(100000,record.state?.total||0),detail:`trace=${record.traceRun}; reason=${reason}; steps=${steps}; elapsed_ms=${Math.round(performance.now()-started)}; position=${record.state?.position||0}`});
    remoteFindTraceActive=false;remoteFindTraceStop=false;syncRemoteFindTraceButton();paint();
  }
}
function tracePercentile(values,fraction){
  if(!values.length)return 0;
  const sorted=[...values].sort((a,b)=>a-b);
  return sorted[Math.min(sorted.length-1,Math.floor(sorted.length*fraction))];
}
/* Deterministic places to jump to: the ends, the middle, then a fixed sequence
   of pseudo-random positions. Two runs of the same search are comparable, and
   every run visits the ends, where the wrapping cases live. */
function traceSeekPositions(total){
  if(total<2)return total===1?[1]:[];
  const positions=new Set([1,total,Math.max(1,Math.round(total/2))]);
  let seed=0x9e3779b9;
  while(positions.size<6&&positions.size<total){
    seed=(seed*1664525+1013904223)>>>0;
    positions.add(1+(seed%total));
  }
  return [...positions].sort((a,b)=>a-b);
}
/* Waits until the strip reports the expected position: navGoto resolves before
   the navigation does, so a jump cannot be timed by awaiting it. */
async function traceWaitForPosition(expected,timeout=5000){
  const started=performance.now();
  while(performance.now()-started<timeout){
    const position=Number(fullTextNavigation?.current)||0;
    if(position===expected)return position;
    await new Promise(resolve=>setTimeout(resolve,25));
  }
  return Number(fullTextNavigation?.current)||0;
}
async function runTraceRun(record,run){
  const navigation=fullTextNavigation,generation=navigation?.generation;
  const started=performance.now(),timings=[];
  let steps=0,reason="done";
  await remoteFindDiagnostic(record,"navigation_trace_run_start",{detail:`trace=${record.traceRun}; run=${run}; position=${navigation?.current||0}; total=${navigation?.total||0}`});
  try{
    if(run==="forward"||run==="backward"){
      const direction=run==="forward"?1:-1;
      let previous=Number(fullTextNavigation?.current)||0;
      while(!remoteFindTraceStop&&fullTextNavigation?.generation===generation&&steps<100000){
        const current=fullTextNavigation;
        if(direction>0&&current.complete&&current.current>=current.total)break;
        if(direction<0&&(Number(current.current)||0)<=1)break;
        const from=Number(current.current)||0,t0=performance.now();
        const advanced=direction>0?await current.next():await current.previous();
        const next=fullTextNavigation;
        if(!next||next.generation!==generation){reason="search_changed";break}
        const position=Number(next.current)||0;
        steps++;timings.push(performance.now()-t0);
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,next.total||0),detail:`trace=${record.traceRun}; run=${run}; step=${steps}; from=${from}; to=${position}; complete=${!!next.complete}; step_ms=${Math.round(performance.now()-t0)}`});
        if(!position||position===from){reason=advanced===false?"navigation_unavailable":"no_next_match";break}
        // A forward walk that comes back to where it was has wrapped: the
        // count was still growing when the last occurrence answered.
        if(direction>0&&next.complete&&position<=previous){await next.previous();break}
        previous=position;
        await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
      }
    }else{
      for(const page of traceSeekPositions(Number(fullTextNavigation?.total)||0)){
        if(remoteFindTraceStop)break;
        const t0=performance.now();
        fullTextNavigation?.goto?.(page);
        const landed=await traceWaitForPosition(page);
        steps++;timings.push(performance.now()-t0);
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=seek; step=${steps}; goto=${page}; to=${landed}; step_ms=${Math.round(performance.now()-t0)}`});
        // Both sides of a jump: the block it landed in is exercised in both
        // directions, which is where the prefetch and the wrap live.
        for(const direction of [1,-1]){
          const from=Number(fullTextNavigation?.current)||0,t1=performance.now();
          await (direction>0?fullTextNavigation?.next?.():fullTextNavigation?.previous?.());
          const position=Number(fullTextNavigation?.current)||0;
          steps++;timings.push(performance.now()-t1);
          await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=seek; step=${steps}; from=${from}; to=${position}; dir=${direction}; step_ms=${Math.round(performance.now()-t1)}`});
        }
        await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
      }
    }
  }catch(error){reason="error";await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; run=${run}; error=${String(error?.message||error).slice(0,700)}`})}
  if(remoteFindTraceStop)reason="stopped";
  const summary={run,steps,elapsed:Math.round(performance.now()-started),reason,
    p50:Math.round(tracePercentile(timings,.5)),p90:Math.round(tracePercentile(timings,.9)),
    max:Math.round(timings.length?Math.max(...timings):0)};
  await remoteFindDiagnostic(record,"navigation_trace_run_end",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=${run}; steps=${steps}; p50=${summary.p50}; p90=${summary.p90}; max=${summary.max}; elapsed_ms=${summary.elapsed}; reason=${reason}`});
  return summary;
}
async function runFullTextTrace(mode){
  const navigation=fullTextNavigation,generation=navigation?.generation;
  if(remoteFindTraceActive||options.mode!=="fts"||!navigation?.canNext)return;
  const run=mode||"forward";
  remoteFindTraceActive=true;remoteFindTraceStop=false;
  const record={diagnosticRun:++remoteFindDiagnosticRun,diagnosticStarted:performance.now(),traceRun:++remoteFindTraceRun};
  const origin=Number(navigation.current)||0;
  syncRemoteFindTraceButton();
  const summaries=[];
  try{
    await remoteFindDiagnostic(record,"navigation_trace_start",{detail:`trace=${record.traceRun}; source=full_text; run=${run}; position=${origin}; total=${navigation.total||0}`});
    for(const name of run==="full"?["forward","backward","seeks"]:[run]){
      if(remoteFindTraceStop)break;
      summaries.push(await runTraceRun(record,name));
    }
    // A diagnostic leaves the reader where it found them.
    if(origin&&!remoteFindTraceStop){
      fullTextNavigation?.goto?.(origin);
      await traceWaitForPosition(origin);
    }
  }catch(error){await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; error=${String(error?.message||error).slice(0,700)}`})}
  finally{
    const totals=summaries.map(s=>`${s.run}: steps=${s.steps} p50=${s.p50} p90=${s.p90} max=${s.max} elapsed_ms=${s.elapsed} reason=${s.reason}`).join(" | ");
    await remoteFindDiagnostic(record,"navigation_trace_end",{completed:summaries.reduce((sum,s)=>sum+s.steps,0),total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; source=full_text; suite=${run}; position=${fullTextNavigation?.current||0}`});
    await remoteFindDiagnostic(record,"navigation_trace_summary",{completed:summaries.reduce((sum,s)=>sum+s.steps,0),detail:`trace=${record.traceRun}; suite=${run}; ${totals||"no runs"}; position=${fullTextNavigation?.current||0}`});
    remoteFindTraceActive=false;remoteFindTraceStop=false;syncRemoteFindTraceButton();
  }
}
async function restoreRemoteFindTarget(){
  if(!remoteFind?.target||remoteFindPending)return;
  const record=remoteFind,token=remoteFindToken,target=record.target,state=record.state;
  remoteFindPending=true;paint();
  try{record.error=await showRemoteFindTarget(target,state,token)?"":t("search.fullListUnavailable")}
  catch(error){record.error=String(error?.message||error)}
  finally{if(token===remoteFindToken&&remoteFind===record){remoteFindPending=false;paint()}}
}
async function gotoRemoteFind(page){
  if(!remoteFind?.id||remoteFindPending||!Number.isInteger(page)||page<1)return;
  remoteFindPending=true;const record=remoteFind,token=remoteFindToken,request=++remoteFindRequestSeq;
  try{
    const state=await remoteFindFetch(record,"navGoto",{position:String(page)});
    if(request!==remoteFindRequestSeq||token!==remoteFindToken||remoteFind!==record)return;
    if(state.found){record.state=state;const paintStarted=performance.now();record.error=await showRemoteFindTarget(state.target,state,token)?"":t("search.fullListUnavailable");
      remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${state.target?.dict||""}; block=${state.target?.blockStart??""}; success=${!record.error}`})}
    else record.state={...record.state,total:state.total,complete:state.complete};
  }catch(error){record.error=String(error?.message||error)}
  finally{if(request===remoteFindRequestSeq&&token===remoteFindToken){remoteFindPending=false;paint()}}
}
function rebuild() {
  if (options.mode === "fts") { closeRemoteFind();syncControls(); return; }
  active=!!options.query;
  if(!active){closeRemoteFind();matches=[];index=-1;docs.clear();restoreExamples();clearPaint();syncControls();return}
  void startRemoteFind();
}
function showExample(hit) {
  for (let el = hit.example; el; el = el.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")) {
    if (el.ownerDocument.defaultView.getComputedStyle(el).display !== "none") continue;
    restored.push([el,el.style.getPropertyValue("display"),el.style.getPropertyPriority("display")]);
    el.style.setProperty("display",el.matches("span,.wu-inline-example") ? "inline" : "block","important");
  }
}
function topOf(hit) {
  const rect = hit.range.getBoundingClientRect();
  // WebView2 can defer layout inside a closed dictionary's content-visibility
  // subtree. Until the text has a box, its dictionary is the safe target.
  if (!window.wudictNativeShell && !rect.width && !rect.height)
    return (hit.host.closest("details.dict") || hit.host).getBoundingClientRect().top;
  return rect.top + (hit.host.shadowRoot ? 0 : hit.host.getBoundingClientRect().top);
}
function settleOn(hit, det) {
  if (settle) settle();
  let moves = 0, stable = 0, pending = 0, frame = 0, observer = null;
  const quit = ["wheel","touchstart","pointerdown","keydown"];
  const stopSettle = () => {
    clearTimeout(pending); clearTimeout(timeout); cancelAnimationFrame(frame);
    observer?.disconnect();
    for (const event of quit) removeEventListener(event,stopSettle);
    if (settle === stopSettle) settle = null;
  };
  const align = () => {
    if (!active || matches[index] !== hit || !hit.host.isConnected) return stopSettle();
    const rect = hit.range.getBoundingClientRect(), laidOut = !!(rect.width || rect.height);
    const top = topOf(hit), bottom = top + rect.height;
    const ceiling = barH() + (det?.querySelector(":scope>summary")?.offsetHeight || 0);
    const floor = innerHeight - strip.offsetHeight - 8;
    // Opening the target dictionary can move it again as earlier sections
    // finish laying out. Keep checking until the whole result list settles.
    if (laidOut && top >= ceiling && top < floor && Math.min(bottom,top+24) <= floor) {
      if (++stable >= 8) return stopSettle();
    } else {
      stable = 0;
      if (++moves > 30) return stopSettle();
      if (!laidOut) {
        // A zero-sized Range inside content-visibility:auto has no useful
        // page coordinates. Scrolling its text node into view makes WebView2
        // lay out that part of a shadow article or iframe first.
        hit.range.startContainer.parentElement?.scrollIntoView({block:"center",behavior:"instant"});
      } else revealAt(top,det,true);
    }
    pending = setTimeout(align,80);
  };
  observer = new ResizeObserver(() => { stable = 0; });
  observer.observe($("out"));
  if (det) observer.observe(det);
  observer.observe(hit.host);
  for (const event of quit) addEventListener(event,stopSettle,{passive:true});
  const timeout = setTimeout(stopSettle,6500);
  settle = stopSettle;
  frame = requestAnimationFrame(align);
}
function go(step) {
  if ($("articleFindMode").value === "fts") {
    if (step<0?fullTextNavigation?.canPrevious:fullTextNavigation?.canNext)
      fullTextNavigation[step < 0 ? "previous" : "next"]?.();
    return;
  }
  if(remoteFind){void moveRemoteFind(step);return}
  if(active){void startRemoteFind();return}
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
}
function goto(page) {
  if ($("articleFindMode").value === "fts") { fullTextNavigation?.goto?.(page); return true; }
  if(remoteFind){void gotoRemoteFind(page);return true}
  if(active){void startRemoteFind();return true}
  return false;
}
function searchFrom(host,node,offset,direction) {
  if (!host?.isConnected || !node) return;
  const doc=node.ownerDocument, range=doc.createRange();
  try { range.setStart(node,Math.max(0,Math.min(offset,node.nodeType===Node.TEXT_NODE?node.length:node.childNodes.length))); range.collapse(true); }
  catch (_) { return; }
  searchAnchor={host,range,direction:direction<0?-1:1};
  index=-1;
  if (options.mode === "fts" && fullTextNavigation) { fullTextNavigation.seekFrom?.(host,node,offset,direction); return; }
  if (!dialog.open) open(true); else { readOptions(); active=!!options.query; rebuild(); }
  $("articleFindQuery").focus();
}
function gotoOpen() {
  // The strip's own count button opens this too, and after a reload the
  // dialog's mode select is not necessarily on fts even though the walk is
  // alive; the walk itself decides, not the dialog's stale value.
  const isFTS = !!fullTextNavigation;
  const isRemote = !isFTS && options.mode !== "fts" && !!remoteFind;
  const total = isFTS ? Number(fullTextNavigation.total)||0 : isRemote ? Number(remoteFind.state?.total)||0 : 0;
  const current = isFTS ? Number(fullTextNavigation.current)||1 : isRemote ? Number(remoteFind.state?.position)||1 : 1;
  if (total) window.wudictOpenArticleFindGoto?.(total,current);
}
function stop() {
  closeRemoteFind();
  const navigation = fullTextNavigation;
  fullTextNavigation = null;
  if (settle) settle();
  active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1;
  docs.clear();
  if (dialog.open) dialog.close();
  strip.hidden = true; measure();
  navigation?.close?.();
}
function setFullTextNavigation(state) {
  const newSearch = !!state && state.generation !== fullTextGeneration;
  if (state && state.generation !== undefined) fullTextGeneration = state.generation;
  fullTextNavigation = state || null;
  const reading = $("articleFindReading");
  reading.hidden = !fullTextNavigation;
  if (fullTextNavigation) reading.textContent = {phrase:"[A B]",near:"[A ↔ B]",fts:"[A … B]"}[fullTextNavigation.reading] || "";
  if (!fullTextNavigation) {
    $("articleFindMode").querySelector('option[value="fts"]').disabled = true;
    if (!active && !dialog.open) strip.hidden = true;
    syncControls();
    measure();
    return;
  }
  if (newSearch && !dialog.open) {
    options = {...options, query:fullTextNavigation.query || "", mode:"fts"};
    closeRemoteFind();
    active=false;matches=[];index=-1;docs.clear();restoreExamples();clearPaint();
  }
  if (active && dialog.open && $("articleFindMode").value === "fts") {
    active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1; docs.clear();
  }
  if (options.mode === "fts" || newSearch || dialog.open && $("articleFindMode").value === "fts") {
    $("articleFindLabel").textContent = fullTextNavigation.query || "";
    $("articleFindLabel").title = fullTextNavigation.query || "";
  }
  if (dialog.open) {
    if ($("articleFindMode").value === "fts") {
      options = {...options, query:fullTextNavigation.query || "", mode:"fts"};
      $("articleFindQuery").value = options.query;
      syncControls();
    }
    strip.hidden = true;
  } else if (options.mode === "fts" || newSearch) {
    strip.hidden = !fullTextNavigation.visible;
    $("articleFindCount").textContent = fullTextNavigation.position || "";
    $("articleFindCount").disabled = !(Number(fullTextNavigation.total)>0);
    $("articleFindPrev").disabled = !fullTextNavigation.canPrevious;
    $("articleFindNext").disabled = !fullTextNavigation.canNext;
  }
  syncControls();
  measure();
}
function schedule() { clearTimeout(timer); timer = setTimeout(() => { if (active) rebuild(); },100); }
function measure() {
  document.documentElement.style.setProperty("--article-find-height", strip.hidden ? "0px" : strip.offsetHeight+"px");
}
$("sbarFind").onclick = open;
$("articleFindLabel").onclick = open;
$("articleFindCount").onclick = gotoOpen;
$("articleFindClose").onclick = () => dialog.close();
$("articleFindStop").onclick = stop;
$("articleFindPrev").onclick = () => fullTextNavigation && options.mode === "fts" ? fullTextNavigation.previous?.() : go(-1);
$("articleFindNext").onclick = () => fullTextNavigation && options.mode === "fts" ? fullTextNavigation.next?.() : go(1);
$("articleFindScrollToMatch").onclick = () => {
  if(options.mode === "fts"){fullTextNavigation?.restore?.();return}
  if(remoteFind){void restoreRemoteFindTarget();return}
};
$("articleFindTrace").onclick = () => {
  if(remoteFindTraceActive){remoteFindTraceStop=true;return}
  if(options.mode==="fts"){void runFullTextTrace("forward");return}
  void runRemoteFindTrace();
};
$("articleFindTraceRun").addEventListener("change",()=>{
  const select=$("articleFindTraceRun"),run=select.value;
  select.value="";
  window.wudictThemedSelects?.articleFindTraceRun?.sync();
  if(!run||remoteFindTraceActive||options.mode!=="fts")return;
  void runFullTextTrace(run);
});
$("sysLogSearch")?.addEventListener("change",syncRemoteFindTraceButton);
$("articleFindDialogPrev").onclick = () => go(-1);
$("articleFindDialogNext").onclick = () => go(1);
$("articleFindWholeWords").onchange = () => {
  const select = $("articleFindMode");
  if (select.value === "fts") return;
  select.value = $("articleFindWholeWords").checked ? "word" : "contains";
  options = {...options, mode: select.value, wholeWords: $("articleFindWholeWords").checked};
  syncControls(); save();
  if (dialog.open) { active = !!options.query; if (active) rebuild(); }
};
$("articleFindMode").onchange = () => {
  if ($("articleFindMode").value === "fts") {
    if (!fullTextNavigation) { $("articleFindMode").value = options.mode = "contains"; syncControls(); return; }
    closeRemoteFind();
    options = {...options, query:fullTextNavigation.query || "", mode:"fts"};
    $("articleFindQuery").value = options.query;
    active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1; docs.clear();
    save(); syncControls();
    return;
  }
  readOptions(); index = -1; active = !!options.query; rebuild(); syncControls();
};
for (const id of ["articleFindCase","articleFindAll","articleFindExamples"]) $(id).onchange = () => { readOptions(); index = -1; active = !!options.query; rebuild(); };
$("articleFindQuery").onfocus = () => { if(!suppressFindHistoryFocus)renderFindHistory(); };
$("articleFindQuery").oninput = () => { if ($("articleFindMode").value === "fts") { hideFindHistory(); return; } readOptions(); renderFindHistory(); index = -1; active = !!options.query; schedule(); };
 dialog.addEventListener("keydown", e => {
  const list=$("articleFindHistory"),input=$("articleFindQuery");
  if(e.key==="Escape"&&!list.hidden){e.preventDefault();e.stopPropagation();hideFindHistory();input.focus();return}
  if(e.key==="ArrowDown"&&e.target===input&&!list.hidden){e.preventDefault();list.firstElementChild?.focus();return}
  if((e.key==="ArrowDown"||e.key==="ArrowUp")&&e.target.matches?.('#articleFindHistory [role="option"]')){
    e.preventDefault();const rows=[...list.querySelectorAll('[role="option"]')],at=rows.indexOf(e.target),next=at+(e.key==="ArrowDown"?1:-1);
    if(rows[next])rows[next].focus();else if(e.key==="ArrowUp")input.focus();return;
  }
  if (e.key === "Enter" && e.target.tagName !== "BUTTON") { e.preventDefault(); go(e.shiftKey ? -1 : 1); }
});
dialog.addEventListener("cancel",e=>{if(!$("articleFindHistory").hidden){e.preventDefault();hideFindHistory();$("articleFindQuery").focus()}});
document.addEventListener("pointerdown",e=>{if(!$("articleFindQueryBox").contains(e.target))hideFindHistory()});
dialog.addEventListener("close", () => {
  if($("articleFindMode").value!=="fts"){readOptions();recordFindHistory(options.query)}
  hideFindHistory();
  if (active) strip.hidden = false;
  else if (fullTextNavigation) strip.hidden = !fullTextNavigation.visible;
  measure();
});
document.addEventListener("keydown", e => {
  if (document.querySelector("dialog[open]:not(#articleFindDialog)")) return;
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f") { e.preventDefault(); open(); }
  else if (active && e.key === "F3") { e.preventDefault(); e.stopImmediatePropagation(); go(e.shiftKey ? -1 : 1); }
},true);
$("out").addEventListener("load", schedule, true);
new MutationObserver(schedule).observe($("out"),{childList:true,subtree:true});
new ResizeObserver(measure).observe(strip);
new MutationObserver(() => { measure(); if (active) schedule(); }).observe($("sbar"),{attributes:true,attributeFilter:["hidden"]});
window.addEventListener("resize",()=>{if(!$("articleFindHistory").hidden)sizeFindHistory()});
if(window.visualViewport){window.visualViewport.addEventListener("resize",()=>{if(!$("articleFindHistory").hidden)sizeFindHistory()});window.visualViewport.addEventListener("scroll",()=>{if(!$("articleFindHistory").hidden)sizeFindHistory()})}
$("sbarExamples").addEventListener("click",schedule);
$("examplesRow")?.addEventListener("click",schedule);
window.wudictArticleFind = {stop, refresh:schedule, setFullTextNavigation,
  serverRanges:(root,occurrences)=>serverFindRanges(root,occurrences),
  saveTraceArticle:(dict,article,html)=>{
  if(!remoteFindTraceActive)return Promise.resolve(false);
  if(!html){
    void remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now()},"navigation_trace_article_save_failed",{detail:`trace=${remoteFindTraceRun}; dict=${dict}; article=${article}; reason=empty_html`});
    return Promise.resolve(false);
  }
  const bytes=new TextEncoder().encode(html).byteLength;
  if(bytes>2*1024*1024){
    void remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now()},"navigation_trace_article_save_failed",{detail:`trace=${remoteFindTraceRun}; dict=${dict}; article=${article}; bytes=${bytes}; reason=over_limit`});
    return Promise.resolve(false);
  }
  return sendBrowseDiagnostic({event:"search_trace_article",runId:remoteFindTraceRun,dict,page:Number(article)||0,completed:remoteFindTraceRun,html})
    .then(async saved=>{
    if(!saved)await remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now()},"navigation_trace_article_save_failed",{detail:`trace=${remoteFindTraceRun}; dict=${dict}; article=${article}; bytes=${bytes}; reason=server_rejected`});
    return saved;
  });
}, traceEvent:(stage,detail,elapsedMs=0)=>{
  if(!remoteFindTraceActive)return;
  void remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now(),traceRun:remoteFindTraceRun},stage,{elapsedMs,detail:`trace=${remoteFindTraceRun}; source=full_text; ${detail}`});
}, fullTextModeActive:()=>options.mode === "fts", isActive:()=>active&&!!options.query, searchAcrossBlocks:()=>true, searchFrom, goto, setHistoryLimit:n=>{findHistory=findHistory.slice(0,Math.max(0,Math.min(1000,Number(n)||0)));saveFindHistory();renderFindHistory()}};
})();
