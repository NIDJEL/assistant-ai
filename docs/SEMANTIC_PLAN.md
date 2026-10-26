# SemanticPlan: язык в GigaChat, исполнение в Go

Дополнение: [Semantic evidence validation](SEMANTIC_EVIDENCE.md) описывает новый source evidence guard до grounding/preview. Исторические результаты v5 остаются в прежнем отчёте; новая оценка публикуется отдельно.

Локальный архитектурный рефакторинг. Production/VPS, production env и БД не
используются при оценке. Результаты проверки: [SEMANTIC_PLAN_REPORT.md](SEMANTIC_PLAN_REPORT.md).

## Маршрутизация

Раньше `fastDialogue → needsContext → InterpretTurn / Semantic.Interpret`
выбирали между contextual и single-turn контрактами. Single-turn дополнительно
пробовал participant search, list, meeting declaration и legacy temporal parsing.
`Hybrid` опирался на parser confidence и мог возвращать rules fallback.

При `GENA_SEMANTIC_PROVIDER=gigachat` сервер теперь создаёт `llm.Planner`:

```text
original message + compact fresh dialogue
    → exactPlanFastPath
    → exact match: existing dialogue/executor policy
    → otherwise: one logical ParsePlan(original message, context)
    → strict JSON + semantic invariants + lexical provenance
    → normalized temporal arithmetic
    → user-scoped repository grounding
    → domain.Command → existing assistant.Service transaction
```

`GENA_SEMANTIC_PROVIDER=ollama` использует тот же provider-neutral plan contract.
Legacy `SemanticCommand`, contextual parser, `Parser` и `Hybrid` сохранены для
совместимости и явного режима rules. Новый planner их не вызывает и не использует
их как fallback при ошибке модели. Нельзя считать legacy corpus результатом
проверки нового runtime: у него отдельный evaluator `cmd/plan-eval`.

## Белый список fast path

- Точные «да», «подтверждаю» при ожидаемом delete/conflict/batch confirmation.
- Точные «нет», «не надо», «отмена», «стоп» в подходящем pending.
- Точный порядковый выбор среди показанных pending candidates, первые пять.
- `через N секунд/минут/часов` при ожидаемой relative delay, N=1..1440.
- `в HH:MM` при ожидаемом time; сегодня/завтра/послезавтра при ожидаемом date.
- Небольшой набор точных ответов на вопрос об уведомлениях.
- Точные приветствия, благодарность и help routing.

Разговорные wrappers, search любого уровня сложности, уточнения фильтров,
исправления и полные команды со ссылкой идут в модель. Полного create fast path
нет: ради уменьшения риска даже обычное создание идёт в SemanticPlan.
`RouteDecision` содержит path, boolean exact match и reason, без ML confidence.

## Контракт

Полная машиночитаемая схема: [semantic_plan.schema.json](../testdata/semantic/semantic_plan.schema.json).
DTO и проверки: `internal/nlp/plan.go`. Все поля объектов обязательны;
необязательные значения nullable. Пустые массивы допускают []/null. На каждом
уровне `additionalProperties=false`. Проверяются enums, типы, integer bounds,
обязательные поля, неизвестные/повторяющиеся поля, trailing JSON, длина/глубина,
непустые changes и несовместимые payload. Model confirmation/cancellation с
status=ok запрещены: модель не может авторизовать мутацию.

Операции: create/list/search/update/delete/complete/batch_update/batch_delete/
batch_complete/chat. Типы: task/meeting/reminder/any. `dialogue_act` различает
command/refinement/slot_reply/choice. Non-ok status не может содержать action data.

План не содержит DB IDs, SQL, absolute timestamps, account identifiers или tools.
Лексическая provenance-проверка сравнивает имена/названия с оригиналом и
разрешённым контекстом; это safety guard без определения intent или разбора дат.
`Таймер` разрешён как техническое название reminder с delay без явного заголовка.
Сохранённый raw ответ в evaluation cache проверяется заново при каждом replay.

Пример payload сложного поиска (показаны только ненулевые поля; wire JSON
также содержит остальные обязательные поля со значением null):

```json
{
  "status": "ok", "dialogue_act": "command",
  "operation": "search", "kind": "meeting", "query": "Алисой",
  "filters": {"period": {"type": "calendar_period", "unit": "month", "offset": -1}}
}
```

«А только за прошлый месяц?» после успешного search:

```json
{
  "status": "ok", "dialogue_act": "refinement", "operation": "search",
  "filters": {"period": {"type": "calendar_period", "unit": "month", "offset": -1}}
}
```

Go наследует query, kind и прежние фильтры, заменяя явно заданные ограничения.
Контекст поиска истекает через 15 минут или четыре нерезультативных turn.
Pending истекает через 30 минут. Inference выполняется вне write transaction;
перед contextual resolution проверяется неизменность снимка state.

