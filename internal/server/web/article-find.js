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
const examinedBlocks = new Map();
function resetCounts(){examinedBlocks.clear()}
function blockPages(det){
  return [...(examinedBlocks.get(det.dataset.searchDict||"")||new Map())]
    .map(([start,count])=>({start,count})).sort((a,b)=>a.start-b.start);
}
function recordCounts(){
  const counts=new Map();
  for(const hit of matches){
    const det=hit.host.closest("details.dict");
    if(det)counts.set(det,(counts.get(det)||0)+1);
  }
  for(const det of $("out").querySelectorAll("details.dict")){
    const hosts=[...det.querySelectorAll(".article")];
    if(!hosts.length||hosts.some(host=>!rootFor(host))||
       hosts.some(host=>host.matches("iframe")&&host.contentDocument?.readyState!=="complete"))continue;
    if(options.mode==="wordLemma"&&hosts.some(host=>lemmaLookups.get(host.dataset.dict+"\u0000"+options.query)?.value===null))continue;
    const id=det.dataset.searchDict||"";
    if(!examinedBlocks.has(id))examinedBlocks.set(id,new Map());
    examinedBlocks.get(id).set(Number(det.dataset.searchPageStart)||0,counts.get(det)||0);
  }
}
/* The dictionary's own result count, from the same place the plate reads it:
   dataset.searchTotal is the result cache's running total - the background
   counting pass raises it - and dataset.searchExact is the flag that closes it,
   the one that makes the plate's number exact. Until it closes, the stored
   number is the plate's own lower bound. */
function dictionaryCount(det){
  const total=Number(det.dataset.searchTotal)||0;
  return det.dataset.searchExact==="true"?total:Math.max(total,Number(det.dataset.searchLowerBound)||0);
}
/* Two numbers, because the dialog counts matches and the plate counts articles.
   COUNTED is the matches of the blocks the dialog examined: the rank it walks,
   and what Go to match can reach. The displayed total is that floored by the
   dictionary's count - every article the dictionary matched holds at least one
   match of the query, so the answer cannot hold fewer matches than the
   dictionary says - and the dialog's own number overtakes the floor as its
   blocks are examined. The floor belongs only to a dictionary the dialog
   counted; one it skipped (a lemma it could not get, articles not ready)
   contributes nothing, or the two sides would count different dictionaries. */
