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
let options = {query:"", wholeWords:false, matchCase:false, all:true, examples:false};
try {
  const saved = JSON.parse(localStorage.getItem("wudict_article_find") || "{}");
  Object.assign(options, saved);
  // The dialog used to carry a mode of its own; it does not any more. The
  // whole-words switch is the only choice, and an old "word" reading is it.
  if (saved.mode === "word") options.wholeWords = true;
} catch (_) {}
delete options.blocks; // Retire the former loaded-block-only preference.
delete options.mode;
function searchMode() { return options.wholeWords ? "word" : "contains"; }
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
  if(!dialog.open||!findHistoryLimit()){hideFindHistory();return}
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
  options = {query:$("articleFindQuery").value.trim(), wholeWords:$("articleFindWholeWords").checked,
    matchCase:$("articleFindCase").checked, all:$("articleFindAll").checked,
    examples:$("articleFindExamples").checked};
  save();
}
function fillOptions() {
  $("articleFindQuery").value = options.query;
  $("articleFindWholeWords").checked = !!options.wholeWords;
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
}
function open(preserveAnchor=false) {
  if (!preserveAnchor) searchAnchor = null;
  /* The dialog is one thing everywhere: a search inside the articles the
     current answer found. It opens empty, with its own history under the field,
     and it does not touch the answer - the walk keeps its cursor and the strip
     keeps showing it on its base layer. The field is filled only when the
     reader asked to search from a selection. */
  if (!preserveAnchor) options = {...options, query:""};
  fillOptions();
  moveStripIntoDialog();
  if (!dialog.open) dialog.showModal();
  if (fullTextNavigation) showAnswerLayer();
  renderFindHistory();
  syncControls();
  $("articleFindQuery").focus();
  $("articleFindQuery").select();
}
/* The strip is one set of controls over two searches. The base layer is the
   answer's walk - it is not ours to close, so it carries no close button. The
   top layer is the dialog's search inside those articles, and it does. Layers
   switch without re-running anything: both cursors are kept. */