После первого fresh v3 добавлена отдельная safety-проверка ordinal grounding:
модель не вправе подставить позицию кандидата вместо неоднозначного местоимения.
Go сверяет ordinal с ограниченным набором явных порядковых форм исходной фразы
и количеством показанных кандидатов. Номер часов и проверяемые temporal-формы
вроде «на вторую неделю» не считаются подтверждением выбора. При отсутствии
проверяемого указания — clarification. Это проверка source evidence после
ParsePlan; intent, изменения и время полного запроса по-прежнему определяет LLM.
Неподдержанная форма ссылки требует уточнения. Fast ordinal selection также
повторно проверяет сохранённый snapshot кандидата перед исполнением.

## Время

`ResolveTemporal` не получает исходную фразу. Нормализованные типы:

| Тип | Значение | Вычисление Go |
|---|---|---|
| calendar_period | unit + offset | Полный day/week/month/quarter/year, [start,end) |
| relative_day | offset | Civil calendar day относительно текущего локального дня |
| relative_delay | unit + value | Elapsed seconds/minutes/hours; civil days/weeks/months/years |
| weekday | weekday + nullable direction | null/next_or_current либо день следующей/прошлой календарной недели |
| calendar_date | year + month + day | Явная дата пользователя, проверка существования |
| rolling_period | unit + value | Последние N календарных единиц, включая сегодняшний день |

Неделя начинается в понедельник. Для 17.10.2026 Europe/Moscow предыдущий месяц
равен [01.09.2026,01.10.2026), следующая неделя — [19.10.2026,26.10.2026).
Изменение года, leap February, quarters, timezone и DST проверяются отдельно.
Recurrence использует существующий domain calendar engine, с timezone/anchor
из Go. Неопределённое «после обеда» для точного переноса требует уточнения часа.
Для поиска это нормализованный нижний порог 14:00.

## Исполнение и batch

Поиск использует существующий search engine и repository. Status, priority,
clock bounds и exclusions применяются к реальным записям. Refinement хранит
все grounded фильтры, а не только query/период. Ordinal/last переводятся в
локальные LastIDs; отсутствующий/неоднозначный target не угадывается.

Любой batch, даже для одной найденной записи, требует preview и следующего
точного confirmation. Preview сохраняет исходные и предложенные записи,
показывает изменение, число записей и конфликты. Лимит — 100 записей.
Подтверждение действует только на сохранённый список, новые совпадения в него
не попадают. Изменённая/удалённая запись или новые конфликты отменяют исполнение.
Все записи изменяются одной existing Store.Write transaction: ошибка второго
update откатывает первый. Для повторяющейся записи относительный time shift
пока безопасно отклоняется, можно задать явное новое время.

Точная отмена, delete confirmation и conflict policy остаются в Go. Chat не
вызывает DB action executor (state/history сохраняются обычным dialogue flow).
Credentials, headers, OAuth и DB IDs в model context не передаются.

## Проверка

```powershell
go test -count=1 ./...
go vet ./...
$env:GIGACHAT_INTEGRATION = '1'
go run ./cmd/plan-eval -input testdata/semantic/plan_complex_dev.jsonl -output artifacts/plan/dev.json
```

Evaluator всегда создаёт отдельную временную SQLite-базу на каждый диалог и
не открывает `DATABASE_URL`. DEV: 120 conversations / 240 turns, 12 категорий.
Скрипт автора не читает старые corpora. Query сравнивается лексически с учётом
склонения, event title/date/time и mutations проверяются точно.

После финализации DEV создаётся fresh v3: 150 conversations / 300 turns.
Сначала `-freeze protocol.json`, затем inference с `-protocol protocol.json`.
Protocol фиксирует input, prompt, schema, исходники, tests, dependency manifests
и authoring scripts. Evaluator сверяет freeze до и после inference,
запрещает partial holdout run и отказывается запускать v2 audit corpus.
Нельзя менять код/prompt/gold внутри замороженного аудита. После исправления
нужен другой fresh; повтор v3 нельзя выдавать за независимую проверку.

Первый v3 обнаружил ошибку безопасности и сохранён именно как неудачный аудит.
Его source snapshot архивирован до исправления; повторного inference v3 нет.
После safety repair добавлены отдельный reference DEV и новый focused fresh
v4 (40 conversations / 80 turns), с собственным freeze. Затем проведён новый
полный fresh v5 текущей версии: 150 conversations / 300 turns. Результаты обоих
аудитов и оставшиеся ошибки приведены в отчёте; v3 не переоценивался.

Dev server логирует только route/reason/calls/validity/latency. Evaluation
сохраняет синтетические запросы и raw model JSON отдельно от credentials.
Метрики включают routing, validity, end-to-end, multi-turn, complex accuracy,
unauthorized mutations и отдельную latency fast / network LLM. Cache replay
не является измерением сетевой latency.
