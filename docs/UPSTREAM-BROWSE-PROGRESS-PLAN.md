# Задание: честный прогресс индексации в Browse — **наблюдение, без запуска**

Это **задание, а не знание**. Когда оно выполнено — удалите этот файл целиком и снимите ссылку
на него из `HANDOFF-NEXT.md`; результат и цифры — в телах коммитов и в `HANDOFF.md`.

Автор: сессия 2026-11-04 (раунд апстрима, MDict, четыре ZIM-правки).

---

## 1. Прочитать сначала (обязательно)

- `AGENTS.md` — правила веток, лицензии/шапки, требования к сборке (cgo + теги), запрет пуша.
- `docs/INDEX.md` — карта «вопрос → файл».
- `HANDOFF.md`, раздел **«Upstream reviewed on 2026-11-04, and what was left»** — там этот
  коммит (`8a48536d`) назван продуктовым решением и лежит его история.
- `HANDOFF-NEXT.md` — решения, которые легко отменить по незнанию (в том числе про этот файл).
- `docs/WINDOWS-VERIFY.md` — как поднять сервер на своей библиотеке **для ручной проверки**
  (она здесь обязательна, это UI).
- `thirdpartytools.md` — чем собирать на этой машине.
- Этот файл целиком.

## 2. Решение, которое определяет всё (НЕ отменяйте его по незнанию)

Это не новая фича: форк её **уже имел и убрал осознанно**.

- `95f02382` «chore(browse): clicking browse triggers indexing» — была и индексация из Browse, и
  прогресс (опрос `/api/demand?explicit=1` каждые 2.5 с, ключи `browse.building` /
  `browse.buildFailed`). Посмотреть, как это выглядело: `git show 95f02382 -- internal/server/web/browse.html`
- `c2196377` (2026-10-12) «Redesigning the algorithm for building dictionary indexes» — убрал
  **и запуск, и прогресс**, оставив статичную карточку `browse.indexRequired` («создайте индекс в
  настройках»). Причины в теле коммита нет, только заголовок — их знает пользователь.
  Дифф: `git show c2196377 -- internal/server/web/browse.html`
- **Решение пользователя 2026-11-04: вернуть только ПРОГРЕСС, без запуска.** Индекс не строится
  из навигации — это та же философия, что записана в `internal/format/wmd/wmd.go`: «Opening it
  does not create a prepared index; library indexes are built by the add queue or explicit
  indexing actions».

**Следствие, которое нельзя нарушать:** `unprepared()` **не** вызывает `/api/ingest` и **не**
вызывает `/api/demand`. Ни при каких условиях. `/api/ingest` «доводит словарь до нужного
состояния» (см. комментарий `handleIngest`, `internal/server/server.go:2214`) — то есть это и есть
запуск.

## 3. Что видит читатель

- **до:** карточка «нужен индекс» — независимо от того, идёт ли индексация прямо сейчас.
- **после:** если для этого словаря задача идёт — живой бар с числами и честным «Finishing…»;
  когда задача завершилась — страница слов; если задача не идёт или упала — карточка как сегодня.

## 4. Откуда брать прогресс (наш сервер; ничего не запускает)

`GET /api/index-work` — маршрут `internal/server/routes.go:113`, обработчик
`handleIndexWork` (`internal/server/index_work.go:269`), ответ — `indexWorkStatus`
(`index_work.go:137`) со полями:

    running, dict, done, total, dictionaryDone, dictionaryTotal,
    current, currentDone, currentTotal, stage, action, elapsedMs, id

- Форму запроса (нужен ли `?dict=`) уточните по `handleIndexWork` / `indexWorkStatus(requested …)`
  (`index_work.go:164`) и **напишите в теле коммита**, что именно используете. `GET` обязан
  отвечать и когда ничего не идёт — это и есть ветка «карточка».
- Для in-process окна Windows у того же статуса есть мост (`internal/server/desktop_search.go:663`,
  случай `"indexWork"`) — проверьте, нужен ли отдельный путь; в браузере это обычный `fetch`.
