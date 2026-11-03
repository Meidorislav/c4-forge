-- name: ListWorkspaces :many
SELECT w.id, w.name::text AS name, w.description, m.role, w.created_at, w.updated_at
FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
WHERE m.user_id = sqlc.arg(user_id) AND w.name IS NOT NULL
AND (strpos(lower(w.name), lower(sqlc.arg(search)::text)) > 0
 OR strpos(lower(w.description), lower(sqlc.arg(search)::text)) > 0)
ORDER BY
 CASE WHEN sqlc.arg(sort)::text = 'modified_desc' THEN w.updated_at END DESC,
 CASE WHEN sqlc.arg(sort)::text = 'modified_asc' THEN w.updated_at END ASC,
 CASE WHEN sqlc.arg(sort)::text = 'name_asc' THEN lower(w.name) END ASC,
 CASE WHEN sqlc.arg(sort)::text = 'name_desc' THEN lower(w.name) END DESC,
 w.id;

-- name: GetWorkspace :one
SELECT w.id, w.name::text AS name, w.description, m.role, w.created_at, w.updated_at
FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
WHERE w.id = $1 AND m.user_id = $2 AND w.name IS NOT NULL;

-- name: CreateWorkspaceMetadata :exec
INSERT INTO workspaces (id, name, description, created_by) VALUES ($1, $2, $3, $4);

-- name: CreateWorkspaceOwner :exec
INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner');

-- name: UpdateWorkspaceMetadata :exec
UPDATE workspaces SET name = $2, description = $3 WHERE id = $1;

-- name: DeleteWorkspace :exec
DELETE FROM workspaces WHERE id = $1;

-- name: LockOwnedWorkspaces :many
SELECT w.id FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
WHERE m.user_id = $1 AND m.role = 'owner' ORDER BY w.id FOR UPDATE OF w;

-- name: IsLastWorkspaceOwner :one
SELECT EXISTS (
 SELECT 1 FROM workspace_members m WHERE m.user_id = $1 AND m.role = 'owner'
 AND NOT EXISTS (
  SELECT 1 FROM workspace_members other JOIN users u ON u.id = other.user_id
  WHERE other.workspace_id = m.workspace_id AND other.role = 'owner'
  AND other.user_id <> m.user_id AND NOT u.disabled
 )
);
