# P1: результаты проверки

Baseline P0: `7046eb0`. Ветка: `p1-react-ui-voice`.

| Проверка | Результат |
| --- | --- |
| Go regression / shared parity corpus / contract tests | `go test ./...` — passed |
| Go static checks | `go vet ./...` — passed |
| Race detector | `go test -race -count=1 ./...` — passed, Go 1.27.1 / Linux / CGO |
| Python reference | 73 pytest tests passed; ruff passed |
| Legacy voice reference | 6 Node tests passed |
| React / voice / dates | 24 Vitest tests passed |
| Browser E2E с реальным Go HTTP | 9 Playwright tests passed, без retries |
| TypeScript + Vite production build | passed |
| Production Docker | healthy; создание, idempotency, restart persistence, SPA refresh, graceful stop — passed |
| Runtime filesystem | Go binary + React static build; без Node/Python runtime |

Проверены все восемь страниц на 1920×1080, 1440×900, 1280×800, 834×1112 и 390×844. Desktop screenshots, конфликт встречи и voice sequence находятся в [галерее](P1_SCREENSHOTS.md).

Voice proof использует контролируемый Web Speech API mock и реальные React + Go HTTP: пауза → 0 POST; продолжение диктовки → дополненный draft, 0 POST; Send и запоздалый onend → ровно 1 POST. [Машиночитаемый результат](screenshots/p1/voice-proof.json). Качество живого микрофона этим тестом не измеряется.

CI повторяет regression, race, frontend, browser и Docker проверки на GitHub Actions; актуальный статус доступен в PR. Python `app/`, reference tests, P0 NLP/search и corpus не изменены. P2 live CalDAV и P3 drag & drop не входят в этот PR.
