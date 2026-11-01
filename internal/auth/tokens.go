package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var encoding = base64.RawURLEncoding

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return encoding.EncodeToString(b), nil
}
func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func validToken(token string) bool {
	b, err := encoding.DecodeString(token)
	return err == nil && len(b) == 32 && encoding.EncodeToString(b) == token
}

func (s *Service) mac(label, token string) string {
	h := hmac.New(sha256.New, s.csrfKey[:])
	_, _ = h.Write([]byte(label))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(token))
	return encoding.EncodeToString(h.Sum(nil))
}
func secureEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Service) preauth() (cookie, csrf string, err error) {
	id, err := randomToken()
	if err != nil {
		return "", "", err
	}
	payload := fmt.Sprintf("%s.%d", id, time.Now().Add(5*time.Minute).Unix())
	cookie = payload + "." + s.mac("preauth-cookie", payload)
	return cookie, s.mac("preauth-csrf", payload), nil
}

func (s *Service) validPreauth(cookie, csrf string) bool {
	parts := strings.Split(cookie, ".")
	if len(parts) != 3 || !validToken(parts[0]) {
		return false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expiry || expiry > time.Now().Add(5*time.Minute).Unix()+1 {
		return false
	}
	payload := parts[0] + "." + parts[1]
	return secureEqual(parts[2], s.mac("preauth-cookie", payload)) && secureEqual(csrf, s.mac("preauth-csrf", payload))
}

func (s *Service) cookieName(session bool) string {
	name := "c4forge_preauth"
	if session {
		name = "c4forge_session"
	}
	if !s.insecure {
		return "__Host-" + name
	}
	return name
}

func (s *Service) setCookie(w http.ResponseWriter, session bool, value string, maxAge int) {
	// #nosec G124 -- insecure cookies are restricted to explicit loopback-only development.
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(session), Value: value, Path: "/", HttpOnly: true, Secure: !s.insecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
