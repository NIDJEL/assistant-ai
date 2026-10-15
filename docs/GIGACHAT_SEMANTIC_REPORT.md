# GigaChat SemanticCommand: локальная интеграция и оценка

15 октября 2026, Europe/Moscow. Production VPS, production env/services и production DB не использовались. Qwen inference не запускался.

## Результат

Порог hackathon production достигнут: Ultra. Credentials ротированы перед production release; проверка deployment выполняется отдельно от frozen evaluation.

## Архитектура и безопасность

`Voice/Text → conservative Go fast path → SemanticProvider.Parse → strict SemanticCommand JSON → grounding/safety guards → deterministic Go temporal parsing + domain.Command validation → existing assistant.Service/executor → PostgreSQL`.

В текущем main отсутствовали прежние semantic-файлы: необходимые SemanticCommand, prompt, temporal helpers, корпуса и scoring восстановлены из `D:/assistantai-backups/20261015-sync/local-source-before.tar.gz`. Domain DTO не менялся. В executor восстановлены pending-slot context и атомарный rename после поиска/validation/conflict checks. Frontend передаёт только source=text|voice; профиль, история, БД и now/timezone модели не передаются.

Rules fast path ограничен однозначными запросами списка; подтверждения и slot replies остаются в Go. Evaluation вызывает Normalize напрямую, без fast path, чтобы измерять модель. После model output не вызывается полный intent/title parser; Go разрешает temporal roles отдельно. ID извлекаются только из исходных #/№/номер; ID отсутствуют в schema.

Один reusable http.Client с pooling, context/timeout, ограничением body 1 MiB и запретом redirects. GIGACHAT_MAX_CONCURRENT=1 ограничивает только provider. Очередь учитывает ctx.Done. Semaphore действует внутри одного процесса; нескольким replicas Freemium нужна общая очередь.

Authorization Key используется приоритетно; Client ID/Secret сохранены локально, не применяются для OAuth. Access token живёт только в памяти вместе с expires_at; single refresh разделяется между goroutines, обновление за минуту до expiry. 401 инвалидирует token и допускает один повтор. 429/5xx/network имеют bounded retry, максимум два повтора; ошибки не включают HTTP body, headers, credential или token.

TLS verification включена; дополнительный CA скачан с официального HTTPS-источника и подключён через GIGACHAT_CA_FILE, без изменения системного trust store. Не используются InsecureSkipVerify или незашифрованные реальные endpoints.

GigaChat получает response_format={type:json_schema,schema:SemanticSchema(),strict:true}. Контракт един для GigaChat/Ollama; источник — internal/nlp/semantic.go, точный export с prompt — testdata/semantic/structured_contract.json. Проверяются required fields, unknown/duplicate keys, enum/shape, trailing JSON, SemanticCommand invariants, grounding и guards. Invalid/unavailable output fail-closed.

Cache содержит только model JSON и usage/latency; key=sha256(provider+model+prompt+schema+source+input), путь artifacts/cache/semantic/gigachat/<hash>.json. Cached JSON повторно декодируется. HTTP/OAuth envelopes и secrets не сохраняются.

`.env` ignored/not tracked; local env-варианты, credentials TXT patterns, var/certs и semantic cache ignored. Секретный TXT не копировался в repository и не изменялся. Automated scan проверяет рабочие файлы/index/history/артефакты без печати значений. **Инцидент предыдущей сессии:** старый секретный TXT ошибочно попал в tool output. Перед production release credentials заменены; старый ключ больше не используется для OAuth.

## Connectivity и реально доступные модели

OAuth, TLS, models endpoint и structured request: успешно. Строгость Go boundary дополнительно подтверждена обязательным набором и unit tests.

Account model IDs: `GigaChat-2`, `GigaChat-2-Max`, `GigaChat-2-Pro`, `GigaChat-3-Lightning`, `GigaChat-3-Pro`, `GigaChat-3-Ultra`, `Embeddings`, `Embeddings-2`, `EmbeddingsGigaR`, `GigaEmbeddings-3B-2025-09`.

Connectivity request Ultra: 3.391 s; input 2279, output 167, total 2446 tokens.

Для классов сравнения выбраны реальные IDs: Ultra=GigaChat-3-Ultra, Max=GigaChat-2-Max, Pro=GigaChat-3-Pro, Lite=GigaChat-2. GigaChat-2-Pro и Lightning перечислены API; отдельный дополнительный прогон не выполнялся.

## Методика и freeze

DEV 572 восстановлен byte-for-byte; sampling seed=20261015. DEV screening: 72 фразы, включая task/reminder/meeting/list/search/update/delete/complete/recurrence/ASR/ambiguity. Состав: 34 create, 6 list, 10 search, 6 update, 5 delete, 5 complete, 6 help; 4 ASR-tagged. Mandatory тексты исключены из sample. В старом DEV нет полного SemanticCommand gold, поэтому full semantic/changes/temporal extraction на DEV не размечены.

