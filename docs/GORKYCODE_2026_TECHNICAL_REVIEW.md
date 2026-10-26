# Гена и 11 потенциальных конкурентов GORKYCODE 2026

Дата проверки: **26 октября 2026**, Europe/Moscow. Это технический аудит зафиксированных исходников, а не результаты жюри или общий live-бенчмарк всех продуктов.

Ниже сохранён срез до следующего этапа реализации. Актуальные изменения Гены по всем десяти направлениям, результаты и оставшиеся ограничения опубликованы в [TOP-10](TOP10_IMPLEMENTATION.md). Рейтинг этого аудита не пересчитывался без общего бенчмарка конкурентов.

## Главный вывод

Наиболее серьёзный инженерный конкурент — **xm1k / «Голос v2»**. Его преимущество подтверждается не Kubernetes, а подключённым циклом LLM → query → проверка → исправление, транзакционным применением пакета, версиями записей, outbox и защитой заданий от повторной доставки. У Гены сильнее подтверждённый в этом аудите пользовательский контур: личные и совместные встречи, неоднозначные друзья, приглашения, Google/Яндекс, Zoom и уведомления. Без одинакового live-корпуса нельзя доказательно назвать один проект безусловно лучшим по пониманию русского языка.

В исходной Гене `simple_llm` действительно терял дополнительные команды: декодер, интерфейс интерпретатора и `respondSimple` обслуживали одну операцию. В этой рабочей копии добавлены контракт плана, отдельное состояние шагов, зависимости, продолжения, подтверждения и тесты. **Первый дефект исправлен на уровнях NLU, state и executor**, а не только в промпте. Подробности — [MULTI_ACTION.md](MULTI_ACTION.md). Изменения пока локальные; публикация сервера в рамках этого задания не выполнялась.

## Метод и ограничения доказательств

- **L** — реально запущен указанный сценарий с настоящим внешним LLM. Для Гены — GigaChat и отдельная PostgreSQL; операции выполнялись только в тестовых аккаунтах.
- **T** — тесты запущены в этом аудите. Mock провайдера не доказывает качество самого LLM, микрофона или внешнего календаря.
- **K** — прослежен исходный код и подключение к вызывающему пути; запуск соответствующего внешнего сервиса не проверялся.
- **D** — только документация/контракт. **S** — явная заглушка или неподключённый компонент.
- «Не подтверждено» не означает «невозможно» или «отсутствует во всех ветках». Проверены default-ветки на указанных SHA; чужие приватные ветки, развёртывания и последующие коммиты не оценивались.

Оценки 0–5 — экспертная оценка проверенного пути, не проценты точности: 0 — исполнимого пути в рассмотренном контракте нет/заглушка; 1 — существенная неполнота; 2 — ограниченная реализация; 3 — рабочая базовая реализация с оговорками; 4 — развитая реализация с проверками; 5 — особенно сильные механизмы надёжности. Пять баллов не означает отсутствие ошибок. Оценка учитывает подключение, а не число файлов. У каждой строки есть раздел с первичными ссылками и оговорками.

Нагрузочных измерений на одинаковом оборудовании, живого STT/TTS всех кандидатов и доступа к реальным календарям конкурентов нет. Внешние интеграции нельзя объявлять работающими end-to-end только по unit-тестам. В исходном PDF четыре обязательных действия: создание задачи/напоминания, встреча, список за период, поиск истории с сохранением данных; голос → STT → действие → ответ входит в минимальный сценарий. Multi-Action, повторы, социальное планирование и внешний календарь — усиления продукта, а не замена обязательных четырёх действий.

Источник ТЗ: [PDF в репозитории ArtemST2006](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/Testovoe_zadanie_golosovoj_assistent_dlya_zadach_i_vstrech_4d9cb6145e.pdf). Проверены все пять страниц. Документ не содержит списка участников и сам по себе не подтверждает допуск конкретного автора.

## Сравнение по одинаковым критериям

«База» — полнота четырёх обязательных действий через ассистента, а не прямой CRUD API. «Контекст» включает уточнения и исправления. Ссылки в первом столбце ведут к объяснениям оценок с кодом.

