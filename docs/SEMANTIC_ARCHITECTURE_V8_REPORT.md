# Локальная архитектура и первый frozen v8

Полные журналы и архивы из `artifacts/plan/` сохранены локально и не входят в публикуемый репозиторий. Наборы для воспроизведения находятся в `testdata/semantic/`; актуальные публичные доказательства приведены в [отчёте об улучшениях](QUALITY5_IMPROVEMENTS.md).


**Production ready: NO. Production/VPS не использовались, deploy не выполнялся. Frozen v7 не перезапускался.**

V8: end-to-end 472/563 = 83,84%, multi-turn 121/180 = 67,22%, complex 429/519 = 82,66%, strict validity 365/385 = 94,81%. Не достигнуты все четыре quality gate. Помимо model failures выявлен false-reject в source coverage: 10 семантически правильных source/destination date pairs отклонены из-за неверного определения роли двух «на X». После holdout код не исправлялся.

Изменения проверены в локальном SemanticPlan runtime с синтетическими данными и временными SQLite. Сетевые обращения — только inference GigaChat. Второго LLM verifier нет.

## 1. DetectIntent и runtime dialogue state

Из `rememberDialogue` DetectIntent удалён полностью. После ParsePlan semantic runtime не вызывает DetectIntent, needsContext, legacy parser или Hybrid. Состояние обновляется по validated plan, фактическому Response/ActionResult, pending, result IDs и явным переходам.

В файле legacy `resolveTurn` остаются два DetectIntent для самостоятельных старых режимов. Они не достигаются после ParsePlan; совместимые старые режимы не удалялись из репозитория. Это утверждение об активном SemanticPlan пути, а не об отсутствии имени DetectIntent во всех исходниках.

## 2. State transitions

| Событие | Переход |
|---|---|
| Clarification / invalid plan / evidence rejection | Сохранить result IDs и успешный Search без продления TTL |
| Успешный search/list | Заменить result set и сохранённые query/period/filters |
| Контекстная правка date/time/title/delay | Обновить Affected, сохранить Search и ordinal-позиции |
| Correction без target | Использовать один fresh Affected; при неоднозначности/TTL/pending уточнить |
| Refinement | Применить delta к успешному Search |
| Deferred confirmation | Сохранить preview snapshot, evidence version и исходный TTL |
| Confirm | Исполнить только существующий проверенный preview |
| Cancel / expiry | Очистить соответствующий context/pending |
| Confirm без pending | «Сейчас нечего подтверждать.», без model call |

Search/reference TTL — 15 минут либо 4 unrelated chat turns; pending TTL — 30 минут. Safe clarification не расходует idle-turn TTL. Тесты проверяют шесть последовательных safe failures, независимые Search/Affected, повторную загрузку deferred preview и истечение TTL.

## 3. Source coverage omissions

40/40 намеренно неполных JSON-планов проходят strict shape/compatibility и отклоняются SOURCE → PLAN guard до preview/write. После последующего подтверждения записей также нет: **wrong writes=0**.

Те же 40 планов дополнительно проверены с реально существующими matching targets/exclusions, тремя result candidates и одним fresh Affected в достижимом состоянии после update. Результат также 40/40 PASS, wrong writes=0: отказ не зависит от отсутствующей записи в базе. Это повторная проверка тех же payloads, не 40 дополнительных уникальных атак. Seeded audit (локальный файл `artifacts/plan/omission-adversarial40-seeded.json`).

| Отсутствующее поле | Поймано |
|---|---:|
| changes.date | 5 |
| changes.time | 5 |
| changes.time_shift_minutes | 5 |
| filters.exclude | 5 |
| filters.status | 5 |
| schedule.date | 5 |
| schedule.time | 5 |
| target.ordinal | 5 |

Whitelist: relative days, HH:MM, числовые delay, ordinal, status, exclusion, shift; для явных таймеров также countdown amount. Это ограниченная lexical защита, а не универсальная проверка semantic entailment.

## 4. Correction

First-class `dialogue_act=correction`, target=null. Для context-derived target evidence в текущем сообщении не требуется. Избыточный пустой last target у correction удаляется; explicit query/ordinal не исправляются.

DEV: correction actions **30/30**; вся категория — 60/60 = 100,00%. Ambiguous, устаревший, read-only context и pending не дают correction права выбрать запись.

## 5. Refinement inheritance

