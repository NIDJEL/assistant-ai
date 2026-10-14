# Ритм P1 — desktop 1440×900

Снимки сделаны browser E2E с production React build и реальным Go API. Данные — импортированный CSV, текущая дата — Go health. Они не являются статическим frontend или mocked event fixtures.

## Сегодня

![Сегодня](screenshots/p1/today.png)

## Календарь

![Календарь](screenshots/p1/calendar.png)

## Задачи

![Задачи](screenshots/p1/tasks.png)

## Встречи

![Встречи](screenshots/p1/meetings.png)

## Напоминания

![Напоминания](screenshots/p1/reminders.png)

## Архив

![Архив](screenshots/p1/archive.png)

## Интеграции

![Интеграции](screenshots/p1/integrations.png)

## Настройки

![Настройки](screenshots/p1/settings.png)

## Mobile 390×844

![Mobile](screenshots/p1/mobile.png)

## Конфликт встречи

![Конфликт](screenshots/p1/meeting-conflict.png)

## Voice proof

Контролируемый Web Speech mock, реальный React UI и Go HTTP. Проверено: после паузы 700 мс — 0 POST; после продолжения — 0 POST; после Send и late onend — ровно 1 POST. Recognition остановлен один раз. [Машинный результат](screenshots/p1/voice-proof.json).

![Пауза: draft, 0 POST](screenshots/p1/voice-paused.png)

![Продолжение: дополненный draft, 0 POST](screenshots/p1/voice-continued.png)

![Send: реальный ответ Go, 1 POST](screenshots/p1/voice-sent.png)
