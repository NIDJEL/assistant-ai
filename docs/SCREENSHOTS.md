# Актуальные скриншоты

[README](../README.md) · [Документация](README.md)

Съёмка подготовлена 26 октября 2026 года через настоящий интерфейс локального Go/React-приложения с PostgreSQL. Все аккаунты, имена и записи демонстрационные. API не подменяется; внешние интеграции не выдаются за подключённые. Режим на снимках — правила, поэтому изображения не являются оценкой качества GigaChat или микрофона.

## Галерея

| Экран | Скриншот |
| --- | --- |
| Вход | [auth.png](screenshots/current/auth.png) |
| Onboarding | [onboarding.png](screenshots/current/onboarding.png) |
| Чат | [assistant.png](screenshots/current/assistant.png) |
| Три команды у «+» | [quick-commands.png](screenshots/current/quick-commands.png) |
| Календарь | [calendar.png](screenshots/current/calendar.png) |
| Задачи | [tasks.png](screenshots/current/tasks.png) |
| Встречи | [meetings.png](screenshots/current/meetings.png) |
| Напоминания | [reminders.png](screenshots/current/reminders.png) |
| Друзья | [friends.png](screenshots/current/friends.png) |
| Интеграции | [integrations.png](screenshots/current/integrations.png) |
| Настройки | [settings.png](screenshots/current/settings.png) |
| Архив | [archive.png](screenshots/current/archive.png) |
| Светлая тема: чат | [assistant-light.png](screenshots/current/assistant-light.png) |
| Светлая тема: календарь | [calendar-light.png](screenshots/current/calendar-light.png) |
| Телефон: задачи | [mobile.png](screenshots/current/mobile.png) |
| Телефон: календарь | [mobile-calendar.png](screenshots/current/mobile-calendar.png) |
| Телефон: интеграция | [mobile-integration.png](screenshots/current/mobile-integration.png) |
| Телефон: команды | [mobile-commands.png](screenshots/current/mobile-commands.png) |
| Ошибка 404 | [error-404.png](screenshots/current/error-404.png) |
| Ошибка 503 | [error-503.png](screenshots/current/error-503.png) |

Desktop: 1440×1000; mobile: 390×844; `ru-RU`, `Europe/Moscow`, reduced motion. Файлы, маршруты, размеры, время съёмки и SHA256 перечислены в [manifest.json](screenshots/current/manifest.json). `source_commit` вместе с `working_tree_changes` описывает состояние рабочей копии при запуске скрипта.

## Воспроизведение

Поднимите отдельный локальный сервис по [QUICKSTART](QUICKSTART.md). Для съёмки, в отличие от запуска контейнеров, нужны Node.js 24 и pnpm:

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web exec playwright install chromium
pnpm --dir web exec node scripts/capture-readme.mjs
```

По умолчанию используется `http://localhost:8081`; другой локальный адрес задаётся в `P1_BASE_URL`. Скрипт создаёт новые аккаунты с явным согласием текущей версии, реальные записи и дружеские связи, получает ответ настоящего текстового ассистента и сохраняет снимки. Ключи и пароли не записываются в manifest.

Съёмка удалённой установки по умолчанию запрещена. Для production используйте отдельную процедуру и явное разрешение: обычное обновление скриншотов не должно создавать данные на рабочем сервере.