- `POST`/`DELETE /api/index-work` — это старт/остановка. **Не использовать.**

## 5. Что взять у апстрима (`8a48536d`, `internal/server/web/browse.html`) — только честность показа

`git show 8a48536d -- internal/server/web/browse.html` — брать приёмы, не код:

- **ничего первые 400 мс**: ожидание ниже этого читается как мгновенное, а вспыхнувшая на
  маленьком словаре карточка — шум;
- настоящий `<progress>` с «X of Y words» и процентом **в `document.title`**;
- когда счётная часть кончилась, а несчётный хвост (индексы, commit, ANALYZE) ещё идёт —
  заголовок «Finishing…» и **снятие `value` у бара**: «бар, замерший на 100, врёт»;
- флаг «уже проиндексирован» (`indexed` у апстрима): словарь, который проиндексировался и всё
  равно не открывается, — это отказ, а не повод индексировать снова (у нас роль «не зацикливаться»
  играет то, что мы ничего не запускаем);
- `failed(name, why)` — карточка отказа с причиной и кнопкой «Try again», которая просто
  **перезапрашивает статус** (не запускает задачу);
- хуки жизненного цикла: `unwatch()` в `pagehide` и `popstate`, перезапрос на `pageshow` для
  страницы из back-forward cache. У нас эти обработчики есть: `browse.html:900` (pagehide),
  `2269` (popstate), `2294` (pageshow).

**Не брать:** `EventSource("/api/ingest?dict=…")`, `/api/demand`, и их CSS дословно — у нас свои
переменные (`--accent`, `--soft`, `--faint`, `--line`); сверьтесь с нашим `.msg` в `browse.html`.

## 6. Наш код: имена и правила

- Имена уже совпадают с апстримными: `dictId`, `state`, `last`, `poll` (`browse.html:513`),
  `card(...)`, `esc()` (481), `tx()` (483 — он **уже экранирует**).
- Числа форматировать через `window.wudictI18n.number(...)` — правило форка (никакого
  `toLocaleString`/`en-US`): «every count is formatted with the reader's own separators».
- **Интервал опроса:** `poll` в состоянии уже есть (наследие `95f02382`); берите 1000–2000 мс и
  обязательно останавливайте опрос при уходе со страницы (`pagehide`).
- Новые строки — только через `tx()` и **ключи в ОБОИХ каталогах**
  (`internal/server/web/i18n/en.json` и `ru.json`), иначе падает `TestI18nCatalogs` (он сверяет
  набор ключей и набор подстановок `{…}`). Предлагаемые ключи — сверьте, нет ли уже таких:

      browse.indexingTitle         "Indexing {name}"
      browse.indexingWords         "{done} of {total} words"
      browse.indexingFinishing     "Finishing…"
      browse.indexingCounted       "{total} words"
      browse.indexingCanLeave      "You can leave this page: indexing goes on."
      browse.indexingSearchMeanwhile "Search meanwhile"
      browse.indexFailed           "Could not index {name}"
      browse.tryAgain              "Try again"

  Русские значения — в стиле каталога (императив, «Создайте…», как в `browse.indexRequired`).

## 7. Гейты

    export PATH="/c/Qt/Tools/mingw1310_64/bin:$PATH"; export CGO_ENABLED=1
    node tools/i18n-test.cjs
    go test -tags sqlite_fts5,sqlite_dbstat ./internal/server/ -run TestI18n -count=1
    go test -tags sqlite_fts5,sqlite_dbstat ./internal/server/ -count=1
    go test -tags sqlite_fts5,sqlite_dbstat ./...            # полный набор

Синтаксис JS страницы (у `index.html`/`browse.html` есть серверные `{{…}}` — их надо подставить,
иначе `node --check` споткнётся; приём уже применялся в коммите `b3468c26`):

    node -e 'const fs=require("fs");const s=fs.readFileSync("internal/server/web/browse.html","utf8");
    const re=/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g;let m,out="",n=0;
    while((m=re.exec(s))){out+="\n;{\n"+m[1].replace(/\{\{[^}]*\}\}/g,"0")+"\n}\n";n++}
    fs.writeFileSync("D:/Temp/browse-check.js",out);console.log("blocks:",n)' && node --check "D:/Temp/browse-check.js"