| Проект | База | Русский NLU | Даты/время | Контекст | Multi-Action | Поиск истории |
|---|---:|---:|---:|---:|---:|---:|
| [Гена после исправления](#гена) | 4 L/T | 4 L/T | 4 L/T | 4 L/T | 4 L/T | 3 T/K |
| [xm1k](#xm1k) | 4 T/K | 4 K | 4 T/K | 4 T/K | 4 T/K | 4 K |
| [TRIADA](#triada) | 4 T | 3 T/K | 3 T | 3 T | 0 K | 2 T/K |
| [Margaryan22](#margaryan22) | 3 K | 2 K | 3 K | 4 K | 0 K | 3 K |
| [ryzhilx](#ryzhilx) | 3 T | 3 K | 3 T/K | 2 T/K | 2 K | 3 T/K |
| [ArtemST2006](#artemst2006) | 3 T/K | 3 K | 3 T/K | 3 T/K | 0 K | 1 T/K |
| [Britnispirs](#britnispirs) | 3 T | 3 K | 3 T/K | 2 T/K | 0 K | 2 T/K |
| [Svoloch2940194](#svoloch2940194) | 2 K | 1 K | 1 T/K | 2 K | 0 K | 1 K |
| [Agend AI](#agend-ai) | 1 K | 2 K | 2 K | 1 K | 0 K | 1 K |
| [Lefiper](#lefiper) | 1 T/K/S | 0 S | 2 T/K | 1 T/K/S | 0 S | 1 T/K |
| [anastbezu](#anastbezu) | 1 T/K/S | 2 T/K/S | 2 K | 1 K/S | 0 K | 1 K/S |
| [Neyromark](#neyromark) | 0 K/S | 1 K | 1 K | 0 K | 0 K | 0 S |

Следующая таблица отделяет наличие кода от подтверждённой интеграции. `—` означает, что в прослеженном пользовательском пути реализация не подтверждена; это не оценка скрытых веток. Голос оценивается по соединению компонентов, без сравнения качества распознавания реальной речи.

| Проект | Голос STT/TTS | Реальный внешний календарь | Повторы | Совместные встречи | Доставка уведомлений | Надёжность/безопасность | Транзакции/идемпотентность | UX/готовность |
|---|---|---|---|---|---|---:|---|---:|
| Гена | 4 K/T | Google, Яндекс K/T | K/T, API/legacy; пробел simple NLU | Друзья, приглашения, слоты K/T | Web Push, durable delivery K/T | 4 T | Транзакция реплики, request cache, snapshots T | 4 K, build T |
| xm1k | 3 K | Яндекс CalDAV K/T, live пропущен | K/T | Конфликты личного календаря; социальный контур не подтверждён | Core/очереди K/T | 5 T/K | Пакет, версии, claim/generation, outbox T/K | 3 K |
| TRIADA | 3 K | — | — | Конфликты; приглашения не подтверждены | Запись reminder подтверждена, реальная доставка — нет | 3 T/K | SQLAlchemy, durable dialogue; request dedupe не найден в chat path | 3 K |
| Margaryan22 | 2 K, браузер | — | — | Конфликты; приглашения не подтверждены | Доставка не подтверждена | 3 K | Транзакция turn; dedupe CSV не равен dedupe chat | 3 K |
| ryzhilx | 3 K | — | Задачи и встречи K; уточнено при повторном аудите | Предупреждение о конфликте K/T | Telegram reminder loop T/K | 2 T/K | Отдельные commit инструментов; общей атомарности/ключа запроса не найдено | 3 K |
| ArtemST2006 | 2 K, STT | — | — | Создание meeting; приглашения не подтверждены | Доставка не подтверждена | 3 T/K | Redis draft + PostgreSQL create; атомарный пакет отсутствует в контракте | 2 K |
| Britnispirs | 2 K, браузер | Только ICS export K; это не sync | — | attendees и предупреждение K | Доставка не подтверждена | 2 T/K | SQLite; SESSION_CONTEXT в памяти, общего request cache нет в path | 3 K |
| Svoloch2940194 | 2 K | — | — | Только тип event | Доставка не подтверждена | 1 K | SQLite CRUD; FSM MemoryStorage | 2 K |
| Agend AI | 2 K, браузер | — | — | Только item meeting | Доставка не подтверждена | 2 K | Spring transaction; LLM вызван внутри неё, все данные DefaultUser | 3 K |
| Lefiper | 2 T/K, transport | — | — | Отдельные events; приглашения не подтверждены | MAX transport имеет durable reminders T; агент не подключён | 2 T/K/S | UNIQUE(user,key), Redis CAS T/K; общей транзакции Redis/PG нет | 1 K/S |
| anastbezu | Код frontend K | — | — | В схемах; полный путь не подключён | Доставка не подтверждена | 2 T/K/S | PG services есть; LLM actions не сохраняют запись | 2 K/S |
| Neyromark | Vosk → текст VK K | — | — | Не подтверждены | Notifications repo содержит только README S | 1 K | Разрозненные модули; полного request path нет | 1 K/S |

## Технический рейтинг

Порядок ориентирован на работоспособность обязательных сценариев, затем на устойчивость и развитие. Разница между соседними местами без общего live-корпуса условна; это приоритет внимания, не прогноз решения жюри.

| Место | Проект | Почему |
|---:|---|---|
| 1 | xm1k | Наиболее сильная проверенная инфраструктура команд: preview тем же executor, финальная повторная проверка, rollback всего пакета, защищённые claim и доставка. Полный проверенный локальный набор: 137 passed / 8 skipped. |
| 2 | Гена после исправления | Сильный законченный пользовательский контур и теперь полноценный durable Multi-Action; три реальных LLM-сценария с PostgreSQL проверены, включая пакет из пяти действий. Отстаёт по циклу исправления невалидного LLM-ответа и общему neural retrieval. По совместным встречам и широте подтверждённых интеграций — лидер этого обзора. |
| 3 | TRIADA | Связанный одиночный runtime, строгая схема, persistent dialogue и 91 прошедший тест. Дополнительные сложные механизмы не заменяют отсутствие Multi-Action. |
| 4 | Margaryan22 | Чистый FSM/effects/transaction подход и осмысленный поиск. Свобода языка ограничена rule parser; собственный test suite в аудите не запущен. |
| 5 | ryzhilx | Реальный список tool calls, Telegram и уведомления; 31 выбранный тест прошёл. Хуже защищён от частичных сбоев, потери памяти и удаления без preview. |
| 6 | ArtemST2006 | Хорошие границы Go/Python, Redis draft и строгий API; подключённый поиск всё ещё ILIKE, контракт одиночный. Часть Go-проверок прошла; полный Go service блокируется отсутствующим C compiler в среде аудита. |
| 7 | Britnispirs | Рабочая база, понятные уточнения и FTS5; 9 тестов прошли. Volatile context, общий DB-путь и ограниченный «семантический» fallback. |
| 8 | Svoloch2940194 | Соединённый Telegram/FSM/SQLite путь ценнее набора неподключённых сервисов. Но воспроизводимая ошибка «в 8 вечера» → 08:00 и узкий NLU снижают оценку. |
| 9 | Agend AI | Создание через NLU и прямые API сделаны; все остальные intent в dispatcher отвергаются. Хорошая форма Spring/Testcontainers/WireMock не компенсирует неполную бизнес-логику. |
| 10 | Lefiper | Качественные изолированные storage/transport компоненты. Связанный LLM agent и HTTP API в snapshot не реализованы, поэтому прототип целиком не подтверждён. |
| 11 | anastbezu | БД и сервисы реальны, но публичный chat использует временный rule dispatcher; LLM action handlers — TODO и возвращают несохранённые records. |
| 12 | Neyromark | STT, LLM и векторизатор существуют отдельно; поиск — pass, бот возвращает только транскрипцию, notifications — README. Четыре обязательных действия вместе не подтверждаются. |

## Доказательства по проектам

### Гена

До изменения: [контракт одной операции](../internal/nlu/operation.go), [один InterpretOperation/applySimple в respondSimple](../internal/assistant/simple.go), [LLM adapter](../internal/llm/simple.go). Существовавшие уточнения, durable state, user-scoped repository, карточки, голос и календарь не записывались в недостатки автоматически.

После изменения: [типизированный multi-контракт](../internal/nlu/multi.go), [план и результаты в domain](../internal/domain/action_plan.go), [исполнитель шагов](../internal/assistant/multi.go), [регрессии](../internal/assistant/multi_test.go), [PostgreSQL/live tests](../tests/contract/multi_action_postgres_test.go). Защита времени также в [simple_time.go](../internal/assistant/simple_time.go). Модельный `time_ambiguous` дополняется Go-guard; сложные речевые формы не объявляются полностью решёнными регулярным выражением. Отдельный тест проверяет, что поздний search по ранее созданной записи не запускает её внешнюю синхронизацию заново.

Имеющиеся сильные части: [friends/scheduling](../internal/assistant/scheduling.go), [социальный executor](../internal/social/meetings.go), [Google](../internal/integrations/cloud/google.go), [Яндекс service](../internal/integrations/yandexcalendar/service.go), [уведомления](../internal/notifications/worker.go), [поиск](../internal/search/engine.go), [голосовой UI](../web/src/features/assistant/Conversation.tsx). Яндекс интегрирован как существующий календарный контур/экспорт; whitelist `calendar` у simple create всё ещё Google, это разные уровни продукта.

Реальные ограничения: поиск Гены использует русскую нормализацию/стемминг/синонимы и ранжирование, а не обученный embedding retrieval; general semantic recall не измерен одинаковым held-out набором. `nlu.Create` не выражает recurrence, хотя повторения в domain/API/legacy уже есть. `simple_llm` по-прежнему не имеет bounded LLM regeneration по конкретным Go-ошибкам. Новый Multi-Action ограничен пятью шагами, одной активной репликой уточнения, 30 минутами и не даёт общей транзакции с внешними API. Источники шагов проверяются как подстроки, но это не доказательство полной semantic coverage всего сообщения: модель всё ещё может не выделить намерение, поэтому нужен отдельный live-корпус пропусков.

### xm1k

[Decision/Command](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/shared/backend/common/schemas.py#L134) ограничивает пакет пятью командами и различает ready/clarify/query. `clarify` требует пустой commands: готовые независимые действия не коммитятся одновременно с этой веткой. `context` — строка о намерении плюс история; это не отдельный durable pending каждого шага.

[apply_decision](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/services/core/app/backend/chat/decisions.py#L21) последовательно применяет весь пакет, проверяет конечное состояние и конфликты, сравнивает согласие с точным payload. [core validate/result](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/services/core/app/backend/chat/main.py#L690) делает dry run в откатываемом savepoint и повторяет проверку при финальном сохранении; версии, user scope, claim token/generation/lease защищают от устаревших worker результатов.

[цикл генерации и исправлений](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/services/assistant-worker/app/backend/llm/client.py#L62) имеет общий бюджет обращений, возвращает ошибки и результаты query в модель, проверяет record refs. [worker wiring](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/services/assistant-worker/app/backend/workers/assistant.py#L40) действительно связывает core, RAG, validate и публикацию результата; это не отдельная архитектурная картинка.

[гибридный retrieval](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/shared/backend/rag/main.py#L155) объединяет текстовый и embedding ranking; используются user_id, версия и модель embedding, выдаётся предупреждение при деградации. Внешний Яндекс подключён через [calendar service](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/services/yandex-calendar/app/backend/yandex_calendar/service.py) и отдельный worker; [pipeline tests](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/shared/tests_v2/test_yandex_pipeline.py) полезнее наличия CalDAV dependency. Живые credentials не использовались.

Прочитаны и сопоставлены с кодом [ARCHITECTURE](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/ARCHITECTURE.md), [LLM doc](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/docs/llm-service.md), [README.v2](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/README.v2.md) и [local tests README](https://github.com/xm1k/voice-assistant/blob/2c07a471067b92da2998fa6766de40273fff0358/local%20tests/README.md). Сценарии в local tests требуют работающего приложения и провайдера; они **не запускались**. Запущен полный `shared/tests_v2`: **137 passed, 8 skipped**, 9 subtests; пропуски внешних/PG проверок не записаны как успех. Найденные tests подтверждают JSON repair, rollback пакета, stale consent, broker delivery и календарную сериализацию, а не одинаковую с Геной точность реальной речи.

### ryzhilx

Уточнение повторного аудита: повторения в этом SHA **есть**. `CreateTaskTool` и `CreateMeetingTool` принимают recurrence, reminder loop продвигает следующую дату. В первоначальной таблице они были ошибочно пропущены. При ошибке Telegram-отправки этот loop также продвигает/помечает запись в `finally`, поэтому наличие повторов не означает надёжный повтор доставки.

[ToolCallingAgent.run](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/agent/agent.py#L210) действительно принимает несколько calls и исполняет цикл. Обработаны provider tool calls и текстовый JSON fallback. Это подтверждает Multi-Action на уровне исполнения, но не гарантирует атомарность: вызовы инструментов коммитятся отдельно, общего preflight нет, необработанное исключение инструмента может прервать цикл.

[has_pending](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/orchestrator/dialogue.py#L30) всегда возвращает False. История и last_event — process-local словари в агенте; LLM получает последние реплики, но durable список незавершённых действий отсутствует в этом пути. [DeleteRecordTool](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/agent/tools/manage.py#L5) непосредственно вызывает delete_record без отдельного preview/confirm.

[SemanticSearchEngine](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/ai/embeddings.py#L61) содержит Yandex embedding client и локальный hash/synonym vectorizer; основной синхронный search ранжирует локальным вектором. Поэтому название embeddings не доказывает neural search во всём production path. [Telegram reminders](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/bot/reminders.py) и [SpeechKit](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/app/speech/speechkit.py) подключают отдельные практические функции. **31 passed**: выбранные tool-agent/database/reminder tests. [mocked NLU tests](https://github.com/ryzhilx/voice-assistant-gorkycode2026/blob/4d3d9c488b681e195422492ccd6472e282defef9/tests/test_tool_calling_agent.py#L20) подменяет `_call_llm`, поэтому это не live-оценка YandexGPT.

### ArtemST2006

[TaskProcess](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/backend/internal/service/task_process_service.go#L228) связывает Go с AI-service и действительно маршрутизирует create_event/list_events/search_events. `needs_clarification` сохраняет Redis draft, продолжение приходит по task_id. Строгий JSON и контроль timezone — сильные решения; multi operations отсутствуют в этом singular контракте.

[SQL search](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/backend/internal/storage/postgres/task_repository.go#L54) использует `title ILIKE`. Наличие pgvector/embedding-компонентов не превращает именно этот подключённый поиск в семантический. [search service](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/backend/internal/service/subservice/task_search_service.go) подтверждает путь от action к repository. Участок создания требует сохранённой даты и времени; задачи без срока требуют отдельной проверки на live-базовом сценарии, а не вывода по README.

Запущенные Go packages subservice/utils/handlers/config проходят. Полный service package не собран в этой Windows-среде из-за `gcc`/sherpa cgo; это ограничение проверки, а не доказанный дефект проекта. В выбранных Python unit suites **18 passed, 1 failed**, 54 subtests: [test_contract_mirrors_equal_authoritative_bytes](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/ai-service/tests/unit/test_contract_sync.py) обнаружил рассинхронизацию основного OpenAPI и копий AI-service. Сравнение UTF-8 текста подтверждает различия ветки on_delete; это не только переносы строк Windows. Ошибка чтения кодировки отдельного теста устранена запуском Python `-X utf8`, но drift остался. Есть [AI schema tests](https://github.com/ArtemST2006/GorkyCode/blob/ee29c9e1ab95a3fd17461c4a970ac0d964dcfc1d/ai-service/tests/unit/test_schemas.py) и отдельные live suites; внешний AI и STT в этом аудите не поднимались. OpenAPI и многосервисность сами по себе не дают преимущества над работающим простым прототипом.

### Margaryan22

[parse](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/server/nlu/parse.ts#L32) возвращает один Intent после rule/token анализа. [transition/effects](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/server/dialog/transition.ts) отделяет чистый автомат от эффектов; [handleChat](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/server/dialog/engine.ts#L67) исполняет эффекты и сохраняет диалог в одной транзакции. [persisted FSM](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/server/dialog/state.ts) сохраняет draft, выбор и подтверждение, что лучше volatile памяти ryzhilx.

[поиск со стеммингом/синонимами/fuzzy](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/server/search/rank.ts) — реальный retrieval, но не neural embedding. [browser STT](https://github.com/Margaryan22/hokaton_test/blob/89c5442e03f5451737674c7522d1176af2c611d4/src/client/speech/recognition.ts) зависит от SpeechRecognition браузера. Идемпотентный CSV import в schema не доказывает dedupe повторного HTTP chat turn. Unit/E2E файлы найдены, но **не запускались**; браузерная совместимость не измерена. Оценка 4 за контекст основана на архитектуре FSM, а не на утверждении, что все свободные формулировки проходят.

### TRIADA

[AssistantCommand](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/domain/commands.py#L31) — строгая singular схема. [AssistantService.handle_text](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/services/assistant.py#L75) читает pending, вызывает интерпретатор и исполняет команду; [EventService/DialogueService](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/services/events.py) хранит события и уточнения. Конфликты и correction не являются чисто README-функциями.

[search_events](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/services/events.py#L94) ранжирует нормализованный текст/стемы. Это полезный базовый поиск, но общий semantic retrieval не доказан. [STT client](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/integrations/polza.py) и [браузерное озвучивание](https://github.com/TRIADA-TEAM/gorkycode-test/blob/ffe66e7255c2b2e89262cbaaf81ffee2451ec8a1/app/static/app.js) соединены с UI/API. Публичный HTTP контур идентифицирует session, а не предоставляет сравнимую с аккаунтами Гены аутентификацию; нельзя смешивать эти уровни безопасности. **91 passed** в `tests`; провайдеры в тестах подменяются. Live микрофон/внешняя модель не проверялись.

### Britnispirs

[DialogManager/SESSION_CONTEXT](https://github.com/Britnispirs/testovoe_gorkycode/blob/09e672d93c5db2792d34b429c8b78b06525394f1/backend/app/services/dialog_manager.py#L28) хранит контекст в памяти процесса, но действительно уточняет title/time и выполняет CRUD. [LLM/fallback parser](https://github.com/Britnispirs/testovoe_gorkycode/blob/09e672d93c5db2792d34b429c8b78b06525394f1/backend/app/services/llm_client.py) возвращает один intent. Повторный процесс теряет pending, даже если SQLite events сохраняются.

[hybrid_search_events](https://github.com/Britnispirs/testovoe_gorkycode/blob/09e672d93c5db2792d34b429c8b78b06525394f1/backend/app/services/search_service.py#L62) соединяет FTS5 с ручными synonym clusters. Fallback перебирает `db_list_events(limit=250)`, что ограничивает recall на большой истории. [SQLite persistence](https://github.com/Britnispirs/testovoe_gorkycode/blob/09e672d93c5db2792d34b429c8b78b06525394f1/backend/app/database.py) не получает текущего user_id в этих функциях: произвольный session_id разделяет контекст, но не изолирует все events как аккаунты Гены.

[ICS export](https://github.com/Britnispirs/testovoe_gorkycode/blob/09e672d93c5db2792d34b429c8b78b06525394f1/backend/app/api/events.py) — экспорт, а не доказанная синхронизация с Google/Яндекс. При конфликте встреча в отдельных ветках создаётся и сопровождается предупреждением; это отличается от подтверждения до записи. **9 passed** в test_assistant; внешний LLM и браузерная речь не проверялись.

### Svoloch2940194

[text/FSM path](https://github.com/Svoloch2940194/GorkyKod_schedule_bot/blob/159cd00f3e509b049bf795e0b7dd09c89f3ff49f/handlers/text.py#L309) и [voice → text](https://github.com/Svoloch2940194/GorkyKod_schedule_bot/blob/159cd00f3e509b049bf795e0b7dd09c89f3ff49f/handlers/voice.py) показывают соединённый Telegram-путь. [SQLite](https://github.com/Svoloch2940194/GorkyKod_schedule_bot/blob/159cd00f3e509b049bf795e0b7dd09c89f3ff49f/database.py) сохраняет records, но [MemoryStorage](https://github.com/Svoloch2940194/GorkyKod_schedule_bot/blob/159cd00f3e509b049bf795e0b7dd09c89f3ff49f/bot.py#L45) не сохраняет FSM после рестарта.

В [_parse_time](https://github.com/Svoloch2940194/GorkyKod_schedule_bot/blob/159cd00f3e509b049bf795e0b7dd09c89f3ff49f/nlp/parser.py#L124) общий regex `в <число>` стоит до части суток. Воспроизведено прямым вызовом без API: **`_parse_time("в 8 вечера") == 08:00:00`**. Это реальный дефект NLU, а не предположение из описания стека. Полный bot/live voice не запускался; качество edge_tts и recognizer не сравнивалось.

### Agend AI

[IntentDispatcher](https://github.com/ZininRo/agend-ai/blob/d964aa9a966fdc7941e724145badd61bdcf1cbe0/backend/src/main/java/com/example/agend_ai/service/IntentDispatcher.java#L65) перечисляет create/list/search/complete/update в промпте, но dispatch имеет только `if create_item`, остальные отправляет в refused. Значит наличие ItemQueryService/API списка не означает выполнение «Что у меня сегодня?» через ассистента.

[DateResolver](https://github.com/ZininRo/agend-ai/blob/d964aa9a966fdc7941e724145badd61bdcf1cbe0/backend/src/main/java/com/example/agend_ai/service/DateResolver.java#L20) разрешает относительные даты, но имеет эвристику перевода 1–7 в дневные часы. Dispatcher не использует флаги needsTimeClarification/needsDateClarification как полноценный продолжительный диалог. Все операции связываются с `DefaultUser.ID`; сохранение clientMessageId не равно доказанной идемпотентности.

[Testcontainers/WireMock tests](https://github.com/ZininRo/agend-ai/blob/d964aa9a966fdc7941e724145badd61bdcf1cbe0/backend/src/test/java/com/example/agend_ai/api/MessageApiTest.java) — полезные интеграционные тесты с mocked LLM, не запускались: JDK/Maven в среде не найдены. [browser TTS](https://github.com/ZininRo/agend-ai/blob/d964aa9a966fdc7941e724145badd61bdcf1cbe0/frontend/src/features/chat/useSpeech.ts) — реально существующий frontend-контур, без live-проверки. Связь с GORKYCODE **не подтверждена**.

### Lefiper

[MCP create/get/search](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/data-platform/new_assistant_storage/app/mcp_server.py) действительно публикует три инструмента. [create_event](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/data-platform/new_assistant_storage/app/events.py#L99) использует UNIQUE(user_id,idempotency_key), сравнивает payload при повторе, выполняет parameterized SQL в transaction context. Поиск `get_event_by_query` — текстовый SQL, не neural retrieval.

[Redis history/pending CAS](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/data-platform/new_assistant_storage/app/memory.py) использует Lua для bounded history, pending TTL и очистки по expected key. [save_confirmed_event](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/data-platform/new_assistant_storage/app/confirmation.py#L12) связывает Redis → MCP и сохраняет ключ при сбое; одновременно атомарно коммитить Redis и PostgreSQL не пытается. Это удачная граница, но явное распознавание согласия должен делать вызывающий backend.

[MAX transport](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/max-bot/bot.py#L229) отправляет HTTP к AGENT_URL; transport имеет durable inbox/delivery/reminder retry и stable IDs. **15 transport tests passed**, **9 storage validation tests passed / 10 skipped**, 33 subtests; Redis/PG integrations без стенда не засчитаны.

Критический разрыв: [agent-backend README](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/agent-backend/README.md) содержит только заголовок, рядом лежат prompt markdown; [http-api README](https://github.com/Lefiper/gorky-code/blob/4ff466da7bc6ccf2dd456cd4c59223aa3d95c02b/http-api/README.md) также только заголовок. Реализация HTTP agent endpoint в этих каталогах не найдена. Наличие mini-app HTML, data-platform и транспорта ещё не замыкает voice → agent → state → tools → reply. Поэтому устойчивые компоненты оценены отдельно, а готовность всего приложения низко.

### anastbezu

[get_action/process_user_turn](https://github.com/anastbezu/gorkycode-test/blob/493bda8005fce07c5d69a21683ce09d7b22bd01a/backend/agent/agent.py#L44) берёт только первый tool call; известные conversation state не передаются в model messages. [action handlers](https://github.com/anastbezu/gorkycode-test/blob/493bda8005fce07c5d69a21683ce09d7b22bd01a/backend/agent/actions.py#L15) возвращает новый Record с uuid без записи в настоящую БД; list/search возвращают пустые списки.

При этом [durable conversation services](https://github.com/anastbezu/gorkycode-test/blob/493bda8005fce07c5d69a21683ce09d7b22bd01a/backend/services/conversation.py) и настоящие records services существуют. Ключевой вопрос подключения решает [public chat wiring](https://github.com/anastbezu/gorkycode-test/blob/493bda8005fce07c5d69a21683ce09d7b22bd01a/backend/api/chat.py#L18): публичный endpoint всё ещё использует [temporary dispatcher](https://github.com/anastbezu/gorkycode-test/blob/493bda8005fce07c5d69a21683ce09d7b22bd01a/backend/api/dispatch.py#L37). Он умеет список по простым маркерам, прочие фразы превращает в задачу без даты. Значит ни pgvector migration, ни LLM tools, ни merge_pending по отдельности не доказывают полноценный работающий ассистент.

**13 selected agent/prompt/client tests passed**, преимущественно на заглушках. Это не подтверждение persistence LLM actions и не доказательство общей полноты ТЗ. До исправления интеграции этот проект не должен получать бонус только за модульность и документацию.

### Neyromark

Проверены основной repo и все четыре компонента, а не только monorepo README. [главный compose](https://github.com/Neyromark/GorkyCodeTest/blob/6d890b9467db15bb2080693557122ee224c70d46/docker-compose.yml) показывает намерение соединить сервисы. В backend [GigaChat call](https://github.com/Neyromark/GorkyCodeTest-Backend/blob/20e86f1598d71846bfd6a4c7feabb85a0d092e1e/src/services/llm.py) действительно вызывает GigaChat-3-Ultra, но отключает TLS certificate verification. [singular dispatch](https://github.com/Neyromark/GorkyCodeTest-Backend/blob/20e86f1598d71846bfd6a4c7feabb85a0d092e1e/src/choose.py) выбирает один intent; [search stub](https://github.com/Neyromark/GorkyCodeTest-Backend/blob/20e86f1598d71846bfd6a4c7feabb85a0d092e1e/src/key_func/search.py) — буквальный `pass`.

[SentenceTransformer](https://github.com/Neyromark/GorkyCodeTest-Backend/blob/20e86f1598d71846bfd6a4c7feabb85a0d092e1e/src/vectorization.py) загружает multilingual model, но это ещё не подключённый поиск. В [VK voice handler](https://github.com/Neyromark/GorkyCodeTest-VK-Bot/blob/3f6111be25df1110f45b7822842f4d88f9c1c7d2/src/handlers/menu.py#L18) распознанный текст отправляется пользователю; вызова backend planner после STT нет. [Notifications repo](https://github.com/Neyromark/GorkyCodeTest-Notifications/blob/f02575f7bc401f5aa953f088e758510ab5715d75/README.md) — единственный файл этого компонента в snapshot. [database schema](https://github.com/Neyromark/GorkyCodeTest-Database/blob/9a2cb7d6dc17764277c569f5b7c58310a2576193/init.sql) подтверждает БД отдельно; полная интеграция базовых сценариев не подтверждается. Тяжёлые модели не скачивались и внешние VK/GigaChat запросы не выполнялись.

## Подробное сравнение Multi-Action: что адаптировать

| Механизм | Гена до | Гена после | xm1k | ryzhilx |
|---|---|---|---|---|
| NLU | Одна Operation | multi_action до 5, старый envelope сохранён | Decision.commands до 5 | Список tool calls |
| Незавершённые шаги | Один pending | Отдельный pending каждого, durable JSON | clarify с context и history; commands при clarify пусты | Текстовая история процесса, has_pending=False |
| Независимый шаг при неполных данных другого | Терялся | Выполняется и фиксируется, остальные остаются | clarify не допускает side effects | Зависит от ответа LLM/инструмента; structured queue нет |
| Зависимости | Только контекст разных реплик | Явные depends_on + локальный step target | Порядок команд, existing IDs/versions из query; локальные ссылки на новый шаг в schema не подтверждены | Последовательный цикл, неявный last_event |
| Транзакция | Одна команда/реплика | Все эффекты текущей реплики + state/cache; не через уточнения | Атомарный готовый пакет, preview rollback | Commit отдельно по tool |
| Подтверждение | Есть preview/snapshot | Отдельно для каждого шага; inline confirm запрещён | Consent связан с точным пакетом/конфликтами | delete выполняется непосредственно |
| Retry | Request cache | Тот же cache + done/IDs в плане | Outbox, claim, lease/generation, повторная проверка | Durable request dedupe в agent path не найден |
| Ошибки модели | Уточнение | Уточнение с сохранением плана | Bounded regeneration с конкретными ошибками | JSON fallback/общий отказ |

У xm1k следует адаптировать **bounded validation feedback**, проверку итогового состояния готового связанного пакета и операционные метрики retry/claim. Для небольшой Go-системы это можно сделать внутри текущего Service/Store без RabbitMQ и Kubernetes. Готовый пакет xm1k атомарен, а независимые неполные команды Гены намеренно могут завершаться по отдельности; это разные политики, и слепое копирование убрало бы полезный частичный прогресс.

У ryzhilx полезны понятный реестр инструментов и отображение отдельных результатов. Его volatile memory, непосредственное delete и отсутствие общей проверки не следует переносить. У Lefiper полезен payload-bound idempotency key и CAS очистки pending, но аналогичные задачи у Гены уже частично решены request cache и транзакцией JSON state — отдельный Redis ради этого не нужен.

## Сильные стороны Гены, подтверждённые кодом

1. Сквозная изоляция аккаунтов и разговоров, проверка request_id и snapshot до применения результата LLM; это сильнее общего DefaultUser Agend или process-local session dictionaries.
2. Полноценный контур друзей/приглашений/подбора слотов, личные встречи с незарегистрированным человеком и явное отсутствие автоматических приглашений. Большинство соперников подтверждает только локальную запись meeting/attendees.
3. Существующие Google Calendar, Яндекс, Zoom и уведомления связаны с продуктом; не нужно предлагать «добавить календарь/голос/контекст» заново.
4. Новый Multi-Action сохраняет независимые результаты и несколько уточнений, переживает реконструкцию Service и не повторяет done при retry.
5. Проверки точечных и массовых мутаций, отказ от model-provided DB IDs и сохранение нумерации последнего поиска. Многосервисность конкурента не заменяет этих свойств.

Эти преимущества не означают доказанного превосходства STT, естественного языка или скорости на всех запросах: таких общих измерений нет.

## TOP-10 улучшений и план

Это улучшения конкретных границ текущего продукта. Уже работающие голос, контекст, календарь, повторы в domain и социальные встречи повторно реализовывать не предлагается. Сложность: S ≈ до 1–2 рабочих дней, M ≈ несколько дней, L ≈ неделя и больше; это инженерная оценка без обещания срока.

| Приоритет | Что и почему | Как / затронутые компоненты | Сложность | Приёмка |
|---:|---|---|---|---|
| P0-1 | **Multi-Action — выполнено в этой копии.** Команды не должны теряться | nlu.MultiAction, domain.ActionPlan, assistant executor, compact context, API result; существующие безопасность и Store | M | Исходные три команды, несколько pending, зависимости, ошибка рядом с успехом, retry, PostgreSQL isolation, live GigaChat |
| P0-2 | Регулярный live-корпус четырёх обязательных действий и omissions | Расширить cmd/simple-smoke и corpus: одиночные/составные, отрицания, разговорная речь, точность записей в БД; фиксировать исходный SHA/model/timezone | M | Полные диалоги + проверка БД; пропуски команд считаются провалом, не только валидный JSON |
| P0-3 | Ограниченное исправление невалидного LLM-ответа | llm.Simple/GigaChat: максимум 1 повтор с безопасными типизированными ошибками; inference вне write tx; повторная snapshot check | M | Некорректный JSON, неизвестные поля, даты, отмена/таймаут; ни одной мутации до валидного результата; лимит задержки |
| P0-4 | Устранить разрыв повторений между simple NLU и domain | Расширить Create/Changes существующим recurrence DTO и reuse domain validation/calendar recurrence; не создавать новый scheduler | M | «Каждую пятницу», конец/число повторений, изменение серии/экземпляра, конфликт; одиночные даты без регрессий |
| P0-5 | Стабилизировать временную неопределённость и даты в плане | Развить time_ambiguous и explicit missing slots, фиксировать reference time, проверять omission/необоснованную подстановку дня; simple_time/Temporal/socialContinuation | M | «в 8», «в половине восьмого», «ночью», таймер секунд, граница суток/DST; не теряются участники и длительность |
| P1-6 | Измерить и затем улучшить semantic recall | Общий независимый набор запросов на 1000 CSV, сравнить текущий Snowball/synonyms с optional embeddings + reranking; сохранить lexical fallback и user/date filters | L | Recall@k/MRR по вручную размеченным целям, слова вне synonym словаря, имена, даты, деградация модели, latency |
| P1-7 | Явная политика для связанного готового пакета | Поверх текущих depends_on добавить, при реальной потребности, preflight/atomic group через repository savepoint/transaction; независимые шаги оставить отдельно | M/L | Последний шаг группы невалиден → группа не изменилась; соседняя независимая задача не потеряна; точное подтверждение итогового snapshot |
| P1-8 | Управление несколькими уточнениями в UI | Использовать уже добавленный Response.operations для выбора ожидающего шага/отмены одного; typed server token для выбора key, без model IDs | M | Пользователь исправляет третье действие раньше первого; старые кнопки не подтверждают другой preview; мобильный экран и клавиатура |
| P1-9 | Видимость внешних синхронизаций и доставки | Расширить существующие cloud/yandex/notifications statuses метриками и привязкой к шагу; retry существующих outbox/jobs, не новая доставка с нуля | M | Локально сохранено / remote pending / failed / synced различимы; timeout и повтор не создают remote duplicate; интеграционные smoke |
| P2-10 | Контролируемые пределы времени и размера NLU-плана | Простые метрики plan size/LLM latency/ожиданий и тесты 5 шагов; на основе измерений настроить token/concurrency budgets, не добавлять микросервисы заранее | S/M | Пять длинных команд, cancellation, ограниченная память/state JSON, timeout, concurrent sessions и p95 на одном стенде |

Порядок реализации: закончить ревью и выпуск P0-1; запустить P0-2 на финальном SHA; устранить найденные провалы через P0-3/4/5; проверить голосовой сценарий в браузере и реальный календарь с тестовым аккаунтом. Затем измерять поиск и внешние сбои, а UI нескольких уточнений и atomic groups внедрять по подтверждённым сценариям. Не следует тратить оставшееся время отбора на миграцию Go-сервиса в Kubernetes только ради архитектурного сходства с xm1k.

## Относятся ли репозитории к отбору 2026

Подтверждён факт проведения GORKYCODE 2026 и обязательного тестового задания: [публикация о мероприятии](https://www.xn--80aa3anexr8c.xn--p1acf/tpost/egbejr4eb1-gorkycode-2026). Она **не содержит официальный список этих участников**. Название repo, дата коммита или одинаковый CSV не подтверждают регистрацию/отправку работы/допуск.

| Проект | Найденная связь | Уровень вывода |
|---|---|---|
| ArtemST2006 | GorkyCode в названии + PDF совпадающего ТЗ | Самоидентификация и совпадение задания; официальное участие не проверено |
| ryzhilx | gorkycode2026 в имени + общий 1000-row CSV | Сильная самоидентификация именно с 2026; официальная подача не подтверждена |
| Margaryan22 | Общий исходный CSV и тот же набор сценариев | Совпадение тестового материала; конкретный конкурс/год требует внешнего подтверждения |
| TRIADA | gorkycode-test в имени + PDF того же задания | Связь с заданием; допуск в 2026 не подтверждён |
| Britnispirs | testovoe_gorkycode в имени и соответствующие сценарии | Самоидентификация; не официальный список |
| anastbezu | gorkycode-test, общий CSV, gorky.bond в проектных материалах | Самоидентификация/материалы задания; не подтверждение подачи |
| Svoloch2940194 | GorkyKod_schedule_bot в имени | Только самоидентификация и сходная задача |
| Lefiper | gorky-code, MAX «Горький код» | Самоидентификация; официальный статус не найден |
| Neyromark | GorkyCodeTest и четыре одноимённых компонента | Самоидентификация; официальная подача не подтверждена |
| xm1k | Совпадающие обязательные сценарии и материалы Голос v2 | Технический кандидат для сравнения; прямое официальное подтверждение GORKYCODE 2026 не найдено |
| ZininRo | Agend AI, совпадающий класс задачи | **Потенциальный конкурент; принадлежность к GORKYCODE не подтверждена** |

Нормализованные CSV fingerprints и все SHA находятся в [машиночитаемом snapshot](evidence/competitors_snapshot_2026-10-26.json). CSV Гены, ryzhilx, anastbezu и Margaryan22 совпадают после разбора CSV: canonical SHA-256 начинается с `480298fbe53f228c`; различия raw-byte hash связаны с представлением файла. Даты октябрьских коммитов подтверждают свежесть исходников, но не являются датой регистрации на конкурс.

## Воспроизводимость

| Проверка | Результат аудита |
|---|---|
| Гена, весь go test + vet | Финальный статус и счётчики в `artifacts/multi_action_go_test.jsonl`; PG tests запускались на отдельной loopback БД |
| Гена, PostgreSQL durable plan/user isolation | Пройдено, включая Service reconstruction |
| Гена, настоящий GigaChat + PG: 3 точных действия | Пройдено |
| Гена, исходная фраза со скриншота + уточнения | Пройдено; transcript `artifacts/multi_action_live_postgres.txt` |
| Гена, зависимая цепочка с настоящим GigaChat | Пройдено: создание Отчёта → complete именно его, независимый Хлеб остаётся open; transcript `artifacts/multi_action_dependency_live_postgres.txt` |
| Гена, web build | TypeScript + Vite build пройден; существующее предупреждение о размере chunk |
| Гена, локальный race detector | Не запустился: в Windows нет gcc/cgo; concurrency regression без race прошла, CI остаётся местом race-проверки |
| xm1k, shared/tests_v2 | 137 passed / 8 skipped / 9 subtests; live календарь и внешние integrations не засчитаны |
| ryzhilx, tool agent + database + reminders | 31 passed, NLU mocked |
| TRIADA, tests | 91 passed |
| Britnispirs, test_assistant | 9 passed |
| anastbezu, prompt/process/client | 13 passed; stubs не становятся production implementation |
| Lefiper MAX transport | 15 passed |
| Lefiper storage memory/events/MCP | 9 passed / 10 skipped / 33 subtests |
| Artem, Go subservice/utils/handlers/config | Passed; полный service требует C compiler |
| Artem, выбранные Python unit suites | 18 passed / 1 failed / 54 subtests; реальный OpenAPI contract drift |
| Margaryan22 | Код и тесты прочитаны; npm/E2E запуск не выполнялся |
| Agend AI | Код и интеграционные тесты прочитаны; JDK/Maven недоступны |
| Svoloch | Воспроизведён дефект времени прямым вызовом parser |
| Neyromark | Проверена связность всех компонентов; full runtime не запускался |

Никакие «прошло N тестов» не используются как общий балл качества NLU. Чужие live-модели, календарные аккаунты и публичные демонстрационные записи не изменялись.
