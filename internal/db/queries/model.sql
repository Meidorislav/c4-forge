-- name: CreateWorkspace :exec
INSERT INTO workspaces (id) VALUES ($1);

-- name: WorkspaceSequence :one
SELECT seq FROM workspaces WHERE id = $1;

-- name: LockWorkspace :one
SELECT seq FROM workspaces WHERE id = $1 FOR UPDATE;

-- name: AdvanceSequence :one
UPDATE workspaces SET seq = seq + 1 WHERE id = $1 RETURNING seq;

-- name: ListElements :many
SELECT * FROM elements WHERE workspace_id = $1 ORDER BY id;

-- name: ListRelationships :many
SELECT * FROM relationships WHERE workspace_id = $1 ORDER BY id;

-- name: PutElement :exec
INSERT INTO elements (workspace_id, id, type, parent_id, name, dsl_id, attrs)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT ON CONSTRAINT elements_pkey DO UPDATE
SET type = EXCLUDED.type, parent_id = EXCLUDED.parent_id,
    name = EXCLUDED.name, dsl_id = EXCLUDED.dsl_id, attrs = EXCLUDED.attrs;

-- name: PutRelationship :exec
INSERT INTO relationships (workspace_id, id, source_id, destination_id, attrs)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT ON CONSTRAINT relationships_pkey DO UPDATE
SET source_id = EXCLUDED.source_id, destination_id = EXCLUDED.destination_id,
    attrs = EXCLUDED.attrs;

-- name: DeleteElement :exec
DELETE FROM elements WHERE workspace_id = $1 AND id = $2;

-- name: DeleteRelationship :exec
DELETE FROM relationships WHERE workspace_id = $1 AND id = $2;

-- name: InsertOperation :exec
INSERT INTO operations (workspace_id, seq, batch_id, actor_id, ops)
VALUES ($1, $2, $3, $4, $5);

-- name: ListViews :many
SELECT * FROM views WHERE workspace_id = $1 ORDER BY id;

-- name: ListPlacements :many
SELECT * FROM placements WHERE workspace_id = $1 ORDER BY id;

-- name: ListShapes :many
SELECT * FROM shapes WHERE workspace_id = $1 ORDER BY id;

-- name: PutView :exec
INSERT INTO views (workspace_id,id,type,key,scope_id,attrs) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT ON CONSTRAINT views_pkey DO UPDATE
SET type=EXCLUDED.type,key=EXCLUDED.key,scope_id=EXCLUDED.scope_id,attrs=EXCLUDED.attrs;

-- name: PutPlacement :exec
INSERT INTO placements (workspace_id,id,view_id,element_id,x,y,width,height) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT ON CONSTRAINT placements_pkey DO UPDATE
SET view_id=EXCLUDED.view_id,element_id=EXCLUDED.element_id,x=EXCLUDED.x,y=EXCLUDED.y,width=EXCLUDED.width,height=EXCLUDED.height;

-- name: PutShape :exec
INSERT INTO shapes (workspace_id,id,view_id,type,attrs) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT ON CONSTRAINT shapes_pkey DO UPDATE
SET view_id=EXCLUDED.view_id,type=EXCLUDED.type,attrs=EXCLUDED.attrs;

-- name: DeleteView :exec
DELETE FROM views WHERE workspace_id=$1 AND id=$2;

-- name: DeletePlacement :exec
DELETE FROM placements WHERE workspace_id=$1 AND id=$2;

-- name: DeleteShape :exec
DELETE FROM shapes WHERE workspace_id=$1 AND id=$2;
