package fsjail

import (
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	TmpDir    = ".dsd-tmp" // per-folder scratch dir for in-flight uploads (hidden)
	hiddenPre = ".dsd-"
)

var winReserved = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "conin$": true, "conout$": true}

// ValidSeg is the lenient check for existing path segments.
func ValidSeg(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 255 || !utf8.ValidString(s) {
		return false
	}
	if IsHidden(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '\\' || r == '/' {
			return false
		}
		if runtime.GOOS == "windows" && r == ':' { // alternate data streams
			return false
		}
	}
	return true
}

// IsHidden matches our scratch names case-insensitively: Windows and macOS
// file systems would otherwise resolve ".DSD-TMP" to ".dsd-tmp".
func IsHidden(name string) bool {
	return len(name) >= len(hiddenPre) && strings.EqualFold(name[:len(hiddenPre)], hiddenPre)
}

// spoofRune reports invisible / direction-changing characters that let a name
// such as "invoice‮fdp.exe" display as "invoiceexe.pdf".
func spoofRune(r rune) bool {
	return unicode.Is(unicode.Bidi_Control, r) || r == 0x200B || r == 0x2060 || r == 0xFEFF
}

// ValidNewName is the strict check for names a user is allowed to create:
// portable across Windows, macOS and Linux and free of display tricks.
func ValidNewName(s string) bool {
	if !ValidSeg(s) || strings.ContainsAny(s, `<>:"|?*`) || strings.IndexFunc(s, spoofRune) >= 0 {
		return false
	}
	if strings.HasSuffix(s, " ") || strings.HasSuffix(s, ".") {
		return false
	}
	base := strings.ToLower(s)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if winReserved[base] {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9' {
		return false
	}
	return true
}

// CleanRel validates a client path and returns it as "/a/b".
func CleanRel(p string) (string, bool) {
	return cleanWith(p, ValidSeg)
}

// CleanNewRel is CleanRel with the strict ValidNewName rule for every segment;
// use it for folder structures a client is about to create.
func CleanNewRel(p string) (string, bool) {
	return cleanWith(p, ValidNewName)
}

func cleanWith(p string, ok func(string) bool) (string, bool) {
	if strings.IndexByte(p, 0) >= 0 || len(p) > 4096 {
		return "", false
	}
	var segs []string
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." {
			continue
		}
		if !ok(s) {
			return "", false
		}
		segs = append(segs, s)
	}
	return "/" + strings.Join(segs, "/"), true
}

// OSPath converts a clean "/a/b" path into an os.Root-relative OS path.
func OSPath(rel string) string {
	if rel == "/" {
		return "."
	}
	return filepath.FromSlash(rel[1:])
}

// FSPath converts "/a/b" to an io/fs path ("a/b" or ".").
func FSPath(rel string) string {
	if rel == "/" {
		return "."
	}
	return rel[1:]
}

// Join appends a single validated name to a clean "/a/b" path.
func Join(dir, name string) string { return path.Join(dir, name) }

// Within reports whether clean path p equals base or lies beneath it,
// ignoring case. Use it for "into itself" checks: on case-insensitive file
// systems "/A" and "/a" are the same folder.
func Within(base, p string) bool {
	if base == "/" {
		return true
	}
	if len(p) < len(base) || !strings.EqualFold(p[:len(base)], base) {
		return false
	}
	return len(p) == len(base) || p[len(base)] == '/'
}
