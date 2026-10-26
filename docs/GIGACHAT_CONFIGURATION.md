# Настройка GigaChat

[Документация](README.md) · [Быстрый запуск](QUICKSTART.md) · [Контракт модели](SIMPLE_LLM_RUNTIME.md)

GigaChat — единственный внешний провайдер текста. Groq STT и Azure TTS настраиваются отдельно; для текстового ассистента они не обязательны.

## Запуск в Docker одной командой

Создайте `.env` из `.env.example` или сначала запустите обычный demo. Заполните `GIGACHAT_AUTHORIZATION_KEY` и при необходимости `GIGACHAT_CA_FILE_HOST`, затем:

```bash
python scripts/service.py up --mode full
```

Launcher сохраняет существующие секреты/данные, выбирает `simple_llm` и GigaChat, собирает приложение, запускает PostgreSQL и проверяет health. Базовый Compose уже передаёт GigaChat-переменные. Действительность credentials проверяется реальным запросом ассистенту, а не только health сервера.

Для доверенного CA bundle задайте абсолютный путь на Docker-хосте, например `D:/certs/gigachat-ca.pem`. Launcher создаёт override и монтирует файл read-only как `/etc/gigachat/ca.pem`. TLS-проверка остаётся включённой. Файл CA и ключи не добавляйте в Git.

## Сервер вне Docker

```dotenv
GENA_NLU_MODE=simple_llm
GENA_SEMANTIC_PROVIDER=gigachat
GIGACHAT_AUTHORIZATION_KEY=<ключ авторизации>
GIGACHAT_SCOPE=GIGACHAT_API_PERS
GIGACHAT_MODEL=GigaChat-3-Ultra
GIGACHAT_TIMEOUT=30s
GIGACHAT_MAX_CONCURRENT=1
```

Для native Go `GIGACHAT_CA_FILE` — путь на той машине, где работает процесс. `GIGACHAT_AUTHORIZATION_KEY` — credential приложения; отдельные client ID/secret runtime не использует. Полученный access token остаётся в памяти backend.

Ручная альтернатива launcher: [compose.gigachat.example.yaml](../deploy/compose.gigachat.example.yaml) вместе с локальным `compose.yaml`. Override требует `GIGACHAT_CA_FILE_HOST`; все команды управления выполняйте с той же парой `-f`. Production использует отдельные [файлы и процедуру](../deploy/README.md).

## Ошибки и контракт

Клиент переиспользует transport, обновляет токен и ограничивает concurrency на процесс. После 401 инвалидирует token и повторяет запрос один раз; временные ошибки имеют ограниченные повторы. Для нескольких replicas с общей квотой нужен общий inference worker/очередь.

Модель получает текст и компактный контекст без DB IDs. Go отклоняет неизвестные JSON-поля, выбирает только записи текущего пользователя и проверяет даты/конфликты. Timeout, отмена и некорректный ответ не должны изменять дела. Точные подтверждения/отмена pending обрабатываются без inference, остальные реплики в AI-режиме идут в модель.

## Проверки

Без платной модели:

```bash
go run ./cmd/simple-smoke -mode fixture -output artifacts/simple_llm_smoke_fixture.json
```

Явный живой запуск с расходованием квоты:

```bash
go run ./cmd/simple-smoke -mode live -output artifacts/simple_llm_smoke_live.json
go run ./cmd/simple-smoke -interactive -output artifacts/simple_llm_manual.json
```

Запускайте команды последовательно. Smoke использует временную SQLite; PostgreSQL live-проверка — в [ASSIGNMENT_REVIEW](ASSIGNMENT_REVIEW.md). `cmd/semantic-eval` оценивает прежний semantic-контракт и не заменяет оценку `simple_llm`.

Для независимой демонстрации без внешних ключей используйте отдельный env с `GENA_NLU_MODE=semantic_plan_legacy`, `GENA_SEMANTIC_PROVIDER=rules`, `NLP_PROVIDER=rules`. Demo проверяет правила/интерфейс и не обещает качество свободной речи.
