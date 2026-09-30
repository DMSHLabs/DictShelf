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
const speech = fs.readFileSync(path.join(web, 'speak.js'), 'utf8');
new vm.Script(speech, {filename:'speak.js'});
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
  const speechContext = {Intl, window:{wudictI18n:context.window.wudictI18n},
    navigator:{language:'de-DE',languages:['de-DE']}, localStorage:{getItem:()=>null}};
  const exposed = speech.replace('window.wuSpeak = {',
    'window.testSpeech = {decide, langName, voiceLabel, pickVoice, noVoice, setVoices: v => voices = v};\nwindow.wuSpeak = {');
  vm.runInNewContext(exposed,speechContext);
  const sp = speechContext.window.testSpeech;
  assert.equal(sp.langName('en'),language==='ru'?'английский':'English');
  assert.equal(sp.decide('Latn','en','ru'),'en');
  assert.equal(sp.decide('Cyrl','en','ru'),'ru');
  assert.equal(sp.decide('Latn','',''),'de'); // speech fallback retains its own rules
  const voice={name:'Engine brand {language}',voiceURI:'stable-id',lang:'en-US',localService:true};
  sp.setVoices([voice]);
  assert.equal(sp.voiceLabel(voice),voice.name); // browser-provided name is literal
  speechContext.window.__wdSpeech={};
  assert.equal(sp.voiceLabel(voice),sp.langName('en-US'));
  assert.equal(sp.pickVoice('en'),voice);
  assert.equal(sp.noVoice('en'),t('speech.noVoice',{language:sp.langName('en')}));
  const writes=[],notes=[];
  const systemContext={tx:t,sysNote:s=>notes.push(s),sysSet:r=>writes.push(r)};
  vm.runInNewContext(main.match(/function sysCommitNumber\(row,field\)\{[\s\S]*?\n}/)[0],systemContext);
  const row={key:'SEARCH_MEMORY',stored:'64',min:0,max:512};
  const field={value:'900'};
  systemContext.sysCommitNumber(row,field);
  assert.equal(field.value,'64');assert.equal(writes.length,0);
  assert.equal(notes[0],t('system.invalid',{value:'900',min:0,max:512}));
  field.value='128';systemContext.sysCommitNumber(row,field);
  assert.equal(JSON.stringify(writes[0]),JSON.stringify({action:'set',field:'override',key:'SEARCH_MEMORY',value:'128'}));
  // Exercise actual result rendering: translating the surrounding UI must not
  // alter headwords, article HTML, dictionary IDs or the index action values.
  const articles = [];
  const node = () => ({dataset:{}, children:[], appendChild(n){this.children.push(n)}, addEventListener(){}});
  const resultContext = {tx:t, window:context.window, document:{createElement:node},
    esc:s=>String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'),
    expandAll:false, anchorOnce:null,
    renderArticle:(dd,body,id)=>{articles.push({body,id});return null}};
  vm.runInNewContext(main.match(/function renderSlot\(box,h,mode,q,perDict,group\)\{[\s\S]*?\n}/)[0], resultContext);
  const box=node(), body='<p lang="en">word {query}</p>', headword='<word> {number}';
  resultContext.renderSlot(box,{dict:'original-id',name:'<Dictionary>',results:[{Headword:headword,Body:body}]},'prefix','word',1,false);
  assert.match(box.children[0].innerHTML, /&lt;Dictionary&gt;/);
  assert.ok(box.children[0].innerHTML.includes(t('search.results',{count:1,number:'1+'})));
  assert.ok(box.children[0].innerHTML.includes(t('search.more')));
  assert.equal(box.children[0].children[0].children[0].textContent,headword);
  assert.deepEqual(articles,[{body,id:'original-id'}]);
  const skipped=node();
  resultContext.renderSlot(skipped,{dict:'original-id',name:'<Dictionary>',skipped:true},'fts','word',5,false);
  assert.match(skipped.innerHTML,/data-id="original-id" data-feat="fts"/);
  assert.ok(skipped.innerHTML.includes(t('search.enableMode',{mode:t('search.fullText')})));
  for(const [count,word] of [[1,'результат'],[2,'результата'],[5,'результатов'],[11,'результатов'],[21,'результат'],[22,'результата']]){
    assert.equal(t('search.results',{count,number:count}),`${count} ${language==='ru'?word:count===1?'result':'results'}`);
  }
  assert.equal(t('search.noQuery',{query:'<b>{query}</b>'}),language==='ru'?'По запросу «<b>{query}</b>» ничего не найдено':'No results for “<b>{query}</b>”');
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
