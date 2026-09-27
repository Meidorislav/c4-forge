# c4-forge — Requirements

> Status: **accepted**; updated as development goes on. This document captures *what* we are building and *why*. *How* is decided separately in ADRs (`docs/adr/`).

## 1. Vision

c4-forge is an open-source tool for modeling software architecture with the [C4 model](https://c4model.com). A team deploys it on its own server and edits diagrams together in real time.

Principles:

1. **Model first, diagrams second.** An element lives in a shared model and can appear on many diagrams; a change is visible everywhere. C4 levels are linked by drill-down navigation.
2. **Compatible with architecture-as-code.** The model can be imported from and exported to Structurizr DSL.
3. **C4 model and freeform drawing on one canvas** — notes, sticky notes and sketches where strict notation is not enough.
4. **Fully self-hosted**, with no mandatory external dependencies (works in air-gapped environments).

## 2. Glossary

| Term | Meaning |
| --- | --- |
| **Instance** | A single c4-forge installation on an organization's server. |
| **Workspace** | The unit of modeling: one C4 model plus its diagrams. Corresponds to `workspace` in Structurizr. |
| **Model** | The set of elements and relationships of a workspace, independent of diagrams. |
| **Element** | A model entity: person, software system, container, component, deployment node, etc. |
| **Relationship** | A directed link between two model elements, with a description and technology. |
| **Diagram (view)** | A view of part of the model with a manual layout. May contain freeform shapes. |
| **Diagram scope** | The element a diagram looks "inside" (a system for a container diagram, a container for a component diagram). |
| **Implied relationship** | A relationship shown on a higher-level diagram but derived from relationships between nested elements. |
| **Freeform shape** | A canvas object outside the model: sticky note, rectangle, text, arrow, freehand line, image. Belongs to a single diagram. |
| **Board** | A diagram with no C4 level — freeform drawing only. |

## 3. Users and scenarios

### 3.1. Roles

- **Architect / tech lead** — creates and maintains the model, draws diagrams, runs architecture sessions.
- **Developer** — reads diagrams, refines their own part (containers, components), sometimes edits the model as code in a repository.
- **Reader** (manager, analyst, newcomer) — views and comments, does not edit.
- **Instance administrator** — deploys, upgrades, backs up, manages users and access.

### 3.2. Key scenarios

- **S1. Modeling from scratch.** An architect creates a workspace, draws the system landscape, drills into a system, draws its containers, then components. Elements are reused across diagrams.
- **S2. Collaborative session.** Several people edit the same diagram at once, see each other's cursors and changes, and add sticky notes and sketches.
- **S3. Migrating from Structurizr.** A team imports an existing `workspace.dsl` and continues working visually.
- **S4. Model in a repository.** A team keeps the DSL in git, exports changes from c4-forge and/or imports an updated DSL back.
- **S5. Documentation.** A developer embeds a diagram into a README or Confluence as an image or Mermaid code.
- **S6. Onboarding.** A newcomer explores the system: from the landscape downwards, checks where else an element is used, reads descriptions and comments.
- **S7. Operations.** An admin brings up an instance with a single `docker compose up`, configures corporate SSO login, and takes backups.

## 4. Scale and constraints

Target load for one instance (guideline, not a hard limit):

| Parameter | Target |
| --- | --- |
| Users per instance | up to ~200 (one organization, several teams) |
| Workspaces | up to hundreds |
| Elements in one workspace model | up to ~2,000 |
| Elements + shapes on one diagram | up to ~300 without noticeable lag |
| Concurrent editors of one workspace | up to ~20 |
| Latency of delivering changes to other participants | < 200 ms on a local network |

Multi-tenancy (SaaS with tenant isolation) is **not** required: one instance = one organization.

## 5. Functional requirements

Priorities:

- **MVP** — the minimal useful version that can be handed to a team.
- **v1** — the first complete release.
- **Later** — later / on demand.

### 5.1. Workspaces

| ID | Requirement | Priority |
| --- | --- | --- |
| WS-1 | Create, rename, delete a workspace; description. | MVP |
| WS-2 | Workspace list with search and sorting by last modified. | MVP |
| WS-3 | Duplicate a workspace. | v1 |
| WS-4 | Group workspaces by team/folder. | v1 |
| WS-5 | Archive instead of delete; trash. | Later |

### 5.2. C4 model

The c4-forge metamodel is a **superset of the Structurizr metamodel**, so that import/export is lossless for the model.

| ID | Requirement | Priority |
| --- | --- | --- |
| MD-1 | Element types: Person, Software System, Container, Component. | MVP |
| MD-2 | Hierarchy: a Container belongs to a Software System, a Component to a Container. | MVP |
| MD-3 | Element attributes: name, description, technology (for containers/components), tags, URL, arbitrary key–value properties. | MVP (name, description, technology, tags), v1 (URL, properties) |
| MD-4 | "External" flag (location external / tag), visually distinct. | MVP |
| MD-5 | Relationships: directed, with description, technology and tags; multiple relationships between the same pair of elements. | MVP |
| MD-6 | Element groups (`group` in Structurizr). | v1 |
| MD-7 | Deployment model: environments, deployment nodes, infrastructure nodes, software system/container instances. | v1 |
| MD-8 | Perspectives (Structurizr). | Later |
| MD-9 | Cascading delete: deleting an element removes nested elements, their relationships and their placements on diagrams; the affected items are shown before deletion. | MVP |
| MD-10 | Move an element to another parent (e.g. a container to another system). | v1 |
| MD-11 | Technology catalog with icons (Postgres, Kafka, Go…) to quickly fill in the "technology" field. | v1 |

### 5.3. Diagrams

| ID | Requirement | Priority |
| --- | --- | --- |
| DG-1 | Diagram types: System Landscape, System Context, Container, Component. | MVP |
| DG-2 | Deployment diagrams. | v1 |
| DG-3 | Dynamic diagrams (numbered scenario steps) / flows — scenarios on top of a diagram. | Later |
| DG-4 | Manual layout: drag elements around; positions are stored per diagram. | MVP |
| DG-5 | Add an existing model element to a diagram (reuse) and create a new one directly on the canvas. | MVP |
| DG-6 | Removing from a diagram ≠ deleting from the model; both actions are available and clearly distinguished. | MVP |
| DG-7 | Relationships between elements on a diagram are shown automatically when both ends are present. | MVP |
| DG-8 | Implied relationships: relationships between nested elements are shown on higher levels (visually distinct); the user can see which relationships they are derived from. | MVP |
| DG-9 | Hide individual relationships (including implied ones) on a specific diagram. | v1 |
| DG-10 | Scope boundary: on container/component diagrams, nested elements are enclosed in the parent's boundary. | MVP |
| DG-11 | Drill-down: open (or create) an element's child diagram; breadcrumbs to go back. | MVP |
| DG-12 | Navigation: diagram tree, model tree, "where else is this element used". | MVP |
| DG-13 | Auto-layout (for imported diagrams and on demand). | v1 |
| DG-14 | Relationship routing: straight, orthogonal, curved; waypoints. | v1 |
| DG-15 | Styles: colors/shape/icon by tag (like `styles` in Structurizr). Basic shapes: box, person, cylinder (database), queue, web browser, mobile. | MVP (basic shapes), v1 (tag-based styles) |
| DG-16 | Diagram legend (generated from the types and styles in use). | v1 |
| DG-17 | Align, distribute, snap to grid, multi-select, copy/paste. | v1 |
| DG-18 | Undo/redo (of one's own actions, aware of collaborative editing). | MVP |

### 5.4. Freeform drawing

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-1 | A freeform shape layer on any diagram: sticky notes, rectangle, ellipse, text, arrow/line. | MVP |
| FR-2 | Freeform arrows can bind to shapes and to model elements (and move with them), but **do not create** relationships in the model. | v1 |
| FR-3 | Freehand drawing (pen), images (file upload), frames. | v1 |
| FR-4 | Boards — diagrams without a C4 level, freeform drawing only. | v1 |
| FR-5 | Convert a freeform shape into a model element ("this rectangle is actually a service"). | v1 |
| FR-6 | Freeform shapes are not included in the Structurizr DSL export (except what the DSL can express), but are preserved in our own format and in images. | MVP |
| FR-7 | Shape templates and libraries (AWS/GCP/K8s cloud icons). | Later |

### 5.5. Collaboration

| ID | Requirement | Priority |
| --- | --- | --- |
| CO-1 | Other participants' changes appear in real time without reloading. | MVP |
| CO-2 | Presence: who is in the workspace, on which diagram, other participants' cursors and selections. | MVP |
| CO-3 | Concurrent edits of the same element never corrupt data; the result is deterministic and identical for everyone. | MVP |
| CO-4 | A short connection loss does not lose local edits: they are sent after reconnecting. | MVP |
| CO-5 | Collaborative text editing (descriptions, sticky notes) without overwriting each other's edits. | v1 |
| CO-6 | Comments on elements, relationships and canvas points; threads; mentions; "resolved". | v1 |
| CO-7 | "Follow a user" (see their viewport). | Later |

### 5.6. History and versions

| ID | Requirement | Priority |
| --- | --- | --- |
| HI-1 | Workspace change history: who, what, when. | v1 |
| HI-2 | Named versions (snapshots) of the model; view and restore. | v1 |
| HI-3 | Compare two versions (added/removed/changed). | Later |
| HI-4 | Drafts: changes in a "branch" that are applied later. | Later |

### 5.7. Import and export

#### Structurizr DSL

| ID | Requirement | Priority |
| --- | --- | --- |
| SZ-1 | Import a `workspace.dsl` into a new workspace. | MVP |
| SZ-2 | Export a workspace to a `workspace.dsl` that Structurizr (Lite/CLI) accepts without errors. | MVP |
| SZ-3 | DSL subset supported in MVP: `workspace`, `model`, `person`, `softwareSystem`, `container`, `component`, `->` relationships, `description`, `technology`, `tags`, `url`, `properties`, `group`, `!identifiers`, `views` (`systemLandscape`, `systemContext`, `container`, `component`) with `include`/`exclude`/`autoLayout`, `styles` (`element`, `relationship`). | MVP |
| SZ-4 | Deployment model and deployment views; `dynamic` views. | v1 / Later |
| SZ-5 | `!include` of files — only within an uploaded archive (no fetching by URL: the instance may be air-gapped). | v1 |
| SZ-6 | Not supported (with a clear error message): `!script`, `!plugin`, `!docs`, `!adrs`, external `themes` by URL. | MVP |
| SZ-7 | DSL views are defined by rules (`include *`), ours by explicit layout. On import, the rules are "materialized" into a set of elements on the diagram with auto-layout. | MVP |
| SZ-8 | Layout on export: additionally export `workspace.json` (Structurizr format) with element coordinates. | v1 |
| SZ-9 | Re-import DSL into an existing workspace: update the model by identifiers, preserving manual layout and freeform shapes; preview changes before applying. | v1 |
| SZ-10 | Sync with a git repository (DSL in the repo as the source of truth, or two-way). The architecture must not rule this out. | Later |

Compatibility properties:

- **Model round-trip.** "Export → import" preserves the model (elements, relationships, attributes, tags) losslessly.
- **Formatting is not preserved.** "Import → export" preserves the meaning of the model, but not the formatting and comments of the source file.
- **Stable identifiers.** Elements have DSL-friendly identifiers that stay stable across exports, so git diffs are meaningful.

#### Mermaid

| ID | Requirement | Priority |
| --- | --- | --- |
| MM-1 | Export a C4 diagram to Mermaid C4 syntax (`C4Context`, `C4Container`, `C4Component`) for embedding in Markdown/GitHub/Confluence. | v1 |
| MM-2 | Import Mermaid C4 (the diagram is merged into the model). | Later |

#### Other

| ID | Requirement | Priority |
| --- | --- | --- |
| EX-1 | Export a diagram to PNG and SVG. | MVP (PNG), v1 (SVG) |
| EX-2 | Our own export/import format for a whole workspace (JSON, including layout and freeform shapes) — for backups and moving between instances. | MVP |
| EX-3 | Public REST API to read/write the model (integrations, CI). | v1 |
| EX-4 | Diagram embedding (embed link, always up-to-date image). | Later |

### 5.8. Users and access

| ID | Requirement | Priority |
| --- | --- | --- |
| AU-1 | Local accounts (email + password); the first user becomes the administrator. | MVP |
| AU-2 | Login via OIDC (Keycloak, GitLab, Google, Azure AD, etc.). | v1 |
| AU-3 | Workspace-level roles: owner, editor, commenter, viewer. | MVP (editor/viewer), v1 (all) |
| AU-4 | Instance-level roles: admin, user. The admin manages users. | MVP |
| AU-5 | Teams (user groups) and granting access to a team. | v1 |
| AU-6 | "View-only" link for people without an account (enabled by the admin). | Later |
| AU-7 | API tokens for integrations/CI. | v1 |

### 5.9. Search

| ID | Requirement | Priority |
| --- | --- | --- |
| SE-1 | Search elements by name/technology/tags within a workspace. | MVP |
| SE-2 | Global search across all accessible workspaces. | v1 |

## 6. Non-functional requirements

### 6.1. Deployment and operations

- **NF-1.** Distributed as an application Docker image plus a `docker-compose.yml` with two services: the application and **PostgreSQL**. Starts with a single command.
- **NF-2.** The application is a single (Go) binary with the frontend embedded; configured via environment variables.
- **NF-3.** Air-gapped operation: no calls to the internet at runtime (fonts, icons, CDN, telemetry — all bundled or disabled).
- **NF-4.** Database migrations are applied automatically on startup; upgrading requires no manual steps other than replacing the image.
- **NF-5.** Backups with standard Postgres tooling (`pg_dump`), plus workspace export to our own format (EX-2).
- **NF-6.** Health-check endpoint, structured logs; Prometheus metrics in v1.
- **NF-7.** Horizontal scaling (multiple application instances) is not required for MVP, but the architecture must not rule it out.

### 6.2. Reliability and data

- **NF-8.** A change confirmed by the server is not lost when the application restarts.
- **NF-9.** The server validates every change (referential integrity of the model); a client cannot bring the model into an inconsistent state.

### 6.3. Security

- **NF-10.** All endpoints (including the realtime channel) require authentication and check workspace permissions.
- **NF-11.** Passwords use a strong hash (argon2id/bcrypt); sessions use httpOnly cookies; CSRF protection.
- **NF-12.** Limits on message/upload size; DSL import never executes code or accesses the network.

### 6.4. Client

- **NF-13.** Latest versions of Chrome, Firefox, Safari, Edge.
- **NF-14.** UI in English; i18n architecture from day one, Russian localization in v1.
- **NF-15.** Light and dark themes in v1.

### 6.5. Development and licensing

- **NF-16.** The project is licensed under Apache-2.0. All dependencies must have compatible licenses (no copyleft that hinders self-hosted use, no paid-license or watermark requirements in production).
- **NF-17.** Automated tests for backend and frontend, CI on every PR. Structurizr compatibility is verified against a set of reference DSL files (golden tests).
- **NF-18.** Development in small increments: each step is a separate PR with tests.

## 7. Out of scope

- Cloud SaaS and multi-tenancy.
- A full-featured online whiteboard (video calls, timers, voting, presentations, etc.). Freeform drawing complements C4; it is not a standalone product.
- Other notations (UML, ArchiMate, BPMN).
- Code generation from the model and reverse-engineering the model from code.

## 8. Open questions

1. **Model ↔ DSL as the source of truth.** If git sync arrives in v1/Later, which side wins on conflict — changes in the UI or in the repository? (Affects SZ-9, SZ-10.)
2. **Dynamic diagrams vs flows.** Do we build flows (steps on top of any diagram), Structurizr-style dynamic views, or map one onto the other?
3. **Where styles live.** Tag-based styles at the workspace level (as in Structurizr), or also at the instance level (a shared organization theme)?
4. **Comments and freeform shapes in DSL export.** Drop silently, warn, or preserve them in `properties`/comments?
5. **Default access.** Is a new workspace visible to everyone in the instance (wiki-style) or only to its creator?

## 9. Next steps

ADRs for the key decisions (in dependency order):

1. Canvas library (C4 + freeform drawing) — with a prototype spike.
2. Collaborative editing engine (server-authoritative LWW vs CRDT).
3. PostgreSQL storage (relational schema vs JSONB, change history).
4. Data model (metamodel as a superset of Structurizr, identifiers).
5. Structurizr DSL strategy (parser, supported subset, golden tests).
6. Authentication and authorization.

Then an MVP roadmap broken down into small steps.
