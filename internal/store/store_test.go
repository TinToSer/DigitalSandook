package store

import (
	"errors"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []User{{Name: "admin", Admin: true}, {Name: "alice"}, {Name: "bob"}, {Name: "carol"}} {
		if err := s.CreateUser(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PutMount(Mount{Name: "docs", Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	return s
}

func lvl(s *Store, name, p string) int { u, _ := s.User(name); return s.Level(u, "docs", p) }

func TestReshareChainIsRevokedWithGranter(t *testing.T) {
	s := testStore(t)
	ga, _ := s.SetGrant(Grant{Mount: "docs", Path: "/team", User: "alice", Level: LvlWrite, By: "admin"})
	s.SetGrant(Grant{Mount: "docs", Path: "/team/x", User: "bob", Level: LvlWrite, By: "alice"})
	s.SetGrant(Grant{Mount: "docs", Path: "/team/x/y", User: "carol", Level: LvlRead, By: "bob"})

	if got := lvl(s, "carol", "/team/x/y/file"); got != LvlRead {
		t.Fatalf("carol before revoke = %d", got)
	}
	if got := lvl(s, "carol", "/team/x"); got != 0 {
		t.Fatalf("carol must not see above her grant, got %d", got)
	}
	s.DeleteGrant(ga.ID) // admin revokes alice
	for _, n := range []string{"alice", "bob", "carol"} {
		if got := lvl(s, n, "/team/x/y"); got != 0 {
			t.Fatalf("%s still has level %d after alice was revoked", n, got)
		}
	}
	if u, _ := s.User("bob"); len(s.Roots(u)) != 0 {
		t.Fatalf("bob still lists a root after revoke: %+v", s.Roots(u))
	}
}

func TestGrantCappedByGranterLevel(t *testing.T) {
	s := testStore(t)
	s.SetGrant(Grant{Mount: "docs", Path: "/", User: "alice", Level: LvlRead, By: "admin"})
	// a stored grant that claims more than alice has (e.g. alice was downgraded later)
	s.SetGrant(Grant{Mount: "docs", Path: "/a", User: "bob", Level: LvlOwner, By: "alice"})
	if got := lvl(s, "bob", "/a"); got != LvlRead {
		t.Fatalf("bob = %d, want capped to alice's level %d", got, LvlRead)
	}
}

func TestGrantCycleWithoutAdminRootGrantsNothing(t *testing.T) {
	s := testStore(t)
	s.SetGrant(Grant{Mount: "docs", Path: "/", User: "alice", Level: LvlOwner, By: "bob"})
	s.SetGrant(Grant{Mount: "docs", Path: "/", User: "bob", Level: LvlOwner, By: "alice"})
	if lvl(s, "alice", "/") != 0 || lvl(s, "bob", "/") != 0 {
		t.Fatal("a grant cycle must not create access")
	}
}

func TestDisabledGranterRevokes(t *testing.T) {
	s := testStore(t)
	s.SetGrant(Grant{Mount: "docs", Path: "/", User: "alice", Level: LvlWrite, By: "admin"})
	s.SetGrant(Grant{Mount: "docs", Path: "/", User: "bob", Level: LvlRead, By: "alice"})
	if _, err := s.UpdateUser("alice", func(u *User) error { u.Disabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := lvl(s, "bob", "/"); got != 0 {
		t.Fatalf("bob = %d after alice was disabled", got)
	}
}

func TestCreateUserDoesNotOverwrite(t *testing.T) {
	s := testStore(t)
	if err := s.CreateUser(User{Name: "alice", Admin: true}); !errors.Is(err, ErrUserExists) {
		t.Fatalf("CreateUser over an existing user: err = %v", err)
	}
	if u, _ := s.User("alice"); u.Admin {
		t.Fatal("existing user was overwritten")
	}
}

func TestUpdateUserKeepsLastAdmin(t *testing.T) {
	s := testStore(t)
	if _, err := s.UpdateUser("admin", func(u *User) error { u.Disabled = true; return nil }); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disabling the only admin: err = %v", err)
	}
	if _, err := s.UpdateUser("admin", func(u *User) error { u.Admin = false; return nil }); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the only admin: err = %v", err)
	}
	if err := s.DeleteUser("admin"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting the only admin: err = %v", err)
	}
	if u, _ := s.User("admin"); !u.Admin || u.Disabled {
		t.Fatal("refused change was applied anyway")
	}
	// with a second admin it is allowed
	s.UpdateUser("alice", func(u *User) error { u.Admin = true; return nil })
	if _, err := s.UpdateUser("admin", func(u *User) error { u.Disabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateUserAppliesToCurrentRecord(t *testing.T) {
	s := testStore(t)
	// an admin disables bob while bob's (slow) login is in flight...
	s.UpdateUser("bob", func(u *User) error { u.Disabled = true; return nil })
	// ...then the login records its timestamp: it must not resurrect bob
	s.UpdateUser("bob", func(u *User) error { u.LastLogin = u.Created; return nil })
	if u, _ := s.User("bob"); !u.Disabled {
		t.Fatal("a later partial update re-enabled a disabled user")
	}
}

func TestDeletedPathLosesGrantsAndLinks(t *testing.T) {
	s := testStore(t)
	s.SetGrant(Grant{Mount: "docs", Path: "/proj", User: "alice", Level: LvlRead, By: "admin"})
	s.SetGrant(Grant{Mount: "docs", Path: "/proj/sub", User: "bob", Level: LvlRead, By: "admin"})
	s.SetGrant(Grant{Mount: "docs", Path: "/project2", User: "carol", Level: LvlRead, By: "admin"})
	s.PutLink(Link{Token: "t1", Mount: "docs", Path: "/proj/sub/f.txt", Owner: "admin"})
	if err := s.DropPath("docs", "/proj"); err != nil {
		t.Fatal(err)
	}
	// a new "/proj" created later must not be visible to the old audience
	if lvl(s, "alice", "/proj") != 0 || lvl(s, "bob", "/proj/sub") != 0 {
		t.Fatal("grants survived deletion of their folder")
	}
	if _, ok := s.Link("t1"); ok {
		t.Fatal("link survived deletion of its file")
	}
	if lvl(s, "carol", "/project2") != LvlRead {
		t.Fatal("a sibling with a common name prefix lost its grant")
	}
}

func TestMovedPathKeepsGrantsAndLinks(t *testing.T) {
	s := testStore(t)
	s.SetGrant(Grant{Mount: "docs", Path: "/a/proj", User: "alice", Level: LvlWrite, By: "admin"})
	s.SetGrant(Grant{Mount: "docs", Path: "/a/proj/x", User: "bob", Level: LvlRead, By: "alice"})
	s.PutLink(Link{Token: "t1", Mount: "docs", Path: "/a/proj/x/f.txt", Owner: "alice"})
	if err := s.MovePath("docs", "/a/proj", "/b/renamed"); err != nil {
		t.Fatal(err)
	}
	if lvl(s, "alice", "/a/proj") != 0 {
		t.Fatal("old path still grants access (a new folder there would inherit it)")
	}
	if lvl(s, "alice", "/b/renamed") != LvlWrite || lvl(s, "bob", "/b/renamed/x") != LvlRead {
		t.Fatal("grants did not follow the moved folder")
	}
	if l, _ := s.Link("t1"); l.Path != "/b/renamed/x/f.txt" {
		t.Fatalf("link path = %q", l.Path)
	}
}

func TestLinkHitsAreFlushed(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.PutLink(Link{Token: "t1", Mount: "docs", Path: "/"})
	s.HitLink("t1")
	s.HitLink("t1")
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := s2.Link("t1"); l.Hits != 2 {
		t.Fatalf("hits after reopen = %d, want 2", l.Hits)
	}
}
