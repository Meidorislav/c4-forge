# ADR-0002: Collaborative editing engine

- **Status:** Accepted
- **Date:** 2026-09-28
- **Related requirements:** CO-1…CO-5, DG-18, HI-1, HI-2, NF-7, NF-8, NF-9, SZ-9, EX-3

## Context

Up to ~20 people edit one workspace at the same time (requirements, section 4). The engine decides how their changes are ordered, merged, validated, persisted and delivered.

Requirements that shape the choice:

- **NF-9.** The server validates every change and the model never becomes inconsistent. Examples: no relationship to a deleted element, no container inside a component, no cycles in the hierarchy.
- **NF-8.** A change confirmed by the server survives a restart.
- **CO-3 / CO-4.** Results are deterministic, and edits are not lost on a short disconnect.
- **HI-1 / HI-2.** Change history and versions.
- **EX-3 / SZ-9.** A REST API and DSL re-import change the model outside the editor, so the server must understand and apply changes itself.
- **CO-5 (v1).** Two people typing in the same description or sticky note must not overwrite each other.

Most of the data is **structured**: entities with a small set of fields (name, technology, parent, position…). Only a few fields are free text.

## Options considered

### A. Server-authoritative operations, last-writer-wins per field

Clients send operations: *set these fields of entity X*, *delete entity Y*. The server:

1. applies them in a single order;
2. validates them against the current model;
3. expands cascades;
4. persists the result;
5. broadcasts what was applied.

Clients apply their own operations optimistically. When confirmed state arrives, they rebase pending operations on top of it. Conflicts on the same field resolve by server order (last writer wins).

- **+** Validation, cascades, permissions and history are ordinary server code in Go, working on plain data.
- **+** The model is stored as regular rows, so search, REST API and DSL export read it directly.
- **+** Simple to reason about and to test; no extra runtime dependency.
- **−** Concurrent typing in the same text field loses one side's edits (CO-5).
- **−** Offline editing is limited to short disconnects. Long offline sessions are not a goal.

### B. CRDT for the whole workspace (Yjs)

The workspace is one Yjs document. Clients merge updates peer-to-peer style, and the server relays and stores them. Pure-Go Yjs ports have appeared recently (`reearth/ygo`, `Deln0r/ygo`, both MIT), so the server could read the document without a Node sidecar or cgo.

- **+** Merging and concurrent text editing come for free; strong offline story.
- **−** A CRDT merge always succeeds. The server cannot *reject* an invalid change, only apply a compensating one afterwards. Example: one user deletes a system while another adds a container to it. NF-9 becomes much harder.
- **−** The model lives in an opaque document. Search, REST API, history and DSL export need a projection into tables that must be kept in sync.
- **−** The Go ports are young, with small communities and essentially single maintainers. Our core data layer would depend on them.

### C. Operational transformation

Designed mainly for text. For a structured model it adds a lot of complexity over option A without clear benefits. Rejected.

### D. Locking (one editor per element or diagram)

Simple, but contradicts the real-time co-editing experience (CO-1, CO-2). Rejected.

## Decision

**Option A: server-authoritative operations with last-writer-wins per field**, for all structured data.

Concurrent text editing (CO-5, v1) is solved later and only for text fields. The likely way is a text CRDT (Yjs `Y.Text` via a Go port) for those fields alone, with the result written back into the field. That keeps the CRDT dependency's blast radius small. It will be a separate ADR once the Go ports have proven themselves.

### Rules of the engine

1. **Operations.** Two kinds exist: `set(kind, id, fields)` (a field-level patch; `null` removes a field) and `delete(kind, id)`. Entity IDs are generated on the client and are random and collision-free, so an entity can be created without a server round-trip.
2. **Atomic batches.** One user action (for example "create a container and place it on this diagram") is one batch. The server applies a batch completely or rejects it completely.
3. **Single order.** Each workspace has a monotonic sequence number. The server assigns the next number to every applied batch; that number defines the order for everyone.
4. **Validation and cascades on the server.** The server checks references, hierarchy rules and permissions. Deletes cascade on the server, for example deleting a system removes its containers, relationships, placements and scoped diagrams. The expanded operations are what gets broadcast.
5. **Durable before acknowledged.** A batch is committed to Postgres (entity changes and the operation log, in one transaction) before it is acknowledged or broadcast. This satisfies NF-8 without a background flush window.
6. **Idempotent retries.** Every batch carries a client-generated ID. After a reconnect, the client resends unacknowledged batches, and the server ignores batch IDs it has already applied.
7. **Catch-up on reconnect.** The client reports the last sequence number it saw. The server replies with the missed batches from the operation log, or with a full snapshot if the gap is too large.
8. **Rejection.** A rejected batch is dropped on the client, and its optimistic effect disappears after the rebase. The user gets a short notice. If the client state looks inconsistent, it requests a snapshot.
9. **Ephemeral vs persistent.** Presence data is broadcast but never stored: cursor, selection, current diagram, and *positions while dragging*. Only the final position on drop becomes an operation. This keeps write volume and history noise low.
10. **Undo/redo is client-side and per user.** Each local action records its inverse operations, computed against the state at the time of the action. Undo sends those inverses as a new batch, so it only reverts *your* changes. If someone else changed the same field since then, undo still wins by the normal last-writer-wins rule. If the inverse is no longer valid (for example the entity was deleted), that undo step is skipped.
11. **The operation log is the history.** HI-1 is built from the log. Named versions (HI-2) are snapshots that reference a sequence number.
12. **Transport.** One WebSocket per open workspace carries operations, acknowledgements, broadcasts and presence. JSON first; a binary encoding can come later if needed.

### Scaling (NF-7)

In MVP one application instance serves everything. For several instances, each workspace is owned by exactly one instance at a time. Requests are routed by workspace, or ownership is coordinated through Postgres (advisory locks, `LISTEN/NOTIFY` for hand-over). Nothing in the rules above assumes a single process. Ordering comes from the per-workspace sequence in the database, not from process memory.

## Consequences

**Positive**

- Model integrity is enforced in one place, in Go, on plain data.
- The same apply/validate code path serves the editor, the REST API, DSL import and restore-from-version.
- History and versions come almost for free from the operation log.
- No young third-party CRDT library in the core data path.

**Negative**

- Until the text CRDT lands (v1), concurrent typing in the same field loses edits. Mitigation for MVP: show who is editing a field (presence), so people rarely collide.
- The client needs a careful optimistic-update and rebase layer; it needs thorough tests.
- A Postgres commit per batch adds a few milliseconds of latency. That is acceptable within the 200 ms target, and dragging does not write until drop.
- Long offline editing is not supported.

**Follow-ups**

- ADR-0003: data model and Postgres storage — entity tables, operation log, sequence numbers, snapshots.
- A later ADR on collaborative text fields (CO-5).
- A protocol document with message formats, written together with the first implementation step.

## References

- Yjs ports to other languages: https://docs.yjs.dev/ecosystem/ports-to-other-languages
- reearth/ygo: https://github.com/reearth/ygo
- Deln0r/ygo: https://github.com/Deln0r/ygo
- automerge-go (cgo wrapper over the Rust core): https://github.com/automerge/automerge-go
