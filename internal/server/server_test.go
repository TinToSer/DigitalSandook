package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

const testPW = "test-password-123"

type client struct {
	t    *testing.T
	base string
	hc   *http.Client
	csrf string
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, hc: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Origin", c.base)
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (c *client) must(method, path string, body any) []byte {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if code >= 300 {
		c.t.Fatalf("%s %s: %d %s", method, path, code, out)
	}
	return out
}

func (c *client) login(user, pw string) int {
	c.t.Helper()
	code, out := c.do("POST", "/api/login", map[string]string{"username": user, "password": pw})
	var r struct{ CSRF string }
	json.Unmarshal(out, &r)
	c.csrf = r.CSRF
	return code
}

// testApp starts a server with one shared folder "docs" and an admin "admin".
func testApp(t *testing.T) (*App, *httptest.Server, string) {
	t.Helper()
	shared := t.TempDir()
	a, err := New(Config{DataDir: t.TempDir(), PlainHTTP: true, Shares: []string{"docs=" + shared}, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.CreateUser(store.User{Name: "admin", Hash: auth.HashPassword(testPW), Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); a.Close() }) // cleanups run LIFO: before the TempDirs are removed
	return a, srv, shared
}

func q(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return "?" + v.Encode()
}

func TestFileWorkflow(t *testing.T) {
	_, srv, shared := testApp(t)
	c := newClient(t, srv.URL)
	if code := c.login("admin", "wrong-password"); code != 401 {
		t.Fatalf("bad password: %d", code)
	}
	if code := c.login("admin", testPW); code != 200 {
		t.Fatalf("login: %d", code)
	}

	// CSRF: a state-changing call without the token is rejected
	saved := c.csrf
	c.csrf = ""
	if code, _ := c.do("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/", "name": "x"}); code != 403 {
		t.Fatalf("mkdir without CSRF token: %d", code)
	}
	c.csrf = saved

	c.must("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/", "name": "proj"})
	c.must("PUT", "/api/put"+q("m", "docs", "p", "/", "name", ".gitignore"), []byte("ignored"))
	c.must("PUT", "/api/put"+q("m", "docs", "p", "/proj", "name", "a.txt"), []byte("hello"))
	c.must("PUT", "/api/put"+q("m", "docs", "p", "/", "rel", "up/sub", "name", "b.txt"), []byte("folder upload"))
	if code, _ := c.do("PUT", "/api/put"+q("m", "docs", "p", "/", "rel", "up/CON", "name", "c.txt"), []byte("x")); code != 400 {
		t.Fatalf("folder upload with a reserved name: %d", code)
	}

	var list struct {
		Entries []entry
	}
	json.Unmarshal(c.must("GET", "/api/list"+q("m", "docs", "p", "/"), nil), &list)
	if len(list.Entries) != 3 || list.Entries[0].Name != "proj" && list.Entries[0].Name != "up" {
		t.Fatalf("listing: %+v", list.Entries)
	}

	// ZIP of the whole shared folder keeps dotfile names intact
	z := c.must("GET", "/api/zip"+q("m", "docs", "p", "/"), nil)
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{".gitignore", "proj/", "proj/a.txt", "up/", "up/sub/", "up/sub/b.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("zip entries = %v, want %v", names, want)
	}

	if got := c.must("GET", "/api/dl"+q("m", "docs", "p", "/proj/a.txt"), nil); string(got) != "hello" {
		t.Fatalf("download = %q", got)
	}
	if code, _ := c.do("GET", "/api/dl"+q("m", "docs", "p", "/proj"), nil); code != 400 {
		t.Fatalf("download of a folder: %d", code)
	}
	if code, _ := c.do("GET", "/api/list"+q("m", "docs", "p", "/../"), nil); code != 400 {
		t.Fatalf("path traversal: %d", code)
	}

	// copying a folder into itself is refused, whatever the letter case
	c.must("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/proj", "name": "sub"})
	for _, n := range []string{"proj", "PROJ"} {
		if code, _ := c.do("POST", "/api/paste", map[string]any{"mount": "docs", "from": "/", "to": "/proj/sub", "op": "copy", "names": []string{n}}); code == 200 {
			t.Fatalf("copy %q into its own sub-folder succeeded", n)
		}
	}
	if _, err := os.Stat(filepath.Join(shared, "proj", "sub", "proj")); err == nil {
		t.Fatal("folder was copied into itself")
	}
}

