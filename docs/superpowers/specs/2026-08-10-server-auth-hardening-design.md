# Server-Hosted Authenticated Tracker Design

## Goal

Run the tracker’s frontend and API on one server, deploy both from GitHub, and give each account isolated progress data and theme preferences.

## Deployment Constraints

- Vercel is removed from the deployment path.
- Nginx serves the static frontend and proxies same-origin `/api/` requests to the Go container.
- The API container binds only to `127.0.0.1:2026` on the host.
- GitHub Actions invokes one server-side deployment script that pulls the pinned branch, builds the API, and syncs frontend assets.
- Server configuration and credentials live outside the repository in a protected environment file.
- The user has explicitly deferred HTTPS/domain setup. The deployment is therefore HTTP-only and must be treated as unsuitable for untrusted networks because credentials and sessions are observable in transit.

## Authentication

- `users` stores unique usernames, Argon2id password hashes, a JSONB `theme_config`, and timestamps.
- A bootstrap user is created only when `BOOTSTRAP_USERNAME` and `BOOTSTRAP_PASSWORD` are supplied through the server environment. Neither value is committed, logged, or shown by API responses.
- Successful login produces a 15-minute signed access JWT returned in JSON and a seven-day refresh token placed in an HttpOnly, SameSite=Lax cookie.
- The database stores only a SHA-256 digest of each refresh token, together with user ID, expiry, token family ID, and revocation time.
- Refresh uses a database transaction to atomically consume one refresh token, issue its replacement in the same token family, and reject reuse.
- Logout revokes the current refresh-token session. Existing access JWTs may remain usable until their 15-minute expiration.

## Progress and Preferences

- `progress` remains normalized as one `(user_id, school_id)` row per selected status.
- `PUT /api/progress/{schoolID}` writes exactly one status. A blank status deletes that row, so “未报名” reliably persists.
- `GET /api/progress` returns only the authenticated user’s progress map.
- `GET /api/me` returns the authenticated user and theme configuration; `PUT /api/me/theme` updates a validated theme configuration.
- The browser keys offline progress by authenticated user ID and never displays one account’s cache to another account when the API is unavailable.

## API Contract

| Endpoint | Method | Authentication | Result |
|---|---|---|---|
| `/api/health` | GET | none | process health |
| `/api/auth/login` | POST | none | access token + user; sets refresh cookie |
| `/api/auth/refresh` | POST | refresh cookie | rotated session + access token |
| `/api/auth/logout` | POST | refresh cookie | revokes session and clears cookie |
| `/api/me` | GET | access token | current user and theme |
| `/api/me/theme` | PUT | access token | validates and saves theme |
| `/api/progress` | GET | access token | user progress map |
| `/api/progress/{schoolID}` | PUT | access token | upserts or deletes one progress row |

## Error Handling and Operations

- API handlers return JSON for success and failure paths.
- Expired/invalid access tokens return 401; the frontend attempts one refresh and then returns to login.
- Refresh rotation errors never issue a replacement token.
- Database query, scan, row-iteration, and transaction errors are handled.
- Nginx and Docker health checks use `/api/health`.
- No database port or API port is publicly published by Docker.

## Validation

- Add `go.sum` and make Docker dependency installation fail on missing dependencies.
- Add Go tests for password verification, JWT expiration validation, refresh-token hashing, and authenticated handler behavior using `httptest` plus a test database abstraction.
- Run `gofmt`, `go vet ./...`, and `go test ./...` before deployment.
- After deployment, use an authenticated, read-only health/schema check that reports only table names and row counts, never connection values or tokens.

## Out of Scope

- HTTPS/domain configuration, external identity providers, password-reset email, and public self-registration.
- Migrating the static `schools.json` catalogue into PostgreSQL.
