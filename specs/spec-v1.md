# mdview v1 — Specification

## Scope

CLI-утилита: принимает путь к `.md`-файлу, открывает окно с отрендеренным HTML.

## Функциональные требования

| # | Требование |
|---|---|
| F1 | Запуск: `mdview <path>`. Если аргумент не передан или файл не существует — ошибка в stderr, exit code 1 |
| F2 | Рендеринг Markdown: GFM, таблицы, strikethrough, task lists |
| F3 | Светлая тема, шрифт 18px, читаемая типографика, max-width контента |
| F4 | Заголовок окна: `<filename> — mdview` |
| F5 | Выход: `Escape` |
| F6 | Перезагрузка (перечитать файл + перерендерить): `F5`, `Ctrl+R` |

## Вне скопа v1

- Подсветка синтаксиса кода
- Дерево файлов
- Автообновление при изменении файла
- Открытие в редакторе

## Архитектура

```
src/
  main.go                         — точка входа, парсинг аргументов
  internal/
    renderer/                     — Markdown → HTML (goldmark)
      assets/
        style.css                 — тема оформления
        template.html             — HTML-шаблон страницы с JS хоткеями
    viewer/                       — управление окном WebView2, IPC
    watcher/                      — интерфейс FileWatcher (no-op в v1)
```

**Принципы:**
- `renderer` и `viewer` не зависят друг от друга — тестируются изолированно
- `watcher.FileWatcher` — интерфейс; в v1 — no-op, в v2 — fsnotify
- Хоткеи обрабатываются в JS (`keydown`), команды передаются в Go через WebView `Bind`
- HTML генерируется Go и передаётся в WebView через `SetHtml` (не файловый сервер)

## Юнит-тесты

| Пакет | Что тестируем |
|---|---|
| `renderer` | HTML для таблиц, кода, заголовков, strikethrough, task list; пустой файл |
| `viewer` | формирование заголовка окна из пути к файлу |

Интеграционных тестов в v1 нет (WebView2 требует дисплея).

## Сборка

```bash
cd src
go get github.com/jchv/go-webview2 github.com/yuin/goldmark
go mod tidy
go build -ldflags="-H windowsgui" -o ../mdview.exe .
```

Флаг `-H windowsgui` убирает консольное окно при запуске через проводник.
При запуске из терминала stderr/stdout доступны в обычном режиме.

## Тестирование

```bash
cd src
go test ./...
```

## Требования к окружению

- Go 1.22+
- Windows 10 21H2+ или Windows 11 (WebView2 предустановлен)
- Для более старых Windows: [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