Held-out 208 SHA-256: `704b0c5324dab12bf5673afeab92df1c7507d7555f1761e619456eb8ab093b1e`. Prompt/schema исходного Qwen baseline сохранены без tuning. После DEV финальный Go mapping повторно проверен только из cache. До первого held-out зафиксированы finalist list, scoring, provider/validation/temporal code hashes в artifacts/gigachat/heldout-protocol.json. Finalists: GigaChat-3-Ultra, GigaChat-3-Pro.

Held-out не использовался для tuning. Во время baseline prompt/schema/зафиксированный Go code не менялись; повторный inference и компактный prompt experiment не выполнялись. При release preflight расширен только runtime fast path для полной канонической декларации «В четверг в 11 у меня встреча с дизайнером»; Normalize/model boundary/scoring не менялись, held-out не перезапускался. Приведённые метрики принадлежат сохранённому baseline. Часы Go: 2026-10-15 12:30:17+03:00, Europe/Moscow. Это синтетические текстовые фразы, не live-ASR и не оценка WER.

CORE A–D: create task/reminder без recurrence, create meeting, list, search. BONUS: update/delete/complete/recurrence/ambiguity. Action/kind/title/query/target/changes/temporal extraction/full semantic оцениваются на model boundary до Go guards; target accepted + scope, temporal domain и domain.Command — после Go. Сравнение наследует scoring Qwen; title/query требуют нормализованного точного совпадения, clarification wording не оценивается. Unsafe accepted включает мутацию вместо read-only/help, invented ID и неверный mutation target/kind/scope; это проверка данного корпуса, не доказательство универсальной безопасности.

Latency: concurrency=1. Cold обозначает первый запрос нового клиента с OAuth/новым соединением; hosted model cold-start независимо измерить невозможно. Warm p50/p95 — nearest-rank остальных live requests. Cache hits исключены из latency. API usage отражает возвращённые prompt/completion/total tokens; запросы, завершившиеся timeout до usage, не получают выдуманный token count.

## Mandatory 13

| Model | domain.Command | Full semantic | Unsafe accepted |
|---|---:|---:|---:|
| Ultra | 13/13 = 100.00% | 13/13 = 100.00% | 0 |
| Max | N/A | N/A | N/A |
| Pro | 11/13 = 84.62% | 11/13 = 84.62% | 0 |
| Lite | N/A | N/A | N/A |

Ultra: все 13 сохранённых реальных ответов дополнительно прошли существующий executor на изолированных SQLite fixtures (create/list/search/update/delete confirmation/complete/rename), без новых model calls. CSV 1000 событий проверен в отдельной test DB: import, period list, search и fuzzy search; CSV не изменялся.

## DEV screening

| Model | Completed | Action | CORE domain.Command | domain.Command | warm p50 / p95 |
|---|---:|---:|---:|---:|---:|
| Ultra | 72/72 | 72/72 = 100.00% | 43/46 = 93.48% | 68/72 = 94.44% | 2.438 / 3.941 s |
| Max | diagnostics only | N/A | N/A | N/A | N/A |
| Pro | 72/72 | 67/72 = 93.06% | 38/46 = 82.61% | 48/72 = 66.67% | 2.028 / 2.941 s |
| Lite | 3/72 | N/A | N/A | N/A | N/A |

Max: при timeout=30s ответы не завершались; отдельный probe с timeout=90s вернул невалидный SemanticCommand через 51.301 s. Полный DEV72 не выполнялся после диагностики, полный held-out отсутствует. Lite: три последовательных timeout по 30s, прогон остановлен после 3/72. Их показатели качества не оцениваются как 0/72 и обозначены N/A.

## Frozen held-out 208: сравнение

| Metric | Qwen 4B | GigaChat Ultra | Max | Pro | Lite |
|---|---:|---:|---:|---:|---:|
| Action | 96.63% | 100.00% | N/A | 99.04% | N/A |
| Kind | 97.12% | 95.19% | N/A | 96.63% | N/A |
| Title | 95.54% | 100.00% | N/A | 95.54% | N/A |
| Query | 95.83% | 100.00% | N/A | 100.00% | N/A |
| Target semantic | 82.50% | 100.00% | N/A | 72.50% | N/A |
| Target accepted + scope | 40.00% | 100.00% | N/A | 62.50% | N/A |
| Changes | 56.25% | 100.00% | N/A | 87.50% | N/A |
| Temporal extraction | 84.62% | 96.15% | N/A | 86.06% | N/A |
| Temporal domain | 86.06% | 100.00% | N/A | 88.94% | N/A |
| Full semantic | 73.56% | 91.35% | N/A | 75.48% | N/A |
| domain.Command | 77.40% | 91.35% | N/A | 75.96% | N/A |
| CORE domain.Command | 87.50% | 97.79% | N/A | 86.03% | N/A |
| Unsafe accepted | 0 | 0 | N/A | 0 | N/A |
| Warm p50 | 1.790 s | 2.020 s | N/A | 1.610 s | N/A |
| Warm p95 | 2.457 s | 3.441 s | N/A | 2.530 s | N/A |
| Avg tokens/request (API) | 2391.4 | 2450.6 | N/A | 211.0* | N/A |

