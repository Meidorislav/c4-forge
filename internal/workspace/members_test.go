package workspace

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func (f *fixture) add(t *testing.T, owner, target account, w workspace, role string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": "  " + strings.ToUpper(target.id.String()) + "@EXAMPLE.TEST  ", "role": role})
	f.call(t, owner, "POST", "/api/workspaces/"+w.ID.String()+"/members", string(body), 204)
}

func (f *fixture) role(t *testing.T, w workspace, a account) string {
	t.Helper()
	var role string
	if err := f.pool.QueryRow(t.Context(), `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, w.ID, a.id).Scan(&role); err != nil {
		t.Fatal(err)
	}
	return role
}

func TestMembersLifecycleAndRESTAuthorization(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	viewer := f.user(t, "user")
	editor := f.user(t, "user")
	outsider := f.admin
	w := f.create(t, owner, "Shared", "Workspace")
	base := "/api/workspaces/" + w.ID.String()
	f.add(t, owner, viewer, w, "viewer")
	f.add(t, owner, editor, w, "editor")
	_, data := f.call(t, owner, "GET", base+"/members", "", 200)
	var members []member
	if err := json.Unmarshal(data, &members); err != nil || len(members) != 3 {
		t.Fatalf("%s %v", data, err)
	}
	selves := 0
	for _, m := range members {
		if m.IsCurrentUser {
			selves++
			if m.UserID != owner.id {
				t.Fatal("wrong self")
			}
		}
	}
	if selves != 1 {
		t.Fatal("missing current user")
	}
	for _, a := range []account{viewer, editor, outsider, {}} {
		want := 403
		if a.id == outsider.id {
			want = 404
		}
		if a.token == "" {
			want = 401
		}
		for _, op := range []struct{ method, path, body string }{
			{"GET", base + "/members", ""},
			{"POST", base + "/members", `{"email":"person@example.test","role":"viewer"}`},
			{"PUT", base + "/members/" + owner.id.String(), `{"role":"viewer"}`},
			{"DELETE", base + "/members/" + owner.id.String(), ""},
			{"POST", base + "/transfer", `{"userId":"` + editor.id.String() + `"}`},
		} {
			f.call(t, a, op.method, op.path, op.body, want)
		}
	}
	f.call(t, owner, "PUT", base+"/members/"+viewer.id.String(), `{"role":"editor"}`, 204)
	if f.role(t, w, viewer) != "editor" {
		t.Fatal("role not changed")
	}
	// A promoted owner can manage members using the same session.
	f.call(t, owner, "PUT", base+"/members/"+editor.id.String(), `{"role":"owner"}`, 204)
	f.call(t, editor, "GET", base+"/members", "", 200)
	f.call(t, editor, "DELETE", base+"/members/"+viewer.id.String(), "", 204)
	f.call(t, viewer, "GET", base, "", 404)
	_, data = f.call(t, viewer, "GET", "/api/workspaces", "", 200)
	if string(data) != "[]\n" {
		t.Fatal(string(data))
	}
	// Other workspaces and memberships are unaffected by removal.
	own := f.create(t, viewer, "Own", "Private")
	f.call(t, viewer, "GET", "/api/workspaces/"+own.ID.String(), "", 200)
	f.call(t, editor, "PUT", base+"/members/"+owner.id.String(), `{"role":"viewer"}`, 204)
	f.call(t, owner, "GET", base+"/members", "", 403)
	f.call(t, owner, "PUT", base, `{"name":"No","description":""}`, 403)
	f.call(t, owner, "DELETE", base, "", 403)
	_, data = f.call(t, editor, "GET", base, "", 200)
	var after workspace
	_ = json.Unmarshal(data, &after)
	if !after.UpdatedAt.Equal(w.UpdatedAt) {
		t.Fatal("membership changed model timestamp")
	}
}

func TestLastOwnerAndTransfer(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	next := f.user(t, "user")
	w := f.create(t, owner, "Transfer", "Test")
	base := "/api/workspaces/" + w.ID.String()
	f.call(t, owner, "DELETE", base+"/members/"+owner.id.String(), "", 409)
	f.call(t, owner, "PUT", base+"/members/"+owner.id.String(), `{"role":"editor"}`, 409)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"`+owner.id.String()+`"}`, 400)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"`+next.id.String()+`"}`, 404)
	f.add(t, owner, next, w, "viewer")
	// Disabled owners cannot satisfy last-owner protection.
	exec(t, f, `UPDATE workspace_members SET role='owner' WHERE workspace_id=$1 AND user_id=$2`, w.ID, next.id)
	exec(t, f, `UPDATE users SET disabled=true WHERE id=$1`, next.id)
	f.call(t, owner, "DELETE", base+"/members/"+owner.id.String(), "", 409)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"`+next.id.String()+`"}`, 409)
	exec(t, f, `UPDATE users SET disabled=false,must_change_password=true,temporary_password_expires_at=now()+interval '1 hour' WHERE id=$1`, next.id)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"`+next.id.String()+`"}`, 409)
	f.call(t, owner, "PUT", base+"/members/"+next.id.String(), `{"role":"owner"}`, 409)
	exec(t, f, `UPDATE users SET must_change_password=false,temporary_password_expires_at=NULL WHERE id=$1`, next.id)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"`+next.id.String()+`"}`, 204)
	if f.role(t, w, owner) != "editor" || f.role(t, w, next) != "owner" {
		t.Fatal("transfer not applied")
	}
	f.call(t, owner, "GET", base, "", 200)
	f.call(t, owner, "GET", base+"/members", "", 403)
	f.call(t, next, "PUT", base+"/members/"+owner.id.String(), `{"role":"owner"}`, 204)
	f.call(t, next, "DELETE", base+"/members/"+next.id.String(), "", 204)
	f.call(t, next, "GET", base, "", 404)
	f.call(t, owner, "GET", base+"/members", "", 200)
}

