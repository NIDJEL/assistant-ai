# P1.1 — Visual alignment / design parity

Approved user references are saved unchanged in [screenshots/p1-1/reference](screenshots/p1-1/reference). The [comparison gallery](screenshots/p1-1/index.html) and JPEG boards place REFERENCE and ACTUAL side by side. Actual screenshots use 1440 × 900; original reference proportions are preserved. Archive and Settings have no dedicated supplied reference and use the Tasks / Reminders design system for comparison.

## Preserved scale

The global type scale, main padding and existing desktop/tablet/mobile breakpoints remain. No zoom, transform scale or global font-size adjustment is applied. The latest width correction keeps the shell across the available screen, with navigation on the left and widgets on the right. At 1440 the side panels retain their 238 px / 330 px widths and 16 px gaps. When widgets are hidden, only the central page content is limited to 1280 px, aligned beside navigation; the shell itself is not compressed or centered. Desktop navigation can collapse to 76 px with icons and tooltips, and this preference survives refresh. Mobile navigation slides in from the left; widgets still open from the right. A stable scrollbar gutter prevents horizontal shifts as content changes.

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

Schema v5 adds `deleted_at` for recoverable deletion. Normal lists, counts, lookups, conflicts and due reminders exclude deleted records. Archive → Deleted reads `GET /api/events?deleted=true`; its Restore button calls `POST /api/events/{id}/restore`. Recovery preserves the same ID, original date, status and metadata, including after server restart. An open meeting must pass conflict checks before recovery; overriding a conflict requires a separate explicit action. Completed records remain in Archive → History. Deletion and completion show feedback with Undo. Records physically deleted before this migration cannot be recovered by this feature.

`GET/PUT /api/preferences/{profile}` persists a browser UUID profile in SQLite. `panel_visible` (plus the compatible `widgets` field), name/TTS, per-page widget order, hidden widget types and record limits survive refresh and server restart. Old browser preferences are imported once. A monotonic revision prevents stale writes from replacing newer settings; a newer cached edit is replayed after an immediate refresh. Writes are serialized, failures are visible with Retry, and a widget type is never duplicated.

Widget sorting uses dnd-kit PointerSensor and KeyboardSensor. Only the handle activates dragging. An overlay lifts the card while the original keeps its height; adjacent cards animate. Space / Up / Down / Space reorder by one logical card even when heights differ. Reduced-motion preference removes CSS transitions and drop animation.

## Motion

Navigation uses browser View Transitions with 440 ms easing and a gentle slide. Individual widget snapshots animate between their positions across pages. The router commits synchronously inside the snapshot callback; it never waits for an animation frame while rendering is paused. Rapid navigation skips the previous visual transition while applying each state update once. Browsers without this API and navigation beneath a closing mobile drawer use a live page fade. Snapshot images retain their natural size when the layout width changes, avoiding text scaling.

Showing or hiding widgets animates the live grid for 440 ms. The panel keeps its full width inside a clipping region and moves as a whole toward or from the right screen edge; cards and text retain their dimensions and opacity. Closing content remains mounted until the slide finishes and becomes inert immediately. A quick reversal resumes from the visible position. This works with or without native snapshots and honors System / Smooth / No animations. The hide button occupies its own grid column beside the title, and the subtitle uses a separate full-width row, preventing overlap at every desktop breakpoint.

Calendar day/view selection and task, meeting and integration filters update only the relevant content. They do not capture or animate the shell, heading or toolbar. Measured containers interpolate changed heights; lists retain departing rows in position until their collapse finishes. Completing a reminder checks and strikes its title, collapses the row, and explains that it moved to the archive with an Undo action. Opening its inline editor leaves the title visible. Calendar date markers have equal fixed dimensions so they cannot shrink into ovals.

The widget panel stays mounted across routes. A measured layout animation moves retained cards and introduces added cards when native snapshots are unavailable or a picker is open. The inner card animates independently of the sortable wrapper, avoiding interference with dragging. Adjacent drag cards and the drop overlay use 440 ms easing.

Task details, priority filters and inline reminders animate their measured height. Closing content becomes inert, and reopening during the 220 ms exit preserves the reminder draft. Dialogs, drawers and menus have entry and exit animations; closing dialogs reject clicks and submission until native focus is restored on unmount. Buttons, checks, switches and new chat bubbles have gentle feedback. Type sizes are unchanged.

Settings → Interface → Animations offers System (default), Smooth and No animations. The selected mode persists in the existing SQLite profile and browser cache. System follows `prefers-reduced-motion`; Smooth explicitly enables motion within Rhythm even when the browser reports reduced motion. No animations disables snapshot, layout, backdrop and CSS animations, with immediate closing. No OS settings are changed.

## Verification

- `go test ./...` and `go vet ./...`: passed; existing shared parity/contract corpus retained.
- Linux Docker `go test -race -count=1 ./...`: passed.
- Frontend: 26 tests passed; production build and frozen lockfile installation passed. Delayed search keeps menus/editors mounted; immediate refresh replays newer preferences.
- Browser: 26 E2E passed, including pointer and keyboard sorting, handle-only activation, add/remove/duplicate prevention, widget settings and persistence, task subtasks, meeting selection, selected day, inline reminder and integration detail. Motion checks verify a successfully started destination snapshot, rapid navigation, reversible reminder closing with draft preservation, blocked submission during modal exit, focus restoration, reduced motion, explicit Smooth mode despite system reduced motion, persistence, widget movement without native snapshots and navigation beneath closing drawers at 593 px. Additional regressions verify side panels at screen edges, bounded content with widgets hidden, persistent navigation collapse, left mobile navigation, round date markers, stable headers/toolbars during filters, animated reminder completion with Undo, and same-ID recovery through the real Go API. Panel checks measure both opening and closing at an intermediate animation frame, verify fixed card dimensions and opacity, test rapid reversal and reduced motion, and check title/button overlap at 1180–2560 px.
- Responsive checks: 2560, 1920, 1600, 1440, 1280, 834, 593 and 390 px, with overflow assertions and tablet/mobile drawers.
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

## Latest layout and recovery screenshots

The original comparison boards document the initial P1.1 alignment. These later browser captures show the user-requested corrections with records served by the real Go API in the separate E2E database:

- [Full-width shell with side panels at screen edges, 2560 px](screenshots/p1-1/fixes/wide-calendar.png)
- [Navigation collapsed to icons, 2560 px](screenshots/p1-1/fixes/collapsed-navigation.png)
- [Widgets hidden: bounded central content, full-width shell](screenshots/p1-1/fixes/hidden-widgets.png)
- [Mobile navigation opening from the left, 593 px](screenshots/p1-1/fixes/left-navigation.png)
- [Reminder completion feedback and Undo](screenshots/p1-1/fixes/reminder-completed.png)
- [Deleted records with Restore buttons](screenshots/p1-1/fixes/archive-restore.png)
- [Widget header with space for the hide button](screenshots/p1-1/fixes/widget-heading.png)
- [Whole panel sliding out, captured during the animation](screenshots/p1-1/fixes/widgets-closing.png)
