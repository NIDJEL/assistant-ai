# Контекстный диалог Гены

Актуальная оценка v2 и решение о production: [CONTEXT_DIALOGUE_V2_REPORT.md](CONTEXT_DIALOGUE_V2_REPORT.md).
Первый аудит ниже оставлен как исторический baseline, а не независимый hold-out.

Контекст добавлен поверх `assistant.Service`, `domain.Command`, repository и
существующего executor. Миграции БД и изменения frontend не нужны. Production
не использовался: оценка работает с временными локальными SQLite-базами.

## Причина прежнего поведения

Перед interpreter передавался только `PendingExpectation`: название ожидаемого
поля без содержимого команды. `Semantic.Interpret` пропускал многие ответы на
уточнение как `help`, а после inference executor пытался объединить их с pending.
Разговорное продолжение могло попасть в независимый semantic parser, потерять
название и превратиться в новую неполную команду. `LastIDs` не имели TTL.

## Путь сообщения

1. В пользовательской read-транзакции загружаются state и компактный контекст.
2. Локальный fast path обрабатывает уверенные ответы на ожидаемое поле,
   подтверждения, отмену, простой выбор и стандартные приветствия.
3. Для продолжения вызывается `Semantic.InterpretTurn` / `ContextualProvider`.
   Самостоятельные команды используют прежние SemanticCommand/prompt/schema.
4. Strict JSON и semantic guards проверяют результат. Имена, временные выражения
   и intent должны опираться на сообщение и разрешённый контекст.
5. Go resolver внутри прежней write-транзакции выбирает реальные ID и строит
   `domain.Command`. Изменившийся за время inference state блокирует применение
   старого контекста.
6. Прежний executor проверяет данные, поиск, конфликты и подтверждения, выполняет
   действие и сохраняет state/exchange. Ответ формирует Go; второго LLM-вызова нет.

`ContextualTurn` различает command, slot_reply, reference, choice, confirmation,
cancellation, refinement и chat. У reference есть semantic action и отношение
last/first/second/third/fourth/fifth/previous/matching. Изменения передаются в
SlotUpdate. `previous` без однозначного доступного контекста вызывает уточнение.
Все объекты схемы запрещают дополнительные поля; decoder также отклоняет
пропущенные обязательные поля, duplicate keys, ID и trailing JSON.

## Состояние и privacy

`State.Pending` и `State.LastIDs` сохраняются без дублирования. Новый `Dialogue`
содержит последнее действие/тип, query и границы фильтра, последние сообщения
пользователя/ассистента, время результата и счётчик несвязанных реплик.
`expected_slot` вычисляется из Pending, отдельная копия не сохраняется.

Pending действует до выполнения/отмены/замены, максимум 30 минут. Ссылки на
результат действуют 15 минут и менее четырёх несвязанных реплик. Chat не
продлевает время результата. State старой версии без времени контекста не
используется как свежая ссылка на запись.

В LLM передаются только разрешённые поля: pending action/kind/title/date/time,
ожидаемое поле, предыдущий фильтр, до восьми коротких описаний кандидатов и
обрезанные предыдущие сообщения. Нет raw Event, профиля, email участников,
OAuth, токенов, внутренних ID и полной истории. Позиции недоступных записей
сохраняются, чтобы ordinal не сдвинулся на другую запись.

Удаление всегда проходит существующий preview/confirmation со snapshot.
Semantic confirmation не заменяет проверку фактического «да». При нескольких
кандидатах relation=last не выбирает запись автоматически. Исправления без
явного действия разрешены только для свежего однозначного create/update.

## Fast path и таймеры

До LLM локально обрабатываются ответы на известный temporal slot, включая
разговорную оболочку и обратный порядок «минут через N». Helper
`ExtractExpectedTemporalSlot(message, expectedSlot)` возвращает фрагмент исходного
сообщения; Go parser должен разобрать его целиком. Безопасные вводные слова
игнорируются только по границам ответа на pending; обычный command parsing не
переписывается. Альтернативы, отрицания и дополнительные действия не отбрасываются.
Также локально обрабатываются настройки уведомлений, простой ordinal и chat.

