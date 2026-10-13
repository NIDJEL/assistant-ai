# P1.1 — Visual alignment / design parity

Approved user references are saved unchanged in [screenshots/p1-1/reference](screenshots/p1-1/reference). The [comparison gallery](screenshots/p1-1/index.html) and JPEG boards place REFERENCE and ACTUAL side by side. Actual screenshots use 1440 × 900; original reference proportions are preserved. Archive and Settings have no dedicated supplied reference and use the Tasks / Reminders design system for comparison.

## Preserved scale

The existing `app.css` shell widths, main padding and global type scale are retained: at 1440, sidebar 238 px, main 808 px and widgets 330 px, with 16 px gaps. Existing desktop/tablet/mobile breakpoints remain. No zoom, transform scale or global font-size adjustment is applied to the application. The calendar grid height is adjusted to keep the selected-day card visible; Today uses a bounded conversation scroll area to keep its composer at the bottom.

## Pages

| Page | Structure implemented | Comparison |
| --- | --- | --- |
| Today | Decorative wave, quick actions, conversation cards, bottom input → microphone → Send | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/today.jpg) |
| Calendar | Month/week/day, selected circle, cell dots/overflow, selected-day card with time ranges | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/calendar.jpg) |
| Tasks | Search + filter icon, pills, Today / Week / Later groups, priority dots, tags, expanded description and subtasks | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/tasks.jpg) |
| Meetings | Search, period, online/offline, timeline, selected detail, duration, description, participant list and actions | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/meetings.jpg) |
| Reminders | Morning / Day / Evening, colored icons, enabled switch, inline title/date/time/note editor | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/reminders.jpg) |
| Archive | Search/date/type/status filters, soft card rows using the common surfaces | [Design system / ACTUAL](screenshots/p1-1/comparison/archive.jpg) |
| Integrations | Six service cards, search/status filters, service selection replaces widgets with detail | [REFERENCE / ACTUAL](screenshots/p1-1/comparison/integrations.jpg) |
| Settings | General, Voice, Interface, Notifications, Integrations; language, TTS and real microphone permission state | [Design system / ACTUAL](screenshots/p1-1/comparison/settings.jpg) |
| Widget panel / drag | Compact calendar, Today, Tasks, Progress, upcoming meetings, tags and quick scheduling | [Drag state](screenshots/p1-1/comparison/widgets-drag.jpg) |

Unsupported recurrence, reminder sound, system notifications and live integration actions are disabled or explicitly marked Soon. Yandex is **not connected**; no fabricated synchronization time is shown. No assignee/team/workspace system is added.

## Minimal persistence additions

Schema v4 adds an optional `events.details` JSON column: tags, subtasks with stable IDs, meeting participants, and reminder enabled state. Existing event fields and the assistant/NLP contracts are retained. Empty optional fields are omitted from JSON. Disabling a reminder excludes it from the due-reminders endpoint. Search also includes real task tags.

`GET/PUT /api/preferences/{profile}` persists a browser UUID profile in SQLite. `panel_visible` (plus the compatible `widgets` field), name/TTS, per-page widget order, hidden widget types and record limits survive refresh and server restart. Old browser preferences are imported once. Writes are serialized, failures are visible with Retry, and a widget type is never duplicated.

Widget sorting uses dnd-kit PointerSensor and KeyboardSensor. Only the handle activates dragging. An overlay lifts the card while the original keeps its height; adjacent cards animate. Space / Up / Down / Space reorder by one logical card even when heights differ. Reduced-motion preference removes CSS transitions and drop animation.

## Verification

- `go test ./...` and `go vet ./...`: passed; existing shared parity/contract corpus retained.
- Linux Docker `go test -race -count=1 ./...`: passed.
- Frontend: 24 tests passed; production build and frozen lockfile installation passed.
- Browser: 11 E2E passed, including pointer and keyboard sorting, handle-only activation, add/remove/duplicate prevention, widget settings and persistence, task subtasks, meeting selection, selected day, inline reminder and integration detail.
- Responsive checks: 1920, 1600, 1440, 1280, 834 and 390 px, with overflow assertions and tablet/mobile drawers.
- Voice: pause → zero POST; continued dictation → appended draft; Send → exactly one POST. Voice controller and manual-send implementation unchanged.
- Python reference: 73 tests passed; legacy voice: 6 tests passed.
- Docker image build and dedicated container healthcheck passed; SQLite metadata/preferences are also tested after closing and reopening the database.

## Reproduce screenshots

Start an isolated QA database; do not use the user's production database:

```powershell
$env:APP_ADDR='127.0.0.1:8084'
$env:DATABASE_PATH='var/p1-1-visual.sqlite3'
$env:APP_TIMEZONE='Europe/Moscow'
$env:NLP_PROVIDER='rules'
$env:AUTO_IMPORT_DEMO='true'
pnpm --dir web build
go run ./cmd/server
```

In another terminal:

```powershell
$env:P1_VISUAL_FIXTURES='true'
node web/scripts/capture-parity.mjs
```

The optional fixture flag adds QA metadata/reminders through the actual Go API to that isolated database, exercising populated expanded/selected states. Original demo records supply the underlying titles and dates; screenshot records are not shipped as application data. Counts, progress and dates are calculated from API records. No screenshot April 2025 data is copied into the frontend.

The P1 baseline branch is `p1-react-ui-voice` (P1 has not yet been merged into `main`), so the P1.1 PR targets that branch to keep the diff limited to visual alignment.
