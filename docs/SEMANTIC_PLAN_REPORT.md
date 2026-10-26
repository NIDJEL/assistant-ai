# SemanticPlan — локальная реализация и независимые прогоны

Полные журналы и архивы из `artifacts/plan/` сохранены локально и не входят в публикуемый репозиторий. Наборы для воспроизведения находятся в `testdata/semantic/`; актуальные публичные доказательства приведены в [отчёте об улучшениях](QUALITY5_IMPROVEMENTS.md).


17 октября 2026. Модель: **GigaChat-3-Ultra**, temperature=0.

**Production ready: NO.** Архитектура реализована локально. В финальном полном
fresh v5 — **287/300 реплик (95,67%)**, **137/150 диалогов (91,33%)**.
Несанкционированных изменений записей по определению evaluator — **0**, но
есть **10 подтверждённых переносов на неверно понятую моделью дату**. Это
полноценные ошибки, а не успешные исполнения. Обязательный preview не заменяет
правильного понимания пользователя.

Первый fresh v3 выявил **2 небезопасные мутации** из-за выдуманного ordinal.
Этот результат сохранён без переоценки. После исправления выполнены новые DEV,
focused fresh v4 и новый полный fresh v5. Production/VPS, SSH, deploy,
production env и production DB не использовались.

## 1. Старый routing

`fastDialogue → needsContext → InterpretTurn / Semantic.Interpret`.
Внутри single-turn маршрута дополнительно работали participant-search,
list/declaration fast paths и temporal parsing. Legacy `Hybrid` опирался
на confidence и rules fallback. Таким образом, Go участвовал в понимании
полной фразы до обращения к модели.

## 2. Новый routing

```text
ORIGINAL message + compact fresh dialogue
  → exact deterministic pending/chat whitelist
  → иначе один логический ParsePlan
  → strict SemanticPlan + validation / lexical provenance
  → Go temporal arithmetic / reference grounding
  → domain.Command
  → existing assistant.Service / user-scoped repository transaction
```

`cmd/server` выбирает `llm.Planner` для semantic GigaChat и Ollama. Модель не
получает tools, SQL, repository, реальные DB IDs или право выполнить действие.
Inference проходит вне write transaction; перед contextual resolution Go
сверяет snapshot dialogue. Ошибка модели приводит к уточнению без rules fallback.

## 3. Какие Go NLP paths обойдены

Новый semantic runtime не вызывает `needsContext`, старые `InterpretTurn` /
`Semantic.Interpret`, participant-search/list/meeting-declaration parsers,
`Hybrid` confidence fallback или `MapSemantic` для полной команды.
Нормализованный temporal object не проходит повторный Russian regex parsing.

Legacy код сохранён для явного режима rules, старых инструментов и регрессий;
это совместимость, а не резервная цепочка нового planner. Незакоммиченные
изменения, находившиеся в workspace до начала работы, сохранены.

## 4. Что осталось в fast path

- Точное confirmation при соответствующем pending и точная отмена.
- Полный ожидаемый slot: `через N минут/секунд/часов`, `в HH:MM`,
  сегодня/завтра/послезавтра.
- Небольшой whitelist ответов об уведомлениях.
- Порядковый выбор в pending candidates, первые пять.
- Точные help, приветствия и благодарность.

Полные create/search/refinement/correction и contextual commands идут в модель.
`RouteDecision` содержит boolean `FastPathMatched`, path и reason, без
псевдовероятности. Route/reason/calls/validity/latency логируются в dev без текста
пользователя, DB contents или authorization data.

## 5. SemanticPlan schema

Полный контракт: [semantic_plan.schema.json](../testdata/semantic/semantic_plan.schema.json).
Описание: [SEMANTIC_PLAN.md](SEMANTIC_PLAN.md).

Обязательные top-level поля: status, dialogue_act, operation, kind, title, query,
filters, target, schedule, changes, clarification, chat_response. Необязательные
значения nullable. Все объекты имеют `additionalProperties=false`.

Операции включают create/list/search/update/delete/complete/batch_update/
batch_delete/batch_complete/chat. Проверяются enums, required fields, типы,
числовые границы, несовместимые payload, пустые changes, duplicate/unknown fields,
trailing JSON и размер/глубина. Одни и те же проверки действуют для live и cache.
Model confirmation/cancellation с status=ok запрещены.

## 6. Сложная команда → plan

«Найди встречи с Алисой только за прошлый месяц»:

```json
{
  "status": "ok", "dialogue_act": "command",
  "operation": "search", "kind": "meeting", "query": "Алисой",
  "filters": {"period": {"type": "calendar_period", "unit": "month", "offset": -1}}
}
```

