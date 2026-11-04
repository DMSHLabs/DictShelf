# Задание: подключить `internal/limits` (`MAX_*`) к конфигу и ко всем границам

Это **задание, а не знание**. Когда оно выполнено — удалите этот файл целиком и снимите
ссылку на него из `HANDOFF-NEXT.md`; результат и числа — в телах коммитов и в `HANDOFF.md`.

Автор: сессия 2026-11-04 (раунд апстрима, MDict, четыре ZIM-правки, wmd-порт, Browse-прогресс).

---

## 1. Прочитать сначала (обязательно)

- `AGENTS.md` — правила веток, лицензии/шапки, сборка (cgo + теги), запрет пуша.
- `docs/INDEX.md` — карта «вопрос → файл».
- `HANDOFF.md`, раздел **«Upstream reviewed on 2026-11-04, and what was left»** — пункт про
  P6 (`c8b92dda`, `f1c5fb27`) и пункт про ZIM (там же — почему ZIM трогают осторожно).
- `docs/WINDOWS-VERIFY.md`, раздел **«ZIM archives on this machine»** — фикстура и команды
  ZIM-гейта.
- `docs/UPSTREAM-TRANSFER-PLAN.md`, раздел **«Required invariants»** — инварианты ресурсов и
  медиа, которые нельзя сломать.
- `thirdpartytools.md` — чем собирать на этой машине.
- Этот файл целиком.

## 2. Задача

`internal/limits` уже есть в дереве (порт 2026-11-04, 103 строки, только stdlib) и **читается
ровно в одном месте** — `internal/gomdict/block.go:228` (`maxLZOBlock()` =
`limits.Get().MDictBlock`). Остальные восемь границ по-прежнему жёсткие константы, а ключей
`MAX_*` в конфиге нет вовсе.

Нужно: добавить ключи `MAX_*` в конфиг, вызывать `limits.Set(cfg.Limits)` на старте каждого
входа, и заменить каждую жёсткую константу чтением `limits.Get()` — **не меняя поведения по
умолчанию** (см. §4, это главная ловушка).

## 3. Таблица границ (проверено 2026-11-04)

| Поле `limits` | Ключ апстрима (min..max) | Наша константа сегодня | Наше значение | `limits.Default` |
|---|---|---|---|---|
| `ZIMCluster` | `MAX_ZIM_CLUSTER` (16 MiB..blockMax) | `maxClusterBytes`, `internal/format/zim/container.go:91` | 256 MiB | 256 MiB |
| `SlobBin` | `MAX_SLOB_BIN` (16 MiB..) | `maxItemBytes`, `internal/format/slob/container.go:714` | 256 MiB | 256 MiB |
| `MDictBlock` | `MAX_MDICT_BLOCK` (16 MiB..) | — | — | 256 MiB | **уже подключено** |
| `StarDictIndex` | `MAX_STARDICT_INDEX` (16 MiB..) | `maxIndexBytes`, `internal/format/stardict/stardict.go:261` | 256 MiB | 256 MiB |
| `BGLBlock` | `MAX_BGL_BLOCK` (8 MiB..) | `maxBlockBytes`, `internal/format/bgl/reader.go:140` | 64 MiB | 64 MiB |
| `LemmaSize` | `MAX_LEMMA_SIZE` (16 MiB..1 GiB) | `MaxPackBytes`, `internal/morph/file.go:51` | 64 MiB | 64 MiB |
| `ImportSize` | `MAX_IMPORT_SIZE` (1 GiB..64 TiB) | `maxDownloadBytes`, `internal/intake/fetch.go:72` | **16 GiB** | **256 GiB** |
| `ImportRatio` | `MAX_IMPORT_RATIO` (100..100000) | `maxRatio`, `internal/intake/extract.go:110` | **найти** | 2000 |
| `UploadSize` | `MAX_UPLOAD_SIZE` (1 GiB..64 TiB) | **не найдено** — найти путь загрузки | ? | 16 GiB |

Ключи и их min/max взяты из таблицы апстрима (`5b646bbe:internal/config/config.go:222-230`),
дефолты — из нашего `internal/limits/limits.go:37-47`.

## 4. Главная ловушка: наши значения ≠ `limits.Default`

`limits.Default` — это **апстримный** файл, и его числа не совпадают с тем, что форк делает
сегодня. Пример, который виден сразу: скачивание у нас ограничено **16 GiB**
(`maxDownloadBytes`), а `ImportSize` в дефолте — **256 GiB**. Подключить «как есть» значит
либо ослабить границу в 16 раз, либо (если взять min из таблицы) ужесточить её и начать
отказывать в словарях, которые сегодня импортируются.

