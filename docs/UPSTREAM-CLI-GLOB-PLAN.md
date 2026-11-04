# Задание: globbing в `dictshelf res` и `dictshelf dump` (`4b00e48b`)

Это **задание, а не знание**. Когда оно выполнено — удалите этот файл целиком и снимите
ссылку на него из `HANDOFF-NEXT.md`; результат и числа — в телах коммитов и в `HANDOFF.md`.

Автор: свежая сессия 2026-11-04 (та, что написала три задания по продуктовым фичам — MEGA,
избранное, globbing в CLI).

**Ветка.** По `AGENTS.md` это новая фича: работа идёт в ветке **от `dev`**, не прямо в `dev` и
не в `master`. Предлагаемое имя: `cli-glob`.

---

## 1. Прочитать сначала (обязательно, в этом порядке)

- `AGENTS.md` — правила веток, реестр мёртвого кода (там же — почему wildcard-поиск апстрима
  не берут), шапки файлов, сборка `cgo` + теги.
- `docs/INDEX.md` — карта «какой вопрос к какому файлу».
- `HANDOFF.md`, разделы про `res`/`dump`, если они там есть, и общий «State by area».
- `HANDOFF-NEXT.md` — решения, которые легко отменить по незнанию.
- `docs/WINDOWS-VERIFY.md` и `thirdpartytools.md` — как и чем собирать здесь (`make` на PATH нет).
- Апстримный коммит: `git show --stat 4b00e48b` и `git show 4b00e48b`.
- Этот файл целиком.

## 2. Задача

Дать CLI две вещи из `4b00e48b` («feat(cli): globbing in `wudict dump` and `wudict res`»,
2026-10-31):

- **`res` принимает маски**: `dictshelf res Oxford.mdx '*.css' '*.js' -o /path/to/dir`, а также
  `-dry-run`;
- **`dump` получает `-include` / `-exclude` (повторяемые) и `-articles=false`**: «выгрузить
  всё, кроме ogg и mp3» и «только CSS/JS, без статей и аудио».

Почему это «продуктовая фича», а не порт: **имена команд и справка у форка свои** (`dictshelf`,
не `wudict`), а `openResources` опирается на форковый механизм форматов, у которого нет
апстримного `dict.SelfPrepares` (см. §4.4).

Форк сегодня (измерено 2026-11-04):

- `dictshelf res` принимает **ровно одно имя**, без масок и без `-dry-run`:
  `fs.NArg() != 2` → `usage: wudict res [-o out] [-f] <dictfile> <name>` (`internal/cli/cli.go:2345`,
  проверка на `:2350`).
- `dictshelf dump` знает только `-o/-output`, `-format`, `-mode`, `-compress`, `-resources`
  (`internal/cli/dump.go:50-58`); ни `-include`, ни `-exclude`, ни `-articles` нет.
- Матчера имён в форке **нет вовсе**: `git grep -nE 'path\.Match|filepath\.Match|func Glob' dev --
  internal/` → **0 совпадений**.

## 3. Что именно делает коммит (измерено)

`git show --stat 4b00e48b`: 12 файлов, +1557/−388.

- **`internal/resource/glob.go` — НОВЫЙ, 148 строк; матчер написан руками** (никакого
  `path.Match`/`filepath.Match`). Его собственный комментарий — это и есть спецификация
  (ниже — перевод; читать оригинал: `git show 4b00e48b:internal/resource/glob.go | sed -n
  '1,30p'`):

  > `*` — любое число символов **внутри одного** имени папки или файла;
  > `?` — один символ;
  > `**` целым именем между слэшами — любое число папок, **включая ни одной**;
  > концевой слэш `a/` — всё в папке `a`, на любой глубине.
  > Шаблон **без** слэша сопоставляется с именем файла на любой глубине («`*.ogg`» — все .ogg);
  > шаблон **со** слэшем — с начала. Обе стороны сравниваются через `Key`, поэтому регистр,
  > Unicode-нормализация, разделитель и ведущий слэш **не считаются** — как и при поиске.
  > Акценты считаются: `café.mp3` и `cafe.mp3` — два файла, поэтому это **не** `wildcard.Parse`.
  > Классов `[…]` и экранирования нет: имя вида `word [1].mp3` — обычное, а имя с буквальным
  > `*` или `?` достигается точным написанием, которое `res` пробует **раньше**, чем прочитает
  > строку как шаблон.

  Символы и строки: `Glob` `:29`, `globSeg` `:34`, `IsPattern` `:41`, `EndsInSeparator` `:46`,
  `NewGlob` `:52` (пустой шаблон и `.`/`..`/`/..` — ошибка; `/` — «все ресурсы»; концевой
  разделитель дописывает сегмент `**`; односегментный шаблон получает префикс `**`; `**/**`
  сворачивается в `**`), `Match` `:85`, `MatchKey` `:89`, `matchSegs` `:99` (серединный и
  ведущий `**` — **ноль или больше** имён, концевой `**` — **одно или больше**), `matchName`
  `:124` (с возвратом; `?` — одна **руна**). Зависимость — `resource.Key` (в форке есть).