* Pro: API total_tokens исключает precached_prompt_tokens. В отдельном DEV/connectivity probe после held-out API вернул prompt=90, cached-prefix=2189, completion=169, total=259. Полный вход этого probe: 2279 токенов. Поэтому 211,0 API tokens/request нельзя считать полным размером контекста или напрямую сравнивать с Ultra для оценки стоимости. Cached-prefix не был сохранён для каждого held-out request; его среднее на held-out — N/A. Probe не менял prompt/schema и не использовался для повторной оценки held-out.

Qwen quality — сохранённый final Go report; performance/usage — сохранённый исходный live held-out report. Provenance и hashes в testdata/semantic/qwen_baseline.json. Qwen не запускался.

## CORE, BONUS и ошибки

| Model | CORE action | CORE title | CORE query | CORE domain | BONUS domain | False unknown | Validation/model errors | Unsafe proposals | Unsafe accepted |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Ultra | 136/136 = 100.00% | 96/96 = 100.00% | 24/24 = 100.00% | 133/136 = 97.79% | 57/72 = 79.17% | 0 | 10 | 0 | 0 |
| Pro | 136/136 = 100.00% | 92/96 = 95.83% | 24/24 = 100.00% | 117/136 = 86.03% | 41/72 = 56.94% | 25 | 33 | 7 | 0 |

### Ultra remaining failures

Failure categories: {"kind_accuracy": 3, "other": 5, "validation_or_model_error": 10}.

Strict response validation failures: **10**; Go temporal/mapping failures: **0**; safety guard rejections: **0**; transport/envelope failures: **0**.

Reasons: {"semantic/domain mismatch": 8, "invalid SemanticCommand JSON": 10}. Полные cases и outputs сохраняются только в локальном JSON report; по ним prompt/schema не менялись.

### Pro remaining failures

Failure categories: {"other": 4, "target_accuracy": 1, "temporal_accuracy": 7, "title_accuracy": 5, "validation_or_model_error": 33}.

Strict response validation failures: **21**; Go temporal/mapping failures: **12**; safety guard rejections: **7**; transport/envelope failures: **0**.

Reasons: {"semantic/domain mismatch": 11, "unsupported duration expression": 5, "ungrounded title/query": 4, "unsupported relative time expression": 3, "ungrounded target": 2, "invalid SemanticCommand JSON": 21, "unsupported period expression": 3, "notification time needs event time": 1}. Полные cases и outputs сохраняются только в локальном JSON report; по ним prompt/schema не менялись.

## Tokens и производительность

| Model | First client request | Input avg (API) | Output avg | Total avg (API) | Usage coverage | Input total (API) | Output total |
|---|---:|---:|---:|---:|---:|---:|---:|
| Ultra | 2.332 s | 2281.0 | 169.6 | 2450.6 | 208/208 | 474448 | 35282 |
| Pro | 2.287 s | 28.0 | 183.0 | 211.0 | 208/208 | 5824 | 38060 |

Baseline prompt+schema дают около 2,3 тыс. input tokens/request по connectivity. Полная схема дополнительно передаётся как API response_format; server token accounting может учитывать её иначе. Компактный prompt/schema experiment не выполнялся: сначала сохранён исходный baseline.

## Проверки и production решение

`go test -count=1 ./...`: PASS. `go vet ./...`: PASS. Targeted token/strict JSON/timeout/retry/cancellation/semaphore/cache/grounding/ID tests: PASS. Обычные тесты не обращаются к API; live test требует GIGACHAT_INTEGRATION=1. Race не запускался: gcc не найден. Frontend: tsc --noEmit и 27 App/voice tests PASS; полный frontend suite не запускался.

Критерии: CORE domain >=97%, CORE action >=99%, CORE title/query >=98% каждый, unsafe accepted=0. Выполнили все критерии: Ultra.

Интеграция и opt-in production override подготовлены. Production не изменён. Следующие настройки описаны в [GIGACHAT_CONFIGURATION.md](GIGACHAT_CONFIGURATION.md). При недостигнутом CORE-пороге нужны улучшения на DEV и новый fresh held-out для новой честной оценки; текущий held-out не становится независимым после повторного использования.

Источники API: [OAuth](https://developers.sber.ru/docs/ru/gigachat/api/reference/rest/gigachat-api), [structured output](https://developers.sber.ru/docs/ru/gigachat/guides/structured-output), [CA](https://developers.sber.ru/docs/ru/gigachat/certificates).
