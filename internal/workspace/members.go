package workspace

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Meidorislav/c4-forge/internal/auth"
	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
)

var (
	errForbidden         = errors.New("forbidden")
	errLastOwner         = errors.New("last_owner")
	errMemberUnavailable = errors.New("member_unavailable")
)

func memberError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		failure(w, 404, "not found")
	case errors.Is(err, errForbidden):
		failure(w, 403, "forbidden")
	case errors.Is(err, errLastOwner):
		failure(w, 409, "last_owner")
	case errors.Is(err, errMemberUnavailable):
		failure(w, 409, "member_unavailable")
	default:
		internalError(w)
	}
}

func workspaceOwner(ctx context.Context, q *sqlc.Queries, id, actor uuid.UUID, lock bool) error {
	if lock {
		if _, err := q.LockWorkspace(ctx, id); err != nil {
			return err
		}
	}
	row, err := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{ID: id, UserID: actor})
	if err != nil {
		return err
	}
	if row.Role != "owner" {
		return errForbidden
	}
	return nil
}

type member struct {
	UserID             uuid.UUID `json:"userId"`
	Email              string    `json:"email"`
	Role               string    `json:"role"`
	Disabled           bool      `json:"disabled"`
	MustChangePassword bool      `json:"mustChangePassword"`
	IsCurrentUser      bool      `json:"isCurrentUser"`
}

func (s *service) members(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "invalid input")
		return
	}
	actor := r.Context().Value(actorKey{}).(auth.User)
	q := sqlc.New(s.pool)
	if err := workspaceOwner(r.Context(), q, id, actor.ID, false); err != nil {
		memberError(w, err)
		return
	}
	rows, err := q.ListWorkspaceMembers(r.Context(), id)
	if err != nil {
		internalError(w)
		return
	}
	result := make([]member, 0, len(rows))
	for _, row := range rows {
		result = append(result, member{row.UserID, row.Email, row.Role, row.Disabled, row.MustChangePassword, row.UserID == actor.ID})
	}
	respond(w, 200, result)
}

type memberInput struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	UserID string `json:"userId"`
}

func validRole(role string) bool { return role == "owner" || role == "editor" || role == "viewer" }

func (s *service) mutateMember(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		failure(w, 400, "invalid input")
		return
	}
	transfer := strings.HasSuffix(r.URL.Path, "/transfer")
	adding := r.Method == http.MethodPost && !transfer
	var body memberInput
	// Decode only the fields accepted by this particular endpoint.
	if r.Method != http.MethodDelete {
		switch {
		case adding:
			var input struct {
				Email string `json:"email"`
				Role  string `json:"role"`
			}
			if !readBody(w, r, &input) {
				failure(w, 400, "invalid input")
				return
			}
			body.Email, body.Role = input.Email, input.Role
		case transfer:
			var input struct {
				UserID string `json:"userId"`
			}
			if !readBody(w, r, &input) {
				failure(w, 400, "invalid input")
				return
			}
			body.UserID = input.UserID
		default:
			var input struct {
				Role string `json:"role"`
			}
			if !readBody(w, r, &input) {
				failure(w, 400, "invalid input")
				return
			}
			body.Role = input.Role
		}
	}
	if adding {
		body.Email = strings.TrimSpace(body.Email)
		address, e := mail.ParseAddress(body.Email)
		if e != nil || len(body.Email) > 254 || address.Address != body.Email || !strings.Contains(body.Email, "@") || (body.Role != "editor" && body.Role != "viewer") {
			failure(w, 400, "invalid input")
			return
		}
	} else if !transfer && r.Method != http.MethodDelete && !validRole(body.Role) {
		failure(w, 400, "invalid input")
		return
	}
	var target uuid.UUID
	if !adding {
		value := r.PathValue("userId")
		if transfer {
			value = body.UserID
		}
		target, err = uuid.Parse(value)
		if err != nil {
			failure(w, 400, "invalid input")
			return
		}
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
	if err := workspaceOwner(ctx, q, id, actor.ID, true); err != nil {
		memberError(w, err)
		return
	}
	if adding {
		account, e := q.FindUserByEmail(ctx, strings.ToLower(body.Email))
		if errors.Is(e, pgx.ErrNoRows) || e == nil && account.Disabled {
			memberError(w, errMemberUnavailable)
			return
		}
		if e != nil {
			internalError(w)
			return
		}
		n, e := q.AddWorkspaceMember(ctx, sqlc.AddWorkspaceMemberParams{WorkspaceID: id, UserID: account.ID, Role: body.Role})
		if e != nil {
			internalError(w)
			return
		}
		if n != 1 {
			memberError(w, errMemberUnavailable)
			return
		}
	} else {
		current, e := q.GetWorkspaceMember(ctx, sqlc.GetWorkspaceMemberParams{WorkspaceID: id, UserID: target})
		if e != nil {
			memberError(w, e)
			return
		}
		if transfer && target == actor.ID {
			failure(w, 400, "invalid input")
			return
		}
		if (transfer || body.Role == "owner") && (current.Disabled || current.MustChangePassword) {
			memberError(w, errMemberUnavailable)
			return
		}
		if transfer {
			err = q.SetWorkspaceMemberRole(ctx, sqlc.SetWorkspaceMemberRoleParams{WorkspaceID: id, UserID: target, Role: "owner"})
			if err == nil {
				err = q.SetWorkspaceMemberRole(ctx, sqlc.SetWorkspaceMemberRoleParams{WorkspaceID: id, UserID: actor.ID, Role: "editor"})
			}
		} else {
			if current.Role == "owner" && !current.Disabled && (r.Method == http.MethodDelete || body.Role != "owner") {
				others, e := q.CountOtherEnabledOwners(ctx, sqlc.CountOtherEnabledOwnersParams{WorkspaceID: id, UserID: target})
				if e != nil {
					internalError(w)
					return
				}
				if others == 0 {
					memberError(w, errLastOwner)
					return
				}
			}
			if r.Method == http.MethodDelete {
				err = q.RemoveWorkspaceMember(ctx, sqlc.RemoveWorkspaceMemberParams{WorkspaceID: id, UserID: target})
			} else {
				err = q.SetWorkspaceMemberRole(ctx, sqlc.SetWorkspaceMemberRoleParams{WorkspaceID: id, UserID: target, Role: body.Role})
			}
		}
		if err != nil {
			internalError(w)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(204)
}
