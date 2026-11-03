package workspace

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/auth"
	"github.com/Meidorislav/c4-forge/internal/db"
	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
	"github.com/Meidorislav/c4-forge/internal/server"
)

const origin = "http://localhost:5173"

type fixture struct {
	pool  *pgxpool.Pool
	url   string
	admin account
}
type account struct {
	id          uuid.UUID
	token, csrf string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dbtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	log := slog.New(slog.DiscardHandler)
	if err := db.Migrate(t.Context(), pool, log); err != nil {
		t.Fatal(err)
	}
	authentication, err := auth.New(t.Context(), pool, auth.Options{Origin: origin, InsecureDevelopment: true, IdleTimeout: time.Hour, AbsoluteTimeout: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.NewHandler(log, server.Options{Auth: authentication.Handler(), Workspaces: Handler(pool, authentication)}))
	t.Cleanup(srv.Close)
	f := &fixture{pool: pool, url: srv.URL}
	f.admin = f.user(t, "admin")
	return f
}

func (f *fixture) user(t *testing.T, role string) account {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO users(id,email,email_canonical,password_hash,role) VALUES($1,$2,$2,'unused',$3)`, id, id.String()+"@example.test", role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO auth_sessions(token_hash,user_id,absolute_expiry) VALUES($1,$2,now()+interval '2 hours')`, hash[:], id); err != nil {
		t.Fatal(err)
	}
	a := account{id: id, token: token}
	_, data := f.call(t, a, "GET", "/api/auth/csrf", "", 200)
	var result struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	a.csrf = result.CSRF
	return a
}

func (f *fixture) call(t *testing.T, a account, method, path, body string, want int) (http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, f.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("X-CSRF-Token", a.csrf)
	if a.token != "" {
		req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: a.token})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Errorf("%s %s: got %d want %d: %s", method, path, res.StatusCode, want, data)
	}
	return res.Header, data
}

func (f *fixture) create(t *testing.T, a account, name, description string) workspace {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "description": description})
	headers, data := f.call(t, a, "POST", "/api/workspaces", string(body), 201)
	var w workspace
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if w.Role != "owner" || w.ID == uuid.Nil || headers.Get("Location") != "/api/workspaces/"+w.ID.String() {
		t.Fatalf("bad creation: %s", data)
	}
	return w
}