Правило: **дефолт каждого ключа в конфиге должен воспроизводить сегодняшнее поведение форка**,
а не `limits.Default`. Порядок действий для каждой строки таблицы:

1. найди константу и её значение (`git grep -n "<имя>" dev -- internal/`);
2. если константы нет — найди фактическое поведение в коде (какой `LimitReader`, какой
   `MaxBytesReader`, какая проверка) и зафиксируй его числом;
3. поставь это число дефолтом ключа;
4. только после этого замени константу на `limits.Get().<Поле>`.

`limits.Default` при этом **не трогать** (это апстримный файл): он остаётся тем, чем его
сделал апстрим, и тесты, которым нужно вернуть состояние, делают
`t.Cleanup(func(){ limits.Set(limits.Default) })` — как уже делает
`internal/gomdict/block_test.go:121-122`.

## 5. Конфиг: как добавлять

У нашего `internal/config/config.go` **нет** табличного механизма апстрима (проверено: ни
`{"MAX_`, ни `keySpec`, ни списка настроек) — конфиг написан руками. Значит на каждый ключ:

- поле в структуре конфига (рядом с другими числовыми настройками — посмотри, как сделан
  ближайший по смыслу ключ, и повтори его форму);
- разбор значения: число с суффиксами (`256MB`, `1GB`) — посмотри, есть ли уже такой парсер
  (`grep -n "MB\|GB\|parseSize\|bytes" internal/config/*.go`); если нет — добавь, с тестом;
- проверка диапазона (min/max из таблицы §3) с внятной ошибкой, называющей ключ;
- строка в образце конфига (там, где `config.go` печатает закомментированный пример —
  у апстрима это `config.go:671+`; у нас найди аналог);
- тест: каждый ключ парсится, значение вне диапазона отвергается с сообщением, содержащим
  имя ключа.

## 6. Старт: где вызывать `limits.Set`

У апстрима это четыре места в `internal/cli/cli.go` (554, 1249, 1401, 2153) — по одному на
каждый вход. У нас найди свои входы:

    grep -n "^func cmd" internal/cli/cli.go

и поставь `limits.Set(cfg.Limits)` в каждом, **до** первого чтения словаря. Отдельно проверь
встроенные пути: Android-шелл и Windows-окно в процессе (`internal/cli/desktop_*.go`,
`android/nativebridge/`) — они не проходят через `cmdServe`, а лимиты им нужны те же
(это требование из `docs/UPSTREAM-TRANSFER-PLAN.md` §6: «make sure embedded Android startup
applies the same limits as CLI/server startup»).

## 7. ZIM — самый рискованный кусок

`maxClusterBytes` используется в **семи** местах `internal/format/zim/container.go`:

- `:664` — `n := min(end-start-1, uint64(2*maxClusterBytes))`: эвристика размера таблицы
  смещений. **Не подставляй сюда конфигурируемое значение слепо**: при пониженном лимите это
  может отвергнуть законный архив. Реши осознанно (оставить отдельную константу или связать с
  лимитом) и напиши решение в теле коммита;
- `:781`, `:787` — проверки таблицы смещений;
- `:891` — `zstd.WithDecoderMaxMemory(maxClusterBytes)`: **обязано** брать конфигурируемое
  значение, иначе пониженный лимит не ограничит сам декодер, а только пост-проверку;
- `:900`, `:904`, `:905` — `io.LimitReader(r, maxClusterBytes+1)` и проверка
  `errClusterTooLarge`.

Гейт для этого куска (обязателен, до и после):

    export PATH="/c/Qt/Tools/mingw1310_64/bin:$PATH"; export CGO_ENABLED=1
    export WUDICT_TEST_ZIM="D:/Dictionaries/wuDict/Dictionaries/zim/wiktionary_ru_all_nopic_2026-09.zim"
    go test -tags sqlite_fts5,sqlite_dbstat ./internal/format/zim/... -count=1
    go test -tags sqlite_fts5,sqlite_dbstat ./internal/format/zim/ -run 'TestRealFile' -count=1 -v
    #   ожидание: "2527809 names, 1528188 articles the archive claims";
    #             TestRealFileRedirects: "file version 6.3, content namespace 'C'",
    #             deepest chain 1 hop, 0 cycles, 0 unreadable
    go run -tags sqlite_fts5,sqlite_dbstat ./tools/xapian-glass-check "$WUDICT_TEST_ZIM" "кларнетист" "джаз"
    #   ожидание: "кларнетист" 11 статей / 11 вхождений, "джаз" 162 / 162, FullText 5 и 5

