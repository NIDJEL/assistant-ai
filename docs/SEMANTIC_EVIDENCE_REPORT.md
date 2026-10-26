# Semantic evidence: DEV и первый frozen v6

**Production ready: NO. Deploy не выполнялся. Production/VPS, production env и production DB не затрагивались.** Архитектура сохранена. V5 не перезапускался и не переоценивался; его 10 подтверждённых wrong-date записей остаются в [историческом отчёте](SEMANTIC_PLAN_REPORT.md).

## Реализация

В SemanticPlan добавлен `evidence: [{field,text}]`. Текст копируется из ORIGINAL current message дословно; проверяются регистр, пробелы и границы слов. `завтра` внутри `послезавтра` не принимается. Для update/delete/complete/batch_* нужны evidence для заполненных changes, target, filters, query; для batch также `batch.scope`. Исключения проверяются по каждому заполненному полю. Схема остаётся strict, с required nullable полями и additionalProperties=false.

Маленькая Go whitelist проверяет пять relative-day слов, явные delays, weekdays/direction, ordinal references, shifts и числовое время. Проверка выполняется после strict DTO validation, до grounding/temporal arithmetic и **до batch preview**. Source filter и destination имеют разные пути evidence; одно вхождение нельзя использовать для обеих ролей. При противоречии выбрана разрешённая в запросе clarification, без автоматической записи исправленного значения.

Evidence rejection очищает старый pending. Следующее «да» не исполняет предыдущий preview. Проверенные pending получают `evidence_version=1`; старые неподтверждённые snapshots требуют повторить исходную команду. Проверены restart, сохранённое состояние и наследование отметки точными slot/choice fast paths. Модель не получила ID, tools или право подтверждать mutation.

Второго LLM verifier нет: **0 calls**, дополнительного сетевого round trip нет. Вне whitelist полное понимание языка остаётся за GigaChat; для calendar_period/calendar_date guard проверяет provenance, а не доказывает произвольное семантическое entailment. Create/read-only не требуют evidence, но ошибочно присланное evidence всё равно проверяется — это стало причиной части ложных отказов v6.

Подробности: [контракт](SEMANTIC_EVIDENCE.md), [реальные DEV plan → Go command примеры](semantic_evidence_examples.json), [schema v1](../testdata/semantic/semantic_plan_evidence_v1.schema.json).

## Проверки без LLM

`go test ./...` — **PASS**; `go vet ./...` — **PASS**. Логи: `artifacts/plan/evidence-go-test.log`, `artifacts/plan/evidence-go-vet.log`. `git diff --check` — PASS (только существующее предупреждение CRLF README).

48 adversarial model-plan cases в 8 группах проходят настоящий strict JSON decoder, затем Service и временную SQLite. Проверены неверные day destination, source day filter, ordinal, shift, exclusion query, delay, clock и weekday. Все первые ответы — clarification без preview; после дополнительного «да» записи тоже не меняются. **Mutation count = 0 во всех 48 случаях.** Машинный результат: `artifacts/plan/evidence-adversarial.json`. В этих случаях guard вернул 48 `value_mismatch`.

Дополнительные тесты покрывают missing/not-in-original evidence, слово внутри другого слова, пропущенное исключение, отзыв предыдущего preview, старый persisted pending, temporal ordinal и отличие delay от record shift.

Все четыре обязательные регрессии прошли с проверкой preview и итоговой БД:

- «Перенеси все встречи послезавтра на день позже»: source +2, shift +1440 минут от каждой записи.
- «все встречи послезавтра перенеси на 18:00»: source +2, новое время 18:00.
- «послезавтра все встречи сдвинь на час»: source +2, shift +60 минут.
- «перенеси задачи с послезавтра на пятницу»: source +2, destination weekday friday.

## Новый DEV

Новые 120 conversations / 240 turns, 8 категорий по 15 разговоров. Автор: `scripts/plan_evidence_corpus.py`; corpus: `testdata/semantic/plan_evidence_dev.jsonl`. SHA256 `a10101dcf32d139fd5fd7d2b21b6523a33a420b16c37f70212cdd0b7c73e9b9c`. Старые holdout и outputs генератор не читает.

| Метрика | Результат |
|---|---:|
| End-to-end | 236/240 = **98,33%** |
| Multi-turn | 116/120 = **96,67%** |
| Complex (весь этот DEV) | 236/240 = **98,33%** |
| Новая reference категория | 30/30 ходов = **100%**, 15/15 диалогов |
| Routing | 240/240 = 100% |
| Strict plan validity | 118/135 = 87,41% |
| Unsafe / wrong-value / confirmed wrong-value | **0 / 0 / 0** |
| Evidence guard rejections / value mismatches | **2 / 2** |
| Clarification turns всего | **49** |
| Verifier calls | **0** |

Четыре ошибочных хода безопасны: в двух модель использовала `offset` вместо `value` у relative_delay (strict validation); в двух «на день раньше» ошибочно превратилось в relative_day от текущего дня (evidence guard). Правильная семантика — сдвиг от исходной записи. 49 clarification включают 30 batch preview, 15 намеренных ambiguity и 4 отказа; два из отказов принадлежат guard.

Live DEV: 123 сетевых provider requests / 135 logical ParsePlan. Network latency p50/p95 **2331,321 / 3235,502 ms**. Результат первого live прогона сохранён в `artifacts/plan/evidence-dev-first.json`. Финальная версия повторно проверена **offline raw-response replay**, без новых inference: `artifacts/plan/evidence-dev-final.json`; показатели accuracy не изменились. Replay latency не используется как LLM latency.

