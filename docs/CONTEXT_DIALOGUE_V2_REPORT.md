# Contextual dialogue: локальная оценка v2, 16 октября 2026

**Production ready: NO. Deployment не выполнялся, Production/VPS не использовались.**

Cancellation и timer word order исправлены. Temporal replies закрываются локально
без LLM. Refinement сохраняет query/kind и валидирует успешность предыдущего
поиска, но новый независимый аудит обнаружил неподдержанный класс календарного
периода. Это оставлено блокером: код/prompt по результатам held-out v2 не менялись.

## Before → After

Это разные frozen выборки: исторический v1 (110 диалогов / 220 реплик) и новый
v2 (135 / 270). Это сравнение аудитов, а не парная оценка на тех же фразах.
Старый context_heldout не переоценивался и не использовался для настройки.

| Метрика | Before, v1 | After, fresh v2 | Порог |
|---|---:|---:|---:|
| Pending slot completion | 80% | **100% (30/30)** | ≥97% |
| Context retention | 82,86% | **90,53% (86/95)** | ≥95% — FAIL |
| Multi-turn success | 86,36% | **93,33% (126/135)** | ≥95% — FAIL |
| End-to-end turn accuracy | 91,36% | **96,67% (261/270)** | ≥97% — FAIL |
| Unsafe accepted mutations | 0 | **0** | 0 |
| Reference relation, однозначные | 100% | **100% (30/30)** | ≥99% |
| Target resolution | 100% | **100% (30/30)** | ≥99% |
| New command vs continuation | 95% | **100% (270/270)** | ≥98% |
| Mandatory contextual, DEV | 100% | **100% (13/13)** | 100% |
| Turns without LLM | 17,27% | **41,11% (111/270)** | correctness first |
| Semantic calls / turn | 0,827 | **0,589 (159/270)** | informational |
| Contextual GigaChat p50 / p95 | 1789 / 2607 мс | **1530 / 2284 мс** | informational |

Slot fast path: mean **0,594 мс**, p50 **0,553 мс**, p95 **0,743 мс**, n=30.
Прежний mean/p95: 0,264/2,638 мс, n=10; отдельные прежние нули связаны с
разрешением часов. Весь turn без LLM: mean 1,915 мс, p95 2,743 мс.

Модель GigaChat-3-Ultra, temperature=0, последовательная оценка. 159 logical
semantic calls, 120 сетевых вызовов; 39 одинаковых запросов переиспользованы из
cache внутри первого полного run. Cache попадания считаются semantic calls и
не повышают turns_without_llm. Contextual p50/p95 посчитаны только по 64 реальным
сетевым измерениям; replay не включён в latency.

## Проверки

- Новые DEV: **95/95 диалогов, 190/190 реплик**. Temporal filler — 30,
  cancellation/confirmation — 25, refinement — 25, timer word order — 15.
- Отдельный DEV поиска по новым именам: **25/25**, включая мужские, женские,
  редкие имена и склонения. Все 25 также прошли реальный semantic provider с
  уточнённым prompt; более широкий DEV всё ещё выявлял отдельные unknown,
  поэтому простой явный поиск получил узкий read-only fast path без словаря имён.
- Прежний contextual DEV: **24/24 диалога, 53/53 реплики**, mandatory **13/13**.
  Финальный DEV replay повторно проверяет сохранённые strict JSON через текущие
  guards, resolver, executor и временные SQLite-базы.
- Прежние **13 CORE**: сохранённые ответы GigaChat проверены через executor.
- Обязательные A–I: отдельные Go tests для A–F; G — DEV поиска;
  H — Go test + DEV refinement; I — Go tests и 15 новых DEV таймеров.
- `go test -count=1 ./...`: **PASS**, с включённым recorded CORE artifact.
- `go vet ./...`: **PASS**. Validation и safety не ослаблялись.

Fresh v2: temporal **30/30**, cancellation/confirmation **25/25**, timer
**15/15**, references/ambiguity **40/40**, refinement **16/25**. Все 10
неоднозначных ссылок после списка требуют clarification; target не угадывается.

Все девять ошибок относятся к фильтру предыдущего календарного месяца. Go
не поддержал этот period и вернул безопасное уточнение. Action/query/kind и
turn type сохранены, мутаций нет, но ожидаемый поиск не выполнен: это полноценный
функциональный FAIL. Gold, код и prompt после первого полного run не менялись.
Следующий этап потребует независимых DEV примеров класса календарных периодов
и нового hold-out; текущий v2 нельзя использовать для дальнейшего tuning.

## Freeze и артефакты

Fresh создан после финальных изменений и DEV. Перед inference, в
**21:18:09 Europe/Moscow**, зафиксированы input SHA-256, обе prompt/schema и
хеши **151 файла** кода/тестов/зависимостей. Evaluator проверил protocol до
создания provider; проверка после run подтвердила отсутствие изменений.

SHA-256 `context_heldout_v2.jsonl`:
`5c75ba500ca35e0ea58d4581fb3d00ad1ccb593d03e99bd2afa09ade4f03f916`.

Protocol: `testdata/semantic/context_heldout_v2_protocol.json`.
В `contract_hashes` значения хешируются в JSON encoding; отдельный
`prompt_sha256` отчёта хеширует UTF-8 строку contextual prompt.

Локальные отчёты:

- `artifacts/context/heldout-v2-first.json` — первый полный fresh run.
- `artifacts/context/dev2-final.json` — новый DEV, финальный replay.
- `artifacts/context/search-dev2-run2.json` — отдельный live semantic search DEV.
- `artifacts/context/search-dev2-fast-final.json` — финальный search fast path.
- `artifacts/context/dev-original-v2.json` — прежние contextual regressions.
- `artifacts/context/freeze-verification-v2.json` — проверка freeze после аудита.
- `artifacts/context/go-test-v2.log`, `go-vet-v2.log` — финальные проверки.

| Проверка | Итог |
|---|---|
| Cancellation fixed | YES |
| Timer word-order fixed | YES |
| Refinement fixed | NO — частично; календарный period остаётся блокером |
| New fresh holdout used | YES — 135 conversations / 270 turns |
| Production ready | NO |
