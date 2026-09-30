Для корректировки **русского перевода** меняйте эти файлы. Все пути относительно `D:\Projects\Android\wudict`:

| Что переводится | Файл |
|---|---|
| Основной интерфейс: настройки, поиск, папки, словоформы, категории, озвучивание | [internal/server/web/i18n/ru.json](D:/Projects/Android/wudict/internal/server/web/i18n/ru.json) |
| Android: нативные окна, уведомления, сообщения оболочки | [android/app/src/main/res/values-ru/strings.xml](D:/Projects/Android/wudict/android/app/src/main/res/values-ru/strings.xml) |
| Дополнительные сообщения FOSS-сборки | [android/app/src/foss/res/values-ru/strings.xml](D:/Projects/Android/wudict/android/app/src/foss/res/values-ru/strings.xml) |
| Дополнительные сообщения Google Play-сборки | [android/app/src/play/res/values-ru/strings.xml](D:/Projects/Android/wudict/android/app/src/play/res/values-ru/strings.xml) |

**Английские оригиналы:** для веба — соседний `en.json`; для Android — `strings.xml` в соседней папке `values` вместо `values-ru`.

При исправлении формулировок:

- Меняйте только текст, сохраняя ключи JSON и атрибуты `name` в XML.
- Сохраняйте параметры: `{name}`, `{count}`, `%1$s`, `%1$d` и подобные.
- В счётных сообщениях учитывайте все формы: `one`, `few`, `many`, `other`.
- После правок пересоберите приложение.

Названия языков вроде «английский» автоматически берутся из справочника браузера — в этих файлах их нет.

Подробные правила: [translation.md](D:/Projects/Android/wudict/translation.md).