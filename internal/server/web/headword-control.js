/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

(function () {
  "use strict";

  const thresholdKey = "wudict_browse_mask_threshold";
  const historyKeys = Object.freeze({search:"wudict_search_history_v2", wildcards:"wudict_wildcards_history_v2"});
  const thresholds = [-1, 0, 10000, 50000, 100000, 500000, 1000000];
  const icons = {
    search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 5 5"/></svg>',
    filter: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M3 5h18l-7 8v5l-4 2v-7z"/></svg>'
  };
  let ribbon = null;
  function cacheRead(key, ttl = 30 * 60 * 1000) {
    try {
      const saved = JSON.parse(sessionStorage.getItem(key) || "null");
      return saved && Date.now() - saved.savedAt <= ttl ? saved.value : null;
    } catch (_) { return null; }
  }
  function cacheWrite(key, value) {
    try { sessionStorage.setItem(key, JSON.stringify({savedAt:Date.now(),value})); return true; }
    catch (_) { return false; }
  }

  function threshold() {
    try {
      const saved = localStorage.getItem(thresholdKey);
      if (saved === null) return 100000;
      const value = Number(saved);
      return thresholds.includes(value) ? value : 100000;
    } catch (_) { return 100000; }
  }
  function setThreshold(value) {
    value = Number(value);
    if (!thresholds.includes(value)) return false;
    try { localStorage.setItem(thresholdKey, String(value)); } catch (_) { return false; }
    window.dispatchEvent(new CustomEvent("wudict:headword-threshold-change", {detail:value}));
    return true;
  }
  function historyLimit() {
    try {
      const raw = localStorage.getItem("wudict_history_length");
      if (raw === null) return 20;
      const value = Number(raw);
      return Number.isInteger(value) && value >= 0 && value <= 100 ? value : 20;
    } catch (_) { return 20; }
  }
  function historyKey(value) { return String(value).normalize("NFC").toLocaleLowerCase(); }
  function readHistory(kind) {
    try {
      if (window.wuSearchHistory?.getFor) return window.wuSearchHistory.getFor(kind).slice(0, historyLimit());
      const limit = historyLimit();
      if (!limit) return [];
      const values = JSON.parse(localStorage.getItem(historyKeys[kind]) || "[]");
      if (!Array.isArray(values)) return [];
      const seen = new Set(), rows = [];
      for (const value of values) {
        if (typeof value !== "string" || !value.trim()) continue;
        const key = historyKey(value);
        if (seen.has(key)) continue;
        seen.add(key);
        rows.push(value);
        if (rows.length >= limit) break;
      }
      return rows;
    } catch (_) { return []; }
  }
  function recordHistory(kind, value) {
    const word = String(value || "").trim(), limit = historyLimit();
    if (!word || !limit) return;
    if (window.wuSearchHistory?.recordFor) { window.wuSearchHistory.recordFor(kind, word); return; }
    const key = historyKey(word), rows = [word, ...readHistory(kind).filter(item => historyKey(item) !== key)].slice(0, limit);
    try { localStorage.setItem(historyKeys[kind], JSON.stringify(rows)); } catch (_) {}
  }

  const findModeGroups = Object.freeze([
    {key:"eachWord", modes:["prefix", "suffix", "contains", "word", "wordLemma"]},
    {key:"wholeHeadword", modes:["exact", "exactLemma", "wildcards"]}
  ]);
  const findModeIcons = Object.freeze({prefix:"ab→", suffix:"→ab", contains:"ab", word:"·ab·", wordLemma:"·ab·+", exact:"=", exactLemma:"=+", wildcards:"*?"});
  function create({dock, ids, labels, maxRows = 2, countAction = false, onQuery, onMode, onFindMode, onThreshold, onApply, onStep, onCount}) {
    if (!dock || !ids) throw new TypeError("headword control needs a dock and element ids");
    const id = name => ids[name];
    dock.classList.add("headword-control");
    dock.innerHTML = `<button type="button" class="headword-control-mode" id="${id("mode")}"></button>` +
      `<span class="headword-control-find-picker"><button type="button" class="headword-control-find-button" id="${id("mask")}" aria-haspopup="menu" aria-expanded="false"></button>` +
      `<span class="headword-control-find-menu" role="menu" hidden></span></span>` +
      `<input class="headword-control-input" id="${id("input")}" type="search" placeholder="${labels.search}" ` +
      `spellcheck="false" autocomplete="off" autocapitalize="off" enterkeyhint="search" aria-label="${labels.search}">` +
      `<button type="button" class="headword-control-apply" id="${id("apply")}" aria-label="${labels.apply}" title="${labels.apply}" hidden>✓</button>` +
      (countAction ? `<button type="button" class="headword-control-count headword-control-count-button" id="${id("count")}" aria-live="polite">0/0</button>` :
        `<output class="headword-control-count" id="${id("count")}" aria-live="polite">0/0</output>`) +
      `<button type="button" class="headword-control-step headword-control-previous" id="${id("previous")}" aria-label="${labels.previous}" title="${labels.previous}">▲</button>` +
      `<button type="button" class="headword-control-step headword-control-next" id="${id("next")}" aria-label="${labels.next}" title="${labels.next}">▼</button>`;

    const el = Object.fromEntries(Object.entries(ids).map(([name, value]) => [name, document.getElementById(value)]));
    el.findPicker = el.mask.closest(".headword-control-find-picker");
    el.findMenu = el.findPicker.querySelector(".headword-control-find-menu");
    el.history = document.createElement("div");
    el.history.className = "headword-control-history";
    el.history.setAttribute("role", "group");
    el.history.setAttribute("aria-label", labels.history || "Recent searches");
    el.history.hidden = true;
    let findMenuOpen = false;
    let historyOpen = false, historyDraft = false, historyPicking = false;
    const findModes = labels.findModes || {};
    let selectedFindMode = "prefix";
    const singleWordModes = new Set(["prefix", "suffix", "contains", "word", "wordLemma"]);
    function firstWord(value) { return String(value || "").match(/[^\s]+/u)?.[0] || ""; }
    function normalizeSingleWord(mode, notify = false) {
      if (!singleWordModes.has(mode)) return false;
      const value = el.input.value, word = firstWord(value);
      if (word === value) return false;
      const cursor = el.input.selectionStart ?? value.length;
      const start = value.indexOf(word);
      el.input.value = word;
      const nextCursor = Math.min(word.length, Math.max(0, cursor - Math.max(0, start)));
      try { el.input.setSelectionRange(nextCursor, nextCursor); } catch (_) {}
      if (notify) onQuery?.(word);
      return true;
    }
    for (const group of findModeGroups) {
      const section = document.createElement("div");
      section.className = "headword-control-find-group";
      const heading = document.createElement("div");
      heading.className = "headword-control-find-group-title";
      heading.textContent = labels.findModeGroups?.[group.key] || group.key;
      section.append(heading);
      for (const mode of group.modes) {
        const item = document.createElement("button");
        item.type = "button";
        item.className = "headword-control-find-option";
        item.dataset.findMode = mode;
        item.setAttribute("role", "menuitemradio");
        const icon = document.createElement("span");
        icon.className = "headword-control-find-option-icon";
        icon.textContent = findModeIcons[mode];
        const copy = document.createElement("span");
        copy.className = "headword-control-find-option-copy";
        const title = document.createElement("span");
        title.className = "headword-control-find-option-title";
        title.textContent = findModes[mode]?.title || findModes[mode]?.description || mode;
        copy.append(title);
        if (findModes[mode]?.hint) {
          const hint = document.createElement("small");
          hint.className = "headword-control-find-option-hint";
          hint.textContent = findModes[mode].hint;
          copy.append(hint);
        }
        item.append(icon, copy);
        item.addEventListener("click", () => {
          if (el.mask.disabled) return;
          hideHistory();
          closeFindMenu();
          if (selectedFindMode !== mode) {
            normalizeSingleWord(mode);
            selectedFindMode = mode;
            recordCurrentHistory();
            onFindMode?.(mode);
          }
        });
        section.append(item);
      }
      el.findMenu.append(section);
    }
    const canvas = document.createElement("canvas").getContext("2d");
    function fit() {
      if (dock.hidden || dock.parentElement?.hidden) return;
      const inputStyle = getComputedStyle(el.input), dockStyle = getComputedStyle(dock);
      canvas.font = inputStyle.font;
      const textWidth = Math.max(canvas.measureText("xxxxxxxxxxxxxxx").width,
        canvas.measureText("жжжжжжжжжжжжжжж").width);
      const inputPadding = parseFloat(inputStyle.paddingLeft || 0) + parseFloat(inputStyle.paddingRight || 0);
      const gap = parseFloat(dockStyle.columnGap) || 0;
      const controls = [el.mode, el.findPicker, el.apply, el.count, el.previous, el.next].filter(item => !item.hidden);
      const controlsWidth = controls.reduce((sum, item) => sum + item.getBoundingClientRect().width, 0) + gap * controls.length;
      const sidePadding = parseFloat(dockStyle.paddingLeft || 0) + parseFloat(dockStyle.paddingRight || 0);
      const available = dock.clientWidth - sidePadding;
      const rowControls = [el.mode, el.findPicker, el.count, el.previous, el.next].filter(item => !item.hidden);
      const rowWidth = rowControls.reduce((sum, item) => sum + item.getBoundingClientRect().width, 0) + gap * rowControls.length;
      const threeRows = maxRows >= 3 && available + .5 < rowWidth;
      const twoRows = threeRows || available + .5 < controlsWidth + textWidth + inputPadding;
      if (twoRows) dock.setAttribute("data-two-rows", "1"); else dock.removeAttribute("data-two-rows");
      if (threeRows) dock.setAttribute("data-three-rows", "1"); else dock.removeAttribute("data-three-rows");
      if (dock.id === "maskDock") document.body.style.setProperty("--mask-dock-height", dock.parentElement.getBoundingClientRect().height + "px");
    }
    function closeFindMenu() {
      el.findMenu.hidden = true;
      if (el.findMenu.parentElement !== el.findPicker) el.findPicker.append(el.findMenu);
      el.findMenu.style.removeProperty("left");
      el.findMenu.style.removeProperty("top");
      el.findMenu.style.removeProperty("max-height");
      findMenuOpen = false;
      el.mask.setAttribute("aria-expanded", "false");
    }
    function historyKind() { return selectedFindMode === "wildcards" ? "wildcards" : "search"; }
    function hideHistory() {
      el.history.hidden = true;
      el.history.remove();
      el.history.style.removeProperty("left");
      el.history.style.removeProperty("top");
      el.history.style.removeProperty("width");
      el.history.style.removeProperty("max-height");
      historyOpen = false;
    }
    function positionHistory() {
      if (!historyOpen || el.history.hidden) return;
      const anchor = el.input.getBoundingClientRect(), gap = 4, edge = 8;
      const above = Math.max(0, anchor.top - gap - edge);
      const below = Math.max(0, window.innerHeight - anchor.bottom - gap - edge);
      const placeAbove = above > below, room = placeAbove ? above : below;
      const width = Math.max(0, Math.min(Math.max(anchor.width, 224), window.innerWidth - edge * 2));
      el.history.style.width = `${width}px`;
      el.history.style.maxHeight = `${Math.max(0, Math.min(320, window.innerHeight * .5, room))}px`;
      const box = el.history.getBoundingClientRect();
      const left = Math.max(edge, Math.min(anchor.left, window.innerWidth - box.width - edge));
      const top = placeAbove ? Math.max(edge, anchor.top - gap - box.height) : anchor.bottom + gap;
      el.history.style.left = `${left}px`;
      el.history.style.top = `${top}px`;
    }
    function renderHistory(filter = "") {
      if (!historyOpen) return;
      el.history.replaceChildren();
      const query = historyKey(filter.trim());
      for (const value of readHistory(historyKind())) {
        if (query && !historyKey(value).includes(query)) continue;
        const option = document.createElement("button");
        option.type = "button";
        option.textContent = value;
        option.addEventListener("click", () => {
          historyPicking = false;
          historyDraft = false;
          el.input.value = value;
          hideHistory();
          onQuery?.(value);
        });
        el.history.append(option);
      }
      el.history.hidden = !el.history.childElementCount;
      if (el.history.hidden) return;
      if (!el.history.isConnected) document.body.append(el.history);
      positionHistory();
    }
    function showHistory(filter = "") {
      if (el.input.disabled) return;
      closeFindMenu();
      historyOpen = true;
      renderHistory(filter);
    }
    function recordCurrentHistory(minLength = 1) {
      const value = el.input.value.trim();
      if (value.length >= minLength) recordHistory(historyKind(), value);
      historyDraft = false;
    }
    function positionFindMenu() {
      if (!findMenuOpen) return;
      const anchor = el.mask.getBoundingClientRect();
      const gap = 4, edge = 8;
      const above = Math.max(0, anchor.top - gap - edge);
      const below = Math.max(0, window.innerHeight - anchor.bottom - gap - edge);
      const placeAbove = above >= below;
      const room = placeAbove ? above : below;
      el.findMenu.style.maxHeight = `${Math.max(0, Math.min(room, window.innerHeight - edge * 2))}px`;
      const menu = el.findMenu.getBoundingClientRect();
      const left = Math.max(edge, Math.min(anchor.left, window.innerWidth - menu.width - edge));
      const top = placeAbove ? Math.max(edge, anchor.top - gap - menu.height) : anchor.bottom + gap;
      el.findMenu.style.left = `${left}px`;
      el.findMenu.style.top = `${top}px`;
    }
    function paint(state = {}) {
      const filtering = !!state.filtering;
      selectedFindMode = findModeIcons[state.findMode] ? state.findMode : "prefix";
      el.mode.innerHTML = filtering ? icons.filter : icons.search;
      el.mode.classList.toggle("is-on", filtering);
      el.mode.setAttribute("aria-pressed", String(filtering));
      el.mode.setAttribute("aria-label", filtering ? labels.searchMode : labels.filterMode);
      el.mode.title = el.mode.getAttribute("aria-label");
      el.mask.textContent = findModeIcons[selectedFindMode];
      el.mask.setAttribute("aria-label", findModes[selectedFindMode]?.title || findModes[selectedFindMode]?.description || selectedFindMode);
      el.mask.title = el.mask.getAttribute("aria-label");
      for (const item of el.findMenu.querySelectorAll("[data-find-mode]")) {
        const active = item.dataset.findMode === selectedFindMode;
        item.setAttribute("aria-checked", String(active));
        item.classList.toggle("is-selected", active);
      }
      if (historyOpen) renderHistory(el.input.value);
      el.input.placeholder = filtering ? labels.filter : labels.search;
      el.input.setAttribute("aria-label", el.input.placeholder);
      el.apply.hidden = !state.applyRequired;
      el.apply.disabled = el.apply.hidden || !state.applyEnabled || !!state.busy;
      el.count.textContent = state.count ?? "0/0";
      if (countAction) {
        el.count.disabled = !!state.countDisabled;
        el.count.setAttribute("aria-label", state.countLabel || el.count.textContent);
        el.count.title = state.countLabel || "";
      }
      else el.count.value = el.count.textContent;
      el.previous.disabled = !!state.previousDisabled;
      el.next.disabled = !!state.nextDisabled;
      for (const control of [el.mode, el.mask]) control.disabled = !!state.busy;
      if (state.busy) { closeFindMenu(); hideHistory(); }
      el.input.disabled = !!state.busy && !state.allowInputWhileBusy;
      fit();
    }
    function onInput() {
      historyDraft = true;
      normalizeSingleWord(selectedFindMode);
      if (historyOpen) renderHistory(el.input.value);
      onQuery?.(el.input.value);
    }
    function onBeforeInput(event) {
      if (singleWordModes.has(selectedFindMode) && event.inputType?.startsWith("insert") && /\s/u.test(event.data || "")) {
        event.preventDefault();
      }
    }
    function onPaste(event) {
      if (!singleWordModes.has(selectedFindMode)) return;
      event.preventDefault();
      const word = firstWord(event.clipboardData?.getData("text/plain") || "");
      if (!word) return;
      const start = el.input.selectionStart ?? el.input.value.length;
      const end = el.input.selectionEnd ?? start;
      el.input.setRangeText(word, start, end, "end");
      onInput();
    }
    function onModeClick() { recordCurrentHistory(); hideHistory(); onMode?.(); }
    function onMaskClick() {
      hideHistory();
      if (!el.findMenu.hidden) { closeFindMenu(); return; }
      findMenuOpen = true;
      document.body.append(el.findMenu);
      el.findMenu.hidden = false;
      el.mask.setAttribute("aria-expanded", "true");
      positionFindMenu();
      el.findMenu.querySelector(`[data-find-mode="${selectedFindMode}"]`)?.focus();
    }
    function onFindMenuKey(event) {
      if (event.key === "Escape") { closeFindMenu(); el.mask.focus(); }
      else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        const items = [...el.findMenu.querySelectorAll("button")];
        const index = items.indexOf(document.activeElement);
        items[(index + (event.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus();
        event.preventDefault();
      }
    }
    function onOutsideFindMenu(event) { if (!el.findPicker.contains(event.target) && !el.findMenu.contains(event.target)) closeFindMenu(); }
    function onHistoryClickOutside(event) {
      if (!el.input.contains(event.target) && !el.history.contains(event.target)) hideHistory();
    }
    function onHistoryPointerDown(event) { historyPicking = true; event.preventDefault(); }
    function onHistoryKeydown(event) {
      if (event.key === "Escape") { hideHistory(); el.input.focus(); return; }
      if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
      const items = [...el.history.querySelectorAll("button")], index = items.indexOf(document.activeElement);
      const next = index < 0 ? (event.key === "ArrowDown" ? 0 : items.length - 1) :
        (index + (event.key === "ArrowDown" ? 1 : items.length - 1)) % items.length;
      historyPicking = true;
      items[next]?.focus();
      event.preventDefault();
    }
    function onInputBlur() {
      setTimeout(() => {
        if (historyPicking) { historyPicking = false; return; }
        if (historyDraft) recordCurrentHistory(2);
        hideHistory();
      }, 0);
    }
    function onInputKeydown(event) {
      if (event.key === "Escape" && historyOpen) { hideHistory(); return; }
      if (event.key === "ArrowDown" && historyOpen && !el.history.hidden) {
        event.preventDefault();
        historyPicking = true;
        el.history.querySelector("button")?.focus();
      }
    }
    function onInputFocus() { showHistory(); }
    function onInputPointerDown() { showHistory(el.input.value); }
    function onThresholdChange(event) { onThreshold?.(Number(event?.detail ?? threshold())); }
    function onStorageChange(event) { if (event.key === thresholdKey) onThreshold?.(threshold()); }
    function onApplyClick() { recordCurrentHistory(); hideHistory(); onApply?.(); }
    function onPreviousClick() { onStep?.(-1); }
    function onNextClick() { onStep?.(1); }
    function onCountClick() { onCount?.(); }
    el.input.addEventListener("input", onInput);
    el.input.addEventListener("focus", onInputFocus);
    el.input.addEventListener("pointerdown", onInputPointerDown);
    el.input.addEventListener("blur", onInputBlur);
    el.input.addEventListener("keydown", onInputKeydown);
    el.input.addEventListener("beforeinput", onBeforeInput);
    el.input.addEventListener("paste", onPaste);
    el.mode.addEventListener("click", onModeClick);
    el.mask.addEventListener("click", onMaskClick);
    el.findMenu.addEventListener("keydown", onFindMenuKey);
    el.history.addEventListener("pointerdown", onHistoryPointerDown);
    el.history.addEventListener("keydown", onHistoryKeydown);
    document.addEventListener("pointerdown", onOutsideFindMenu);
    document.addEventListener("pointerdown", onHistoryClickOutside);
    window.addEventListener("resize", positionFindMenu);
    window.addEventListener("resize", positionHistory);
    window.addEventListener("scroll", positionFindMenu, true);
    window.addEventListener("scroll", positionHistory, true);
    el.apply.addEventListener("click", onApplyClick);
    el.previous.addEventListener("click", onPreviousClick);
    el.next.addEventListener("click", onNextClick);
    if (countAction) el.count.addEventListener("click", onCountClick);
    const observer = new ResizeObserver(fit);
    observer.observe(dock);
    if (dock.id === "maskDock") observer.observe(dock.parentElement);
    window.addEventListener("wudict:headword-threshold-change", onThresholdChange);
    window.addEventListener("storage", onStorageChange);
    addEventListener("resize", fit);

    return Object.freeze({
      elements: el,
      threshold,
      needsApply: total => threshold() !== -1 && Number(total || 0) >= threshold(),
      cacheRead,
      cacheWrite,
      paint,
      fit,
      destroy() {
        closeFindMenu();
        hideHistory();
        observer.disconnect();
        window.removeEventListener("wudict:headword-threshold-change", onThresholdChange);
        window.removeEventListener("storage", onStorageChange);
        removeEventListener("resize", fit);
        el.input.removeEventListener("input", onInput);
        el.input.removeEventListener("focus", onInputFocus);
        el.input.removeEventListener("pointerdown", onInputPointerDown);
        el.input.removeEventListener("blur", onInputBlur);
        el.input.removeEventListener("keydown", onInputKeydown);
        el.input.removeEventListener("beforeinput", onBeforeInput);
        el.input.removeEventListener("paste", onPaste);
        el.mode.removeEventListener("click", onModeClick);
        el.mask.removeEventListener("click", onMaskClick);
        el.findMenu.removeEventListener("keydown", onFindMenuKey);
        el.history.removeEventListener("pointerdown", onHistoryPointerDown);
        el.history.removeEventListener("keydown", onHistoryKeydown);
        document.removeEventListener("pointerdown", onOutsideFindMenu);
        document.removeEventListener("pointerdown", onHistoryClickOutside);
        window.removeEventListener("resize", positionFindMenu);
        window.removeEventListener("resize", positionHistory);
        window.removeEventListener("scroll", positionFindMenu, true);
        window.removeEventListener("scroll", positionHistory, true);
        el.apply.removeEventListener("click", onApplyClick);
        el.previous.removeEventListener("click", onPreviousClick);
        el.next.removeEventListener("click", onNextClick);
        if (countAction) el.count.removeEventListener("click", onCountClick);
        dock.replaceChildren();
        dock.classList.remove("headword-control");
      }
    });
  }

  window.wudictHeadwordControl = Object.freeze({create, cacheRead, cacheWrite, threshold, setThreshold,
    registerRibbon(api){if(api&&typeof api.measure==="function"&&typeof api.paint==="function")ribbon=api},
    get ribbon(){return ribbon}});
})();
