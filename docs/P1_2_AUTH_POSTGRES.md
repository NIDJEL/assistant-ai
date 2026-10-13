# P1.2 — authentication, multi-user PostgreSQL and onboarding

Go runtime now requires PostgreSQL and server-side sessions. SQLite and the Python reference remain available for parity tests and explicit import. New accounts start empty; `/api/import-demo` imports sample events only into the authenticated account.

## Run and environment

Copy `.env.example` to `.env`. Set `POSTGRES_PASSWORD` and `SESSION_SECRET` to independently generated random values. Hexadecimal passwords need no escaping in the Compose connection URL. Never commit `.env`.

```powershell
Copy-Item .env.example .env
# Generate two independent values and put them in .env:
[Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
docker compose up --build -d --wait
```

Default URL: `http://localhost:8081`. Use the same host in `PUBLIC_URL`, the browser and provider redirect URIs: localhost and 127.0.0.1 are different origins. PostgreSQL stays on the Compose network and uses a persistent volume. Startup waits for PostgreSQL health, retries readiness for 30 seconds, then applies migrations.

Native development requires a running PostgreSQL instance, `DATABASE_URL` and `SESSION_SECRET`. Build with `pnpm --dir web build`, then `go run ./cmd/server`. Compose supplies its own internal database URL; the localhost URL in `.env.example` is for native Go. Percent-encode non-hex passwords in native connection URLs.

Production requires `APP_ENV=production`, HTTPS `PUBLIC_URL` and a TLS reverse proxy. Use `sslmode=verify-full` for remote databases. Keep `SESSION_SECRET` stable; rotation invalidates sessions and OAuth handshakes. `AUTH_RATE_LIMIT` defaults to 20 expensive auth attempts per IP per minute per server process. Multi-instance deployment needs an additional shared/proxy limit. Arbitrary forwarded IP headers are deliberately not trusted.

Optional Ollama: `docker compose --profile ollama up -d`, pull the configured model in that container and select `NLP_PROVIDER=hybrid`. Auth and rules-based planning work without Ollama.

## Architecture and ownership

HTTP → `internal/auth.Service` → `internal/user.Service` → repository interfaces → `internal/storage/postgres`. Password work and provider HTTP happen outside domain write transactions.

- `users`: UUID, normalized optional unique contact email, name, avatar, durable onboarding completion.
- `auth_identities`: unique `(provider, provider_subject)` and `(user_id, provider)`; only email identities store password hashes.
- `sessions`: HMAC of a random 256-bit cookie token, owner, creation, fixed 30-day expiry and last-seen time. Raw session tokens are never stored in PostgreSQL.
- `oauth_states`: HMAC state and browser binding, server-held PKCE verifier, optional linking owner and 10-minute expiry. Consumption is atomic and single-use.
- `events`, `dialogue_sessions`, `dialogue_exchanges`, `preferences`, `widget_layouts`, `calendar_connections`: ownership and foreign keys. Dialogue and idempotency keys include the user.

Middleware supplies the authenticated user in context. PostgreSQL rejects missing ownership, and every domain SELECT/UPDATE/DELETE includes the owner. Search only receives that user's records. Per-user PostgreSQL advisory transaction locks serialize domain writes across server instances, protecting meeting conflict checks and idempotency. Browser preference caches and dialogue identifiers use per-user keys; anonymous legacy caches are not assigned to new accounts. BroadcastChannel propagates login/logout between tabs and clears protected content before checking the changed session.

## API and security

`POST /api/auth/register`, `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`, `PATCH /api/auth/me`, `POST /api/auth/onboarding`. Register accepts `name`, `email`, `password`, `password_confirmation`; minimum password length is 10 characters. `/me` returns `user.id`, `email`, `name`, `avatar_url`, `providers`, `onboarding_completed`.

Main APIs return 401 without a valid session. Foreign event IDs and preference profiles return 404. Public `/health` exposes readiness and no personal event count. API responses are `Cache-Control: no-store`.

Argon2id: unique 16-byte salt, 64 MiB, three iterations, two lanes, 32-byte hash. Four concurrent password computations maximum. Wrong password and unknown email both verify a hash and return the same generic message. Registration duplicate email returns the requested inline validation message.

Cookies: HttpOnly, SameSite=Lax, Path=/, no Domain, Secure in production or on HTTPS. Login rotates the current session; logout deletes it and clears the cookie. Credentials never enter localStorage. Auth bodies are limited to 8 KiB, other JSON bodies to 64 KiB. CSRF protection checks the exact configured Origin, Fetch Metadata and JSON for body methods; DELETE has no required body and retains origin checks. No CORS permissions are issued. Access logs record paths, not queries or request bodies.

## Yandex ID