Отдельно перепроверен прежний reference DEV (разрешённый DEV, не holdout): **29/40 = 72,5%**, multi-turn **12/20 = 60%**, unsafe/wrong-value **0/0**, guard rejections **3**, clarification **13**. Остаются ошибки kind при поиске «проверок», несовместимые search payload и ошибки контекста. Поэтому новая reference категория 100% не заменяет обязательный reference gate. Live report: `artifacts/plan/evidence-reference-dev.json`; финальный offline replay: `artifacts/plan/evidence-reference-dev-final.json`.

## Независимый fresh v6

Создан после DEV: **150 conversations / 300 turns**, 15 категорий, независимая календарная точка `2027-01-20T10:35:00+03:00`. Read-only, create, chat, исторический поиск, meetings, timers, contextual edit, references, batch source/exclusions/delete/complete, reminders, ambiguity, destructive safety.

Генератор: `scripts/plan_fresh_v6.py`. До первого inference заморожены input/prompt/schema и 164 исходных файла. Input SHA256: `0bcc3c82f53477d18eb6f5379dfe54c6a37a1fe98bf9cfbb28770133683d5a14`. Протокол: `testdata/semantic/plan_heldout_v6_protocol.json`. Снимок исходников: `artifacts/plan/v6-frozen-source.zip`. Проверка до/после inference прошла. Corpus, prompt и implementation во время прогона не менялись. Единственный fresh результат: `artifacts/plan/heldout-v6-first.json`. Никакой пост-audit переоценки v6 нет.

| Метрика | Первый frozen v6 |
|---|---:|
| End-to-end | 274/300 = **91,33%** |
| Multi-turn | 134/150 = **89,33%** |
| Complex | 214/240 = **89,17%** |
| Routing | 300/300 = 100% |
| Strict plan validity | 182/190 = 95,79% |
| Unsafe mutations | **0** |
| Wrong-value mutations | **0** |
| Confirmed wrong-value mutations | **0** |
| Evidence guard rejections | **21** |
| Доказанные lexical value mismatches | **0** |
| Guard → clarification | **21** |
| Все clarification turns | **74** |
| Verifier calls | **0** |
| Fast path / LLM turns | 36,67% / 63,33% |
| ParsePlan calls per turn | 0,6333 |
| Logical / network provider calls | 190 / 147 |

Guard rejections v6: 6 `not_in_original`, 10 `missing`, 3 `unsupported_reference`, 1 `unsupported_clock`, 1 `unsupported_relative_day`. Пять отказов на неподдерживаемые reference/time/day пришлись на ожидаемую ambiguity и прошли end-to-end. Не следует выдавать все 21 отказ за пойманные неправильные normalized values: 16 являются ложными отказами или избыточными требованиями evidence.

26 ошибок end-to-end распределены так:

1. **6 timer create отказов.** Модель прислала необязательное evidence для title со словом «Таймер», которого с таким регистром нет в original «Запусти таймер…». Значение таймера могло быть корректным; guard заблокировал provenance. Это UX regression, не wrong-value write.
2. **10 отказов первого destructive-safety хода.** Для «Удали все задачи» модель прислала нейтральный `filters.status="any"` без отдельного evidence. Guard требует его и не показывает preview. Это избыточное требование к отсутствию ограничения. Один cached ParsePlan повторяется в 10 разных seeded conversations; это один повторившийся класс ошибки, а не 10 независимых model failures.
3. **10 провалов продолжения этих диалогов.** Gold ожидает сохранённый pending preview; фактически он не был создан на первом ходу. Дополнительно модель местами возвращает несовместимый chat payload либо help. Все отказы/обсуждения завершились без удаления.

Остальные категории имеют 20/20 ходов: source/destination batch, exclusions, delete, complete, references, contextual edit, temporal reminders, read/create meeting и ambiguity. Это не оправдывает низкий общий показатель: все 26 провалов сохранены в accuracy.

74 clarification = 40 настоящих batch preview + 21 evidence rejection + 13 остальных безопасных уточнений/отказов. Неверных записей на разрешённые ID не было. Новый wrong-value metric проверяет фактические writes, значения ожидаемых полей и сохранение остальных полей; безопасный отказ снижает accuracy, но не становится фиктивной mutation. Unsafe отдельно считает неожиданные ID и лишние создания.

| Latency | n | p50 ms | p95 ms |
|---|---:|---:|---:|
| LLM network inference | 147 | **2172,807** | **3139,997** |
| Semantic turn end-to-end, включая cache | 190 | 1965,918 | 3042,108 |
| Exact fast path end-to-end | 110 | 1,645 | 3,273 |

Сетевые latency отделены от cache. Это 147 provider inference requests, не счётчик возможных внутренних HTTP retries. Второго verifier вызова нет, поэтому дополнительного verifier network latency нет; Go evidence work входит в end-to-end. Сравнивать эти числа с прежним v5 как чистый причинный overhead нельзя: prompt/response и набор запросов изменились.

## Production gate

| Условие | Факт | Gate |
|---|---:|---|
| Unsafe = 0 | 0 | PASS |
| Confirmed wrong-value = 0 | 0 | PASS |
| V6 end-to-end ≥97% | 91,33% | FAIL |
| V6 multi-turn ≥95% | 89,33% | FAIL |
| V6 complex ≥95% | 89,17% | FAIL |
| Reference DEV ≥95% | 72,5% | FAIL |
| go test / go vet | PASS / PASS | PASS |

**Production ready: NO.** Evidence слой устраняет проверяемый класс неверных normalized values до preview и записи, но текущий строгий контракт создаёт ложные отказы для optional create evidence и нейтрального `any`. Остаются и прежние ошибки kind/context/schema. Для следующей итерации нужны отдельные DEV исправления этих классов и новый независимый holdout; этот v6 сохраняется как завершённый аудит.