- **`internal/resource/glob_test.go` — НОВЫЙ, 99 строк.** Среди случаев: `{"?.png","ж.png",true}`,
  `{"cafe.mp3","café.mp3",false}`, `word [1].*` как литерал, `{"*","",false}`.
- **`internal/cli/extract.go` — НОВЫЙ, 272 строки.** Общее для `dump` и `res`: `resSource`,
  `selfPrepared` (карта форматов → контейнеры ресурсов), `selfPreparedFormat(src)`,
  `openResources(src, format)` (импорт апстримных версий `openResources`/`sourceSet`/`dumpRes`
  из `dump.go`), `sourceSet`, `resFilePath`, `safeName`, `resJob`, `planRes` (дедуп по
  `resource.Key`, отказ от невыразимых имён и коллизий регистра), `resWrite`, `writeRes`
  (пишет через новый `fsx.WriteReplaceFrom` — **без fsync на каждый файл**).
  `selfPreparedFormat` в апстриме гейтится на `dict.SelfPrepares(src)`, затем по суффиксу
  (`.bgl` → `bgl`; `.dsl`/`.dsl.dz` → `dsl`; иначе `dict.Claims(src, …) == nil` → `wmd.Format`).
- **`internal/cli/res.go` — НОВЫЙ, 368 строк.** `cmdRes`, `resOne`, `resMany`, `selectRes`,
  `resMissing`, `existingDests`, `overwriteError`, `resDest`, `resBasename`, `isTerminal`.
  Флаги: `-o` (как раньше: файл, папка, `-`), `-f`, **`-dry-run`** (печатает `name\tdest` и
  ничего не пишет). Использование: `res [-o out] [-f] [-dry-run] <dictfile> <name|pattern>…`,
  требует `NArg() >= 2`. Сопоставление: если имя одно и не кончается разделителем — сначала
  **точный** `d.Resource(name)`, и только если не нашлось и это не шаблон — ошибка `resMissing`;
  иначе `resMany`→`selectRes`: по каждому аргументу сначала точное совпадение (буквальный `?`
  или `*` в настоящем имени побеждает), затем `NewGlob` против `resource.Filter(d.list())`;
  ноль совпадений — ошибка (`-dry-run '**'` перечисляет всё); склейка по `resource.Key`;
  порядок вывода — порядок аргументов.
- **`internal/cli/dump.go` — переписан** (447 строк изменено). Появились `withResources`,
  `dumpArticles`, `resSelect`, `newResSelect`, `pick`, `dumpPlan`, `planDump`, `writeDump`;
  **удалены** `dumpResources`, `dumpRes`, `openResources`, `sourceSet`, `dumpOneResource`,
  `resFilePath`, `safeName` (уехали в `extract.go`) и поля `csvDump.format`/`mdDump.format`.
  Новые флаги: `-include` и `-exclude` — **повторяемые** (`fs.Var` + `multiFlag`), `-articles`
  (bool, **умолчание true**). Отказы: `-resources none` вместе с include/exclude;
  `-articles=false` вместе с `-resources none`; `-articles=false` вместе с `-mode`/`-compress`;
  пустой шаблон и `..`. `-include`, не совпавший ни с чем, — **фатально**; `-exclude`, не
  совпавший ни с чем, — предупреждение в stderr.
