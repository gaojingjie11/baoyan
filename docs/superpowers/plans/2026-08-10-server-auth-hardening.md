# Server Authentication Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy one server-hosted frontend/API with per-user progress, secure-at-rest credentials and refresh sessions, and repeatable GitHub-triggered deployment.

**Architecture:** Nginx serves the static application and proxies `/api/` to a Go process bound to loopback. Go owns authentication, session rotation, user preferences, and normalized per-school progress; PostgreSQL owns durable rows. The browser holds only a short-lived access token in memory and uses a same-origin refresh cookie.

**Tech Stack:** Go 1.22, `net/http`, PostgreSQL, `golang.org/x/crypto/argon2`, Docker Compose, Nginx, GitHub Actions, vanilla JavaScript.

## Global Constraints

- Do not commit database connection strings, passwords, bootstrap credentials, JWT keys, peppers, access tokens, or refresh tokens.
- Keep deployment HTTP-only because the user explicitly deferred domain/HTTPS; document it as unsafe for untrusted networks.
- Bind the Docker API port only to `127.0.0.1:2026`.
- Remove Vercel from runtime and deployment configuration.
- Use same-origin `/api`; do not configure CORS for the production path.
- Access token TTL is 15 minutes and refresh-session TTL is 7 days.
- Password hashes use Argon2id and refresh-token storage uses SHA-256 token digests.
- Run `gofmt`, `go vet ./...`, and `go test ./...` before deployment.

---

### Task 1: Make server configuration and the container reproducible

**Files:**
- Modify: `server/.env.example`, `server/Dockerfile`, `server/docker-compose.yml`, `server/setup.sh`, `server/nginx.conf`, `.github/workflows/deploy.yml`, `README.md`, `server/README.md`, `.gitignore`
- Create: `server/deploy.sh`, `server/.env.schema`

**Interfaces:**
- Consumes: `/etc/baoyan/baoyan.env` on the server.
- Produces: `server/deploy.sh`, callable as `sudo /opt/baoyan/server/deploy.sh` and from GitHub Actions.

- [ ] **Step 1: Write deployment configuration checks**

Create `server/deploy_test.sh` with checks that reject an API port other than loopback, reject a tracked `.env`, require `go.sum`, and require `docker compose config` to resolve with a dummy environment file.

```bash
grep -F '127.0.0.1:2026:8080' server/docker-compose.yml
git ls-files --error-unmatch server/.env >/dev/null && exit 1 || true
test -f server/go.sum
```

- [ ] **Step 2: Run the checks and verify the current configuration fails**

Run: `bash server/deploy_test.sh`

Expected: failure because the current compose port is public and `go.sum` is absent.

- [ ] **Step 3: Implement server-only configuration**

Make Compose use `env_file: /etc/baoyan/baoyan.env`, set `ports: ["127.0.0.1:2026:8080"]`, and remove CORS/Vercel variables. Create `.env.schema` containing only variable names and comments. Update the Dockerfile to copy `go.mod go.sum`, run `go mod download` without `|| true`, build with `-trimpath`, and run as a non-root user.

`server/deploy.sh` must:

```bash
#!/usr/bin/env bash
set -euo pipefail
APP_DIR=/opt/baoyan
WEB_DIR=/var/www/baoyan
git -C "$APP_DIR" pull --ff-only
rsync -a --delete --exclude=.git --exclude=server --exclude=.github "$APP_DIR/" "$WEB_DIR/"
cd "$APP_DIR/server"
docker compose up -d --build --remove-orphans
curl -fsS http://127.0.0.1:2026/api/health
```

- [ ] **Step 4: Change Nginx and automation**

Keep one Nginx virtual host for static files and `/api/`, add upload/body-size and proxy timeout limits, and retain the explicit HTTP-only warning. Change GitHub Actions to invoke `sudo /opt/baoyan/server/deploy.sh` once after SSH host-key pinning from a `SERVER_KNOWN_HOSTS` secret; remove two separate rsync/pull paths.

- [ ] **Step 5: Run configuration verification**

Run: `bash server/deploy_test.sh && docker compose --env-file /tmp/baoyan-test.env config`

Expected: exit 0; rendered API port is `127.0.0.1:2026:8080`.

- [ ] **Step 6: Commit**

```bash
git add .gitignore README.md server .github/workflows/deploy.yml
git commit -m "chore: harden server deployment"
```

### Task 2: Replace authentication and schema handling

