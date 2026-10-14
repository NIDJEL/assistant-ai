# P1: React UI и ручная отправка диктовки

## Baseline

13 октября 2026, main `7046eb0` (merged P0). До изменений прошли `go test ./...`, `go vet ./...`, `go test -race -count=1 ./...` в Linux Docker, 73 Python-теста и 6 legacy voice-тестов. Python reference остаётся на месте.

Изучены все шесть макетов. Основа: голубовато-белый фон, отдельные белые панели, синий accent, sidebar, wave mascot, лёгкие границы и отступы. Данные и даты поступают из Go. Assignee/team/auth и fake integration state отсутствуют.

## Implementation plan

1. React/TypeScript/Vite в web/, design tokens, common layout и sidebar.
2. Typed API client и небольшой React context для общего набора событий, clocks/config и обновления после mutations. Без Redux и отдельного cache framework.
3. Today, история assistant, editable draft, independent voice controller и TTS. Только Send/Enter вызывает chat API.
4. Calendar month/week/day на /api/events (в P0 нет /api/calendar), Tasks, Meetings, Reminders, Archive, Integrations, Settings; общие create/edit/delete dialogs.
5. Минимальное расширение metadata встречи: location/meeting_url, schema v3 и contract/migration tests. Существующая conflict logic остаётся на Go.
6. Widget registry и show/hide panel; tablet/mobile drawers, accessibility и reduced motion. Drag & drop — P3.
7. Frontend unit/integration tests и browser E2E с реальным Go backend. Проверка 1920×1080, 1440×900, 1280×800, tablet/mobile; desktop screenshots всех страниц и trace ручной отправки диктовки.
8. Node build → Go build → runtime без Node/Python, SPA routes при refresh, CI, README и схемы. PR: P1: React UI and manual-send voice UX.

## Границы

P2 live CalDAV/OAuth и P3 widget persistence не начинаются. Яндекс — подготовленный UI с объяснением этапа P2; остальные интеграции — «Скоро». Повторение/звук напоминаний и subtasks/tags отсутствуют до backend semantics.
