/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

/* A small list search that can be attached to any item container and dock.
   Other lists can use window.wudictListSearch.create({root, itemSelector,
   dock, scrollContainer, labels}); call refresh() after replacing their items. */
(function () {
  "use strict";

  const icons = {
    search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 5 5"/></svg>',
    filter: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M3 5h18l-7 8v5l-4 2v-7z"/></svg>'
  };

  function create(options) {
    const {
      root, itemSelector, dock, scrollContainer = root,
      getItems = () => [...root.querySelectorAll(itemSelector)],
      getText = item => item.innerText || item.textContent || "",
      labels = {}, initiallyFiltering = false
    } = options;
    if (!root || !dock || typeof getItems !== "function")
      throw new TypeError("list search needs a root, a dock and an item getter");

    dock.classList.add("list-search-bar");
    dock.innerHTML = `<span class="list-search-icon">${icons.search}</span>` +
      `<input class="list-search-input" type="search" autocomplete="off" enterkeyhint="search">` +
      `<output class="list-search-count" aria-live="polite">0/0</output>` +
      `<button class="list-search-step" type="button" data-step="-1"></button>` +
      `<button class="list-search-step" type="button" data-step="1"></button>` +
      `<button class="list-search-filter" type="button">${icons.filter}</button>`;
    const input = dock.querySelector(".list-search-input");
    const count = dock.querySelector(".list-search-count");
    const [previous, next] = dock.querySelectorAll(".list-search-step");
    const filter = dock.querySelector(".list-search-filter");
    input.placeholder = labels.placeholder || "Search…";
    input.setAttribute("aria-label", labels.search || input.placeholder);
    previous.textContent = "▲";
    next.textContent = "▼";
    previous.setAttribute("aria-label", labels.previous || "Previous match");
    next.setAttribute("aria-label", labels.next || "Next match");
    previous.title = previous.getAttribute("aria-label");
    next.title = next.getAttribute("aria-label");
    filter.setAttribute("aria-label", labels.filter || "Filter list");
    filter.title = labels.filter || "Filter list";
    let filtering = !!initiallyFiltering;
    let activeIndex = -1;
    let items = [];
    let matches = [];

    const normalize = value => String(value).normalize("NFKC").trim().toLowerCase();
    function paint() {
      const query = normalize(input.value);
      items = getItems();
      matches = [];
      for (const item of items) {
        const matched = !query || normalize(getText(item)).includes(query);
        item.classList.toggle("list-search-match", !!query && matched && !filtering);
        item.classList.toggle("list-search-current", !!query && matched && !filtering && matches.length === activeIndex);
        item.classList.toggle("list-search-filtered-out", filtering && !matched);
        if (query && matched) matches.push(item);
      }
      filter.setAttribute("aria-pressed", String(filtering));
      filter.classList.toggle("is-on", filtering);
      if (filtering) {
        const visible = items.filter(item => !item.classList.contains("list-search-filtered-out")).length;
        count.value = `${visible}/${visible}`;
        count.textContent = count.value;
        count.setAttribute("aria-label", labels.count ? labels.count.replace("{current}", visible).replace("{total}", visible) : count.value);
        previous.disabled = next.disabled = true;
      } else {
        if (activeIndex >= matches.length) activeIndex = matches.length - 1;
        const current = matches.length ? activeIndex + 1 : 0;
        count.value = `${current}/${matches.length}`;
        count.textContent = count.value;
        count.setAttribute("aria-label", labels.count ? labels.count.replace("{current}", current).replace("{total}", matches.length) : count.value);
        previous.disabled = next.disabled = matches.length < 2;
        for (const item of items) item.classList.remove("list-search-current");
        if (activeIndex >= 0 && matches[activeIndex]) matches[activeIndex].classList.add("list-search-current");
      }
    }
    function scrollToCurrent() {
      const item = matches[activeIndex];
      if (!item || !scrollContainer) return;
      const viewport = scrollContainer.getBoundingClientRect();
      const rect = item.getBoundingClientRect();
      if (rect.top < viewport.top) scrollContainer.scrollTop -= viewport.top - rect.top;
      else if (rect.bottom > viewport.bottom) scrollContainer.scrollTop += rect.bottom - viewport.bottom;
    }
    function navigate(delta) {
      if (filtering || !matches.length) return;
      activeIndex = (activeIndex + delta + matches.length) % matches.length;
      paint();
      scrollToCurrent();
    }
    function toggleFilter() {
      filtering = !filtering;
      activeIndex = filtering ? -1 : (matches.length ? 0 : -1);
      paint();
      if (!filtering) scrollToCurrent();
    }
    function onInput() {
      activeIndex = normalize(input.value) ? 0 : -1;
      paint();
      if (activeIndex >= 0) scrollToCurrent();
    }
    function onKeyDown(event) {
      if (event.key === "Tab" && !event.shiftKey) {
        event.preventDefault();
        toggleFilter();
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        navigate(-1);
      } else if (event.key === "ArrowDown" || event.key === "Enter") {
        event.preventDefault();
        navigate(event.key === "Enter" && event.shiftKey ? -1 : 1);
      } else if (event.key === "Escape" && input.value) {
        event.preventDefault();
        input.value = "";
        onInput();
      }
    }
    function onStep(event) { navigate(Number(event.currentTarget.dataset.step)); }
    input.addEventListener("input", onInput);
    input.addEventListener("keydown", onKeyDown);
    previous.addEventListener("click", onStep);
    next.addEventListener("click", onStep);
    filter.addEventListener("click", toggleFilter);
    paint();

    return {
      refresh: paint,
      focus: () => input.focus(),
      destroy() {
        input.removeEventListener("input", onInput);
        input.removeEventListener("keydown", onKeyDown);
        previous.removeEventListener("click", onStep);
        next.removeEventListener("click", onStep);
        filter.removeEventListener("click", toggleFilter);
        for (const item of getItems()) item.classList.remove("list-search-match", "list-search-current", "list-search-filtered-out");
        dock.replaceChildren();
        dock.classList.remove("list-search-bar");
      }
    };
  }

  window.wudictListSearch = Object.freeze({ create });
  const t = window.wudictI18n?.t;
  const dock = document.getElementById("dictSettingsSearch");
  const root = document.getElementById("panelList");
  if (dock && root && t) {
    window.dictSettingsListSearch = create({
      root, itemSelector: ".pd", dock,
      scrollContainer: document.getElementById("dictSettingsScroll"),
      labels: {
        placeholder: t("dictUI.listSearchPlaceholder"),
        search: t("dictUI.listSearchLabel"),
        previous: t("dictUI.listSearchPrevious"),
        next: t("dictUI.listSearchNext"),
        filter: t("dictUI.listSearchFilter"),
        count: t("dictUI.listSearchCount", {current:"{current}", total:"{total}"})
      }
    });
  }
})();
