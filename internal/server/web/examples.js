/**
 * Copyright (C) 2026 glowinthedark
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

// Example folding: MARK the lines that hold nothing but examples, so the
// "Examples: Show | Hide" layer can fold them away with one CSS rule.
//
// It marks and never hides. The class says a FACT about the line - there is
// nothing here but examples - and the hiding is that layer's article half
// (presets/examples/hide_examples_article.css), which exists in the article
// sheet only while the layer is on. That split is what keeps this file
// stateless: it knows nothing about the toggle, is never told about it, needs
// no on/off call, and a line marked while the layer is off is simply visible.
// It also means the switch needs no second pass: taking the rule away brings
// every example back, in the articles already on screen included.
//
// Shadow articles only. A script-bearing dictionary is rendered in a sandboxed
// iframe, which this page cannot reach into - and which does not need it: the
// roles this test reads (wu-ex, wu-audio) are emitted by wudict's OWN DSL and
// XDXF rendering (internal/artmark), and those articles never take the iframe
// route.
(function () {
"use strict";
const CLASS = "wu-xonly";
// Whitespace, the bullets a DSL line hangs on, and dashes: text that says
// nothing about whether the line has content of its own. The no-break space is
// in the class because a dictionary's own file may carry one where a plain
// space was meant.
const JUNK = /^[\s\u00a0▪•·\-–—]*$/;

/* True when the paragraph holds at least one example and no text of its own.
   Text inside a link is skipped because the audio button lives there, and the
   examples themselves are skipped because they are what the layer folds: a
   line reading "▪ 🔊 They have a beautiful home." is an example line and
   nothing else. Everything else counts, whether or not it wears a role class -
   a translation in plain text keeps its line, which is the difference between
   folding the examples and folding whatever looked empty. */
function exampleOnly(p) {
  if (!p.querySelector(".wu-ex")) return false;
  const walker = document.createTreeWalker(p, NodeFilter.SHOW_TEXT);
  for (let n; (n = walker.nextNode()); ) {
    if (n.parentElement.closest("a,.wu-ex")) continue;
    if (JUNK.test(n.textContent)) continue;
    return false;
  }
  return true;
}

function mark(root) {
  if (!root || !root.querySelectorAll) return;
  for (const p of root.querySelectorAll("p")) if (exampleOnly(p)) p.classList.add(CLASS);
}
function markHost(el) {
  if (el.classList && el.classList.contains("article") && el.shadowRoot) mark(el.shadowRoot);
}
function markAll() {
  for (const host of document.querySelectorAll(".article")) markHost(host);
}

/* Results stream in, every new search builds fresh articles, and highlighting
   re-renders the articles it marks - so the marking follows the DOM rather
   than the search: whatever is added is marked once, where it lands. The root
   watched is #out, the results' own container, not the body: the panel, the
   sheet and the status line never wake it. */
const results = document.getElementById("out") || document.body;
if (results) {
  new MutationObserver(muts => {
    for (const m of muts) for (const n of m.addedNodes) {
      if (n.nodeType !== 1) continue;
      markHost(n);
      if (n.querySelectorAll) for (const host of n.querySelectorAll(".article")) markHost(host);
    }
  }).observe(results, { childList: true, subtree: true });
  markAll();
}
})();
