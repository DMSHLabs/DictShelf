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
  window.wudictI18n = Object.freeze({language, t, number: n => numbers.format(n)});

  function openLanguage(anchor) {
    if (document.getElementById("uiLanguageDialog")) return;
    const dialog = document.createElement("dialog");
    dialog.id = "uiLanguageDialog";
    dialog.className = "group-dialog panel-card ui-language-dialog";
    dialog.lang = language;
    dialog.setAttribute("aria-labelledby", "uiLanguageTitle");
    function element(tag, text, parent = dialog) {
      const el = document.createElement(tag);
      if (text) el.textContent = text;
      parent.appendChild(el);
      return el;
    }
    const title = element("h2", "Language");
    title.id = "uiLanguageTitle";
    element("p", t("language.hint"));
    const fields = element("fieldset");
    element("legend", t("language.choose"), fields);
    for (const [code, name] of [["en", "English"], ["ru", "Русский"]]) {
      const label = element("label", "", fields);
      const input = element("input", "", label);
      input.type = "radio"; input.name = "uiLanguage"; input.value = code;
      input.checked = code === language;
      const span = element("span", name, label); span.lang = code;
    }
    const error = element("p"); error.setAttribute("role", "alert");
    const actions = element("div"); actions.className = "ui-language-actions";
    const cancel = element("button", t("language.cancel"), actions); cancel.type = "button";
    const apply = element("button", t("language.apply"), actions); apply.type = "button";
    cancel.onclick = () => dialog.close();
    let saving = false;
    dialog.addEventListener("cancel", e => { if (saving) e.preventDefault(); });
    dialog.addEventListener("close", () => { dialog.remove(); anchor.focus(); });
    apply.onclick = async () => {
      const chosen = dialog.querySelector("input:checked").value;
      if (chosen === language) { dialog.close(); return; }
      saving = true; fields.disabled = true; apply.disabled = true; cancel.disabled = true;
      error.textContent = "";
      try {
        const response = await fetch("/api/language", {method: "PUT",
          headers: {"Content-Type": "application/json"}, body: JSON.stringify({language: chosen})});
        if (!response.ok) throw new Error("save");
        location.reload();
      } catch (_) {
        saving = false; fields.disabled = false; apply.disabled = false; cancel.disabled = false;
        error.textContent = t("language.saveFailed");
      }
    };
    document.body.appendChild(dialog);
    dialog.showModal();
    dialog.querySelector("input:checked").focus();
  }
  document.addEventListener("click", event => {
    const anchor = event.target.closest("[data-ui-language]");
    if (anchor) openLanguage(anchor);
  });
})();
