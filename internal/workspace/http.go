// Package workspace implements membership-scoped workspace metadata management.
package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/auth"
	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
)

type service struct {
	pool *pgxpool.Pool
	auth *auth.Service
}

type actorKey struct{}

type workspace struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Handler serves /api/workspaces and its children, with the prefix stripped.
func Handler(pool *pgxpool.Pool, authentication *auth.Service) http.Handler {
	s := &service{pool: pool, auth: authentication}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.list)
	mux.HandleFunc("POST /{$}", s.mutate)
	mux.HandleFunc("GET /{id}", s.get)
	mux.HandleFunc("PUT /{id}", s.mutate)
	mux.HandleFunc("DELETE /{id}", s.mutate)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, err := s.auth.AuthorizeRequest(r, nil)
		if err != nil {
			s.auth.WriteAccessError(w, err)
			return
		}
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
	})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func internalError(w http.ResponseWriter) { failure(w, 500, "internal error") }

func (s *service) list(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(actorKey{}).(auth.User)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "modified_desc"
	}
	if !utf8.ValidString(search) || strings.ContainsRune(search, 0) || utf8.RuneCountInString(search) > 200 {
		failure(w, 400, "invalid input")
		return
	}
	switch sort {
	case "modified_desc", "modified_asc", "name_asc", "name_desc":
	default:
		failure(w, 400, "invalid input")
		return
	}
	rows, err := sqlc.New(s.pool).ListWorkspaces(r.Context(), sqlc.ListWorkspacesParams{UserID: actor.ID, Search: search, Sort: sort})
	if err != nil {
		internalError(w)
		return
	}
	result := make([]workspace, 0, len(rows))
	for _, row := range rows {
		result = append(result, workspace{row.ID, row.Name, row.Description, row.Role, row.CreatedAt.Time, row.UpdatedAt.Time})
	}
	respond(w, 200, result)
}

func (s *service) get(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(actorKey{}).(auth.User)
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "invalid input")
		return
	}
	row, err := sqlc.New(s.pool).GetWorkspace(r.Context(), sqlc.GetWorkspaceParams{ID: id, UserID: actor.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, 404, "not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	respond(w, 200, workspace{row.ID, row.Name, row.Description, row.Role, row.CreatedAt.Time, row.UpdatedAt.Time})
}

type metadata struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func readMetadata(w http.ResponseWriter, r *http.Request) (metadata, bool) {
	var body metadata
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return body, false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
	if err != nil || !utf8.Valid(data) {
		return body, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) || body.Name == nil || body.Description == nil {
		return body, false
	}
	name := strings.TrimSpace(*body.Name)
	body.Name = &name
	if name == "" || utf8.RuneCountInString(name) > 200 || utf8.RuneCountInString(*body.Description) > 4000 {
		return body, false
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return body, false
		}
	}
	for _, c := range *body.Description {
		if unicode.IsControl(c) && c != '\n' && c != '\r' && c != '\t' {
			return body, false
		}
	}
	return body, true
}

func (s *service) mutate(w http.ResponseWriter, r *http.Request) {
	var body metadata
	if r.Method != http.MethodDelete {
		var ok bool
		body, ok = readMetadata(w, r)
		if !ok {
			failure(w, 400, "invalid input")
			return
		}
	}
	var id uuid.UUID
	var err error
	if r.Method == http.MethodPost {
		id, err = uuid.NewV7()
	} else {
		id, err = uuid.Parse(r.PathValue("id"))
	}
	if err != nil {
		failure(w, 400, "invalid input")
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	actor, err := s.auth.AuthorizeRequest(r, tx)
	if err != nil {
		s.auth.WriteAccessError(w, err)
		return
	}
	q := sqlc.New(tx)
	if r.Method != http.MethodPost {
		// Same row lock as model writes; the lifecycle lock is already held.
		if _, err = q.LockWorkspace(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			failure(w, 404, "not found")
			return
		}
		if err != nil {
			internalError(w)
			return
		}
		row, e := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{ID: id, UserID: actor.ID})
		if errors.Is(e, pgx.ErrNoRows) {
			failure(w, 404, "not found")
			return
		}
		if e != nil {
			internalError(w)
			return
		}
		if row.Role != "owner" {
			failure(w, 403, "forbidden")
			return
		}
	}
	switch r.Method {
	case http.MethodPost:
		err = q.CreateWorkspaceMetadata(ctx, sqlc.CreateWorkspaceMetadataParams{ID: id, Name: pgtype.Text{String: *body.Name, Valid: true}, Description: *body.Description, CreatedBy: pgtype.UUID{Bytes: actor.ID, Valid: true}})
		if err == nil {
			err = q.CreateWorkspaceOwner(ctx, sqlc.CreateWorkspaceOwnerParams{WorkspaceID: id, UserID: actor.ID})
		}
	case http.MethodPut:
		err = q.UpdateWorkspaceMetadata(ctx, sqlc.UpdateWorkspaceMetadataParams{ID: id, Name: pgtype.Text{String: *body.Name, Valid: true}, Description: *body.Description})
	case http.MethodDelete:
		err = q.DeleteWorkspace(ctx, id)
	}
	if err != nil {
		internalError(w)
		return
	}
	var result workspace
	if r.Method != http.MethodDelete {
		row, e := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{ID: id, UserID: actor.ID})
		if e != nil {
			internalError(w)
			return
		}
		result = workspace{row.ID, row.Name, row.Description, row.Role, row.CreatedAt.Time, row.UpdatedAt.Time}
	}
	if err = tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(204)
		return
	}
	status := 200
	if r.Method == http.MethodPost {
		status = 201
		w.Header().Set("Location", "/api/workspaces/"+id.String())
	}
	respond(w, status, result)
}
