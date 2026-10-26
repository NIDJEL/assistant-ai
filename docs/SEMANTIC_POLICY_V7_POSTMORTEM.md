# Первый frozen v7: разбор без исправления результатов

Полные журналы и архивы из `artifacts/plan/` сохранены локально и не входят в публикуемый репозиторий. Наборы для воспроизведения находятся в `testdata/semantic/`; актуальные публичные доказательства приведены в [отчёте об улучшениях](QUALITY5_IMPROVEMENTS.md).


**Production ready: NO.** Единственный полный прогон: 150 диалогов / 370 ходов. Corpus, prompt, schema, code и evaluator после freeze не менялись; inference/replay v7 не повторялся. Изменения после получения результатов ограничены отчётом и диагностическими артефактами.

## Неправильная запись

`fresh-v7-correction_chain-09`, первый ход: «Заведи задачу «Заменить привод купола» на завтра».

Модель вернула create task, правильный title, `schedule=null` и необязательное evidence с путём `changes.date`. Создана задача без срока вместо даты 2027-04-14. Отсутствующее значение вообще не дошло до temporal value mismatch guard. Это **wrong-value=1**, не безопасный отказ. Следующая коррекция была отклонена из-за evidence; неправильная первая запись остаётся в метрике. Confirmed wrong-value=0: эта запись не требовала подтверждения.

Optional evidence не должно становиться blocking constraint. Одновременно его игнорирование не решает отдельный класс ошибок — модель полностью теряет пользовательское ограничение. DEV не покрывал этот класс достаточно широко. Новых правил или второго verifier по этому v7 не добавлено.

## 91 неуспешный ход

| Место/класс отказа | Ходы |
|---|---:|
| Evidence guard | 38 |
| Невалидный semantic payload/temporal meaning/запрещённое подтверждение моделью | 28 |
| Коррекция reminder ошибочно названа slot_reply при отсутствии pending | 10 |
| Потеря списка после безопасной reference clarification | 10 |
| Timer duration вместо delay и последующее переименование несозданной записи | 2 |
| Отложенное подтверждение ошибочно интерпретировано как новый list | 2 |
| Создание без явно запрошенной даты | 1 |
| **Всего** | **91** |

Это классификация места неуспеха каждого хода; последующие отказы не являются независимыми первопричинами. Например, один cached plan с пропущенным status evidence повторён в десяти seeded диалогах и приводит к двадцати неуспешным ходам, включая последующее подтверждение. Все двадцать остаются в end-to-end; их нельзя считать двадцатью независимыми ошибками inference.

Evidence: 19 unsupported_reference (модель цитирует «Нет» вместо «лучше»), 11 missing (10 status, 1 source period), 6 duplicate (5 date, 1 reference), 1 not_in_original (придуманное «её»), 1 unsupported_shift («на полчаса позже»). Всего 38. Доказанных lexical value_mismatch среди этих отказов — 0; это не 38 пойманных неверных normalized values.

Строгие payload failures: 14 incompatible semantic plan, 11 model cannot authorize confirmation/cancellation, 3 invalid normalized temporal meaning. Среди них есть последствия прежнего отказа: подтверждать уже нечего, а модель всё равно возвращает confirmation.

## Reference/context

Focused DEV: 56 диалогов / 152 хода — 100%; старый reference DEV: 20 / 40 — 100%. На разнообразных v7 продолжениях качество ниже:

- После «Ту, которая раньше обсуждалась» безопасный clarification через command search не попадает в новую защиту ambiguous/refinement. В `rememberDialogue` прежний `DetectIntent` считает слово «раньше» новым search и очищает LastIDs/фильтр. Следующее правильное ordinal2 уже нельзя разрешить. 10 повторений этого сценария провалились на следующем выборе.
- Для «Теперь отметь её выполненной» модель добавляет `changes.status=done` к operation=complete. Текущий strict contract допускает changes только у update; 8 таких payload отклонены. Ещё один complete содержит duplicate reference evidence.
- Для исправлений reminder модель выбирает slot_reply вместо command update target last, хотя pending уже завершён: 10 отказов.
- После «Согласия ещё не давал, пока только смотрю список» и похожего сообщения модель возвращает новый list; destructive pending снимается без удаления. Последующее «отмена» уже не попадает в точный cancellation fast path: ещё 2 downstream отказы.

Эти случаи не исправлялись после holdout. Текущая защита сохранения контекста покрывает часть clarification paths; общее утверждение, что любой safe clarification сохраняет контекст, было бы неверным.

## Таймеры, any и границы результата

Обязательные timer regressions в новом DEV: 12/12 create, evidence false rejects=0. Neutral any/read/shift: evidence false rejects=0. В v7 у timer create также **0 evidence rejection**, но одна модельная ошибка осталась: «длительностью полчаса» превращено в `duration_minutes=30`, без delay. Это 9/10 успешных timer create и 18/20 ходов категории с последующим переименованием. Нельзя считать ноль evidence rejects стопроцентной успешностью таймеров.

В v7 initial batch delete с нейтральным any проходит; отсутствие evidence для any не блокирует preview. Отдельные explicit status=done требуют provenance. Один cached plan не прислал `filters.status` evidence, хотя `batch.scope` содержал «все завершённые задачи»; 10 таких отказов относятся к required explicit constraint, а не к neutral any.

Все исходные 48 и дополнительные 20 adversarial model-plan cases PASS с mutation count=0. Это сохраняет проверенные гарантии для заданных неверных значений, но не доказывает отсутствие всех пропущенных ограничений в произвольном create.

## Результаты и воспроизводимость

- End-to-end: 279/370 = **75,41%**.
- Multi-turn: 75/150 = **50%**.
- Complex: 241/330 = **73,03%**.
- Unsafe / wrong-value / confirmed wrong-value: **0 / 1 / 0**.
- LLM network p50/p95: **1837,548 / 2667,653 ms**, n=261.
- 304 logical ParsePlan calls, 66 exact fast paths, verifier calls=0.
- `go test -count=1 ./...` и `go vet ./...`: PASS перед freeze; все 172 замороженных файла совпали после полного inference.

Raw result: heldout-v7-first.json (локальный файл `artifacts/plan/heldout-v7-first.json`). Protocol: [plan_heldout_v7_protocol.json](../testdata/semantic/plan_heldout_v7_protocol.json). Freeze verification: v7-freeze-verification.json (локальный файл `artifacts/plan/v7-freeze-verification.json`). Машинная сводка: policy-v7-summary.json (локальный файл `artifacts/plan/policy-v7-summary.json`).
