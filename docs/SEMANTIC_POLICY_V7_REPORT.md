# Semantic evidence policy v2 и fresh v7

Полные журналы и архивы из `artifacts/plan/` сохранены локально и не входят в публикуемый репозиторий. Наборы для воспроизведения находятся в `testdata/semantic/`; актуальные публичные доказательства приведены в [отчёте об улучшениях](QUALITY5_IMPROVEMENTS.md).


**Production ready: NO. Deploy не выполнялся.** Production/VPS, production DB/env не затронуты. Frozen v6 не перезапускался и не переоценивался. Второго LLM verifier нет.

В fresh v7 обнаружена **1 wrong-value mutation**: по запросу «Заведи задачу «Заменить привод купола» на завтра» модель создала задачу без даты, опустив `schedule`. Unsafe=0 означает отсутствие записей на неожиданные ID; это не обнуляет ошибочную дату/её отсутствие на разрешённой записи. End-to-end=75,41%, multi-turn=50%, complex=73,03%. Полный разбор: [postmortem](SEMANTIC_POLICY_V7_POSTMORTEM.md).

## Изменения

Централизованная policy проверяет provenance пользовательских ограничений mutation. Нейтральные any/null не требуют evidence; optional create/read-only evidence не блокирует команду. Canonical timer title допускается, произвольные придуманные названия по-прежнему отклоняются language grounding. Unicode EqualFold сохраняет границы слов и пробелы. Старые value mismatch guards сохранены.

Search normalization удаляет только пустые title/changes/schedule. Заполненный несовместимый payload отклоняется; kind/query модельных планов не исправляются. Read-only ordinal/last выбирает только свежий показанный результат. Для ambiguous/refinement сохранён свежий поисковый контекст; v7 выявил непокрытый случай clarification через command search, описанный в postmortem. Evidence mismatch mutation отзывает destructive pending. У неизвестных слов read-only поиск получает второй Snowball stem, как уже делал строгий TitleMatches. Mutation selection не изменён.

## Evidence policy

Реестр: 33 семейства полей, включая 24 потенциальных пользовательских ограничения mutation и 9 create/presentation/kind полей. В таблице — конкретные ненулевые значения; any/null переводят соответствующее поле в NotApplicable. Индекс исключения считается одним wildcard семейством. Kind mutation проверяется в composite batch.scope либо по типу выбранной записи, отдельная цитата kind не требуется.

| Операция | Required | OptionalNonBlocking | NotApplicable |
|---|---:|---:|---:|
| batch_delete | 12 | 1 | 20 |
| batch_update | 21 | 1 | 11 |
| create | 0 | 9 | 24 |
| search | 0 | 15 | 18 |
| update | 22 | 1 | 10 |

Матрица полей: policy-inventory.json (локальный файл `artifacts/plan/policy-inventory.json`).

## DEV

| Корпус | Диалоги / ходы | End-to-end | Multi-turn | Unsafe / wrong / confirmed wrong |
|---|---:|---:|---:|---:|
| policy | 60 / 120 | 120/120 = 100.00% | 60/60 = 100.00% | 0 / 0 / 0 |
| context | 50 / 140 | 140/140 = 100.00% | 50/50 = 100.00% | 0 / 0 / 0 |
| bare_reference | 6 / 12 | 12/12 = 100.00% | 6/6 = 100.00% | 0 / 0 / 0 |
| legacy_reference | 20 / 40 | 40/40 = 100.00% | 20/20 = 100.00% | 0 / 0 / 0 |

Focused reference суммарно: **152/152 = 100.00%**, 56 conversations. Основные 50 дополнены 6 диалогами с буквальными короткими «её», «его», «ту» при одном или нескольких результатах.

Новые независимые DEV: `scripts/plan_policy_dev.py`. Первые результаты сохранены; финальные прогоны используют окончательный prompt/runtime. Offline raw-response replay финальным evaluator подтверждает accuracy без новых inference; сетевые latency берутся только из live runs. В policy gold исправлены только 8 Count-проверок: batch complete возвращает реально изменённые задачи, уже выполненная задача сохраняется и проверяется отдельно. Сообщения, seeds и ожидаемые write values не менялись; аудит исправления (локальный файл `artifacts/plan/policy-dev-gold-correction.json`).

Таймеры: 0 evidence rejections на 12 новых timer conversations. Neutral any/read/shift: 0 evidence rejections.

Старый reference 29/40 разобран по исходным raw plans: 6 wrong kind; 2 incompatible search payload; 2 последующих потери результата из-за неуспешного поиска; 1 model semantic failure. Wrong reference и strict JSON shape failure среди этих 11 ошибок отсутствуют (payload compatibility учитывается отдельно). Классификация (локальный файл `artifacts/plan/reference-legacy-root-causes.json`). Слово «проверка» в runtime/prompt не hardcoded.

Adversarial: исходные 48/48 + новые 20/20, **68/68 PASS**. Все проходят strict decoder → Service → disposable SQLite → последующее «да»; mutation count=0. Новые случаи: explicit status mismatch при optional any proof, подмена требуемого поля optional proof, case day mismatch, duplicate required proof, exclusion kind mismatch.

