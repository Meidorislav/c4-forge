# Model persistence

Roadmap 2.3 implements NF-8 and the log portion of HI-1, following ADR-0002
and ADR-0003. `internal/modelstore` is the internal storage boundary; it has no
HTTP endpoints, authentication, workspace membership or retry protocol.

## Working set

Create a `Store` from a connected, migrated `pgxpool.Pool`. `Create` inserts an
empty workspace storage root. `Open` loads a working set: the sequence and all
entities are read in one read-only, repeatable-read transaction, then validated
with `model.New`. Recreating the store and pool and opening the workspace loads
the committed state without replaying history.

`Workspace.Snapshot` returns an immutable model and its sequence. Retaining a
snapshot across writes is safe. `Workspace.Apply` serializes callers of that
handle and returns a `Commit` only after the PostgreSQL commit succeeds:

1. Begin a transaction and lock the workspace row with `SELECT … FOR UPDATE`.
2. Compare the stored sequence with the working set. Reject a stale handle with
   `ErrStaleSnapshot`; reopen it before submitting another action.
3. Apply and validate the batch on a private candidate using the model engine.
4. Increment the workspace sequence and insert the expanded history, batch ID,
   caller-supplied actor ID and timestamp in `operations`.
5. Write changed rows from the final candidate, then remove absent rows.
6. Commit, then replace the handle's snapshot and sequence.

Every rejected validation, row-write failure or failed commit retains the old
in-memory snapshot. PostgreSQL rolls back the transaction on definite failures.
A lost connection during commit can leave the database outcome unknown: reopen
before retrying. The sequence comparison prevents a stale handle from overwriting
another committed batch. Sequence numbers are scoped to each workspace.

The batch ID is unique per workspace. Duplicate inserts fail without incrementing
the committed sequence. Transport-level idempotent replies and catch-up are
roadmap 4.3. Callers must supply real actor identities; the store does not invent
users. Authentication and the user foreign key arrive in M3/M4.

## Entity storage

Elements, relationships, views and shapes have typed identity/reference columns and a JSONB
`attrs` object. Placements store references, coordinates and optional dimensions in typed columns. The field registry selects which fields go into `attrs`.
Updates write the final validated candidate rather than executing transient
states from the input batch. Lists/maps retain the engine's replacement rules;
null removal is reflected in the new `attrs` object.

Reference constraints include `workspace_id`, so a parent or relationship endpoint
cannot point into another workspace. Deleting an element cascades through its
children and all relationships touching them. Deleting a workspace cascades to
its model, presentation and history. View deletion removes its shapes and
placements; deleting a scoped element also deletes the view. Direct database
cascade tests use the same fixtures as
the engine, without applying its expanded deletes first.

Foreign keys and DSL identifier uniqueness are deferred until commit. That allows
forward references and identifier swaps within a valid batch. Surviving references
are updated before absent roots are deleted, preserving retargeted entities.
`ON CONFLICT` targets the non-deferred primary key explicitly. Full hierarchy,
field-value and cross-kind identity validation remains in the server engine.

UUIDv7 identifiers use canonical lowercase hyphenated text in memory and JSON.
This keeps Go map identity identical to PostgreSQL's UUID identity after reload.
Empty maps and lists normalize to nil in immutable snapshots, matching omitted
optional JSON fields on reload.

## History

`model.ApplyWithChanges` returns the same candidate and expanded operations as
`Apply`, plus history for every operation:

- `existed`: whether the entity existed immediately before that operation;
- `before`: the previous value of every patched field for `set`, or every
  registered field for `delete`. Missing optional values are JSON null.

Repeated edits record intermediate previous values, not just the initial batch
snapshot. Cascaded deletes retain complete previous entity values. History data
owns its maps and JSON bytes and cannot mutate a model snapshot.

## Generated queries and verification

Queries live in `internal/db/queries`; `sqlc.yaml` generates
`internal/db/sqlc` from them and the forward-only migrations. Run `make generate`
with Go installed; the target pins sqlc v1.31.1 (MIT, development tool only).
Generated Go files are checked in, so ordinary builds do not run sqlc or download
it. CI checks that regeneration produces no diff.

`make test` runs the storage tests on a real Testcontainers PostgreSQL. Tests
cover the shared operation fixtures, reload after closing the original pool,
previous values, direct SQL cascades, workspace isolation, concurrent writers,
duplicate batches and rollback. A deferred constraint trigger deliberately fails
at commit to verify that neither memory, sequence, entity rows nor history is
published. CI runs on PostgreSQL 16 and 18.