function countedPosition(){
  const det=matches[index]?.host.closest("details.dict");
  const dictionaries=[...$("out").querySelectorAll("details.dict")];
  let counted=0,current=0,shown=0;
  for(const section of dictionaries){
    const start=Number(section.dataset.searchPageStart)||0,id=section.dataset.searchDict||"";
    let here=0;
    for(const page of blockPages(section)){
      if(section===det&&page.start===start){
        current=counted+matches.slice(0,index+1).filter(hit=>hit.host.closest("details.dict")===det).length;
      }
      counted+=page.count;here+=page.count;
    }
    shown+=Math.max(here,examinedBlocks.has(id)?dictionaryCount(section):0);
  }
  return {current,counted,total:shown};
}
let settle = null;
let laidOutList = null;
const fallbackSelections = new Map();
const watched = new WeakSet();
const lemmaLookups = new Map();
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
  const previous=options;
  options = {query:$("articleFindQuery").value.trim(), mode:$("articleFindMode").value,
    matchCase:$("articleFindCase").checked, all:$("articleFindAll").checked,
    examples:$("articleFindExamples").checked};
  if(previous.query!==options.query||previous.mode!==options.mode||previous.matchCase!==options.matchCase||previous.examples!==options.examples)resetCounts();
  save();
}
function fillOptions() {
  $("articleFindQuery").value = options.query;
  if(options.mode==="exact")options.mode="word"; // saved by the earlier article-search picker
  options.mode = ["prefix","suffix","contains","word","wordLemma","phrase","near"].includes(options.mode) || options.mode === "fts" && fullTextNavigation ? options.mode : "contains";
  $("articleFindMode").value = options.mode;
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
}
function open(preserveAnchor=false) {
  if (!preserveAnchor) searchAnchor = null;
  /* The words reading navigates the live full-text answer, so it is filled from
     that answer whenever the dialog opens; the two word readings do it when the
     search that ran IS that reading - the dialog opened from the mode, the way
     [A … B] has always been filled. */
  if (fullTextNavigation && (options.mode === "fts" || options.mode === fullTextNavigation.reading))
    options = {...options, query:fullTextNavigation.query};
  fillOptions();
  active = options.mode !== "fts" && !!options.query;
  if (active&&!searchAnchor) rebuild();
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
  $("articleFindHiddenHint").hidden = fts || !options.query || !!matches.length || options.examples || !hiddenExamples;
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
  if(matches.length>=2)return true;
  if(!matches.length)return countedPosition().counted>0 || options.blocks!==false &&
    [...$("out").querySelectorAll("details.dict")].some(det=>window.wudictArticleBlockHasMore?.(det,step));
  const hit=matches[index>=0?index:0];
  return !!(options.query&&hit&&options.blocks!==false&&window.wudictArticleBlockHasMore?.(hit.host.closest("details.dict"),step));
}
function lemmaFor(host,query){
  if(options.mode!=="wordLemma"||/\s/u.test(query))return query;
  const id=host.dataset.dict;
  if(!id)return query;
  const key=id+"\u0000"+query;
  let record=lemmaLookups.get(key);
  if(!record){
    record={value:null};lemmaLookups.set(key,record);
    if(lemmaLookups.size>500)lemmaLookups.delete(lemmaLookups.keys().next().value);
    record.pending=fetch("/api/article-find/lemma?"+new URLSearchParams({dict:id,q:query}))
      .then(response=>response.ok?response.json():null)
      .then(data=>{
        record.value=typeof data?.lemma==="string"&&data.lemma?data.lemma:query;
        examinedBlocks.delete(id);
        if(active&&options.mode==="wordLemma"&&options.query===query)schedule();
      }).catch(()=>{record.value=query});
  }
  return record.value||query;
}
function escapeRE(value){return value.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")}
/* Phrase and near are not one pattern but a rule over the WORDS of the query -
   [A B] the words adjacent and in this order, [A ↔ B] the words within the
   window and in any order - so they are matched on the words of the block
   instead of by a regular expression. */
const wordRun=/[\p{L}\p{M}\p{N}_]+/gu;
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
function wordKey(word){return options.matchCase?word:word.toLowerCase()}
function wordReading(){return options.mode==="phrase"||options.mode==="near"}
function matcherFor(host,query){
  if(wordReading())return {words:(query.match(wordRun)||[]).map(wordKey),bound:options.mode==="near"?nearWords():0};
  const alternatives=[query];
  if(options.mode==="wordLemma"&&/^\S+$/u.test(query))alternatives.push(lemmaFor(host,query));
  const unique=[...new Set(alternatives)].sort((a,b)=>b.length-a.length);
  return {regex:new RegExp(unique.map(escapeRE).join("|"),options.matchCase?"gu":"giu")};
}
function paint() {
  syncRemoteFindTraceButton();
  clearPaint();
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
  const position=countedPosition();
  const partial = [...$("out").querySelectorAll("details.dict")].some(det=>{
    if(det.dataset.searchExact!=="true")return true;
    const total=Number(det.dataset.searchTotal)||0;
    const size=Number(det.dataset.searchPageSize)||20,first=Number(det.dataset.searchFirstPageSize)||size;
    const expected=total>0?1+Math.ceil(Math.max(0,total-first)/size):0;
    return (examinedBlocks.get(det.dataset.searchDict||"")?.size||0)<expected;
  });
  const total = String(position.total)+(partial?"+":"");
  const block = index >= 0 ? window.wudictArticleFindBlockRange?.(matches[index]?.host.closest("details.dict")) : "";
  const count = position.counted ? t("articleFind.position", {current:position.current, total})+(block?" · "+block:"") : t("articleFind.noMatches");
  if(blockNavigationPending)return;
  $("articleFindCount").textContent = count;
  $("articleFindCount").disabled = !position.counted;
  $("articleFindStatus").textContent = options.query ? count : "";
  $("articleFindHiddenHint").hidden = !options.query || !!matches.length || options.examples || !hiddenExamples;
  $("articleFindLabel").textContent = options.query;
  $("articleFindLabel").title = options.query;
  $("articleFindPrev").disabled = !canStep(-1);
  $("articleFindNext").disabled = !canStep(1);
  $("articleFindDialogPrev").disabled = !canStep(-1);
  $("articleFindDialogNext").disabled = !canStep(1);
  $("articleFindScrollToMatch").disabled = options.mode === "fts" ? !fullTextNavigation?.canRestore : !matches.length || index < 0;
}
let hiddenExamples = false;
// Build a text stream per paragraph, retaining offsets across inline formatting.
// Whitespace is folded so phrases also match line breaks and nonbreaking spaces.
function scan(host, root, finder) {
  const doc = root.ownerDocument, win = doc.defaultView;
  if (!win) return;
  const regex = finder.regex, words = finder.words, bound = finder.bound;
  docs.add(doc);
  if (!root.querySelector("style[data-wudict-find]")) {
    const style = doc.createElement("style"); style.dataset.wudictFind = "";
    style.textContent = highlightCSS; root.appendChild(style);
  }
  if (!watched.has(root)) {
    watched.add(root);
    root.addEventListener("load",schedule,true);
    new MutationObserver(() => { if (active) schedule(); }).observe(root, {childList:true, subtree:true, characterData:true});
  }
  const walker = doc.createTreeWalker(root, win.NodeFilter.SHOW_TEXT | win.NodeFilter.SHOW_ELEMENT);
  let text = "", points = [], group = null;
  function addMatch(start,end) {
    const a = points[start], b = points[end-1];
    if (!a || !b) return;
    const range = doc.createRange(); range.setStart(a.node,a.start); range.setEnd(b.endNode,b.end);
    matches.push({host,range,example:a.node.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex") || null});
  }
  function flush() {
    if (words) flushWords(); else flushPattern();
    text = ""; points = [];
  }
  function flushPattern() {
    regex.lastIndex = 0;
    for (let found; (found = regex.exec(text)); ) {
      const start = found.index, end = start+found[0].length;
      const before = Array.from(text.slice(0,start)).pop() || "", after = Array.from(text.slice(end))[0] || "";
      if (["prefix","word","wordLemma"].includes(options.mode) && wordChar.test(before)) continue;
      if (["suffix","word","wordLemma"].includes(options.mode) && wordChar.test(after)) continue;
      addMatch(start,end);
    }
  }
  /* The two word readings, over the words of this block. A word is a run of word
     characters, so whatever stands between two of them - a space, a line break,
     a comma - separates them without joining them, and a mark covers the span
     from the first word of the match to the last: the phrase as it was typed,
     or the window `near` counts. */
  function flushWords() {
    if (!words.length) return;
    const tokens = [];
    wordRun.lastIndex = 0;
    for (let found; (found = wordRun.exec(text)); ) tokens.push({start:found.index,end:found.index+found[0].length,key:wordKey(found[0])});
    if (options.mode === "phrase") {
      for (let i = 0; i + words.length <= tokens.length; i++) {
        let joined = true;
        for (let j = 0; joined && j < words.length; j++) joined = tokens[i+j].key === words[j];
        if (!joined) continue;
        addMatch(tokens[i].start,tokens[i+words.length-1].end);
        i += words.length-1;
      }
      return;
    }
    /* near: every word of the query, with the multiplicity it was typed with,
       inside one window no wider than the bound - measured from the first word
       of the window to the word that completes it, the way the hint counts. */
    const need = new Map();
    for (const word of words) need.set(word,(need.get(word)||0)+1);
    for (let i = 0; i < tokens.length; i++) {
      if (!need.has(tokens[i].key)) continue;
      const held = new Map([[tokens[i].key,1]]);
      let left = words.length-1, last = i;
      for (let j = i+1; left && j < tokens.length && j-i <= bound+1; j++) {
        const key = tokens[j].key, have = held.get(key)||0;
        if (have >= (need.get(key)||0)) continue;
        held.set(key,have+1); left--; last = j;
      }
      if (left) continue;
      addMatch(tokens[i].start,tokens[last].end);
      i = last;
    }
  }
  for (let node; (node = walker.nextNode()); ) {
    if (node.nodeType === 1) {
      if (node.tagName === "BR" && text && !text.endsWith(" ")) {
        const last = points[points.length-1];
        text += " "; points.push({node:last.endNode,endNode:last.endNode,start:last.end,end:last.end});
      }
      continue;
    }
    const parent = node.parentElement;
    if (parent?.closest(skip)) { flush(); group = null; continue; }
    const nextGroup = parent?.closest(block) || root;
    if (group !== nextGroup) { flush(); group = nextGroup; }
    let excluded = false;
    for (let el = parent; el && el !== root; el = el.parentElement) {
      const css = win.getComputedStyle(el);
      if (css.visibility === "hidden" || css.visibility === "collapse" || el.hidden) { excluded = true; break; }
      if (css.display === "none") {
        if (el.matches(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")) {
          hiddenExamples = true;
          if (options.examples) continue;
        }
        excluded = true; break;
      }
      if (el.matches("details:not([open])") && !el.querySelector(":scope>summary")?.contains(parent)) { excluded = true; break; }
    }
    if (excluded) { flush(); group = null; continue; }
    for (let i = 0; i < node.data.length; i++) {
      const c = /\s/u.test(node.data[i]) ? " " : node.data[i];
      if (c === " " && text.endsWith(" ")) { points[points.length-1].endNode = node; points[points.length-1].end = i+1; continue; }
      text += c; points.push({node,endNode:node,start:i,end:i+1});
    }
  }
  flush();
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
  const details=[...$("out").querySelectorAll("details.dict[data-searchDict]")];
  const dictionaries=source.dictionaries?.length?source.dictionaries:details.map(det=>det.dataset.searchDict).filter(Boolean);
  const first=details[0];
  return {dictionaries,query:source.query??first?.dataset.searchQuery??"",mode:source.mode??first?.dataset.searchMode??"",
    ignore:!!source.ignoreDiacritics,firstSize:Number(first?.dataset.searchInitial)||5,
    pageSize:Number(first?.dataset.searchPageSize)||20,groupID:source.groupID||"",
    rungs:source.rungs?.length?source.rungs:details.map(det=>det.dataset.searchRung||"")};
}
function remoteFindSignature(context){
  return JSON.stringify([context.groupID,context.dictionaries,context.query,context.mode,options.query,options.mode,
    context.ignore,context.rungs,options.matchCase,options.examples,nearWords()]);
}
function remoteFindURL(record,action,extra={}){
  return "/api/search?"+new URLSearchParams({cacheAction:action,cacheID:record.id,...extra});
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
    const params={dict:context.dictionaries.join(","),q:options.query,mode:options.mode,
      resultQuery:context.query,resultMode:context.mode,ignoreDiacritics:context.ignore?"1":"0",
      matchCase:options.matchCase?"1":"0",examples:options.examples?"1":"0",nearWords:String(nearWords()),
      pageSize:String(size),firstSize:String(firstSize),prime:"0",groupID:context.groupID,rungs:context.rungs.join(",")};
    if(anchor){params.anchorDict=anchor.dict;params.anchorArticle=String(anchor.article);params.anchorGroup=String(anchor.group);
      params.anchorOffset=String(anchor.offset);params.anchorDirection=String(anchor.direction)}
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
function serverFindPoints(root,includeExamples){
  const doc=root.ownerDocument,win=doc.defaultView;
  if(!win)return [];
  const groups=[];let text="",points=[],group=null;
  const flush=()=>{if(points.length)groups.push(points);text="";points=[]};
  const walker=doc.createTreeWalker(root,win.NodeFilter.SHOW_TEXT|win.NodeFilter.SHOW_ELEMENT);
  for(let node;(node=walker.nextNode());){
    if(node.nodeType===1){
      if(node.tagName==="BR"&&text&&!text.endsWith(" ")){
        const last=points[points.length-1];text+=" ";points.push({node:last.endNode,endNode:last.endNode,start:last.end,end:last.end});
      }
      continue;
    }
    const parent=node.parentElement;
    if(parent?.closest(skip)){flush();group=null;continue}
    const nextGroup=parent?.closest(block)||root;
    if(group!==nextGroup){flush();group=nextGroup}
    let excluded=false;
    for(let el=parent;el&&el!==root;el=el.parentElement){
      const css=win.getComputedStyle(el);
      if(css.visibility==="hidden"||css.visibility==="collapse"||el.hidden){excluded=true;break}
      if(css.display==="none"){
        if(el.matches(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")&&includeExamples)continue;
        excluded=true;break;
      }
      if(!includeExamples&&el.matches(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")){excluded=true;break}
      if(el.matches("details:not([open])")&&!el.querySelector(":scope>summary")?.contains(parent)){excluded=true;break}
    }
    if(excluded){flush();group=null;continue}
    for(let i=0;i<node.data.length;i++){
      const c=/\s/u.test(node.data[i])?" ":node.data[i];
      if(c===" "&&text.endsWith(" ")){points[points.length-1].endNode=node;points[points.length-1].end=i+1;continue}
      text+=c;points.push({node,endNode:node,start:i,end:i+1});
    }
  }
  flush();
  return groups;
}
function serverFindRanges(root,occurrences,includeExamples){
  const doc=root.ownerDocument,groups=serverFindPoints(root,includeExamples);
  if(!doc.defaultView)return [];
  const out=[];
  for(const occurrence of occurrences||[]){
    const items=groups[Number(occurrence.group)],start=Number(occurrence.start),end=Number(occurrence.end);
    if(!items||start<0||end<=start||end>items.length)continue;
    const a=items[start],b=items[end-1],range=doc.createRange();
    try{range.setStart(a.node,a.start);range.setEnd(b.endNode,b.end)}catch(_){continue}
    out.push({range,example:a.node.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex")||null});
  }
  return out;
}
function serverFindAnchor(){
  if(!searchAnchor||options.mode==="fts")return null;
  const host=searchAnchor.host,det=host?.closest("details.dict"),row=host?.closest("dd");
  if(!det||!row)return null;
  const rows=[...det.querySelectorAll(":scope>dl>dd")],articleOffset=rows.indexOf(row);
  if(articleOffset<0)return null;
  const root=rootFor(host),groups=root&&serverFindPoints(root,options.examples);
  if(!groups)return null;
  for(let group=0;group<groups.length;group++){
    const points=groups[group];
    for(let i=0;i<points.length;i++){
      const point=points[i],node=searchAnchor.range.startContainer,offset=searchAnchor.range.startOffset;
      if(node===point.node&&offset>=point.start&&offset<=point.end){
        const within=offset>=point.end?1:0;
        return {dict:det.dataset.searchDict||"",article:(Number(det.dataset.searchPageStart)||0)+articleOffset,
          group,offset:i+within,direction:searchAnchor.direction};
      }
      if(node===point.endNode&&point.endNode!==point.node&&offset<=point.end){
        return {dict:det.dataset.searchDict||"",article:(Number(det.dataset.searchPageStart)||0)+articleOffset,
          group,offset:i+1,direction:searchAnchor.direction};
      }
    }
  }
  return null;
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
    if(root)found=serverFindRanges(root,target.occurrences,options.examples);
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
async function runFullTextTrace(){
  const navigation=fullTextNavigation,generation=navigation?.generation;
  if(remoteFindTraceActive||options.mode!=="fts"||!navigation?.canNext)return;
  remoteFindTraceActive=true;remoteFindTraceStop=false;
  const record={diagnosticRun:++remoteFindDiagnosticRun,diagnosticStarted:performance.now(),traceRun:++remoteFindTraceRun};
  let steps=0,reason="last_match",previous=Number(navigation.current)||0;
  syncRemoteFindTraceButton();
  await remoteFindDiagnostic(record,"navigation_trace_start",{detail:`trace=${record.traceRun}; source=full_text; position=${previous}; total=${navigation.total||0}`});
  try{
    while(!remoteFindTraceStop&&fullTextNavigation?.generation===generation&&steps<100000){
      const current=fullTextNavigation;
      if(current.complete&&current.current>=current.total)break;
      const started=performance.now(),from=Number(current.current)||0;
      await remoteFindDiagnostic(record,"navigation_request",{detail:`trace=${record.traceRun}; source=full_text; action=navNext; from=${from}`});
      await current.next();
      const next=fullTextNavigation;
      if(!next||next.generation!==generation){reason="search_changed";break}
      const position=Number(next.current)||0;
      steps++;
      await remoteFindDiagnostic(record,"navigation_trace_step",{completed:steps,total:Math.min(100000,next.total||0),detail:`trace=${record.traceRun}; source=full_text; step=${steps}; from=${from}; to=${position}; complete=${!!next.complete}; step_ms=${Math.round(performance.now()-started)}`});
      if(!position||position===from){reason="no_next_match";break}
      if(next.complete&&position<=previous){await next.previous();break}
      previous=position;
      await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
    }
    if(steps>=100000)reason="limit";
    if(remoteFindTraceStop)reason="stopped";
  }catch(error){reason="error";await remoteFindDiagnostic(record,"navigation_trace_error",{detail:`trace=${record.traceRun}; source=full_text; error=${String(error?.message||error).slice(0,700)}`})}
  finally{
    await remoteFindDiagnostic(record,reason==="stopped"?"navigation_trace_stop":"navigation_trace_end",{completed:steps,total:Math.min(100000,fullTextNavigation?.total||0),detail:`trace=${record.traceRun}; source=full_text; reason=${reason}; position=${fullTextNavigation?.current||0}`});
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
function compareToAnchor(hit) {
  if (hit.host !== searchAnchor.host) {
    const hosts = [...$("out").querySelectorAll(".article")];
    return (hosts.indexOf(hit.host) - hosts.indexOf(searchAnchor.host)) * searchAnchor.direction;
  }
  try { return hit.range.compareBoundaryPoints(Range.START_TO_START, searchAnchor.range) * searchAnchor.direction; }
  catch (_) { return 0; }
}
function forceLayout(det) {
  const dl = det?.querySelector(":scope>dl") || null;
  if (laidOutList?.element === dl) return;
  if (laidOutList) {
    const {element,value,priority} = laidOutList;
    value ? element.style.setProperty("content-visibility",value,priority) : element.style.removeProperty("content-visibility");
    laidOutList = null;
  }
  if (!dl) return;
  laidOutList = {element:dl,value:dl.style.getPropertyValue("content-visibility"),priority:dl.style.getPropertyPriority("content-visibility")};
  dl.style.setProperty("content-visibility","visible");
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
function revealCurrent(wrapped=false,step=1) {
  if (!active || !matches.length || index < 0) { paint(); return; }
  dialog.close(); strip.hidden = false; measure();
  restoreExamples();
  const hit = matches[index], det = hit.host.closest("details.dict");
  if (det) det.open = true;
  if (!window.wudictNativeShell) forceLayout(det);
  showExample(hit); paint();
  if (wrapped) $("articleFindCount").textContent += " · " + t(step > 0 ? "articleFind.fromStart" : "articleFind.fromEnd");
  if (!hit.range.startContainer.ownerDocument.defaultView.CSS?.highlights) {
    const doc = hit.range.startContainer.ownerDocument;
    const selection = doc.getSelection(); selection.removeAllRanges(); selection.addRange(hit.range);
    fallbackSelections.set(doc,hit.range);
  }
  freezeBar();
  if (window.wudictNativeShell) {
    settleOn(hit,det);
    return;
  }
  // Bring the opened dictionary into view first so WebView2 lays out its
  // article before the Range supplies the match's final coordinates.
  if (det) revealAt(det.getBoundingClientRect().top,det,true);
  settleOn(hit,det);
}
let blockNavigationPending=false,blockNavigationToken=0;
function advanceLoaded(step){
  const next=index+step,wrapped=next<0||next>=matches.length;
  index=(next+matches.length)%matches.length;
  revealCurrent(wrapped,step);
}
function selectAfterBlock(det,step){
  const list=[...$("out").querySelectorAll("details.dict")],at=list.indexOf(det);
  const rows=matches.map((hit,i)=>({hit,i,at:list.indexOf(hit.host.closest("details.dict"))}));
  let row=step>0?rows.find(item=>item.at>at):rows.slice().reverse().find(item=>item.at<at);
  if(!row)row=step>0?rows[0]:rows.at(-1);
  if(!row){index=-1;paint();return false}
  index=row.i;revealCurrent();return true;
}
async function crossResultBlocks(current,step){
  if(blockNavigationPending)return;
  blockNavigationPending=true;
  const token=++blockNavigationToken,query=options.query,mode=options.mode;
  let det=current.matches?.("details.dict")?current:current.host.closest("details.dict"),changed=false;
  const originStart=Number(det?.dataset.searchPageStart)||0;
  try{
    const visited=new Set();
    while(det&&window.wudictArticleBlockHasMore?.(det,step)){
      const start=Number(det.dataset.searchPageStart)||0;
      if(visited.has(start))break;
      visited.add(start);
      const replacement=await window.wudictRequestArticleBlock?.(det,step);
      if(token!==blockNavigationToken||!active||options.query!==query||options.mode!==mode||options.blocks===false)return;
      if(!replacement)break;
      changed=true;det=replacement;index=-1;
      for(let check=0;check<10;check++){
        if(check)await new Promise(resolve=>setTimeout(resolve,100));
        if(token!==blockNavigationToken||!active||options.query!==query||options.mode!==mode||options.blocks===false)return;
        rebuild();
        const found=matches.map((hit,i)=>({hit,i})).filter(row=>row.hit.host.closest("details.dict")===det);
        if(found.length){index=step>0?found[0].i:found.at(-1).i;blockNavigationPending=false;revealCurrent();return}
        const frames=[...det.querySelectorAll("iframe.article")];
        if(frames.every(frame=>frame.contentDocument?.readyState==="complete"))break;
      }
    }
    if(changed){
      const restored=await window.wudictRestoreArticleBlock?.(det,originStart);
      if(token!==blockNavigationToken||!active||options.query!==query||options.mode!==mode||options.blocks===false)return;
      if(restored)det=restored;
      rebuild();blockNavigationPending=false;selectAfterBlock(det,step);
    }else{blockNavigationPending=false;if(matches.length)advanceLoaded(step);else paint()}
  }finally{
    if(token===blockNavigationToken)blockNavigationPending=false;
  }
}
function go(step) {
  if(blockNavigationPending)return;
  if ($("articleFindMode").value === "fts") {
    if (step<0?fullTextNavigation?.canPrevious:fullTextNavigation?.canNext)
      fullTextNavigation[step < 0 ? "previous" : "next"]?.();
    return;
  }
  if(remoteFind){void moveRemoteFind(step);return}
  if(active){void startRemoteFind();return}
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  if(options.mode!=="fts")recordFindHistory(options.query);
  if (!active) return;
  if (!matches.length) {
    const sections=[...$("out").querySelectorAll("details.dict")];
    const candidates=step>0?sections:sections.reverse();
    const det=options.blocks!==false&&candidates.find(section=>window.wudictArticleBlockHasMore?.(section,step));
    if(det){crossResultBlocks(det,step);return}
    const {counted}=countedPosition();
    if(counted){goto(step>0?1:counted);return}
    paint();return;
  }
  let wrapped = false;
  if (index < 0) {
    if (searchAnchor) {
      const candidates = matches.map((hit,i) => ({hit,i})).filter(({hit}) => compareToAnchor(hit) >= 0);
      const chosen = searchAnchor.direction > 0 ? candidates[0] : candidates.at(-1);
      wrapped = !chosen;
      index = chosen ? chosen.i : searchAnchor.direction > 0 ? 0 : matches.length-1;
      searchAnchor = null;
    }
    else {
      // The first jump starts at the reading position, ignoring collapsed sections.
      const candidates = matches.map((hit,i) => ({hit,i})).filter(({hit}) => hit.host.closest("details.dict")?.open && (!hit.example || hit.example.ownerDocument.defaultView.getComputedStyle(hit.example).display !== "none"));
      const found = step > 0 ? candidates.find(({hit}) => topOf(hit) >= barH()) : candidates.reverse().find(({hit}) => topOf(hit) < innerHeight/2);
      index = found ? found.i : step > 0 ? 0 : matches.length-1;
    }
  } else {
    const current = matches[index];
    const following = matches[index+step];
    const currentDet=current?.host.closest("details.dict");
    const boundary=step>0?(!following||following.host.closest("details.dict")!==currentDet):
      (!matches[index-1]||matches[index-1].host.closest("details.dict")!==currentDet);
    if(options.blocks!==false&&current&&boundary&&window.wudictArticleBlockHasMore?.(currentDet,step)){
      crossResultBlocks(current,step);return;
    }
    const next = index+step; wrapped = next < 0 || next >= matches.length;
    index = (next+matches.length)%matches.length;
  }
  revealCurrent(wrapped,step);
}
function goto(page) {
  if ($("articleFindMode").value === "fts") { fullTextNavigation?.goto?.(page); return true; }
  if(remoteFind){void gotoRemoteFind(page);return true}
  if(active){void startRemoteFind();return true}
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  /* Go to match reaches the matches the dialog has counted, so it is bounded by
     that number and not by the floored total: a dictionary's own count can name
     a match beyond the blocks examined, and a jump nobody can make would read
     as a dead control. */
  const {counted}=countedPosition();
  if (!Number.isInteger(page) || page < 1 || page > counted) return false;
  let offset=0;
  for(const det of $("out").querySelectorAll("details.dict")){
    for(const entry of blockPages(det)){
      if(page>offset+entry.count){offset+=entry.count;continue}
      const local=page-offset-1;
      if((Number(det.dataset.searchPageStart)||0)===entry.start){
        const hits=matches.map((hit,i)=>({hit,i})).filter(row=>row.hit.host.closest("details.dict")===det);
        if(!hits[local])return false;
        index=hits[local].i;revealCurrent();return true;
      }
      const token=blockNavigationToken,query=options.query;
      window.wudictRestoreArticleBlock?.(det,entry.start)?.then(async replacement=>{
        if(!replacement||!active||token!==blockNavigationToken||query!==options.query)return;
        for(let attempt=0;attempt<10;attempt++){
          if(attempt)await new Promise(resolve=>setTimeout(resolve,100));
          if(!active||token!==blockNavigationToken||query!==options.query)return;
          rebuild();
          const hits=matches.map((hit,i)=>({hit,i})).filter(row=>row.hit.host.closest("details.dict")===replacement);
          if(hits[local]){index=hits[local].i;revealCurrent();return}
        }
      });
      return true;
    }
  }
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
  const isFTS = $("articleFindMode").value === "fts" && !!fullTextNavigation;
  const isRemote=options.mode!=="fts"&&!!remoteFind;
  const position=countedPosition();
  const total=isFTS ? Number(fullTextNavigation.total)||0 : isRemote?Number(remoteFind.state?.total)||0:(active ? position.counted : 0);
  const current=isFTS ? Number(fullTextNavigation.current)||1 : isRemote?Number(remoteFind.state?.position)||1:Math.max(1,position.current);
  if (total) window.wudictOpenArticleFindGoto?.(total,current);
}
function stop() {
  blockNavigationToken++;blockNavigationPending=false;
  closeRemoteFind();
  resetCounts();
  const navigation = fullTextNavigation;
  fullTextNavigation = null;
  if (settle) settle();
  forceLayout(null);
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
  if(!matches.length)return;
  if(index<0)index=0;
  revealCurrent();
};
$("articleFindTrace").onclick = () => {
  if(remoteFindTraceActive){remoteFindTraceStop=true;return}
  if(options.mode==="fts"){void runFullTextTrace();return}
  void runRemoteFindTrace();
};
$("sysLogSearch")?.addEventListener("change",syncRemoteFindTraceButton);
$("articleFindDialogPrev").onclick = () => go(-1);
$("articleFindDialogNext").onclick = () => go(1);
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
window.wudictArticleFind = {stop, refresh:schedule, setFullTextNavigation, saveTraceArticle:(dict,article,html)=>{
  if(!remoteFindTraceActive||!html)return Promise.resolve(false);
  const bytes=new TextEncoder().encode(html).byteLength;
  if(bytes>2*1024*1024){
    void remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now()},"navigation_trace_article_save_failed",{detail:`trace=${remoteFindTraceRun}; dict=${dict}; article=${article}; bytes=${bytes}; reason=over_limit`});
    return Promise.resolve(false);
  }
  return sendBrowseDiagnostic({event:"search_trace_article",runId:remoteFindTraceRun,dict,page:Number(article)||0,completed:remoteFindTraceRun,html}).then(async saved=>{
    if(!saved)await remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now()},"navigation_trace_article_save_failed",{detail:`trace=${remoteFindTraceRun}; dict=${dict}; article=${article}; bytes=${bytes}; reason=server_rejected`});
    return saved;
  });
}, traceEvent:(stage,detail,elapsedMs=0)=>{
  if(!remoteFindTraceActive)return;
  void remoteFindDiagnostic({diagnosticRun:remoteFindDiagnosticRun,diagnosticStarted:performance.now(),traceRun:remoteFindTraceRun},stage,{elapsedMs,detail:`trace=${remoteFindTraceRun}; source=full_text; ${detail}`});
}, fullTextModeActive:()=>options.mode === "fts", isActive:()=>active&&!!options.query, searchAcrossBlocks:()=>true, searchFrom, goto, setHistoryLimit:n=>{findHistory=findHistory.slice(0,Math.max(0,Math.min(1000,Number(n)||0)));saveFindHistory();renderFindHistory()}};
})();
