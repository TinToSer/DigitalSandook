package fsjail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHiddenAndSpoofNames(t *testing.T) {
	for _, p := range []string{"/.dsd-tmp", "/.DSD-TMP", "/a/.Dsd-Tmp/x", "/a/../b", "/a\\b"} {
		if _, ok := CleanRel(p); ok {
			t.Errorf("CleanRel(%q) accepted", p)
		}
	}
	for _, n := range []string{"invoice‮fdp.exe", "a​b", "CON", "lpt1.txt", "x.", "a:b", "a?b", "a/b"} {
		if ValidNewName(n) {
			t.Errorf("ValidNewName(%q) accepted", n)
		}
	}
	for _, n := range []string{"report.pdf", "Ünïcødé 文件.txt", "👨‍👩‍👧 family.jpg", ".gitignore"} {
		if !ValidNewName(n) {
			t.Errorf("ValidNewName(%q) rejected", n)
		}
	}
	if _, ok := CleanNewRel("photos/CON/x"); ok {
		t.Error("CleanNewRel accepted a reserved folder name")
	}
	if p, ok := CleanNewRel("/photos//2024/"); !ok || p != "/photos/2024" {
		t.Errorf("CleanNewRel = %q, %v", p, ok)
	}
}

func TestWithinIgnoresCase(t *testing.T) {
	cases := []struct {
		base, p string
		want    bool
	}{
		{"/A", "/a/b", true}, {"/a", "/A", true}, {"/a", "/ab", false}, {"/a/b", "/a", false}, {"/", "/x", true},
	}
	for _, c := range cases {
		if got := Within(c.base, c.p); got != c.want {
			t.Errorf("Within(%q, %q) = %v", c.base, c.p, got)
		}
	}
}

func TestCopyTreeNeverCopiesIntoItself(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "f.txt"), []byte("x"), 0o644)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// what a case-insensitive alias ("A" -> "a/b/A") would slip past a name check
	if err := CopyTree(root, "a", "a/b/copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "copy", "f.txt")); err != nil {
		t.Fatal("content was not copied")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "copy", "b", "copy")); err == nil {
		t.Fatal("copy recursed into its own destination")
	}
}

func TestOpenRegular(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "d"), 0o755)
	os.WriteFile(filepath.Join(dir, "f"), []byte("hi"), 0o644)
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	if f, st, err := OpenRegular(root, "/f"); err != nil || st.Size() != 2 {
		t.Fatalf("regular file: %v", err)
	} else {
		f.Close()
	}
	if _, _, err := OpenRegular(root, "/d"); !IsDirErr(err) {
		t.Fatalf("folder: err = %v", err)
	}
}
