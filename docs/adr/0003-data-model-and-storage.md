# ADR-0003: Data model and PostgreSQL storage

- **Status:** Accepted
- **Date:** 2026-09-28
- **Related requirements:** MD-1…MD-11, DG-1…DG-16, FR-1…FR-6, HI-1…HI-3, SZ-1…SZ-9, SE-1, EX-2, NF-1, NF-4, NF-5, NF-7, NF-9
- **Builds on:** ADR-0002 (operations, per-workspace sequence, operation log, durable-before-ack)

## Context

We need to decide three things:

- **What the model consists of.** The metamodel must be a superset of Structurizr's (MD, SZ) and also cover diagram layout and freeform shapes.
- **How it is stored in PostgreSQL** (NF-1).

Storage has to serve several consumers:

- the collaboration engine (ADR-0002), which applies field-level operations, validates references and commits each batch together with the operation log;
- search (SE-1), the REST API (EX-3) and DSL export (SZ-2), which need to query the model directly;
- history and versions (HI-1, HI-2);
- evolution of the schema over time without painful migrations (NF-4).

Scale is modest (section 4 of the requirements): up to ~2,000 elements per workspace and hundreds of workspaces.

## Decision

### 1. Metamodel

Entities of a workspace, grouped by purpose. Every entity has an `id` and belongs to exactly one workspace.

**Model**

| Entity | Purpose | Key fields |
| --- | --- | --- |
| **Element** | Person, software system, container, component. In v1 also deployment node, infrastructure node, software system instance, container instance. | `type`, `parentId`, `name`, `description`, `technology`, `tags[]`, `url`, `properties{}`, `group`, `external`, `dslId`. Deployment types add `environment` and `instanceOf`. |
| **Relationship** | Directed link between two elements. | `sourceId`, `destinationId`, `description`, `technology`, `tags[]`, `url`, `properties{}`, `interactionStyle` |

**Presentation**

| Entity | Purpose | Key fields |
| --- | --- | --- |
| **View** | A diagram. | `type` (system landscape, system context, container, component; v1: deployment, board; later: dynamic), `key`, `title`, `description`, `scopeId`, `environment` |
| **Placement** | An element placed on a view. At most one per (view, element). | `viewId`, `elementId`, `x`, `y`, optional `width`/`height` |
| **Relationship presentation** | Per-view overrides for a relationship (DG-9, DG-14). Created only when something is customized. | `viewId`, `relationshipId`, `hidden`, `routing`, `vertices[]` |
| **Shape** | A freeform shape on one view (FR-1…FR-3). | `viewId`, `type` (sticky, rectangle, ellipse, text, arrow, freehand, image, frame), `geometry`, `style`, `text`, `z`, `frameId`, arrow `bindings` (to a shape or a placement) |
| **Style** | Tag-based styling (DG-15, v1). Mirrors Structurizr `styles`. | `target` (element/relationship), `tag`, `properties{}` |

**Outside the model**

| Entity | Purpose |
| --- | --- |
| **Asset** | Uploaded file (images for FR-3). Referenced by shapes. |
| **Comment** | Comment thread on an element, relationship or canvas point (CO-6, v1). |

Rules:

- **Hierarchy constraints are enforced by the server** (NF-9). A container's parent is a software system, and a component's parent is a container. There are no cycles.
- **Relationships shown on a view are never stored.** Visible and implied relationships (DG-7, DG-8) are derived from the model and placements. Only per-view overrides are stored.
- **Scope of a view.** A view's `scopeId` points to the element it looks into. Drill-down (DG-11) finds the view whose scope is the element.
- **`dslId` is the DSL identifier.** It is unique within the workspace and derived from the name when the element is created. It stays stable across renames, so exported DSL produces meaningful git diffs (SZ, "stable identifiers"). Import keeps the identifiers from the source file.
- **Shapes and comments stay out of the model.** They never take part in DSL export (FR-6) or implied relationships. They are part of our own export format (EX-2).

### 2. Storage: typed tables with a JSONB attribute column

Each entity kind gets its own table:

- **Typed columns** hold identity, references and fields we filter or join on: `id`, `workspace_id`, `type`, `parent_id`, `source_id`, `destination_id`, `view_id`, `element_id`, `name`, `dsl_id`, `x`, `y`, …
- **An `attrs JSONB` column** holds everything else: description, technology, tags, properties, url, style, geometry, and so on.

The database backs up the server's own checks. It enforces:

- foreign keys with `ON DELETE CASCADE` along the same cascade paths the engine uses (element → children, relationships, placements, scoped views; view → placements, shapes, relationship presentations);
- unique constraints: one placement per (view, element), unique `dsl_id` per workspace, unique view `key` per workspace;
- check constraints on `type` values.

The operation engine stays generic (`set(kind, id, fields)`). A per-kind **field registry** in Go declares, for each field, whether it is a column or an attrs key, how it is validated, and whether it is a reference. Adding an optional field is usually just an attrs key and needs no migration.

