// Optional JS checks (Node's standard library only; no npm/install/build step).
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const web = path.join(__dirname, '../internal/server/web');
const script = fs.readFileSync(path.join(web, 'i18n.js'), 'utf8');
const fallback = JSON.parse(fs.readFileSync(path.join(web, 'i18n/en.json'), 'utf8'));
for (const language of ['en', 'ru']) {
  const messages = JSON.parse(fs.readFileSync(path.join(web, `i18n/${language}.json`), 'utf8'));
  delete messages['language.cancel'];
  const context = {Intl, window: {}, document: {
    getElementById: () => ({textContent: JSON.stringify({language, messages, fallback})}),
    addEventListener: () => {}
  }};
  vm.runInNewContext(script, context);
  const {t} = context.window.wudictI18n;
  assert.equal(t('language.cancel'), 'Cancel');
  assert.equal(t('missing.key'), 'missing.key');
  assert.equal(t('browse.pageTitle', {name: '<b>{name}</b>'}),
    language === 'ru' ? '<b>{name}</b> · Просмотр словаря' : '<b>{name}</b> · Browse');
  for (const [count, word] of [[0,'слов'], [1,'слово'], [2,'слова'], [5,'слов'],
    [11,'слов'], [21,'слово'], [22,'слова'], [25,'слов'], [101,'слово'], [1.5,'слова']]) {
    assert.equal(t('browse.words', {count, number: String(count)}),
      `${count} ${language === 'ru' ? word : count === 1 ? 'word' : 'words'}`);
  }
}
for (const file of ['index', 'setup', 'browse']) {
  const html = fs.readFileSync(path.join(web, `${file}.html`), 'utf8');
  for (const [,attrs,body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
    if (!attrs.includes('src=')) new vm.Script(body, {filename: file + '.html'});
  }
}
console.log('Interface JS syntax, plural rules, interpolation and fallback passed.');