Плюс новый тест по образцу `internal/gomdict/block_test.go:121`: понизить лимит
(`limits.Set(limits.Limits{ZIMCluster: 1 << 20})`), открыть кластер, который его превышает, и
убедиться, что ошибка — `errClusterTooLarge` (а не паника и не тихое усечение), затем
`t.Cleanup(func(){ limits.Set(limits.Default) })`.

## 8. Тесты по каждому формату

Для каждой строки §3 — тест «пониженный лимит отвергает» по образцу gomdict: выставить
маленький лимит, подать блок/бин/индекс, который его превышает, проверить ошибку и что она
называет ключ (`limits.Exceeded` уже даёт это в тексте). Это то, что делает лимит
проверяемым, а не декоративным.

## 9. Гейты (полный список)

    gofmt -l internal/config internal/format internal/intake internal/morph internal/cli
    go build -tags sqlite_fts5,sqlite_dbstat ./...
    go vet   -tags sqlite_fts5,sqlite_dbstat ./internal/config/ ./internal/format/... ./internal/intake/
    go test  -tags sqlite_fts5,sqlite_dbstat ./internal/config/ ./internal/format/... ./internal/intake/ ./internal/morph/ -count=1
    go test  -tags sqlite_fts5,sqlite_dbstat ./...            # полный набор, обязателен

Плюс ZIM-гейт из §7 и, если менялся путь загрузки, — серверные тесты загрузки.

## 10. Чего НЕ делать

- **Не менять `limits.Default`** и не «подгонять» его под форк: это апстримный файл, и его
  числа — точка отсчёта для тестов.
- **Не менять поведение по умолчанию**: ни один словарь, который импортируется/открывается
  сегодня, не должен начать отвергаться после подключения. Если какое-то значение форка
  *выше* апстримного — оно остаётся выше.
- Не трогать `internal/store` (`pipeline.go`, `ingest.go`, `compress.go`) — это отдельная
  работа, и там своя история.
- Не добавлять ключ без строки в образце конфига и без теста диапазона.
- Не пушить; коммиты в `dev` (или в ветке, если пользователь так скажет), по одному на
  смысловой шаг (конфиг → старт → формат → ZIM), сообщения английские и нарративные.

## 11. Когда остановиться и доложить

- Если для какой-то границы **нет** сегодняшнего значения (например, путь загрузки ничем не
  ограничен) — это не «поставить дефолт апстрима», а вопрос: доложить и спросить, какое
  значение считать текущим.
- Если понижение лимита ломает законный архив (ZIM `:664`, таблица смещений) — доложить с
  числами, не «подкручивать» эвристику молча.
- Если подключение требует правки `internal/store` — стоп, это вне области (§10).
- Если ZIM-гейт дал другие числа, чем база (11/162, FullText 5/5) — стоп.

## 12. Как закончить сессию

- `HANDOFF.md`: пункт про P6 переписать как сделанный (или убрать из «что осталось»), назвать
  дефолты, которые остались форковыми, и почему.
- Удалить этот файл (`git rm docs/UPSTREAM-LIMITS-PLAN.md`) и ссылку на него из
  `HANDOFF-NEXT.md`.

## 13. Что уже сделано (не переделывать)

`internal/limits` портирован целиком (2026-11-04) и подключён к MDict
(`internal/gomdict/block.go:228`); `internal/gomdict/block_test.go` уже показывает, как
понижать лимит в тесте. Из апстримного диапазона `bf4e83cd..5b646bbe` взято 8 изменений,
`65325a4d` (MDict) целиком, четыре куска ZIM, wmd-порт (`96e9a334`) и Browse-прогресс
(`8a48536d`). Полный список — `git log --oneline --since=2026-11-04`.

## 14. Полезные команды

    git show 5b646bbe:internal/config/config.go | sed -n '215,235p'   # таблица ключей апстрима
    git show 5b646bbe:internal/config/config.go | sed -n '665,690p'   # образец конфига
    git grep -n "limits.Set" 5b646bbe -- internal | head              # где апстрим ставит лимиты
    git grep -n "limits.Get()" 5b646bbe -- internal | head -20        # все читатели у апстрима
    sed -n '37,47p' internal/limits/limits.go                         # наши дефолты
