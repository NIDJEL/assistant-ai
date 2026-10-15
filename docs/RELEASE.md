# Актуальная версия и синхронизация

Версия: `20261015-gigachat-ultra-01`. Добавлены GigaChat-3-Ultra, strict SemanticCommand и production override. Сохраняются Web Push/PWA, Яндекс OAuth и календарь, onboarding, повторения и исправления жизненного цикла напоминаний.

В `main` поддерживается одно приложение: Go API + React + PostgreSQL. Семантический provider включается явно через GENA_SEMANTIC_PROVIDER; production service использует подготовленный GigaChat override. Frozen evaluation сохранён без повторного held-out inference.

Go-код импорта SQLite, версии SQL-миграций, корпус совместимости и тесты сохранены: они проверяют поведение и позволяют переносить данные. История Git не переписывается.

Перед очисткой сохранены исходники локального рабочего дерева, Git bundle и исходники VPS в `D:\assistantai-backups\20261015-sync`. `.env` хранится отдельно от Git. Архивы могут содержать непубличные данные и предназначены для владельца проекта.

Новые релизы собираются из зафиксированного Git-коммита. На VPS файл `REVISION` в каталоге релиза содержит его SHA; `VERSION` определяет название версии. Активный каталог — `/opt/rhythm/current`. Старые каталоги релизов архивируются вне `/opt/rhythm/releases`; рабочие данные PostgreSQL находятся в отдельном постоянном томе.

Порядок обновления и отката: [deployment](../deploy/README.md). Проверки: [VALIDATION.md](VALIDATION.md).
