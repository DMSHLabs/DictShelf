# Сборка DictShelf по каналам

Обе команды принимают параметр `--channel`: `release`, `pre-release` или `beta`.
Если параметр не указан, сборка использует обычную версию из Git, как и раньше.

## Android

```bat
build-android.cmd release --channel release
build-android.cmd release --channel pre-release
build-android.cmd release --channel beta
```

ABI можно указать перед каналом:

```bat
build-android.cmd release arm64-v8a --channel beta
build-android.cmd release armeabi-v7a --channel pre-release
build-android.cmd all --channel beta
```

Для отладочной сборки сохраняется прежний выбор `debug`; к нему тоже можно
добавить канал, например `build-android.cmd debug intel --channel beta`.

## Windows

Собрать приложение для выбранного канала:

```bat
build-windows.cmd --channel release
build-windows.cmd --channel pre-release
build-windows.cmd --channel beta
```

Затем, если нужен установщик, упаковать собранный `DictShelf.exe` с тем же
каналом:

```bat
build-windows.cmd installer --channel beta
```

Установщик проверяет, что версия существующего `DictShelf.exe` соответствует
выбранному каналу. Для Pre-release или Beta сначала пересоберите EXE с тем же
каналом.

## Номер версии и публикация

- **Release** оставляет версию без дополнительного суффикса.
- **Pre-release** добавляет суффикс `-pre`.
- **Beta** добавляет суффикс `-beta.1`, если в версии ещё нет слова `beta`.
  Проверка Beta нечувствительна к регистру и ищет это слово в любом месте
  номера версии.
- Явный `WUDICT_VERSION` остаётся основой версии; выбранный канал добавляет
  нужный суффикс.
- Сборка создаёт файлы приложения, но не публикует их. При публикации Pre-release
  на GitHub также отметьте сам релиз как **Pre-release**. Beta определяется по
  номеру версии и может одновременно иметь GitHub-признак Pre-release.

Примеры выше собирают артефакты для соответствующего канала, но не устанавливают
APK и не публикуют релиз.
