package fsjail

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/randx"
)

const copyBuf = 1 << 20

var bufPool = sync.Pool{New: func() any { b := make([]byte, copyBuf); return &b }}

// CopyBuffered is io.Copy with a pooled 1 MiB buffer.
func CopyBuffered(dst io.Writer, src io.Reader) (int64, error) {
	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	return io.CopyBuffer(dst, src, *bp)
}

// EnsureTmp creates the hidden scratch folder of a shared folder.
func EnsureTmp(root *os.Root) error { return root.MkdirAll(TmpDir, 0o700) }

// SkipEntry hides symlinks, devices, sockets and our own scratch files.
func SkipEntry(d fs.DirEntry) bool {
	t := d.Type()
	if !(t.IsRegular() || t.IsDir()) {
		return true
	}
	return IsHidden(d.Name())
}

// OpenRegular opens rel for reading only if it is a plain file. Listings hide
// symlinks, FIFOs and devices; refusing them here too keeps a guessed name from
// following a link or blocking a request forever on a named pipe.
func OpenRegular(root *os.Root, rel string) (*os.File, os.FileInfo, error) {
	p := OSPath(rel)
	lst, err := root.Lstat(p)
	if err != nil {
		return nil, nil, err
	}
	if lst.IsDir() {
		return nil, lst, errIsDir
	}
	if !lst.Mode().IsRegular() {
		return nil, nil, fs.ErrNotExist
	}
	f, err := root.Open(p)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() { // swapped after the Lstat
		f.Close()
		return nil, nil, fs.ErrNotExist
	}
	return f, st, nil
}

var errIsDir = errors.New("is a directory")

// IsDirErr reports whether OpenRegular failed because rel is a folder.
func IsDirErr(err error) bool { return err == errIsDir }

func suffixName(name string, i int) string {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" { // dotfile
		base, ext = name, ""
	}
	return fmt.Sprintf("%s (%d)%s", base, i, ext)
}

// UniqueName returns a name inside dir that does not exist yet.
func UniqueName(root *os.Root, dir, name string) string {
	n := name
	for i := 1; i < 10000; i++ {
		if _, err := root.Lstat(filepath.Join(OSPath(dir), n)); errors.Is(err, fs.ErrNotExist) {
			return n
		}
		n = suffixName(name, i)
	}
	return n + "-" + randx.Token(4)
}

// Place moves a finished temp file to dir/name according to the conflict
// policy ("overwrite", "fail" or anything else = keep both).
func Place(root *os.Root, tmp, dir, name, conflict string) (string, error) {
	final := func(n string) string { return filepath.Join(OSPath(dir), n) }
	if conflict == "overwrite" {
		return name, root.Rename(tmp, final(name))
	}
	for i := 0; i < 10000; i++ {
		n := name
		if i > 0 {
			n = suffixName(name, i)
		}
		err := root.Link(tmp, final(n)) // atomic create-if-absent
		if err == nil {
			root.Remove(tmp)
			return n, nil
		}
		if errors.Is(err, fs.ErrExist) {
			if conflict == "fail" {
				return "", fs.ErrExist
			}
			continue
		}
		// hard links unsupported (FAT/exFAT/network shares): fall back
		if _, serr := root.Lstat(final(n)); serr == nil {
			if conflict == "fail" {
				return "", fs.ErrExist
			}
			continue
		}
		return n, root.Rename(tmp, final(n))
	}
	return "", fs.ErrExist
}

func copyFile(root *os.Root, src, dst string) error {
	in, err := root.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := root.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in) // copy_file_range/clone on supporting platforms
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		root.Remove(dst)
		return err
	}
	_ = root.Chtimes(dst, time.Now(), st.ModTime())
	return nil
}

// CopyTree copies src to dst (io/fs style paths), skipping symlinks and
// hidden scratch files. dst must not exist yet.
//
// The walk never descends into dst itself. Callers already refuse to copy a
// folder into itself, but name comparison alone cannot catch every alias on
// case-insensitive or 8.3-short-name file systems, and missing one would copy
// the tree into itself until the disk is full.
func CopyTree(root *os.Root, src, dst string) error {
	var dstInfo os.FileInfo
	return fs.WalkDir(root.FS(), src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if SkipEntry(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, src), "/")
		target := dst
		if rel != "" {
			target = path.Join(dst, rel)
		}
		if d.IsDir() {
			if dstInfo != nil {
				if st, err := root.Stat(filepath.FromSlash(p)); err == nil && os.SameFile(st, dstInfo) {
					return fs.SkipDir
				}
			}
			if err := root.MkdirAll(filepath.FromSlash(target), 0o755); err != nil {
				return err
			}
			if dstInfo == nil {
				if dstInfo, err = root.Stat(filepath.FromSlash(dst)); err != nil {
					return err
				}
			}
			return nil
		}
		return copyFile(root, filepath.FromSlash(p), filepath.FromSlash(target))
	})
}
