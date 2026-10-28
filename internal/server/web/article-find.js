/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

// Find in loaded articles, or navigate the exact full-text result marks.
(function () {
"use strict";
const $ = id => document.getElementById(id), t = window.wudictI18n.t;
const dialog = $("articleFindDialog"), strip = $("articleFindStrip");
const wordChar = /[\p{L}\p{M}\p{N}_]/u;
const skip = "script,style,noscript,template,button,input,select,textarea,audio,video,svg";
const block = "p,div,li,dt,dd,td,th,h1,h2,h3,h4,h5,h6,blockquote,pre,.wu-m";
let options = {query:"", mode:"contains", matchCase:false, all:true, examples:false, blocks:true};
try { Object.assign(options, JSON.parse(localStorage.getItem("wudict_article_find") || "{}")); } catch (_) {}
let matches = [], index = -1, active = false, restored = [], docs = new Set(), timer, fullTextNavigation = null, fullTextGeneration = null, searchAnchor = null;
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
function countedPosition(){
  const det=matches[index]?.host.closest("details.dict");
  const dictionaries=[...$("out").querySelectorAll("details.dict")];
  let total=0,current=0;
  for(const section of dictionaries){
    const start=Number(section.dataset.searchPageStart)||0;
    for(const page of blockPages(section)){
      if(section===det&&page.start===start){
        current=total+matches.slice(0,index+1).filter(hit=>hit.host.closest("details.dict")===det).length;
      }
      total+=page.count;
    }
  }
  return {current,total};
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
function readOptions() {
  const previous=options;
  options = {query:$("articleFindQuery").value.trim(), mode:$("articleFindMode").value,
    matchCase:$("articleFindCase").checked, all:$("articleFindAll").checked,
    examples:$("articleFindExamples").checked, blocks:$("articleFindBlocks").checked};
  if(previous.query!==options.query||previous.mode!==options.mode||previous.matchCase!==options.matchCase||previous.examples!==options.examples)resetCounts();
  save();
}
function fillOptions() {
  $("articleFindQuery").value = options.query;
  if(options.mode==="exact")options.mode="word"; // saved by the earlier article-search picker
  options.mode = ["prefix","suffix","contains","word","wordLemma"].includes(options.mode) || options.mode === "fts" && fullTextNavigation ? options.mode : "contains";
  $("articleFindMode").value = options.mode;
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
  $("articleFindBlocks").checked = options.blocks !== false;
  syncControls();
}
function open(preserveAnchor=false) {
  if (!preserveAnchor) searchAnchor = null;
  if (fullTextNavigation && options.mode === "fts") options = {...options, query:fullTextNavigation.query};
  fillOptions();
  active = options.mode !== "fts" && !!options.query;
  if (active) rebuild();
  dialog.showModal();
  strip.hidden = true; measure();
  if (options.mode === "fts") $("articleFindMode").focus();
  else { $("articleFindQuery").focus(); $("articleFindQuery").select(); }
}
function syncControls() {
  const select = $("articleFindMode"), hasFTS = !!fullTextNavigation;
  const ftsOption=select.querySelector('option[value="fts"]');
  ftsOption.disabled=!hasFTS;ftsOption.hidden=!hasFTS;
  let fts = select.value === "fts";
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
  } else {
    $("articleFindDialogPrev").disabled = !canStep(-1);
    $("articleFindDialogNext").disabled = !canStep(1);
  }
  window.wudictThemedSelects?.articleFindMode?.sync();
}
function canStep(step){
  if(options.mode==="fts")return !!(step<0?fullTextNavigation?.canPrevious:fullTextNavigation?.canNext);
  if(matches.length>=2)return true;
  if(!matches.length)return countedPosition().total>0 || options.blocks!==false &&
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
function matcherFor(host,query){
  const alternatives=[query];
  if(options.mode==="wordLemma"&&/^\S+$/u.test(query))alternatives.push(lemmaFor(host,query));
  const unique=[...new Set(alternatives)].sort((a,b)=>b.length-a.length);
  return new RegExp(unique.map(escapeRE).join("|"),options.matchCase?"gu":"giu");
}
function paint() {
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
  const count = position.total ? t("articleFind.position", {current:position.current, total})+(block?" · "+block:"") : t("articleFind.noMatches");
  if(blockNavigationPending)return;
  $("articleFindCount").textContent = count;
  $("articleFindCount").disabled = !position.total;
  $("articleFindStatus").textContent = options.query ? count : "";
  $("articleFindHiddenHint").hidden = !options.query || !!matches.length || options.examples || !hiddenExamples;
  $("articleFindLabel").textContent = options.query;
  $("articleFindLabel").title = options.query;
  $("articleFindPrev").disabled = !canStep(-1);
  $("articleFindNext").disabled = !canStep(1);
  $("articleFindDialogPrev").disabled = !canStep(-1);
  $("articleFindDialogNext").disabled = !canStep(1);
  $("articleFindScrollToMatch").disabled = options.mode === "fts" || !matches.length || index < 0;
}
let hiddenExamples = false;
// Build a text stream per paragraph, retaining offsets across inline formatting.
// Whitespace is folded so phrases also match line breaks and nonbreaking spaces.
function scan(host, root, regex) {
  const doc = root.ownerDocument, win = doc.defaultView;
  if (!win) return;
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
  function flush() {
    regex.lastIndex = 0;
    for (let found; (found = regex.exec(text)); ) {
      const start = found.index, end = start+found[0].length;
      const before = Array.from(text.slice(0,start)).pop() || "", after = Array.from(text.slice(end))[0] || "";
      if (["prefix","word","wordLemma"].includes(options.mode) && wordChar.test(before)) continue;
      if (["suffix","word","wordLemma"].includes(options.mode) && wordChar.test(after)) continue;
      const a = points[start], b = points[end-1];
      if (!a || !b) continue;
      const range = doc.createRange(); range.setStart(a.node,a.start); range.setEnd(b.endNode,b.end);
      matches.push({host,range,example:a.node.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt,.wu-exonly,.wu-ex,.dsl_ex") || null});
    }
    text = ""; points = [];
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
function rebuild() {
  if (options.mode === "fts") { syncControls(); return; }
  const previous = matches[index];
  restoreExamples(); clearPaint(); matches = []; docs = new Set(); hiddenExamples = false;
  if (!options.query) { strip.hidden = true; measure(); }
  if (options.query) {
    let query = options.query;
    if (query.length > 1 && query.startsWith('"') && query.endsWith('"')) query = query.slice(1,-1);
    query = query.replace(/\s+/gu," ");
    if (query) {
      for (const host of $("out").querySelectorAll(".article")) {
        const root = rootFor(host); if (root) scan(host,root,matcherFor(host,query));
      }
    }
  }
  index = previous ? matches.findIndex(hit => hit.range.startContainer === previous.range.startContainer && hit.range.startOffset === previous.range.startOffset) : -1;
  recordCounts();
  if (index < 0 && !window.wudictNativeShell) forceLayout(null);
  if (index >= 0) showExample(matches[index]);
  paint();
  if (index >= 0 && active && !strip.hidden && !dialog.open && !window.wudictNativeShell) {
    const det = matches[index].host.closest("details.dict");
    forceLayout(det);
    settleOn(matches[index],det);
  }
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
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  if (!active) return;
  if (!matches.length) {
    const sections=[...$("out").querySelectorAll("details.dict")];
    const candidates=step>0?sections:sections.reverse();
    const det=options.blocks!==false&&candidates.find(section=>window.wudictArticleBlockHasMore?.(section,step));
    if(det){crossResultBlocks(det,step);return}
    const {total}=countedPosition();
    if(total){goto(step>0?1:total);return}
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
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  const {total}=countedPosition();
  if (!Number.isInteger(page) || page < 1 || page > total) return false;
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
  const position=countedPosition();
  const total=isFTS ? Number(fullTextNavigation.total)||0 : (active ? position.total : 0);
  const current=isFTS ? Number(fullTextNavigation.current)||1 : Math.max(1,position.current);
  if (total) window.wudictOpenArticleFindGoto?.(total,current);
}
function stop() {
  blockNavigationToken++;blockNavigationPending=false;
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
  if (!fullTextNavigation) {
    $("articleFindMode").querySelector('option[value="fts"]').disabled = true;
    if (!active && !dialog.open) strip.hidden = true;
    syncControls();
    measure();
    return;
  }
  if (newSearch && !dialog.open) options = {...options, query:fullTextNavigation.query || "", mode:"fts"};
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
  if(options.mode === "fts" || !matches.length)return;
  if(index<0)index=0;
  revealCurrent();
};
$("articleFindDialogPrev").onclick = () => go(-1);
$("articleFindDialogNext").onclick = () => go(1);
$("articleFindMode").onchange = () => {
  if ($("articleFindMode").value === "fts") {
    if (!fullTextNavigation) { $("articleFindMode").value = options.mode = "contains"; syncControls(); return; }
    options = {...options, query:fullTextNavigation.query || "", mode:"fts"};
    $("articleFindQuery").value = options.query;
    active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1; docs.clear();
    save(); syncControls();
    return;
  }
  readOptions(); index = -1; active = !!options.query; rebuild(); syncControls();
};
for (const id of ["articleFindCase","articleFindAll","articleFindExamples"]) $(id).onchange = () => { readOptions(); index = -1; active = !!options.query; rebuild(); };
$("articleFindBlocks").onchange=()=>{readOptions();if(options.mode!=="fts"){index=-1;active=!!options.query;rebuild()}else{fullTextNavigation?.refresh?.();syncControls()}};
$("articleFindQuery").oninput = () => { if ($("articleFindMode").value === "fts") return; readOptions(); index = -1; active = !!options.query; schedule(); };
dialog.addEventListener("keydown", e => { if (e.key === "Enter" && e.target.tagName !== "BUTTON") { e.preventDefault(); go(e.shiftKey ? -1 : 1); } });
dialog.addEventListener("close", () => {
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
$("sbarExamples").addEventListener("click",schedule);
$("examplesRow")?.addEventListener("click",schedule);
window.wudictArticleFind = {stop, refresh:schedule, setFullTextNavigation, fullTextModeActive:()=>options.mode === "fts", searchAcrossBlocks:()=>options.blocks!==false, searchFrom, goto};
})();
