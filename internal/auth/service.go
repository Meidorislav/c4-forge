// Package auth implements local authentication and server-side sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
)

var (
	ErrCredentials      = errors.New("invalid credentials")
	ErrSetupUnavailable = errors.New("setup unavailable")
	ErrInvalidInput     = errors.New("invalid input")
)

// Options contains server-controlled authentication settings. BootstrapSecret
// must never be logged or returned to the browser.
type Options struct {
	Origin              string
	BootstrapSecret     string
	InsecureDevelopment bool
	IdleTimeout         time.Duration
	AbsoluteTimeout     time.Duration
}

// Service uses a migrated database and never owns its pool.
type Service struct {
	pool             *pgxpool.Pool
	origin           string
	bootstrapHash    [32]byte
	bootstrapEnabled bool
	insecure         bool
	idle             time.Duration
	absolute         time.Duration
	csrfKey          [32]byte
	dummyHash        string
	hashSlots        chan struct{}
	limiter          *limiter
}

// New creates an authentication service. Origin and development mode must have
// already been validated against the listener by the configuration loader.
func New(ctx context.Context, pool *pgxpool.Pool, opts Options) (*Service, error) {
	if pool == nil || opts.Origin == "" || opts.IdleTimeout <= 0 || opts.AbsoluteTimeout <= 0 || opts.IdleTimeout > opts.AbsoluteTimeout {
		return nil, errors.New("invalid authentication settings")
	}
	if opts.BootstrapSecret != "" && len(opts.BootstrapSecret) < 32 {
		return nil, errors.New("bootstrap secret must contain at least 32 bytes")
	}
	s := &Service{pool: pool, origin: opts.Origin, insecure: opts.InsecureDevelopment, idle: opts.IdleTimeout, absolute: opts.AbsoluteTimeout, hashSlots: make(chan struct{}, 2), limiter: newLimiter()}
	if opts.BootstrapSecret != "" {
		s.bootstrapEnabled = true
		s.bootstrapHash = sha256.Sum256([]byte(opts.BootstrapSecret))
	}
	if _, err := rand.Read(s.csrfKey[:]); err != nil {
		return nil, fmt.Errorf("generate CSRF key: %w", err)
	}
	var err error
	s.dummyHash, err = s.hashPassword(ctx, "dummy password for comparison only")
	if err != nil {
		return nil, fmt.Errorf("initialize password verifier: %w", err)
	}
	return s, nil
}

// User identifies an authenticated account without returning credentials.
type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}

func normalizeEmail(value string) (display, canonical string, err error) {
	display = strings.TrimSpace(value)
	if len(display) > 254 || !utf8.ValidString(display) {
		return "", "", ErrInvalidInput
	}
	address, e := mail.ParseAddress(display)
	if e != nil || address.Address != display || !strings.Contains(display, "@") {
		return "", "", ErrInvalidInput
	}
	return display, strings.ToLower(display), nil
}

func (s *Service) initialized(ctx context.Context) (bool, error) {
	return sqlc.New(s.pool).AuthInitialized(ctx)
}

func (s *Service) bootstrap(ctx context.Context, email, password, secret string) (string, User, error) {
	if !s.bootstrapEnabled {
		return "", User{}, ErrSetupUnavailable
	}
	supplied := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(supplied[:], s.bootstrapHash[:]) != 1 {
		return "", User{}, ErrCredentials
	}
	display, canonical, err := normalizeEmail(email)
	if err != nil || !validPassword(password) {
		return "", User{}, ErrInvalidInput
	}
	hash, err := s.hashPassword(ctx, password)
	if err != nil {
		return "", User{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", User{}, err
	}
	token, err := randomToken()
	if err != nil {
		return "", User{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", User{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := sqlc.New(tx)
	initialized, err := q.LockAuthState(ctx)
	if err != nil {
		return "", User{}, err
	}
	if initialized {
		return "", User{}, ErrSetupUnavailable
	}
	if err = q.CreateUser(ctx, sqlc.CreateUserParams{ID: id, Email: display, EmailCanonical: canonical, PasswordHash: hash, Role: "admin"}); err != nil {
		return "", User{}, err
	}
	if err = q.SetAuthInitialized(ctx); err != nil {
		return "", User{}, err
	}
	if err = q.CreateSession(ctx, sqlc.CreateSessionParams{TokenHash: tokenHash(token), UserID: id, AbsoluteExpiry: pgtype.Timestamptz{Time: time.Now().Add(s.absolute), Valid: true}}); err != nil {
		return "", User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", User{}, err
	}
	return token, User{ID: id, Email: display, Role: "admin"}, nil
}

func (s *Service) login(ctx context.Context, email, password string) (string, User, error) {
	_, canonical, err := normalizeEmail(email)
	if err != nil {
		canonical = "invalid"
	}
	if len(password) > passwordMaxBytes || !utf8.ValidString(password) {
		return "", User{}, ErrCredentials
	}
	q := sqlc.New(s.pool)
	account, err := q.FindUserByEmail(ctx, canonical)
	missing := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !missing {
		return "", User{}, err
	}
	hash := account.PasswordHash
	if missing {
		hash = s.dummyHash
	}
	matches, upgrade, err := s.verifyPassword(ctx, password, hash)
	if err != nil {
		return "", User{}, err
	}
	if missing || account.Disabled || !matches {
		return "", User{}, ErrCredentials
	}
	if upgrade {
		updated, err := s.hashPassword(ctx, password)
		if err != nil {
			return "", User{}, err
		}
		if err = q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{ID: account.ID, PasswordHash: updated, PasswordHash_2: hash}); err != nil {
			return "", User{}, err
		}
	}
	token, err := randomToken()
	if err != nil {
		return "", User{}, err
	}
	if err = q.CreateSession(ctx, sqlc.CreateSessionParams{TokenHash: tokenHash(token), UserID: account.ID, AbsoluteExpiry: pgtype.Timestamptz{Time: time.Now().Add(s.absolute), Valid: true}}); err != nil {
		return "", User{}, err
	}
	return token, User{ID: account.ID, Email: account.Email, Role: account.Role}, nil
}

func (s *Service) resolve(ctx context.Context, token string) (User, error) {
	if !validToken(token) {
		return User{}, ErrCredentials
	}
	row, err := sqlc.New(s.pool).ResolveSession(ctx, sqlc.ResolveSessionParams{TokenHash: tokenHash(token), IdleSeconds: s.idle.Seconds()})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrCredentials
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role}, nil
}
