# Structured School Catalogue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Import the repository school catalogue into PostgreSQL and associate every user-progress row with a real school record.

**Architecture:** `schools.json` remains the editable source in Git. The Go API validates and transactionally upserts it into `schools` at startup, serves `/api/schools`, and uses a foreign key from `progress.school_id`. The browser reads the API catalogue and keeps existing per-user progress behavior.

**Tech Stack:** Go 1.22, `database/sql`, PostgreSQL, vanilla JavaScript.

## Global Constraints

- Preserve users, progress, JWT access tokens, refresh sessions, theme preferences, Nginx port 26, and loopback API port 2026.
- Do not print or commit database credentials, generated passwords, hashes, tokens, or connection strings.
- `schools.json` is the deployment source of truth; PostgreSQL is the runtime source of truth.
- Reject malformed/duplicate school IDs before writing any school rows.

---

### Task 1: Add validated catalogue import and relational migration

**Files:**
- Modify: `server/main.go`, `server/Dockerfile`, `server/docker-compose.yml`, `server/verify.sh`
- Create: `server/schools.go`, `server/schools_test.go`

**Interfaces:**
- Consumes: `/app/schools.json`, mounted read-only from the repository root.
- Produces: `syncSchools(ctx context.Context) error` and table `schools(id TEXT PRIMARY KEY, ...)`.

- [ ] **Step 1: Write failing catalogue tests**

```go
func TestDecodeSchoolsRejectsDuplicateID(t *testing.T) {
    _, err := decodeSchools(strings.NewReader(`{"schools":[{"id":"a"},{"id":"a"}]}`))
    if err == nil { t.Fatal("duplicate ID accepted") }
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `cd server && go test ./... -run TestDecodeSchoolsRejectsDuplicateID`

Expected: FAIL because no catalogue decoder exists.

- [ ] **Step 3: Implement decoder and transactional upsert**

Define `school` with every JSON field. Parse `schools.json`, ensure each non-empty ID is unique, then within one transaction upsert all columns into `schools`. Add the table before `progress`; delete only progress rows whose school ID is absent from `schools`, then add `FOREIGN KEY (school_id) REFERENCES schools(id)` when absent.

- [ ] **Step 4: Mount source data and run import**

Add the Compose volume:

```yaml
volumes:
  - ../schools.json:/app/schools.json:ro
```

Call `syncSchools` after schema migration and before bootstrap. Extend `verify.sh` to print only the `schools` table row count.

- [ ] **Step 5: Run validation**

Run: `cd server && gofmt -w *.go && go test ./... && go vet ./...`

Expected: PASS.

### Task 2: Serve structured schools and use them in the browser

**Files:**
- Modify: `server/main.go`, `app.js`, `server/schools_test.go`

**Interfaces:**
- Consumes: `GET /api/schools` unauthenticated JSON `{updated_at, schools}`.
- Produces: browser `rows` loaded from the database catalogue.

- [ ] **Step 1: Write failing API ordering test**

```go
func TestSchoolsHandlerReturnsJSON(t *testing.T) {
    // Seed two schools, invoke GET /api/schools, and assert response contains schools.
}
```

- [ ] **Step 2: Implement API and frontend fetch**

Add `GET /api/schools`; query all catalogue columns and return `{schools:[...]}`. Replace the frontend `fetch('./schools.json')` call with `apiFetch('/schools')` only after a refresh/login succeeds.

- [ ] **Step 3: Run complete checks**

Run: `cd server && go test ./... && go vet ./... && node --check ../app.js && bash deploy_test.sh`

Expected: PASS.

### Task 3: Run the authorized remote migration and seed the first account

**Files:**
- Modify: `server/verify.sh`

**Interfaces:**
- Consumes: server-only database configuration without displaying it.
- Produces: table/count-only migration result and a one-time generated initial-password output.

- [ ] **Step 1: Run local preflight**

Run: `cd server && go test ./... && go vet ./... && docker compose config`

Expected: PASS except an unavailable local Docker daemon, if present.

- [ ] **Step 2: Run remote migration once**

Run a temporary, non-committed Go migration entrypoint that loads the server-only environment, invokes `migrate`, `syncSchools`, and `bootstrap`, then exits. Generate a random first password in process memory; only its Argon2id hash is inserted.

- [ ] **Step 3: Verify remote state**

Run a read-only query that reports counts for `schools`, `users`, `progress`, and `refresh_tokens`; do not print credentials or hashes.

- [ ] **Step 4: Commit implementation**

```bash
git add server app.js docs
git commit -m "feat: store schools in postgres"
```

## Self-Review

- Tasks 1-2 cover schema, import, foreign key, API, and browser replacement.
- Task 3 covers the explicitly authorized remote database mutation and redacted verification.
- API names and the `school` model match across import, persistence, and browser consumption.
