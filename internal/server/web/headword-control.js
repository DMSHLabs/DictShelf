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
      if (saved === null) return window.wudictAndroidGo || window.wudictAndroidShell ? 10000 : 50000;
      const value = Number(saved);
      return thresholds.includes(value) ? value : (window.wudictAndroidGo || window.wudictAndroidShell ? 10000 : 50000);
    } catch (_) { return window.wudictAndroidGo || window.wudictAndroidShell ? 10000 : 50000; }
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

  /* ── the prunable find menu ──────────────────────────────────────────
     A gear in the menu's top-right corner turns its rows into checkboxes: a
     tick keeps a command, an empty box hides it, and the tick and the cross
     leave the editing (the tick also applies, the cross puts back what was
     there before the gear). One hidden set of mode VALUES and one set of
     hidden section names are shared by every find menu - this component's,
     which the browse window and the mask dock both build, and the search
     menu's own builder in index.html - because it is one list of modes the
     reader means to keep: off in one window, off in the others. The store is
     localStorage, like every other choice about this window. */
  const hiddenModesKey = "wudict_hidden_find_modes";
  const hiddenHeadingsKey = "wudict_hidden_find_headings";
  /* The same gear the Dictionary groups panel's edit button wears
     (#groupEditToggle in index.html): one drawn cog with an inner circle, so the
     two places that open a panel for editing a list are recognisably the same
     kind of control. Both menus that show it - this one and the mode menu's own
     builder in index.html - take it from here. */
  const gearIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="3"/><path d="M10 2h4l.5 2.4 1.8.8 2.1-1.3 1.7 1.7-1.3 2.1.8 1.8L22 10v4l-2.4.5-.8 1.8 1.3 2.1-1.7 1.7-2.1-1.3-1.8.8L14 22h-4l-.5-2.4-1.8-.8-2.1 1.3-1.7-1.7 1.3-2.1-.8-1.8L2 14v-4l2.4-.5.8-1.8-1.3-2.1 1.7-1.7 2.1 1.3 1.8-.8Z"/></svg>';
  function readNames(key) {
    try {
      const value = JSON.parse(localStorage.getItem(key) || "[]");
      return new Set(Array.isArray(value) ? value.filter(item => typeof item === "string") : []);
    } catch (_) { return new Set(); }
  }
  function writeNames(key, values) {
    try { localStorage.setItem(key, JSON.stringify([...values])); } catch (_) {}
  }
  /* One icon column for a whole menu, sized to the widest mark that menu is
     actually showing, so every description begins at the same place AND the
     column is no wider than this menu needs. A fixed column of 4.4em - what the
     stylesheet used to carry - did the first without the second: it was sized
     for the widest mark there is, "[A ↔ B]", and left a hole in front of "=" and
     "ab", worse the more of the long marks were hidden. Hugging each mark did
     the second without the first, starting each description at a different
     place.
     Measured from the text's own box in the DOM, not with a canvas: canvas
     resolved a different face for these marks - "[A ↔ B]" measured 34.5px there
     and occupies 48.3px on screen - which is how the widest marks came to be
     overlapped by the descriptions beside them. Module scope, and exposed on
     wudictHeadwordControl, because the mode menu's rows are built by index.html
     rather than here. */
  function measureIconColumn(menu) {
    if (!menu) return;
    const icons = menu.querySelectorAll(".headword-control-find-option-icon");
    if (!icons.length) { menu.style.removeProperty("--find-icon-column"); return; }
    let widest = 0;
    const range = document.createRange();
    for (const icon of icons) {
      range.selectNodeContents(icon);
      widest = Math.max(widest, range.getBoundingClientRect().width);
    }
    /* A menu that is still hidden measures nothing: leave the column as it is
       and let the open that shows it measure for real. */
    if (widest < 1) return;
    /* Plus one to two spaces of the row's own font: the gap between the longest
       mark and the description. Half of it here, the rest is the grid's own
       .2em gap, so the widest mark ends up about two spaces from its text. */
    menu.style.setProperty("--find-icon-column", "calc(" + Math.ceil(widest) + "px + .15em)");
  }
  /* A menu never renders empty: if the shared set would hide every command of
     the menu it is read in, that menu shows them all rather than a bare bar. */
  function visibleModes(values, hidden) {
    const shown = values.filter(value => !hidden.has(value));
    return shown.length ? shown : values.slice();
  }
  const findMenuPrefs = Object.freeze({
    hiddenModes() { return readNames(hiddenModesKey); },
    hiddenHeadings() { return readNames(hiddenHeadingsKey); },
    save(hiddenModes, hiddenHeadings) {
      writeNames(hiddenModesKey, hiddenModes);
      writeNames(hiddenHeadingsKey, hiddenHeadings);
    },
    visibleModes
  });
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
    const allFindModes = findModeGroups.flatMap(group => group.modes);
    const customizeLabel = labels.customizeMenu || "Customize this menu";
    const customizeApplyLabel = labels.customizeApply || "Apply";
    const customizeCancelLabel = labels.customizeCancel || "Cancel";
    let customizing = false, draftHiddenModes = null, draftHiddenHeadings = null, adjustingMode = false;
    function findOptionCopy(mode) {
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
      return copy;
    }
    function findOptionIcon(mode) {
      const icon = document.createElement("span");
      icon.className = "headword-control-find-option-icon";
      icon.textContent = findModeIcons[mode];
      return icon;
    }
    function findOptionRow(mode) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "headword-control-find-option";
      item.dataset.findMode = mode;
      item.setAttribute("role", "menuitemradio");
      item.append(findOptionIcon(mode), findOptionCopy(mode));
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
      return item;
    }
    /* One drawn box, shared by a command's row and a section's name row: the
       app's own checkbox control is not loaded in every window this menu opens
       in (browse.html loads only this stylesheet and workspace.css), so the
       look is drawn here from the same custom properties. */
    function findCheckBox() {
      const box = document.createElement("span");
      box.className = "headword-control-find-box";
      return box;
    }
    function findCustomizeOption(mode, hidden) {
      const checked = !hidden.has(mode);
      const item = document.createElement("button");
      item.type = "button";
      item.className = "headword-control-find-option is-customizing";
      item.dataset.customizeMode = mode;
      item.setAttribute("role", "checkbox");
      item.setAttribute("aria-checked", String(checked));
      /* The last kept command cannot be dropped, so a menu is never left with
         nothing to search by. */
      item.disabled = checked && allFindModes.filter(value => !hidden.has(value)).length === 1;
      item.append(findCheckBox(), findOptionIcon(mode), findOptionCopy(mode));
      item.addEventListener("click", () => {
        if (hidden.has(mode)) hidden.delete(mode); else hidden.add(mode);
        renderFindMenu();
      });
      return item;
    }
    function findCustomizeHeading(group, hidden) {
      const row = document.createElement("button");
      row.type = "button";
      row.className = "headword-control-find-group-title is-customizing headword-control-find-action";
      row.dataset.customizeHeading = group.key;
      row.setAttribute("role", "checkbox");
      row.setAttribute("aria-checked", String(!hidden.has(group.key)));
      const text = document.createElement("span");
      text.textContent = labels.findModeGroups?.[group.key] || group.key;
      row.append(findCheckBox(), text);
      row.addEventListener("click", () => {
        if (hidden.has(group.key)) hidden.delete(group.key); else hidden.add(group.key);
        renderFindMenu();
      });
      return row;
    }
    function findCustomizeBar() {
      const bar = document.createElement("div");
      bar.className = "headword-control-find-bar";
      if (customizing) {
        const apply = document.createElement("button");
        apply.type = "button";
        apply.className = "headword-control-find-action headword-control-find-confirm";
        apply.textContent = "✓";
        apply.setAttribute("aria-label", customizeApplyLabel);
        apply.title = customizeApplyLabel;
        apply.addEventListener("click", applyFindCustomize);
        const cancel = document.createElement("button");
        cancel.type = "button";
        cancel.className = "headword-control-find-action headword-control-find-cancel";
        cancel.textContent = "✕";
        cancel.setAttribute("aria-label", customizeCancelLabel);
        cancel.title = customizeCancelLabel;
        cancel.addEventListener("click", cancelFindCustomize);
        bar.append(apply, cancel);
      } else {
        const gear = document.createElement("button");
        gear.type = "button";
        gear.className = "headword-control-find-action headword-control-find-customize";
        gear.setAttribute("aria-label", customizeLabel);
        gear.title = customizeLabel;
        gear.innerHTML = gearIcon;
        gear.addEventListener("click", () => {
          customizing = true;
          draftHiddenModes = findMenuPrefs.hiddenModes();
          draftHiddenHeadings = findMenuPrefs.hiddenHeadings();
          renderFindMenu();
        });
        bar.append(gear);
      }
      return bar;
    }
    function markFindSelection() {
      for (const item of el.findMenu.querySelectorAll("[data-find-mode]")) {
        const active = item.dataset.findMode === selectedFindMode;
        item.setAttribute("aria-checked", String(active));
        item.classList.toggle("is-selected", active);
      }
    }
    /* Rebuilt on every open, so a set changed in another window is already in
       force the next time the menu is shown. In the editing state nothing is
       filtered out - the reader must see every command to put it back - and
       each row carries a checkbox instead of selecting its mode. */
    function renderFindMenu() {
      const hidden = customizing ? draftHiddenModes : findMenuPrefs.hiddenModes();
      const hiddenHeadings = customizing ? draftHiddenHeadings : findMenuPrefs.hiddenHeadings();
      const shownSet = new Set(customizing ? allFindModes : visibleModes(allFindModes, hidden));
      const nodes = [findCustomizeBar()];
      for (const group of findModeGroups) {
        const modes = group.modes.filter(mode => shownSet.has(mode));
        if (!modes.length) continue;
        const section = document.createElement("div");
        section.className = "headword-control-find-group";
        if (customizing) section.append(findCustomizeHeading(group, hiddenHeadings));
        else if (!hiddenHeadings.has(group.key)) {
          const heading = document.createElement("div");
          heading.className = "headword-control-find-group-title";
          heading.textContent = labels.findModeGroups?.[group.key] || group.key;
          section.append(heading);
        }
        for (const mode of modes) section.append(customizing ? findCustomizeOption(mode, hidden) : findOptionRow(mode));
        nodes.push(section);
      }
      el.findMenu.replaceChildren(...nodes);
      markFindSelection();
      if (findMenuOpen && !el.findMenu.hidden) positionFindMenu();
    }
    function applyFindCustomize() {
      findMenuPrefs.save(draftHiddenModes, draftHiddenHeadings);
      customizing = false;
      draftHiddenModes = null;
      draftHiddenHeadings = null;
      const visible = visibleModes(allFindModes, findMenuPrefs.hiddenModes());
      if (!visible.includes(selectedFindMode)) {
        selectedFindMode = visible[0];
        onFindMode?.(selectedFindMode);
      }
      renderFindMenu();
    }
    function cancelFindCustomize() {
      customizing = false;
      draftHiddenModes = null;
      draftHiddenHeadings = null;
      renderFindMenu();
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
      /* Closing the menu abandons an editing pass, the way the cross does. */
      customizing = false;
      draftHiddenModes = null;
      draftHiddenHeadings = null;
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
      /* Measured HERE, not where the rows are built: both this menu and the
         mode menu are filled while still hidden, and a hidden element measures
         nothing. By now the menu is on the page and laid out, and the width the
         column comes back with is what the height below is computed from. */
      measureIconColumn(el.findMenu);
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
      /* A mode hidden from the menu cannot stay the chosen one: the host is
         told to move to the first mode still on offer. The flag keeps a host
         that answers synchronously from re-entering this call. */
      const offered = visibleModes(allFindModes, findMenuPrefs.hiddenModes());
      if (!offered.includes(selectedFindMode)) {
        selectedFindMode = offered[0];
        if (!adjustingMode) {
          adjustingMode = true;
          try { onFindMode?.(selectedFindMode); } finally { adjustingMode = false; }
        }
      }
      el.mode.innerHTML = filtering ? icons.filter : icons.search;
      el.mode.classList.toggle("is-on", filtering);
      el.mode.setAttribute("aria-pressed", String(filtering));
      el.mode.setAttribute("aria-label", filtering ? labels.searchMode : labels.filterMode);
      el.mode.title = el.mode.getAttribute("aria-label");
      el.mask.textContent = findModeIcons[selectedFindMode];
      el.mask.setAttribute("aria-label", findModes[selectedFindMode]?.title || findModes[selectedFindMode]?.description || selectedFindMode);
      el.mask.title = el.mask.getAttribute("aria-label");
      markFindSelection();
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
      renderFindMenu();
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
    findMenuPrefs, findMenuGear:gearIcon, measureIconColumn,
    registerRibbon(api){if(api&&typeof api.measure==="function"&&typeof api.paint==="function")ribbon=api},
    get ribbon(){return ribbon}});
})();