func TestMemberInputAndSecurity(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	target := f.user(t, "user")
	w := f.create(t, owner, "Input", "Test")
	base := "/api/workspaces/" + w.ID.String()
	for _, body := range []string{`{}`, `null`, `{"email":"bad","role":"viewer"}`, `{"email":"a@example.test","role":"owner"}`, `{"email":"a@example.test","role":"commenter"}`, `{"email":"a@example.test","role":"viewer","userId":"spoof"}`} {
		f.call(t, owner, "POST", base+"/members", body, 400)
	}
	f.call(t, owner, "POST", base+"/members", `{"email":"absent@example.test","role":"viewer"}`, 409)
	f.add(t, owner, target, w, "editor")
	body := `{"email":"` + target.id.String() + `@example.test","role":"viewer"}`
	f.call(t, owner, "POST", base+"/members", body, 409)
	memberPath := base + "/members/" + target.id.String()
	for _, role := range []string{"commenter", "admin", ""} {
		f.call(t, owner, "PUT", memberPath, `{"role":"`+role+`"}`, 400)
	}
	f.call(t, owner, "PUT", memberPath, `{"role":"owner","email":"extra"}`, 400)
	f.call(t, owner, "PUT", base+"/members/not-id", `{"role":"viewer"}`, 400)
	f.call(t, owner, "DELETE", base+"/members/"+uuid.NewString(), "", 404)
	f.call(t, owner, "POST", base+"/transfer", `{"userId":"bad"}`, 400)
	// All mutation endpoints require CSRF and exact origin, independently.
	for _, op := range []struct{ method, path, body string }{
		{"POST", base + "/members", body}, {"PUT", memberPath, `{"role":"viewer"}`}, {"DELETE", memberPath, ""}, {"POST", base + "/transfer", `{"userId":"` + target.id.String() + `"}`},
	} {
		bad := owner
		bad.csrf = "invalid"
		f.call(t, bad, op.method, op.path, op.body, 403)
		req, _ := http.NewRequestWithContext(t.Context(), op.method, f.url+op.path, strings.NewReader(op.body))
		req.Header.Set("Origin", "https://other.example")
		req.Header.Set("X-CSRF-Token", owner.csrf)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: owner.token})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal(res.StatusCode)
		}
	}
}

