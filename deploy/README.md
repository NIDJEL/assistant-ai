# Production: развёртывание и эксплуатация

[← Документация](../docs/README.md)

Руководство описывает файлы развёртывания в репозитории. Состояние конкретного хоста, установленные сертификаты и успешность последнего выпуска проверяются на самом сервере.

## Состав

| Файл | Назначение |
| --- | --- |
| `compose.production.yaml` | Go/React-контейнер, PostgreSQL 17, сеть, постоянный том и ограничения ресурсов. |
| `compose.gigachat.example.yaml` | Environment GigaChat, режим simple_llm, CA mount. |
| `rhythm.service` | Запуск двух Compose-файлов из `/opt/rhythm/current`. |
| `nginx.conf`, `nginx-proxy.conf` | HTTPS, proxy, ограничения запросов и voice upload. |
| `postgres-init.sh` | Создание отдельной роли приложения при инициализации пустой базы. |
| `backup.sh`, `rhythm-backup.service`, `rhythm-backup.timer` | PostgreSQL dump, архив конфигурации и ежедневный таймер. |
| `setup-web.sh`, `harden-host.sh` | Подготовка конкретного Linux-хоста; перед применением прочитайте и адаптируйте параметры. |
| `gigachat_ops.py` | Служебные операции GigaChat-развёртывания; приватный JSON принимается через stdin. |

Compose project — `rhythm`, постоянный PostgreSQL-том — `rhythm-postgres-production`. Приложение опубликовано на `127.0.0.1:8081`; порт базы наружу не публикуется. Контейнер приложения работает без root, с read-only filesystem и ограничениями ресурсов.

## Подготовка

Нужен Linux-хост с Docker Compose v2, Nginx, сертификатом HTTPS и доступом к внешним провайдерам. Исходники релизов размещаются в `/opt/rhythm/releases/<release>`, `/opt/rhythm/current` указывает на активный каталог.

Файлы Nginx и setup-web содержат домен конкретной установки. При переносе замените `server_name`, HTTPS redirect и пути сертификатов. `harden-host.sh` меняет SSH, firewall и системные настройки: адаптируйте его к существующему доступу администратора.

Создайте `/etc/rhythm/production.env` с доступом только root (0600):

| Переменная | Назначение |
| --- | --- |
| `POSTGRES_DB`, `POSTGRES_USER` | База и отдельная роль приложения, по умолчанию rhythm. |
| `POSTGRES_PASSWORD` | Пароль роли приложения; для Compose URL удобно случайное hex-значение. |
| `POSTGRES_ADMIN_PASSWORD` | Независимый пароль PostgreSQL administrator. |
| `SESSION_SECRET` | Постоянный случайный секрет длиной от 32 символов. |
| `PUBLIC_URL` | HTTPS origin приложения. |
| `RELEASE_TAG` | Тег образа текущего релиза. |
| `GIGACHAT_AUTHORIZATION_KEY` | Credential GigaChat. |
| `GIGACHAT_CA_FILE_HOST` | Абсолютный Linux-путь к проверенному CA bundle. |
| `GENA_NLU_MODE`, `GIGACHAT_MODEL` | Для AI: simple_llm и GigaChat-3-Ultra. |

При необходимости добавьте Groq, Яндекс OAuth и VAPID из `.env.example`. Windows-пути к CA на сервере не работают. Приватные значения не размещайте в репозитории и аргументах команд.

Базовый production Compose без GigaChat override использует legacy rules. Штатный `rhythm.service` подключает override и требует GigaChat credential/CA. Для эксплуатации без AI нужно согласованно изменить список Compose-файлов сервиса и явно выбрать режим правил.

## Первый запуск

Из каталога подготовленного релиза:

```sh
sudo docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml -f deploy/compose.gigachat.example.yaml config --quiet
sudo docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml -f deploy/compose.gigachat.example.yaml build --pull app
```

Проверьте и установите Nginx-конфигурацию и сертификаты, создайте symlink `/opt/rhythm/current`, затем установите сервис:

```sh
sudo install -m 644 deploy/rhythm.service /etc/systemd/system/rhythm.service
sudo systemctl daemon-reload
sudo systemctl enable --now rhythm
```

Для первого старта PostgreSQL используется init-скрипт. Изменение паролей в env существующего тома не меняет роли базы автоматически.

