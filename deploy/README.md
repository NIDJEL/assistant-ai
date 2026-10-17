# Production: meinangelplatz.top

Сервер: `31.77.232.46`, Ubuntu 24.04 LTS. Приложение: https://meinangelplatz.top.

## Доступ администратора

На компьютере, с которого выполнено развертывание:

```powershell
ssh rhythm-prod
```

Пользователь: `rhythm-admin`, права администратора через `sudo`.
SSH-ключ: `C:\Users\nidjel\.ssh\rhythm_server_ed25519`.
Ключ сервера закреплен в `C:\Users\nidjel\.ssh\rhythm_known_hosts`.
Закрытый ключ ограничен правами Windows; храните его резервную копию в защищенном месте.
Вход root и вход по паролю отключены, пароль root заблокирован.
Новый административный ключ добавляется в `/home/rhythm-admin/.ssh/authorized_keys` через уже работающий доступ.
При утрате всех ключей понадобится консоль или rescue-режим провайдера.

## Что развернуто

- Nginx принимает HTTPS на 443; HTTP на 80 используется для ACME и перенаправления.
- Let’s Encrypt, ECDSA-сертификат, таймер `certbot.timer`, reload Nginx после успешного продления.
- Firewall разрешает только TCP 22, 80, 443; SSH имеет ограничение частоты соединений и Fail2ban.
- Go/React работает в контейнере без root, с файловой системой только для чтения, без Linux capabilities, с ограничениями CPU, памяти, процессов и логов.
- PostgreSQL 17 не публикует порт; пользователь приложения не имеет SUPERUSER, CREATEDB, CREATEROLE или REPLICATION.
- Backend опубликован только на `127.0.0.1:8081`; Docker-сеть `172.30.81.0/24` имеет фиксированный gateway.
- Nginx перезаписывает `X-Real-IP`; приложение доверяет этому заголовку только от gateway `172.30.81.1/32`. Изменяя сеть, обновите `TRUSTED_PROXY_CIDRS`.
- Попытки входа ограничены по IP в Nginx и приложении; дополнительный общий лимит входа защищает ресурсы при запросах с множества IP. Установлены HSTS, CSP, HttpOnly/Secure cookies, CSRF-проверки. Публичные страницы технической документации закрыты.
- Секреты созданы независимо от локальной разработки и находятся в `/etc/rhythm/production.env` (root, 0600).
- Обновления безопасности Ubuntu устанавливаются ежедневно. При необходимости сервер автоматически перезагружается в 04:30 по Москве.

## Проверка и обслуживание

### Web Push

14 октября 2026 Web Push настроен и развернут в production: релиз `20261014-web-push-01`. Постоянная VAPID-пара уже создана существующим генератором и сохранена в `/etc/rhythm/production.env`, root:root, 0600. Subject — `mailto:nikkruglov16@gmail.com`. Reminder worker работает внутри Go process. Сохраняйте эту пару при обновлениях и на всех экземплярах; повторно генерировать production keys не требуется.

Production compose передаёт эти значения backend. После backup применены migrations 003 и 004; subscriptions/outbox включены в PostgreSQL backup. Manifest/icons/notifications-sw.js доступны через существующий HTTPS proxy. Проверены backend health, worker startup, public key API, подписки, успешная зашифрованная доставка на synthetic receiver, retry/dead logic и отсутствие дублей из polling. [Production отчёт](../docs/WEB_PUSH.md), [настройка и device checklist](../docs/WEB_PUSH.md).

```sh
sudo systemctl status rhythm nginx certbot.timer fail2ban
sudo docker ps
curl -fsS https://meinangelplatz.top/health
sudo certbot certificates
sudo certbot renew --dry-run --run-deploy-hooks
sudo fail2ban-client status sshd
sudo ufw status verbose
sudo systemctl list-timers --all
```

Логи: `/var/log/nginx/rhythm-access.log`, `/var/log/nginx/rhythm-error.log`,
`sudo docker compose --env-file /etc/rhythm/production.env -f /opt/rhythm/current/deploy/compose.production.yaml logs --tail 100`.
HTTP access log не записывает query string, cookies и OAuth-коды.

## Резервные копии

`rhythm-backup.timer` запускает backup ежедневно около 03:15–03:25 по Москве.
В `/var/backups/rhythm` сохраняются PostgreSQL custom dump и архив настроек/сертификатов, доступ только root; срок хранения — 14 дней.
Первый комплект также скопирован на компьютер администратора в `C:\Users\nidjel\.codex\server-backups\rhythm`.

