// Optional JS checks (Node's standard library only; no npm/install/build step).
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const web = path.join(__dirname, '../internal/server/web');
const script = fs.readFileSync(path.join(web, 'i18n.js'), 'utf8');
const fallback = JSON.parse(fs.readFileSync(path.join(web, 'i18n/en.json'), 'utf8'));
const main = fs.readFileSync(path.join(web, 'index.html'), 'utf8');
const looks = fs.readFileSync(path.join(web, 'looks.js'), 'utf8');
new vm.Script(looks, {filename: 'looks.js'});
const groups = fs.readFileSync(path.join(web, 'group-editor.js'), 'utf8');
new vm.Script(groups, {filename: 'group-editor.js'});
assert.doesNotMatch(main, /\s(?:title|aria-label)=tx\(/, 'HTML attributes must interpolate and escape translated text');
const manifest = JSON.parse(fs.readFileSync(path.join(web, 'presets/manifest.json'), 'utf8'));
for (const language of ['en', 'ru']) {
  const messages = JSON.parse(fs.readFileSync(path.join(web, `i18n/${language}.json`), 'utf8'));
  delete messages['language.cancel'];
  const context = {Intl, window: {}, document: {
    getElementById: () => ({textContent: JSON.stringify({language, messages, fallback})}),
    addEventListener: () => {}
  }};
  vm.runInNewContext(script, context);
  const {t} = context.window.wudictI18n;
  const lemmas = fs.readFileSync(path.join(web, 'lemmas.html'), 'utf8');
  const rowContext = {Intl, window:context.window, tx:t, mb:n=>String(n), document:{
    createElement:()=>({children:[], setAttribute(k,v){this[k]=v}, addEventListener(){}, append(...nodes){this.children.push(...nodes)}})
  }};
  vm.runInNewContext(lemmas.match(/const languageNames=[\s\S]*?\n}\r?\n/)[0] + '\n' +
    lemmas.match(/function rowFor\(l,j\)\{[\s\S]*?\n}/)[0], rowContext);
  const lemmaRow = rowContext.rowFor({code:'en', name:'English', state:'downloading', done:3, total:4}, {});
  assert.equal(lemmaRow.children[0].id, 'cb-en');
  assert.equal(lemmaRow.children[0].disabled, true);
  assert.equal(lemmaRow.children[1].textContent, language === 'ru' ? 'английский (en)' : 'English (en)');
  assert.equal(lemmaRow.children[3].textContent, t('pages.downloadPercent',{percent:75}));
  assert.equal(rowContext.languageName({code:'invalid_code',name:'Unlisted language'}), 'Unlisted language');
  // Built-in presentation may change; user-owned names and unknown upstream
  // additions must remain literal, even if they match a built-in's name.
  context.tx = t;
  const helpers = [looks.match(/function looksDisplayName\(l\) \{[\s\S]*?\n\}/)[0],
    main.match(/function layerText\(key, fallback\)\{[\s\S]*?\n\}/)[0],
    main.match(/function layerTitle\(p\)\{[^\n]+/)[0]];
  vm.runInNewContext(helpers.join('\n'), context);
  assert.equal(context.looksDisplayName({id:'clean', builtin:true, name:'Clean'}), language === 'ru' ? 'Чистое' : 'Clean');
  assert.equal(context.looksDisplayName({id:'mine', builtin:false, name:'Clean {name}'}), 'Clean {name}');
  assert.equal(context.looksDisplayName({id:'upstream_new', builtin:true, name:'New look'}), 'New look');
  assert.equal(context.layerTitle({id:'upstream_new', title:'New layer'}), 'New layer');
  vm.runInNewContext(groups.match(/function groupHint\(group,showAll,ordered\)\{[\s\S]*?\n\}/)[0], context);
  assert.equal(context.groupHint({members:[], readonly:false}, false, []), t('dictUI.emptyGroup'));
  assert.equal(context.groupHint({members:[], readonly:false}, true, []), t('dictUI.noMembers'));
  // Render a real dictionary card with hostile-looking user text. Translation
  // must preserve identifiers, escape the name and keep action attributes.
  const cards = [];
  const name = '<b>My "dictionary" {name}</b>';
  const panel = {innerHTML:'', closest:()=>null, appendChild:card=>cards.push(card)};
  const cardContext = {tx:t, window:context.window,
    $:()=>panel, document:{createElement:()=>({dataset:{}})},
    orderedDicts:()=>[{id:'unchanged-id',name,source:'sample.dsl',format:'dsl',entries:22,caps:{},dbSize:0}],
    dictLabel:d=>d.name, mb:n=>`${n} B`, baseIndexBytes:()=>100,
    esc:s=>String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;'),
    escAttr:s=>String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/"/g,'&quot;'),
    provenance:()=>'', traits:()=>'', renderReindex:()=>{}};
  vm.runInNewContext(main.match(/function renderPanel\(\)\{[\s\S]*?\n\}/)[0], cardContext);
  cardContext.renderPanel();
  assert.equal(cards[0].dataset.id, 'unchanged-id');
  assert.ok(cards[0].innerHTML.includes('&lt;b>My "dictionary" {name}&lt;/b>'));
  assert.ok(cards[0].innerHTML.includes(t('dictUI.about')));
  assert.ok(cards[0].innerHTML.includes('data-feat="contains"'));
  assert.ok(cards[0].innerHTML.includes('data-feat="fts"'));
  for (const g of manifest.groups) {
    assert.equal(fallback['layers.group.' + g.dir], g.title);
    assert.ok(messages['layers.group.' + g.dir]);
    for (const p of g.presets) for (const field of ['title','desc']) {
      const key = `layers.${p.id}.${field}`;
      assert.equal(fallback[key], p[field]);
      assert.ok(messages[key], key);
    }
  }
  for (const [count, folders, dictionaries] of [[1,'папка','словарь'],[2,'папки','словаря'],[5,'папок','словарей'],[21,'папка','словарь']]) {
    assert.equal(t('panel.folders', {count, number:count}), `${count} ${language === 'ru' ? folders : count === 1 ? 'folder' : 'folders'}`);
    assert.equal(t('panel.dictionaryCount', {count, number:count}), `${count} ${language === 'ru' ? dictionaries : count === 1 ? 'dictionary' : 'dictionaries'}`);
  }
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
for (const file of ['index', 'setup', 'browse', 'lemmas']) {
  const html = fs.readFileSync(path.join(web, `${file}.html`), 'utf8');
  for (const [,attrs,body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
    if (!attrs.includes('src=')) new vm.Script(body, {filename: file + '.html'});
  }
}
console.log('Interface JS syntax, plural rules, interpolation and fallback passed.');