`ClassifyConfirmation` классифицирует полную реплику по полярности: отказ,
согласие или неизвестно. Отказ очищает pending без изменения события; согласие
исполняется только при delete/delete_many/conflict pending. «Нет» в отдельном
pending настройки уведомлений сохраняет прежний смысл отключения уведомления.
Модель не может заменить проверку согласия пользователя. Неподдержанный ответ
передаётся contextual provider, ошибка вызывает уточнение без изменения событий.
Пустой slot_reply восстанавливается только при отдельной typed validation error,
из grounded ожидаемого temporal slot. Некорректный JSON/schema не исправляется.

Контекстный разбор используется для разговорных продолжений, ссылок,
исправлений и refinement. Новая явная команда заменяет старый pending.
Refinement сохраняет query/kind/период и меняет только указанный фильтр. Простое
«только <период/тип>» обрабатывается Go только при свежем успешном search/list.
Неудачная новая попытка поиска инвалидирует прежний фильтр. Успешный поиск с
нулевыми результатами остаётся валидным. Фильтр по часам «после/до N» не
поддерживается и вызывает уточнение.

Новые DEV выявили, что prompt с поиском по участнику сам по себе ещё не стабилен
на всех именах. Для полного явного «найди встречи с X» добавлен консервативный
read-only fast path: X копируется дословно, без словаря имён и лемматизации.
Временные фильтры, исключения, цитаты и составные директивы остаются у semantic
provider. Поисковый движок `internal/search` не менялся.

В таймерном контексте интервал может предшествовать слову «таймер» или следовать
за ним; поддерживаются «N минут», «минут N», «минут на N», часы и полчаса.
Go проверяет интервал по исходной countdown-конструкции. Модель сохраняет
написание числа и порядок слов в temporal expression; clean title содержит
только предмет/действие. Встреча «на 30 минут» сохраняет duration; это правило
не применяется глобально.

## Воспроизведение оценки

```powershell
go test ./internal/nlp/... ./internal/assistant/... ./internal/llm/...
$env:GIGACHAT_INTEGRATION = '1'
go run ./cmd/context-eval -input testdata/semantic/context_dev_v2.jsonl -output artifacts/context/dev2.json
go run ./cmd/context-eval -input testdata/semantic/context_search_dev_v2.jsonl -output artifacts/context/search-dev2.json
# Fresh v2: только с ранее зафиксированным protocol. Не менять код по failures.
go run ./cmd/context-eval -input testdata/semantic/context_heldout_v2.jsonl -protocol testdata/semantic/context_heldout_v2_protocol.json -cache artifacts/cache/context-v2-heldout -output artifacts/context/heldout-v2.json
# Повторная Go-проверка сохранённых strict ответов без API:
go run ./cmd/context-eval -cache-only -input testdata/semantic/context_dev.jsonl -output artifacts/context/dev-replay.json
$env:GIGACHAT_EVAL_ARTIFACT = 'artifacts/gigachat/mandatory-GigaChat-3-Ultra.json'
go test -count=1 ./...
go vet ./...
```

Оценка не использует `cfg.DatabaseURL`: каждая conversation получает временную
SQLite-базу. Cache учитывает model, prompt, schema, source, сообщение и весь
компактный контекст; cached JSON декодируется заново. Прогресс сохраняется JSONL,
полный результат — JSON; в консоли максимум 20 идентификаторов ошибок.

`context_dev.jsonl`: 24 conversation / 53 turn, включая 13 mandatory.
`context_heldout.jsonl`: 110 новых conversation / 220 turn. Наборы разделены до
inference; SHA-256 сохранены в `context_manifest.json`. Старый frozen held-out
208 не использовался для tuning или новой оценки.

DEV допускает настройку. Held-out служит аудитом и не используется для исправления
prompt/грамматики. Первоначальный аудит был прерван из-за независимо найденной
регрессии на прежних 13 CORE-тестах; после исправления маршрутизации prompt и
held-out остались прежними, уже полученные raw ответы переиспользованы. Протокол
и hash исходников сохранены в `artifacts/context/heldout-protocol.json`.

Метрики turn type/reference проверяют классификацию и отношение, target —
ожидаемый логический ID, slot completion/context retention — контракт нужного
результата. Domain command accuracy проверяет action и заданные gold поля
результата/цели/фильтра, а не побайтовое равенство всех необязательных полей DTO.
Unsafe mutation означает запись при ожидаемом read-only/clarification/cancel
или изменение неверной ожидаемой цели. Это конечная тестовая выборка, а не
доказательство безопасности для всех возможных фраз.

