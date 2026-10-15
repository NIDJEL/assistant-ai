# Проверки совместимости и импорт SQLite

Рабочий сервер использует PostgreSQL. `cmd/import-sqlite`, `internal/storage/sqlite` и `migrations/` нужны для переноса прежних данных и Go-тестов; они не запускают второе веб-приложение.

`testdata/parity/corpus.json`, `testdata/parity/legacy_v1.sqlite3` и `internal/search/data/russian.json` — фиксированные регрессионные данные. Их использование не требует Python. Источник словаря описан в `internal/search/data/NOTICE.md`.

```powershell
$env:TEST_DATABASE_URL='postgres://rhythm@127.0.0.1:5433/rhythm?sslmode=disable'
go test -count=1 ./...
go vet ./...
```

Используйте отдельную тестовую PostgreSQL. Интеграционные тесты создают и удаляют свои схемы. Без `TEST_DATABASE_URL` они пропускаются. `go test -race ./...` выполняется в Linux CI с C-компилятором; обычные тесты доступны на Windows.

Импорт требует пустого аккаунта назначения и резервной копии SQLite. Настройка и команда: [аккаунты и миграции](ACCOUNTS.md).
