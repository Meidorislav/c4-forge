-- name: ListWorkspaceMembers :many
SELECT u.id AS user_id, u.email, m.role, u.disabled, u.must_change_password
FROM workspace_members m JOIN users u ON u.id=m.user_id
WHERE m.workspace_id=$1 ORDER BY lower(u.email), u.id;

-- name: GetWorkspaceMember :one
SELECT u.id AS user_id, m.role, u.disabled, u.must_change_password
FROM workspace_members m JOIN users u ON u.id=m.user_id
WHERE m.workspace_id=$1 AND m.user_id=$2;

-- name: AddWorkspaceMember :execrows
INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,$3)
ON CONFLICT DO NOTHING;

-- name: SetWorkspaceMemberRole :exec
UPDATE workspace_members SET role=$3 WHERE workspace_id=$1 AND user_id=$2;

-- name: RemoveWorkspaceMember :exec
DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2;

-- name: CountOtherEnabledOwners :one
SELECT count(*) FROM workspace_members m JOIN users u ON u.id=m.user_id
WHERE m.workspace_id=$1 AND m.user_id<>$2 AND m.role='owner' AND NOT u.disabled;