Model calls per turn считает обращения к semantic provider, включая cache replay;
network calls отдельно считает реальные обращения при запуске оценки. Поэтому
cache не создаёт искусственный выигрыш в доле turns без LLM. Latency inference
считается только по реальным сетевым вызовам; локальный fast path измеряется
отдельно. HTTP retries/OAuth не являются дополнительным semantic pass.

Полный исходный CORE score 97,79% не переизмерялся на прежнем held-out. Проверены
сохранённые ответы всех 13 mandatory CORE через действующий executor и весь Go
test suite. Frontend не менялся, его тесты не запускались.

## Исторический результат первого аудита v1, 16 октября 2026

| Метрика | DEV | Fresh held-out |
|---|---:|---:|
| Полностью успешные диалоги | 24/24 — 100% | 95/110 — 86,36% |
| End-to-end по репликам | 53/53 — 100% | 201/220 — 91,36% |
| Mandatory contextual | 13/13 — 100% | отдельная аудитная выборка |
| Turn type | 53/53 — 100% | 209/220 — 95% |
| Pending slot completion | 5/5 — 100% | 16/20 — 80% |
| Reference relation, однозначные случаи | 10/10 — 100% | 30/30 — 100% |
| Target ID, однозначные случаи | 8/8 — 100% | 30/30 — 100% |
| Context retention | 19/19 — 100% | 58/70 — 82,86% |
| New command vs continuation | 53/53 — 100% | 209/220 — 95% |
| Domain command/result contract | 31/31 — 100% | 128/140 — 91,43% |
| Unsafe accepted mutations | 0 | 0 |
| Semantic calls / turn | 0,717 | 0,827 |
| Turns без LLM | 28,30% | 17,27% |

Локальный slot fast path: среднее 0,264 мс, p95 2,638 мс (10 измерений в fresh
аудите; нулевые значения отдельных измерений связаны с разрешением часов).
Весь turn без LLM: среднее 2,078 мс, p95 4,724 мс.
Contextual GigaChat: среднее 1821 мс, p50 1789 мс, p95 2607 мс, 48 успешных
сетевых измерений финального запуска. Cache replay в сетевую latency не включён.
Из 182 обращений к provider в финальном fresh запуске 93 были сетевыми; остальные
использовали ранее полученные ответы, включая прерванный аудит. Модель —
GigaChat-3-Ultra, temperature=0, concurrency=1.

Основной сценарий проходит с точным ответом:

> Напомни вытащить пиццу из духовки → Когда напомнить? → Ну через 10 минут →
> Готово. Через 10 минут напомню вытащить пиццу из духовки.

Также проходят перенос последней созданной встречи, ordinal второй записи,
цепочка date/time updates, исправление участника, настройка напоминания после
создания, refinement поиска с Алисой, замена pending новой командой, простая
отмена/подтверждение и chat. Таймер с пиццей и «засеки 5 минут проверить чай»
создаются без уточнений; duration встречи на 30 минут не превращается в таймер.

Из 15 проваленных fresh conversations четыре имеют только ошибку классификации
неоднозначной ссылки: фактический безопасный ответ-уточнение верный. Остальные
11 показывают функциональные ограничения. Разговорные временные ответы дают
четыре сбоя; refinement — четыре, из которых три начинаются с rejection ещё
в прежнем single-turn поиске по новым именам; разговорная отмена — два; таймер
с обратным порядком слов — один. «Нет, не удаляй» иногда оставляет pending
подтверждения вместо отмены: это существенная причина не выпускать изменение.

После held-out исходники/prompt/схема не менялись по его результатам. Следующий
цикл должен добавлять независимые DEV-сценарии и новый свежий hold-out; текущий
fresh набор после этого аудита больше не является неизвестным.

`go test -count=1 ./...` и `go vet ./...` — PASS; сохранённые 13 CORE-регрессий —
13/13 PASS. Postgres integration tests требуют отдельной тестовой базы и могут
быть пропущены штатным test suite; production DB не подключалась.

**Рекомендация: production deploy пока не готов.** Mandatory-порог 98% пройден,
но общий fresh multi-turn score и разговорная отмена требуют следующего цикла.
Деплой, SSH, изменения production DB/env не выполнялись.

Полные локальные результаты: `artifacts/context/dev-final.json`,
`artifacts/context/heldout-final.json`, `artifacts/context/representative-failures.json`,
`artifacts/context/core-mandatory.log`, `artifacts/context/go-test-final.log` и
`artifacts/context/go-vet-final.log`. Файлы artifacts игнорируются Git.
