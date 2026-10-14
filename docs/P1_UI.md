# Ассистент Гена: интерфейс и ручная отправка диктовки

React 19 / TypeScript 7 / Vite 8 в `web/`, production build обслуживается Go. Все 8 страниц работают с реальными данными. Макеты задают visual language, а не события, даты или счётчики.

## Запуск

```bash
docker compose up --build -d
# http://localhost:8081
```

`compose.go.yaml` сохраняет прежнюю команду Go-запуска и тот же том `rhythm-go-data`. Python reference запускается через `docker compose -f compose.python.yaml up --build -d` на порту 8000, со своим прежним томом. `app/` и Python-тесты остаются на месте.

Для разработки нужны Go 1.26+ (production: 1.27.2), Node 24 и pnpm 11.25:

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go run ./cmd/server
# http://localhost:8081; web/dist должен быть собран
```

Для hot reload запустите Go на 8082 с отдельной базой и затем Vite:

```powershell
$env:APP_ADDR = '127.0.0.1:8082'
$env:DATABASE_PATH = 'var/p1-dev.sqlite3'
go run ./cmd/server
# во втором терминале
pnpm --dir web dev
```

Vite proxy направляет `/api` и `/health` на 8082. В production нет Node/Python. Multi-stage Docker: Node → `web/dist`; Go → binary; distroless nonroot → binary + static frontend. Объявленные SPA routes обслуживают index.html при refresh. API 404 остаётся JSON.

## Модули и данные

`app/` содержит routing и небольшие global providers. Pages разделены по 8 каталогам; components содержат reusable UI и native dialogs; features разделяют voice, assistant, events, calendar и widget registry; api — единственное место с fetch; hooks — debounced server search; styles — общие tokens и responsive layout; types — Go DTO.

DataContext загружает все страницы `/api/events` (limit=200), поэтому записи за пределами первой страницы не теряются. Calendar, группы и counters вычисляются из этого набора. Поиск выполняется Go API; результаты получают актуальные metadata из общего cache после mutations. Обновление после записи и раз в минуту позволяет другим локальным вкладкам увидеть изменения. Отдельный Redux/query framework не нужен для personal dataset из 1000 строк.

Текущая дата берётся из `/health.now` с `/health.timezone`. Calendar реализует month/week/day, selected date, create/edit/delete. Нет отдельной `/api/calendar`: используются event API. Task может быть без срока; priority — настоящий enum Go. Meeting location/meeting_url добавлены минимальной миграцией schema v3, с backup и tests миграции v1/v2. URL проверяется как HTTP(S). Conflicts вычисляет только Go; force требует отдельного действия пользователя.

Reminder использует status open/done как завершение/возобновление, а не фиктивное enable/disable. Repeat и sound controls отсутствуют. Archive содержит прошлые и выполненные записи; search/type/date/status filters работают. Яндекс показывает «Не подключён», Настроить открывает информацию о P2; остальные сервисы — «Скоро».

Widget registry содержит Calendar/Today/Tasks/UpcomingMeetings/Progress. Desktop panel можно скрыть/открыть; tablet/mobile используют drawer. Layout registry отделён от компонентов для P3. Локально сохраняются имя, TTS и видимость панели; drag & drop и его persistence не реализованы.

## Voice

VoiceController имеет только draft, recognition factory и callback состояния. API/submit callback в нём отсутствует. onresult заменяет partial на final для текущего recognition run; onend продолжает новый run, добавляя его текст к draft. Повторные и запоздалые callbacks игнорируются. Редактирование клавиатурой останавливает текущий run, после чего microphone продолжает диктовку с изменённого текста.

Send/Enter останавливает recognition и вызывает assistant. Shift+Enter — перенос строки. Mutex защищает от двойного Send/Enter. При network error draft сохраняется, повтор использует тот же request_id для idempotency. Permission/network STT errors ничего не отправляют. TTS сохранён и настраивается в Settings.

Новый UI переиспользует корректный legacy browser session UUID и настройку TTS, чтобы сохранить доступ к истории. При повреждённом UUID создаётся новый. Озвучивание по умолчанию выключено и включается в Settings.

Автоматический browser voice test использует контролируемый Web Speech mock и **реальный React + Go HTTP**, без подмены `/api/chat`. Это проверяет voice lifecycle и сетевые эффекты, но не качество распознавания живого микрофона. Реальное STT зависит от браузера, разрешения microphone и его сервиса распознавания.

## Проверки

```bash
go test ./...
go vet ./...
go test -race ./...
python -m pytest -q
node --test tests/voice.test.js
pnpm --dir web test
pnpm --dir web build
pnpm --dir web exec playwright install chromium
pnpm --dir web e2e
```

E2E запускает отдельный Go server на 8083 и отдельную `var/p1-e2e.sqlite3`; созданные тестами события удаляются в этом test workflow. При `P1_BASE_URL` запуск сервера пропускается: адрес должен указывать на выделенный test runtime. Не используйте рабочую базу для E2E.

Проверены 1920×1080, 1440×900, 1280×800, tablet 834×1112 и mobile 390×844 на всех 8 routes: отсутствие horizontal overflow, drawers и refresh. Native dialog получает initial focus, удерживает Tab/Shift+Tab, закрывается ESC и возвращает focus. Все icon buttons подписаны; voice state доступен screen reader; reduced motion поддерживается.

Скриншоты и voice proof: [галерея](P1_SCREENSHOTS.md). Пересоздать локальные screenshot artifacts: `P1_CAPTURE_SCREENSHOTS=1 pnpm --dir web e2e` (PowerShell: `$env:P1_CAPTURE_SCREENSHOTS='1'`). CI сохраняет Playwright report, screenshots и traces failures как artifact, не обновляя tracked screenshots.

## Схемы

![Архитектура](diagrams/p1-architecture.svg)

![Voice flow](diagrams/p1-voice-flow.svg)

Открываются в работающем Go приложении: `/architecture`. Исходник SVG: `python -m scripts.build_p1_diagrams`. Python нужен только для перегенерации документации, не для runtime.

P2 CalDAV/OAuth и P3 draggable widget persistence остаются следующими этапами.