func TestSharingFollowsRenameAndDiesWithDelete(t *testing.T) {
	a, srv, _ := testApp(t)
	c := newClient(t, srv.URL)
	c.login("admin", testPW)
	a.store.CreateUser(store.User{Name: "bob", Hash: auth.HashPassword(testPW), Created: time.Now()})
	c.must("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/", "name": "proj"})
	c.must("POST", "/api/grants", map[string]any{"mount": "docs", "path": "/proj", "user": "bob", "level": store.LvlRead})

	bob, _ := a.store.User("bob")
	c.must("POST", "/api/rename", map[string]string{"mount": "docs", "path": "/", "from": "proj", "to": "renamed"})
	if a.store.Level(bob, "docs", "/renamed") != store.LvlRead {
		t.Fatal("grant did not follow the renamed folder")
	}
	// someone creates a new "proj": bob must not see it
	c.must("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/", "name": "proj"})
	if a.store.Level(bob, "docs", "/proj") != 0 {
		t.Fatal("old grant applies to a new folder with the old name")
	}
	c.must("POST", "/api/delete", map[string]any{"mount": "docs", "path": "/", "names": []string{"renamed"}})
	c.must("POST", "/api/mkdir", map[string]string{"mount": "docs", "path": "/", "name": "renamed"})
	if a.store.Level(bob, "docs", "/renamed") != 0 {
		t.Fatal("grant survived deletion and applies to a new folder")
	}

	// a disabled user can no longer sign in
	c.must("PATCH", "/api/admin/users/bob", map[string]bool{"disabled": true})
	if code := newClient(t, srv.URL).login("bob", testPW); code != 401 {
		t.Fatalf("disabled user login: %d", code)
	}
	// the last admin cannot lock themselves out
	if code, _ := c.do("PATCH", "/api/admin/users/admin", map[string]bool{"admin": false}); code != 400 {
		t.Fatalf("removing own admin rights: %d", code)
	}
}

func TestPublicLinkPassword(t *testing.T) {
	_, srv, shared := testApp(t)
	os.WriteFile(filepath.Join(shared, "f.txt"), []byte("public"), 0o644)
	c := newClient(t, srv.URL)
	c.login("admin", testPW)
	var l struct{ Token string }
	json.Unmarshal(c.must("POST", "/api/links", map[string]any{"mount": "docs", "path": "/", "days": 1, "password": "secret-1"}), &l)

	anon := newClient(t, srv.URL)
	base := "/api/public/" + l.Token
	if code, _ := anon.do("GET", base+"/list", nil); code != 401 {
		t.Fatalf("locked link listing: %d", code)
	}
	if code, _ := anon.do("POST", base+"/unlock", map[string]string{"password": "nope"}); code != 403 {
		t.Fatalf("wrong link password: %d", code)
	}
	anon.must("POST", base+"/unlock", map[string]string{"password": "secret-1"})
	if got := anon.must("GET", base+"/dl"+q("p", "/f.txt"), nil); string(got) != "public" {
		t.Fatalf("link download = %q", got)
	}
	// the anonymous visitor cannot reach the private API
	if code, _ := anon.do("GET", "/api/list"+q("m", "docs", "p", "/"), nil); code != 401 {
		t.Fatalf("private API without session: %d", code)
	}
}

func TestZipName(t *testing.T) {
	cases := []struct{ root, prefix, p, want string }{
		{".", "", ".", ""},
		{".", "", ".env", ".env"},
		{".", "", "a/.b", "a/.b"},
		{"docs", "", "docs", ""},
		{"docs", "", "docs/.x", ".x"},
		{"docs/n", "n", "docs/n", "n"},
		{"docs/n", "n", "docs/n/.y", "n/.y"},
		{".", "", `a\b`, "a_b"},
	}
	for _, c := range cases {
		if got := zipName(c.root, c.prefix, c.p); got != c.want {
			t.Errorf("zipName(%q, %q, %q) = %q, want %q", c.root, c.prefix, c.p, got, c.want)
		}
	}
}
