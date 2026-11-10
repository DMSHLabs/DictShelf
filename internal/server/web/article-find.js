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
let remoteFindSpot = "";
let remoteFindSettled = Promise.resolve();
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
      const input=$("articleFindQuery");input.value=value;hideFindHistory();readOptions();index=-1;active=!!options.query;syncControls();
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
    /* The end of the track is either an exact count the position reached, or the
       outermost match a walk from the end found: after "to the last match" the
       count is usually still provisional, and asking for one more press to
       learn that nothing follows left the down arrow alive at the end of the
       answer. `fromEnd` is the server's own "how many matches stand between
       this one and the end", so 1 IS the last match. */
    const atEnd=!!state.found&&(Number(state.target?.fromEnd)===1||!!state.complete&&total>0&&position>=total);
    $("articleFindLabel").textContent=options.query;
    $("articleFindLabel").title=options.query;
    // The strip is where a reader of the page looks, and the dialog that would
    // carry a message is usually closed by then: a step that could not be shown
    // says so here instead of leaving the counter to claim a place on its own.
    const error=String(found?.error||"");
    $("articleFindCount").textContent=error?"⚠ "+error:serverFindPosition();
    $("articleFindCount").title=error;
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
  const found=remoteFind,pending=!!found?.pending;
  // The dialog's two buttons are how the search starts as well as how it steps:
  // they are live as soon as there is a query, not only once a session answers.
  // A session that is already at its end keeps them live - the strip reports the
  // end, the dialog simply hands over.
  // A refusal is shown as it came: "no matches" and "the anchor was rejected"
  // look the same to a reader otherwise, and neither says why Find did nothing.
  const display=found?(found.error?"⚠ "+found.error:serverFindPosition()):"";
  // The strip's own count and crosshair belong to its layer renderer; the
  // dialog mirrors the search in the status line.
  $("articleFindStatus").textContent=display||t("articleFind.scope");
  // While the search runs the dialog says so, and offers to stop it.
  $("articleFindSearching").hidden=!pending;
  $("articleFindStopFind").hidden=!pending;
  syncRemoteFindTraceButton();
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
  if(remoteFind&&options.query)stripLayer="found";
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
  findProgressEnd();
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
/* "Trace all" is a developer switch, not a release feature (config
   TEST_FIND_TRACE, read by the page as window.wudictFindTrace): the walk it
   starts writes every step into the system log, and a reader has no use for
   it. Both controls exist in the markup and stay hidden unless the
   installation asked for them. */