Register an authorization application in [Yandex OAuth](https://oauth.yandex.ru/). Set `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET`, `YANDEX_REDIRECT_URI`; grant `login:info login:email login:avatar`.

Routes: `/api/auth/yandex/start`, `/api/auth/yandex/callback`. Authorization code with S256 PKCE, fixed redirect, browser-bound one-use state, server token exchange, Authorization-header profile request. The stable Yandex ID is the identity key. References: [Yandex code/PKCE](https://yandex.ru/dev/id/doc/ru/codes/code-url), [profile API](https://yandex.ru/dev/id/doc/ru/user-information).

## VK ID

Register a web app in the [VK ID console](https://id.vk.ru/). Set `VK_CLIENT_ID`, `VK_REDIRECT_URI`, and `VK_CLIENT_SECRET` if issued. Register the exact callback and site origin.

Routes: `/api/auth/vk/start`, `/api/auth/vk/callback`. Authorization code, PKCE (`s256` as used by VK's SDK), one-use browser-bound state and callback `device_id`. The server validates the token response state and gets a stable user ID through `oauth2/user_info`; a supplied token subject must match. Provider tokens are transient and never reach React. References: [official VK SDK exchange/user-info](https://github.com/VKCOM/vkid-web-sdk/blob/master/src/auth/auth.ts), [current domain constants](https://github.com/VKCOM/vkid-web-sdk/blob/master/src/constants.ts).

These are OAuth profile flows. ID tokens are not accepted as identity evidence, so this flow does not use an OIDC nonce/JWT path. A future OIDC path must implement nonce, issuer, audience and signature verification.

## Linking, onboarding and calendars

Provider contact email never signs into an existing account. A new identity with a conflicting email is rejected, avoiding automatic takeover and duplicates. Sign into the existing account and use **Settings → Account → Connect**. `?link=1` starts explicit linking; callback requires the same current user. Identities cannot transfer between accounts.

Yandex/VK login never creates a calendar connection. Onboarding separately offers Yandex Calendar or allows skipping. Connect opens the existing P2 setup, which remains **not connected**. Live CalDAV is outside P1.2; there is no VK Calendar.

## Migrations and SQLite import

Embedded versioned SQL lives in `internal/storage/postgres/migrations`. Startup takes an advisory migration lock and applies pending files plus `schema_migrations` records in one transaction. Repeated startup is safe. Use `pg_dump` before future deployment migrations and restore a backup for data rollback.

Stop and back up the old SQLite app before import. Import before other PostgreSQL event data, because preserved IDs must not collide. The destination user must have no events, dialogue or preferences. Multiple legacy preference profiles require explicit `-profile`; the command never mixes them silently.

Set `DATABASE_URL`, `SESSION_SECRET`, `IMPORT_USER_EMAIL` (default `dev@rhythm.local`), `IMPORT_USER_NAME`, `IMPORT_USER_PASSWORD` in the shell. Passwords are environment values, never CLI arguments. The command creates the development email account or checks its existing password.

```powershell
go run ./cmd/import-sqlite -sqlite var/assistant-go.sqlite3
# When multiple legacy profiles exist:
go run ./cmd/import-sqlite -sqlite var/assistant-go.sqlite3 -profile LEGACY_PROFILE_UUID
```

The retained SQLite migration system backs up older schemas before upgrade. A consistent read snapshot transfers IDs, details, deletions, dialogue state/history, idempotency keys, selected preferences and widget layouts atomically. The PostgreSQL event sequence advances safely. A digest makes repeat imports a no-op; changed snapshots are rejected after the destination has data. SQLite remains on disk.

## UI and checks

[Reference / actual comparison](screenshots/p1-2/index.html) includes desktop login, registration and mobile login. [Onboarding](screenshots/p1-2/onboarding.png), [Account settings](screenshots/p1-2/account.png). The auth screen uses CSS/SVG, fade/slide, sliding segmented selection, gentle decoration, focus rings, inline errors, guarded submissions, pending labels and success transition. System reduced motion disables auth decoration even after using full app animations.

```powershell
$env:TEST_DATABASE_URL='postgres://rhythm@127.0.0.1:5433/rhythm?sslmode=disable'
go test ./...
go vet ./...
pnpm --dir web test
pnpm --dir web build
pnpm --dir web e2e
```

Local testing can use a PostgreSQL container with `POSTGRES_HOST_AUTH_METHOD=trust` bound to 127.0.0.1:5433. Never use trust for deployment. Tests create/drop unique schemas, cover auth, isolation, OAuth state/linking, migrations, rollback, constraints, foreign keys, concurrency and import. Provider protocol tests use local HTTP mocks. Browser tests use real Go/PostgreSQL/email sessions and retain Web Speech mocks for voice UX.

CI provisions PostgreSQL and runs Go/race, frontend tests/build/E2E, Python/voice parity and Compose smoke, verifying session and data across a server restart.

Live Yandex/VK login requires configured applications and real-account verification. Protocol mocks do not confirm credentials, provider moderation or live consent screens. Password recovery mail is a later feature; the UI states its availability without claiming to send mail. Public legal pages describe runtime handling and must be completed by the deployment operator.
