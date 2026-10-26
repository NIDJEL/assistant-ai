# Запуск и настройка

[README](../README.md) · [Материалы для организаторов](../SUBMISSION.md)

## Требования

- Docker Desktop/Engine с Compose v2 и работающим daemon.
- Python 3.10+; на Linux команда обычно называется `python3`.
- Интернет для первой сборки. Go/Node/pnpm на хосте не требуются.
- Свободный локальный порт 8081. Для телефона используйте HTTPS-прототип, а не `localhost` другого компьютера.

## Без ключей: одна команда

Из корня клонированного репозитория:

```powershell
python scripts/service.py up
```

На Linux/macOS: `./scripts/up.sh` или `python3 scripts/service.py up`. На Windows также доступен `.\scripts\up.ps1`.

Создаётся `.env` на основе `.env.example`, два независимых случайных секрета и VAPID-пара. Сборка выполняется внутри Docker, миграции PostgreSQL применяет приложение. После `Health: ok` откройте http://localhost:8081 и зарегистрируйтесь. Аккаунт пуст; демонстрационный CSV загружается через настройки по явному действию.

Повторный `up` сохраняет секреты, аккаунты, записи и том. Изменение пароля в `.env` для уже созданной базы не меняет пароль роли PostgreSQL: не заменяйте сохранённый `POSTGRES_PASSWORD` при обычном обновлении.

## Полный режим: GigaChat, голос и интеграции

Сначала заполните соответствующие поля в `.env`:

| Функция | Настройки |
| --- | --- |
| Свободный текст | `GIGACHAT_AUTHORIZATION_KEY`, при необходимости `GIGACHAT_CA_FILE_HOST`; остальные GigaChat defaults уже заданы. |
| Диктовка | `GROQ_API_KEY`; при необходимости `GROQ_PROXY_URL`. |
| Озвучивание | `AZURE_SPEECH_KEY`, `AZURE_SPEECH_REGION`; голос по умолчанию `ru-RU-DmitryNeural`. |
| Яндекс | `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET`, `YANDEX_REDIRECT_URI`. |

```bash
python scripts/service.py up --mode full
```

Launcher включает `simple_llm`/GigaChat. Если заданы Groq/Azure credentials, включает соответствующий голосовой сервис. При `GIGACHAT_CA_FILE_HOST` создаёт отдельный Compose override и монтирует проверенный CA read-only. Пример Windows-пути: `D:/certs/gigachat-ca.pem`. Проверка TLS не отключается.

Внешние ключи не создаются скриптом. OAuth-приложения нужно зарегистрировать у провайдеров, задать callback `<PUBLIC_URL>/api/auth/yandex/callback`, а затем явно подключить аккаунт из интерфейса. Для доступа с телефона, Web Push и внешних callback нужен корректный HTTPS origin. [GigaChat](GIGACHAT_CONFIGURATION.md) · [Голос](DEADLINES_AND_VOICE.md) · [Azure](AZURE_TTS.md) · [Яндекс](YANDEX_CALENDAR.md).

## Независимые установки и управление

```bash
python scripts/service.py up --env-file .env.review --project rhythm-review --port 18086
python scripts/service.py check --env-file .env.review --project rhythm-review
python scripts/service.py status --env-file .env.review --project rhythm-review
python scripts/service.py logs --env-file .env.review --project rhythm-review
python scripts/service.py down --env-file .env.review --project rhythm-review
```

Порт выбирается при создании нового env. Для уже существующего файла меняйте `APP_HTTP_PORT` и `PUBLIC_URL` согласованно. Нельзя незаметно превратить существующую AI-конфигурацию в demo: выберите отдельный env или явно настройте нужный режим.

`--no-build` использует уже собранный образ и подходит для быстрого повторного запуска; после изменения кода выполняйте обычный `up`. Скрипт не печатает env и не удаляет тома. Env-файлы и `var/runtime` исключены из Git. Файлы создаются с ограниченными правами, где это поддерживает ОС; на общем Windows-компьютере настройте ACL для `.env`.

## Частые проблемы

| Симптом | Решение |
| --- | --- |
| Docker не найден / daemon недоступен | Установите и запустите Docker Desktop/Engine, проверьте `docker info`. |
| Не скачивается базовый образ | Проверьте доступ к Docker Hub/GCR и сетевые настройки Docker. Первый запуск требует загрузки образов. |
| Порт занят | Создайте независимую установку с другим `--port`, как в примере выше. |
| Полный режим не стартует | Проверьте ключ GigaChat, наличие CA-файла и сохранённый env; смотрите `logs`. |
| Нет микрофона | Используйте localhost/HTTPS и разрешите микрофон. Ввод текста остаётся доступен. |
| Нет OAuth-подключения | Проверьте регистрацию приложения, scopes, callback и `PUBLIC_URL`; ключа одного провайдера недостаточно для другого. |
| Нет уведомлений на телефоне | Проверьте разрешения, VAPID и требования устройства; [Web Push](WEB_PUSH.md). |

## Production

Эта команда разворачивает локальный комплект приложения и PostgreSQL. Текущий production выпускается через [CI/CD](../deploy/CI_CD.md). Установка HTTPS, Nginx, системных сервисов и серверных секретов описана отдельно в [deploy/README.md](../deploy/README.md): launcher не меняет SSH/firewall и не получает root-доступ автоматически.