Nginx перезаписывает `X-Real-IP`; Compose доверяет только gateway `172.30.81.1/32`. При изменении сети обновите `TRUSTED_PROXY_CIDRS`. Voice location разрешает 16 МиБ multipart и отключает request buffering; общий лимит остальных маршрутов сохраняется. Технические маршруты `/docs`, `/architecture`, `/diagrams`, `/openapi.json` закрыты Nginx.

## Проверка и логи

```sh
sudo systemctl status rhythm nginx
sudo docker compose --env-file /etc/rhythm/production.env -f /opt/rhythm/current/deploy/compose.production.yaml -f /opt/rhythm/current/deploy/compose.gigachat.example.yaml ps
curl -fsS http://127.0.0.1:8081/health
sudo docker compose --env-file /etc/rhythm/production.env -f /opt/rhythm/current/deploy/compose.production.yaml -f /opt/rhythm/current/deploy/compose.gigachat.example.yaml logs --tail 100 app
```

Проверьте также публичный HTTPS `/health`, вход, создание/перенос дела и диктовку на реальном устройстве. OAuth и Web Push проверяются с пользовательским согласием на тестовом аккаунте. Health подтверждает готовность backend, но не качество распознавания или получение push телефоном.

Nginx logs: `/var/log/nginx/rhythm-access.log` и `rhythm-error.log`. Access format не включает query string и cookies. Не публикуйте обычный вывод `docker compose config`: он раскрывает подставленные env; для валидации используйте `config --quiet`.

## Резервные копии

Установите проверенный `backup.sh` в `/usr/local/sbin/rhythm-backup` и units из репозитория:

```sh
sudo install -m 700 deploy/backup.sh /usr/local/sbin/rhythm-backup
sudo install -m 644 deploy/rhythm-backup.service deploy/rhythm-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now rhythm-backup.timer
sudo systemctl start rhythm-backup.service
sudo journalctl -u rhythm-backup.service --no-pager -n 20
```

Таймер задан на 03:15 с задержкой до 10 минут в часовом поясе сервера. `backup.sh` сохраняет custom PostgreSQL dump и архив env/Nginx/сертификатов в `/var/backups/rhythm`, удаляя копии старше 14 дней. Пути в скрипте должны соответствовать установке.

Проверьте dump через `pg_restore --list` и восстановление в отдельной базе. Настройте внешнюю копию: backup на том же сервере не защищает от потери хоста. Архив конфигурации содержит секреты и закрытые ключи сертификатов.

## Обновление

Штатный путь — push в `main`: после успешного CI релиз выпускается автоматически. См. [CI/CD](CI_CD.md) для environment secrets, серверного receiver, backup и автоматического отката. Ниже остаётся ручной путь для первоначальной установки и аварийных работ.

1. Выполните проверки из [руководства разработчика](../docs/DEVELOPMENT.md), зафиксируйте Git SHA и подготовьте отдельный каталог релиза.
2. Сделайте backup PostgreSQL и сохраните предыдущую конфигурацию, тег образа и цель symlink.
3. Установите новый `RELEASE_TAG` в production.env. Из нового каталога выполните обе команды Compose проверки и сборки, приведённые выше.
4. Переключите `/opt/rhythm/current` на подготовленный каталог и выполните `sudo systemctl restart rhythm`.
5. Проверьте health, вход и основные действия. Сохраняйте предыдущий образ до завершения проверки.

Сохраняйте `SESSION_SECRET`, VAPID-пару и постоянный том. Не выполняйте `docker compose down -v` для рабочей установки.

## Откат

Верните symlink и `RELEASE_TAG` на предыдущий релиз, восстановите совместимую конфигурацию и перезапустите `rhythm`. Откат образа не отменяет миграции базы. Если старая версия несовместима со схемой, восстановление проверенного dump выполняется отдельно, после остановки записи и сохранения текущей базы.

Смена `GENA_NLU_MODE` на legacy внутри GigaChat override сохраняет GigaChat provider и включает прежний semantic planner. Для rules-only отката требуется также убрать override и явно выбрать rules.

[Настройка GigaChat](../docs/GIGACHAT_CONFIGURATION.md) · [Голос](../docs/DEADLINES_AND_VOICE.md) · [Web Push](../docs/WEB_PUSH.md) · [Яндекс](../docs/YANDEX_CALENDAR.md)
