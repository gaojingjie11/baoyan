# Structured School Catalogue Design

## Goal

Store the school catalogue and every user’s application progress in PostgreSQL, while preserving the existing multi-user authentication, JWT access token, refresh-cookie rotation, theme preference, and automatic deployment workflow.

## Data Model

`schools` is the canonical application catalogue. Its stable `id` is the string ID already present in `schools.json`; it is never derived from display order.

| Table | Key fields | Purpose |
|---|---|---|
| `schools` | `id`, `school`, `type`, `college`, `direction`, `major`, `start_at`, `end_at`, `status`, `site`, `admit`, `source`, `remark`, `updated_at` | Shared catalogue |
| `users` | `id`, `username`, `password_hash`, `theme_config` | Account and visual preference |
| `progress` | `user_id`, `school_id`, `status`, `updated_at` | One user’s status for one school; foreign-key references `schools(id)` |
| `refresh_tokens` | `token_hash`, `user_id`, `family_id`, `expires_at`, `revoked_at` | Rotated session records |

## Import and API Flow

1. `schools.json` remains the repository-maintained source file.
2. On API startup, `syncSchools` loads the JSON file mounted read-only into the container and upserts every record into `schools` in one transaction.
3. `GET /api/schools` returns the catalogue from PostgreSQL, ordered by hierarchy, deadline, direction, and ID.
4. `GET /api/progress` returns only the authenticated user’s rows. `PUT /api/progress/{schoolID}` rejects unknown school IDs through the foreign key and validates the status enum.
5. The frontend loads `/api/schools` and `/api/progress`; it no longer fetches `schools.json` directly.

## Initial Database State

- A single remote migration/import run is authorized after local validation.
- The migration creates the `schools` table, adds the `progress.school_id → schools.id` foreign key, and imports every `schools.json` record.
- Existing progress rows with IDs absent from the current catalogue are deleted before adding the foreign key, and the migration reports their count without exposing secrets.
- The first user is created by the existing bootstrap environment variables. The password is generated at runtime and stored only as an Argon2id hash; the generated value is shown once in the execution result.

## Error Handling and Validation

- Malformed or duplicate school IDs make startup fail before changing rows.
- School upserts are transactional; an import either fully succeeds or rolls back.
- API responses remain JSON and include no database credentials, password hashes, JWTs, or refresh-token values.
- The remote migration is preceded by a read-only schema check and followed by a table/count-only verification query.

## Deployment Constraint

The current operator-selected HTTP deployment on frontend port 26 is unchanged. API port 2026 remains loopback-only. No Vercel component is introduced.
