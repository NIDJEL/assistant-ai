# Проверки развертывания — 14 октября 2026

Приложение: https://meinangelplatz.top, сервер 31.77.232.46.
Активный релиз: `/opt/rhythm/releases/20261014-02`.

## Подтверждено

- Сертификат Let’s Encrypt ECDSA: проверена доверенная цепочка и имя домена через внешнее соединение TLS 1.3. Срок текущего сертификата: до 12 января 2027, 17:18 по Москве.
- `certbot renew --dry-run --run-deploy-hooks`: успешное пробное продление и проверка/reload Nginx. `certbot.timer` включен и пережил перезагрузку.
- HTTP перенаправляет на HTTPS. Технические страницы `/docs`, `/architecture`, `/diagrams`, `/openapi.json` и dotfiles закрыты на публичном proxy.
- Регистрация и вход по email, флаги cookies Secure/HttpOnly/SameSite, CSRF, недоступность чужой записи, создание/изменение/удаление/восстановление записи и logout проверены на работающем HTTPS-приложении.
- Ограничение входа срабатывает при подмене `X-Real-IP` и `X-Forwarded-For`. Доверенный proxy и раздельный учет клиентов покрыты Go-тестами.
- Chromium: страницы входа 1440×900 и 390×844 без ошибок JavaScript и переполнения экрана.
- Backend доступен только через loopback; PostgreSQL не публикует порт. Пользователь БД `rhythm`: SUPERUSER/CREATEDB/CREATEROLE/REPLICATION выключены.
- Контейнер приложения: uid 65532, readonly rootfs, все capabilities удалены, no-new-privileges включен.
- SSH: `PermitRootLogin no`, `PasswordAuthentication no`, `AuthenticationMethods publickey`, `AllowUsers rhythm-admin`; вход по новому ключу проверен после перезагрузки.
- Firewall, Fail2ban, таймеры backup/ACME/обновлений и systemd-сервис приложения включены. После финального запуска нет failed units и ожидающего reboot.
- Ядро Ubuntu обновлено до `6.8.0-147-generic`; запуск на нем проверен. Данные PostgreSQL, сессии и изоляция аккаунтов пережили перезагрузку.
- Backup восстановлен через `pg_restore --exit-on-error` в отдельную базу: 13 таблиц, число пользователей совпало. Временная база удалена.

## Проверки исходного кода

- `go test ./...` и `go vet ./...`: passed, Go 1.27.2.
- На Linux в отдельной PostgreSQL: `go test -race ./... && go vet ./...`: passed. Рабочая база в этих тестах не использовалась.
- `pnpm --dir web build`: passed.
- `pnpm --dir web test`: 6 файлов, 46 тестов passed.
- `pnpm --dir web audit --prod`: известных уязвимостей не найдено.
- `govulncheck ./cmd/server`: 0 уязвимостей в импортируемых пакетах и вызываемых функциях после обновления Go до 1.27.2, pgx до 5.11.0, x/crypto до 0.58.0, x/text до 0.43.0.

Осталась запись на уровне всего модуля x/crypto: [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) относится к устаревшему OpenPGP. Эти пакеты не импортируются сервером и не входят в используемый код. Проверка не является гарантией отсутствия неизвестных уязвимостей.

## Что требует отдельной настройки

Для Яндекс OAuth необходимо зарегистрировать callback `https://meinangelplatz.top/api/auth/yandex/callback` в кабинете разработчика Яндекса; вход через Яндекс не подтвержден. Вход по email работает.
Ежедневные backups пока хранятся на сервере. Первый комплект дополнительно сохранен на компьютере администратора, но постоянное внешнее хранилище не настроено.

Справочная документация: [Certbot renewal](https://eff-certbot.readthedocs.io/en/stable/using.html#renewing-certificates), [Ubuntu linux-virtual](https://packages.ubuntu.com/noble/linux-virtual), [Go vulnerability management](https://go.dev/doc/security/vuln/).