**Files:**
- Modify: `server/go.mod`, `server/main.go`
- Create: `server/auth.go`, `server/auth_test.go`, `server/store.go`, `server/store_test.go`, `server/go.sum`

**Interfaces:**
- Consumes: `JWT_SECRET`, `PASSWORD_PEPPER`, `BOOTSTRAP_USERNAME`, and `BOOTSTRAP_PASSWORD` from the process environment.
- Produces: `AuthService.Login`, `AuthService.Refresh`, `AuthService.Logout`, `AuthService.Authenticate`; `Store.UpsertProgress`; `Store.DeleteProgress`.

- [ ] **Step 1: Write failing authentication tests**

Create tests that assert Argon2id hashes verify only the correct password, expired JWTs are rejected, refresh-token digests do not contain the raw token, and a refresh token cannot be consumed twice.

```go
func TestPasswordHashRejectsWrongPassword(t *testing.T) {
    hash, err := hashPassword("correct", "pepper")
    if err != nil { t.Fatal(err) }
    if verifyPassword("wrong", hash, "pepper") { t.Fatal("wrong password verified") }
}
```

- [ ] **Step 2: Run the targeted test and verify it fails**

Run: `go test ./... -run 'TestPasswordHashRejectsWrongPassword|TestRefresh'`

Expected: failure because the existing SHA-256 functions and raw-token schema do not meet the test contract.

- [ ] **Step 3: Implement domain types and migrations**

Split authentication/storage concerns out of `main.go`. Add a migration list that creates `users(theme_config JSONB NOT NULL DEFAULT '{}')`, `progress`, and `refresh_tokens(token_hash BYTEA PRIMARY KEY, family_id UUID, revoked_at TIMESTAMPTZ)`. Do not use application startup to create a database; require `DATABASE_URL` to point at an existing database. Create only the bootstrap user specified by the two bootstrap environment variables, never constants.

- [ ] **Step 4: Implement token issuance and rotation**

Define these behaviors:

```go
func (a *AuthService) Login(ctx context.Context, username, password string) (Session, error)
func (a *AuthService) Refresh(ctx context.Context, rawRefresh string) (Session, error)
func (a *AuthService) Logout(ctx context.Context, rawRefresh string) error
func (a *AuthService) Authenticate(r *http.Request) (User, error)
```

Use Argon2id with a random 16-byte salt and encoded parameters. Generate refresh tokens from 32 cryptographically random bytes; hash them before persistence. Refresh must run in one transaction that locks/consumes the old row and inserts the replacement before commit.

- [ ] **Step 5: Implement HTTP handlers and cookie behavior**

`POST /api/auth/login` returns `{access_token, expires_in, user}` and sets `refresh_token` as `HttpOnly; SameSite=Lax; Path=/api/auth`. `POST /api/auth/refresh` reads only the cookie and rotates it. `POST /api/auth/logout` reads the cookie, revokes it, and expires it. `GET /api/me` requires an access token. Disable public registration in this release.

- [ ] **Step 6: Run Go quality checks**

Run: `gofmt -w server/*.go && cd server && go mod tidy && go test ./... && go vet ./...`

Expected: all checks pass and `server/go.sum` is tracked.

- [ ] **Step 7: Commit**

```bash
git add server/go.mod server/go.sum server/*.go
git commit -m "feat: secure user authentication sessions"
```

### Task 3: Make progress updates user-scoped and deletion-safe

**Files:**
- Modify: `server/store.go`, `server/main.go`, `server/store_test.go`, `app.js`

**Interfaces:**
- Consumes: `PUT /api/progress/{schoolID}` body `{ "status": "applied" }` or `{ "status": "" }`.
- Produces: `204 No Content`; one user cannot read or alter another user’s rows.

- [ ] **Step 1: Write failing repository/handler tests**

Cover: upsert creates one row, blank status deletes it, GET returns only the active user map, invalid status returns 400, and missing/invalid JWT returns 401.

```go
func TestPutProgressBlankStatusDeletesRow(t *testing.T) {
    // Seed one row for user A, invoke PUT with {"status":""}, then assert GET is {}.
}
```

- [ ] **Step 2: Run the targeted tests and verify they fail**

Run: `cd server && go test ./... -run 'TestPutProgress|TestGetProgress'`

Expected: failure because the old snapshot POST has no deletion path.

- [ ] **Step 3: Implement single-row handlers**