Go наследует query, kind, period, status, priority, time bounds и exclusions. Возвращённая моделью delta меняет только присутствующие поля. Поиск и last affected хранятся раздельно.

DEV: реальные refinement turns **43/50**, вся цепочка — 68/75 = 90,67%. Оставшиеся отказы включают невалидные повторённые model temporal payloads; их Go не чинит угадыванием даты.

## 6. Timer duration

Explicit timer + duration при delay=null/date=null/time=null/recurrence=null канонизируется в countdown delay. Source-aware normalization выполняется до compatibility validation, в том числе для двухминутного таймера. Meeting duration сохраняется. Существующий неверный delay, например value=0, не переписывается duration-значением.

DEV create timer: **19/20**; категория со вторым ходом — 39/40 = 97,50%.

## 7. Semantic canonicalization

Допустимы: empty search payloads; sole complete + status=done; redundant correction last; неисполняемый payload ambiguous/unknown; explicit timer duration. Wrong kind/date/query/target/exclusion не исправляются. Неверные обязательные evidence и положительные дубликаты блокируют mutation. Явно отрицательная relative-day альтернатива не считается вторым supporting evidence.

Счётчик evaluator: DEV **19**, frozen v8 **2**. DEV canonicalization category — 40/40 = 100,00%; negated alternatives — 40/40 = 100,00%.

## 8. Strict validity и DEV

Strict validity измеряется **после safe canonicalization**, отдельно от правильности действия и значения.

| Метрика | DEV |
|---|---:|
| End-to-end | 466/485 = 96,08% |
| Multi-turn | 182/195 = 93,33% |
| Strict plan validity | 364/375 = 97,07% |
| Reference DEV | 38/40 = 95,00% |
| Новый context preservation DEV, полные цепочки | 79/90 = 87,78% |
| Source coverage DEV | 60/60 = 100,00% |
| Deferred confirmation DEV | 80/80 = 100,00% |

DEV: 195 диалогов / 485 ходов; итоговая проверка — offline raw-response replay, cache misses=0. Interrupted DEV attempts и ответы с API throttling сохранены отдельно и не подменяют итоговый DEV. Frozen v7 не использовался для tuning.

## 9. Adversarial 68 + omission corpus

Исходные 48 и дополнительные 20 adversarial cases: **68/68 PASS**, mutation_count=0. Новый omission corpus: **40/40 PASS**, wrong_writes=0. Проверяются реальный Service, SQLite, preview/pending и последующее подтверждение. Ordinal protection сохранена; direct LLM execution не вводился.

## 10. Fresh v8

**180 диалогов / 563 ходов**; 18 семейств по 10 диалогов. Длины: {2: 43, 3: 83, 4: 42, 5: 12}. Включены create, corrections, references, historical search, multi-step refinements, timers/meeting duration, batch, exclusions, confirmation/defer/cancel, ambiguity, chat и omission-pressure requests. Все ходы идут через настоящий model/planner runtime; намеренно испорченные JSON находятся в отдельном 40-case корпусе.

Freeze: **17.10.2026 07:39:21 МСК**, model `GigaChat-3-Ultra`, Go `go1.27.2`. Заморожены corpus, prompt, schema и 183 файлов. До и после inference совпали current source и архив; v7 corpus/archive intact. Единственный полный first run, без holdout replay/tuning.

- Corpus SHA-256: `cd4b2cd1202de9196140ea6462791c07a808fe9329b34ea0d1f2a016b2ed376d`
- Prompt SHA-256: `ec807395b56f388fc9fc4a6ea049a5954f70539b7356de1080066f180655d956`
- Schema SHA-256: `2dd5906a34f98620f022f2532e4acc043ed17ff6ab18f2813d516f21ff3b6db1`

| Метрика v8 | Результат |
|---|---:|
| End-to-end | 472/563 = 83,84% |
| Multi-turn | 121/180 = 67,22% |
| Complex | 429/519 = 82,66% |
| Strict validity | 365/385 = 94,81% |

