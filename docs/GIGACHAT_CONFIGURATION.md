# Настройка GigaChat

[← Документация](README.md)

GigaChat используется для разбора текста в `simple_llm`. Голос предварительно превращается в текст отдельным провайдером Groq. [Контракт LLM](SIMPLE_LLM_RUNTIME.md) · [Голос](DEADLINES_AND_VOICE.md).

## Сервер вне Docker

В локальном `.env` задайте:

```dotenv
GENA_NLU_MODE=simple_llm
GENA_SEMANTIC_PROVIDER=gigachat
GIGACHAT_AUTHORIZATION_KEY=<ключ авторизации>
GIGACHAT_SCOPE=GIGACHAT_API_PERS
GIGACHAT_MODEL=GigaChat-3-Ultra
GIGACHAT_TIMEOUT=30s
GIGACHAT_MAX_CONCURRENT=1
```

`GIGACHAT_AUTHORIZATION_KEY` — credential приложения; runtime не использует отдельные `GIGACHAT_CLIENT_ID` и `GIGACHAT_CLIENT_SECRET`. Access token получает backend и хранит только в памяти.

Если доверенных системных сертификатов недостаточно, укажите `GIGACHAT_CA_FILE` — абсолютный путь к проверенному CA bundle на машине, где запущен Go. TLS-проверка остаётся включённой. Файл сертификатов и ключи не добавляйте в Git.

## Docker Compose

Базовый `compose.yaml` не передаёт GigaChat environment в контейнер. Используйте [override](../deploy/compose.gigachat.example.yaml) вместе с базовым файлом:

1. Добавьте AI-переменные выше в `.env`.
2. Задайте `GIGACHAT_CA_FILE_HOST` — абсолютный путь к проверенному CA bundle на Docker-хосте. На Windows используйте путь с прямыми слешами, например `D:/certs/gigachat-ca.pem`.
3. Выполните:

```bash
docker compose -f compose.yaml -f deploy/compose.gigachat.example.yaml config --quiet
docker compose -f compose.yaml -f deploy/compose.gigachat.example.yaml up --build -d --wait
```

Override монтирует bundle read-only как `/etc/gigachat/ca.pem` и задаёт контейнерный `GIGACHAT_CA_FILE`. Команды управления этим запуском выполняйте с той же парой `-f`.

Для production используется `deploy/compose.production.yaml` вместо локального `compose.yaml`; оба production-файла уже указаны в `deploy/rhythm.service`. [Порядок обновления](../deploy/README.md).

## Поведение при ошибках

Клиент переиспользует HTTP transport, обновляет токен и ограничивает число одновременных запросов. После 401 инвалидирует токен и повторяет запрос один раз; временные ошибки имеют ограниченные повторы. Timeout, отмена и некорректный ответ не должны приводить к мутации пользовательских данных.

LLM получает текст и компактный контекст. Go отклоняет неизвестные JSON-поля и разрешает ссылки только на существующие записи пользователя. Точные подтверждения/отмена при pending обрабатываются без inference; остальные реплики в AI-режиме идут в модель.

Concurrency ограничена на процесс. Для нескольких replicas с общей квотой провайдера нужна общая очередь либо один inference worker.

## Проверки

Обычные Go-тесты не требуют обращения к GigaChat. Проверка исполнительного пути с фиксированными ответами:

```bash
go run ./cmd/simple-smoke -mode fixture -output artifacts/simple_llm_smoke_fixture.json
```

Явная проверка реального провайдера, с использованием его квоты:

```bash
go run ./cmd/simple-smoke -mode live -output artifacts/simple_llm_smoke_live.json
go run ./cmd/simple-smoke -interactive -output artifacts/simple_llm_manual.json
```

Smoke работает во временной SQLite-базе, не в PostgreSQL приложения. Команды запускайте последовательно. `cmd/semantic-eval` и корпуса `testdata/semantic` сохранены для регрессий legacy-контракта; их оценки не являются оценками `simple_llm`.

## Режим правил

Для запуска без LLM задайте `GENA_NLU_MODE=semantic_plan_legacy`, `GENA_SEMANTIC_PROVIDER=rules`, `NLP_PROVIDER=rules` и используйте базовый Compose без GigaChat override. Это выбор другого runtime, а не автоматический fallback при недоступности GigaChat.
