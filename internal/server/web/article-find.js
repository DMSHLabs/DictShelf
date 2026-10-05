/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

// Find in loaded articles. Ranges preserve dictionary markup, links and selections.
(function () {
"use strict";
const $ = id => document.getElementById(id), t = window.wudictI18n.t;
const dialog = $("articleFindDialog"), strip = $("articleFindStrip");
const wordChar = /[\p{L}\p{M}\p{N}_]/u;
const skip = "script,style,noscript,template,button,input,select,textarea,audio,video,svg";
const block = "p,div,li,dt,dd,td,th,h1,h2,h3,h4,h5,h6,blockquote,pre,.wu-m";
let options = {query:"", mode:"contains", matchCase:false, all:true, examples:false};
try { Object.assign(options, JSON.parse(localStorage.getItem("wudict_article_find") || "{}")); } catch (_) {}
let matches = [], index = -1, active = false, restored = [], docs = new Set(), timer;
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
  $("articleFindMode").value = ["prefix","exact","contains"].includes(options.mode) ? options.mode : "contains";
  $("articleFindCase").checked = options.matchCase;
  $("articleFindAll").checked = options.all;
  $("articleFindExamples").checked = options.examples;
}
function open() {
  fillOptions();
  active = !!options.query;
  rebuild();
  dialog.showModal();
  $("articleFindQuery").focus();
  $("articleFindQuery").select();
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
  const count = matches.length ? t("articleFind.position", {current:index < 0 ? 0 : index+1, total:matches.length}) : t("articleFind.noMatches");
  $("articleFindCount").textContent = count;
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
      matches.push({host,range,example:a.node.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt") || null});
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
        if (el.matches(".wu-xonly,.wu-sec,.dsl_opt")) {
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
  if (index >= 0) showExample(matches[index]);
  paint();
}
function showExample(hit) {
  for (let el = hit.example; el; el = el.parentElement?.closest(".wu-xonly,.wu-sec,.dsl_opt")) {
    if (el.ownerDocument.defaultView.getComputedStyle(el).display !== "none") continue;
    restored.push([el,el.style.getPropertyValue("display"),el.style.getPropertyPriority("display")]);
    el.style.setProperty("display",el.matches("span,.wu-inline-example") ? "inline" : "block","important");
  }
}
function topOf(hit) {
  return hit.range.getBoundingClientRect().top + (hit.host.shadowRoot ? 0 : hit.host.getBoundingClientRect().top);
}
function go(step) {
  if (dialog.open || !active) { clearTimeout(timer); readOptions(); active = !!options.query; rebuild(); }
  if (!active) return;
  dialog.close(); strip.hidden = false; measure();
  if (!matches.length) { paint(); return; }
  let wrapped = false;
  if (index < 0) {
    // The first jump starts at the reading position, ignoring collapsed sections.
    const candidates = matches.map((hit,i) => ({hit,i})).filter(({hit}) => hit.host.closest("details.dict")?.open && (!hit.example || hit.example.ownerDocument.defaultView.getComputedStyle(hit.example).display !== "none"));
    const found = step > 0 ? candidates.find(({hit}) => topOf(hit) >= barH()) : candidates.reverse().find(({hit}) => topOf(hit) < innerHeight/2);
    index = found ? found.i : step > 0 ? 0 : matches.length-1;
  } else {
    const next = index+step; wrapped = next < 0 || next >= matches.length;
    index = (next+matches.length)%matches.length;
  }
  restoreExamples();
  const hit = matches[index], det = hit.host.closest("details.dict");
  if (det) det.open = true;
  showExample(hit); paint();
  if (wrapped) $("articleFindCount").textContent += " · " + t(step > 0 ? "articleFind.fromStart" : "articleFind.fromEnd");
  if (!hit.range.startContainer.ownerDocument.defaultView.CSS?.highlights) {
    const doc = hit.range.startContainer.ownerDocument;
    const selection = doc.getSelection(); selection.removeAllRanges(); selection.addRange(hit.range);
    fallbackSelections.set(doc,hit.range);
  }
  freezeBar();
  requestAnimationFrame(() => {
    revealAt(topOf(hit),det,true);
    // An iframe may need a height report after revealing an example.
    setTimeout(() => { if (active && matches[index] === hit && hit.host.isConnected) revealAt(topOf(hit),det,true); },120);
  });
}
function stop() {
  active = false; clearTimeout(timer); restoreExamples(); clearPaint(); matches = []; index = -1;
  docs.clear();
  if (dialog.open) dialog.close();
  strip.hidden = true; measure();
}
function schedule() { clearTimeout(timer); timer = setTimeout(() => { if (active) rebuild(); },100); }
function measure() {
  document.documentElement.style.setProperty("--article-find-height", strip.hidden ? "0px" : strip.offsetHeight+"px");
}
$("sbarFind").onclick = open;
$("articleFindLabel").onclick = open;
$("articleFindClose").onclick = () => dialog.close();
$("articleFindStop").onclick = stop;
$("articleFindPrev").onclick = () => go(-1);
$("articleFindNext").onclick = () => go(1);
$("articleFindDialogPrev").onclick = () => go(-1);
$("articleFindDialogNext").onclick = () => go(1);
for (const id of ["articleFindMode","articleFindCase","articleFindAll","articleFindExamples"]) $(id).onchange = () => { readOptions(); index = -1; active = !!options.query; rebuild(); };
$("articleFindQuery").oninput = () => { readOptions(); index = -1; active = !!options.query; schedule(); };
dialog.addEventListener("keydown", e => { if (e.key === "Enter" && e.target.tagName !== "BUTTON") { e.preventDefault(); go(e.shiftKey ? -1 : 1); } });
dialog.addEventListener("close", () => { if (active) { strip.hidden = false; measure(); } });
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
window.wudictArticleFind = {stop, refresh:schedule};
})();
