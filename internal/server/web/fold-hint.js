// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

(() => {
  const selector = "details.fold-hint";

  function refresh(details) {
    if (details.open) return;
    const summary = details.querySelector(":scope > summary");
    const text = summary?.querySelector(":scope > span");
    if (!text || text.clientWidth <= 0) return;
    details.classList.add("measuring");
    const overflows = text.scrollWidth > text.clientWidth + 1;
    details.classList.remove("measuring");
    details.classList.toggle("fits", !overflows);
  }

  function attach(details) {
    if (details.dataset.foldHintReady) return;
    const summary = details.querySelector(":scope > summary");
    if (!summary) return;
    details.dataset.foldHintReady = "1";
    const observer = new ResizeObserver(() => refresh(details));
    observer.observe(summary);
    summary.addEventListener("click", event => {
      if (details.classList.contains("fits")) event.preventDefault();
    });
    details.addEventListener("toggle", () => refresh(details));
    refresh(details);
  }

  function scan(root) {
    if (root.matches?.(selector)) attach(root);
    root.querySelectorAll?.(selector).forEach(attach);
  }

  scan(document);
  new MutationObserver(records => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === Node.ELEMENT_NODE) scan(node);
      }
      const target = record.target.nodeType === Node.ELEMENT_NODE ? record.target : record.target.parentElement;
      const details = target?.closest?.(selector);
      if (details) refresh(details);
    }
  }).observe(document.documentElement, {childList: true, characterData: true, subtree: true});

  document.fonts?.ready.then(() => document.querySelectorAll(selector).forEach(refresh));
})();
