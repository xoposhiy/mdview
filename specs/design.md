# mdview — Design Document

## Язык и рантайм

**Выбор: Go 1.22+**

| Вариант | За | Против |
|---|---|---|
| **Go** | один бинарник, быстрый старт, простая сборка | чуть медленнее Rust |
| Rust | быстрее, меньше бинарник | сложнее, WebView2-биндинги сырее |
| Python | богатая экосистема | медленный старт, нужен рантайм |
| C# | хорошие WPF-компоненты | нужен .NET, медленнее стартует |

---

## WebView (окно приложения)

**Выбор: [go-webview2](https://github.com/jchv/go-webview2)**

Встраивает WebView2 (Edge/Chromium) без CGO через статически слинкованный загрузчик.

| Вариант | За | Против |
|---|---|---|
| **go-webview2** | нет CGO, один .exe, WebView2 системный | только Windows |
| webview/webview | кроссплатформенный | требует CGO, сложнее сборка |
| Wails | красивый фреймворк | избыточен, медленнее стартует |
| Tauri (Rust) | отличная экосистема | другой язык |

WebView2 предустановлен в Windows 10 21H2+ и Windows 11. Для более старых — автоустановка через bootstrapper (~2 МБ).

---

## Парсер Markdown

**Выбор: [goldmark](https://github.com/yuin/goldmark)**

| Вариант | За | Против |
|---|---|---|
| **goldmark** | GFM-совместимый, расширяемый, активная поддержка | — |
| blackfriday | популярный | устаревший, v2 не развивается |
| gomarkdown | простой | нет GFM-таблиц |
| markdown-it (JS) | лучший рендеринг | JS в webview, нет контроля из Go |

Расширения goldmark: `extension.GFM` (включает Table, Strikethrough, TaskList, Linkify).

---

## Подсветка синтаксиса

**Выбор: [chroma](https://github.com/alecthomas/chroma)**

| Вариант | За | Против |
|---|---|---|
| **chroma** | 200+ языков, нативный Go, интеграция с goldmark | — |
| highlight.js (JS) | популярный | запускается в webview, задержка |
| Prism.js (JS) | лёгкий | то же |

Интеграция: `goldmark-highlighting` — плагин, рендерит блоки кода через chroma в HTML на этапе парсинга.

---

## Файловый watcher

**Выбор: [fsnotify](https://github.com/fsnotify/fsnotify)**

| Вариант | За | Против |
|---|---|---|
| **fsnotify** | стандарт де-факто в Go, нативный на Windows | debounce нужен вручную |
| polling | простой | тратит CPU |
| `ReadDirectoryChangesW` напрямую | нативный Windows API | много boilerplate |

---

## Файловое дерево (UI)

Реализуется на стороне JS внутри WebView: простой `<ul>`-список, сгенерированный Go и переданный через `ExecuteScript`.

Сторонних зависимостей не нужно.

---

## Итоговые зависимости

```
yuin/goldmark                  — парсер Markdown
yuin/goldmark-highlighting     — интеграция chroma с goldmark (v2+)
alecthomas/chroma/v2           — подсветка синтаксиса
jchv/go-webview2               — окно приложения (WebView2)
fsnotify/fsnotify              — слежение за изменениями файла
```

Итого: 5 внешних пакетов. Один `.exe` ~10–12 МБ.