func TestTransferRollsBackBothRoles(t *testing.T) {
	f := setup(t)
	owner := f.user(t, "user")
	target := f.user(t, "user")
	w := f.create(t, owner, "Rollback", "Test")
	f.add(t, owner, target, w, "viewer")
	exec(t, f, `CREATE FUNCTION fail_demotion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.role='owner' AND NEW.role='editor' THEN RAISE EXCEPTION 'test failure'; END IF; RETURN NEW; END $$`)
	exec(t, f, `CREATE TRIGGER reject_demotion BEFORE UPDATE ON workspace_members FOR EACH ROW EXECUTE FUNCTION fail_demotion()`)
	f.call(t, owner, "POST", "/api/workspaces/"+w.ID.String()+"/transfer", `{"userId":"`+target.id.String()+`"}`, 500)
	if f.role(t, w, owner) != "owner" || f.role(t, w, target) != "viewer" {
		t.Fatal("partial transfer committed")
	}
}

func TestConcurrentOwnerChangesAndDisable(t *testing.T) {
	for _, operation := range []string{"demote", "remove", "transfer"} {
		t.Run(operation, func(t *testing.T) {
			f := setup(t)
			a := f.user(t, "user")
			b := f.user(t, "user")
			w := f.create(t, a, "Concurrent", "Test")
			f.add(t, a, b, w, "editor")
			base := "/api/workspaces/" + w.ID.String()
			f.call(t, a, "PUT", base+"/members/"+b.id.String(), `{"role":"owner"}`, 204)
			var wg sync.WaitGroup
			for i := range 2 {
				wg.Go(func() {
					actor := a
					method, path, body := "", "", ""
					if i == 0 {
						switch operation {
						case "demote":
							method, path, body = "PUT", base+"/members/"+a.id.String(), `{"role":"editor"}`
						case "remove":
							method, path = "DELETE", base+"/members/"+a.id.String()
						case "transfer":
							method, path, body = "POST", base+"/transfer", `{"userId":"`+b.id.String()+`"}`
						}
					} else {
						actor = f.admin
						method, path = "POST", "/api/auth/users/"+b.id.String()+"/disable"
					}
					req, _ := http.NewRequestWithContext(t.Context(), method, f.url+path, strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Origin", origin)
					req.Header.Set("X-CSRF-Token", actor.csrf)
					req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: actor.token})
					res, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					_ = res.Body.Close()
					if res.StatusCode != 204 && res.StatusCode != 200 && res.StatusCode != 409 {
						t.Error(res.StatusCode)
					}
				})
			}
			wg.Wait()
			var count int
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=$1 AND m.role='owner' AND NOT u.disabled`, w.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("enabled owners=%d err=%v", count, err)
			}
		})
	}
}

func TestConcurrentOwnersCannotBothLeave(t *testing.T) {
	f := setup(t)
	a := f.user(t, "user")
	b := f.user(t, "user")
	w := f.create(t, a, "Stay owned", "Test")
	f.add(t, a, b, w, "editor")
	base := "/api/workspaces/" + w.ID.String()
	f.call(t, a, "PUT", base+"/members/"+b.id.String(), `{"role":"owner"}`, 204)
	var wg sync.WaitGroup
	for _, actor := range []account{a, b} {
		wg.Go(func() {
			req, _ := http.NewRequestWithContext(t.Context(), "DELETE", f.url+base+"/members/"+actor.id.String(), nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", actor.csrf)
			req.AddCookie(&http.Cookie{Name: "c4forge_session", Value: actor.token})
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_ = res.Body.Close()
			if res.StatusCode != 204 && res.StatusCode != 409 {
				t.Error(res.StatusCode)
			}
		})
	}
	wg.Wait()
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND role='owner'`, w.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("owners=%d err=%v", count, err)
	}
}
