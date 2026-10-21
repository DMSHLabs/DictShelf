/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

(function () {
  "use strict";

  const thresholdKey = "wudict_browse_mask_threshold";
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

  function create({dock, ids, labels, maxRows = 2, countAction = false, onQuery, onMode, onMask, onThreshold, onApply, onStep, onCount}) {
    if (!dock || !ids) throw new TypeError("headword control needs a dock and element ids");
    const id = name => ids[name];
    dock.classList.add("headword-control");
    dock.innerHTML = `<button type="button" class="headword-control-mode" id="${id("mode")}"></button>` +
      `<button type="button" class="headword-control-mask" id="${id("mask")}" aria-pressed="false">*?</button>` +
      `<input class="headword-control-input" id="${id("input")}" type="search" placeholder="${labels.search}" ` +
      `spellcheck="false" autocomplete="off" autocapitalize="off" enterkeyhint="search" aria-label="${labels.search}">` +
      `<button type="button" class="headword-control-apply" id="${id("apply")}" aria-label="${labels.apply}" title="${labels.apply}" hidden>✓</button>` +
      (countAction ? `<button type="button" class="headword-control-count headword-control-count-button" id="${id("count")}" aria-live="polite">0/0</button>` :
        `<output class="headword-control-count" id="${id("count")}" aria-live="polite">0/0</output>`) +
      `<button type="button" class="headword-control-step headword-control-previous" id="${id("previous")}" aria-label="${labels.previous}" title="${labels.previous}">▲</button>` +
      `<button type="button" class="headword-control-step headword-control-next" id="${id("next")}" aria-label="${labels.next}" title="${labels.next}">▼</button>`;

    const el = Object.fromEntries(Object.entries(ids).map(([name, value]) => [name, document.getElementById(value)]));
    const canvas = document.createElement("canvas").getContext("2d");
    function fit() {
      if (dock.hidden || dock.parentElement?.hidden) return;
      const inputStyle = getComputedStyle(el.input), dockStyle = getComputedStyle(dock);
      canvas.font = inputStyle.font;
      const textWidth = Math.max(canvas.measureText("xxxxxxxxxxxxxxx").width,
        canvas.measureText("жжжжжжжжжжжжжжж").width);
      const inputPadding = parseFloat(inputStyle.paddingLeft || 0) + parseFloat(inputStyle.paddingRight || 0);
      const gap = parseFloat(dockStyle.columnGap) || 0;
      const controls = [el.mode, el.mask, el.apply, el.count, el.previous, el.next].filter(item => !item.hidden);
      const controlsWidth = controls.reduce((sum, item) => sum + item.getBoundingClientRect().width, 0) + gap * controls.length;
      const sidePadding = parseFloat(dockStyle.paddingLeft || 0) + parseFloat(dockStyle.paddingRight || 0);
      const available = dock.clientWidth - sidePadding;
      const rowControls = [el.mode, el.mask, el.count, el.previous, el.next].filter(item => !item.hidden);
      const rowWidth = rowControls.reduce((sum, item) => sum + item.getBoundingClientRect().width, 0) + gap * rowControls.length;
      const threeRows = maxRows >= 3 && available + .5 < rowWidth;
      const twoRows = threeRows || available + .5 < controlsWidth + textWidth + inputPadding;
      if (twoRows) dock.setAttribute("data-two-rows", "1"); else dock.removeAttribute("data-two-rows");
      if (threeRows) dock.setAttribute("data-three-rows", "1"); else dock.removeAttribute("data-three-rows");
      if (dock.id === "maskDock") document.body.style.setProperty("--mask-dock-height", dock.parentElement.getBoundingClientRect().height + "px");
    }
    function paint(state = {}) {
      const filtering = !!state.filtering;
      el.mode.innerHTML = filtering ? icons.filter : icons.search;
      el.mode.classList.toggle("is-on", filtering);
      el.mode.setAttribute("aria-pressed", String(filtering));
      el.mode.setAttribute("aria-label", filtering ? labels.searchMode : labels.filterMode);
      el.mode.title = el.mode.getAttribute("aria-label");
      el.mask.classList.toggle("is-on", !!state.mask);
      el.mask.setAttribute("aria-pressed", String(!!state.mask));
      el.mask.setAttribute("aria-label", labels.wildcard);
      el.mask.title = labels.wildcard;
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
      el.input.disabled = !!state.busy && !state.allowInputWhileBusy;
      fit();
    }
    function onInput() { onQuery?.(el.input.value); }
    function onModeClick() { onMode?.(); }
    function onMaskClick() { onMask?.(); }
    function onThresholdChange(event) { onThreshold?.(Number(event?.detail ?? threshold())); }
    function onStorageChange(event) { if (event.key === thresholdKey) onThreshold?.(threshold()); }
    function onApplyClick() { onApply?.(); }
    function onPreviousClick() { onStep?.(-1); }
    function onNextClick() { onStep?.(1); }
    function onCountClick() { onCount?.(); }
    el.input.addEventListener("input", onInput);
    el.mode.addEventListener("click", onModeClick);
    el.mask.addEventListener("click", onMaskClick);
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
        observer.disconnect();
        window.removeEventListener("wudict:headword-threshold-change", onThresholdChange);
        window.removeEventListener("storage", onStorageChange);
        removeEventListener("resize", fit);
        el.input.removeEventListener("input", onInput);
        el.mode.removeEventListener("click", onModeClick);
        el.mask.removeEventListener("click", onMaskClick);
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