let stripLayer = "answer";
function showAnswerLayer(){ stripLayer="answer"; renderStrip(); }
function renderStrip(){
  const answer=stripLayer==="answer"?fullTextNavigation:null;
  const found=stripLayer==="found"?remoteFind:null;
  if(!answer&&!found){ strip.hidden=true; measure(); syncControls(); return; }
  strip.hidden=false;
  $("articleFindStop").hidden=!found;
  const reading=$("articleFindReading");
  reading.hidden=!answer;
  if(answer){
    reading.textContent={phrase:"[A B]",near:"[A ↔ B]",fts:"[A … B]"}[answer.reading]||"";
    $("articleFindLabel").textContent=answer.query||"";
    $("articleFindLabel").title=answer.query||"";
    $("articleFindCount").textContent=answer.position||"";
    $("articleFindCount").disabled=!(Number(answer.total)>0);
    $("articleFindFirst").disabled=!answer.canFirst;
    $("articleFindPrev").disabled=!answer.canPrevious;
    $("articleFindNext").disabled=!answer.canNext;
    $("articleFindLast").disabled=!answer.canLast;
    $("articleFindScrollToMatch").disabled=!answer.canRestore;
  }else{
    const state=found?.state||{};
    const position=Number(state.position)||0,total=Number(state.total)||0;
    const pending=!!found?.pending;
    const atStart=!!state.found&&position<=1;
    const atEnd=!!state.found&&!!state.complete&&total>0&&position>=total;
    $("articleFindLabel").textContent=options.query;
    $("articleFindLabel").title=options.query;
    $("articleFindCount").textContent=serverFindPosition();
    $("articleFindCount").disabled=!state.found;
    $("articleFindFirst").disabled=pending||atStart;
    $("articleFindPrev").disabled=pending||atStart;
    $("articleFindNext").disabled=pending||atEnd;
    $("articleFindLast").disabled=pending||atEnd;
    $("articleFindScrollToMatch").disabled=pending||!found?.target;
  }
  measure();
  syncControls();
}
function syncControls() {
  const found=remoteFind;
  const step=!!found?.id&&!pending&&serverFindCanStep();
  const display=found?(found.error?"⚠ "+t("search.fullListUnavailable"):serverFindPosition()):"";
  // The strip's own count and crosshair belong to its layer renderer; the
  // dialog mirrors the search in the status line and its two buttons.
  $("articleFindStatus").textContent=display||t("articleFind.scope");
  $("articleFindDialogPrev").disabled=!step;
  $("articleFindDialogNext").disabled=!step;
  syncRemoteFindTraceButton();
}
function canStep(step){
  return !!remoteFind&&!remoteFindPending&&serverFindCanStep();
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
  renderStrip();
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
  const fullText=stripLayer==="answer"&&!!fullTextNavigation;
  const visible=fullText||stripLayer==="found"&&!!remoteFind?.id;
  button.hidden=false;
  button.textContent=remoteFindTraceActive?t("articleFind.traceStop"):t("articleFind.traceStart");
  button.setAttribute("aria-label",button.textContent);
  button.title=searchArticleDiagnosticsEnabled()?button.textContent:`${button.textContent} — ${t("articleFind.traceLogDisabled")}`;
  button.disabled=!visible||!remoteFindTraceActive&&(fullText?!fullTextNavigation?.canNext:!remoteFind?.state?.found||remoteFindPending);
}
function remoteFindDiagnostic(record,stage,fields={}){
  if(!remoteFindTraceEnabled())return Promise.resolve(false);
  const payload={event:"search_article",stage,runId:record?.diagnosticRun||remoteFindDiagnosticRun,
    query:String(options.query||"").replace(/[\r\n]/g," ").slice(0,1024),mode:searchMode(),
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
  return JSON.stringify([context.groupID,context.dictionaries,context.orders,context.cacheCount,context.query,context.mode,options.query,searchMode(),
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
  if(!options.query){
    closeRemoteFind();matches=[];index=-1;docs.clear();restoreExamples();clearPaint();
    stripLayer="answer";renderStrip();
    return;
  }
  const context=remoteFindContext(),anchor=serverFindAnchor(),signature=remoteFindSignature(context)+JSON.stringify(anchor);
  if(remoteFind?.signature===signature){paint();return}
  closeRemoteFind();
  if(!context.dictionaries.length||!context.query||!context.mode){remoteFind={signature,state:{complete:true,total:0,found:false}};stripLayer="found";paint();return}
  const record={id:remoteFindID(),signature,state:null,pending:true,error:"",diagnosticRun:++remoteFindDiagnosticRun,diagnosticStarted:performance.now()};
  remoteFind=record;remoteFindPending=true;stripLayer="found";
  const token=++remoteFindToken;
  matches=[];index=-1;docs.clear();restoreExamples();clearPaint();paint();
  try{
    const size=Math.max(1,Math.min(500,context.pageSize)),firstSize=Math.max(1,Math.min(500,context.firstSize));
    const params={dict:context.dictionaries.join(","),cacheCount:String(context.cacheCount),
      cacheOrders:context.orders.join(","),q:options.query,mode:searchMode(),
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
  // Same reading as the answer's layer: while the count is still provisional
  // the number counts from the edge the reader came from, and a small arrow says
  // which way they are travelling.
  const fromEnd=Number(state.target?.fromEnd)||0;
  const shown=fromEnd>0?fromEnd:Number(state.position)||1;
  const direction=total&&!state.complete?(fromEnd>0?"↑ ":"↓ "):"";
  return t("articleFind.position",{current:direction+shown,total:String(total)+(state.complete?"":"+")});
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
  if(!searchAnchor)return null;
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
  const visited=[],landed={};
  let steps=0,reason="done";
  await remoteFindDiagnostic(record,"navigation_trace_run_start",{detail:`trace=${record.traceRun}; run=${run}; position=${navigation?.current||0}; total=${navigation?.total||0}`});
  try{
    if(run==="forward"||run==="backward"){
      // Every run starts at its own edge: "start to end" at the first match,
      // "end to start" at the last, so the trace covers the whole track.
      await (run==="forward"?fullTextNavigation?.first?.():fullTextNavigation?.last?.());
      const direction=run==="forward"?1:-1;
      let previous=Number(fullTextNavigation?.current)||0;
      if(previous)visited.push({p:previous,at:fullTextNavigation?.at||""});
      while(!remoteFindTraceStop&&fullTextNavigation?.generation===generation&&steps<100000){
        const current=fullTextNavigation;
        if(!current||current.generation!==generation){reason="search_changed";break}
        if(direction>0&&current.complete&&current.current>=current.total)break;
        if(direction<0&&(Number(current.current)||0)<=1)break;
        const from=Number(current.current)||0,t0=performance.now();
        const advanced=direction>0?await current.next():await current.previous();
        const next=fullTextNavigation;
        if(!next||next.generation!==generation){reason="search_changed";break}
        const position=Number(next.current)||0;
        // An arrow that has nowhere left to go answers "found=false, complete"
        // and leaves the cursor alone: the walk does not wrap for a reader, so
        // the position is unchanged and there is nothing to record or undo.
        if(!position||position===from){
          if(advanced===false&&next.complete)reason="end";
          else reason=advanced===false?"navigation_unavailable":"no_next_match";
          break;
        }
        steps++;timings.push(performance.now()-t0);
        visited.push({p:position,at:next.at||""});
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,next.total||0),detail:`trace=${record.traceRun}; run=${run}; step=${steps}; from=${from}; to=${position}; complete=${!!next.complete}; at=${next.at||""}; step_ms=${Math.round(performance.now()-t0)}`});
        previous=position;
        await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
      }
    }else{
      // Jump around: a random place, a few steps down, the end, a few steps up,
      // the start - the short hops and both edges in one run.
      const positions=traceSeekPositions(Number(fullTextNavigation?.total)||0);
      const landing=positions[Math.min(1,positions.length-1)]||1;
      const step=async(label,action)=>{
        const t0=performance.now();
        await action?.();
        const position=Number(fullTextNavigation?.current)||0;
        steps++;timings.push(performance.now()-t0);
        visited.push({p:position,at:fullTextNavigation?.at||""});
        landed[label]=position;
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=jumps; step=${steps}; ${label}; to=${position}; at=${fullTextNavigation?.at||""}; step_ms=${Math.round(performance.now()-t0)}`});
      };
      fullTextNavigation?.goto?.(landing);
      await traceWaitForPosition(landing);
      if(Number(fullTextNavigation?.current))visited.push({p:Number(fullTextNavigation.current),at:fullTextNavigation?.at||""});
      for(let i=0;i<3;i++)await step("down",()=>fullTextNavigation?.next?.());
      await step("last",()=>fullTextNavigation?.last?.());
      for(let i=0;i<3;i++)await step("up",()=>fullTextNavigation?.previous?.());
      await step("first",()=>fullTextNavigation?.first?.());
      // The edges are known without walking: first is 1, last is the total once
      // the count is exact. A jump that missed says so here, not in a grep.
      const total=Number(fullTextNavigation?.total)||0;
      landed.edges=(landed.first===1?"first=ok":"first="+landed.first)+" "+
        (!fullTextNavigation?.complete?"last=provisional":
          landed.last===total?"last=ok":"last="+landed.last+"/"+total);
    }
  }catch(error){reason="error";await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; run=${run}; error=${String(error?.message||error).slice(0,700)}`})}
  if(remoteFindTraceStop)reason="stopped";
  const seen=new Map();
  let revisited=0,mismatched=0;
  for(const visit of visited){
    if(!visit.p||!visit.at)continue;
    if(seen.has(visit.p)){revisited++;if(seen.get(visit.p)!==visit.at)mismatched++;}
    else seen.set(visit.p,visit.at);
  }
  const sequence=visited.map(visit=>visit.p);
  const ordered=run==="backward"?sequence.every((p,i)=>!i||p<sequence[i-1]):
    run==="forward"?sequence.every((p,i)=>!i||p>sequence[i-1]):true;
  const digest=sequence.length<=40?sequence.join(","):sequence.slice(0,10).join(",")+",…,"+sequence.slice(-10).join(",");
  await remoteFindDiagnostic(record,"navigation_trace_positions",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=${run}; count=${sequence.length}; ordered=${ordered}; revisited=${revisited}; mismatched=${mismatched}; ${run==="seeks"?(landed.edges||"edges=none")+"; ":""}seq=${digest}`});
  const summary={run,steps,elapsed:Math.round(performance.now()-started),reason,
    p50:Math.round(tracePercentile(timings,.5)),p90:Math.round(tracePercentile(timings,.9)),
    max:Math.round(timings.length?Math.max(...timings):0),
    sequence,atOf:Object.fromEntries(seen)};
  await remoteFindDiagnostic(record,"navigation_trace_run_end",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=${run}; steps=${steps}; p50=${summary.p50}; p90=${summary.p90}; max=${summary.max}; elapsed_ms=${summary.elapsed}; reason=${reason}`});
  return summary;
}
async function runFullTextTrace(mode){
  const navigation=fullTextNavigation,generation=navigation?.generation;
  if(remoteFindTraceActive||stripLayer!=="answer"||!navigation?.canNext)return;
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
    // The two walks cover the same track from opposite ends: one sequence
    // reversed must equal the other, position for position, and a position must
    // carry the same target whichever run reached it. That is what "the jumps
    // are correct" means, and it is checked here rather than by eye later.
    const forward=summaries.find(summary=>summary.run==="forward");
    const backward=summaries.find(summary=>summary.run==="backward");
    if(forward&&backward&&forward.sequence.length&&backward.sequence.length){
      const left=[...forward.sequence].reverse(),right=backward.sequence;
      let divergence=-1;
      const shared=Math.min(left.length,right.length);
      for(let index=0;index<shared;index++){
        if(left[index]!==right[index]){divergence=index;break}
      }
      const agreement=divergence<0&&left.length===right.length?"same":
        divergence<0?"prefix":"at "+(divergence+1)+": "+left[divergence]+"≠"+right[divergence];
      let retargeted=0;
      for(const [position,at] of Object.entries(forward.atOf)){
        if(backward.atOf[position]&&backward.atOf[position]!==at)retargeted++;
      }
      await remoteFindDiagnostic(record,"navigation_trace_compare",{detail:`trace=${record.traceRun}; suite=${run}; forward=${left.length}; backward=${right.length}; agreement=${agreement}; retargeted=${retargeted}`});
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
async function remoteFindEdge(direction){
  if(!remoteFind?.id||remoteFindPending)return;
  remoteFindPending=true;const record=remoteFind,token=remoteFindToken,request=++remoteFindRequestSeq;
  try{
    const state=await remoteFindFetch(record,direction>0?"navFirst":"navLast");
    if(request!==remoteFindRequestSeq||token!==remoteFindToken||remoteFind!==record)return;
    if(state.found){record.state=state;const paintStarted=performance.now();
      record.error=await showRemoteFindTarget(state.target,state,token)?"":t("search.fullListUnavailable");
      remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${state.target?.dict||""}; block=${state.target?.blockStart??""}; edge=${direction>0?"first":"last"}; success=${!record.error}`})}
    else record.state={...record.state,total:state.total,complete:state.complete};
  }catch(error){record.error=String(error?.message||error)}
  finally{if(request===remoteFindRequestSeq&&token===remoteFindToken){remoteFindPending=false;paint()}}
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
  active=!!options.query;
  if(!active){
    closeRemoteFind();
    matches=[];index=-1;docs.clear();restoreExamples();clearPaint();
    stripLayer="answer";renderStrip();
    return;
  }
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
  if(remoteFind){void moveRemoteFind(step);return}
  readOptions();active=!!options.query;rebuild();
}
function goto(page) {
  if(remoteFind){void gotoRemoteFind(page);return true}
  return false;
}
function searchFrom(host,node,offset,direction) {
  if (!host?.isConnected || !node) return;
  const doc=node.ownerDocument, range=doc.createRange();
  try { range.setStart(node,Math.max(0,Math.min(offset,node.nodeType===Node.TEXT_NODE?node.length:node.childNodes.length))); range.collapse(true); }
  catch (_) { return; }
  searchAnchor={host,range,direction:direction<0?-1:1};

  index=-1;
  if (!dialog.open) open(true);
  // The field is empty until the reader types, but a selection under a live
  // answer means "find this text again": the answer's own query is the word
  // this gesture is about.
  if (!options.query && fullTextNavigation?.query) {
    options = {...options, query:fullTextNavigation.query};
    $("articleFindQuery").value = options.query;
  }
  readOptions(); active=!!options.query; rebuild();
  $("articleFindQuery").focus();
}
function gotoOpen() {
  const isFTS = stripLayer==="answer"&&!!fullTextNavigation;
  const isRemote = !isFTS && !!remoteFind;
  const total = isFTS ? Number(fullTextNavigation.total)||0 : isRemote ? Number(remoteFind.state?.total)||0 : 0;
  const current = isFTS ? Number(fullTextNavigation.current)||1 : isRemote ? Number(remoteFind.state?.position)||1 : 1;
  if (total) window.wudictOpenArticleFindGoto?.(total,current);
}
/* The close button belongs to the search layer alone: it ends the dialog's
   search over the found articles and hands the strip back to the answer
   underneath. With no answer - an ordinary headword search - there is nothing
   to fall back to, and the strip hides. */
function stop() {
  closeRemoteFind();
  active=false; clearTimeout(timer); restoreExamples(); clearPaint(); matches=[]; index=-1; docs.clear();
  options = {...options, query:""};
  $("articleFindQuery").value = "";
  stripLayer = "answer";
  renderStrip();
  if (dialog.open) renderFindHistory();
}
function setFullTextNavigation(state) {
  const newSearch = !!state && state.generation !== fullTextGeneration;
  if (state && state.generation !== undefined) fullTextGeneration = state.generation;
  fullTextNavigation = state || null;
  if (newSearch) {
    // A new answer is a new search: a search inside the previous answer
    // is over, and the strip goes back to its base layer.
    closeRemoteFind();
    active=false; clearTimeout(timer); restoreExamples(); clearPaint(); matches=[]; index=-1; docs.clear();
    options = {...options, query:""};
    if (dialog.open) $("articleFindQuery").value = "";
    stripLayer = "answer";
  }
  renderStrip();
}
function schedule() { clearTimeout(timer); timer = setTimeout(() => { if (active) rebuild(); },100); }
function measure() {
  // Inside the dialog the strip is not the page's bottom bar: the page keeps its
  // full height and the strip scrolls with the dialog.
  document.documentElement.style.setProperty("--article-find-height", strip.hidden||dialog.open ? "0px" : strip.offsetHeight+"px");
}
/* A modal dialog puts its backdrop over the page, so a strip left in the page
   could not be clicked while the dialog is open. It travels into the dialog for
   the duration and comes back to its place afterwards. */
const stripHome = document.createComment("articleFindStrip");
strip.parentNode.insertBefore(stripHome, strip);
function moveStripIntoDialog(){ const actions=dialog.querySelector(".article-find-actions"); if(strip.parentElement!==dialog) dialog.insertBefore(strip,actions||null); }
function restoreStripHome(){ if(strip.parentElement===dialog) stripHome.parentNode.insertBefore(strip,stripHome.nextSibling); }
$("sbarFind").onclick = open;
$("articleFindLabel").onclick = () => { if(!dialog.open) open(); };
$("articleFindCount").onclick = gotoOpen;
$("articleFindClose").onclick = () => dialog.close();
$("articleFindStop").onclick = stop;
$("articleFindFirst").onclick = () => { if(stripLayer==="found"){void remoteFindEdge(1);return} fullTextNavigation?.first?.(); };
$("articleFindPrev").onclick = () => { if(stripLayer==="found"){void moveRemoteFind(-1);return} fullTextNavigation?.previous?.(); };
$("articleFindNext").onclick = () => { if(stripLayer==="found"){void moveRemoteFind(1);return} fullTextNavigation?.next?.(); };
$("articleFindLast").onclick = () => { if(stripLayer==="found"){void remoteFindEdge(-1);return} fullTextNavigation?.last?.(); };
$("articleFindScrollToMatch").onclick = () => {
  if(stripLayer==="found"){if(remoteFind)void restoreRemoteFindTarget();return}
  fullTextNavigation?.restore?.();
};
$("articleFindTrace").onclick = () => {
  if(remoteFindTraceActive){remoteFindTraceStop=true;return}
  if(stripLayer==="found"){void runRemoteFindTrace();return}
  void runFullTextTrace("forward");
};
$("articleFindTraceRun").addEventListener("change",()=>{
  const select=$("articleFindTraceRun"),run=select.value;
  select.value="";
  window.wudictThemedSelects?.articleFindTraceRun?.sync();
  if(remoteFindTraceActive)return;
  if(stripLayer==="found"){void runRemoteFindTrace();return}
  if(!run)return;
  void runFullTextTrace(run);
});
$("sysLogSearch")?.addEventListener("change",syncRemoteFindTraceButton);
$("articleFindDialogPrev").onclick = () => go(-1);
$("articleFindDialogNext").onclick = () => go(1);
$("articleFindWholeWords").onchange = () => {
  readOptions(); syncControls(); save();
  if (dialog.open) { active = !!options.query; if (active) rebuild(); }
};
for (const id of ["articleFindCase","articleFindAll","articleFindExamples"]) $(id).onchange = () => { readOptions(); index = -1; active = !!options.query; rebuild(); };
$("articleFindQuery").onfocus = () => { if(!suppressFindHistoryFocus)renderFindHistory(); };
$("articleFindQuery").oninput = () => { readOptions(); renderFindHistory(); index = -1; active = !!options.query; if(active)schedule(); else rebuild(); };
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
  restoreStripHome();
  readOptions();
  if(options.query)recordFindHistory(options.query);
  // An empty field means no search layer: the strip goes back to the answer's
  // walk (or hides when there is none).
  if(!options.query)stripLayer="answer";
  hideFindHistory();
  renderStrip();
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
}, fullTextModeActive:()=>stripLayer==="answer"&&!!fullTextNavigation, isActive:()=>active&&!!options.query, searchAcrossBlocks:()=>true, searchFrom, goto, setHistoryLimit:n=>{findHistory=findHistory.slice(0,Math.max(0,Math.min(1000,Number(n)||0)));saveFindHistory();renderFindHistory()}};
})();
