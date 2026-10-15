# GigaChat: локальная настройка и подготовка production

Production default остаётся `NLP_PROVIDER=rules`, пока `GENA_SEMANTIC_PROVIDER` не задан явно. Текущая работа не выполняет deploy.

`Voice/Text → Go fast path → GigaChat → strict SemanticCommand → Go temporal parsing/validation → existing executor → PostgreSQL`.

В локальном `.env` задаются переменные из `.env.example`. Основной credential — `GIGACHAT_AUTHORIZATION_KEY`. Client ID/Secret можно хранить там для управления учётной записью; приложение их не использует. Ключи и access token не передаются в сообщения модели. `.env`, его local-варианты и semantic cache исключены из Git.

Список доступных account model IDs нужно получить до выбора модели:

```powershell
$env:GIGACHAT_INTEGRATION = '1'
go run ./cmd/semantic-eval -models
```

OAuth идёт на `https://ngw.devices.sberbank.ru:9443/api/v2/oauth`; inference — на `https://api.giga.chat/v1/chat/completions`. Используется `GIGACHAT_API_PERS`, timeout `30s`, inference concurrency `1`. Одна goroutine обновляет токен, остальные ждут с учётом context; токен хранится только в памяти и обновляется за минуту до expiry. После 401 — invalidation и один повтор; после 429/5xx — максимум два повтора с ограниченной задержкой.

TLS verification обязательна. Если системного trust store недостаточно, скачайте CA bundle по [официальной инструкции GigaChat](https://developers.sber.ru/docs/ru/gigachat/certificates), проверяя HTTPS при скачивании, и задайте `GIGACHAT_CA_FILE` абсолютным локальным путём. Не отключайте проверку TLS. В текущей локальной проверке используется отдельный файл CA в `var/certs/`; он не добавлен в системное хранилище.

Обычный `go test ./...` работает без GigaChat. Live-проверка требует явного флага:

```powershell
$env:GIGACHAT_INTEGRATION = '1'
go test ./internal/llm -run '^TestGigaChatIntegration$' -count=1
```

Оценка по исходному prompt/schema, без runtime fast path:

```powershell
go run ./cmd/semantic-eval -model GigaChat-3-Ultra -input testdata/semantic/structured_regressions.jsonl -output artifacts/gigachat/mandatory-ultra.json
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/gigachat-screen.ps1
```

Cache key включает provider, model, system prompt, schema, source и input. `-mode go` использует только cache и не делает inference. `-bypass-cache` предназначен для отдельного latency experiment, а не для повторного независимого held-out. Все cached responses повторно строго декодируются. Частичный прогон сохраняется с `planned_total` и `aborted_reason`; три последовательных транспортных сбоя останавливают прогон.

DEV 572 и frozen held-out 208 восстановлены из локального бэкапа. `structured_heldout.jsonl` не изменён: SHA-256 `704b0c5324dab12bf5673afeab92df1c7507d7555f1761e619456eb8ab093b1e`. DEV-выборка и её seed/IDs/hashes сохраняются в `artifacts/gigachat/freeze.json`. Итоги и ограничения — в [GIGACHAT_SEMANTIC_REPORT.md](GIGACHAT_SEMANTIC_REPORT.md).

## Production override

На VPS используется hardened `deploy/compose.production.yaml` вместе с `deploy/compose.gigachat.example.yaml`; оба файла подключены в `deploy/rhythm.service`. Основной локальный `compose.yaml` не менялся. В `/etc/rhythm/production.env` сохраняется только нужный runtime Authorization Key; Client ID/Secret туда не передаются. `GIGACHAT_CA_FILE_HOST` указывает на проверенный Linux CA bundle, монтируемый read-only как `/etc/gigachat/ca.pem`. Не копируйте Windows-путь из локального `.env` на сервер. При нескольких backend replicas Freemium concurrency=1 нужна общая очередь или единственный inference worker; текущий semaphore действует в одном процессе.

Для проверки сборки compose-конфигурации используйте только вывод без значений:

```sh
docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml -f deploy/compose.gigachat.example.yaml config --quiet
```

Не публикуйте обычный `docker compose config` с раскрытыми env. Access token не задаётся в deployment env и не сохраняется на диск. При unavailable/timeout/invalid output semantic adapter возвращает уточнение, мутация не передаётся executor. Rules fast path консервативен: однозначные списки, полные декларации встреч с датой/временем и существующий dialogue flow обрабатываются локально.

Для включения GigaChat в контейнерном запуске нужен указанный override: основной compose сам не передаёт GIGACHAT-переменные из локального `.env`. После замены credentials и подготовки environment будущая команда выпуска:

```sh
docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml -f deploy/compose.gigachat.example.yaml up -d --build --no-deps --wait app
```

Обновляется только app; PostgreSQL и постоянные volumes сохраняются. Перед выпуском выполняются штатный backup PostgreSQL, проверка dump через pg_restore --list и сохранение старых release/config. `deploy/gigachat_ops.py` принимает приватный JSON только через stdin и печатает только metadata/counts; credentials не передаются как argv. При откате используется предыдущий release/config без восстановления устаревшего GigaChat ключа. Подробный workflow — [deploy/README.md](../deploy/README.md).

Полный локальный audit: `python scripts/security-audit.py --extra-source <old-credentials-txt>`. Он проверяет рабочие файлы, index, diff и Git objects всех refs/reflog; выводит только counts/paths/types. `.env` — разрешённое ignored secret-хранилище. Dependencies исключены из обхода. Проверенные синтетические OAuth fixtures исключаются только по path-scoped hashes; exact-value matches реальных credentials никогда не исключаются.
