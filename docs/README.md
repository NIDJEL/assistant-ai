# Документация Ассистента Гены

Руководства описывают код и конфигурацию в репозитории. Они не подтверждают состояние конкретного сервера или результаты последнего CI.

| Задача | Руководство |
| --- | --- |
| Запустить приложение и познакомиться с возможностями | [README проекта](../README.md) |
| Понять компоненты и границы транзакций | [Архитектура](ARCHITECTURE.md) |
| Работать с диалогом, выбором и подтверждениями | [Поведение ассистента](ASSISTANT_BEHAVIOR.md) |
| Настроить GigaChat локально или в Compose | [GigaChat](GIGACHAT_CONFIGURATION.md) |
| Изменить контракт LLM и компактный контекст | [Simple LLM runtime](SIMPLE_LLM_RUNTIME.md) |
| Настроить Groq, голосовой ввод и дедлайны | [Дедлайны и голос](DEADLINES_AND_VOICE.md) |
| Разобраться с сессиями, OAuth и импортом SQLite | [Аккаунты и данные](ACCOUNTS.md) |
| Подключить одностороннюю синхронизацию календаря | [Яндекс Календарь](YANDEX_CALENDAR.md) |
| Настроить фоновые уведомления и PWA | [Web Push](WEB_PUSH.md) |
| Разрабатывать, запускать тесты и обновлять скриншоты | [Разработка](DEVELOPMENT.md) |
| Обновить production, сделать backup или откат | [Эксплуатация](../deploy/README.md) |

## Где проверять настройки

- Переменные окружения и их значения по умолчанию: [config.go](../internal/config/config.go), [.env.example](../.env.example).
- Локальные контейнеры: [compose.yaml](../compose.yaml).
- Production: [Compose](../deploy/compose.production.yaml), [GigaChat override](../deploy/compose.gigachat.example.yaml), [systemd](../deploy/rhythm.service).
- Версии инструментов: [go.mod](../go.mod), [package.json](../web/package.json), [Dockerfile](../Dockerfile).
- Обязательные автоматические проверки: [GitHub Actions](../.github/workflows/ci.yml).
- HTTP-контракт: `/openapi.json` и `/docs` после локального запуска.

История изменений доступна в Git. Результаты конкретного прогона следует искать в CI или его артефактах; старые отчёты не служат инструкцией для текущего runtime.