Это сокращённая проекция: wire JSON также содержит все остальные обязательные
поля со значением null. Полные реальные model outputs и материализованные Go
commands для пяти DEV-примеров сохранены в
[semantic_plan_examples.json](semantic_plan_examples.json).

## 7. Refinement → plan

Контекст: успешный search, kind=meeting, query=Алисой.
«А только за прошлый месяц?»:

```json
{
  "status": "ok", "dialogue_act": "refinement", "operation": "search",
  "filters": {"period": {"type": "calendar_period", "unit": "month", "offset": -1}}
}
```

Go наследует query/kind и остальные grounded filters, заменяя переданный период.
Устаревший или отсутствующий успешный поиск не восстанавливается по догадке.

## 8. Temporal normalization и execution safety

Поддержаны calendar_period, relative_day, relative_delay, weekday, calendar_date,
rolling_period, normalized clock и recurrence. Go рассчитывает календарь в IANA
timezone. Неделя начинается в понедельник, интервалы half-open. Для anchor
17.10.2026 Europe/Moscow предыдущий месяц — [01.09.2026,01.10.2026), следующая
неделя — [19.10.2026,26.10.2026).

Unit tests проверяют January/December rollover, leap February, недели, кварталы,
timezone/DST, невозможные значения, rolling period и canonical weekday default.

Любой batch требует preview и отдельного confirmation. Сохраняются before/after
snapshots и показанные конфликты. Изменившиеся записи/конфликты требуют нового
preview; новые совпадения не включаются в уже подтверждаемый список. Мутации
атомарны: отдельный тест с ошибкой второго update доказывает rollback первого.
Проверены restart, отмена, single-item batch, exclusions, delete и complete.

## 9. Complex DEV result

Основной новый DEV: **120 conversations / 240 turns**, 12 категорий. Исходники
примеров не читают прежние holdouts. Финальный replay текущим кодом:

| Метрика | Результат |
|---|---:|
| End-to-end / complex scenario turns | 231/240 — 96,25% |
| Multi-turn success | 111/120 — 92,50% |
| Routing accuracy | 240/240 — 100% |
| SemanticPlan validation | 165/180 — 91,67% |
| Fast / LLM | 25% / 75% |
| Semantic calls / turn | 0,750 |
| Несанкционированные мутации | 0 |

Дополнительный reference DEV после safety repair: **20 диалогов / 40 реплик**,
**34/40 (85%)**, multi-turn **16/20**, Plan validity **35/40**, unsafe **0**.
Этот более низкий результат также включён в оценку готовности.

Остаточные ошибки основного DEV: шесть невалидных list/query комбинаций,
необоснованный done filter в поиске, один некорректный temporal payload и
неправильно понятый гипотетический запрос без DB mutation.

## 10. Fresh audits и метрики

Это разные синтетические выборки, не парное сравнение до/после. Автор один;
это новые frozen примеры, не внешнее независимое исследование реальных пользователей.

| Метрика | Первый fresh v3, до safety repair | Полный fresh v5, текущий код |
|---|---:|---:|
| Conversations / turns | 150 / 300 | 150 / 300 |
| Fast-path percentage | 33,33% | 36,67% |
| LLM percentage | 66,67% | 63,33% |
| Semantic calls / turn | 0,667 | 0,633 |
| Routing accuracy | 300/300 — 100% | 300/300 — 100% |
| SemanticPlan validation | 190/200 — 95% | 182/190 — 95,79% |
| End-to-end accuracy | 298/300 — 99,33% | 287/300 — 95,67% |
| Multi-turn success | 148/150 — 98,67% | 137/150 — 91,33% |
| Complex scenario turn accuracy | 238/240 — 99,17% | 227/240 — 94,58% |
| Несанкционированные мутации | **2 — FAIL** | **0** |

Complex cohort включает все реплики сценариев, кроме simple_chat и простых
pending-slot классов, включая их followups. SemanticPlan validation означает
provider/IR/lexical checks, а не доказательство правильности смысла.

Между полными аудитами отдельно проведён **focused fresh v4: 40 диалогов /
80 реплик**, **80/80**, multi-turn **40/40**, Plan validity **73/80 (91,25%)**,
unsafe **0**. Это проверка ambiguous/explicit/temporal references; она не
подменяет полный fresh v5.

Для каждого fresh зафиксированы до inference input/prompt/schema SHA-256,
исходники, tests, manifests зависимостей и authoring scripts. Freeze проверен
после каждого прогона. V3: 155 файлов; v4/v5: 158. Код/prompt/gold не менялись
внутри аудитов. После v3 выполнен safety repair с новым DEV; v3 не запускался
повторно и не выдаётся за успешный результат текущей версии. V2 не запускался
и его corpus не использовался для настройки.

V5 выявил 13 функциональных ошибок:

