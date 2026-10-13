# Ритм — личный ассистент для задач и встреч

React / TypeScript / Vite + Go + PostgreSQL. Email/password, Яндекс ID и VK ID, серверные сессии, изоляция данных пользователей и короткий onboarding. Голос или текст становятся редактируемым черновиком; сообщение отправляется **только по Send или Enter**. Пауза, onend и остановка microphone ничего не отправляют.

![Ритм — Сегодня](docs/screenshots/p1/today.png)

## Запуск

Нужен Docker Engine / Docker Desktop и Compose v2:

```bash
git clone https://github.com/NIDJEL/assistant-ai.git
cd assistant-ai
cp .env.example .env
# Задайте POSTGRES_PASSWORD и SESSION_SECRET (32+ случайных символа) в .env.
docker compose up --build -d
```

[Приложение — localhost:8081](http://localhost:8081), [OpenAPI](http://localhost:8081/docs), [блок-схемы](http://localhost:8081/architecture). Эти адреса работают на компьютере, где запущен сервер. Выбраны локальный запуск и Docker.

```bash
docker compose logs -f app
docker compose down
```

События, история, аккаунты, сессии и настройки сохраняются в PostgreSQL, том `rhythm-postgres-data`. Compose содержит `app`, `postgres` и optional `ollama`, PostgreSQL healthcheck и ожидание readiness. Runtime работает без root, Node, Python и FastAPI. Node нужен только на стадии build. Старый `docker compose -f compose.go.yaml up --build -d` также работает с тем же PostgreSQL-томом и портом.

## Интерфейс

Сегодня: assistant, история, уточнения и карточки событий. Календарь: month/week/day, выбор даты и CRUD. Задачи: сроки, priority, поиск, завершение/возобновление. Встречи: duration, место, URL, перенос и конфликты. Напоминания: дата/время, edit/delete/complete. Архив: история и удалённые записи с кнопкой «Восстановить», search/type/date/status filters. Удаление сохраняет запись в PostgreSQL; восстановление возвращает прежние ID, дату, статус и детали. Записи, физически удалённые до schema v5, этим способом не восстановить. Настройки: имя, TTS, видимость виджетов и режим анимаций.

Навигация расположена слева и сворачивается до иконок, виджеты — справа. При скрытых виджетах ширина центрального содержимого ограничена 1280 px, а оболочка занимает доступный экран. На мобильном экране меню открывается слева. Дни и фильтры меняют содержимое без движения заголовков; завершённые напоминания плавно сворачиваются с сообщением и кнопкой «Отменить».

Все даты и счётчики основаны на Go API и ограничены текущим пользователем. Новый аккаунт пустой. Явный `POST /api/import-demo` с cookie импортирует CSV из 1000 записей только в этот аккаунт; повторный импорт не создаёт дубли.

Яндекс Календарь показывает «Не подключён» и подготовленный UI P2. Google Calendar, Telegram, Zoom, Notion и Slack — «Скоро». Live CalDAV остаётся этапом P2. Вход через Яндекс ID работает отдельно от календаря; он предлагает настройку, а не отмечает календарь подключённым. P1.1 добавляет визуальное выравнивание, подзадачи, участников встреч, inline-редактор напоминаний и draggable widgets с хранением настроек в PostgreSQL. Приложение остаётся personal assistant.

## Голос

Microphone расположен справа перед Send. Partial/final transcript объединяются в draft, который можно редактировать. После паузы диктовка продолжается; после редактирования можно снова нажать microphone. Send останавливает recognition и отправляет один запрос. Enter отправляет, Shift+Enter переносит строку. Ошибки STT не отправляют текст; при сетевой ошибке chat черновик сохраняется, а повтор использует тот же request_id. Озвучивание ответов управляется в Settings.

Web Speech API требует поддержки браузера, разрешения microphone и обычно соединения с сервисом распознавания. При отсутствии поддержки работает текстовый ввод. Browser E2E использует контролируемый Speech mock с настоящим Go API; он проверяет UX и количество HTTP-запросов, а не качество живого распознавания.

## Разработка и проверки

Go 1.25+, Node 24, pnpm 11.25:

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go run ./cmd/server
```

Go требует `DATABASE_URL` и `SESSION_SECRET`, использует PostgreSQL, порт 8081 и `.env`/переменные процесса. SQLite не является runtime fallback. Для Vite hot reload Go запускается на 8082, затем `pnpm --dir web dev`. Подробная конфигурация и Windows-команды: [P1_UI.md](docs/P1_UI.md).

```bash
go test ./...
go vet ./...
go test -race ./...
pnpm --dir web test
pnpm --dir web build
pnpm --dir web exec playwright install chromium
pnpm --dir web e2e
```

На Windows без C compiler race tests выполняются в Linux Docker, как описано в [Go parity](docs/GO_PARITY.md). E2E использует отдельный Go-сервер на 8083 и PostgreSQL (`TEST_DATABASE_URL`); каждый product test создаёт свой аккаунт. CI проверяет Go race на 1.25/1.27, frontend tests/build/E2E, оба Docker runtime и неизменённый Python corpus.

## Authentication и PostgreSQL

При старте frontend проверяет `/api/auth/me`, показывает splash и направляет на `/auth`, если сессии нет. Login/Register повторяют approved reference. После регистрации — три коротких шага onboarding; Settings → Аккаунт показывает профиль, способы входа и logout. Sidebar использует текущего пользователя.

Пароли: Argon2id. Cookie: HttpOnly, SameSite=Lax, Secure на production/HTTPS. Сессия хранится на сервере, переживает refresh и инвалидируется logout. Пользовательские события, search, история, idempotency, preferences, widgets и calendar connections ограничены user_id во всех PostgreSQL-запросах.

Миграции выполняются автоматически в транзакции под advisory lock. Импорт старой SQLite: `go run ./cmd/import-sqlite -sqlite var/assistant-go.sqlite3`, с `IMPORT_USER_PASSWORD` в окружении. Старую SQLite-реализацию и данные сохранили. Полная инструкция по импорту, окружению и безопасности: [P1.2](docs/P1_2_AUTH_POSTGRES.md).

Яндекс: `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET`, `YANDEX_REDIRECT_URI`. VK: `VK_CLIENT_ID`, `VK_CLIENT_SECRET` если выдан, `VK_REDIRECT_URI`. Callback URI должен совпадать с PUBLIC_URL. Оба flow используют одноразовый state, browser binding и PKCE. Для проверки живого входа нужны настроенные приложения провайдеров; локальные тесты проверяют протокол с HTTP mocks. Account linking выполняется явно из настроек. Восстановление пароля по email пока не реализовано; интерфейс сообщает об этом.

![Ритм — вход](docs/screenshots/p1-2/login-desktop.png)

## Python reference и P0

`app/`, 73 Python-теста, 6 legacy voice-тестов и corpus сохранены на прежних местах. Запуск reference: `docker compose -f compose.python.yaml up --build -d` на 8000, со старым Python-томом. [Историческая инструкция](docs/PYTHON_REFERENCE.md).

```bash
python -m pytest -q
node --test tests/voice.test.js
```

SQLite настроен на WAL, busy_timeout=5000 и foreign_keys=1. Миграции создают согласованный backup и сохраняют ID/history/pending. Schema v3 добавляет место и URL встречи; v4 — метки, подзадачи, участников, состояние напоминаний и preferences. LLM HTTP выполняется вне короткой write transaction. Hybrid NLP и исходный общий corpus P0 сохранены.

## Документация

- [P1.2: auth architecture, PostgreSQL, OAuth, import и tests](docs/P1_2_AUTH_POSTGRES.md)
- [Approved auth reference / actual comparison](docs/screenshots/p1-2/index.html)

- [P1.1: visual alignment, REFERENCE / ACTUAL и проверки](docs/P1_1_DESIGN_PARITY.md)

- [P1: модули, runtime, ограничения и проверки](docs/P1_UI.md)
- [Desktop screenshots всех страниц, mobile и voice proof](docs/P1_SCREENSHOTS.md)
- [Implementation plan и baseline](docs/P1_IMPLEMENTATION.md)
- [Актуальный Go API](docs/API_CONTRACT_NEXT.md)
- [Go foundation и parity](docs/GO_PARITY.md)
- [Приоритеты P0–P4](docs/MIGRATION_PLAN.md)

![Архитектура P1](docs/diagrams/p1-architecture.svg)

![Ручная отправка voice](docs/diagrams/p1-voice-flow.svg)