```sh
sudo systemctl start rhythm-backup.service
sudo journalctl -u rhythm-backup.service --no-pager -n 20
sudo ls -lh /var/backups/rhythm
```

Ежедневные копии на сервере не защищают от полной потери сервера: для этого нужно настроить отдельное внешнее хранилище.
Архив настроек содержит секреты и закрытые ключи сертификатов; передавайте и храните его защищенно.
Сначала проверяйте dump в отдельной базе. Восстановление в рабочую базу требует остановить запись приложения и сохранить копию текущих данных.

## Обновление приложения

Исходники релиза лежат в `/opt/rhythm/releases`, активный релиз — symlink `/opt/rhythm/current`.
Compose project `rhythm`, постоянный том `rhythm-postgres-production`.
Обновления контейнерных образов выполняются отдельно от автоматических обновлений Ubuntu, после проверки новой версии.
Для приведенных ниже действий войдите через `ssh rhythm-prod` и откройте root-shell командой `sudo -i`.

1. Создайте backup; загрузите новый исходный код в новый каталог `/opt/rhythm/releases/<release>`.
2. Соберите `app` из нового каталога с `RELEASE_TAG=<release> docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml build --pull app`.
3. Установите `RELEASE_TAG=<release>` в `/etc/rhythm/production.env`, переключите `/opt/rhythm/current` на новый релиз и выполните `systemctl restart rhythm`.
4. Проверьте health endpoint, регистрацию, вход и основные действия с календарем.

Не выполняйте `docker compose down -v`: эта команда удаляет рабочие данные.
Перед изменением схемы базы нужен backup; откат исходного кода не отменяет миграцию базы.
Для отката нужно восстановить предыдущие symlink и `RELEASE_TAG`, затем перезапустить сервис. Храните предыдущий образ до завершения проверки нового релиза.

## Яндекс OAuth

Скопированы существующие client ID/secret из локальной конфигурации.
В настройках приложения Яндекс OAuth необходимо зарегистрировать callback:

`https://meinangelplatz.top/api/auth/yandex/callback`

Для календаря требуется право `calendar:all`. До настройки callback используйте регистрацию и вход по email/паролю.
Фоновые Web Push реализованы и настроены. Восстановление пароля по email пока отсутствует.

## Границы защиты

Настройка уменьшает поверхность атаки и защищает доступ и передачу данных. Она не гарантирует отсутствие уязвимостей в приложении, зависимости или аккаунте провайдера. Защита от крупного DDoS требует возможностей провайдера или внешнего proxy/CDN.

## Обновление «Ассистент Гена»

Технические пути `/opt/rhythm`, systemd units, имя Compose и постоянные тома сохранены. Публичный бренд — Гена / Ассистент Гена. Настроек VK больше нет. Migration `003_remove_vk.sql` применена в production вместе с 004: только старые provider identities/state, без удаления аккаунтов или событий. До deployment проверено отсутствие пользователей с единственным VK identity. Recurrence использует существующие JSON metadata, отдельной migration для branding/повторений нет. Яндекс OAuth application name остаётся «Ассистент Гена».

## Единственная актуальная версия

Поддерживаемая версия указана в `VERSION`, исходники — в GitHub `main`.
Основа синхронизации 15 октября 2026: работавший релиз `20261014-reminder-fix-03`.
Новый каталог релиза содержит `REVISION` с полным SHA Git-коммита. Сверяйте его с `git rev-parse origin/main`.
Загружайте исходники через `git archive` зафиксированного коммита; локальные незакоммиченные файлы не входят в deployment.
Старые релизы упаковываются в `/var/backups/rhythm/source-archive-*` и удаляются из `/opt/rhythm/releases` после проверки активного приложения.
Для отката распакуйте нужный каталог, восстановите `RELEASE_TAG` и `/opt/rhythm/current`, затем обновите только сервис `app` через `docker compose up -d --no-deps --wait app`.
Постоянный том `rhythm-postgres-production`, секреты и VAPID-ключи сохраняются между обновлениями.

## GigaChat Ultra release

