# Локальная архитектура после v7

Semantic runtime: exact whitelist routing → ParsePlan → SemanticCanonicalizer → strict compatibility → source coverage / evidence → grounding → execution → state transition. Повторный DetectIntent, needsContext, Hybrid и full-command parser на этом пути не вызываются. Старые самостоятельные NLP режимы сохранены для совместимости; rememberDialogue больше не анализирует текст.

## Состояние

- `LastIDs` — позиции последнего показанного результата.
- `Dialogue.Search` — отдельный успешный поиск: query, kind, filters, period, IDs и timestamp.
- `Dialogue.AffectedIDs/AffectedAt` — фактически изменённые/созданные записи и время результата.
- `Pending` — неизменяемый preview snapshot с evidence version и исходным TTL. `ConfirmationState`: pending_confirmation / deferred_confirmation.

| Результат | Переход |
|---|---|
| Успешный новый search/list, включая пустой | Заменить IDs и Search, установить timestamp |
| Clarification, invalid model plan, evidence rejection | Сохранить последний результат/поиск без продления TTL |
| Контекстное изменение только title/date/time/delay | Обновить Affected; сохранить Search и ordinal-позиции |
| Независимая успешно выполненная команда | Заменить контекст фактическим результатом |
| Correction без explicit target | Единственный fresh Affected + отсутствие pending; иначе clarification |
| Refinement | Применить delta к последнему успешному Search |
| Deferred confirmation | Сохранить preview, IDs, evidence version и исходное время preview |
| Точное подтверждение | Выполнить только проверенный существующий preview |
| Точная отмена | Удалить pending и ссылочный контекст |
| Подтверждение без pending | «Сейчас нечего подтверждать.», без вызова модели |
| TTL | Search/reference: 15 минут или 4 unrelated chat turns; pending: 30 минут |

## Проверки источника

Source coverage — закрытый лексический whitelist, а не второй parser: relative days, HH:MM, числовая relative delay, ordinal 1–8, явные статусы, exclusion и shift. Create/mutation с отсутствующим critical field отклоняются целиком. Quoted titles не считаются schedule instructions. Простые `не X` / `не на X` исключают отрицательную temporal alternative.

Model evidence и Go anchors разделены. Однозначные source anchors могут заменить отсутствующую цитату. Присланные неверные, дублированные или выдуманные обязательные цитаты по-прежнему блокируют mutation. Non-whitelist semantics требуют model evidence и grounding. Context-derived target не нуждается в цитате текущего сообщения.

Канонизация удаляет только семантически неактивные данные: пустые search payloads; sole changes.status=done при complete; избыточный last target у correction; неисполняемый payload при ambiguous/unknown. Explicit timer + duration становится countdown delay; meeting duration сохраняется. Wrong kind/date/query/target/exclusion не исправляются.

Это ограниченная защита, а не доказательство полного semantic entailment. Качество модели и production gate измеряются отдельно на DEV и независимом frozen v8. Второго LLM verifier нет. Production/VPS не используется.
