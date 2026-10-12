# API Go: P0 contract и границы последующих этапов

P0 сохраняет существующие успешные ответы и HTTP-коды общего corpus. Строки kind/action/status/priority — JSON boundaries типизированного Go domain. Новый error.code/message дополняется detail для совместимости существующего frontend.

## Реализованные P0 endpoints

| Метод | Путь | Назначение |
| --- | --- | --- |
| POST | `/api/chat` | Выполнение/уточнение команды |
| GET | `/api/history/{session_id}` | Последние 50 обменов и состояние |
| GET | `/api/events` | Период, kind/status/q, offset/limit |
| GET | `/api/events/{id}` | Одна запись |
| POST | `/api/events` | Создание через JSON |
| PATCH | `/api/events/{id}` | Изменение, перенос, завершение |
| DELETE | `/api/events/{id}` | Удаление через API; UI должен запросить подтверждение |
| GET | `/api/tasks` | Список task |
| GET | `/api/meetings` | Список meeting |
| GET | `/api/reminders` | Список reminder |
| GET | `/api/reminders/due` | Open reminders, наступившие не более 5 минут назад |
| POST | `/api/import-demo` | Atomic/idempotent CSV import |
| GET | `/health` | Go/SQLite status, count, timezone, now, schema_version |
| GET | `/openapi.json`, `/docs` | OpenAPI и Swagger |

GET `/`, `/architecture`, `/static/*`, `/diagrams/*` сохраняет существующий интерфейс/схемы. Новые React-страницы в P0 не добавлены.

## DTO и совместимость

Chat input: message (1–2000 Unicode characters), session_id/request_id UUID; при отсутствии UUID генерируется сервером. Output сохраняет reply/action/events/total/candidates/conflicts/provider/pending/timezone/session_id/request_id. `clarify` — успешный ответ 200, не создание события.

Event output сохраняет id/kind/title/date/time/duration_minutes/status/created_at/updated_at/source_key; добавляет description, priority, external_calendar_id. Дополнительные поля обратно совместимы. Date — ISO civil date или null; time — HH:MM или null; timestamps — RFC3339; duration_minutes — integer 5–1440. Meeting/reminder требуют дату и время. Task может быть без срока.

POST input: kind, title, необязательные description/date/time/duration_minutes/status/priority/force. Kind: task/meeting/reminder. Status: open/done. Priority: normal/low/high. Создание возвращает 201.

PATCH input: title/description/date/time/duration_minutes/status/priority/force. Отсутствующее поле не меняется; null очищает nullable date/time, если domain validation это допускает. Kind и source_key не изменяются через PATCH. Пустой PATCH возвращает 422. DELETE возвращает 204.

Список сохраняет start/end/kind/status/q/offset/limit. Интервал `[start,end)`, end не включается. limit 1–200, default 100; offset >= 0. Поиск q работает по title/description, с нормализацией и ранжированием.

## Ошибки

```json
{
  "error": {"code": "validation_error", "message": "Не указано время встречи"},
  "detail": "Не указано время встречи",
  "request_id": "HTTP request id"
}
```

- 422: JSON/DTO/date/time/UUID validation; сохранённый legacy-код при другом тексте под тем же session_id/request_id.
- 404: запись/маршрут не найдены.
- 405: метод не поддерживается.
- 409: meeting_conflict в прямом REST CRUD; conflicts содержит подходящие встречи.
- 413: превышен body limit 64 KiB.
- 504: request deadline; 408: canceled request.
- 500: внутренняя ошибка без SQL/секретов в ответе.

Пересечение проверяется повторно в том же коротком write scope, где сохраняется запись. force=true — явное подтверждение пересечения в REST. В чате сохраняется pending/conflict и ответ «да/нет». Удаление через чат всегда запрашивает отдельное подтверждение.

## Отложенные API

Calendar views/new UI — P1, live Yandex CalDAV и provider API — P2, preferences/widgets layout — P3, расширенные возможности — P4. API этих этапов будет добавляться вместе с работающим сервисом и тестами. Workspace/team/auth/permissions и assignee endpoints не планируются. Telegram/Zoom/Notion/Slack backend не реализуется; карточки — «Скоро».
