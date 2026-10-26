# Разработка и проверки

[Документация](README.md) · [Быстрый запуск](QUICKSTART.md) · [Результаты проверки к сдаче](ASSIGNMENT_VALIDATION.md)

Версии закреплены в `go.mod`, `web/package.json`, Dockerfile и CI: Go с toolchain 1.27.2, Node.js 24, pnpm 11.25, PostgreSQL 17. Для обычного запуска достаточно Python 3.10+ и Docker Compose v2; инструменты ниже нужны для разработки.

## Полный локальный комплект

```bash
python scripts/service.py up
python scripts/service.py check
```

Launcher собирает frontend и Go внутри Docker. Секреты и PostgreSQL сохраняются между запусками. Команды управления, другой порт, AI и сертификаты — в [QUICKSTART](QUICKSTART.md).

## Native Go и Vite

Скопируйте `.env.example` в `.env`, задайте `DATABASE_URL` на отдельную доступную PostgreSQL и случайный `SESSION_SECRET` не короче 32 символов. Базовый Compose не публикует порт базы. Не меняйте рабочий пароль существующего тома ради native-запуска.

Для режима без ключей явно оставьте `GENA_NLU_MODE=semantic_plan_legacy`, `GENA_SEMANTIC_PROVIDER=rules`, `NLP_PROVIDER=rules`, пустые `GENA_STT_PROVIDER`/`GENA_TTS_PROVIDER`.

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go run ./cmd/server
```

Сервер читает `.env` из корня; переменные процесса имеют приоритет, включая пустые значения. Миграции применяются при старте. Для hot reload в серверном окружении задайте:

```dotenv
APP_ADDR=127.0.0.1:8082
PUBLIC_URL=http://127.0.0.1:5173
YANDEX_REDIRECT_URI=http://127.0.0.1:5173/api/auth/yandex/callback
```

В первом терминале запустите `go run ./cmd/server`, во втором — `pnpm --dir web dev`. Vite проксирует `/api` и `/health` к 8082. При смене origin обновите OAuth callback и его регистрацию у провайдера. AI настраивается через [GigaChat](GIGACHAT_CONFIGURATION.md).

## Go и PostgreSQL

```bash
go mod verify
go vet ./...
go test -race -count=1 ./...
```

`TEST_DATABASE_URL` должен указывать на отдельную тестовую PostgreSQL. Тесты создают/удаляют собственные схемы; без этой переменной PostgreSQL-проверки пропускаются. Не используйте рабочую базу. Race detector требует CGO и C-компилятор; штатно запускается в Linux CI. SQLite и legacy-корпуса сохранены для импорта и регрессионных тестов; рабочее хранилище — PostgreSQL.

## Frontend и браузер

```bash
pnpm --dir web test --maxWorkers=2
pnpm --dir web build
pnpm --dir web exec playwright install chromium
pnpm --dir web e2e
```

E2E с `TEST_DATABASE_URL` запускает Go на `127.0.0.1:8083` с rules provider и тестовыми аккаунтами. С `P1_BASE_URL` использует готовую установку; направляйте такие проверки только на тестовый сервис. HTML-отчёт — `web/playwright-report/`, следы ошибок — `web/test-results/`.

Браузерные mocks проверяют работу записи/черновика и ручную отправку, но не физический микрофон, качество внешнего STT, согласие OAuth или доставку push на телефон.

## Скрипты и выпуск

```bash
python -m unittest discover -s scripts -p 'test_*.py' -v
python scripts/check_docs.py
```

Linux-проверка receiver, использующего `fcntl`:

```bash
docker run --rm -v "$PWD/deploy:/work" -w /work python:3.12-slim python -m unittest discover -p 'test_*.py' -v
```

На PowerShell замените volume на абсолютный путь, например `D:/assistantai/deploy:/work`. Тесты проверяют валидацию архива, сохранение env и откат приложения. [CI/CD](../deploy/CI_CD.md) описывает штатный выпуск.

## AI smoke

```bash
go run ./cmd/simple-smoke -mode fixture -output artifacts/simple_llm_smoke_fixture.json
```

Live/interactive запускаются явно с credentials и расходуют квоту; используют временную SQLite. [Контракт и команды](SIMPLE_LLM_RUNTIME.md). Живые contract-тесты с PostgreSQL описаны в [руководстве проверки](ASSIGNMENT_REVIEW.md).

## CI

[Workflow](../.github/workflows/ci.yml) проверяет Go/vet/race с PostgreSQL 17, Python, frontend unit/TypeScript/build и Chromium smoke. Перед выпуском `main` дополнительно проверяет runtime-образ из готовых файлов, CRUD/auth, сохранение сессии/данных после рестарта и остановку контейнера. Приложение собирается один раз.

Workflow запускается для PR в `main` и push `main` при изменении приложения/инфраструктуры. Push рабочей ветки отдельно не дублирует проверки; чистые Markdown-изменения пропускаются. Устаревшие PR runs отменяются, выпуск `main` выполняется последовательно. Артефакты хранятся 3 дня. Полную браузерную регрессию можно запросить через **Actions → CI/CD → Run workflow → full_browser**. Push `main` после проверок выпускается автоматически.

## Документация и изображения

[Скриншоты](SCREENSHOTS.md) снимаются `web/scripts/capture-readme.mjs` с реальной локальной установки и синтетическими данными. Скрипт запрещает удалённый адрес без явного флага. Диаграммы регенерируются из `.mmd` командой `python scripts/render_diagrams.py`; нужен Node.js 24/npm. Готовые SVG не требуют Mermaid/CDN при просмотре.
