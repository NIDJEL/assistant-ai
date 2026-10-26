# CI/CD для Rhythm

Один workflow [CI/CD](../.github/workflows/ci.yml) проверяет PR в `main` и изменения приложения в `main`. Сборка использует Go toolchain из `go.mod`, Node 24, pnpm из `web/package.json` и PostgreSQL 17. Push в другие ветки, расписание и изменения только документации не расходуют runner minutes. Полный Chromium-набор доступен вручную с `full_browser=true`.

## Выпуск

1. Проверки: `go mod verify`, `go vet`, Go race с отдельной PostgreSQL, тесты release receiver, frontend unit-тесты, TypeScript, Vite и короткий Chromium smoke.
2. Успешный `main` упаковывается в `production-release`: статический Linux/amd64 Go binary, `web/dist`, runtime-данные, диаграммы и `REVISION`. Runtime-образ проверяется с read-only filesystem, auth/CRUD, сохранением данных после restart и штатной остановкой. Приложение не пересобирается на VPS.
3. Job `Production` использует environment `production`, разрешённый только для ветки `main`, и проверяет, что SHA всё ещё является текущим `main`.
4. Архив с SHA256 передаётся по SSH с закреплённым host key. Ключ отдельного пользователя `rhythm-deploy` разрешает только установленный receiver; shell, forwarding и произвольные sudo-команды недоступны.
5. Receiver собирает runtime-образ из готовых файлов, сохраняет PostgreSQL dump и проверяет его через `pg_restore --list`, сохраняет production.env и предыдущий release.
6. Меняется только `RELEASE_TAG`. Compose обновляет только `app` с `--no-deps --no-build --wait`; контейнер PostgreSQL сохраняется. Проверяются health контейнера, публичный HTTPS `/health` и точное содержимое нового `/auth`. После успеха переключается `/opt/rhythm/current`.
7. При ошибке активации receiver возвращает прежний env, образ и symlink и проверяет их health. База автоматически не восстанавливается: миграции должны быть совместимы с предыдущим приложением.

Параллельные выпуски блокируются в GitHub и через `flock` на VPS. Обрыв SSH после загрузки не прерывает активацию. Повторная передача того же архива разрешает повторить неудачный выпуск; другой архив с тем же SHA отвергается.

## Однократная настройка сервера

Receiver [release.py](release.py) устанавливается администратором как root-owned `/usr/local/sbin/rhythm-deploy` с режимом 0755. Python 3.10+, Docker Compose v2, рабочий `rhythm` и HTTPS уже должны быть настроены по [production guide](README.md).

Каталог `/etc/rhythm/deploy` принадлежит root, режим 0700. В нём находятся проверенные `Dockerfile.release`, `compose.production.yaml`, `compose.gigachat.example.yaml` и `postgres-init.sh`. В production.env используется существующая конфигурация, включая GigaChat и OAuth. Receiver не принимает Compose/Dockerfile, скрипты, ссылки или пути вне runtime-файлов из загружаемого архива.

У пользователя `rhythm-deploy` свой SSH-ключ. Строка `authorized_keys`:

```text
restrict,command="/usr/bin/sudo -n /usr/local/sbin/rhythm-deploy" ssh-ed25519 PUBLIC_KEY rhythm-github-actions
```

В `/etc/sudoers.d/rhythm-deploy` с режимом 0440:

```sudoers
rhythm-deploy ALL=(root) NOPASSWD: /usr/local/sbin/rhythm-deploy ""
```

Добавьте пользователя в существующий `AllowUsers`, выполните `sshd -t` и reload SSH. Проверьте, что даже запрос команды `id` по этому ключу попадает в receiver и отвергается без корректного архива.

В GitHub создайте environment `production` с deployment branch policy `main` и четырьмя environment secrets:

| Секрет | Значение |
| --- | --- |
| `DEPLOY_HOST` | Адрес VPS |
| `DEPLOY_USER` | `rhythm-deploy` |
| `DEPLOY_SSH_KEY` | Отдельный private key OpenSSH |
| `DEPLOY_KNOWN_HOSTS` | Заранее проверенная запись host key для этого адреса |

Production.env и ключи AI/OAuth в GitHub не передаются. Новые серверные шаблоны и изменения receiver устанавливаются администратором отдельно после проверки; автоматический архив их не меняет. Не заменяйте рабочие шаблоны без сравнения текущих настроек.

Receiver также публикует standalone-страницы ошибок из `web/dist/errors` в `/var/www/rhythm-error-releases/<SHA>` и атомарно переключает `/var/www/rhythm-errors`. Эти HTML-файлы доступны Nginx без backend и без доступа к приватному каталогу релиза. После первоначальной установки страниц проверьте `deploy/nginx.conf` через `nginx -t` и примените reload. API-ответы приложения остаются JSON; перехват upstream-ошибок включён только для страниц.

## Диагностика и восстановление

Если GitHub пишет `The job was not started because recent account payments have failed or your spending limit needs to be increased`, задания не стартовали: исправьте **Settings → Billing & licensing** аккаунта. Изменение workflow не устраняет платёжную блокировку. После исправления выполните **Actions → CI/CD → Run workflow**, выберите `main`, включите `deploy`.

Предыдущая установка остаётся в `/opt/rhythm/releases`, backup каждого выпуска — в `/var/backups/rhythm/cicd-<UTC timestamp>-<SHA>/`. Каталог содержит `postgres.dump`, `production.env` и `previous-release`; доступен только root. При ручном откате верните env из backup и symlink, затем поднимите только `app` предыдущим Compose с `--no-deps --no-build --wait`. Не восстанавливайте dump поверх рабочей базы без отдельной процедуры восстановления.

Старые каталоги `sha-*`, соответствующие образы `rhythm-app:sha-*` и каталоги `cicd-*` можно удалять после проверки внешней резервной копии, сохраняя текущий и предыдущий успешный выпуск. Автодеплой их не удаляет.
