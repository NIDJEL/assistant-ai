# Ассистент Гена: отчёт об изменениях

Дата: 14 октября 2026, Europe/Moscow. Изменения выполнены в рабочей копии `D:/assistantai`; удалённый production не обновлялся. Исходные незакоммиченные изменения сохранены.

## Результат

Удалён VK Auth; короткий бренд — Гена, официальное имя — Ассистент Гена. Расширен существующий deterministic Russian NLU, без замены LLM и без переписывания parser. Поддержаны составные интервалы, календарные месяцы/годы и структурные повторяющиеся задачи/напоминания.

## Файлы

Новые файлы именно этого этапа:

- `internal/calendar/recurrence.go`
- `internal/domain/recurrence.go`
- `internal/domain/recurrence_test.go`
- `internal/nlp/recurrence.go`
- `internal/nlp/recurrence_test.go`
- `internal/storage/postgres/migrations/003_remove_vk.sql`
- `tests/contract/identity_test.go`
- `tests/contract/recurrence_test.go`
- `web/e2e/recurrence.spec.ts`
- `web/src/features/events/RecurrenceEditor.tsx`
- `web/src/features/events/recurrence.ts`
- `web/src/features/events/recurrence.test.tsx`
- `docs/GENA_NLU_REPORT.md`

Удалены: `internal/integrations/vk/provider.go`, `web/src/assets/oauth/vk.svg`, `web/src/assets/oauth/VK-LICENSE.txt`.

Ниже полный inventory текущего working tree; он включает прежние изменения. «Предыдущий этап» означает существовавшее изменение, сохранённое без отдельной реализации в этой задаче. В deploy/ файлы уже существовали как untracked; изменены только README и production Compose. Служебные скрипты и логи tmp/ не входят в поставку. Локальный игнорируемый `.env` также очищен только от VK_* (значения секретов не публикуются).