Route `GET /api/progress` and `PUT /api/progress/{schoolID}`. Validate `schoolID` against a safe length/character set and validate status against the existing status enum. For blank status execute `DELETE FROM progress WHERE user_id=$1 AND school_id=$2`; otherwise use one parameterized UPSERT. Handle every query, scan, row-iteration, commit, and rollback error.

- [ ] **Step 4: Scope browser cache and API calls**

Replace global `bjt_progress_v1` with `bjt_progress_v1:<userID>`. On login, load API progress before rendering; on logout, clear in-memory state. Send a single PUT for a changed select; import iterates validated entries and persists individual values. Check response status before claiming a save succeeded.

- [ ] **Step 5: Run focused and complete checks**

Run: `cd server && go test ./... && go vet ./...`

Expected: all progress handler tests and complete Go validation pass.

- [ ] **Step 6: Commit**

```bash
git add server app.js
git commit -m "feat: isolate progress by user"
```

### Task 4: Add per-user theme preferences and fix client rendering

**Files:**
- Modify: `index.html`, `styles.css`, `app.js`, `server/main.go`, `server/store.go`, `server/store_test.go`

**Interfaces:**
- Consumes: `PUT /api/me/theme` body `{ "theme": "light" | "dark" | "system" }`.
- Produces: `GET /api/me` user object with `theme`; root document has `data-theme` set to the saved value.

- [ ] **Step 1: Write failing API and browser behavior tests**

Add a Go handler test for rejected invalid themes and a browser-level smoke test (or a DOM test harness) asserting that a saved theme is applied after login. Add a regression test/specification that `.reminder-item .r-time` changes without removing `.r-school`.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `cd server && go test ./... -run TestUpdateTheme`

Expected: failure because no theme endpoint or validation exists.

- [ ] **Step 3: Implement preference and rendering changes**

Add an authenticated theme endpoint, save validated values in `users.theme_config`, and create a theme control in the user bar. Apply `document.documentElement.dataset.theme` from `/api/me`. In `tick()`, update the nested `.r-time` span instead of assigning `textContent` to the `.reminder-item` container.

- [ ] **Step 4: Validate the frontend**

Run: `node --check app.js && cd server && go test ./... && go vet ./...`

Expected: no JavaScript syntax errors and all Go tests pass.

- [ ] **Step 5: Commit**

```bash
git add app.js index.html styles.css server
git commit -m "feat: save user theme preferences"
```

### Task 5: Verify a safe deployment and document operations

**Files:**
- Modify: `README.md`, `server/README.md`, `server/setup.sh`, `.github/workflows/deploy.yml`
- Create: `server/verify.sh`

**Interfaces:**
- Consumes: a server with `/etc/baoyan/baoyan.env`, a checked-out `/opt/baoyan`, and Nginx configured from this repository.
- Produces: a redacted deployment verification report.

- [ ] **Step 1: Write failing deployment verifier tests**

Make `server/verify.sh` fail when health is unavailable, API binds publicly, or required tables are absent. It must print only `health=ok`, listener address, table names, and counts.

- [ ] **Step 2: Implement the verifier**

Use `curl -fsS http://127.0.0.1:2026/api/health`, `ss -ltn`, and a parameterized read-only `psql` query sourced from `/etc/baoyan/baoyan.env`; redact all connection strings and never echo the environment.

- [ ] **Step 3: Run local validation**

Run: `bash server/deploy_test.sh && bash server/verify.sh`

Expected: configuration check passes; verifier reports either a redacted successful connection or a clear missing-server dependency error locally.

- [ ] **Step 4: Run server deployment validation after push**

Run on server: `sudo /opt/baoyan/server/deploy.sh && sudo /opt/baoyan/server/verify.sh`

Expected: `health=ok`, loopback-only API listener, and the `users`, `progress`, and `refresh_tokens` tables present.

- [ ] **Step 5: Commit**

```bash
git add README.md server .github/workflows/deploy.yml
git commit -m "docs: document secure server operations"
```

## Self-Review

- Spec coverage: Tasks 1-5 cover Vercel removal, loopback-only Docker, same-origin Nginx, GitHub deployment, migration, bootstrap user, JWT/refresh rotation, theme preferences, progress deletion, tests, and server verification.
- Placeholder scan: no TBD/TODO or deferred implementation language remains in executable tasks.
- Type consistency: Auth service returns `Session`; progress uses one `PUT` payload; theme uses the `theme` enum consistently across storage, API, and browser.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-10-server-auth-hardening.md`.

1. Subagent-Driven — dispatch a fresh subagent per task and review between tasks.
2. Inline Execution — execute in this session with checkpoints.