- **`internal/fsx/fsx.go` (+19):** `WriteReplaceFrom(path, r, perm)`; у внутреннего `write`
  появляется параметр `sync bool`, а `WriteAtomic*` передают `true`.
- **`internal/cli/dump_md.go` (3 строки):** поле `mdDump.format` и его присваивание убраны.
- **Тесты:** `dump_test.go` +63 (`TestNewResSelect`, `TestResSelectPick` — чистые),
  `res_test.go` +181 (`TestSelectRes`, `TestPlanRes`, `TestResMany`; in-memory `mapRes`,
  `termStub`), `dump_md_test.go` +96 (`TestDumpResourcesOnly`, `TestDumpIncludeOpenFailure` —
  диск + `WUDICT_DB_DIR=t.TempDir()`).
- **`pages/docs/reference/cli.md` (+61)** — справка `res`/`dump` с таблицей шаблонов.

**Апстримное состояние:** `4b00e48b` — **единственная и последняя** версия этой фичи. После неё
`internal/cli/`, `internal/resource/`, `internal/fsx/` трогал только `476b5bf6` (XDG-папки,
`basedir.go`), и он не касается ни `dump.go`, ни `extract.go`, ни `res.go`, ни `glob*.go`.

## 4. Что СОХРАНИТЬ и что помешает (измерено 2026-11-04)

1. **`internal/cli/dump.go` форка — построчно апстримный предком** (`4b00e48b^`): у обоих 568
   строк, а `git diff 4b00e48b^ dev -- internal/cli/dump.go` показывает **только** пути импортов
   и текст пары комментариев. Значит переписывание `dump.go` ложится почти чисто; но вместе с
   ним обязаны уехать `dump_md.go` (поле `format`), `res_test.go` (новые символы) и переименование
   `dumpRes` → `resSource`.
2. **`internal/cli/cli.go` разошёлся сильно** (форк 2523 строки; расхождение в 1178 строк — это
   Windows/desktop/tray-код форка). Хунки коммита в `cli.go` — это текст справки (`:110-127`),
   комментарий `multiFlag` (`:1934-1942`) и **удаление** `cmdRes`/`resDest`/`resBasename`/
   `isTerminal` (форк `:2345`+). **Главная ловушка форка:**
   - **`applyLimits()` внутри `cmdRes`** (`cli.go:2353`). У апстримного `res.go` `cmdRes` его
     **нет**, потому что у апстрима нет форкового `applyLimits()` (no-arg, `cli.go:1675`).
     Принять `res.go` как есть — значит **молча снять** `MAX_*` с команды `res`. Форковый
     комментарий к `applyLimits` прямо называет `res` среди её вызовов (`cli.go:843`, `:883`,
     `:2353`). Обязательно сохранить вызов в новом `res.go` (и в новом `dump.go`, если он где-то
     открывает словарь по пути).
   - Форковая справка и комментарий `multiFlag` — не апстримные; обновлять их вручную, не
     заменять блоками апстрима.
3. **`internal/cli/extract.go` и `res.go` в форке отсутствуют** → новые файлы, конфликтов нет.
   `res_test.go` в форке — предкоммитный (232 строки: только `TestResBasename`, `TestResDest`,
   `TestWriteFileAtomic`); апстримный приносит три новых теста и свои хелперы.
