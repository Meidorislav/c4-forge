package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
)

const csrfHeader = "X-CSRF-Token"

// Handler exposes only the local authentication API (ADR-0004).
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /csrf", s.csrf)
	mux.HandleFunc("GET /me", s.me)
	mux.HandleFunc("POST /bootstrap", s.bootstrapHTTP)
	mux.HandleFunc("POST /login", s.loginHTTP)
	mux.HandleFunc("POST /logout", s.logoutHTTP)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost && (r.Header.Get("Origin") != s.origin || strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site")) {
			respond(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Service) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(s.cookieName(true))
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Service) requestUser(r *http.Request) (User, error) {
	return s.resolve(r.Context(), s.sessionToken(r))
}

func (s *Service) status(w http.ResponseWriter, r *http.Request) {
	initialized, err := s.initialized(r.Context())
	if err != nil {
		respond(w, 500, map[string]string{"error": "internal error"})
		return
	}
	user, err := s.requestUser(r)
	if err != nil && !errors.Is(err, ErrCredentials) {
		respond(w, 500, map[string]string{"error": "internal error"})
		return
	}
	result := map[string]any{"initialized": initialized, "authenticated": err == nil}
	if err == nil {
		result["email"] = user.Email
		result["role"] = user.Role
	}
	respond(w, 200, result)
}

func (s *Service) csrf(w http.ResponseWriter, r *http.Request) {
	token := s.sessionToken(r)
	_, err := s.resolve(r.Context(), token)
	if err == nil {
		respond(w, 200, map[string]string{"csrfToken": s.mac("session-csrf", token)})
		return
	}
	if !errors.Is(err, ErrCredentials) {
		respond(w, 500, map[string]string{"error": "internal error"})
		return
	}
	cookie, csrf, err := s.preauth()
	if err != nil {
		respond(w, 500, map[string]string{"error": "internal error"})
		return
	}
	s.setCookie(w, false, cookie, 300)
	respond(w, 200, map[string]string{"csrfToken": csrf})
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.requestUser(r)
	if err != nil {
		if errors.Is(err, ErrCredentials) {
			respond(w, 401, map[string]string{"error": "unauthorized"})
		} else {
			respond(w, 500, map[string]string{"error": "internal error"})
		}
		return
	}
	respond(w, 200, user)
}

func (s *Service) preauthValid(r *http.Request) bool {
	cookie, err := r.Cookie(s.cookieName(false))
	return err == nil && s.validPreauth(cookie.Value, r.Header.Get(csrfHeader))
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		respond(w, 400, map[string]string{"error": "invalid input"})
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || !utf8.Valid(body) {
		respond(w, 400, map[string]string{"error": "invalid input"})
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		respond(w, 400, map[string]string{"error": "invalid input"})
		return false
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		respond(w, 400, map[string]string{"error": "invalid input"})
		return false
	}
	return true
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Secret   string `json:"secret,omitempty"`
}

func (s *Service) bootstrapHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.preauthValid(r) {
		respond(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var body credentials
	if !readJSON(w, r, &body) {
		return
	}
	if !s.limiter.allow(clientIP(r), "bootstrap:"+strings.ToLower(strings.TrimSpace(body.Email)), time.Now()) {
		respond(w, 429, map[string]string{"error": "too many attempts"})
		return
	}
	token, user, err := s.bootstrap(r.Context(), body.Email, body.Password, body.Secret)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, false, "", -1)
	s.setCookie(w, true, token, int(s.absolute.Seconds()))
	respond(w, 201, user)
}

func (s *Service) loginHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.preauthValid(r) {
		respond(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var body credentials
	if !readJSON(w, r, &body) {
		return
	}
	if !s.limiter.allow(clientIP(r), "login:"+strings.ToLower(strings.TrimSpace(body.Email)), time.Now()) {
		respond(w, 429, map[string]string{"error": "too many attempts"})
		return
	}
	token, user, err := s.login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, false, "", -1)
	s.setCookie(w, true, token, int(s.absolute.Seconds()))
	respond(w, 200, user)
}

func (s *Service) logoutHTTP(w http.ResponseWriter, r *http.Request) {
	token := s.sessionToken(r)
	if _, err := s.resolve(r.Context(), token); err != nil {
		s.authError(w, err)
		return
	}
	if !secureEqual(r.Header.Get(csrfHeader), s.mac("session-csrf", token)) {
		respond(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	if err := s.revoke(r, token); err != nil {
		respond(w, 500, map[string]string{"error": "internal error"})
		return
	}
	s.setCookie(w, true, "", -1)
	respond(w, 200, map[string]string{"status": "signed out"})
}

func (s *Service) revoke(r *http.Request, token string) error {
	return sqlc.New(s.pool).RevokeSession(r.Context(), tokenHash(token))
}

func (s *Service) authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCredentials):
		respond(w, 401, map[string]string{"error": "invalid credentials"})
	case errors.Is(err, ErrInvalidInput):
		respond(w, 400, map[string]string{"error": "invalid input"})
	case errors.Is(err, ErrSetupUnavailable):
		respond(w, 409, map[string]string{"error": "setup unavailable"})
	default:
		respond(w, 500, map[string]string{"error": "internal error"})
	}
}
