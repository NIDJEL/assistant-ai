# Проверка актуальной версии

15 октября 2026. Основа: фактически запущенный VPS-релиз `20261014-reminder-fix-03`.

Проверки очищенной версии:

- `go test -count=1 ./...` с отдельной PostgreSQL: PASS. Покрыты аккаунты, изоляция, OAuth, миграции, команды, повторения, worker и Web Push.
- `go vet ./...`: PASS.
- `pnpm --dir web test`: PASS, 86 тестов.
- `pnpm --dir web build`: PASS. Получены те же frontend assets, что в рабочем релизе: `index-Di994K9O.js`, `index-Cyjf569C.css`.
- `docker compose config --quiet`: PASS.
- `pnpm --dir web e2e`: PASS, все 76 браузерных сценариев (5,3 минуты).
- Локальная Docker-сборка и запуск на `http://localhost:8081`: PASS, контейнер healthy, PostgreSQL сохранена.

Полные браузерные сценарии запускаются через `pnpm --dir web e2e` с `TEST_DATABASE_URL` на тестовой PostgreSQL; отчёт сохраняется в `web/playwright-report/`. Race detector проверяется в Linux CI.

Старый Python runtime и статический frontend удалены. Если React build отсутствует, сервер возвращает `frontend_unavailable` с командой сборки. Проверка этого сценария добавлена в Go suite.

Локальные browser-тесты используют тестовые аккаунты и управляемый SpeechRecognition. Они не подтверждают распознавание живого микрофона, consent внешнего Яндекс OAuth или доставку Web Push на физическое устройство. Эти функции сохранены из рабочего VPS-релиза.