4. **Единственное настоящее препятствие — `selfPreparedFormat` и `dict.SelfPrepares`.**
   Апстримный `extract.go` зовёт `dict.SelfPrepares(src)`, а в форке **нет**
   `internal/dict/urgent.go` (проверено: файла нет, `RegisterSelfPrepared`/`SelfPrepares` —
   0 совпадений) — это форковое состояние, записанное ещё в задании по wmd. **Но препятствие
   снимается своими средствами:**
   - форковый `internal/cli/dump.go` **уже** держит локальную карту `selfPrepared`
     (`dump.go:420`) с ключами `"dsl"`, `wmd.Format` и `"bgl"` и `openResources(src, format)`
     (`dump.go:436`) — то есть механизм форматов у форка свой и уже похож на апстримный
     посткоммитный;
   - `dict.MainFile` и `dict.Claims` в форке есть (`internal/dict/registry.go:96` — гейт по
     содержимому для suffix’ов, зарегистрированных через `dict.RegisterSniffed`, а их
     регистрирует `internal/format/wmd/wmd.go:35`);
   - кроме того, у форка есть свой реестр **ресурсов по формату**:
     `resource.Register(format, resource.Provider{…})` (`internal/resource/link.go:86`) с
     регистрациями для `dsl`, `dsl-gd`, `mdx`, `stardict`, `wmd` и чтением в
     `internal/server/registry.go:320`.
   Значит `-articles=false` (единственный случай, когда формат **нельзя** узнать из
   прочитанных статей) закрывается **двумя честными путями**, и выбрать надо осознанно,
   записав причину в тело коммита: **(а)** написать в `extract.go` свой вариант
   `selfPreparedFormat` из форковых кусков (суффиксы `.bgl`/`.dsl`/`.dsl.dz` + `dict.Claims`
   для wmd) — без порта `urgent.go`; **(б)** не изобретать определение формата вовсе, а
   потребовать его явно либо использовать `resource.Provider` по имени формата. Порт
   `internal/dict/urgent.go` целиком — **не** цель этой задачи: он тянет `store`-половину
   (urgent-очередь), которой у форка нет и которая к globbing отношения не имеет.
5. **`internal/resource/` в форке есть и уже даёт всё, что нужно:** `ListAll`, `MIME`, `Clean`,
   `Key` (`resource.go`), `Filter` (`junk.go:79`), интерфейс `Source`. `glob.go` ложится рядом
   без конфликтов имён; **не путать с `internal/wildcard/`** — это другой, мёртвый по решению
   форка движок (`AGENTS.md`), и `glob.go` сам предупреждает: «это не `wildcard.Parse`».
6. **`internal/fsx/fsx.go`:** форк добавил `FileSize` (+14); апстримное изменение
   (`WriteReplaceFrom` + параметр `sync`) — аддитивно и не конфликтует.
7. **`internal/cli/dump_md.go:159`** зовёт `openResources(src, meta.Format)`; переименование
   `*dumpRes` → `*resSource` едет вместе с переписыванием.
8. **Документация — форка:** `pages/docs/reference/cli.md` в форке есть и написан под
   `dictshelf` (`:148-153` — `res`, `:164-168` — `dump`), поэтому 61 строку апстрима
   **переписать**, а не копировать.

## 5. Что ВЗЯТЬ

Посмотреть, что именно там, до правок:

    git show 4b00e48b --stat
    git show 4b00e48b -- internal/resource/glob.go internal/cli/res_test.go
    git show 4b00e48b:internal/cli/extract.go | head -80

Новые файлы — **как есть** (шапка апстрима `// Copyright (C) 2026 glowinthedark` + SPDX, без
форковой строки; пути импортов — на `github.com/DMSHLabs/DictShelf/…`):

- `internal/resource/glob.go`, `internal/resource/glob_test.go` — ложатся без правок, кроме пути
  импорта (зависимость `Key` в форке есть);
- `internal/cli/res.go`, дополнение `internal/cli/res_test.go` (апстримные тесты поверх
  форкового файла — сохранить существующие три теста);
- `internal/cli/extract.go` — с **одной** осознанной правкой вместо `dict.SelfPrepares` (§4.4).

Существующие файлы — **руками, по одному смыслу**:

- `internal/cli/dump.go` — переписывание по апстриму, сохранив форковые импорты/комментарии и
  **не потеряв вызовы `applyLibrarySettings()`/`applyLimits()`** там, где они были;
- `internal/cli/cli.go` — справка `res`/`dump` и комментарий `multiFlag` приведены к новому
  использованию; `cmdRes`/`resDest`/`resBasename`/`isTerminal` **удалены** (они переехали в
  `res.go`) — и **`applyLimits()` в новом `cmdRes` сохранён** (§4.2);
- `internal/fsx/fsx.go` — `WriteReplaceFrom` + параметр `sync`;
- `internal/cli/dump_md.go` — как апстрим;
- `pages/docs/reference/cli.md` — переписать под `dictshelf`.

## 6. Границы

