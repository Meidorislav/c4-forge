# Roadmap

> Status: **draft**. The plan for reaching MVP in small steps, plus an outline of what follows.

## How to read this

- A **milestone** groups steps that deliver one capability.
- A **step** is one reviewable change (one PR) with its tests. When it is done, something new can be run or demonstrated.
- **Covers** lists the requirement IDs from [`requirements.md`](requirements.md) that the step implements, fully or partly.
- **Needs** lists what must exist first: an ADR, a document or an earlier step.
- **Status:** `todo`, `in progress`, `done`.

Steps are done in order unless noted. The canvas spike (M1) runs in parallel with M0 and M2.

## MVP

### M0 — Foundations

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 0.1 | Go service skeleton: module layout, config from env, structured logging, `/healthz`, Makefile, CI (lint + tests). | CI is green; the binary starts and answers `/healthz`. | NF-2, NF-6, NF-17 | — | done |
| 0.2 | PostgreSQL: `docker-compose.yml` with app + Postgres, `pgx` pool, embedded migrations applied at startup, integration-test setup against a real Postgres in CI. | `docker compose up` starts both; an empty migration runs; integration tests run in CI. | NF-1, NF-4 | ADR-0003 | todo |
| 0.3 | Web skeleton: React + TypeScript + Vite in `web/`, embedded into the Go binary, i18n scaffolding, no external assets. | The app page is served by the binary; CI builds and tests the frontend. | NF-2, NF-3, NF-14 | 0.1 | todo |
| 0.4 | Container image: multi-stage Dockerfile, compose uses it. | A clean machine runs the full app with `docker compose up`. | NF-1 | 0.2, 0.3 | todo |

### M1 — Canvas spike (parallel, throwaway)

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 1.1 | Prototype on React Flow on a throwaway branch; run the nine checks from ADR-0001. | ADR-0001 is marked Accepted, or revised with a new decision. The branch is not merged. | — | ADR-0001 | todo |

### M2 — Model engine (backend, no network yet)

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 2.1 | Metamodel and field registry for elements and relationships; hierarchy and reference validation on an in-memory model. | Unit tests cover valid and invalid models. | MD-1…MD-5, NF-9 | ADR-0003 | todo |
| 2.2 | Operations: `set`/`delete`, atomic batches, cascading deletes. Test cases are written as shared JSON fixtures, so the TypeScript client can run them too. | Fixture suite passes in Go. | MD-9, CO-3 | 2.1 | todo |
| 2.3 | Persistence: element/relationship tables, operation log, per-workspace sequence, transactional write path, loading a workspace into memory. | Integration tests: apply → restart → load gives the same model; failed commit leaves memory untouched. | NF-8, HI-1 (log only) | 0.2, 2.2 | todo |
| 2.4 | Views, placements and freeform shapes: tables, field registry entries, operations, cascades. | Fixture and integration tests extended to views and shapes. | DG-1, DG-4, FR-1 (data) | 2.3 | todo |

### M3 — Users and access

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 3.1 | ADR-0004: authentication and authorization (local accounts now, OIDC later, sessions, roles). | ADR accepted. | — | — | todo |
| 3.2 | Local accounts: sign-up of the first user as admin, login/logout, sessions, CSRF protection, login page. | A user can sign in; unauthenticated requests are refused. | AU-1, AU-4, NF-10, NF-11 | 3.1, 0.3 | todo |
| 3.3 | Admin user management: invite/create, disable, reset password. | Admin can manage users from the UI. | AU-4 | 3.2 | todo |

### M4 — Workspaces and realtime

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 4.1 | Workspace REST API and list page: create, rename, delete, search and sort; owner membership. | Workspaces can be managed in the UI. | WS-1, WS-2 | 2.3, 3.2 | todo |
| 4.2 | Workspace roles editor/viewer, enforced in REST; members UI. | A viewer cannot change anything; tests cover each endpoint. | AU-3 (partial) | 4.1 | todo |
| 4.3 | `docs/protocol.md` and the WebSocket endpoint: snapshot, submit batch, ack/reject, broadcast, idempotent batch IDs, catch-up by sequence number; permissions enforced. | A Go test client covers the whole protocol, including reconnect and retries. | CO-1, CO-3, CO-4, NF-10 | 4.2 | todo |
| 4.4 | Presence over the WebSocket: who is online, current view, cursor, selection, drag positions (never persisted). | Test client sees other clients' presence. | CO-2 | 4.3 | todo |

