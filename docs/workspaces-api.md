# Workspace API

M4/4.1 implements WS-1 and WS-2 under ADR-0004 and ADR-0006.
The API uses the session cookie from `auth-api.md`. All requests require an
active session and a completed mandatory password change. Mutations require
an exact configured `Origin` and `X-CSRF-Token` from `GET /api/auth/csrf`.
Responses use `Cache-Control: no-store`.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/workspaces?q=payments&sort=modified_desc` | 200, array of accessible workspaces |
| POST | `/api/workspaces` | 201, created workspace and owner membership committed together |
| GET | `/api/workspaces/{id}` | 200, accessible workspace |
| PUT | `/api/workspaces/{id}` | 200, replaced name and description; owner only |
| DELETE | `/api/workspaces/{id}` | 204; owner only; model, history and memberships deleted together |

POST and PUT require an object such as
`{"name":"Payments","description":"Payment processing architecture"}`.
Both fields are required strings. Names are trimmed, nonempty, at most 200
Unicode code points and cannot contain control characters. Descriptions are
at most 4000 code points; tabs and line breaks are allowed. Request bodies
are limited to 32 KiB, valid UTF-8 JSON, with no unknown fields or trailing
values. Duplicate workspace names are allowed.

Workspace responses contain `id`, `name`, `description`, `role`, `createdAt`
and `updatedAt` (timestamps in RFC 3339). Roles are `owner`, `editor`, `viewer`.
Only explicit members can discover workspaces; instance admins have no bypass.
Metadata changes and committed model batches update `updatedAt`; reads do not.

Search `q` is a case-insensitive literal substring of name or description,
trimmed and limited to 200 code points, without NUL characters. Sort accepts `modified_desc` (default),
`modified_asc`, `name_asc`, `name_desc`, with ascending ID as a stable tie-breaker.
Name sorting uses PostgreSQL's database collation on lowercase names. The
list is not paginated at the documented scale of hundreds of workspaces.

Errors are JSON objects with `error`: 400 `invalid input`, 401
`invalid credentials`, 403 `forbidden` (CSRF/origin, mandatory password change,
or a member without owner permission), 404 `not found` (missing workspace or
nonmember), or 500 `internal error`. Invalid IDs are 400. Unsupported sort
values are 400. DELETE returns no body. POST retries can create duplicates;
clients must not automatically retry mutations after an ambiguous response.

Account disabling returns 409 `last_owner` when the target is the last enabled
owner of any workspace. No account/session change commits, and no inaccessible
workspace details are returned. Member management and ownership transfer are
scheduled for 4.2.

## Upgrade behavior

Pre-M4 model-store workspace rows have no creator or membership. Migration
preserves them, with a null name and creator; they remain inaccessible through
this API. Do not infer ownership from history. There is no automatic assignment
to an administrator or normal administrator API for claiming these rows.
Back up the database before any explicit operator migration of such data.