- **`-dry-run` ничего не пишет** — это контракт, а не удобство: `resWrite` не вызывается.
- **Границы вывода не ослаблять.** `resFilePath`/`safeName` держат дерево папок **ниже**
  `resDir`; маска не должна дать записать файл вне выходной папки. Проверить и записать.
- **Сопоставление — через `resource.Key`** (регистр, NFC, разделитель, ведущий слэш не считаются;
  акценты считаются). Не «улучшать» это до `filepath.Match` — это потеря смысла фичи.
- **Буквальное имя побеждает шаблон** (точный `Resource(name)` раньше `NewGlob`). Не менять
  порядок.
- **Новых ключей конфига и лимитов нет.** `MAX_*` продолжает действовать там, где действовала
  (см. §4.2) — и это проверяется, а не предполагается.
- **Область — только `res` и `dump`.** Никакого globbing в `ingest`/`keys`/`lookup` и никакого
  нового матчера для веба: апстрим этого не делал.

## 7. Гейты

    export PATH="/c/Qt/Tools/mingw1310_64/bin:$PATH"      # gcc: GCC_PATH в build-windows.local.bat
    export CGO_ENABLED=1
    gofmt -l internal/cli internal/resource internal/fsx
    go build -tags sqlite_fts5,sqlite_dbstat ./...
    go vet   -tags sqlite_fts5,sqlite_dbstat ./internal/cli/ ./internal/resource/ ./internal/fsx/
    go test  -tags sqlite_fts5,sqlite_dbstat ./internal/cli/ ./internal/resource/ ./internal/fsx/ -count=1
    go test  -tags sqlite_fts5,sqlite_dbstat ./...            # полный набор, обязателен

Тестам `internal/cli` **настоящий словарь не нужен** (проверено: ни одной `WUDICT_TEST_*` в
`internal/cli`); `dump`-тесты строят свои файлы и ставят `WUDICT_DB_DIR=t.TempDir()`.
Апстримные новые тесты тоже самодостаточны (`mapRes`, `t.TempDir()`).

**Мерить, а не предполагать:** после переноса прогнать `res` на настоящем словаре этой машины,
если он есть (пути в `docs/WINDOWS-VERIFY.md`, переменные `WUDICT_TEST_*` в `Makefile:39-44`:
`MDX/STARDICT/SLOB/DSL/ZIM`), и записать в отчёт **точные** числа/вывод — либо честное «на
настоящем файле не проверял». `-include`, не совпавший ни с чем, обязан быть фатальным, а
`-exclude` — предупредить: это проверяется на настоящем файле, а не только юнит-тестом.

## 8. Порядок работы (шаг = коммит)

0. **Не переключай ветку в общем каталоге.** `D:\Projects\Android\DictShelf` делят несколько
   сессий, а `git checkout -b` переносит чужие незакоммиченные правки на новую ветку. Работай в
   отдельном worktree — он уже создан (2026-11-04, ветка `cli-glob` от `dev`):

       cd D:/Projects/Android/DictShelf-cli-glob && git status    # ожидаем: ## cli-glob, дерево чистое

   Если его нет (удалён), создай заново:

       git -C D:/Projects/Android/DictShelf worktree add ../DictShelf-cli-glob -b cli-glob dev

   Игнорируемых локальных файлов этой машины (`build-*.local.bat`) в worktree нет, поэтому gcc
   для cgo бери из Qt — как и в основном каталоге:

       export PATH="/c/Qt/Tools/mingw1310_64/bin:$PATH"; export CGO_ENABLED=1

   Проверено 2026-11-04: `go build -tags sqlite_fts5,sqlite_dbstat ./...` в этом worktree
   проходит без вывода компилятора.
1. **Матчер:** `internal/resource/glob.go` + `glob_test.go` — как есть, собрать, прогнать
   `internal/resource`. Самодостаточно, ничего не подключено.
2. **Общее для двух команд:** `internal/cli/extract.go` с решением по §4.4 (в теле коммита —
   почему выбран этот путь и почему без `urgent.go`); `internal/fsx/fsx.go`.
3. **`res`:** `internal/cli/res.go`, удаление старых символов из `cli.go`, **`applyLimits()`
   сохранён**, справка. Тесты `res_test.go` (форковые три + апстримные три).