| Файл | Статус / происхождение |
| --- | --- |
| `.env.example` | изменён / расширен |
| `.env.go.example` | изменён / расширен |
| `.github/workflows/ci.yml` | предыдущий этап, сохранён |
| `Dockerfile` | предыдущий этап, сохранён |
| `README.md` | изменён / расширен |
| `app/main.py` | изменён / расширен |
| `app/static/app.js` | изменён / расширен |
| `app/static/architecture.html` | изменён / расширен |
| `app/static/index.html` | изменён / расширен |
| `cmd/server/main.go` | изменён / расширен |
| `compose.yaml` | изменён / расширен |
| `docs/ARCHITECTURE.md` | изменён / расширен |
| `docs/ASSISTANT_BEHAVIOR.md` | изменён / расширен |
| `docs/GO_PARITY.md` | изменён / расширен |
| `docs/P1_2_AUTH_POSTGRES.md` | изменён / расширен |
| `docs/P1_UI.md` | изменён / расширен |
| `docs/YANDEX_CALENDAR.md` | изменён / расширен |
| `docs/assets/readme-cover.svg` | изменён / расширен |
| `docs/diagrams/p1-architecture.svg` | изменён / расширен |
| `docs/diagrams/p1-voice-flow.svg` | изменён / расширен |
| `docs/p1-architecture.html` | изменён / расширен |
| `docs/screenshots/current/assistant.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/current/calendar.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/current/integrations.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/current/tasks.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/account.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/index.html` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/login-compact-desktop.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/login-desktop.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/login-mobile.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding-calendar.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding-calendar@2x.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding-mobile@2x.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding-ready.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding-ready@2x.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/onboarding.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/register-compact-desktop.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/register-desktop.png` | актуальный screenshot обновлён / перегенерирован |
| `docs/screenshots/p1-2/register-mobile.png` | актуальный screenshot обновлён / перегенерирован |
| `go.mod` | предыдущий этап, сохранён |
| `go.sum` | предыдущий этап, сохранён |
| `internal/api/auth.go` | изменён / расширен |
| `internal/api/auth_openapi.go` | изменён / расширен |
| `internal/api/dto/requests.go` | изменён / расширен |
| `internal/api/middleware.go` | изменён / расширен |
| `internal/api/openapi.go` | изменён / расширен |
| `internal/assistant/selection.go` | предыдущий этап, сохранён |
| `internal/assistant/service.go` | изменён / расширен |
| `internal/auth/oauth.go` | изменён / расширен |
| `internal/calendar/service.go` | изменён / расширен |
| `internal/config/config.go` | изменён / расширен |
| `internal/domain/details.go` | изменён / расширен |
| `internal/domain/enums.go` | предыдущий этап, сохранён |
| `internal/domain/models.go` | изменён / расширен |
| `internal/integrations/oauth/provider.go` | изменён / расширен |
| `internal/integrations/oauth/provider_test.go` | изменён / расширен |
| `internal/integrations/vk/provider.go` | удалён в этом этапе |
| `internal/integrations/yandexcalendar/caldav.go` | изменён / расширен |
| `internal/integrations/yandexcalendar/caldav_test.go` | изменён / расширен |
| `internal/integrations/yandexcalendar/ical.go` | изменён / расширен |
| `internal/integrations/yandexcalendar/service.go` | изменён / расширен |
| `internal/nlp/conversation.go` | изменён / расширен |
| `internal/nlp/dates.go` | изменён / расширен |
| `internal/nlp/hybrid.go` | предыдущий этап, сохранён |
| `internal/nlp/parser.go` | изменён / расширен |
| `internal/nlp/reminders.go` | изменён / расширен |
| `internal/nlp/selection.go` | изменён / расширен |
| `internal/nlp/spoken_time.go` | предыдущий этап, сохранён |
| `scripts/build_p1_diagrams.py` | изменён / расширен |
| `start.ps1` | изменён / расширен |
| `tests/contract/auth_postgres_test.go` | изменён / расширен |
| `tests/contract/oauth_http_postgres_test.go` | изменён / расширен |
| `tests/contract/parity_test.go` | изменён / расширен |
| `tests/contract/reminder_planning_test.go` | предыдущий этап, сохранён |
| `tests/contract/yandex_calendar_postgres_test.go` | изменён / расширен |
| `web/e2e/account-profile.spec.ts` | изменён / расширен |
| `web/e2e/auth.spec.ts` | изменён / расширен |
| `web/e2e/fixtures.ts` | изменён / расширен |
| `web/e2e/voice.spec.ts` | предыдущий этап, сохранён |
| `web/index.html` | изменён / расширен |
| `web/playwright.config.ts` | изменён / расширен |
| `web/scripts/capture-auth.mjs` | изменён / расширен |
| `web/src/app/App.test.tsx` | изменён / расширен |
| `web/src/app/App.tsx` | изменён / расширен |
| `web/src/app/Auth.test.tsx` | изменён / расширен |
| `web/src/assets/oauth/README.md` | изменён / расширен |
| `web/src/assets/oauth/VK-LICENSE.txt` | удалён в этом этапе |
| `web/src/assets/oauth/vk.svg` | удалён в этом этапе |
| `web/src/components/Sidebar.tsx` | изменён / расширен |
| `web/src/features/assistant/ChatEventCard.tsx` | изменён / расширен |
| `web/src/features/calendar/DayEvent.tsx` | изменён / расширен |
| `web/src/features/events/EventEditor.tsx` | изменён / расширен |
| `web/src/features/events/EventRow.tsx` | изменён / расширен |
| `web/src/features/events/TaskCard.tsx` | изменён / расширен |
| `web/src/features/notifications/NotificationSettings.tsx` | изменён / расширен |
| `web/src/features/notifications/notifications.test.ts` | изменён / расширен |
| `web/src/features/notifications/notifications.ts` | изменён / расширен |
| `web/src/features/voice/SpeechLearningSettings.tsx` | изменён / расширен |
| `web/src/pages/Auth/AuthPage.tsx` | изменён / расширен |
| `web/src/pages/Auth/OnboardingPage.tsx` | изменён / расширен |
| `web/src/pages/Integrations/IntegrationsPage.tsx` | изменён / расширен |
| `web/src/pages/Reminders/RemindersPage.tsx` | изменён / расширен |
| `web/src/pages/Settings/AccountCard.tsx` | изменён / расширен |
| `web/src/pages/Settings/SettingsPage.tsx` | изменён / расширен |
| `web/src/styles/app.css` | изменён / расширен |
| `web/src/styles/auth.css` | изменён / расширен |
| `web/src/types/api.ts` | изменён / расширен |
| `deploy/README.md` | изменён / расширен |
| `deploy/VALIDATION.md` | предыдущий этап, сохранён |
| `deploy/backup.sh` | предыдущий этап, сохранён |
| `deploy/compose.production.yaml` | изменён / расширен |
| `deploy/harden-host.sh` | предыдущий этап, сохранён |
| `deploy/nginx-proxy.conf` | предыдущий этап, сохранён |
| `deploy/nginx.conf` | предыдущий этап, сохранён |
| `deploy/postgres-init.sh` | предыдущий этап, сохранён |
| `deploy/rhythm-backup.service` | предыдущий этап, сохранён |
| `deploy/rhythm-backup.timer` | предыдущий этап, сохранён |
| `deploy/rhythm.service` | предыдущий этап, сохранён |
| `deploy/setup-web.sh` | предыдущий этап, сохранён |
| `docs/GENA_NLU_REPORT.md` | добавлен в этом этапе |
| `docs/RUSSIAN_NLU.md` | существовал untracked, расширен |
| `internal/api/proxy_test.go` | предыдущий этап, сохранён |
| `internal/assistant/clarification.go` | существовал untracked, расширен |
| `internal/calendar/recurrence.go` | добавлен в этом этапе |
| `internal/domain/recurrence.go` | добавлен в этом этапе |
| `internal/domain/recurrence_test.go` | добавлен в этом этапе |
| `internal/nlp/command.go` | существовал untracked, расширен |
| `internal/nlp/command_test.go` | существовал untracked, расширен |
| `internal/nlp/recurrence.go` | добавлен в этом этапе |
| `internal/nlp/recurrence_test.go` | добавлен в этом этапе |
| `internal/nlp/relative_time.go` | существовал untracked, расширен |
| `internal/storage/postgres/migrations/003_remove_vk.sql` | добавлен в этом этапе |
| `tests/contract/identity_test.go` | добавлен в этом этапе |
| `tests/contract/recurrence_test.go` | добавлен в этом этапе |
| `tests/contract/russian_nlu_test.go` | предыдущий этап, сохранён |
| `web/e2e/recurrence.spec.ts` | добавлен в этом этапе |
| `web/e2e/russian-nlu.spec.ts` | предыдущий этап, сохранён |
| `web/src/features/events/RecurrenceEditor.tsx` | добавлен в этом этапе |
| `web/src/features/events/recurrence.test.tsx` | добавлен в этом этапе |
| `web/src/features/events/recurrence.ts` | добавлен в этом этапе |

## VK

- Удалены provider package, серверная регистрация, маршруты start/callback, OpenAPI endpoints, VK token/profile exchange, device_id handling, специфичная PKCE ветка, frontend кнопка/тип/states/import/CSS, linking option, логотип/лицензия и CSP origins. Общая OAuth-инфраструктура и Яндекс сохранены.
- Удалены `VK_CLIENT_ID`, `VK_CLIENT_SECRET`, `VK_REDIRECT_URI` из env parsing, env examples, локального .env, Compose, deployment и Playwright config. `VK_APP_ID` и другие VK_* не обнаружены.
- Отдельных Go/npm зависимостей только для VK не было. `go mod tidy` выполнен; pnpm install --frozen-lockfile подтвердил актуальность package.json/pnpm-lock. VK SDK в dependency graph отсутствует; package-lock в проекте не используется.
- Добавлена **новая** PostgreSQL migration `003_remove_vk.sql`: удаление только retired provider identities/state и ограничение provider до email/yandex. Старые migrations не менялись. Аккаунты, сессии, события, OAuth/PKCE infrastructure и пользовательский контент сохраняются.
- Существующий аккаунт только с VK больше не имеет этого способа входа; для сохранения возможности входа ему нужен email/Яндекс identity до применения migration в production.
- VK упоминания остаются только в migration history/removal migration, migration documentation, проверках отсутствия и несвязанных строках вида privkey/checksum. Каждый текстовый результат классифицирован ниже.

## Branding

- Гена: sidebar, login/registration/mobile brand, splash, onboarding, settings, speech learning, notification settings, пользовательские тексты integrations, Python reference UI и default help/assistant identity («Я Гена»). Склонения проверены: «Гене», «Гену», «с Геной»; сложные предложения перестроены.
- Ассистент Гена: browser title, metadata Python/Go API, Swagger, onboarding description, документация и Yandex OAuth instructions, новый календарь/CalDAV PRODID. Уже записанная системная коллекция со старым именем переименовывается при sync через PROPPATCH; пользовательские event titles и пользовательские имена календарей сохраняются.
- Отдельного PWA manifest/install flow в проекте нет; существует только notification service worker. Ложные manifest/name/short_name файлы не добавлялись.
- DB migration для branding не потребовалась: системных brand seed/column нет. Пользовательский текст «Пользовательский Ритм» сохранён regression test. Новые env variables для бренда не добавлялись.
- Технические identifiers сохранены: module/package names, RhythmEvent, cookie/storage/BroadcastChannel keys, UID/ресурсы CalDAV, default dev email/DB users/volumes, Docker/systemd/deployment paths. Они не являются пользовательским названием продукта.
- Сохранённые historical docs/reference screenshots могут содержать старое имя. Актуальные README screenshots, auth/onboarding screenshots, SVG cover и diagram titles обновлены.

## NLU, recurrence и execution

Составной interval разбирается последовательностью numeral/unit/conjunction spans. Все компоненты после одного «через» дают один срок. Календарные месяцы/годы суммируются отдельно; сначала months, затем days/weeks, затем elapsed time. День при разовом month/year shift ограничивается концом месяца (31 января → 28/29 февраля).

`domain.Recurrence`: frequency, interval, weekdays, day_of_month, month, local_time, IANA timezone, серверная anchor. Частоты minute/hour — elapsed, остальные — local calendar. Структура сохраняется в существующем events.details JSON PostgreSQL/SQLite и Command/pending; новой колонки/migration для recurrence не нужно. Domain validation, API create/read/patch/null removal, OpenAPI, frontend types, редактор и display label согласованы.

Next(after) вычисляет строго будущий occurrence от стабильной anchor. Месячные/годовые правила пропускают отсутствующие даты, сохраняя 31-е/29 февраля. DST forward сдвигает пропущенный час вперёд; DST backward выбирает первый момент календарного часа. Точный UTC occurrence сохраняется, включая разовые относительные интервалы в повторный час.

Due совместим с прежним polling: транзакционно продвигает ряд и сохраняет последний сигнал; пятиминутный replay ответа API дедуплицируется браузером по UTC instant. Параллельные polls не продвигают одну occurrence несколько раз. Перезапуск не теряет anchor/следующий момент/replay. Старые пропущенные сигналы не вызывают backlog flood.

Delete/complete действуют на всю запись, останавливая серию. Restore/reopen сохраняют правило и anchor, выбирают следующий будущий момент. Reschedule меняет серию целиком: anchor/time; date меняет monthly/yearly selectors или одиночный weekday. Multiple weekdays требуют дату внутри набора либо отдельную правку правила. Обычный save без смены schedule сохраняет фазу и секунды.

Поддержаны все запрошенные составные примеры, месяц/два месяца/полгода/год/два года, дата следующего месяца, daily/weekly/multiple weekdays/workdays/weekends/every N minutes/every N hours/monthly/yearly. «Напомни проверить встречи» сохраняет reminder intent; «создай задачу» — task. Action text очищается по spans, в том числе при перестановках и командах с директивой в конце.

Clarification: recurring Monday/monthly/yearly без часов спрашивает только время; weekly без weekday спрашивает день; pending сохраняет intent/action/rule/date/time/timezone и точный момент. Fully specified recurrence создаётся сразу, без вопроса о lead time. Конфликты/отрицания не выполняют mutation.

## Тесты и результаты

- **180 новых parser cases**: 179 table-driven + отдельная дата следующего месяца. Старые регрессии сохранены. Прежний отказ для compound expression обновлён: исходный пример теперь успешен, conflicting interval остаётся negative case.
- **10 новых contract test functions / 34 сценария**: NLU, SQLite/PostgreSQL/API/read-back, restart, pending, canonical DST, duplicate advancement/drift, lifecycle/reschedule; 3 из функций посвящены removal/branding.
- **9 новых E2E tests**: семь обязательных команд, reminder lifecycle/API/редактор и recurring task в Tasks/Calendar. Старые 56 E2E сохранены.
- **8 новых frontend unit cases**: recurrence labels/editor (6), delivery key в DST (1), recurring task display (1). Existing branding expectations обновлены в App/Auth/E2E, проверены title, sidebar, login, onboarding, integration copy; VK assertions изменены на отсутствие.
- Provider HTTP tests сохранены для Яндекса; VK-specific protocol tests удалены. Добавлены route/OpenAPI/config/migration removal regressions.

| Проверка | Результат |
| --- | --- |
| go test -count=1 ./... | PASS; 660 test/subtest pass records, 0 пропущенных test cases |
| go test -race -count=1 ./... | PASS; Windows CGO + GCC UCRT64 |
| PostgreSQL full integration suite | PASS в обоих Go прогонах; TEST_DATABASE_URL задан, tests не пропущены |
| Python pytest | 73 passed |
| Node/voice | 6 passed |
| frontend unit | 54 passed |
| TypeScript tsc --noEmit | PASS |
| frontend production build | PASS |
| full E2E | 65 passed (полный финальный прогон); прежние полные прогоны 64/64 также PASS |
| go vet / ruff / Compose config / git diff --check | PASS |
| go mod tidy / pnpm frozen lockfile | PASS; VK-only dependency не было |

Результаты проверяют текущую локальную реализацию и HTTP protocol mocks; реальный consent/login аккаунтом Яндекса и удалённый production deployment в этом этапе не выполнялись.

## Ограничения

- Recurring meetings не реализованы: нужны отдельные правила проверки конфликтов всего ряда.
- Нет last/ordinal weekday, holiday exceptions, COUNT/UNTIL, индивидуального completion occurrence и выражений с отдельной speech start date + recurrence. Нет отрицательных/нулевых календарных components; доли допустимы для elapsed relative units, interval recurrence целый.
- Разовые absolute dates сохраняют прежнюю DST-семантику time.Date; явная новая политика действует для relative shifts и recurrence.
- Reminder execution остаётся polling при открытом приложении. Нет отдельного worker/Web Push; CalDAV экспортирует текущий occurrence без RRULE, поэтому серия не продвигается независимо при закрытом приложении.
- Due сканирует open records пользователя; архитектура личного планировщика не заменена очередью большого масштаба. Все UI record operations остаются операциями над серией целиком.

## Финальный глобальный поиск

Поиск выполнен по VK/vk/VK ID/vkid/VK_CLIENT/VK_APP/id.vk.com/oauth.vk.com и Ритм/ритм/РИТМ/Ritm/ritm/Rhythm/rhythm. Локальный .env отдельно проверен по именам ключей: VK_* отсутствуют. Актуальных VK auth endpoints/config/UI/SDK и старого пользовательского бренда в production UI нет. Ниже все текстовые результаты исходного repository search (собственный отчёт об удалении не включён, чтобы не создавать самопополняющуюся таблицу).

Сохранённые исторические media находятся в docs/screenshots/p1, p1-1 и reference.png/p1-2; они документируют прошлые этапы и не являются runtime UI. Современные docs/screenshots/current и actual auth/onboarding images пересняты.

| Файл | Строки | Причина сохранения |
| --- | --- | --- |
| `.env.example` | 2, 3, 5, 27 | technical identifier (compatibility) |
| `.env.go.example` | 2, 3, 5, 23 | technical identifier (compatibility) |
| `.github/workflows/ci.yml` | 28, 31, 38, 39, 46, 47, 52, 55, 98, 99, 104, 107 | technical identifier (compatibility) |
| `Dockerfile.python` | 6, 7, 12, 13 | technical identifier (compatibility) |
| `README.md` | 98 | technical identifier (compatibility) |
| `app/static/app.js` | 5, 6, 352, 381, 383, 436, 464, 494 | technical identifier (compatibility) |
| `app/static/architecture.html` | 43 | unrelated Russian word/slogan |
| `app/static/index.html` | 52 | unrelated Russian word/slogan |
| `cmd/import-sqlite/main.go` | 44 | technical identifier (compatibility) |
| `compose.python.yaml` | 18, 21 | technical identifier (compatibility) |
| `compose.yaml` | 7, 26, 27, 30, 41, 44, 45 | technical identifier (compatibility) |
| `deploy/README.md` | 99 | migration documentation |
| `deploy/README.md` | 10, 13, 14, 15, 18, 31, 37, 47, 48, 53, 54, 55, 58, 59, 60, 69, 70, 72, 74, 75, 76 | technical identifier (compatibility) |
| `deploy/VALIDATION.md` | 4, 14, 16 | technical identifier (compatibility) |
| `deploy/backup.sh` | 4, 6, 7, 9, 10, 13, 14 | technical identifier (compatibility) |
| `deploy/compose.production.yaml` | 1, 13, 17, 51, 54, 55, 59, 89 | technical identifier (compatibility) |
| `deploy/harden-host.sh` | 5, 6, 7, 8, 9, 25, 36, 53, 59, 74 | technical identifier (compatibility) |
| `deploy/nginx.conf` | 1, 2, 3, 4, 8, 47, 56, 58, 59, 70, 71, 72, 73, 76, 77, 80 | technical identifier (compatibility) |
| `deploy/nginx.conf` | 43 | unrelated checksum/key filename |
| `deploy/postgres-init.sh` | 4 | technical identifier (compatibility) |
| `deploy/rhythm-backup.service` | 2, 3, 8 | technical identifier (compatibility) |
| `deploy/rhythm-backup.timer` | 2 | technical identifier (compatibility) |
| `deploy/rhythm.service` | 2, 10, 11, 12 | technical identifier (compatibility) |
| `deploy/setup-web.sh` | 3, 6, 9, 26, 27 | technical identifier (compatibility) |
| `docs/ARCHITECTURE.md` | 34 | unrelated Russian word/slogan |
| `docs/GO_PARITY.md` | 77 | technical identifier (compatibility) |
| `docs/MIGRATION_PLAN.md` | 1, 81 | historical documentation |
| `docs/P1_1_DESIGN_PARITY.md` | 47 | historical documentation |
| `docs/P1_2_AUTH_POSTGRES.md` | 56 | migration documentation |
| `docs/P1_2_AUTH_POSTGRES.md` | 68, 83 | technical identifier (compatibility) |
| `docs/P1_SCREENSHOTS.md` | 1 | historical documentation |
| `docs/P1_UI.md` | 12 | technical identifier (compatibility) |
| `docs/PYTHON_REFERENCE.md` | 1, 11, 50, 76, 78, 80 | historical documentation |
| `docs/YANDEX_CALENDAR.md` | 43 | technical identifier (compatibility) |
| `docs/assets/readme-cover.svg` | 15 | unrelated Russian word/slogan |
| `docs/diagrams/architecture.svg` | 49 | technical identifier (compatibility) |
| `go.sum` | 29, 37, 74 | unrelated checksum/key filename |
| `internal/api/auth.go` | 18, 290, 297, 300 | technical identifier (compatibility) |
| `internal/config/config.go` | 55 | technical identifier (compatibility) |
| `internal/integrations/oauth/provider_test.go` | 21, 36 | technical identifier (compatibility) |
| `internal/integrations/yandexcalendar/caldav.go` | 154, 208, 259 | legacy system calendar recognition / regression |
| `internal/integrations/yandexcalendar/caldav_test.go` | 19, 44, 102 | legacy system calendar recognition / regression |
| `internal/integrations/yandexcalendar/ical.go` | 33, 34 | technical identifier (compatibility) |
| `internal/integrations/yandexcalendar/service.go` | 43 | technical identifier (compatibility) |
| `internal/storage/postgres/migrations/001_initial.sql` | 9 | migration history/removal migration |
| `internal/storage/postgres/migrations/003_remove_vk.sql` | 2, 3 | migration history/removal migration |
| `package.json` | 2 | technical identifier (compatibility) |
| `pyproject.toml` | 2 | technical identifier (compatibility) |
| `scripts/build_diagrams.py` | 73 | technical identifier (compatibility) |
| `scripts/smoke_auth.py` | 32 | technical identifier (compatibility) |
| `tests/contract/auth_postgres_test.go` | 83 | technical identifier (compatibility) |
| `tests/contract/identity_test.go` | 16, 22, 32, 34, 38, 42, 53, 79 | removal regression test / compatibility test |
| `tests/contract/identity_test.go` | 30, 58, 70 | user content preservation regression |
| `tests/contract/import_postgres_test.go` | 17 | technical identifier (compatibility) |
| `tests/contract/oauth_http_postgres_test.go` | 44, 68 | technical identifier (compatibility) |
| `tests/contract/profile_postgres_test.go` | 61, 64, 70 | technical identifier (compatibility) |
| `tests/contract/yandex_calendar_postgres_test.go` | 149, 156, 162, 183, 204, 210 | technical identifier (compatibility) |
| `web/e2e/account-profile.spec.ts` | 33 | removal regression test / compatibility test |
| `web/e2e/auth.spec.ts` | 89 | removal regression test / compatibility test |
| `web/e2e/auth.spec.ts` | 98 | technical identifier (compatibility) |
| `web/e2e/auth.spec.ts` | 53 | unrelated Russian word/slogan |
| `web/e2e/design.spec.ts` | 4, 50 | technical identifier (compatibility) |
| `web/e2e/flows.spec.ts` | 4, 8, 24 | technical identifier (compatibility) |
| `web/e2e/layout-recovery.spec.ts` | 4, 106, 118 | technical identifier (compatibility) |
| `web/e2e/motion.spec.ts` | 4, 44 | technical identifier (compatibility) |
| `web/e2e/recurrence.spec.ts` | 3, 36, 44 | technical identifier (compatibility) |
| `web/e2e/reminder-planning.spec.ts` | 54, 67, 71 | technical identifier (compatibility) |
| `web/e2e/speech.ts` | 3, 16 | technical identifier (compatibility) |
| `web/e2e/voice.spec.ts` | 11, 25, 35, 45, 48, 49, 51, 59, 62 | technical identifier (compatibility) |
| `web/package.json` | 2 | technical identifier (compatibility) |
| `web/playwright.config.ts` | 9 | technical identifier (compatibility) |
| `web/pnpm-lock.yaml` | 73, 104, 118, 432, 496, 518, 560, 698, 701 | unrelated checksum/key filename |
| `web/src/api/client.ts` | 1, 4, 12 | technical identifier (compatibility) |
| `web/src/api/events.ts` | 2, 4, 5, 17, 18, 20 | technical identifier (compatibility) |
| `web/src/app/App.test.tsx` | 82, 93, 100 | technical identifier (compatibility) |
| `web/src/app/Auth.test.tsx` | 12 | removal regression test / compatibility test |
| `web/src/app/AuthContext.tsx` | 12, 13, 14 | technical identifier (compatibility) |
| `web/src/app/DataContext.tsx` | 6, 9, 10, 11, 12, 13, 17, 34 | technical identifier (compatibility) |
| `web/src/app/EventActionsContext.tsx` | 9, 12, 13, 17, 19 | technical identifier (compatibility) |
| `web/src/app/PreferencesContext.tsx` | 10 | technical identifier (compatibility) |
| `web/src/features/assistant/ChatEventCard.tsx` | 6, 9 | technical identifier (compatibility) |
| `web/src/features/assistant/useAssistant.ts` | 11 | technical identifier (compatibility) |
| `web/src/features/calendar/DayEvent.tsx` | 6, 7 | technical identifier (compatibility) |
| `web/src/features/events/EventEditor.tsx` | 9, 12, 16 | technical identifier (compatibility) |
| `web/src/features/events/EventGroup.tsx` | 5, 6 | technical identifier (compatibility) |
| `web/src/features/events/EventMenu.tsx` | 5, 7, 11 | technical identifier (compatibility) |
| `web/src/features/events/EventRow.tsx` | 9, 10, 16 | technical identifier (compatibility) |
| `web/src/features/events/TaskCard.tsx` | 13, 15 | technical identifier (compatibility) |
| `web/src/features/events/presentation.ts` | 1, 4, 5 | technical identifier (compatibility) |
| `web/src/features/notifications/ReminderNotifications.tsx` | 10, 15, 26 | technical identifier (compatibility) |
| `web/src/features/notifications/notifications.test.ts` | 24 | technical identifier (compatibility) |
| `web/src/features/notifications/notifications.ts` | 1, 3, 26, 28, 41, 44, 52 | technical identifier (compatibility) |
| `web/src/features/widgets/Widgets.tsx` | 14, 27 | technical identifier (compatibility) |
| `web/src/hooks/useEventSearch.ts` | 6, 9 | technical identifier (compatibility) |
| `web/src/pages/Archive/ArchivePage.tsx` | 14, 16, 17, 31 | technical identifier (compatibility) |
| `web/src/pages/Integrations/useYandexIntegration.ts` | 4 | technical identifier (compatibility) |
| `web/src/pages/Reminders/RemindersPage.tsx` | 11, 25, 39, 47 | technical identifier (compatibility) |
| `web/src/pages/Tasks/TasksPage.tsx` | 9, 14 | technical identifier (compatibility) |
| `web/src/test/server.ts` | 1, 3 | technical identifier (compatibility) |
| `web/src/types/api.ts` | 9, 24, 29, 30 | technical identifier (compatibility) |
