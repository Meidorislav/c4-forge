# Model operation fixtures

Shared, language-independent cases for roadmap 2.2 (MD-9, CO-3,
ADR-0002, ADR-0003). Go runs them in `TestOperationFixtures`; the future
TypeScript model store must run the same file without translating the cases.
Run the Go suite with `go test ./internal/model -run TestOperationFixtures`.

`suite.json` contains:

- `workspaceId`: the workspace UUIDv7 used by every snapshot.
- `models`: named complete snapshots, each with `elements` and `relationships`.
  Arrays are sorted by ID. Entity field names match the field registry.
- `cases`: an `initial` model name and an ordered `operations` array.
  Successful cases have an `expected` model name and the exact `expanded`
  operation array. Rejected cases have an `error` with a stable `code` and
  `field` (empty when the error concerns the operation itself).

Optional fields omitted, null or at their zero value represent the same
snapshot value. Compare normalized models, not the literal spelling of JSON.
The fixtures are internal engine test data, not a workspace import format or
network protocol (roadmap 6.2 and 4.3).

## Apply rules

- Operations have `op` (`set` or `delete`), `kind` (`element` or
  `relationship`) and `id` (UUIDv7). An ID cannot be shared across kinds.
- `set` has a non-empty `fields` object. It creates a missing entity or patches
  an existing one. Unspecified fields stay unchanged. Lists and maps are
  replaced in full. `null` removes an optional field; it cannot remove a
  required field. `id` cannot appear inside `fields`. New entities must supply
  all required fields; the engine does not generate `dslId`.
- The registry checks every supplied value, including JSON types, before
  application. Unknown kinds, operations and fields are rejected.
- Operations run in array order on a private candidate. Cross-entity and
  complete-entity validation runs on the final candidate. This permits forward
  references and swapping unique DSL identifiers within one atomic batch.
  No intermediate candidate is published. Any failure returns no result and
  no expanded operations; the original snapshot stays unchanged on both
  success and failure. The caller may commit the candidate before publishing it
  when persistence is added in roadmap 2.3.
- Later patches to the same field win in batch order. This engine does not
  assign workspace sequence numbers or implement retry IDs yet.
- `delete` has no `fields`. Deleting a missing entity is a harmless no-op,
  retained in the expanded output. Deleting an element removes its current
  descendants and every relationship touching any removed element. Cascades
  run at the delete's position in the batch. A subsequent set can recreate an
  entity with the same ID, subject to final validation.
- Cascade expansion is deterministic: touching relationships in ID order,
  then elements in depth-first postorder, with siblings visited in ID order.
  Each affected relationship is emitted once per cascade. A relationship
  delete never deletes an element. Explicit operations retain their order.
- Replaying the expanded batch against the same original snapshot must produce
  the same final snapshot.

Placements, scoped views and their cascades belong to roadmap 2.4. UI impact
preview belongs to 5.3; a caller can inspect this immutable candidate and the
expanded operations without changing the original snapshot.
