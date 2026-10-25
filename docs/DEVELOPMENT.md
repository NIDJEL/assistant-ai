# Разработка и проверки

[← Документация](README.md)

Версии инструментов задают `go.mod`, `web/package.json`, `Dockerfile` и CI: Go 1.26+ с toolchain 1.27.2, Node.js 24, pnpm 11.25, PostgreSQL 17.

## Локальный сервер

Скопируйте `.env.example` в `.env`, задайте `DATABASE_URL` на доступную PostgreSQL и `SESSION_SECRET` не короче 32 символов. Обычный локальный Compose не публикует PostgreSQL-порт: native Go требует отдельно доступную базу.

Для работы без внешних ключей явно задайте:

```dotenv
GENA_NLU_MODE=semantic_plan_legacy
GENA_SEMANTIC_PROVIDER=rules
NLP_PROVIDER=rules
GENA_STT_PROVIDER=
```

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go run ./cmd/server
```

По умолчанию сервер читает `.env` из корня проекта; переменные процесса имеют приоритет, включая пустые значения. Миграции PostgreSQL применяются при старте. Для AI настройте [GigaChat](GIGACHAT_CONFIGURATION.md), для диктовки — [Groq](DEADLINES_AND_VOICE.md).

## Vite и горячая перезагрузка

Для двух отдельных терминалов задайте в серверном окружении:

```dotenv
APP_ADDR=127.0.0.1:8082
PUBLIC_URL=http://127.0.0.1:5173
YANDEX_REDIRECT_URI=http://127.0.0.1:5173/api/auth/yandex/callback
```

Первый терминал: `go run ./cmd/server`. Второй: `pnpm --dir web dev`. Откройте `http://127.0.0.1:5173`: Vite проксирует `/api` и `/health` на порт 8082. Если порт Vite изменился, обновите origin и callback; OAuth callback также должен быть зарегистрирован у провайдера.

Для Ollama в `simple_llm` используйте `GENA_SEMANTIC_PROVIDER=ollama`, `OLLAMA_BASE_URL` и `OLLAMA_MODEL` с заранее загруженной моделью. Профиль Compose `ollama` запускает сервис, но не загружает модель автоматически.

## Go и PostgreSQL

```bash
go mod verify
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
```

`TEST_DATABASE_URL` указывает на отдельную тестовую PostgreSQL. Интеграционные тесты создают и удаляют собственные схемы; без переменной PostgreSQL-проверки пропускаются. Не используйте рабочую базу. Race detector требует CGO и C-компилятор; штатная проверка выполняется в Linux CI.

SQLite, `migrations/`, `testdata/parity/` и legacy semantic-корпуса остаются регрессионными фикстурами. Они не определяют runtime production. Перенос прежних данных описан в [аккаунтах](ACCOUNTS.md).

## Frontend и браузерные сценарии

```bash
pnpm --dir web test
pnpm --dir web build
pnpm --dir web exec playwright install chromium
pnpm --dir web e2e
```

E2E требует `TEST_DATABASE_URL`. Playwright сам запускает Go на `127.0.0.1:8083`, выбирает legacy rules provider и создаёт тестовые аккаунты. При `P1_BASE_URL` используется уже запущенный сервер; самостоятельно направляйте тесты только на тестовую установку.

HTML-отчёт: `web/playwright-report/`. Артефакты ошибок: `web/test-results/`. Браузерные mocks проверяют запись, черновик и ручную отправку, но не качество реального микрофона, согласие Яндекс OAuth или доставку push на телефон.

## AI smoke

```bash
go run ./cmd/simple-smoke -mode fixture -output artifacts/simple_llm_smoke_fixture.json
```

Live и интерактивный smoke требуют явного запуска и настроенного провайдера. Они используют временную SQLite-базу. [Подробности](SIMPLE_LLM_RUNTIME.md).

## CI

[GitHub Actions](../.github/workflows/ci.yml) проверяет:

- Одну production-версию Go из `toolchain` в `go.mod`: зависимости, vet, race и тесты с PostgreSQL 17.
- Frontend: все unit-тесты с двумя workers, TypeScript и production build.
- Chromium smoke: CRUD, конфликт/перенос встреч, напоминания, архив и проверки паролей в обеих темах.
- Механизм выпуска: валидацию архива, сохранение настроек и откат при неуспешном health check.
- Перед выпуском `main`: runtime Docker-образ из уже собранных файлов, auth/CRUD, сохранение сессии и данных после restart и штатную остановку. Повторной Go/Vite-сборки для этого нет.

Workflow запускается на PR в `main` и push в `main`, когда изменяется приложение или инфраструктура. Push в рабочую ветку отдельно его не запускает; изменения только Markdown-документации пропускаются. Устаревшие проверки PR отменяются, выпуск `main` завершается последовательно. Кешируются Go и pnpm; повторной сборки приложения внутри Docker нет. Артефакты хранятся 3 дня.

Полную браузерную регрессию запускают через **Actions → CI/CD → Run workflow → full_browser**. Для ручного выпуска выберите ветку `main` и `deploy`. После успешных проверок push в `main` выпускается автоматически: [настройка и эксплуатация CI/CD](../deploy/CI_CD.md).

Состояние конкретного коммита проверяйте в Actions. Документация не фиксирует число тестов и имена собранных assets.

## Обновление скриншотов

После сборки и запуска тестовой установки:

```bash
pnpm --dir web exec node scripts/capture-readme.mjs
```

Скрипт создаёт демонстрационный аккаунт и сохраняет изображения в `docs/screenshots/current/`. По умолчанию используется `http://localhost:8081`; `P1_BASE_URL` задаёт другой адрес. Проверьте изображения перед коммитом.