Текущая версия исходников — `20261015-gigachat-ultra-01`. В production используется hardened `deploy/compose.production.yaml` вместе с `deploy/compose.gigachat.example.yaml`; systemd unit включает оба файла. Runtime использует только Authorization Key из root-owned `/etc/rhythm/production.env` (0600), Ultra, scope PERS, timeout 30s и concurrency 1. Дополнительный CA находится по Linux-пути из `GIGACHAT_CA_FILE_HOST` и монтируется read-only. Windows-пути и credentials TXT на VPS не переносятся.

Перед выпуском: локальный audit tracked/untracked/index/history/artifacts/logs, один opt-in live check, тесты, один commit и обычный push. На VPS сначала read-only audit, затем штатный backup с проверкой `pg_restore --list`, сохранение предыдущего release/config и загрузка `git archive` pushed commit. Только после этого приватный JSON со значением нового ключа передаётся через SSH stdin; не через argv, YAML или image layer. При включённом sudo input logging перенос секретов блокируется.

`deploy/gigachat_ops.py` реализует audit/backup/prepare/build/activate/check/giga/rollback; stdout содержит только metadata/counts. Новые production env/config записываются атомарно с сохранением permissions. Перед сборкой всегда выполняется `config --quiet`. Обновляется только app через `up -d --no-deps --wait`, PostgreSQL и volumes не пересоздаются. На ошибке health/GigaChat/mandatory smoke выполняется rollback предыдущего release/config без возврата скомпрометированного GigaChat ключа и без отката пользовательских данных.

После выпуска проверяются HTTPS/SPA/API, отдельный test account, 13 mandatory и короткие bonus-сценарии, logout/login persistence, redacted logs и повторный secret audit. Frozen held-out не запускается. Реальный microphone input не считается проверенным, пока не пройден manual browser test.

## Simple LLM release

Версия `20261017-simple-llm-01` использует ту же схему выпуска: backup, `git archive` pushed commit, отдельный release, build и обновление только app. Production override передаёт `GENA_NLU_MODE=${GENA_NLU_MODE:-simple_llm}`; legacy выбирается явно через `semantic_plan_legacy`. Модель Ultra и concurrency 1 сохранены.

Для повторного выпуска с действующими production credentials передайте `preserve_credentials: true` в prepare и rollback у `gigachat_ops.py`; ключ и CA остаются в существующем secret storage. При activate передайте `nlu_mode: simple_llm`; check проверяет режим контейнера. Backup включает проверенный PostgreSQL dump, предыдущую revision, env, systemd unit, compose snapshot и схему env без значений. Compose snapshot содержит секреты и хранится только в root-only backup каталоге.

Rollback восстанавливает сохранённые config, release symlink и предыдущий image, обновляет только app и сохраняет PostgreSQL volume. При использовании preserve_credentials возвращается исходная рабочая конфигурация; режим для ротации скомпрометированного ключа сохраняет прежнее поведение. SQL migrations этого релиза не меняются. После activate обязательны HTTPS/auth/API и небольшой smoke на отдельном тестовом аккаунте; при критической ошибке выполните rollback. Физический микрофон проверяется вручную на устройстве.

## Deadline / Groq voice release

`20261017-deadlines-groq-01` добавляет `day_of_month` в simple_llm и MediaRecorder →
authenticated `/api/voice/transcribe` → Groq `whisper-large-v3-turbo`. Автоотправки
и автоматического SpeechRecognition fallback нет. SQL migrations не меняются.

После локальных tests/build, live GigaChat и пяти реальных голосовых проверок:
commit/push, audit, backup, archive pushed revision, prepare с
`preserve_credentials:true`, build. Перед activate вызовите `configure_stt` с
`backup_dir` и `groq_api_key` только по SSH stdin, без вывода и сохранения payload.
Действие сохраняет ключ в root-owned `/etc/rhythm/production.env` (0600), ставит
`GENA_STT_PROVIDER=groq` и `GROQ_STT_MODEL=whisper-large-v3-turbo`.

Activate выполняется с `nlu_mode:simple_llm`, обновляет только app и устанавливает
Nginx config с отдельным upload limit 16 МиБ и `proxy_request_buffering off`.
Это необходимо, чтобы Nginx не сохранял audio body во временные файлы. Backup и
rollback включают прежний Nginx config. Проверяйте public HTTPS/auth, task due
date, сохранность прежних записей, runtime STT config и реальную запись через
браузер; HTTP health сам по себе не подтверждает качество STT. На критической
ошибке rollback с `preserve_credentials:true`, без восстановления БД.