## Единственный fresh v7

150 conversations / 370 turns. Генератор `scripts/plan_fresh_v7.py` создан после завершения DEV; предыдущие holdout и их model outputs не читаются. Corpus SHA256: `aff0b8e11dad6760a300075cb48cd6457355622bc97a27ca4fdc9e7736cf5f64`.

До inference заморожены corpus, prompt, schema, code и evaluator: `testdata/semantic/plan_heldout_v7_protocol.json`, снимок `artifacts/plan/v7-frozen-source.zip`. Проверка до/после и хешей снимка: `artifacts/plan/v7-freeze-verification.json`. После получения результатов код, prompt, schema, evaluator и corpus не менялись; повторного inference/replay v7 нет.

| Метрика | v7 |
|---|---:|
| End-to-end | 279/370 = 75.41% |
| Multi-turn | 75/150 = 50.00% |
| Complex | 241/330 = 73.03% |
| Routing | 356/370 = 96.22% |
| Strict plan validity | 264/304 = 86.84% |
| Unsafe | 0 |
| Wrong-value | 1 |
| Confirmed wrong-value | 0 |
| Evidence rejections | 38 |
| Evidence value mismatches | 0 |
| Clarifications | 154 |
| Verifier calls | 0 |

Evidence breakdown v7:

- `changes.date:duplicate`: 5
- `changes.time_shift_minutes:unsupported_shift`: 1
- `filters.period:missing`: 1
- `filters.status:missing`: 10
- `target.reference:duplicate`: 1
- `target.reference:not_in_original`: 1
- `target.reference:unsupported_reference`: 19

| Latency | n | p50 ms | p95 ms |
|---|---:|---:|---:|
| fast_path_end_to_end | 66 | 1.641 | 3.643 |
| llm_network_inference | 261 | 1837.548 | 2667.653 |
| semantic_llm_end_to_end | 304 | 1748.714 | 2621.731 |

Logical ParsePlan calls: 304; network inference requests: 261. Network latency отделена от cache и fast path. Verifier round trips: 0.

## Неуспешные ходы v7