function remoteFindTraceAvailable(){return window.wudictFindTrace===true}
function syncRemoteFindTraceButton(){
  const button=$("articleFindTrace"),picker=$("articleFindTraceRun");
  if(!button)return;
  if(!remoteFindTraceAvailable()){button.hidden=true;if(picker)picker.hidden=true;return}
  if(picker)picker.hidden=false;
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
/* The page's task strip is where "something is happening" is said. A search is a
   burst of requests - one per step, sometimes seconds each while a block of
   articles is loaded - so the strip starts with the first and lingers a moment
   after the last, instead of starting and finishing on every step. */
let findProgressTimer=0,findProgressOn=false;
function findProgressStart(){
  if(findProgressOn)return;
  if(typeof maskTaskStart!=="function")return;
  findProgressOn=true;
  // background: the thin line at the bottom, the same one the dictionary scan
  // uses - not a card that names itself over the page.
  try{maskTaskStart("article-find",t("articleFind.searching"),null,{background:true})}catch(_){findProgressOn=false}
}
function findProgressPing(){
  findProgressStart();
  clearTimeout(findProgressTimer);
  findProgressTimer=setTimeout(findProgressEnd,600);
}
function findProgressEnd(){
  clearTimeout(findProgressTimer);findProgressTimer=0;
  if(!findProgressOn)return;
  findProgressOn=false;
  if(typeof maskTaskFinish==="function")try{maskTaskFinish("article-find")}catch(_){}
}
/* Where the reader is standing right now: the answer's current match, or the
   spot a selection pointed at. The dialog searches from here - down first, then
   up - so a word above the reader is not missed for being behind them. */
function findAnchor(){
  const fromSelection=serverFindAnchor();
  if(fromSelection)return fromSelection;
  const at=String(fullTextNavigation?.at||"").split(":");
  if(at.length>=2&&at[0]&&Number.isFinite(Number(at[1]))){
    return {dict:at[0],article:Number(at[1]),text:"",direction:1};
  }
  return null;
}
function remoteFindURL(record,action,extra={}){
  return "/api/search?"+new URLSearchParams({cacheAction:action,cacheID:record.id,origin:location.origin,...extra});
}
/* A refusal reaches the reader as a sentence, not as the wire body: the strip
   shows it where the position goes. The raw text still goes to the system log,
   which is where a cause belongs. Every refusal of the article cache means the
   answer's result pages are gone from the server, and a new main search is the
   way back - so that is what the sentence says, whatever the body holds. */
function remoteFindReason(body){
  const text=String(body||"").trim();
  let reason="";
  try{reason=String(JSON.parse(text)?.error||"")}catch(_){}
  if(reason||!text||text.startsWith("{")||text.length>160)return t("articleFind.cacheUnavailable");
  return text;
}
async function remoteFindFetch(record,action,extra={}){
	const started=performance.now();
  findProgressStart();
	const requestMeta={action,fromPosition:Number(record.state?.position)||0,fromTotal:Number(record.state?.total)||0,
    fromDict:record.state?.target?.dict||"",fromBlock:Number(record.state?.target?.blockStart)||0};
	const requestLogStarted=performance.now();
	await remoteFindDiagnostic(record,"navigation_request",{detail:`action=${action}; from=${requestMeta.fromPosition}/${requestMeta.fromTotal}; dict=${requestMeta.fromDict}; block=${requestMeta.fromBlock}; trace=${record.traceRun||0}`});
	const requestLogMs=Math.round(performance.now()-requestLogStarted);
	let response;
	try{response=await apiFetch(remoteFindURL(record,action,extra))}
	catch(error){findProgressPing();await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),detail:`action=${action}; transport_error=${String(error).slice(0,700)}`});throw error}
  if(!response.ok){
    const error=await response.text();
    await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),detail:`action=${action}; status=${response.status}; error=${String(error).slice(0,700)}`});
    // A refusal ends the burst of work: the strip's thin line is the sign that
    // something is happening, and it must not go on for a request that is over.
    findProgressEnd();
    throw new Error(remoteFindReason(error));
  }
  const state=await response.json();
  findProgressPing();
  await remoteFindDiagnostic(record,"navigation_response",{elapsedMs:Math.round(performance.now()-started),completed:Math.min(100000,state.total||0),
    detail:`action=${action}; found=${!!state.found}; complete=${!!state.complete}; position=${state.position||0}/${state.total||0}; dict=${state.target?.dict||""}; article=${state.target?.articleIndex??""}; block=${state.target?.blockStart??""}; occurrences=${state.target?.occurrences?.length||0}; diagnostic_request_ms=${requestLogMs}`});
  return state;
}
async function startRemoteFind(jump=false){
  if(!options.query){
    closeRemoteFind();matches=[];index=-1;docs.clear();restoreExamples();clearPaint();
    stripLayer="answer";renderStrip();
    return;
  }
  const context=remoteFindContext(),selection=serverFindAnchor();
  // A dialog search starts where the reader is; a search from a selection starts
  // at the selection. Both remember it, because a miss downward is retried upward
  // from the same place rather than from the top of the answer.
  const anchor=jump?selection||findAnchor():selection;
  const signature=remoteFindSignature(context)+JSON.stringify(anchor)+(jump?"|jump":"");
  if(remoteFind?.signature===signature){
    stripLayer="found";
    if(jump&&remoteFind.state?.found){const shown=await showRemoteFindTarget(remoteFind.state.target,remoteFind.state,remoteFindToken);if(remoteFind)remoteFind.error=shown;}
    paint();
    return remoteFind.state||null;
  }
  closeRemoteFind();
  if(!context.dictionaries.length||!context.query||!context.mode){remoteFind={signature,state:{complete:true,total:0,found:false}};stripLayer="found";paint();return null}
  const record={id:remoteFindID(),signature,query:options.query,state:null,pending:true,error:"",diagnosticRun:++remoteFindDiagnosticRun,diagnosticStarted:performance.now()};
  remoteFind=record;remoteFindPending=true;stripLayer="found";
  const token=++remoteFindToken;
  matches=[];index=-1;docs.clear();restoreExamples();clearPaint();paint();
  try{
    const size=Math.max(1,Math.min(500,context.pageSize)),firstSize=Math.max(1,Math.min(500,context.firstSize));
    const params={dict:context.dictionaries.join(","),cacheCount:String(context.cacheCount),
      cacheOrders:context.orders.join(","),q:options.query,mode:searchMode(),
      resultQuery:context.query,resultMode:context.mode,ignoreDiacritics:context.ignore?"1":"0",
      matchCase:options.matchCase?"1":"0",examples:options.examples?"1":"0",nearWords:String(nearWords()),
      // prime:"1" - the cursor STANDS on the first match, because Find paints it.
      // The peek start (`prime=0`) leaves the cursor in front of that match, so
      // the reader's first down-arrow only selected the word already on screen:
      // a press that did nothing, and a second press that moved.
      pageSize:String(size),firstSize:String(firstSize),prime:"1",groupID:context.groupID,rungs:context.rungs.join(",")};
    // Down from the reader's place, then up from it: a word above them is not
    // missed for being behind them. Only a miss in both directions is "no such
    // expression", and that is answered by an empty answer, not by an error.
    const first=anchor?.direction||1;
    const passes=jump&&anchor?[first,-first]:[first];
    let state=null;
    for(const direction of passes){
      const request={...params};
      if(anchor){request.anchorDict=anchor.dict;request.anchorArticle=String(anchor.article);request.anchorText=anchor.text;
        request.anchorDirection=String(direction)}
      searchAnchor=null;
      state=await remoteFindFetch(record,"navStart",request);
      if(token!==remoteFindToken||remoteFind!==record)return null;
      if(state.found||!anchor)break;
    }
    // The anchored start can refuse - an article the session does not know, a
    // page that would not open. The whole answer is still worth searching before
    // saying there is nothing, so a refusal is retried without the anchor.
    if(!state?.found&&anchor&&token===remoteFindToken&&remoteFind===record){
      state=await remoteFindFetch(record,"navStart",params);
      if(token!==remoteFindToken||remoteFind!==record)return null;
    }
    record.state=state;record.pendingTarget=!jump&&anchor&&state.found?state:null;record.anchorDirection=anchor?.direction||0;
    record.pending=false;record.error="";
  }catch(error){
    if(token!==remoteFindToken||remoteFind!==record)return null;
    record.state={complete:true,total:0,found:false};record.pending=false;record.error=String(error?.message||error);
  }finally{
    if(token===remoteFindToken&&remoteFind===record){remoteFindPending=false;paint()}
  }
  return record.state;
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
  // An exact count knows the canonical number, so the edge-relative reading
  // stops: otherwise the last match kept reading "1 of 4" once the count closed.
  const shown=!state.complete&&fromEnd>0?fromEnd:Number(state.position)||1;
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
// starting at the node cursor and, in that first node, after the offset the
// previous occurrence ended at. Without the offset a repeated word resolves to
// its first place every time: the counter moves and the mark does not.
function locateFindText(nodes,cursor,occurrence,doc,from=0){
  const want=foldFindText(occurrence.text);
  if(!want)return {range:null,cursor,offset:0};
  for(let i=cursor;i<nodes.length;i++){
    const folded=foldFindText(nodes[i].data),at=folded.indexOf(want,i===cursor?Math.max(0,from):0);
    if(at<0)continue;
    const range=rangeFromNodes(doc,nodes[i],at,nodes[i],at+want.length);
    if(range)return {range,cursor:i,offset:at+want.length};
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
      if(found<0)break;
      let first=null,last=null,firstStart=0,lastEnd=0;
      for(const span of spans){
        if(!first&&found<span.to){first=span.node;firstStart=Math.max(0,found-span.from)}
        if(!last&&found+want.length<=span.to){last=span.node;lastEnd=found+want.length-span.from}
      }
      const range=first&&last?rangeFromNodes(doc,first,firstStart,last,lastEnd):null;
      if(range)return {range,cursor:j,offset:lastEnd};
    }
  }
  return {range:null,cursor,offset:0};
}
function serverFindRanges(root,occurrences){
  const doc=root.ownerDocument||root,win=doc.defaultView;
  if(!win)return [];
  const nodes=findTextNodes(root),out=[];
  // A place in the article is named by its distance in CHARACTERS from the
  // article's first letter, not by the index of a text node: the tree is rebuilt
  // when a block of articles is reloaded, and node indices move with it while
  // the text does not.
  const base=new Array(nodes.length);
  let chars=0;
  for(let index=0;index<nodes.length;index++){base[index]=chars;chars+=nodes[index].data.length}
  let cursor=0,offset=0,missed=0;
  for(const occurrence of occurrences||[]){
    const spans=occurrence?.spans||[];
    if(!spans.length){missed++;continue}
    const first=resolveNodePath(root,spans[0].path),last=resolveNodePath(root,spans[spans.length-1].path);
    let range=first&&last?rangeFromNodes(doc,first,spans[0].start,last,spans[spans.length-1].end):null;
    if(range&&foldFindText(range.toString())!==foldFindText(occurrence.text))range=null;
    if(range){
      const at=nodes.indexOf(range.startContainer);
      if(at>=0){cursor=at;offset=range.startContainer===range.endContainer?range.endOffset:0}
    }else{
      const located=locateFindText(nodes,cursor,occurrence,doc,offset);
      range=located.range;cursor=located.cursor;offset=located.offset;
    }
    if(!range){missed++;continue}
    // Where this range sits in the article's own text order: two positions that
    // paint the same range are the same place on screen, whatever the counter
    // says, and that is exactly the bug worth catching.
    const startAt=nodes.indexOf(range.startContainer),endAt=nodes.indexOf(range.endContainer);
    const spot=startAt>=0&&endAt>=0?`${base[startAt]+range.startOffset}-${base[endAt]+range.endOffset}`:"";
    out.push({range,spot,example:range.startContainer.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")||null});
  }
  if(missed)window.wudictArticleFind?.traceEvent?.("navigation_trace_ranges",`occurrences=${(occurrences||[]).length}; painted=${out.length}; missed=${missed}`);
  return out;
}
/* Is the painted word where the reader can see it? A range can resolve and be
   painted while the viewport stands somewhere else: the block was just
   restored, the article is taller than the screen, or the subtree holds no
   layout yet. The counter follows the server's cursor, so it moves whether or
   not the page did. */
function findHitOnScreen(hit,det){
  if(!hit?.host.isConnected)return false;
  const rect=hit.range.getBoundingClientRect();
  if(!rect.width&&!rect.height)return false;
  /* A closed <details> is skipped in layout rather than zeroed: its content
     answers with a plausible box while nothing of it is drawn. checkVisibility
     at its defaults is not fooled by that. */
  const parent=hit.range.startContainer.parentElement;
  if(parent?.checkVisibility&&!parent.checkVisibility())return false;
  /* A subtree the browser has not laid out (content-visibility:auto) is the
     other way to get a real-looking box over nothing: its place is an estimate
     from contain-intrinsic-size, not the article's own layout. That is why the
     answer may sit "on screen" and still not be there. */
  if(!findHitLaid(hit))return false;
  const top=topOf(hit),bottom=top+rect.height;
  // Beyond the document's end is a box for content the page has not laid out
  // (content-visibility:auto), not a place the reader can be sent to.
  if(top+scrollY>document.documentElement.scrollHeight)return false;
  const {ceiling,floor}=findViewport(det);
  return bottom>ceiling&&top<floor;
}
/* Is the word's own box a laid-out one? Both the folded section and the skipped
   subtree still answer getBoundingClientRect; checkVisibility is what tells them
   apart, and only its contentVisibilityAuto option reads the second case. A mark
   that is merely off screen is laid out - the reveal scrolls to it - so this
   asks about layout, never about position. */
function findHitLaid(hit){
  const parent=hit.range.startContainer.parentElement;
  if(!parent?.checkVisibility)return true;
  return parent.checkVisibility({contentVisibilityAuto:true});
}
/* What is actually at the word's own pixel? A box can be inside the viewport and
   still draw nothing there - covered by the strip, inside a folded section, in a
   subtree that was never laid out. The browser answers "what is at this point"
   better than any rectangle arithmetic, so the reveal believes this and nothing
   else. A frame is asked in its own document: the outer point only ever names
   the frame. Returns null when the browser cannot say. */
function findHitAtPoint(hit){
  const rect=hit.range.getBoundingClientRect();
  const x=rect.left+Math.min(Math.max(rect.width/2,1),24),y=rect.top+rect.height/2;
  /* A frame renders its own document, and the range's rect IS in that document's
     coordinates - so the point is asked there unshifted. Shifting it by the
     frame's own box (which this did first) moved the point outside the frame and
     answered "not visible" for a word the reader could see. A frame the point
     cannot answer for is left to the geometry, which already placed its box. */
  const frame=hit.host.shadowRoot?null:hit.host.contentDocument;
  if(frame){
    if(x<0||y<0||x>=hit.host.clientWidth||y>=hit.host.clientHeight)return null;
    try{
      const inner=frame.elementFromPoint(x,y);
      return inner&&frame.body?.contains(inner)===true?true:null;
    }catch(_){return null}
  }
  if(!(x>=0&&y>=0&&x<innerWidth&&y<innerHeight))return null;
  try{
    const at=document.elementFromPoint(x,y);
    if(!at)return null;
    return hit.host===at||hit.host.contains(at)||at.contains(hit.host);
  }catch(_){return null}
}
/* The band a match may stand in: below the bar and the dictionary's own header,
   above the strip. One place computes it, because the check and the alignment
   disagreeing is how a mark lands under the strip and reads as "not shown". */
function findViewport(det){
  return {ceiling:barH()+(det?.querySelector(":scope>summary")?.offsetHeight||0),
    floor:innerHeight-(strip.hidden?0:strip.offsetHeight)-8};
}
/* Bring the word itself into view, now. The settle loop below refines the place
   over animation frames, and a window that is not painting - occluded,
   minimised, a WebView between loads - runs none of them; then the number would
   move while the page stood still. So the first alignment waits for no frame:
   it scrolls, measures and repeats, and it says whether the word was reached. */
async function revealFindHit(hit,det,token){
  let attempts=0, at=null;
  for(;attempts<12;attempts++){
    if(token!==remoteFindToken)return {shown:false,attempts,at};
    // The block was replaced under us: no scroll can reach it, and the caller
    // re-establishes the target instead of spending its attempts here.
    if(!hit.host.isConnected)return {shown:false,attempts,at};
    at=findHitAtPoint(hit);
    // On screen by the numbers AND at the word's own pixel: a mark that is in
    // the band but covered (the strip, a folded section, a skipped subtree) is
    // not shown, and saying it is was the report this check exists for.
    if(findHitOnScreen(hit,det)&&at!==false)return {shown:true,attempts,at};
    const rect=hit.range.getBoundingClientRect(),parent=hit.range.startContainer.parentElement;
    // A box that draws nothing belongs to a folded section of the article (a
    // wiki table of contents, a folded example). Open the fold that holds the
    // word - the reader asked to see it - and measure again.
    for(let el=parent;el;el=el.parentElement)if(el.tagName==="DETAILS"&&!el.open)el.open=true;
    if(!findHitLaid(hit)){
      /* The browser has not laid the word's part of the article out: its box is
         an estimate, so aiming at a coordinate cannot work. Bringing the
         ARTICLE in is what makes the browser lay the subtree out; the next
         attempt measures the word for real. */
      (hit.host||det)?.scrollIntoView({block:"center",behavior:"instant"});
    }else if(rect.width||rect.height){
      const top=topOf(hit);
      /* A place beyond the document's own end is not a place yet: a scroll to it
         is clamped, so the article is brought in first and measured next. */
      if(top+scrollY>document.documentElement.scrollHeight)hit.host.scrollIntoView({block:"center",behavior:"instant"});
      else revealAt(top,det,attempts>0);
    }else{
      // No box at all: WebView2 has not laid the article out. Scrolling the
      // text's own element in makes it do so, which is what the settle loop's
      // first branch does too - only this one does not wait for a frame.
      (parent?.isConnected?parent:hit.host||det)?.scrollIntoView({block:"center",behavior:"instant"});
    }
    await new Promise(resolve=>setTimeout(resolve,120));
  }
  return {shown:findHitOnScreen(hit,det)&&findHitAtPoint(hit)!==false,attempts,at};
}
/* "" is the word on screen; anything else is the reason it is not. The caller
   puts that reason on the strip's layer, so a click that cannot show its match
   says so instead of moving the counter alone. */
async function showRemoteFindTarget(target,state,token){
  const failed=t("search.fullListUnavailable");
  if(!target)return failed;
  // The answer's own walk and this one share the result list, and a step of
  // either can replace a dictionary's block while the other is still using it.
  // A target that vanished between being painted and being shown is worth
  // re-establishing: the next attempt finds the block again and repaints it.
  for(let attempt=0;attempt<3;attempt++){
    const outcome=await paintRemoteFindTarget(target,state,token);
    if(outcome!=="lost")return outcome==="shown"?"":outcome;
    if(token!==remoteFindToken)return failed;
    await new Promise(resolve=>setTimeout(resolve,250));
  }
  return t("articleFind.showFailed");
}
async function paintRemoteFindTarget(target,state,token){
  const failed=t("search.fullListUnavailable");
  let det=null;
  for(let attempt=0;attempt<50;attempt++){
    det=[...$("out").querySelectorAll("details.dict")].find(node=>node.dataset.searchDict===target.dict);
    if(det)break;
    if(token!==remoteFindToken)return failed;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  if(!det)return failed;
  const blockStart=Number(target.blockStart)||0;
  if((Number(det.dataset.searchPageStart)||0)!==blockStart){
    const replacement=await window.wudictRestoreArticleBlock?.(det,blockStart);
    if(token!==remoteFindToken)return failed;
    if(!replacement)return "lost";
    det=replacement;
  }
  det.open=true;
  const row=det.querySelectorAll(":scope>dl>dd")[Number(target.articleIndex)-blockStart];
  const host=row?.querySelector(".article");
  if(!host)return failed;
  let found=[];
  for(let attempt=0;attempt<50;attempt++){
    const root=rootFor(host);
    if(root)found=serverFindRanges(root,target.occurrences);
    if(found.length||token!==remoteFindToken)break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  if(!found.length||token!==remoteFindToken)return failed;
  matches=found.map(item=>({host,range:item.range,spot:item.spot,example:item.example}));
  remoteFindSettled=Promise.resolve();
  docs=new Set(matches.map(item=>item.range.startContainer.ownerDocument));
  index=Math.max(0,Math.min(Number(target.matchIndex)||0,matches.length-1));
  remoteFind.state=state;remoteFind.target=target;
  dialog.close();strip.hidden=false;measure();
  restoreExamples();showExample(matches[index]);paint();
  // The word itself has to reach the page before the counter may say it did.
  const reveal=await revealFindHit(matches[index],det,token);
  if(token!==remoteFindToken)return failed;
  // What the check saw, in the log: "the counter says it is here and nothing is"
  // is only diagnosable from these numbers.
  if(remoteFind){
    const rect=matches[index].range.getBoundingClientRect(),view=findViewport(det);
    await remoteFindDiagnostic(remoteFind,"navigation_target_reveal",{detail:`dict=${target.dict}; article=${target.articleIndex}; block=${blockStart}; match=${index}; shown=${reveal.shown}; attempts=${reveal.attempts}; at_point=${reveal.at}; laid=${findHitLaid(matches[index])}; top=${Math.round(topOf(matches[index]))}; height=${Math.round(rect.height)}; scroll=${Math.round(scrollY)}; viewport=${innerHeight}; doc=${document.documentElement.scrollHeight}; ceiling=${Math.round(view.ceiling)}; floor=${Math.round(view.floor)}; host=${host.tagName.toLowerCase()}${host.shadowRoot?":shadow":host.contentDocument?":frame":""}; block_open=${det.open}; strip=${strip.hidden?"hidden":Math.round(strip.offsetHeight)}; bar=${Math.round(barH())}`});
  }
  if(!reveal.shown){
    // A step the reader cannot see is recorded as an empty place - the trace's
    // "stuck" count is about the counter, and this is the page standing still.
    remoteFindSpot="";
    if(!host.isConnected||!det.isConnected)return "lost";
    return t("articleFind.showFailed");
  }
  remoteFindSpot=matches[index]?.spot?`${target.dict}:${target.articleIndex}:${matches[index].spot}`:"";
  // A run waits for this before it steps again: otherwise every step cancels the
  // previous scroll and the numbers move while the page stands still.
  remoteFindSettled=new Promise(resolve=>{
    let finished=false;
    const finish=()=>{if(!finished){finished=true;resolve()}};
    settleOn(matches[index],det,finish);
    setTimeout(finish,1500);
  });
  return "shown";
}
async function moveRemoteFind(step){
  if(!remoteFind?.id||remoteFindPending)return false;
  remoteFindPending=true;const record=remoteFind,token=remoteFindToken,request=++remoteFindRequestSeq;
  try{
    if(record.pendingTarget){
      const target=record.pendingTarget;record.pendingTarget=null;
      if(step===record.anchorDirection){
        const paintStarted=performance.now();
        record.error=await showRemoteFindTarget(target.target,target,token);
        await remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${target.target?.dict||""}; article=${target.target?.articleIndex??""}; block=${target.target?.blockStart??""}; position=${target.position||0}/${target.total||0}; success=${!record.error}`});
        return !record.error;
      }
    }
    const state=await remoteFindFetch(record,step<0?"navPrevious":"navNext");
    if(request!==remoteFindRequestSeq||token!==remoteFindToken||remoteFind!==record)return;
    if(state.found){record.state=state;const paintStarted=performance.now();record.error=await showRemoteFindTarget(state.target,state,token);
      await remoteFindDiagnostic(record,"navigation_target_painted",{elapsedMs:Math.round(performance.now()-paintStarted),detail:`dict=${state.target?.dict||""}; article=${state.target?.articleIndex??""}; block=${state.target?.blockStart??""}; position=${state.position||0}/${state.total||0}; success=${!record.error}`})}
    else /* A step that found nothing is the end of the track, not a lost answer:
          the reader still stands on the match they were looking at, and only
          the count's closure changes. Turning `found` off here emptied the
          strip into "no matches" and re-enabled both arrows at the very end. */
      record.state={...record.state,...state,found:!!record.state?.target};
    return !!state.found&&!record.error;
  }catch(error){record.error=String(error?.message||error);await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`action=move; error=${record.error.slice(0,700)}`});return false}
  finally{if(request===remoteFindRequestSeq&&token===remoteFindToken){remoteFindPending=false;paint()}}
}
/* The trace of the search inside the found articles. It is the same walk the
   answer's layer gets - from either edge, then jumping around - because the two
   layers are the same server cursor over the same articles, and the bugs are
   the same: a step that lands where it was, an edge that is not the edge, two
   positions that call themselves one. Every position is recorded with the
   target it stood on, and the runs verify each other at the end. */
async function runRemoteFindTrace(mode){
  const record=remoteFind;
  if(!record?.id||remoteFindPending||remoteFindTraceActive||!record.state?.found)return;
  const run=mode||"forward";
  remoteFindTraceActive=true;remoteFindTraceStop=false;record.traceRun=++remoteFindTraceRun;
  record.traceStarted=performance.now();
  const origin=Number(record.state.position)||0;
  record.spotRuns={};
  syncRemoteFindTraceButton();
  const summaries=[];
  await remoteFindDiagnostic(record,"navigation_trace_start",{detail:`trace=${record.traceRun}; source=found_articles; run=${run}; position=${origin}; total=${record.state.total||0}`});
  try{
    for(const name of run==="full"?["forward","backward","seeks"]:[run]){
      if(remoteFindTraceStop)break;
      summaries.push(await runFoundTraceRun(record,name));
    }
    // A diagnostic leaves the reader where it found them.
    if(origin&&!remoteFindTraceStop&&remoteFind===record)await gotoRemoteFind(origin);
  }catch(error){
    await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; source=found_articles; error=${String(error?.message||error).slice(0,700)}`});
  }finally{
    const forward=summaries.find(summary=>summary.run==="forward");
    const backward=summaries.find(summary=>summary.run==="backward");
    if(forward&&backward&&forward.sequence.length&&backward.sequence.length){
      const left=[...forward.sequence].reverse(),right=backward.sequence;
      let divergence=-1;
      for(let index=0;index<Math.min(left.length,right.length);index++){
        if(left[index]!==right[index]){divergence=index;break}
      }
      const agreement=divergence<0&&left.length===right.length?"same":
        divergence<0?"prefix":"at "+(divergence+1)+": "+left[divergence]+"≠"+right[divergence];
      let retargeted=0,repainted=0;
      for(const [position,at] of Object.entries(forward.atOf)){
        if(backward.atOf[position]&&backward.atOf[position]!==at)retargeted++;
      }
      const forwardSpots=record.spotRuns?.forward||{},backwardSpots=record.spotRuns?.backward||{};
      for(const [position,spot] of Object.entries(forwardSpots)){
        if(backwardSpots[position]&&backwardSpots[position]!==spot)repainted++;
      }
      await remoteFindDiagnostic(record,"navigation_trace_compare",{detail:`trace=${record.traceRun}; source=found_articles; suite=${run}; forward=${left.length}; backward=${right.length}; agreement=${agreement}; retargeted=${retargeted}; repainted=${repainted}`});
    }
    const totals=summaries.map(summary=>`${summary.run}: steps=${summary.steps} p50=${summary.p50} p90=${summary.p90} max=${summary.max} elapsed_ms=${summary.elapsed} reason=${summary.reason}`).join(" | ");
    const position=Number(record.state?.position)||0;
    await remoteFindDiagnostic(record,"navigation_trace_end",{completed:Math.min(100000,summaries.reduce((sum,summary)=>sum+summary.steps,0)),total:Math.min(100000,record.state?.total||0),detail:`trace=${record.traceRun}; source=found_articles; suite=${run}; position=${position}`});
    await remoteFindDiagnostic(record,"navigation_trace_summary",{completed:Math.min(100000,summaries.reduce((sum,summary)=>sum+summary.steps,0)),detail:`trace=${record.traceRun}; source=found_articles; suite=${run}; ${totals||"no runs"}; position=${position}`});
    remoteFindTraceActive=false;remoteFindTraceStop=false;syncRemoteFindTraceButton();paint();
  }
}
function remoteFindTargetAt(){
  const target=remoteFind?.state?.target;
  return target?[target.dict,target.articleIndex,target.matchIndex,target.blockStart].join(":"):"";
}
async function runFoundTraceRun(record,run){
  const started=performance.now(),timings=[],visited=[],landed={};
  let steps=0,reason="done";
  await remoteFindDiagnostic(record,"navigation_trace_run_start",{detail:`trace=${record.traceRun}; source=found_articles; run=${run}; position=${record.state?.position||0}; total=${record.state?.total||0}`});
  try{
    if(run==="forward"||run==="backward"){
      await (run==="forward"?remoteFindEdge(1):remoteFindEdge(-1));
      if(remoteFind!==record){reason="search_changed"}
      /* An edge that could not be shown is a failure of the RUN, not the end of
         the answer: without this the walk started from wherever the cursor was
         and reported "end" with no steps, which reads as "there was nothing". */
      else if(record.error)reason="error";
      let previous=Number(record.state?.position)||0;
      if(previous)visited.push({p:previous,at:remoteFindTargetAt(),spot:remoteFindSpot});
      const direction=run==="forward"?1:-1;
      while(!remoteFindTraceStop&&remoteFind===record&&reason==="done"&&steps<100000){
        const state=record.state||{},position=Number(state.position)||0,total=Number(state.total)||0;
        if(direction>0&&state.complete&&total>0&&position>=total)break;
        if(direction<0&&position<=1)break;
        const stepStarted=performance.now();
        const moved=direction>0?await moveRemoteFind(1):await moveRemoteFind(-1);
        if(remoteFind!==record){reason="search_changed";break}
        if(!moved){reason=record.error?"error":"end";break}
        const next=Number(record.state?.position)||0;
        if(!next||next===position){reason="end";break}
        // The walk does not wrap for a reader; a step that lands back where it
        // began is the walk finding the end and is kept out of the record.
        if(direction>0&&record.state?.complete&&next<=previous){reason="end";break}
        steps++;timings.push(performance.now()-stepStarted);
        visited.push({p:next,at:remoteFindTargetAt(),spot:remoteFindSpot});
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:Math.min(100000,steps),total:Math.min(100000,total),detail:`trace=${record.traceRun}; source=found_articles; run=${run}; step=${steps}; from=${position}; to=${next}; complete=${!!record.state?.complete}; at=${remoteFindTargetAt()}; spot=${remoteFindSpot}; step_ms=${Math.round(performance.now()-stepStarted)}`});
        previous=next;
        await remoteFindSettled;
        await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
      }
    }else{
      const total=Number(record.state?.total)||0;
      const positions=traceSeekPositions(total);
      const landing=Math.max(1,Math.min(total||1,positions[1]||1));
      const step=async(label,action)=>{
        const stepStarted=performance.now();
        await action?.();
        const position=Number(record.state?.position)||0;
        steps++;timings.push(performance.now()-stepStarted);
        visited.push({p:position,at:remoteFindTargetAt(),spot:remoteFindSpot});
        landed[label]=position;
        await remoteFindDiagnostic(record,"navigation_trace_step",{completed:Math.min(100000,steps),total:Math.min(100000,total),detail:`trace=${record.traceRun}; source=found_articles; run=jumps; step=${steps}; ${label}; to=${position}; at=${remoteFindTargetAt()}; spot=${remoteFindSpot}; step_ms=${Math.round(performance.now()-stepStarted)}`});
      };
      await gotoRemoteFind(landing);
      if(Number(record.state?.position))visited.push({p:Number(record.state.position),at:remoteFindTargetAt()});
      const stepSettled=async action=>{await action;await remoteFindSettled;};
      for(let index=0;index<3;index++)await step("down",()=>stepSettled(moveRemoteFind(1)));
      await step("last",()=>stepSettled(remoteFindEdge(-1)));
      for(let index=0;index<3;index++)await step("up",()=>stepSettled(moveRemoteFind(-1)));
      await step("first",()=>stepSettled(remoteFindEdge(1)));
      const finalTotal=Number(record.state?.total)||0;
      landed.edges=(landed.first===1?"first=ok":"first="+landed.first)+" "+
        (!record.state?.complete?"last=provisional":
          landed.last===finalTotal?"last=ok":"last="+landed.last+"/"+finalTotal);
    }
  }catch(error){
    reason="error";
    await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; source=found_articles; run=${run}; error=${String(error?.message||error).slice(0,700)}`});
  }
  if(remoteFindTraceStop)reason="stopped";
  const seen=new Map();
  let revisited=0,mismatched=0;
  for(const visit of visited){
    if(!visit.p||!visit.at)continue;
    if(seen.has(visit.p)){revisited++;if(seen.get(visit.p)!==visit.at)mismatched++;}
    else seen.set(visit.p,visit.at);
  }
  const sequence=visited.map(visit=>visit.p);
  const spots=visited.filter(visit=>visit.spot).map(visit=>visit.spot);
  // "stuck" is the counter moving while the mark does not: two positions in a
  // row painted on the very same range.
  let stuck=0;
  for(let index=1;index<visited.length;index++){
    if(visited[index].spot&&visited[index].spot===visited[index-1].spot)stuck++;
  }
  const ordered=run==="backward"?sequence.every((p,index)=>!index||p<sequence[index-1]):
    run==="forward"?sequence.every((p,index)=>!index||p>sequence[index-1]):true;
  const digest=sequence.length<=40?sequence.join(","):sequence.slice(0,10).join(",")+",…,"+sequence.slice(-10).join(",");
  await remoteFindDiagnostic(record,"navigation_trace_positions",{completed:Math.min(100000,steps),total:Math.min(100000,record.state?.total||0),detail:`trace=${record.traceRun}; source=found_articles; run=${run}; count=${sequence.length}; ordered=${ordered}; revisited=${revisited}; mismatched=${mismatched}; spots=${new Set(spots).size}; stuck=${stuck}; ${run==="seeks"?(landed.edges||"edges=none")+"; ":""}seq=${digest}`});
  record.spotRuns=record.spotRuns||{};
  record.spotRuns[run]=Object.fromEntries(visited.filter(visit=>visit.spot).map(visit=>[visit.p,visit.spot]));
  const summary={run,steps,elapsed:Math.round(performance.now()-started),reason,
    p50:Math.round(tracePercentile(timings,.5)),p90:Math.round(tracePercentile(timings,.9)),
    max:Math.round(timings.length?Math.max(...timings):0),
    sequence,atOf:Object.fromEntries(seen)};
  await remoteFindDiagnostic(record,"navigation_trace_run_end",{completed:Math.min(100000,steps),total:Math.min(100000,record.state?.total||0),detail:`trace=${record.traceRun}; source=found_articles; run=${run}; steps=${steps}; p50=${summary.p50}; p90=${summary.p90}; max=${summary.max}; elapsed_ms=${summary.elapsed}; reason=${reason}`});
  return summary;
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
  // Two positions in a row on the same target: the counter moved and the mark
  // did not. The target here carries the match's own index, so this catches a
  // repeated mark as well as a repeated article.
  let stuck=0;
  for(let i=1;i<visited.length;i++){
    if(visited[i].at&&visited[i].at===visited[i-1].at)stuck++;
  }
  const ordered=run==="backward"?sequence.every((p,i)=>!i||p<sequence[i-1]):
    run==="forward"?sequence.every((p,i)=>!i||p>sequence[i-1]):true;
  const digest=sequence.length<=40?sequence.join(","):sequence.slice(0,10).join(",")+",…,"+sequence.slice(-10).join(",");
  await remoteFindDiagnostic(record,"navigation_trace_positions",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; run=${run}; count=${sequence.length}; ordered=${ordered}; revisited=${revisited}; mismatched=${mismatched}; stuck=${stuck}; ${run==="seeks"?(landed.edges||"edges=none")+"; ":""}seq=${digest}`});
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
  try{record.error=await showRemoteFindTarget(target,state,token)}
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
      record.error=await showRemoteFindTarget(state.target,state,token);
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
    if(state.found){record.state=state;const paintStarted=performance.now();record.error=await showRemoteFindTarget(state.target,state,token);
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
function settleOn(hit, det, done) {
  if (settle) settle("superseded");
  let moves = 0, stable = 0, pending = 0, frame = 0, observer = null, stopped = false;
  const quit = ["wheel","touchstart","pointerdown","keydown"];
  /* Every exit names itself in the trace. A settle that ends with the word off
     screen is the defect behind "the counter moved and the page did not", and
     the log should say which guard ended it rather than leave it to a guess. */
  const stopSettle = (reason = "done") => {
    if (stopped) return;
    stopped = true;
    clearTimeout(pending); clearTimeout(timeout); cancelAnimationFrame(frame);
    observer?.disconnect();
    for (const event of quit) removeEventListener(event,onQuit);
    if (settle === stopSettle) settle = null;
    window.wudictArticleFind?.traceEvent?.("navigation_trace_settle",
      `source=found_articles; stop=${reason}; moves=${moves}; stable=${stable}; on_screen=${findHitOnScreen(hit,det)}`,0);
    if (done) done();
  };
  const onQuit = () => stopSettle("reader");
  const align = () => {
    if (!active) return stopSettle("inactive");
    // A newer target took over: its own settle follows and this one is over.
    if (matches[index] !== hit) return stopSettle("superseded");
    if (!hit.host.isConnected) return stopSettle("replaced");
    const rect = hit.range.getBoundingClientRect(), laidOut = !!(rect.width || rect.height);
    const top = topOf(hit), bottom = top + rect.height;
    const {ceiling,floor} = findViewport(det);
    // Opening the target dictionary can move it again as earlier sections
    // finish laying out. Keep checking until the whole result list settles.
    if (laidOut && top >= ceiling && top < floor && Math.min(bottom,top+24) <= floor) {
      if (++stable >= 8) return stopSettle("aligned");
    } else {
      stable = 0;
      if (++moves > 30) return stopSettle("not_settled");
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
  for (const event of quit) addEventListener(event,onQuit,{passive:true});
  const timeout = setTimeout(() => stopSettle("timeout"),6500);
  settle = stopSettle;
  frame = requestAnimationFrame(align);
}
function go(step) {
  // A step belongs to the search the strip is showing. A different query in the
  // field is another search - the session that is alive answered the old one.
  if(remoteFind&&remoteFind.query===options.query){void moveRemoteFind(step);return}
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
/* The strip's close button belongs to the search layer alone: it ends the
   dialog's search over the found articles and hands the strip back to the
   answer underneath. With no answer - an ordinary headword search - there is
   nothing to fall back to, and the strip hides. */
function closeLayer() {
  closeRemoteFind();
  active=false; clearTimeout(timer); restoreExamples(); clearPaint(); matches=[]; index=-1; docs.clear();
  options = {...options, query:""};
  $("articleFindQuery").value = "";
  stripLayer = "answer";
  renderStrip();
  if (dialog.open) renderFindHistory();
}
/* The page calls this when the answer itself is over - a new search, cleared
   results: everything the find UI holds goes with it, the walk included. */
function stop() {
  closeRemoteFind();
  const navigation = fullTextNavigation;
  fullTextNavigation = null;
  if (settle) settle();
  active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1;
  docs.clear();
  options = {...options, query:""};
  $("articleFindQuery").value = "";
  stripLayer = "answer";
  strip.hidden = true; measure();
  if (dialog.open) dialog.close();
  navigation?.close?.();
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
function schedule() { clearTimeout(timer); timer = setTimeout(() => { if (dialog.open) syncControls(); },100); }
function measure() {
  document.documentElement.style.setProperty("--article-find-height", strip.hidden ? "0px" : strip.offsetHeight+"px");
}
$("sbarFind").onclick = open;
$("articleFindLabel").onclick = () => { if(!dialog.open) open(); };
$("articleFindCount").onclick = gotoOpen;
$("articleFindClose").onclick = () => dialog.close();
$("articleFindStop").onclick = closeLayer;
$("articleFindFirst").onclick = () => { if(stripLayer==="found"){void remoteFindEdge(1);return} fullTextNavigation?.first?.(); };
$("articleFindPrev").onclick = () => { if(stripLayer==="found"){void moveRemoteFind(-1);return} fullTextNavigation?.previous?.(); };
$("articleFindNext").onclick = () => { if(stripLayer==="found"){void moveRemoteFind(1);return} fullTextNavigation?.next?.(); };
$("articleFindLast").onclick = () => { if(stripLayer==="found"){void remoteFindEdge(-1);return} fullTextNavigation?.last?.(); };
$("articleFindScrollToMatch").onclick = () => {
  if(stripLayer==="found"){if(remoteFind)void restoreRemoteFindTarget();return}
  fullTextNavigation?.restore?.();
};
$("articleFindTrace").onclick = () => {
  if(!remoteFindTraceAvailable())return;
  if(remoteFindTraceActive){remoteFindTraceStop=true;return}
  if(stripLayer==="found"){void runRemoteFindTrace("forward");return}
  void runFullTextTrace("forward");
};
$("articleFindTraceRun").addEventListener("change",()=>{
  const select=$("articleFindTraceRun"),run=select.value;
  if(!remoteFindTraceAvailable()){select.value="";return}
  select.value="";
  window.wudictThemedSelects?.articleFindTraceRun?.sync();
  if(remoteFindTraceActive)return;
  if(stripLayer==="found"){void runRemoteFindTrace(run||"forward");return}
  if(!run)return;
  void runFullTextTrace(run);
});
$("sysLogSearch")?.addEventListener("change",syncRemoteFindTraceButton);
// One action: the dialog finds, then hands the search to the strip - the arrows
// there step, jump to the ends and close it. A second pair of arrows in a window
// that closes on use was one set too many.
async function findFromDialog(){
  const status=$("articleFindStatus");
  // Everything here is guarded: a Find that cannot run has to say why in the
  // dialog's own line instead of stopping on a line nobody sees. The log is a
  // courtesy too - a diagnostic that fails must not stop the search.
  const note=(stage,detail)=>{
    try{void sendBrowseDiagnostic({event:"search_article",stage,runId:0,query:String(options.query||"").slice(0,200),detail})}catch(_){}
  };
  try{
    status.textContent="…";
    readOptions(); active=!!options.query;
    const anchor=findAnchor();
    note("navigation_find_click",`query=${options.query}; active=${active}; pending=${remoteFindPending}; anchor=${anchor?anchor.dict+":"+anchor.article:"none"}`);
    if(!active){dialog.close();return}
    closeRemoteFind();
    let state=null,error="";
    try{state=await startRemoteFind(true)}
    catch(failure){
      error=String(failure?.message||failure);
      if(remoteFind)remoteFind.error=error;
    }
    note("navigation_find_result",`found=${!!state?.found}; complete=${!!state?.complete}; total=${state?.total||0}; position=${state?.position||0}; error=${(error||remoteFind?.error||"").slice(0,300)}`);
    if(error||!state){status.textContent="⚠ "+(error||t("search.fullListUnavailable"));return}
    // Found: the dialog has done its job - before the article is painted,
    // because painting waits for a block of articles and the window must not
    // wait with it. Not found: it stays and says so in place.
    if(state.found){
      dialog.close();
      remoteFindSpot="";
      const shown=await showRemoteFindTarget(state.target,state,remoteFindToken);if(shown)remoteFind.error=shown;
      paint();
    }
  }catch(failure){
    status.textContent="⚠ "+String(failure?.message||failure);
  }
}
$("articleFindGo").onclick = () => { void findFromDialog(); };
$("articleFindStopFind").onclick = () => {
  closeRemoteFind();stripLayer="answer";renderStrip();syncControls();
};
$("articleFindWholeWords").onchange = () => {
  readOptions(); syncControls(); save();
  if (dialog.open) { active = !!options.query; if (active) rebuild(); }
};
for (const id of ["articleFindCase","articleFindAll","articleFindExamples"]) $(id).onchange = () => { readOptions(); index = -1; active = !!options.query; rebuild(); };
$("articleFindQuery").onfocus = () => { if(!suppressFindHistoryFocus)renderFindHistory(); };
$("articleFindQuery").oninput = () => {
  readOptions(); renderFindHistory(); index=-1; active=!!options.query;
  // Typing prepares the query. The search runs when the reader says Find, so an
  // abandoned word costs nothing and nothing is in flight behind the button.
  if(active)syncControls(); else rebuild();
};
 dialog.addEventListener("keydown", e => {
  const list=$("articleFindHistory"),input=$("articleFindQuery");
  if(e.key==="Escape"&&!list.hidden){e.preventDefault();e.stopPropagation();hideFindHistory();input.focus();return}
  if(e.key==="ArrowDown"&&e.target===input&&!list.hidden){e.preventDefault();list.firstElementChild?.focus();return}
  if((e.key==="ArrowDown"||e.key==="ArrowUp")&&e.target.matches?.('#articleFindHistory [role="option"]')){
    e.preventDefault();const rows=[...list.querySelectorAll('[role="option"]')],at=rows.indexOf(e.target),next=at+(e.key==="ArrowDown"?1:-1);
    if(rows[next])rows[next].focus();else if(e.key==="ArrowUp")input.focus();return;
  }
  // Enter is the Find button: Shift+Enter still steps backwards when the
  // reader knows they overshot.
  // Enter is the Find button; Shift+Enter steps the search that is already live.
  if (e.key === "Enter" && e.target.tagName !== "BUTTON") {
    e.preventDefault();
    if (e.shiftKey) { readOptions(); active=!!options.query; if (active) go(-1); dialog.close(); return; }
    $("articleFindGo").click();
  }
});
dialog.addEventListener("cancel",e=>{if(!$("articleFindHistory").hidden){e.preventDefault();hideFindHistory();$("articleFindQuery").focus()}});
document.addEventListener("pointerdown",e=>{if(!$("articleFindQueryBox").contains(e.target))hideFindHistory()});
dialog.addEventListener("close", () => {
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
}, find:()=>{void findFromDialog()}, progressPing:findProgressPing, progressEnd:findProgressEnd, fullTextModeActive:()=>stripLayer==="answer"&&!!fullTextNavigation, isActive:()=>active&&!!options.query, searchAcrossBlocks:()=>true, searchFrom, goto, setHistoryLimit:n=>{findHistory=findHistory.slice(0,Math.max(0,Math.min(1000,Number(n)||0)));saveFindHistory();renderFindHistory()}};
})();
