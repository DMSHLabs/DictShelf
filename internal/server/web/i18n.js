/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

/* Interface text only. Never infer the language from the OS, browser or a
   dictionary. Reload after saving so old and new labels cannot mix in a view. */
(function () {
  "use strict";
  const boot = JSON.parse(document.getElementById("wudict-i18n").textContent);
  const language = boot.language;
  const plurals = new Intl.PluralRules(language);
  const numbers = new Intl.NumberFormat(language);
  function t(key, params = {}) {
    let text = boot.messages[key] ?? boot.fallback[key] ?? key;
    if (typeof text === "object") text = text[plurals.select(params.count)] ?? text.other;
    return String(text).replace(/\{([a-zA-Z0-9_]+)\}/g, (match, name) =>
      Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : match);
  }
  let languageNames;
  try { languageNames = new Intl.DisplayNames([language], {type:"language", fallback:"none"}); } catch (_) {}
  // Display only: never rewrite memberships or server labels. Unknown values
  // (including future upstream facets) retain the server's complete label.
  function facetLabels(g) {
    if (language === "en") return {fl:g.fl, vl:g.vl};
    const headings = {lang:"facets.language", pair:"facets.pair", kind:"facets.content", pub:"facets.publisher"};
    const kinds = {encyclopedia:"facets.encyclopedia", thesaurus:"facets.thesaurus",
      idioms:"facets.idioms", slang:"facets.slang", etymology:"facets.etymology",
      abbrev:"facets.abbrev", grammar:"facets.grammar", medical:"facets.medical", legal:"facets.legal"};
    let vl = g.vl;
    if (g.f === "kind" && Object.prototype.hasOwnProperty.call(kinds, g.v)) vl = t(kinds[g.v]);
    if ((g.f === "lang" || g.f === "pair") && languageNames) {
      const codes = g.f === "pair" ? g.v.split("-") : [g.v];
      if (codes.length <= 2 && codes.every(c => /^[a-z]{2}$/.test(c))) {
        try {
          const names = codes.map(c => languageNames.of(c));
          if (names.every((n,i) => n && n !== codes[i])) vl = g.f === "pair" && names.length === 1
            ? t("facets.monolingual", {language:names[0]}) : names.join(" ↔ ");
        } catch (_) {}
      }
    }
    return {fl:Object.prototype.hasOwnProperty.call(headings, g.f) ? t(headings[g.f]) : g.fl, vl};
  }
  // Legacy API errors are plain English text. Match only these complete
  // messages or explicit prefixes; never replace words inside paths/details.
  // Unknown errors and English responses remain literal. New API families
  // should use stable error codes instead of expanding text matching.
  const errorKeys = [
    "errors.workBusy",
    "errors.sharePage",
    "errors.noLinks",
    "errors.siteSilent",
    "errors.driveRefused",
    "errors.orphanEmpty",
    "errors.orphanLimit",
    "errors.notDSL",
    "errors.dslUnavailable",
    "errors.folderMissing",
    "errors.notFolder",
    "errors.fileMissing",
    "errors.notFile",
    "errors.importBusy",
    "errors.noImport",
    "errors.noDictionaries",
    "errors.archiveFormat",
    "errors.noDestination",
    "errors.incomplete",
    "errors.https",
    "errors.notArchive",
    "errors.notLink",
    "errors.downloadLarge",
    "errors.importLarge",
    "errors.localAddress",
    "errors.redirects",
    "errors.noArchiveDictionary",
    "errors.nothingSelected",
    "errors.noCompanion",
    "errors.encrypted",
    "errors.archiveFiles",
    "errors.lemmaFolder",
    "errors.lemmaCatalogue",
    "errors.reindexing",
    "errors.noIndex",
    "errors.nothingRemove",
    "errors.enablePrepared",
    "errors.remoteDelete",
    "errors.localOnly",
    "errors.notAppFolder",
    "errors.lookFolder",
    "errors.rememberFolder",
    "errors.lookName",
    "errors.lookExists",
    "errors.lookBuiltin",
    "errors.lookDelete",
    "errors.lookMissing",
    "errors.presetsFolder",
    "errors.stylesFolder",
    "errors.filesFolder",
    "errors.tooManyFiles",
    "errors.fileLarge",
    "errors.emptyFile",
    "errors.unknownFile",
    "errors.noRoom",
    "errors.host",
    "errors.missingParts",
    "errors.unsafeArchive",
    "errors.archiveSize",
    "errors.archiveRatio",
    "errors.collectionMissing",
    "errors.indexRemoved",
    "errors.collectionFileMissing",
    "errors.notDownload"
  ];
  const errorPrefixes = [
    "errors.receive",
    "errors.hostDetail",
    "errors.missingPartsDetail",
    "errors.unsafeArchiveDetail",
    "errors.archiveSizeDetail",
    "errors.archiveRatioDetail",
    "errors.catalogueRead",
    "errors.noLemmaData",
    "errors.unknownDict",
    "errors.saveLook",
    "errors.deleteLook",
    "errors.applyLook",
    "errors.rememberLook",
    "errors.savePresets",
    "errors.unknownPreset",
    "errors.save",
    "errors.rescan",
    "errors.libraryRead"
  ];
  function errorText(value) {
    const original = String(value ?? "");
    if (language === "en") return original;
    let text = original.trim();
    // Some callers read JSON API failures as text. Unwrap only for matching;
    // unknown responses and English mode still retain the original payload.
    try { const body = JSON.parse(text); if (typeof body?.error === "string") text = body.error.trim(); } catch (_) {}
    for (const key of errorKeys) if (text === boot.fallback[key]) return t(key);
    for (const key of errorPrefixes) {
      const source = boot.fallback[key];
      if (!source || !source.endsWith("{detail}")) continue;
      const prefix = source.slice(0, -8);
      if (text.startsWith(prefix)) return t(key, {detail:text.slice(prefix.length)});
    }
    // Whole-message templates only. Captured names and diagnostics are literal
    // parameters, never translated recursively or interpreted as markup.
    const templates = [
      [/^preparing (".*"): ([\s\S]+)$/, "errors.preparing"],
      [/^removing packed media for (".*"): ([\s\S]+)$/, "errors.removingMedia"],
      [/^(".*") is a prepared dictionary - its original files are gone, so its data cannot be rebuilt$/, "errors.noSource"],
      [/^(".*") has no original files - it is the prepared dictionary itself$/, "errors.noOriginal"],
      [/^(".*") has nothing prepared to remove$/, "errors.noPrepared"],
      [/^(".*") is not prepared, so deleting its files would delete the dictionary$/, "errors.deleteUnprepared"],
      [/^could not save (.+): ([\s\S]+)$/, "errors.saveFile"],
      [/^could not remove (.+): ([\s\S]+)$/, "errors.removeFile"],
      [/^a file named (.+) is already there$/, "errors.fileExists"],
      [/^(app\.css|article\.css|app_night\.css|article_night\.css) is too large$/, "errors.styleLarge"],
      [/^([a-z]{2}) is being downloaded$/, "errors.downloading"],
      [/^([a-z]{2}): expected (\d+) bytes, got (\d+)$/, "errors.downloadSize"],
      [/^([a-z]{2}): checksum mismatch \(expected ([a-f0-9]+), got ([a-f0-9]+)\)$/, "errors.checksum"]
    ];
    for (const [pattern, key] of templates) {
      const m = text.match(pattern);
      if (m) return t(key, {name:m[1], detail:m[2], actual:m[3]});
    }
    return original;
  }
  async function saveLanguage(chosen) {
    const response = await fetch("/api/language", {method: "PUT",
      headers: {"Content-Type": "application/json"}, body: JSON.stringify({language: chosen})});
    if (!response.ok) throw new Error("save");
  }
  window.wudictI18n = Object.freeze({language, t, facetLabels, errorText,
    saveLanguage, number: n => numbers.format(n)});
})();