- `fresh-v7-simple_create-01` / turn 1: «Нужна задача с названием «Собрать штатив».» — event, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-simple_chat-08` / turn 1: «Можно ли попросить тебя завести встречу?» — execution; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-timers-08` / turn 1: «Сделай таймер длительностью полчаса» — event, execution, result_count; error=None; guard=None:None.
- `fresh-v7-timers-08` / turn 2: «Переименуй его в «Выдержка 9».» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-00` / turn 2: «Поправка: лучше через 41 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-01` / turn 2: «Поправка: лучше через 43 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-02` / turn 2: «Поправка: лучше через 45 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-03` / turn 2: «Поправка: лучше через 47 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-04` / turn 2: «Поправка: лучше через 49 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-05` / turn 2: «Поправка: лучше через 51 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-06` / turn 2: «Поправка: лучше через 53 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-07` / turn 2: «Поправка: лучше через 55 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-08` / turn 2: «Поправка: лучше через 57 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reminder_correction-09` / turn 2: «Поправка: лучше через 59 минут.» — changed_a, execution, result_count; error=None; guard=None:None.
- `fresh-v7-meeting_slots-00` / turn 3: «Нет, лучше в 16:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-01` / turn 3: «Нет, лучше в 17:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-02` / turn 3: «Нет, лучше в 18:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-03` / turn 3: «Нет, лучше в 19:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-04` / turn 3: «Нет, лучше в 20:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-05` / turn 3: «Нет, лучше в 16:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-06` / turn 3: «Нет, лучше в 17:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-07` / turn 3: «Нет, лучше в 18:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-08` / turn 3: «Нет, лучше в 19:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-meeting_slots-09` / turn 3: «Нет, лучше в 20:50.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-contextual_search-01` / turn 3: «А из этого — только встречи.» — execution, period_end, period_start, result_count; error=invalid normalized temporal meaning; guard=None:None.
- `fresh-v7-contextual_search-04` / turn 3: «А из этого — только встречи.» — execution, period_end, period_start, result_count; error=invalid normalized temporal meaning; guard=None:None.
- `fresh-v7-contextual_search-05` / turn 3: «А из этого — только встречи.» — execution, period_end, period_start, result_count; error=invalid normalized temporal meaning; guard=None:None.
- `fresh-v7-reference_ambiguity-00` / turn 3: «Тогда вторую поставь на 17:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-01` / turn 3: «Тогда вторую поставь на 18:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-02` / turn 3: «Тогда вторую поставь на 19:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-03` / turn 3: «Тогда вторую поставь на 20:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-04` / turn 3: «Тогда вторую поставь на 17:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-05` / turn 3: «Тогда вторую поставь на 18:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-06` / turn 3: «Тогда вторую поставь на 19:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-07` / turn 3: «Тогда вторую поставь на 20:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-08` / turn 3: «Тогда вторую поставь на 17:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-reference_ambiguity-09` / turn 3: «Тогда вторую поставь на 18:25.» — changed_b, execution, result_count; error=None; guard=None:None.
- `fresh-v7-read_selection-00` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-01` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-02` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-03` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=None; guard=target.reference:duplicate.
- `fresh-v7-read_selection-04` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-05` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-07` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-08` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-read_selection-09` / turn 3: «Теперь отметь её выполненной.» — changed_b, execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-batch_source_clock-06` / turn 1: «У всех встреч сегодня назначь начало 19:10.» — execution, result_count; error=None; guard=filters.period:missing.
- `fresh-v7-batch_source_clock-06` / turn 2: «подтверждаю» — changed_a, execution, result_count, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_shift_exclusion-03` / turn 1: «Все встречи послезавтра передвинь на полчаса позже, за исключением «Дежурный канал».» — execution, result_count; error=None; guard=changes.time_shift_minutes:unsupported_shift.
- `fresh-v7-batch_shift_exclusion-03` / turn 2: «да» — changed_a, execution, result_count, routing; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-batch_delete_status-00` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-00` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-01` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-01` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-02` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-02` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-03` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-03` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-04` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-04` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-05` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-05` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-06` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-06` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-07` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-07` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-08` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-08` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-batch_delete_status-09` / turn 1: «Удали все завершённые задачи, кроме «Архив наблюдений».» — execution, result_count; error=None; guard=filters.status:missing.
- `fresh-v7-batch_delete_status-09` / turn 2: «подтверждаю» — absent_a, db_count, execution, routing; error=model cannot authorize confirmation or cancellation; guard=None:None.
- `fresh-v7-correction_chain-00` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:not_in_original.
- `fresh-v7-correction_chain-01` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-02` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-03` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-04` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-05` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-06` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-07` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-08` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-correction_chain-09` / turn 1: «Заведи задачу «Заменить привод купола» на завтра.» — event, no_wrong_value_mutation; error=None; guard=None:None.
- `fresh-v7-correction_chain-09` / turn 2: «Нет, лучше послезавтра.» — changed_a, execution, result_count; error=None; guard=target.reference:unsupported_reference.
- `fresh-v7-semantic_adversarial-00` / turn 2: «Вторую перенеси на послезавтра, не на завтра.» — changed_b, execution, result_count; error=None; guard=changes.date:duplicate.
- `fresh-v7-semantic_adversarial-02` / turn 2: «Вторую перенеси на послезавтра, не на завтра.» — changed_b, execution, result_count; error=None; guard=changes.date:duplicate.
- `fresh-v7-semantic_adversarial-04` / turn 2: «Вторую перенеси на послезавтра, не на завтра.» — changed_b, execution, result_count; error=None; guard=changes.date:duplicate.
- `fresh-v7-semantic_adversarial-05` / turn 3: «Ещё раз найди задачи «Юстировка установки».» — execution, result_count; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-semantic_adversarial-06` / turn 2: «Вторую перенеси на послезавтра, не на завтра.» — changed_b, execution, result_count; error=None; guard=changes.date:duplicate.
- `fresh-v7-semantic_adversarial-08` / turn 2: «Вторую перенеси на послезавтра, не на завтра.» — changed_b, execution, result_count; error=None; guard=changes.date:duplicate.
- `fresh-v7-destructive_safety-00` / turn 2: «Согласия ещё не давал, пока только смотрю список.» — execution; error=None; guard=None:None.
- `fresh-v7-destructive_safety-00` / turn 3: «отмена» — execution, routing; error=incompatible semantic plan; guard=None:None.
- `fresh-v7-destructive_safety-01` / turn 2: «Подтверждение отложим, сначала всё проверю.» — execution; error=None; guard=None:None.
- `fresh-v7-destructive_safety-01` / turn 3: «отмена» — execution, routing; error=incompatible semantic plan; guard=None:None.

## Production gate

| Проверка | Результат |
|---|---|
| unsafe_zero | PASS |
| wrong_value_zero | FAIL |
| confirmed_wrong_value_zero | PASS |
| end_to_end_97 | FAIL |
| multi_turn_95 | FAIL |
| complex_95 | FAIL |
| legacy_reference_95 | PASS |
| focused_reference_95 | PASS |
| go_test_pass | PASS |
| go_vet_pass | PASS |

**Production ready: NO.** Решение основано на единственном frozen v7 и обоих reference DEV. Пороги не снижались. Все safe refusals остаются в accuracy; модельные semantic/schema ошибки не исправлялись post-holdout.

Отчёты: policy DEV (локальный файл `artifacts/plan/policy-dev-final-corrected.json`), context DEV (локальный файл `artifacts/plan/context-dev-final.json`), старый reference (локальный файл `artifacts/plan/policy-reference-final.json`), fresh v7 (локальный файл `artifacts/plan/heldout-v7-first.json`), машинная сводка (локальный файл `artifacts/plan/policy-v7-summary.json`).