- Два отказа поиска: lexical provenance / неоднозначный разбор запроса.
- **Десять batch-переносов:** модель вернула relative_day offset=1 для
  «послезавтра». Go правильно вычислил неверно заданный смысл как 10.11 вместо
  11.11. Неверная дата была показана в preview, затем подтверждена тестовым «да».
- Один запрос о последствиях удаления ошибочно стал list и сбросил pending;
  удаления не произошло.

После этого аудита runtime, prompt, gold и критерии не корректировались.

## 11. Unsafe mutations: определение и исправление

Evaluator считает unsafe изменение/удаление ID вне ожидаемого набора или
незапрошенное создание. Поэтому `unsafe_accepted_mutations=0` в v5 **не означает**
отсутствия ошибочных значений: десять подтверждённых неверных дат отдельно
зафиксированы выше и являются блокером production.

В v3 в двух ambiguous references модель подставила ordinal 1/2 вместо
местоимения. Go принял валидный по форме, но не подтверждённый фразой выбор
и выполнил update. Это реальные unsafe ошибки в изолированной тестовой БД.

Исправление: перед grounding ordinal Go проверяет ограниченный source-evidence
whitelist для показанных позиций. Местоимение, clock number, несовпадающий
порядковый номер или поддержанные temporal-формы не позволяют выбрать запись.
При отсутствии evidence — clarification. Полная фраза всё равно сначала идёт
в ParsePlan: Go не определяет её intent/changes/date. Дополнительно fast ordinal
choice проверяет исходный snapshot кандидата.

Новые адверсариальные unit tests намеренно возвращают ordinal 1 и 2 от
модели без пользовательского выбора и проверяют отсутствие mutation. Текущий
код: основной DEV unsafe=0, reference DEV unsafe=0, focused v4 unsafe=0,
новый полный v5 unsafe=0 по описанному определению.

## 12. Latency, проверки и артефакты

Финальный v5, реальные сетевые измерения и полное время fast turn:

| Когорта | n | p50 | p95 |
|---|---:|---:|---:|
| Fast path, end-to-end | 110 | 1,657 мс | 2,860 мс |
| Network LLM inference | 163 | 1726,356 мс | 2469,288 мс |
| Все LLM turns, включая cache | 190 | 1642,254 мс | 2404,431 мс |

190 логических semantic calls; 163 сетевых ParsePlan обращения, 27 повторов
переиспользовали raw cache внутри первого run. Cache hits считаются semantic
calls, не fast path. Встроенные HTTP retries не выделены в отдельную метрику.
DEV replay latency не представляется как сетевая latency.

`go test -count=1 ./...` — PASS. `go vet ./...` — PASS.
Strict JSON / no-tools / original-message tests, temporal tests, atomic batch,
stale snapshots, cancellations, references и adversarial ordinal tests — PASS.
Проверка известных credentials: совпадений в Git-visible файлах и plan artifacts
нет. API tests и SQLite tests не заменяют отдельную проверку новой ветки на
production PostgreSQL; production специально не использовался.

Основные локальные артефакты:

- Текущий DEV replay (локальный файл `artifacts/plan/dev-post-safety.json`).
- Дополнительный reference DEV (локальный файл `artifacts/plan/reference-dev.json`).
- Оригинальный v3, первый полный run (локальный файл `artifacts/plan/heldout-v3-first.json`).
- Исходники замороженного v3 (локальный файл `artifacts/plan/v3-frozen-source.zip`).
- Focused v4, первый run (локальный файл `artifacts/plan/reference-v4-first.json`).
- Полный v5, первый run (локальный файл `artifacts/plan/heldout-v5-first.json`).
- Go tests (локальный файл `artifacts/plan/go-test-final.log`), go vet (локальный файл `artifacts/plan/go-vet-final.log`).

Protocols: [v3](../testdata/semantic/plan_heldout_v3_protocol.json),
[v4](../testdata/semantic/plan_reference_heldout_v4_protocol.json),
[v5](../testdata/semantic/plan_heldout_v5_protocol.json).

Input SHA-256 v5:
`839d841ca200bdd54a2ce4e4b831fedfd53c6b5c6e928a4face8532ae1e5c895`.

## 13. Production ready — NO

Новый runtime разделяет language understanding и безопасное исполнение,
поддерживает normalized temporal semantics и обязательные batch previews.
Найденная уязвимость ordinal закрыта и проверена на новых данных. Но осталось
13 ошибок полного fresh, включая десять неверных подтверждённых переносов,
а reference DEV дал 85%. Считать такую версию готовой к production нельзя.

Никакого deploy не выполнялось. Результат этой работы — локальный код,
проверяемые corpora, frozen audits, schema, тесты и этот отчёт.
