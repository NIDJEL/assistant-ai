# Гена: локальный simple_llm, 17 октября 2026

**Ready for manual product test: YES (локально).** Production/VPS, SSH, production env и базы не затрагивались. Новых fresh/holdout/evidence layers нет.

| Проверка | Результат |
| --- | --- |
| Архитектура | `text/voice → compact context → GigaChat-3-Ultra → small intent/arguments → Go validation/calendar/target resolution → existing Service/repository` |
| Обойдённый runtime | `SemanticPlan`, canonicalizer, evidence/provenance, source/reverse coverage и legacy Russian parser остаются в legacy, но не вызываются из `simple_llm` |
| Operation contracts | create task/meeting/reminder; search/list; update/complete/delete; batch update/complete/delete; chat/clarify; continue/select/confirm/cancel. Каждый имеет отдельный небольшой DTO |
| Контекст | now/timezone, последняя успешная операция, до 8 результатов с индексами, последняя изменённая запись без индекса, последние search filters, pending с missing fields/кандидатами/preview. DB IDs и история модели не передаются |
| Safety | Строгие типы и неизвестные поля; low confidence → clarify; проверка обязательных полей; user-scoped repository; неоднозначность → выбор; idempotency; транзакции и rollback; проверка snapshot диалога и конфликтов |
| Batch/удаление | Обязательные preview + отдельный confirm; исключения применяются в Go. Snapshot не расширяется новыми записями; изменившиеся записи/конфликты отклоняют confirm |
| Маленький realistic smoke | **35/35 сценариев**, **61 реплика**, **52 вызова живого GigaChat**. Fixture execution также 35/35 |
| Дополнительный UX-диалог | **27 разговорных реплик** через интерактивный CLI; последний последовательный прогон без transport/JSON errors, ответы проверены |
| Средняя latency live smoke | **552 мс** на реплику; **648 мс** для реплик с обращением к модели. Это end-to-end время, включая Go/SQLite. Дополнительный UX-диалог: **708 мс** на реплику |
| Обязательные команды | `go test -count=1 ./...` — PASS; `go vet ./...` — PASS; `git diff --check` — PASS |
| PostgreSQL | Интеграционные тесты SKIP: `TEST_DATABASE_URL` отсутствует, локальный Docker Engine не запущен. Новый тест isolation + durable confirmation добавлен для отдельной loopback `*_test` БД; PostgreSQL live-pass не заявляется |

Контракты и детали: [SIMPLE_LLM_RUNTIME.md](SIMPLE_LLM_RUNTIME.md). Сам корпус: [smoke.json](../testdata/simple_llm/smoke.json).

## Реальные разговорные примеры

В дополнительном диалоге использовались живые ответы модели, без подстановки fixture operations:

| Реплика | Наблюдавшееся поведение |
| --- | --- |
| «Ту, которая вторая, сдвинь на час позже» | Вторая встреча с Алисой перенесена с 16:00 на 17:00 |
| «Не, давай всё-таки в 18:00» | Та же запись перенесена на 18:00, дата сохранена |
| «И предупреди о ней за час» | У той же встречи установлен lead 60 минут |
| «Эту встречу удали» → «Нет» | Подтверждение показало именно встречу в 18:00; после отмены запись осталась |
| «А только встречи?» | Сохранён период «завтра», из результата убраны task/reminder |
| «Теперь только выполненные» | Сохранён kind task, найден завершённый документный task |
| «Все встречи завтра сдвинь на полчаса, кроме встречи с Матвеем» | Preview двух встреч с Алисой, Матвей исключён; изменение только после «Подтверждаю» |
| «А лучше через полчаса» после таймера | Время последнего напоминания пересчитано от текущего времени |
| «Создай встречу с Олей завтра» → «В 11:00» | Один ожидаемый вопрос о времени; затем создание |
| «Нет, всё-таки послезавтра» | Последняя встреча перенесена на послезавтра, время 11:00 сохранено |

Это проверка текстового UX через локальный CLI: реплики переданы через stdin и ответы просмотрены. Человеческий тест в браузере и проверка микрофона на устройстве ещё предстоят.

## Что исправлено по наблюдениям

Первый live smoke дал 31/35: модель спутала час с числом месяца, повторённая операция с ordinal не выбрала pending-кандидата, один ответ нарушил JSON contract. Исправлены prompt и передача pending-контекста; Go стал принимать ordinal и в повторённой исходной операции, проверяя snapshot кандидата.

В свободном диалоге выявлен лишний `index:1` у `last_affected`: модель путала его с первой строкой результата. Индекс убран, местоименная ссылка после изменения явно обозначена как `target:last`. При повторе подтверждение показало правильную запись. Новый parser/lexical guard для этих фраз не добавлялся.

Параллельный запуск двух live CLI превысил лимит GigaChat и дал 429. Прогоны завершены последовательно. Отдельно встречался неизвестный `title` вместо `query` в exclusion: строгий decoder отклонил ответ без мутации; контракт фильтров уточнён в prompt. Финальный smoke и дополнительный диалог выполнены без этих ошибок. Успех маленького smoke не является гарантией понимания любой речи и не заменяет продуктовую оценку.

## Что проверяет Go без сети

Тесты покрывают создание без обязательных полей, пропущенный моделью очевидный срок, invalid JSON/поля/даты/часы/duration/reminder, запрет DB IDs, low confidence, ordinal/corrections, сохранение search filters, неоднозначность, подтверждение/отмену, рестарт перед batch confirm, stale deletion/conflict/batch preview, изменение диалога во время inference, идемпотентность, rollback мутации вместе со state и атомарный rollback при ошибке второй записи batch. Существующие auth/calendar/reminders/repository/legacy suites проходят; PostgreSQL-зависимые проверки пропущены как указано выше.

Локальные артефакты (каталог `artifacts/` исключён из Git):

- `artifacts/simple_llm_smoke_live.json` — последний полный live smoke;
- `artifacts/simple_llm_smoke_fixture.json` — проверка исполнения без сети;
- `artifacts/simple_llm_ux_dialogue.json` — последний диалог из 27 реплик;
- `artifacts/simple_llm_ux_dialogue_initial.json` и `*_rate_limited.json` — диагностические предыдущие прогоны.

## Ручной продуктовый тест

```powershell
go run ./cmd/simple-smoke -interactive -output artifacts/simple_llm_manual.json
```

Команда использует локальные GigaChat credentials и одноразовую SQLite-БД. Записи приложения не открываются. Завершение — `/quit`; transcript сохраняется в указанный файл. Для запуска обычного локального сервера: `GENA_NLU_MODE=simple_llm`, `GENA_SEMANTIC_PROVIDER=gigachat`, затем `go run ./cmd/server` с вашей локальной конфигурацией приложения.