Search (SE-1) uses a `pg_trgm` index on element names plus a GIN index on tags.

Options considered for this part:

- **Generic entity table** (`kind`, `id`, `data JSONB`). Rejected: the database could not enforce references, and every query would dig into JSON.
- **Fully normalized columns for every field.** Rejected: a migration for every new attribute, and poor fit for open-ended fields like `properties`, `style` and `geometry`.
- **Document per workspace** (one JSONB blob). Rejected: whole-document writes for every batch, no relational integrity, and hard to query across workspaces (SE-2).

### 3. Working set and write path

- **The model is also kept in memory.** The instance that owns a workspace (ADR-0002, "Scaling") loads its model into memory on first use. Validation and cascade expansion run against this in-memory model, which is small at our scale.
- **Every applied batch is written in one transaction:**
  1. lock the workspace row (`SELECT … FOR UPDATE`) — this also guards against two instances writing the same workspace by mistake;
  2. increment `workspaces.seq`;
  3. insert the batch into the operation log;
  4. apply the row changes (insert, update of columns and a merge into `attrs`, delete);
  5. commit, then update the in-memory model, acknowledge the sender and broadcast.

  If the commit fails, the in-memory model is not touched and the batch is rejected.
- **Ownership across instances** (NF-7, later) is coordinated with a per-workspace advisory lock.

### 4. Operation log and versions

- **`operations` table.** Columns: `workspace_id`, `seq`, `batch_id`, `actor_id`, `created_at`, `ops JSONB`.
  - Primary key `(workspace_id, seq)`.
  - A unique `(workspace_id, batch_id)` makes retries idempotent (ADR-0002, rule 6).
  - `ops` stores the *expanded* operations, including cascades, **together with the previous values** of every changed field. That is enough to render history (HI-1) and to diff versions (HI-3) without replaying the whole log.
- **`versions` table.** Columns: `workspace_id`, `seq`, `name`, `created_by`, `created_at`, `snapshot JSONB`. The snapshot is the workspace in our own export format (EX-2).
- **Restoring a version** computes the difference between the current model and the snapshot and submits it as a normal batch. It therefore goes through validation, the log and the broadcast like any other change.
- **Retention.** The log is kept in full for now. Dragging writes only on drop, so volume stays moderate. Compaction (dropping old log entries behind the oldest kept version) can be added later without changing the model.

### 5. Identifiers

- **Entity IDs are UUIDv7**, generated by the client (ADR-0002, rule 1). They are time-ordered, which keeps B-tree indexes compact. The server validates the format and rejects duplicates.
- **`dslId` is separate** from the entity ID and serves humans and DSL files (section 1).

### 6. Other tables (details in later ADRs)

- `users`, `sessions`, `workspace_members` (roles). In v1 also `teams`, `team_members`, `api_tokens` (AU-*).
- `comments` (v1). Comments are stored and broadcast through the same WebSocket, but they are not model operations: they do not enter the operation log, versions or undo.
- `assets`. File content is stored in Postgres (`bytea`) with a size limit (NF-12), so a single `pg_dump` backs up everything (NF-5). S3-compatible storage can be added as an option later.

The model is single-tenant (one organization per instance), so there is no tenant column.

### 7. Tooling

- Driver: `pgx` v5.
- Queries: `sqlc`, which generates typed Go code from SQL. Dynamic queries are few (search, filters) and can be written by hand.
- Migrations: embedded in the binary and applied automatically at startup (NF-4). Schema changes are written as SQL files, which `sqlc` also reads. Migrations that need logic (for example, transforming data) are written in Go. Migrations are forward-only; every migration must be safe for the previous release's data. The migration library is chosen in the first implementation step; `goose` is the default candidate, since it supports both SQL and Go migrations.
- Supported PostgreSQL: 16 and newer. `docker-compose.yml` pins a current major version. The `pg_trgm` extension is required; it ships with the official image.

## Consequences

**Positive**

- The model is queryable with plain SQL for search, API, export and admin tooling.
- Referential integrity is checked twice: by the engine (user-facing errors) and by the database (last line of defence).
- New optional attributes rarely need a migration.
- History, diffs and version restore reuse the operation log and the normal write path.
- `pg_dump` is a complete backup, assets included.

**Negative**

- Every field must be registered in the field registry, which adds a small amount of ceremony per field.
- Keeping cascade rules identical in three places (Go engine, database foreign keys, client-side prediction) needs shared test fixtures.
- Storing assets in Postgres grows the database. That is acceptable at our scale, and external storage can be added later.
- The in-memory working set means a workspace has a warm-up cost on first open. It is negligible at ~2,000 elements.

**Follow-ups**

- The protocol document (ADR-0002 follow-up) uses the field names defined here.
- The Structurizr DSL strategy ADR maps DSL constructs onto this metamodel (for example `external`, `group`, hierarchical identifiers).
- The authentication ADR details the user and permission tables.
