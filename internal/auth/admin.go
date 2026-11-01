package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
)

var (
	errForbidden = errors.New("forbidden")
	errConflict  = errors.New("conflict")
)

// lockedActor serializes account changes and checks the session within the same transaction.
func (s *Service) lockedActor(ctx context.Context, tx pgx.Tx, token string, admin bool) (User, error) {
	q := sqlc.New(tx)
	if _, err := q.LockAuthState(ctx); err != nil {
		return User{}, err
	}
	if !validToken(token) {
		return User{}, ErrCredentials
	}
	row, err := q.ResolveSession(ctx, sqlc.ResolveSessionParams{TokenHash: tokenHash(token), IdleSeconds: s.idle.Seconds()})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrCredentials
	}
	if err != nil {
		return User{}, err
	}
	if admin && (row.Role != "admin" || row.MustChangePassword) {
		return User{}, errForbidden
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, MustChangePassword: row.MustChangePassword}, nil
}

func (s *Service) adminError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, errForbidden):
		respond(w, 403, map[string]string{"error": "forbidden"})
	case errors.Is(err, errConflict), errors.Is(err, pgx.ErrNoRows), errors.As(err, &pgErr) && pgErr.Code == "23505":
		respond(w, 409, map[string]string{"error": "conflict"})
	default:
		s.authError(w, err)
	}
}

func (s *Service) authorized(r *http.Request, admin, csrf bool) (User, error) {
	token := s.sessionToken(r)
	user, err := s.resolve(r.Context(), token)
	if err != nil {
		return User{}, err
	}
	if admin && (user.Role != "admin" || user.MustChangePassword) {
		return User{}, errForbidden
	}
	if csrf && !secureEqual(r.Header.Get(csrfHeader), s.mac("session-csrf", token)) {
		return User{}, errForbidden
	}
	return user, nil
}

func (s *Service) usersHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authorized(r, true, false); err != nil {
		s.adminError(w, err)
		return
	}
	rows, err := sqlc.New(s.pool).ListUsers(r.Context())
	if err != nil {
		s.adminError(w, err)
		return
	}
	users := make([]map[string]any, 0, len(rows))
	for _, u := range rows {
		users = append(users, map[string]any{"id": u.ID, "email": u.Email, "role": u.Role, "disabled": u.Disabled, "mustChangePassword": u.MustChangePassword})
	}
	respond(w, 200, users)
}

func (s *Service) createUserHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authorized(r, true, true); err != nil {
		s.adminError(w, err)
		return
	}
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	display, canonical, err := normalizeEmail(body.Email)
	if err != nil || (body.Role != "admin" && body.Role != "user") {
		s.adminError(w, ErrInvalidInput)
		return
	}
	secret, err := randomToken()
	if err != nil {
		s.adminError(w, err)
		return
	}
	hash, err := s.hashPassword(r.Context(), secret)
	if err != nil {
		s.adminError(w, err)
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		s.adminError(w, err)
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.adminError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = s.lockedActor(ctx, tx, s.sessionToken(r), true); err != nil {
		s.adminError(w, err)
		return
	}
	expiry := time.Now().Add(24 * time.Hour)
	err = sqlc.New(tx).CreateTemporaryUser(ctx, sqlc.CreateTemporaryUserParams{ID: id, Email: display, EmailCanonical: canonical, PasswordHash: hash, Role: body.Role, TemporaryPasswordExpiresAt: pgtype.Timestamptz{Time: expiry, Valid: true}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		s.adminError(w, err)
		return
	}
	respond(w, 201, map[string]any{"id": id, "temporaryPassword": secret, "expiresAt": expiry})
}

func (s *Service) disableUserHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authorized(r, true, true); err != nil {
		s.adminError(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.adminError(w, ErrInvalidInput)
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.adminError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = s.lockedActor(ctx, tx, s.sessionToken(r), true); err != nil {
		s.adminError(w, err)
		return
	}
	q := sqlc.New(tx)
	target, err := q.LockUser(ctx, id)
	if err != nil {
		s.adminError(w, err)
		return
	}
	if target.Disabled {
		s.adminError(w, errConflict)
		return
	}
	if target.Role == "admin" && !target.MustChangePassword {
		count, e := q.CountActiveAdmins(ctx)
		if e != nil {
			s.adminError(w, e)
			return
		}
		if count <= 1 {
			s.adminError(w, errConflict)
			return
		}
	}
	_, err = q.DisableUser(ctx, id)
	if err == nil {
		err = q.RevokeUserSessions(ctx, id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		s.adminError(w, err)
		return
	}
	respond(w, 200, map[string]string{"status": "disabled"})
}

func (s *Service) resetUserHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authorized(r, true, true); err != nil {
		s.adminError(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.adminError(w, ErrInvalidInput)
		return
	}
	secret, err := randomToken()
	if err != nil {
		s.adminError(w, err)
		return
	}
	hash, err := s.hashPassword(r.Context(), secret)
	if err != nil {
		s.adminError(w, err)
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.adminError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	actor, err := s.lockedActor(ctx, tx, s.sessionToken(r), true)
	if err != nil {
		s.adminError(w, err)
		return
	}
	if actor.ID == id {
		s.adminError(w, errConflict)
		return
	}
	q := sqlc.New(tx)
	expiry := time.Now().Add(24 * time.Hour)
	n, err := q.ResetUserPassword(ctx, sqlc.ResetUserPasswordParams{ID: id, PasswordHash: hash, TemporaryPasswordExpiresAt: pgtype.Timestamptz{Time: expiry, Valid: true}})
	if err == nil && n != 1 {
		err = errConflict
	}
	if err == nil {
		err = q.RevokeUserSessions(ctx, id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		s.adminError(w, err)
		return
	}
	respond(w, 200, map[string]any{"temporaryPassword": secret, "expiresAt": expiry})
}

func (s *Service) changePasswordHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authorized(r, false, true); err != nil {
		s.adminError(w, err)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !validPassword(body.Password) {
		s.adminError(w, ErrInvalidInput)
		return
	}
	hash, err := s.hashPassword(r.Context(), body.Password)
	if err != nil {
		s.adminError(w, err)
		return
	}
	token, err := randomToken()
	if err != nil {
		s.adminError(w, err)
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.adminError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	actor, err := s.lockedActor(ctx, tx, s.sessionToken(r), false)
	if err != nil {
		s.adminError(w, err)
		return
	}
	if !actor.MustChangePassword {
		s.adminError(w, errConflict)
		return
	}
	q := sqlc.New(tx)
	err = q.SetPermanentPassword(ctx, sqlc.SetPermanentPasswordParams{ID: actor.ID, PasswordHash: hash})
	if err == nil {
		err = q.RevokeUserSessions(ctx, actor.ID)
	}
	if err == nil {
		err = q.CreateSession(ctx, sqlc.CreateSessionParams{TokenHash: tokenHash(token), UserID: actor.ID, AbsoluteExpiry: pgtype.Timestamptz{Time: time.Now().Add(s.absolute), Valid: true}})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		s.adminError(w, err)
		return
	}
	s.setCookie(w, true, token, int(s.absolute.Seconds()))
	respond(w, 200, map[string]string{"status": "password changed"})
}