| Категория v8 | Ходы PASS |
|---|---:|
| ambiguous_reference | 31/34 = 91,18% |
| batch_complete | 24/24 = 100,00% |
| batch_delete | 15/30 = 50,00% |
| batch_exclusion | 10/30 = 33,33% |
| batch_shift | 18/24 = 75,00% |
| correction_chain | 34/44 = 77,27% |
| deferred_confirmation | 40/40 = 100,00% |
| historical_search | 34/34 = 100,00% |
| negated_alternative | 14/24 = 58,33% |
| omission_filter_pressure | 28/30 = 93,33% |
| omission_schedule_pressure | 13/24 = 54,17% |
| reference_chain | 34/34 = 100,00% |
| refinement_chain | 37/44 = 84,09% |
| scheduled_create | 30/30 = 100,00% |
| simple_chat | 19/20 = 95,00% |
| simple_create | 24/24 = 100,00% |
| slot_then_correction | 44/44 = 100,00% |
| timer_semantics | 23/29 = 79,31% |

## 11. Unsafe / wrong-value

| Корпус | Unsafe | Wrong-value | Confirmed wrong-value |
|---|---:|---:|---:|
| DEV | 0 | 0 | 0 |
| First frozen v8 | 0 | 0 | 0 |

Safe refusal остаётся accuracy failure, если gold требует правильного исполнения. В v8 guard rejections=30; счётчик evaluator `evidence_value_mismatches`=10 не означает столько предотвращённых wrong-value writes. Все 10 verdicts changes.date:value_mismatch относятся к source/destination false rejects: model filters.period=завтра и changes.date=послезавтра правильны, а guard ошибочно считает исходное «на завтра» назначением changes.date. Это дефект precision guard, сохранённый в frozen результате. Отдельно пять полностью пропущенных schedule.date действительно пойманы reverse coverage.

Каждый из 91 неуспешных ходов учтён ровно один раз в диагностике места отказа:

| Место отказа / наблюдаемый класс | Ходы |
|---|---:|
| dialogue_action_or_pending_mismatch | 19 |
| safe_clarification | 17 |
| contract_or_transport:invalid normalized temporal meaning | 10 |
| guard:changes.date:value_mismatch | 10 |
| contract_or_transport:empty exclusion | 8 |
| guard:schedule.date:source_constraint_omitted | 5 |
| guard:filters.period:missing | 5 |
| result_or_context_mismatch | 5 |
| guard:changes.time_shift_minutes:unsupported_shift | 4 |
| guard:target.reference:not_in_original | 3 |
| guard:changes.time:missing | 1 |
| guard:filters.exclude.0.period:missing | 1 |
| guard:target.reference:missing | 1 |
| contract_or_transport:incompatible semantic plan | 1 |
| contract_or_transport:incomplete or non-assistant GigaChat response | 1 |

Это классификация наблюдаемого отказа, не доказательство независимых root causes. Ошибка первоначального поиска может вызвать последующие безопасные отказы в том же диалоге.

## 12. Latency

LLM network p50/p95: **1965.698 / 3162.414 ms**, n=363. Logical ParsePlan calls=385; network requests=363; fast paths=31.62%; verifier calls=0.

Transport retry разрешён только для 429/502/503/transport failure; ни один полученный semantic plan не перегенерируется для улучшения результата. Идентичные model requests могут использовать raw cache; число диалогов не выдаётся за число независимых network inferences. Offline DEV latency не используется как оценка network latency.

## 13. Production gate

| Условие | Итог |
|---|---|
| unsafe | PASS |
| wrong_value | PASS |
| confirmed_wrong_value | PASS |
| end_to_end | FAIL |
| multi_turn | FAIL |
| complex | FAIL |
| strict_plan_validity | FAIL |
| reference_dev | PASS |
| omissions | PASS |
| adversarial68 | PASS |
| go_test | PASS |
| go_vet | PASS |

Пороговые значения: unsafe/wrong-value/confirmed wrong-value=0; end-to-end≥97%; multi-turn/complex≥95%; strict validity≥97%; reference DEV≥95%; omission wrong writes=0; все 68 adversarial PASS; go test ./... и go vet ./... PASS.

**Production ready: NO. Deploy не выполнялся.**

Артефакты: raw v8 (локальный файл `artifacts/plan/heldout-v8-first.json`), [freeze protocol](../testdata/semantic/plan_heldout_v8_protocol.json), freeze verification (локальный файл `artifacts/plan/v8-freeze-verification.json`), machine summary (локальный файл `artifacts/plan/architecture-v8-summary.json`), failure inventory (локальный файл `artifacts/plan/architecture-v8-failures.json`), DEV (локальный файл `artifacts/plan/architecture-dev-verified.json`), omissions (локальный файл `artifacts/plan/omission-adversarial40.json`), [state machine](SEMANTIC_STATE_MACHINE.md).