func decodeList(t *testing.T, data []byte) []workspace {
	t.Helper()
	var items []workspace
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func exec(t *testing.T, f *fixture, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceLifecycleAndIsolation(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	outsider := f.admin
	w := f.create(t, owner, "  Payments  ", "Card processing")
	if w.Name != "Payments" {
		t.Fatal(w.Name)
	}
	path := "/api/workspaces/" + w.ID.String()
	for _, a := range []account{{}, outsider} {
		status := 404
		if a.token == "" {
			status = 401
		}
		f.call(t, a, "GET", path, "", status)
		f.call(t, a, "PUT", path, `{"name":"No","description":""}`, status)
		f.call(t, a, "DELETE", path, "", status)
	}
	_, data := f.call(t, outsider, "GET", "/api/workspaces?q=Payments", "", 200)
	if string(data) != "[]\n" {
		t.Fatal(string(data))
	}
	f.call(t, owner, "PUT", path, `{"name":"Billing","description":"New description"}`, 200)
	_, data = f.call(t, owner, "GET", path, "", 200)
	var updated workspace
	_ = json.Unmarshal(data, &updated)
	if updated.Name != "Billing" || !updated.UpdatedAt.After(w.UpdatedAt) {
		t.Fatal(string(data))
	}
	// Persistent model batches use the same database timestamp trigger.
	exec(t, f, `UPDATE workspaces SET seq=seq+1 WHERE id=$1`, w.ID)
	_, data = f.call(t, owner, "GET", path, "", 200)
	var modelChanged workspace
	_ = json.Unmarshal(data, &modelChanged)
	if !modelChanged.UpdatedAt.After(updated.UpdatedAt) {
		t.Fatal("model write did not update timestamp")
	}
	_, data = f.call(t, owner, "GET", path, "", 200)
	var read workspace
	_ = json.Unmarshal(data, &read)
	if !read.UpdatedAt.Equal(modelChanged.UpdatedAt) {
		t.Fatal("read changed timestamp")
	}
	exec(t, f, `INSERT INTO elements(workspace_id,id,type,name,dsl_id) VALUES($1,$2,'person','User','user')`, w.ID, uuid.New())
	exec(t, f, `INSERT INTO operations(workspace_id,seq,batch_id,actor_id,ops) VALUES($1,1,$2,$3,'[]')`, w.ID, uuid.New(), owner.id)
	f.call(t, owner, "DELETE", path, "", 204)
	for _, table := range []string{"workspaces", "workspace_members", "elements", "operations"} {
		var count int
		column := "workspace_id"
		if table == "workspaces" {
			column = "id"
		}
		if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE "+column+"=$1", w.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	f.call(t, owner, "GET", path, "", 404)
}

func TestWorkspaceSearchSortAndRoles(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	member := f.user(t, "user")
	a := f.create(t, owner, "Alpha", "literal 100%_match")
	b := f.create(t, owner, "Beta", "Payments")
	for _, tc := range []struct {
		query string
		ids   []uuid.UUID
	}{
		{"", []uuid.UUID{b.ID, a.ID}},
		{"?sort=modified_asc", []uuid.UUID{a.ID, b.ID}},
		{"?sort=name_asc", []uuid.UUID{a.ID, b.ID}},
		{"?sort=name_desc", []uuid.UUID{b.ID, a.ID}},
		{"?q=pAyMeNtS", []uuid.UUID{b.ID}},
		{"?q=100%25_match", []uuid.UUID{a.ID}},
		{"?q=absent", nil},
	} {
		_, data := f.call(t, owner, "GET", "/api/workspaces"+tc.query, "", 200)
		rows := decodeList(t, data)
		if len(rows) != len(tc.ids) {
			t.Fatalf("%s: %s", tc.query, data)
		}
		for i := range rows {
			if rows[i].ID != tc.ids[i] {
				t.Fatal(string(data))
			}
		}
	}
	path := "/api/workspaces/" + a.ID.String()
	exec(t, f, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'viewer')`, a.ID, member.id)
	for _, role := range []string{"viewer", "editor"} {
		exec(t, f, `UPDATE workspace_members SET role=$3 WHERE workspace_id=$1 AND user_id=$2`, a.ID, member.id, role)
		f.call(t, member, "GET", path, "", 200)
		f.call(t, member, "PUT", path, `{"name":"No","description":""}`, 403)
		f.call(t, member, "DELETE", path, "", 403)
		_, data := f.call(t, member, "GET", "/api/workspaces", "", 200)
		if rows := decodeList(t, data); len(rows) != 1 || rows[0].ID != a.ID {
			t.Fatal(string(data))
		}
	}
	// Stable ordering for equal sort values.
	exec(t, f, `UPDATE workspaces SET updated_at='2026-01-01' WHERE id IN ($1,$2)`, a.ID, b.ID)
	_, data := f.call(t, owner, "GET", "/api/workspaces", "", 200)
	rows := decodeList(t, data)
	if strings.Compare(rows[0].ID.String(), rows[1].ID.String()) >= 0 {
		t.Fatal("unstable tie order")
	}
}

func TestWorkspaceValidationAndSessionGuards(t *testing.T) {
	f := setup(t)
	a := f.user(t, "user")
	for _, body := range []string{`{}`, `null`, `{"name":"x"}`, `{"name":null,"description":""}`, `{"name":" ","description":""}`, `{"name":"x","description":"","extra":true}`, `{"name":"x","description":""} {}`, `{"name":"x\u0000","description":""}`, `{"name":"` + strings.Repeat("a", 201) + `","description":""}`, `{"name":"a","description":"` + strings.Repeat("b", 4001) + `"}`, string([]byte{'{', 0xff, '}'})} {
		f.call(t, a, "POST", "/api/workspaces", body, 400)
	}
	f.call(t, a, "GET", "/api/workspaces?sort=wrong", "", 400)
	f.call(t, a, "GET", "/api/workspaces?q=%00", "", 400)
	f.call(t, a, "GET", "/api/workspaces/not-a-uuid", "", 400)
	noCSRF := a
	noCSRF.csrf = "bad"
	f.call(t, noCSRF, "POST", "/api/workspaces", `{"name":"x","description":""}`, 403)
	// Origin is independently required even with the right session and CSRF.
	req, _ := http.NewRequestWithContext(t.Context(), "POST", f.url+"/api/workspaces", strings.NewReader(`{"name":"x","description":""}`))
	req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: a.token})
	req.Header.Set("X-CSRF-Token", a.csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal(res.StatusCode)
	}
	exec(t, f, `UPDATE users SET must_change_password=true,temporary_password_expires_at=now()+interval '1 hour' WHERE id=$1`, a.id)
	f.call(t, a, "GET", "/api/workspaces", "", 403)
	f.call(t, a, "POST", "/api/workspaces", `{"name":"x","description":""}`, 403)
	exec(t, f, `UPDATE users SET disabled=true WHERE id=$1`, a.id)
	f.call(t, a, "GET", "/api/workspaces", "", 401)
	var count int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspaces`).Scan(&count)
	if count != 0 {
		t.Fatal("invalid creation persisted")
	}
}

func TestLastOwnerDisableIsAtomic(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	other := f.user(t, "user")
	w := f.create(t, owner, "Private", "Secret")
	_, data := f.call(t, f.admin, "POST", "/api/auth/users/"+owner.id.String()+"/disable", "", 409)
	if string(data) != "{\"error\":\"last_owner\"}\n" {
		t.Fatal(string(data))
	}
	f.call(t, owner, "GET", "/api/workspaces", "", 200)
	exec(t, f, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'owner')`, w.ID, other.id)
	var wg sync.WaitGroup
	for _, a := range []account{owner, other} {
		wg.Go(func() {
			// Either owner can be disabled first, but the second must conflict.
			req, _ := http.NewRequestWithContext(t.Context(), "POST", f.url+"/api/auth/users/"+a.id.String()+"/disable", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", f.admin.csrf)
			req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: f.admin.token})
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_ = res.Body.Close()
			if res.StatusCode != 200 && res.StatusCode != 409 {
				t.Error(res.StatusCode)
			}
		})
	}
	wg.Wait()
	var enabled int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE workspace_id=$1 AND m.role='owner' AND NOT u.disabled`, w.ID).Scan(&enabled); err != nil || enabled != 1 {
		t.Fatalf("enabled=%d err=%v", enabled, err)
	}
}

func TestCreateRacesWithDisable(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	var wg sync.WaitGroup
	for _, op := range []struct {
		a                  account
		method, path, body string
	}{
		{owner, "POST", "/api/workspaces", `{"name":"Race","description":""}`},
		{f.admin, "POST", "/api/auth/users/" + owner.id.String() + "/disable", ""},
	} {
		wg.Go(func() {
			req, _ := http.NewRequestWithContext(t.Context(), op.method, f.url+op.path, strings.NewReader(op.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", op.a.csrf)
			req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: op.a.token})
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_ = res.Body.Close()
			if res.StatusCode != 201 && res.StatusCode != 200 && res.StatusCode != 401 && res.StatusCode != 409 {
				t.Error(res.StatusCode)
			}
		})
	}
	wg.Wait()
	var ownerless int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspaces w WHERE NOT EXISTS (SELECT 1 FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=w.id AND m.role='owner' AND NOT u.disabled)`).Scan(&ownerless); err != nil || ownerless != 0 {
		t.Fatalf("ownerless=%d err=%v", ownerless, err)
	}
}

func TestWorkspaceMutationsRollBackOnDatabaseFailure(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	exec(t, f, `CREATE FUNCTION fail_membership() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test failure'; END $$`)
	exec(t, f, `CREATE TRIGGER reject_membership BEFORE INSERT ON workspace_members FOR EACH ROW EXECUTE FUNCTION fail_membership()`)
	f.call(t, owner, "POST", "/api/workspaces", `{"name":"Fail","description":""}`, 500)
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspaces`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partially created: %d %v", count, err)
	}
	exec(t, f, `DROP TRIGGER reject_membership ON workspace_members`)
	w := f.create(t, owner, "Kept", "History")
	exec(t, f, `INSERT INTO operations(workspace_id,seq,batch_id,actor_id,ops) VALUES($1,1,$2,$3,'[]')`, w.ID, uuid.New(), owner.id)
	exec(t, f, `CREATE TRIGGER reject_history_delete BEFORE DELETE ON operations FOR EACH ROW EXECUTE FUNCTION fail_membership()`)
	f.call(t, owner, "DELETE", "/api/workspaces/"+w.ID.String(), "", 500)
	f.call(t, owner, "GET", "/api/workspaces/"+w.ID.String(), "", 200)
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_members WHERE workspace_id=$1`, w.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial deletion: %d %v", count, err)
	}
}

func TestLegacyWorkspaceAndExpiredSessionAreInaccessible(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	legacy := uuid.New()
	exec(t, f, `INSERT INTO workspaces(id) VALUES($1)`, legacy)
	_, data := f.call(t, f.admin, "GET", "/api/workspaces", "", 200)
	if string(data) != "[]\n" {
		t.Fatal(string(data))
	}
	f.call(t, f.admin, "GET", "/api/workspaces/"+legacy.String(), "", 404)
	f.create(t, owner, "Visible", "Until expiry")
	hash := sha256.Sum256([]byte(owner.token))
	exec(t, f, `UPDATE auth_sessions SET absolute_expiry=now()-interval '1 second' WHERE token_hash=$1`, hash[:])
	f.call(t, owner, "GET", "/api/workspaces", "", 401)
	f.call(t, owner, "POST", "/api/workspaces", `{"name":"Fail","description":""}`, 401)
}
