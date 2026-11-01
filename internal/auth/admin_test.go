package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type temporaryResult struct {
	ID       string `json:"id"`
	Password string `json:"temporaryPassword"`
}

func TestAdminLifecycle(t *testing.T) {
	s, pool := testService(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	admin := client(t)
	csrf := getCSRF(t, admin, srv.URL)
	res, _ := request(t, admin, "POST", srv.URL+"/bootstrap", csrf, testOrigin, credentialsBody("admin@example.test", testPassword, testSecret))
	if res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	adminCSRF := getCSRF(t, admin, srv.URL)
	res, _ = request(t, admin, "POST", srv.URL+"/users", "bad", testOrigin, map[string]string{"email": "x@example.test", "role": "user"})
	if res.StatusCode != 403 {
		t.Fatalf("missing CSRF: %d", res.StatusCode)
	}
	res, data := request(t, admin, "GET", srv.URL+"/me", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	var adminUser User
	if err := json.Unmarshal(data, &adminUser); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, admin, "POST", srv.URL+"/users/"+adminUser.ID.String()+"/disable", adminCSRF, testOrigin, nil)
	if res.StatusCode != 409 {
		t.Fatalf("last admin disable %d", res.StatusCode)
	}
	res, data = request(t, admin, "POST", srv.URL+"/users", adminCSRF, testOrigin, map[string]string{"email": "user@example.test", "role": "user"})
	if res.StatusCode != 201 {
		t.Fatalf("create %d %s", res.StatusCode, data)
	}
	var temp temporaryResult
	if err := json.Unmarshal(data, &temp); err != nil || temp.ID == "" || temp.Password == "" {
		t.Fatalf("temporary: %v %s", err, data)
	}
	user := client(t)
	csrf = getCSRF(t, user, srv.URL)
	res, _ = request(t, user, "POST", srv.URL+"/login", csrf, testOrigin, credentialsBody("user@example.test", temp.Password, ""))
	if res.StatusCode != 200 {
		t.Fatalf("temporary login %d", res.StatusCode)
	}
	res, data = request(t, user, "GET", srv.URL+"/status", "", "", nil)
	if res.StatusCode != 200 || !strings.Contains(string(data), `"mustChangePassword":true`) {
		t.Fatalf("temporary status: %s", data)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE users SET temporary_password_expires_at=now()-interval '1 second' WHERE id=$1", temp.ID); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, user, "GET", srv.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("expired temporary session: %d", res.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE users SET temporary_password_expires_at=now()+interval '1 hour' WHERE id=$1", temp.ID); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, user, "GET", srv.URL+"/users", "", "", nil)
	if res.StatusCode != 403 {
		t.Fatalf("user admin access %d", res.StatusCode)
	}
	csrf = getCSRF(t, user, srv.URL)
	res, _ = request(t, user, "POST", srv.URL+"/change-password", csrf, testOrigin, map[string]string{"password": "A brand new long passphrase!"})
	if res.StatusCode != 200 {
		t.Fatalf("change %d", res.StatusCode)
	}
	res, data = request(t, user, "GET", srv.URL+"/status", "", "", nil)
	if !strings.Contains(string(data), `"mustChangePassword":false`) {
		t.Fatalf("permanent status: %s", data)
	}
	res, _ = request(t, user, "GET", srv.URL+"/users", "", "", nil)
	if res.StatusCode != 403 {
		t.Fatalf("permanent user admin access %d", res.StatusCode)
	}
	res, data = request(t, admin, "POST", srv.URL+"/users/"+temp.ID+"/reset", adminCSRF, testOrigin, nil)
	if res.StatusCode != 200 {
		t.Fatalf("reset %d %s", res.StatusCode, data)
	}
	var reset temporaryResult
	if err := json.Unmarshal(data, &reset); err != nil || reset.Password == "" {
		t.Fatal(err)
	}
	res, _ = request(t, user, "GET", srv.URL+"/me", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("reset did not revoke %d", res.StatusCode)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE users SET temporary_password_expires_at=now()-interval '1 second' WHERE id=$1", temp.ID); err != nil {
		t.Fatal(err)
	}
	csrf = getCSRF(t, user, srv.URL)
	res, _ = request(t, user, "POST", srv.URL+"/login", csrf, testOrigin, credentialsBody("user@example.test", reset.Password, ""))
	if res.StatusCode != 401 {
		t.Fatalf("expired credential %d", res.StatusCode)
	}
	res, _ = request(t, admin, "POST", srv.URL+"/users/"+temp.ID+"/disable", adminCSRF, testOrigin, nil)
	if res.StatusCode != 200 {
		t.Fatalf("disable %d", res.StatusCode)
	}
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE id=$1 AND disabled", temp.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("identity retained: %d %v", count, err)
	}
	unauth := client(t)
	res, _ = request(t, unauth, "GET", srv.URL+"/users", "", "", nil)
	if res.StatusCode != 401 {
		t.Fatalf("anonymous %d", res.StatusCode)
	}
}

func TestConcurrentLastAdminProtection(t *testing.T) {
	s, pool := testService(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	a := client(t)
	csrf := getCSRF(t, a, srv.URL)
	res, _ := request(t, a, "POST", srv.URL+"/bootstrap", csrf, testOrigin, credentialsBody("a@example.test", testPassword, testSecret))
	if res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	aCSRF := getCSRF(t, a, srv.URL)
	res, data := request(t, a, "GET", srv.URL+"/me", "", "", nil)
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	var aUser User
	if err := json.Unmarshal(data, &aUser); err != nil {
		t.Fatal(err)
	}
	res, data = request(t, a, "POST", srv.URL+"/users", aCSRF, testOrigin, map[string]string{"email": "b@example.test", "role": "admin"})
	if res.StatusCode != 201 {
		t.Fatalf("create %d %s", res.StatusCode, data)
	}
	var bInfo temporaryResult
	if err := json.Unmarshal(data, &bInfo); err != nil {
		t.Fatal(err)
	}
	res, _ = request(t, a, "POST", srv.URL+"/users/"+aUser.ID.String()+"/disable", aCSRF, testOrigin, nil)
	if res.StatusCode != 409 {
		t.Fatalf("pending admin must not replace the last usable admin: %d", res.StatusCode)
	}
	b := client(t)
	csrf = getCSRF(t, b, srv.URL)
	res, _ = request(t, b, "POST", srv.URL+"/login", csrf, testOrigin, credentialsBody("b@example.test", bInfo.Password, ""))
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	csrf = getCSRF(t, b, srv.URL)
	res, _ = request(t, b, "POST", srv.URL+"/change-password", csrf, testOrigin, map[string]string{"password": "Another very long password!"})
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	bCSRF := getCSRF(t, b, srv.URL)
	var wg sync.WaitGroup
	wg.Add(2)
	codes := make(chan int, 2)
	for _, item := range []struct {
		c         *http.Client
		id, token string
	}{{a, bInfo.ID, aCSRF}, {b, aUser.ID.String(), bCSRF}} {
		go func() {
			defer wg.Done()
			r, _ := request(t, item.c, "POST", srv.URL+"/users/"+item.id+"/disable", item.token, testOrigin, nil)
			codes <- r.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 401 && code != 409 {
			t.Fatalf("unexpected %d", code)
		}
	}
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE role='admin' AND NOT disabled").Scan(&count); err != nil || count != 1 || success != 1 {
		t.Fatalf("count %d success %d err %v", count, success, err)
	}
}
