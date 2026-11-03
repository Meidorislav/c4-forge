package auth

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// AuthorizeRequest authenticates workspace API requests, including origin and
// CSRF checks for mutations. A non-nil transaction acquires the account lifecycle
// lock and rechecks the actor before any workspace row is locked.
func (s *Service) AuthorizeRequest(r *http.Request, tx pgx.Tx) (User, error) {
	write := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
	if write && (r.Header.Get("Origin") != s.origin || strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site")) {
		return User{}, errForbidden
	}
	var user User
	var err error
	if tx == nil {
		user, err = s.authorized(r, false, write)
	} else {
		token := s.sessionToken(r)
		if write && !secureEqual(r.Header.Get(csrfHeader), s.mac("session-csrf", token)) {
			return User{}, errForbidden
		}
		user, err = s.lockedActor(r.Context(), tx, token, false)
	}
	if err != nil {
		return User{}, err
	}
	if user.MustChangePassword {
		return User{}, errForbidden
	}
	return user, nil
}

// WriteAccessError writes an authentication or authorization failure without
// exposing database errors or credentials.
func (s *Service) WriteAccessError(w http.ResponseWriter, err error) { s.adminError(w, err) }
