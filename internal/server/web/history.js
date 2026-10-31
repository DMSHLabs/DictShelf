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
  const MAX = 1000;
  const searchStorageKey = "wudict_search_history_v2";
  const wildcardsStorageKey = "wudict_wildcards_history_v2";
  const key = s => s.normalize("NFC").toLocaleLowerCase();
  const suggestionCollator = new Intl.Collator(undefined, {sensitivity: "base", numeric: false});
  const caseFold = s => String(s).normalize("NFC").toLocaleLowerCase();
  const accentFold = s => String(s).normalize("NFD").replace(/\p{M}/gu, "").normalize("NFC").toLocaleLowerCase();
  const punctuationFold = s => accentFold(s).replace(/[^\p{L}\p{N}\s]/gu, "").replace(/\s+/gu, " ").trim();
  const whitespaceFold = s => punctuationFold(s).replace(/\s/gu, "");
  const tokens = s => accentFold(s).match(/[\p{L}\p{N}]+/gu) || [];
  const compareRank = (a, b) => {
    for (let i = 0; i < Math.max(a.length, b.length); i++) {
      const av = a[i] ?? 0, bv = b[i] ?? 0;
      if (av !== bv) return av - bv;
    }
    return 0;
  };
  function formRank(word, query) {
    if (word === query) return 0;
    if (caseFold(word) === caseFold(query)) return 1;
    if (accentFold(word) === accentFold(query)) return 2;
    if (punctuationFold(word) === punctuationFold(query)) return 3;
    if (whitespaceFold(word) === whitespaceFold(query)) return 4;
    return 5;
  }
  function phrasePosition(haystack, needle, fromEnd = false) {
    if (!needle.length || needle.length > haystack.length) return -1;
    const start = fromEnd ? haystack.length - needle.length : 0;
    const end = fromEnd ? -1 : haystack.length - needle.length + 1;
    const step = fromEnd ? -1 : 1;
    for (let i = start; i !== end; i += step) {
      if (needle.every((token, j) => haystack[i + j] === token)) return i;
    }
    return -1;
  }
  function tokenMatchRank(headTokens, queryTokens, relation) {
    if (!queryTokens.length) return [9, 0, 0];
    let total = 0, worst = 0, earliest = Infinity;
    for (const queryToken of queryTokens) {
      let best = null;
      for (let i = 0; i < headTokens.length; i++) {
        const token = headTokens[i], grade = relation(token, queryToken);
        if (grade < 0) continue;
        const candidate = [grade, i];
        if (!best || compareRank(candidate, best) < 0) best = candidate;
      }
      if (!best) return [9, 0, 0];
      worst = Math.max(worst, best[0]);
      total += best[0];
      earliest = Math.min(earliest, best[1]);
    }
    return [worst, total, earliest];
  }
  function wildcardExpansion(word, query) {
    let source = "^", wildcards = 0;
    for (const char of accentFold(query)) {
      if (char === "*") { source += "([\\s\\S]*)"; wildcards++; }
      else if (char === "?") { source += "([\\s\\S])"; wildcards++; }
      else source += char.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    }
    try {
      const match = new RegExp(source + "$", "u").exec(accentFold(word));
      if (!match) return [9, 0];
      let expanded = 0;
      for (let i = 1; i <= wildcards; i++) expanded += Array.from(match[i] || "").length;
      return [0, expanded];
    } catch (_) { return [9, 0]; }
  }
  function suggestionRank(word, query, mode, alternatives) {
    const quality = [formRank(word, query)];
    let modeRank = [0, 0, 0];
    const headTokens = tokens(word), queryTokens = tokens(query);
    const exactPhrase = phrasePosition(headTokens, queryTokens);
    switch (mode) {
      case "prefix": {
        if (exactPhrase >= 0) modeRank = [0, exactPhrase, 0];
        else if (caseFold(word).startsWith(caseFold(query))) modeRank = [1, 0, 0];
        else modeRank = [2, ...tokenMatchRank(headTokens, queryTokens,
          (token, part) => token === part ? 0 : token.startsWith(part) ? 1 : -1)];
        break;
      }
      case "suffix": {
        const suffixPhrase = phrasePosition(headTokens, queryTokens, true);
        const tokenRank = tokenMatchRank(headTokens, queryTokens,
          (token, part) => token === part ? 0 : token.endsWith(part) ? 1 : -1);
        if (suffixPhrase >= 0) modeRank = [0, headTokens.length - suffixPhrase - queryTokens.length];
        else if (tokenRank[0] < 9) modeRank = [tokenRank[0], tokenRank[2]];
        else if (caseFold(word).endsWith(caseFold(query))) modeRank = [1, 0];
        else modeRank = [9, 0];
        break;
      }
      case "contains": {
        if (exactPhrase >= 0) modeRank = [0, exactPhrase, 0];
        else modeRank = [1, ...tokenMatchRank(headTokens, queryTokens,
          (token, part) => token === part ? 0 : token.startsWith(part) ? 1 : token.includes(part) ? 2 : -1)];
        break;
      }
      case "word": case "wordLemma": case "exactLemma": {
        const candidates = [query, ...(alternatives || []).filter(value => value && value !== query)];
        modeRank = candidates.map((value, i) => {
          const position = phrasePosition(headTokens, tokens(value));
          return position < 0 ? [9, i, 0] : [i, position, 0];
        }).sort(compareRank)[0] || [9, 0, 0];
        break;
      }
      case "wildcards":
        modeRank = wildcardExpansion(word, query);
        break;
      default:
        if (exactPhrase >= 0) modeRank = [0, exactPhrase, 0];
    }
    const queryLength = Array.from(whitespaceFold(query)).length;
    const wordLength = Array.from(whitespaceFold(word)).length;
    return [...quality, modeRank[0] ?? 0, headTokens.length, ...modeRank.slice(1), Math.max(0, wordLength - queryLength)];
  }
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
  let draft = false, open = false, heads = [], headsByDict = new Map(), picking = false, search = () => {}, suggest = () => {};
  let suggestAC = null, suggestSeq = 0;
  function stopSuggest() {
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
    if (matchMedia("(max-width:600px)").matches) {
      const pill = document.querySelector("#frm>.pill");
      const extra = pill.getBoundingClientRect().right - $("qbox").getBoundingClientRect().right;
      box.style.right = -Math.max(0, extra - 1) + "px";
    } else {
      box.style.right = "";
    }
    const viewport = window.visualViewport;
    const viewportBottom = viewport ? viewport.offsetTop + viewport.height : document.documentElement.clientHeight;
    const dock = $("bottomDock");
    const dockVisible = dock && !dock.hidden;
    const availableBottom = dockVisible
      ? Math.min(viewportBottom, dock.getBoundingClientRect().top)
      : viewportBottom;
    const popupTop = $("qbox").getBoundingClientRect().bottom + 4;
    box.style.maxHeight = Math.max(0, availableBottom - popupTop - (dockVisible ? 10 : 4)) + "px";
  }
  function render(filter = "") {
    if (!open) return;
    const box = $("searchHistory");
    size();
    box.replaceChildren();
    const needle = key(filter.trim()), seen = new Set();
    const typed = !!needle;
    const maxRows = typed ? suggestionListLimit() : Math.max(20, limit());
    const history = activeHistory().slice(0, limit());
    const words = typed ? heads : history;
    for (const word of words) {
      const id = key(word);
      if (seen.has(id)) continue;
      seen.add(id);
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = word;
      button.addEventListener("click", () => { picking = false; hide(); search(word, {kind:typed?"suggestion":"history"}); });
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
    headsByDict = new Map();
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
    headsByDict = new Map();
    show(value);
    const q = value.trim();
    const mode = $("mode").value;
    if ((q.length < 2 && !(mode === "wildcards" && /[*?]/.test(q))) || mode === "fts") return;
    const seq = suggestSeq;
    suggestAC = new AbortController();
    Promise.resolve(suggest(q, (results, resultMode, dict) => {
      if (seq === suggestSeq && mode === $("mode").value && $("q").value.trim() === q)
        addResults(q, results, resultMode, dict);
    }, suggestAC.signal, mode)).catch(err => {
      if (err?.name !== "AbortError") console.warn("Suggestions:", err);
    });
  }
  function suggestionListLimit() {
    return Math.max(1, Math.min(500, Number(window.wudictSuggestionListLimit) || 500));
  }
  function addResults(q, results, mode, dict, term) {
    if (mode === "fts" || !open || $("q").value.trim() !== q || !results?.length) return;
    const id = String(dict || "");
    if (!id) return;
    const alternatives = String(term || q).split("\x1f").filter(Boolean);
    const ranked = results.map(result => String(result.Headword || "").trim()).filter(Boolean)
      .map(word => ({word, rank:suggestionRank(word, q, mode, alternatives)}));
    ranked.sort((a, b) => compareRank(a.rank, b.rank) || suggestionCollator.compare(a.word, b.word) || a.word.localeCompare(b.word));
    const perDictionary = Math.max(1, Math.min(500, Number(window.wudictSuggestionPerDictionary) || 40));
    headsByDict.set(id, ranked.slice(0, perDictionary));
    const best = new Map();
    for (const rows of headsByDict.values()) for (const row of rows) {
      const folded = key(row.word), previous = best.get(folded);
      if (!previous || compareRank(row.rank, previous.rank) < 0) best.set(folded, row);
    }
    const all = [...best.values()];
    all.sort((a, b) => compareRank(a.rank, b.rank) || suggestionCollator.compare(a.word, b.word) || a.word.localeCompare(b.word));
    heads = all.slice(0, suggestionListLimit()).map(row => row.word);
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
    window.wudictArticleFind?.setHistoryLimit?.(n);
    if (!n) hide();
  });
  $("q").addEventListener("focus", () => { if (!open) refreshSuggestions(); });
  $("q").addEventListener("pointerdown", () => { if (!open) refreshSuggestions(); });
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
  // The bottom dock changes height as its status and progress rows appear.
  // Keep an open list above its current upper edge.
  const observeDock = () => {
    const dock = $("bottomDock");
    if (dock) sizer.observe(dock);
  };
  if (document.readyState === "loading")
    document.addEventListener("DOMContentLoaded", observeDock, {once:true});
  else observeDock();
  window.addEventListener("resize", () => { if (open) size(); });
  window.addEventListener("scroll", () => { if (open) size(); }, true);
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", () => { if (open) size(); });
    window.visualViewport.addEventListener("scroll", () => { if (open) size(); });
  }

  window.wuSearchHistory = { setSearch(fn) { search = fn; }, setSuggest(fn) { suggest = fn; }, record, recordFor,
    getFor(kind) { return [...(kind === "wildcards" ? wildcardsSaved : saved)]; },
    hide, onInput, refreshSuggestions, modeChanged, isOpen() { return open; }, isPicking() { return picking; } };
})();
