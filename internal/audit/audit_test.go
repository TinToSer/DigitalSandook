package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditHMACChainDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.SetKey([]byte("super-secret-audit-key-32-bytes!"))
	l.Write("admin", "127.0.0.1", "login", "")
	l.Write("admin", "127.0.0.1", "upload", "docs:/a.txt")
	l.Write("admin", "127.0.0.1", "delete", "docs:/a.txt")

	checked, valid, bad := l.Verify()
	if !valid || checked != 3 || bad != 0 {
		t.Fatalf("Verify() = (%d, %v, %d), want (3, true, 0)", checked, valid, bad)
	}
	l.Close()

	// Tamper with line 2
	p := filepath.Join(dir, "audit.log")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(b), "docs:/a.txt", "docs:/b.txt", 1)
	if err := os.WriteFile(p, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	l2, _ := Open(dir)
	defer l2.Close()
	l2.SetKey([]byte("super-secret-audit-key-32-bytes!"))
	_, valid, bad = l2.Verify()
	if valid || bad != 2 {
		t.Fatalf("tampered log Verify() = valid:%v badLine:%d, want valid:false badLine:2", valid, bad)
	}
}
