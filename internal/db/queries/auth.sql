-- name: AuthInitialized :one
SELECT initialized FROM auth_state WHERE singleton = true;

-- name: LockAuthState :one
SELECT initialized FROM auth_state WHERE singleton = true FOR UPDATE;

-- name: SetAuthInitialized :exec
UPDATE auth_state SET initialized = true WHERE singleton = true;

-- name: CreateUser :exec
INSERT INTO users (id,email,email_canonical,password_hash,role) VALUES ($1,$2,$3,$4,$5);

-- name: FindUserByEmail :one
SELECT * FROM users WHERE email_canonical = $1;

-- name: UpdatePasswordHash :exec
UPDATE users SET password_hash = $2 WHERE id = $1 AND password_hash = $3;

-- name: CreateSession :exec
INSERT INTO auth_sessions (token_hash,user_id,absolute_expiry) VALUES ($1,$2,$3);

-- name: ResolveSession :one
UPDATE auth_sessions AS s SET last_activity = now()
FROM users AS u
WHERE s.token_hash = $1 AND s.user_id = u.id AND NOT u.disabled
  AND s.revoked_at IS NULL AND s.absolute_expiry > now()
  AND s.last_activity > now() - make_interval(secs => sqlc.arg(idle_seconds)::double precision)
RETURNING u.id,u.email,u.role;

-- name: RevokeSession :exec
UPDATE auth_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :exec
DELETE FROM auth_sessions WHERE absolute_expiry <= now() OR revoked_at IS NOT NULL;
