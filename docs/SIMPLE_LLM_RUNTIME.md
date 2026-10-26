# Simple LLM runtime Гены

[← Документация](README.md)

`text/voice → compact context → GigaChat-3-Ultra → operation / multi_action → Go validation/calendar/target resolution → assistant.Service → repository`.

Несколько команд, зависимости, отдельные pending и продолжения поддерживаются контрактом [Multi-Action](MULTI_ACTION.md). Одиночный envelope сохранён. Неоднозначные «в 8» требуют уточнения части суток.

Полный режим использует `GENA_NLU_MODE=simple_llm`, `GENA_SEMANTIC_PROVIDER=gigachat` и `GIGACHAT_AUTHORIZATION_KEY`. GigaChat — единственный внешний текстовый провайдер. При необходимости задаётся проверенный CA bundle. [Запуск одной командой](QUICKSTART.md).

`.env.example`, базовый Compose и launcher по умолчанию задают `semantic_plan_legacy`/`rules`: это демонстрация без внешних ключей. `python scripts/service.py up --mode full` включает `simple_llm`/GigaChat с сохранёнными credentials; модель по умолчанию — `GigaChat-3-Ultra`, concurrency 1. В native config без env development выбирает `simple_llm`, production — legacy; поэтому задавайте режим явно. [Настройка GigaChat](GIGACHAT_CONFIGURATION.md).

Смена только `GENA_NLU_MODE` на legacy сохраняет выбранный provider и включает прежний planner. Для режима без внешних моделей нужны также `GENA_SEMANTIC_PROVIDER=rules` и базовый Compose без GigaChat override. [Обновление и откат](../deploy/README.md).

## Граница модели и Go

Модель понимает фразу, отрицания, corrections, refinements, references и фильтры. Go принимает строгий JSON и проверяет конкретный тип аргументов; неизвестные поля отклоняются рекурсивно. Модель не получает DB IDs и не может прислать `id`/`target_id` вместо ссылки.

Новый путь не вызывает `SemanticPlan`, canonicalizer, evidence, source/reverse coverage, lexical provenance или legacy Russian command parser. В GigaChat отправляются system prompt и один JSON с текущим текстом и компактным контекстом. Ответ строго декодируется в Go. Для невалидного ответа либо ошибки Go-валидации допускается один ограниченный repair до commit. После неудачного repair запрашивается уточнение без изменения данных; обычный ответ после исполнения формирует Go.

Общий исполнитель выделен в `executeValidated`: он получает Go-команду без исходного текста пользователя. Legacy вход `execute` по-прежнему обслуживает старые тесты и явно выбранный legacy mode.

## Operation envelope

```json
{
  "intent": "update",
  "arguments": {
    "target": {"type": "ordinal", "index": 2},
    "changes": {"time": "16:00"}
  },
  "confidence": "high",
  "clarification": null
}
```

| Intent | Аргументы |
| --- | --- |
| `create_task`, `create_meeting`, `create_reminder` | `title`, опциональные `date`, `time`, `duration_minutes`, `reminder_minutes`, `reminder_enabled`, `recurrence` |
| `list`, `search` | `kind`, `strict_kind`, `query`, `date` либо `period`, `status`, `inherit` |
| `update` | `target`, `changes` |
| `complete`, `delete` | `target` |
| `batch_update` | `filter`, `exclude[]`, `changes` |
| `batch_complete`, `batch_delete` | `filter`, `exclude[]` |
| `continue` | `changes` для ожидаемой команды |
| `select` | `target` с порядковым номером pending-кандидата |
| `confirm`, `cancel` | `{}` |
| `chat` | `reply` |
| `clarify` | `{}`, вопрос в `clarification` |

`target`: `last`; `ordinal + index` (с единицы); `query + value + optional filter`. Go выбирает только существующие записи текущего пользователя. Порядковый выбор из pending работает и при повторе исходной операции моделью.

Названная пользователем тема имеет приоритет над `last_affected`. Консервативная проверка в Go может только запретить контекстную мутацию, если сообщение упоминает другую существующую запись; она не выбирает новую цель. Для read-only поиска `strict_kind:true` сохраняет явное ограничение «только задачи». При пустом нестрогом поиске задач разрешён отдельный поиск других типов с пояснением в ответе; остальные фильтры сохраняются. Облачный поиск такого расширения готовится вне транзакции только после пустого исходного поиска.

`changes.recurrence` поддерживает полное правило или только `until`/`count` в продолжении сохранённого правила. Go объединяет границу с исходными частотой и днями недели; без исходной серии частичный объект отклоняется. `until:{type:"period_end",unit:"year",offset:0}` означает последний день текущего года.

`changes`: `title`, `date`, `time`, `duration_minutes`, `shift_minutes`, `reminder_minutes`, `reminder_enabled`, `status`. Длительность — целое число от 5 до 1440 минут; она поддерживается при продолжении создания, изменении существующей записи и массовом изменении с подтверждением. Ответ на уточнение «В 4-15 дня на полчаса» нормализуется в `time:"16:15",duration_minutes:30`: исходная дата, участники и настройки напоминания сохраняются. Форматы распознанного времени с точкой, дефисом и пробелом интерпретирует LLM в контексте вопроса о времени; исполнитель принимает строку `HH:MM`. Сдвиг нельзя совмещать с новым временем/датой. `filter`/`exclude`: `kind`, `query`, `date` либо `period`, `status`. `inherit=true` объединяет явно изменённые фильтры с последним успешным поиском; исходная тема и календарные границы сохраняются.