4. **`dump`:** переписывание `dump.go`, `dump_md.go`, `-include/-exclude/-articles`,
   комментарий `multiFlag`. Тесты `dump_test.go`.
5. **Документы:** `pages/docs/reference/cli.md`; `HANDOFF.md`.

Каждый коммит: сообщение на английском, нарративное: что сделано, почему, какими числами
проверено, что НЕ сделано. **Не пушить** без просьбы.

## 9. Чего НЕ делать

- **Не портить форковую обвязку `cli.go`** (Windows/desktop/tray, `applyLibrarySettings`,
  `applyLimits`, форковые комментарии): заменять блоки апстрима целиком нельзя.
- **Не терять `applyLimits()` в `res`** — это единственная известная тихая потеря от этого
  переноса (§4.2).
- **Не порт `internal/dict/urgent.go` ради `SelfPrepares`** (§4.4).
- **Не путать `glob.go` с `internal/wildcard/`** и не «оживлять» wildcard: в `AGENTS.md` он —
  мёртвый код по решению форка.
- **Не давать новым файлам форковую шапку** (они апстримные) и не переписывать апстримные
  комментарии (в них ASCII-дефис вместо тире — не «улучшать» пунктуацию).
- **Не менять поведение по умолчанию** `dump`: `-articles` по умолчанию `true`, `-resources`
  по умолчанию `all` — как было.
- **Не тянуть вперёд `476b5bf6`** (XDG-папки) — это другой коммит.

## 10. Когда остановиться и доложить пользователю

- Если для `-articles=false` **не** удаётся определить формат форковыми кусками без порта
  `urgent.go` — это вопрос, а не «портнуть на всякий случай»: доложить и решить с пользователем.
- Если после переноса `res`/`dump` **перестали** применять `MAX_*` — это регрессия: откат шага,
  не «дописать потом».
- Если `glob.go` на настоящем словаре даёт не то, что обещает его комментарий (например,
  `audio/` не берёт вложенное) — это баг апстрима: доложить с фактом, не подгонять тесты.
- Если объём вырос за «`glob.go` + `extract.go` + `res.go` + переписанный `dump.go`» (например,
  апстримный `extract.go` тянет `internal/store` или `internal/wildcard`) — доложить и разбить.

## 11. Чем закончить сессию

- `HANDOFF.md`: описать `res`/`dump` как сделанные (или убрать из «что осталось»), назвать
  решение по §4.4 и что осталось непроверенным (настоящий словарь, если его не было).
- Удалить этот файл (`git rm docs/UPSTREAM-CLI-GLOB-PLAN.md`) и снять ссылку из
  `HANDOFF-NEXT.md`.
- Завести/дополнить свой файл сессии в `docs/sessions/`.
- **Убрать worktree:** `git -C D:/Projects/Android/DictShelf worktree remove ../DictShelf-cli-glob`
  (и `git worktree prune`, если от неудачных попыток остались записи).

## 12. Что уже сделано в этом раунде (не переделывать)

Из апстримного диапазона `bf4e83cd..5b646bbe` взято 8 изменений, `65325a4d` (MDict) целиком,
четыре куска ZIM, wmd-порт (`96e9a334`), Browse-прогресс (`8a48536d`) и `internal/limits` (P6).
Список — `git log --oneline --since=2026-11-04`. Из `4b00e48b` не взято ничего — он и есть эта
задача.

## 13. Полезные команды

    git show 4b00e48b --stat
    git show 4b00e48b:internal/resource/glob.go | sed -n '1,60p'     # спецификация матчера
    git diff 4b00e48b^ dev -- internal/cli/dump.go                    # ожидание: только импорты/комменты
    git show 4b00e48b:internal/cli/cli.go | sed -n '110,140p'         # как выглядит справка после
    git grep -n "applyLimits" dev -- internal/cli/                    # что нельзя потерять
    git grep -n "selfPrepared" dev -- internal/cli/dump.go            # форковый механизм форматов
    git grep -n "resource.Register" dev -- internal/                  # реестр ресурсов по формату
    git cat-file -e dev:internal/dict/urgent.go; echo $?              # ожидание: 1 (нет SelfPrepares)
