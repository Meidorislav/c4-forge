package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/db"
	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
	"github.com/Meidorislav/c4-forge/internal/server"
)

const (
	testSecret   = "test-bootstrap-secret-at-least-32-bytes"
	testPassword = "Long correct horse battery staple!"
	testOrigin   = "http://localhost:5173"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dbtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(t.Context(), pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	service, err := New(t.Context(), pool, Options{Origin: testOrigin, BootstrapSecret: testSecret, InsecureDevelopment: true, IdleTimeout: 30 * time.Minute, AbsoluteTimeout: 12 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return service, pool
}

func client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

type response struct{ StatusCode int }

func request(t *testing.T, c *http.Client, method, url, csrf, origin string, body any) (response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set(csrfHeader, csrf)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode}, data
}

func getCSRF(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	res, data := request(t, c, "GET", base+"/csrf", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("csrf: %d %s", res.StatusCode, data)
	}
	var result struct {
		Token string `json:"csrfToken"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Token == "" {
		t.Fatalf("csrf token: %v %s", err, data)
	}
	return result.Token
}

func credentialsBody(email, password, secret string) map[string]string {
	return map[string]string{"email": email, "password": password, "secret": secret}
}

func TestHTTPBootstrapLoginLogoutAndRevocation(t *testing.T) {
	service, pool := testService(t)
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	c := client(t)
	res, data := request(t, c, "GET", server.URL+"/status", "", "", nil)
	if res.StatusCode != 200 || !strings.Contains(string(data), `"initialized":false`) {
		t.Fatalf("status: %d %s", res.StatusCode, data)
	}
	csrf := getCSRF(t, c, server.URL)
	body := credentialsBody("Admin@Example.test", testPassword, testSecret)
	res, _ = request(t, c, "POST", server.URL+"/bootstrap", csrf, "http://evil.test", body)
	if res.StatusCode != 403 {
		t.Fatalf("cross-origin: %d", res.StatusCode)
	}
	res, _ = request(t, c, "POST", server.URL+"/bootstrap", "bad", testOrigin, body)
	if res.StatusCode != 403 {
		t.Fatalf("bad CSRF: %d", res.StatusCode)
	}
	wrong := credentialsBody("Admin@Example.test", testPassword, strings.Repeat("x", 32))
	res, _ = request(t, c, "POST", server.URL+"/bootstrap", csrf, testOrigin, wrong)
	if res.StatusCode != 401 {
		t.Fatalf("bad secret: %d", res.StatusCode)
	}
	res, data = request(t, c, "POST", server.URL+"/bootstrap", csrf, testOrigin, body)
	if res.StatusCode != 201 {
		t.Fatalf("bootstrap: %d %s", res.StatusCode, data)
	}
	res, data = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 200 || !strings.Contains(string(data), `"role":"admin"`) {
		t.Fatalf("me: %d %s", res.StatusCode, data)
	}
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	stolenCookies := c.Jar.Cookies(serverURL)
	var stored string
	if err := pool.QueryRow(t.Context(), "SELECT password_hash FROM users").Scan(&stored); err != nil || !strings.HasPrefix(stored, "$argon2id$") || strings.Contains(stored, testPassword) {
		t.Fatalf("password storage: %v", err)
	}
	sessionCSRF := getCSRF(t, c, server.URL)
	res, _ = request(t, c, "POST", server.URL+"/logout", csrf, testOrigin, nil)
	if res.StatusCode != 403 {
		t.Fatalf("preauth CSRF reused after login: %d", res.StatusCode)
	}
	res, _ = request(t, c, "POST", server.URL+"/logout", sessionCSRF, testOrigin, nil)
	if res.StatusCode != 200 {
		t.Fatalf("logout: %d", res.StatusCode)
	}
	res, _ = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("logged-out me: %d", res.StatusCode)
	}
	replay := client(t)
	replay.Jar.SetCookies(serverURL, stolenCookies)
	res, _ = request(t, replay, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("revoked cookie replay: %d", res.StatusCode)
	}
	var revoked int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM auth_sessions WHERE revoked_at IS NOT NULL").Scan(&revoked); err != nil || revoked != 1 {
		t.Fatalf("revoked=%d err=%v", revoked, err)
	}
	csrf = getCSRF(t, c, server.URL)
	res, data = request(t, c, "POST", server.URL+"/login", csrf, testOrigin, credentialsBody("admin@example.test", testPassword, ""))
	if res.StatusCode != 200 {
		t.Fatalf("login: %d %s", res.StatusCode, data)
	}
	res, _ = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("logged-in me: %d", res.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE users SET disabled=true"); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("disabled session: %d", res.StatusCode)
	}
}

func TestBootstrapRaceCreatesExactlyOneAdmin(t *testing.T) {
	service, pool := testService(t)
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	clients := []*http.Client{client(t), client(t)}
	tokens := []string{getCSRF(t, clients[0], server.URL), getCSRF(t, clients[1], server.URL)}
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() {
			body := credentialsBody([]string{"a@example.test", "b@example.test"}[i], testPassword, testSecret)
			data, _ := json.Marshal(body)
			req, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/bootstrap", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", testOrigin)
			req.Header.Set(csrfHeader, tokens[i])
			res, err := clients[i].Do(req)
			if err != nil {
				codes <- 0
				return
			}
			_ = res.Body.Close()
			codes <- res.StatusCode
		})
	}
	wg.Wait()
	close(codes)
	created, conflict := 0, 0
	for code := range codes {
		switch code {
		case 201:
			created++
		case 409:
			conflict++
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE role='admin'").Scan(&count); err != nil || count != 1 || created != 1 || conflict != 1 {
		t.Fatalf("admins=%d created=%d conflict=%d err=%v", count, created, conflict, err)
	}
}

func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	service, pool := testService(t)
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	c := client(t)
	csrf := getCSRF(t, c, server.URL)
	res, _ := request(t, c, "POST", server.URL+"/bootstrap", csrf, testOrigin, credentialsBody("a@example.test", testPassword, testSecret))
	if res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE auth_sessions SET last_activity=now()-interval '31 minutes'"); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("idle: %d", res.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE auth_sessions SET last_activity=now(),absolute_expiry=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, c, "GET", server.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("absolute: %d", res.StatusCode)
	}
}

func TestPasswordAndLimiter(t *testing.T) {
	if validPassword("123456789012345") {
		t.Fatal("common password accepted")
	}
	if validPassword("too short") || !validPassword(testPassword) {
		t.Fatal("password length policy")
	}
	l := newLimiter()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if !l.allow("127.0.0.1", "someone", now) {
			t.Fatal("premature throttle")
		}
	}
	if l.allow("127.0.0.1", "someone", now) {
		t.Fatal("unlimited attempts")
	}
	if !l.allow("127.0.0.1", "someone", now.Add(16*time.Minute)) {
		t.Fatal("throttle did not expire")
	}
}

func TestSecureCookieFlags(t *testing.T) {
	service, _ := testService(t)
	service.insecure = false
	rec := httptest.NewRecorder()
	service.setCookie(rec, true, "value", 300)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing cookie")
	}
	cookie := cookies[0]
	if !strings.HasPrefix(cookie.Name, "__Host-") || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: %+v", cookie)
	}
}

func TestMountedRoutes(t *testing.T) {
	service, _ := testService(t)
	handler := server.NewHandler(slog.New(slog.DiscardHandler), server.Options{Auth: service.Handler()})
	host := httptest.NewServer(handler)
	defer host.Close()
	c := client(t)
	res, _ := request(t, c, "GET", host.URL+"/healthz", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("health: %d", res.StatusCode)
	}
	res, _ = request(t, c, "GET", host.URL+"/api/auth/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("protected: %d", res.StatusCode)
	}
	csrf := getCSRF(t, c, host.URL+"/api/auth")
	res, data := request(t, c, "POST", host.URL+"/api/auth/bootstrap", csrf, testOrigin, credentialsBody("mounted@example.test", testPassword, testSecret))
	if res.StatusCode != 201 {
		t.Fatalf("bootstrap: %d %s", res.StatusCode, data)
	}
	res, _ = request(t, c, "GET", host.URL+"/api/auth/me", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatalf("authenticated: %d", res.StatusCode)
	}
}
