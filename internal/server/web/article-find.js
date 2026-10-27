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
let options = {query:"", mode:"contains", matchCase:false, all:true, examples:false};
try { Object.assign(options, JSON.parse(localStorage.getItem("wudict_article_find") || "{}")); } catch (_) {}
let matches = [], index = -1, active = false, restored = [], docs = new Set(), timer, fullTextNavigation = null, fullTextGeneration = null, searchAnchor = null;
let settle = null;
let laidOutList = null;
const fallbackSelections = new Map();
const watched = new WeakSet();
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
  options = {query:$("articleFindQuery").value.trim(), mode:$("articleFindMode").value,
    matchCase:$("articleFindCase").checked, all:$("articleFindAll").checked,
    examples:$("articleFindExamples").checked};
  save();
}
function fillOptions() {
  $("articleFindQuery").value = options.query;
  options.mode = ["prefix","exact","contains"].includes(options.mode) || options.mode === "fts" && fullTextNavigation ? options.mode : "contains";
  $("articleFindMode").value = options.mode;
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
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
  const fts = select.value === "fts";
  $("articleFindStop").disabled = fts && hasFTS;
  select.querySelector('option[value="fts"]').disabled = !hasFTS;
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
    $("articleFindDialogPrev").disabled = $("articleFindDialogNext").disabled = !fullTextNavigation?.canNavigate;
  } else {
    $("articleFindDialogPrev").disabled = $("articleFindDialogNext").disabled = !options.query;
  }
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
  const partial = matches.length > 0 && !!window.wudictArticleFindHasMore?.();
  const total = String(matches.length)+(partial?"+":"");
  const block = index >= 0 ? window.wudictArticleFindBlockRange?.(matches[index]?.host.closest("details.dict")) : "";
  const count = matches.length ? t("articleFind.position", {current:index < 0 ? 0 : index+1, total})+(block?" · "+block:"") : t("articleFind.noMatches");
  $("articleFindCount").textContent = count;
  $("articleFindCount").disabled = !matches.length;
  $("articleFindStatus").textContent = options.query ? count : "";
  $("articleFindHiddenHint").hidden = !options.query || !!matches.length || options.examples || !hiddenExamples;
  $("articleFindLabel").textContent = options.query;
  $("articleFindLabel").title = options.query;
  $("articleFindPrev").disabled = $("articleFindNext").disabled = !matches.length;
  $("articleFindDialogPrev").disabled = $("articleFindDialogNext").disabled = !options.query;
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
      if (options.mode !== "contains" && wordChar.test(before)) continue;
      if (options.mode === "exact" && wordChar.test(after)) continue;
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
      const regex = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g,"\\$&"), options.matchCase ? "gu" : "giu");
      for (const host of $("out").querySelectorAll(".article")) {
        const root = rootFor(host); if (root) scan(host,root,regex);
      }
    }
  }
  index = previous ? matches.findIndex(hit => hit.range.startContainer === previous.range.startContainer && hit.range.startOffset === previous.range.startOffset) : -1;
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
  let moves = 0, stable = 0, pending = 0, frame = 0;
  const quit = ["wheel","touchstart","pointerdown","keydown"];
  const stopSettle = () => {
    clearTimeout(pending); clearTimeout(timeout); cancelAnimationFrame(frame);
    for (const event of quit) removeEventListener(event,stopSettle);
    if (settle === stopSettle) settle = null;
  };
  const align = () => {
    if (!active || matches[index] !== hit || !hit.host.isConnected) return stopSettle();
    const rect = hit.range.getBoundingClientRect(), laidOut = !!(rect.width || rect.height);
    const top = topOf(hit), bottom = top + rect.height;
    const ceiling = barH() + (det?.querySelector(":scope>summary")?.offsetHeight || 0);
    const floor = innerHeight - strip.offsetHeight - 8;
    // Expanded dictionaries above this one can change height without resizing
    // the selected dictionary. Keep checking until the match stays on screen.
    if (laidOut && top >= ceiling && top < floor && Math.min(bottom,top+24) <= floor) {
      if (++stable >= 5) return stopSettle();
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
  for (const event of quit) addEventListener(event,stopSettle,{passive:true});
  const timeout = setTimeout(stopSettle,4000);
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
    requestAnimationFrame(() => {
      revealAt(topOf(hit),det,true);
      setTimeout(() => { if (active && matches[index] === hit && hit.host.isConnected) revealAt(topOf(hit),det,true); },120);
    });
    return;
  }
  // Bring the opened dictionary into view first so WebView2 lays out its
  // article before the Range supplies the match's final coordinates.
  if (det) revealAt(det.getBoundingClientRect().top,det,true);
  settleOn(hit,det);
}
function go(step) {
  if ($("articleFindMode").value === "fts") {
    if (fullTextNavigation?.canNavigate) fullTextNavigation[step < 0 ? "previous" : "next"]?.();
    return;
  }
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  if (!active) return;
  if (!matches.length) { paint(); return; }
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
    if (step > 0 && current && (!following || following.host.closest("details.dict") !== current.host.closest("details.dict")))
      window.wudictRequestNextArticleBlock?.(current.host);
    const next = index+step; wrapped = next < 0 || next >= matches.length;
    index = (next+matches.length)%matches.length;
  }
  revealCurrent(wrapped,step);
}
function goto(page) {
  if ($("articleFindMode").value === "fts") { fullTextNavigation?.goto?.(page); return true; }
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  if (!Number.isInteger(page) || page < 1 || page > matches.length) return false;
  index = page-1; revealCurrent(); return true;
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
  const total=isFTS ? Number(fullTextNavigation.total)||0 : (active ? matches.length : 0);
  const current=isFTS ? Number(fullTextNavigation.current)||1 : Math.max(1,index+1);
  if (total) window.wudictOpenArticleFindGoto?.(total,current);
}
function stop() {
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
    $("articleFindPrev").disabled = $("articleFindNext").disabled = !fullTextNavigation.canNavigate;
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
window.wudictArticleFind = {stop, refresh:schedule, setFullTextNavigation, fullTextModeActive:()=>options.mode === "fts", searchFrom, goto};
})();