### M5 — Editor

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 5.1 | Client model store: TypeScript apply rules (passing the shared fixtures), optimistic updates, pending queue and rebase, reconnect and resend. No UI yet. | Fixture suite and store tests pass in CI. | CO-3, CO-4 | 2.4, 4.3 | todo |
| 5.2 | Read-only canvas: render a view with C4 nodes (basic shapes), derived relationships, scope boundary. | Seeded workspace renders correctly. | DG-7, DG-10, DG-15 (basic shapes) | 1.1, 5.1 | todo |
| 5.3 | Editing elements: create on canvas, move, inspector for attributes and "external"; remove from diagram vs delete from model with impact preview. | All actions persist and survive reload. | DG-4, DG-5 (create), DG-6, MD-3, MD-4, MD-9 | 5.2 | todo |
| 5.4 | Relationships: draw, edit, reverse, delete; several between one pair. | As above, for relationships. | MD-5 | 5.3 | todo |
| 5.5 | Navigation: model tree, view tree, reuse existing elements by drag and drop, "where is this element used". | An element can be placed on several views and edited once. | DG-5 (reuse), DG-12 | 5.3 | todo |
| 5.6 | View types and drill-down: open/create child views, breadcrumbs. | Landscape → container → component navigation works. | DG-1, DG-11 | 5.5 | todo |
| 5.7 | Implied relationships, with the list of underlying relationships. | Relationships between nested elements show on higher views. | DG-8 | 5.6 | todo |
| 5.8 | Live collaboration in the UI: remote changes, avatars, cursors, selections, ephemeral drag positions. | Two browsers edit one view together. | CO-1, CO-2 | 4.4, 5.4 | todo |
| 5.9 | Undo/redo of one's own actions. | Undo/redo works across all editing actions, also with a second user editing. | DG-18 | 5.8 | todo |
| 5.10 | Freeform layer: sticky note, rectangle, ellipse, text, arrow/line. | Shapes can be drawn, edited and moved; they never appear in the model. | FR-1 | 5.3 | todo |
| 5.11 | Search within a workspace (name, technology, tags) with jump-to-element. | Search finds elements and focuses them on a view. | SE-1 | 5.5 | todo |

### M6 — Import and export

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 6.1 | ADR-0005: Structurizr DSL strategy — parser approach, supported subset, mapping to our metamodel, layout of imported views, validation against Structurizr tooling. | ADR accepted. | — | — | todo |
| 6.2 | Own JSON export/import of a whole workspace. | Export → import on another instance gives an identical workspace. | EX-2, FR-6 | 2.4 | todo |
| 6.3 | Structurizr DSL export with golden tests. | Exported files match golden files and are accepted by Structurizr tooling. | SZ-2 | 6.1 | todo |
| 6.4 | DSL parser for the MVP subset, with clear errors for unsupported features. | Reference DSL files parse; unsupported features are reported with line numbers. | SZ-3, SZ-6 | 6.1 | todo |
| 6.5 | DSL import: map to the model, materialize views with an initial layout. | Import → export round-trips the model of the reference files. | SZ-1, SZ-7 | 6.3, 6.4 | todo |
| 6.6 | PNG export of a view. | Exported image matches what is on screen, including freeform shapes. | EX-1 (PNG) | 5.10 | todo |

### M7 — MVP release

| # | Step | Done when | Covers | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 7.1 | Operations docs: install, configuration, backup and restore, upgrade. | A new person installs and restores a backup using only the docs. | NF-5 | 0.4 | todo |
| 7.2 | Hardening: size limits, load check against the targets in requirements section 4, security review of auth and WebSocket. | Targets met or gaps recorded as issues. | NF-12 | M5, M6 | todo |
| 7.3 | Release `v0.1.0`: versioned image, changelog. | Image published; upgrade path from this version is tested from then on. | — | 7.1, 7.2 | todo |

## Open points

- **Layout of imported views.** SZ-7 (MVP) needs imported views to get positions, while general auto-layout (DG-13) is planned for v1. ADR-0005 must choose: a simple built-in layout for MVP, or pulling part of DG-13 into MVP.
- **Structurizr validation in CI.** Checking exported DSL with official Structurizr tooling needs a Java runtime in CI. ADR-0005 decides whether that is acceptable.
- **Order of M3 and M4.** Auth comes before realtime so that every stored operation has a real actor from the start. If a quick demo is needed earlier, 4.1–4.3 can run with a development-only fake user.

## After MVP (v1 outline)

Planned in detail once MVP is close; grouped by theme:

- **Access:** OIDC login, all workspace roles, teams, API tokens (AU-2, AU-3, AU-5, AU-7).
- **Modeling:** URL and properties, groups, deployment model and views, moving elements between parents, technology catalog (MD-3, MD-6, MD-7, MD-10, MD-11, DG-2).
- **Diagrams:** hidden relationships, auto-layout, routing and waypoints, tag styles, legend, align/distribute/snap, copy/paste (DG-9, DG-13…DG-17).
- **Freeform:** bound arrows, freehand, images, frames, boards, shape-to-element conversion (FR-2…FR-5).
- **Collaboration:** concurrent text editing, comments (CO-5, CO-6).
- **History:** change history UI, named versions (HI-1, HI-2).
- **Interop:** DSL re-import, `workspace.json` layout export, `!include` in archives, Mermaid export, SVG export, REST API (SZ-5, SZ-8, SZ-9, MM-1, EX-1, EX-3).
- **Product:** duplicate and group workspaces, global search, Prometheus metrics, Russian localization, dark theme (WS-3, WS-4, SE-2, NF-6, NF-14, NF-15).
