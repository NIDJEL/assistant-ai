# Документация Гены

Документация соответствует текущему устройству проекта; материалы для сдачи подготовлены 26 октября 2026 года. Источник истины для конфигурации — код, Compose и CI. Проверка конкретного выпуска выполняется по SHA и Actions, а не по старым отчётам.

## Организаторам и пользователям

| Задача | Документ |
| --- | --- |
| Понять проект и увидеть интерфейс | [README](../README.md) |
| Пройти демонстрацию и проверить результат | [Материалы для организаторов](../SUBMISSION.md) |
| Поднять app + PostgreSQL одной командой | [Быстрый запуск](QUICKSTART.md) |
| Проверить задачи, встречи, CSV и контекст | [Сценарий проверки](ASSIGNMENT_REVIEW.md) |
| Посмотреть актуальные экраны | [Скриншоты и методика](SCREENSHOTS.md) |
| Понять, что проверялось автоматически | [Проверка готовности](ASSIGNMENT_VALIDATION.md) |
| Проверить реальный микрофон и телефон | [Ручная голосовая проверка](VOICE_MANUAL_CHECK.md) |

## Архитектура и разработка

Реализация и проверки: [улучшения после TOP-10](QUALITY5_IMPROVEMENTS.md), [исходный пакет TOP-10](TOP10_IMPLEMENTATION.md).

Положение после изменений: [Повторная оценка Гены и конкурентов](COMPETITIVE_REASSESSMENT_AFTER_TOP10.md).

| Тема | Документ |
| --- | --- |
| Блочные схемы, UML, ER и границы системы | [Архитектура](ARCHITECTURE.md) |
| Среда разработки, PostgreSQL, тесты, генерация материалов | [Разработка](DEVELOPMENT.md) |
| Диалог, контекст, выбор и подтверждения | [Поведение ассистента](ASSISTANT_BEHAVIOR.md) |
| Контракт операции и граница GigaChat/Go | [Simple LLM runtime](SIMPLE_LLM_RUNTIME.md) |
| Несколько действий, зависимости, уточнения и идемпотентность | [Multi-Action](MULTI_ACTION.md) |
| Проверенное сравнение 11 конкурсных проектов | [Технический разбор GORKYCODE 2026](GORKYCODE_2026_TECHNICAL_REVIEW.md) |
| Аккаунты, сессии, изоляция и импорт SQLite | [Аккаунты](ACCOUNTS.md) |
| Темы, навигация, команды и ошибки | [Интерфейс](WORKSPACE_DESIGN.md) |
| Авторизация и onboarding | [Вход](AUTH_VISUALS.md) |
| Компоненты ввода и доступность | [UI/UX](UI_UX_POLISH.md) |
| Правовые страницы и согласия | [Правовые документы](LEGAL.md) |

## Провайдеры и эксплуатация

| Тема | Документ |
| --- | --- |
| GigaChat и проверенный CA bundle | [Настройка GigaChat](GIGACHAT_CONFIGURATION.md) |
| Groq, диктовка и упреждение | [Голос и дедлайны](DEADLINES_AND_VOICE.md) |
| Azure TTS и воспроизведение | [Голос Гены](AZURE_TTS.md) |
| Друзья и подбор времени | [Совместное планирование](FRIENDS_AND_SCHEDULING.md) |
| Яндекс ID / односторонний экспорт | [Яндекс](YANDEX_CALENDAR.md) |
| Google Calendar и Zoom | [Облачные интеграции](GOOGLE_CALENDAR_ZOOM.md) |
| Zoom в диалоге и карточках | [Zoom в чате](ZOOM_CHAT.md) |
| Push, повторения, PWA | [Web Push](WEB_PUSH.md) |
| Production Compose, Nginx, backup и восстановление | [Эксплуатация](../deploy/README.md) |
| Автоматический выпуск main, receiver и rollback | [CI/CD](../deploy/CI_CD.md) |

## Файлы, задающие поведение

- [config.go](../internal/config/config.go), [.env.example](../.env.example) — параметры и значения по умолчанию.
- [service.py](../scripts/service.py), [compose.yaml](../compose.yaml) — проверенный локальный запуск.
- [go.mod](../go.mod), [web/package.json](../web/package.json), [Dockerfile](../Dockerfile) — инструменты и сборка.
- [CI workflow](../.github/workflows/ci.yml) — обязательные автоматические проверки.
- [Mermaid/SVG](diagrams/) — редактируемые исходники и готовые диаграммы.
- [Манифест скриншотов](screenshots/current/manifest.json) — дата, маршруты, размеры и SHA256.

Исторические отчёты и происхождение иллюстраций сохранены в [history](history/). Они явно отделены от текущих инструкций и результатов. Временные оценки, локальные секреты и артефакты экспериментов не входят в комплект для передачи.
