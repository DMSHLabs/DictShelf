/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

// Separate search histories and live headword suggestions for the search field.
// The page supplies a mode-aware search action; this file owns storage,
// rendering, input events and the history-length control.
(function () {
  "use strict";
  const $ = id => document.getElementById(id);
  const MAX = 100;
  const searchStorageKey = "wudict_search_history_v2";
  const wildcardsStorageKey = "wudict_wildcards_history_v2";
  const key = s => s.normalize("NFC").toLocaleLowerCase();
  const limit = () => {
    const raw = localStorage.getItem("wudict_history_length");
    if (raw === null) return 20;
    const n = Number(raw);
    return Number.isInteger(n) && n >= 0 && n <= MAX ? n : 20;
  };
  function load(storageKey) {
    const rows = [];
    try {
      const source = JSON.parse(localStorage.getItem(storageKey) || "[]");
      if (Array.isArray(source)) {
        const seen = new Set();
        for (const word of source) {
          if (typeof word !== "string" || !word.trim()) continue;
          const id = key(word);
          if (seen.has(id)) continue;
          seen.add(id);
          rows.push(word);
          if (rows.length === MAX) break;
        }
      }
    } catch (_) {}
    return rows.slice(0, limit());
  }
  const previousSaved = load("wudict_search_history");
  const hasStored = storageKey => { try { return localStorage.getItem(storageKey) !== null; } catch (_) { return false; } };
  let saved = hasStored(searchStorageKey) ? load(searchStorageKey) : previousSaved.filter(word => !/[?*]/.test(word));
  let wildcardsSaved = hasStored(wildcardsStorageKey) ? load(wildcardsStorageKey) :
    [...load("wudict_wildcards_history"), ...previousSaved.filter(word => /[?*]/.test(word))];
  wildcardsSaved = wildcardsSaved.filter((word, i, rows) =>
    rows.findIndex(other => key(other) === key(word)) === i).slice(0, MAX);
  const activeHistory = () => $("mode").value === "wildcards" ? wildcardsSaved : saved;
  const saveActiveHistory = rows => {
    if ($("mode").value === "wildcards") wildcardsSaved = rows;
    else saved = rows;
  };
  let draft = false, open = false, heads = [], picking = false, search = () => {}, suggest = () => {};
  let suggestTimer = null, suggestAC = null, suggestSeq = 0;
  function stopSuggest() {
    clearTimeout(suggestTimer);
    if (suggestAC) suggestAC.abort();
    suggestAC = null;
    suggestSeq++;
  }

  function persist() {
    try {
      localStorage.setItem(searchStorageKey, JSON.stringify(saved));
      localStorage.setItem(wildcardsStorageKey, JSON.stringify(wildcardsSaved));
    } catch (_) {}
    $("clearHistory").disabled = !activeHistory().length;
  }
  function record(term) {
    recordFor($("mode").value === "wildcards" ? "wildcards" : "search", term);
  }
  function recordFor(kind, term) {
    draft = false;
    const word = String(term || "").trim(), max = limit();
    if (!word || !max) return;
    const rows = kind === "wildcards" ? wildcardsSaved : saved;
    const next = [word, ...rows.filter(x => key(x) !== key(word))].slice(0, max);
    if (kind === "wildcards") wildcardsSaved = next; else saved = next;
    persist();
  }
  function hide() { stopSuggest(); open = false; $("searchHistory").hidden = true; }
  function size() {
    const box = $("searchHistory");
    if (!matchMedia("(max-width:600px)").matches) { box.style.right = ""; return; }
    const pill = document.querySelector("#frm>.pill");
    const extra = pill.getBoundingClientRect().right - $("qbox").getBoundingClientRect().right;
    box.style.right = -Math.max(0, extra - 1) + "px";
  }
  function render(filter = "") {
    if (!open) return;
    const box = $("searchHistory");
    size();
    box.replaceChildren();
    const needle = key(filter.trim()), seen = new Set();
    const maxRows = Math.max(20, limit());
    const history = activeHistory().slice(0, limit());
    const words = needle ? [...heads, ...history] : history;
    for (const word of words) {
      const id = key(word);
      if ((needle && !id.includes(needle)) || seen.has(id)) continue;
      seen.add(id);
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = word;
      button.addEventListener("click", () => { picking = false; hide(); search(word); });
      box.appendChild(button);
      if (box.childElementCount >= maxRows) break;
    }
    box.hidden = !box.childElementCount;
  }
  function show(filter = "") {
    open = true; render(filter);
  }
  function modeChanged() {
    stopSuggest();
    heads = [];
    $("clearHistory").disabled = !activeHistory().length;
    if (open) render($("q").value);
  }
  function onInput(value) {
    draft = true;
    refreshSuggestions(value);
  }
  function refreshSuggestions(value = $("q").value) {
    stopSuggest();
    heads = [];
    show(value);
    const q = value.trim();
    if (q.length < 2 || $("mode").value === "fts" || $("mode").value === "wildcards") return;
    const seq = suggestSeq;
    suggestTimer = setTimeout(() => {
      suggestAC = new AbortController();
      Promise.resolve(suggest(q, (results, mode) => {
        if (seq === suggestSeq) addResults(q, results, mode);
      }, suggestAC.signal)).catch(err => {
        if (err?.name !== "AbortError") console.warn("Suggestions:", err);
      });
    }, 300);
  }
  function addResults(q, results, mode) {
    if (mode === "fts" || !open || $("q").value.trim() !== q || !results?.length) return;
    const seen = new Set(heads.map(key));
    for (const result of results) {
      const word = String(result.Headword || "").trim(), id = key(word);
      if (!word || seen.has(id)) continue;
      seen.add(id);
      heads.push(word);
      if (heads.length >= MAX) break;
    }
    render(q);
  }

  $("historyLength").value = limit();
  $("clearHistory").disabled = !activeHistory().length;
  $("clearHistory").addEventListener("click", () => {
    saveActiveHistory([]);
    draft = false;
    persist();
    render($("q").value);
  });
  $("historyLength").addEventListener("change", e => {
    const n = Math.max(0, Math.min(MAX, Math.trunc(Number(e.target.value) || 0)));
    e.target.value = n;
    try { localStorage.setItem("wudict_history_length", String(n)); } catch (_) {}
    saved = saved.slice(0, n);
    wildcardsSaved = wildcardsSaved.slice(0, n);
    persist();
    if (!n) hide();
  });
  $("q").addEventListener("focus", () => show());
  $("q").addEventListener("pointerdown", () => show());
  $("searchHistory").addEventListener("pointerdown", () => { draft = false; picking = true; });
  $("searchHistory").addEventListener("pointercancel", () => { picking = false; });
  document.addEventListener("pointerdown", e => {
    if (!$("qbox").contains(e.target)) hide();
  });
  $("q").addEventListener("blur", () => {
    if (draft && $("q").value.trim().length >= 2) record($("q").value);
  });
  $("q").addEventListener("keydown", e => {
    if (e.key === "Escape") { hide(); return; }
    if (e.key === "ArrowDown" && !$("searchHistory").hidden) {
      e.preventDefault();
      $("searchHistory").firstElementChild?.focus();
    }
  });
  $("searchHistory").addEventListener("keydown", e => {
    if (e.key === "Escape") { hide(); $("q").focus(); }
  });
  const sizer = new ResizeObserver(() => { if (open) size(); });
  sizer.observe(document.querySelector("#frm>.pill"));
  sizer.observe($("qbox"));

  window.wuSearchHistory = { setSearch(fn) { search = fn; }, setSuggest(fn) { suggest = fn; }, record, recordFor,
    getFor(kind) { return [...(kind === "wildcards" ? wildcardsSaved : saved)]; },
    hide, onInput, refreshSuggestions, modeChanged, isPicking() { return picking; } };
})();