**Ручная проверка в браузере — обязательна** (это UI, автотестами не закрывается). Поднять сервер
на своей библиотеке (`docs/WINDOWS-VERIFY.md`) и пройти:

1. запустить индексацию из панели приложения и открыть Browse для этого словаря → появляется бар
   (после ~400 мс), растут числа, в заголовке вкладки процент;
2. дойти до конца счётной части → «Finishing…» с неопределённым баром, затем страница слов;
3. ничего не индексируется → карточка как сегодня (регрессии нет);
4. оборвать сервер в середине (Ctrl-C) → карточка отказа с причиной, кнопка «Try again» просто
   перезапрашивает статус, задача не запускается;
5. уйти со страницы и вернуться (вперёд/назад) → состояние корректное, лишних запросов нет.

В отчёте напишите, что именно видели; «не проверял в браузере» лучше правдоподобной догадки.

## 8. Чего НЕ делать

- **Не запускать индексацию из Browse** (§2) — ни `/api/ingest`, ни `/api/demand`, ни `POST` на
  `/api/index-work`.
- **Не добавлять новых серверных маршрутов**: прогресс уже отдаётся. Если его не хватает — §9.
- Не менять `AUTO_INDEX` и стоячее решение «не включать сканирование текстов статей по умолчанию
  для архивов без нашего индекса» (`HANDOFF-NEXT.md`).
- Не удалять существующую ветку с карточкой `browse.indexRequired` — она остаётся.
- Не пушить; коммиты в `dev` (или в ветке, если пользователь так скажет), 1–2 коммита, сообщение
  английское и нарративное, как принято в этом репозитории.

## 9. Когда остановиться и доложить

- Если для наблюдения **не хватает** `indexWorkStatus` (например, он не даёт прогресс по одному
  словарю, так что числа не показать) — доложить, что именно даёт, и предложить минимальную
  правку API; молча расширять его не надо.
- Если задача, запущенная из панели, **не появляется** в статусе (разные ключи словаря) — это
  может быть настоящий баг; доложить, не «подгонять» интерфейс.
- Если ручная проверка расходится с §3 — стоп и вопрос пользователю.

## 10. Как закончить сессию

- `HANDOFF.md`: пункт про `8a48536d` переписать как сделанный (или убрать из «что осталось»);
  если появились новые ключи i18n — упомянуть там, где это принято.
- Удалить этот файл (`git rm docs/UPSTREAM-BROWSE-PROGRESS-PLAN.md`) и ссылку на него из
  `HANDOFF-NEXT.md`.

## 11. Что уже сделано (не переделывать)

Из апстримного диапазона `bf4e83cd..5b646bbe` взято 8 изменений, затем `65325a4d` (MDict)
целиком и четыре куска ZIM (предел цепочки редиректов; `isArticle`/`Resource` + `isContent`;
два числа в About для ZIM; ретрай без namespace-префикса в `Exact`). Полный список —
`git log --oneline --since=2026-11-04`.

Сознательно не взято и почему (не «забыто»): дедупликация списков ZIM по цели (цифры в
`HANDOFF.md`), wildcard-поиск апстрима и pin-модель панели (реестр мёртвого кода в `AGENTS.md`),
store-половина `96e9a334`, Android/CI-часть релизных коммитов. Отдельным заданием идёт
`docs/UPSTREAM-WMD-PLAN.md`.

## 12. Полезные команды

    git show 8a48536d -- internal/server/web/browse.html      # что делать (приёмы)
    git show c2196377 -- internal/server/web/browse.html      # что форк убрал и чем заменил
    git show 95f02382 -- internal/server/web/browse.html      # как это выглядело у нас раньше
    sed -n '160,200p' internal/server/index_work.go           # что отдаёт статус
    sed -n '265,300p' internal/server/index_work.go           # как он читается в запросе
    grep -n "GET /api/index-work" internal/server/routes.go
