# Ритм: миграция на Go с приоритетами P0–P4

Обновлено 13 октября 2026 по принятым поправкам. Приложение остаётся personal assistant. Workspace/team/auth/permissions и обязательный assignee не вводятся.

## Приоритеты

| Этап | Результат | Условие начала |
| --- | --- | --- |
| P0 | Go foundation, storage migration, parity существующего backend и contract tests | текущий этап |
| P1 | Новый responsive UI по эскизам и исправление voice UX | Go прошёл общий corpus и race tests |
| P2 | Рабочий live CalDAV adapter Яндекс Календаря | подтверждённый Go parity; отдельно настроенные credentials |
| P3 | Draggable widgets, анимация и сохранение layout | основные страницы и voice UX работают |
| P4 | Расширенные возможности | после P0–P3, по отдельному приоритету |

Telegram, Zoom, Notion, Slack не получают backend на этих этапах. Если карточки присутствуют в UI, они показывают «Скоро». Google ограничивается CalendarProvider interface/dev adapter. Яндекс в P2 должен создавать реальное событие во внешнем календаре, а не ограничиваться dev-provider.

## Изученная reference implementation

Baseline: `1b7fcd6`. Сохранены существующие `app/`, `tests/*.py`, `tests/voice.test.js`, Python requirements, Dockerfile и Compose. Python не перемещается в `reference/python` в P0. Старый код и тесты продолжают запускаться на прежних местах.

Из Python переносятся создание task/reminder/meeting, список по периоду, архивный поиск, изменение/перенос, удаление с подтверждением, завершение, русские относительные даты, уточнения, last_ids, конфликты интервалов, CSV, persistence, history, due и health. Сохраняются идемпотентность запросов, TTL pending 30 минут и атомарная запись event + session state + exchange.

До нового frontend используется общий corpus с фиксированными часами. Его экспортирует существующий Python; Go читает те же JSON fixtures. Отдельно проверяются ошибки ввода, перезапуск, конкурентные операции и миграция синтетической schema v1. Рабочая Python-база в процессе разработки не мигрируется.

## Архитектура P0

```text
cmd/server/main.go
internal/
  api/                 # JSON DTO, decoding, HTTP routing/middleware
    dto/
  assistant/           # use case, уточняющий диалог, context/idempotency
  calendar/            # общие операции событий, интервалы, due, CSV
  domain/              # typed enums, Event/Task/Meeting/Reminder, Command
  nlp/                 # Unicode deterministic parser, confidence, hybrid
  llm/                 # HTTP Ollama → validated Command
  search/              # словоформы, синонимы, fuzzy/ranking
    data/
  repository/          # interfaces и transaction scope
  storage/sqlite/      # database/sql, queries, migrations, backup
  config/
  clock/
migrations/
testdata/parity/
tests/contract/
scripts/export_parity.py
Dockerfile.golang
compose.go.yaml
```

Поток: handlers → services/use cases → domain → repository → SQLite. В handlers нет логики диалога, вычисления due, конфликта или импорта. Core enums Action/EventType/Status/Priority — отдельные uint8/iota types; строковое представление используется на JSON/SQL boundaries. Даты без времени не превращаются в полуночные встречи. Длительность имеет тип time.Duration; интервал вычисляется с учётом часового пояса и перехода через полночь.

HTTP: net/http + chi, context.Context, log/slog JSON, request id, body limit, timeout, recover, security headers, graceful shutdown с отменой active request contexts. Дополнительные поля description/priority/external_calendar_id расширяют существующий event DTO, сохраняя kind/date/time/duration_minutes.

## SQLite

- WAL, busy_timeout=5000, foreign_keys=ON на каждом соединении.
- Ограниченный connection pool; read операции не открывают write transaction.
- Короткий write scope: BEGIN IMMEDIATE на отдельном соединении; context-aware очередь; commit/rollback. Ошибка или cancellation не оставляют частичных изменений.
- LLM и внешние calendar HTTP requests выполняются вне SQLite write transaction. Это проверяется тестом с заблокированным HTTP-ответом Ollama и параллельной записью в SQLite.
- Перед миграцией существующей базы — согласованный SQLite backup с committed WAL. Миграции нумеруются, выполняются атомарно и не повторяются при перезапуске.
- ID, source_key, history и pending сохраняются. Неизвестная будущая schema version отклоняется.
- Python и Go для сравнения используют разные базы/тома. Старый сервер не запускается на уже мигрированной базе.

## Hybrid NLP и поиск

Порядок: deterministic parser → confidence → при низкой уверенности LLM fallback → validation → business logic. Явная дата сама по себе не повышает уверенность неизвестного намерения. Недостающие поля однозначной команды и короткие ответы на уточнения не отправляются в LLM. Ollama поддерживается через HTTP; бизнес-логика зависит от Interpreter interface. Другие модели можно добавить отдельными адаптерами.

Русские команды используют Unicode-aware boundaries вместо ASCII `\b`. Поиск использует реальный словарь лемм и склонений, экспортированный из pymorphy3/OpenCorpora для corpus/CSV; неизвестные слова обрабатывает русский Unicode Snowball stemmer. Проверяются падежи, имена, синонимы, опечатки, title/description и порядок релевантности. Полный эквивалент морфологии для любого русского слова не заявляется; расширение нормализатора/embeddings относится к P4.

## P1: UI и голос

Начинать React только после общего corpus и `go test -race ./...`. Восемь страниц: Сегодня, Календарь, Задачи, Встречи, Напоминания, Архив, Интеграции, Настройки. Общий стиль по изображениям; даты и показатели поступают из Go API. Assignee, команды и права доступа не требуются.

Микрофон справа внутри input перед Send. STT дополняет редактируемый черновик; пауза/onend ничего не отправляют. Пользователь может продолжить говорить. Отправка — по Send/Enter или отдельному явному подтверждению. Старые голосовые тесты остаются reference прежнего поведения, новый adapter получает проверки ручной отправки, продолжения и ошибок.

## P2: реальный Яндекс Календарь

CalendarProvider задаёт CreateEvent, UpdateEvent, DeleteEvent, ListEvents с context и типизированными данными. Для Яндекса реализуется live CalDAV adapter. После настройки логина, пароля приложения Calendar и выбранного календаря событие, созданное в Ритм, должно появляться в Яндекс Календаре. Внешние UID/ETag, retry и состояние синхронизации сохраняются; сетевой запрос не удерживает SQLite writer. Реальная проверка выполняется на разрешённом аккаунте. Google пока interface/dev adapter.

## P3–P4

P3: скрытие/добавление/удаление виджетов, сортировка drag & drop с анимацией и сохранение layout. P4: подзадачи, метки, повторение, дополнительные интеграции и расширенный поиск — только после основных этапов. Эти функции не включены в текущую P0-реализацию.

## Критерии выхода из P0

1. Go проходит общий corpus исходных сценариев: даты, команды, HTTP/dialogue и поиск по 1000 строкам.
2. Schema v1 мигрируется без потери ID/history/pending; повторный импорт не создаёт дубли.
3. Go unit/integration/contract tests, `go vet` и **`go test -race ./...`** проходят.
4. Все 73 старых Python-теста и 6 старых голосовых тестов остаются и проходят.
5. Изолированный Go Docker runtime запускается, отвечает по HTTP и сохраняет записи после restart.
6. Python-структура и рабочая база сохранены; новый React и live-интеграции не начаты.

Подробные команды и границы этапа: [GO_PARITY.md](GO_PARITY.md). Актуальный P0 API и будущие расширения: [API_CONTRACT_NEXT.md](API_CONTRACT_NEXT.md).