## Время

| Смысл | Нормализованный аргумент |
| --- | --- |
| Завтра / послезавтра | `{"type":"relative_day","offset":1}` / offset 2 |
| Через полчаса | `{"type":"relative_delay","unit":"minute","value":30}` |
| Прошлый месяц | `{"type":"calendar_period","unit":"month","offset":-1}` |
| Следующая пятница | `{"type":"weekday","weekday":"friday","direction":"next"}` |
| Явная дата | `{"type":"absolute","date":"2026-10-18"}` |
| До двадцатого / к 20 числу | `{"type":"day_of_month","day":20}`; ближайший существующий день вычисляет Go |
| Последние N дней | `{"type":"rolling_period","unit":"day","value":7}` |
| Готовые границы сохранённого фильтра | `{"type":"date_range","date":"2026-09-01","end_date":"2026-10-01"}` |

`calendar_period`: day/week/month/year. Задержка: second/minute/hour/day. Go вычисляет календарные даты в timezone пользователя; границы поиска полуоткрытые. Задержки сохраняют секунды. Конкретное событие требует дня, а не неопределённого месяца/периода.

Пропущенные обязательные поля сохраняются как pending, без мутации. Для task обязательно название; для meeting/reminder — ещё дата и время. Маленькая проверка очевидных `завтра`, `послезавтра`, `HH:MM` не позволяет молча создать task без явно названного, но пропущенного моделью срока. Повторения передаются структурным полем recurrence; невыразимые запросы модель должна уточнять.

## Compact context и state

Модель получает `now`, `timezone`, `last_action`, до восьми `last_results` с порядковыми номерами, `last_affected` без порядкового номера, `last_search`, `pending` с ожидаемым полем/подтверждением и кандидатами. Для pending batch передаются фильтр, исключения и изменения. Полная история и DB IDs не передаются.

В state сохраняются существующие IDs результатов, последняя успешная операция/поиск, последняя изменённая запись и pending. Тексты предыдущих реплик и derived NLP metadata новый путь не сохраняет. Результаты и ссылки действуют 15 минут, pending — 30 минут. После изменения записи список предыдущего поиска сохраняет свою нумерацию.

Fast path — только точные `да`, `подтверждаю`, `удаляй` при pending confirmation и `нет`, `отмена`, `не надо` при pending. Опциональный slot fast path намеренно не добавлен: даже `завтра` и `в 15:00` модель понимает с контекстом. Остальная речь сразу идёт в GigaChat.

## Safety и batch

Сохранены auth middleware, user-scoped repository, блокировки сессии, request idempotency, транзакции/rollback и проверки конфликтов. Inference идёт вне write-транзакции. Перед выполнением Go сверяет snapshot диалога: устаревший ответ модели не применяется к новому контексту.

Неоднозначная одиночная цель вызывает выбор. Любое удаление требует отдельного подтверждения. Все batch, включая complete/update, сначала показывают preview; до `confirm` записи не меняются. Исключения применяются в Go. Подтверждение использует конкретный snapshot списка, проверяет изменения/удаление записей и конфликты. Новые записи не присоединяются к старому preview. Ошибка в середине batch откатывает все записи и состояние подтверждения.

Одиночное подтверждение конфликта дополнительно проверяет snapshot самой изменяемой записи и показанные пересечения. Ошибка интерпретации нового запроса сбрасывает старое pending confirmation. `confidence=low` не выполняет действие.

## Локальные проверки

```powershell
go test -count=1 ./...
go vet ./...
go run ./cmd/simple-smoke -mode fixture -output artifacts/simple_llm_smoke_fixture.json
go run ./cmd/simple-smoke -mode live -output artifacts/simple_llm_smoke_live.json
go run ./cmd/simple-smoke -interactive -output artifacts/simple_llm_manual.json
```

По умолчанию `simple-smoke` использует временные пустые SQLite-базы. `-postgres` создаёт отдельную временную схему и пользователя в локальном `TEST_DATABASE_URL`, имя БД должно содержать `semantic_test`; схема удаляется после прогона. Этот режим проверяет production-путь совместного планирования. `DATABASE_URL`/`DATABASE_PATH` приложения не используются. Live mode читает конфигурацию GigaChat. Интерактивный режим начинает пустую сессию, сохраняет transcript; `/quit` завершает диалог.

В runtime добавлены bounded repair, откатываемый preflight, повторы, атомарные группы, управление шагами и внешние статусы. Пределы, конфигурация и результаты fixture/live проверок описаны в [TOP-10](TOP10_IMPLEMENTATION.md).

Корпус `testdata/simple_llm/smoke.json` содержит небольшие диалоговые сценарии; `deadlines.json` проверяет формулировки дедлайнов. Fixture mode проверяет исполнитель, live mode — реальную модель и исполнитель вместе. Старые semantic-корпуса и их отчёты относятся к legacy-контракту и не измеряют качество этого runtime.
