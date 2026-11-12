package server

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

// Types that may be shown inline (preview). Everything else is forced to
// download, and every response carries a sandboxing CSP, so an uploaded
// HTML/SVG file can never run script on the app's origin.
var inlineTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true,
	"image/bmp": true, "image/x-icon": true,
	"video/mp4": true, "video/webm": true, "video/ogg": true, "video/quicktime": true,
	"audio/mpeg": true, "audio/ogg": true, "audio/wav": true, "audio/webm": true, "audio/flac": true,
	"audio/mp4": true, "audio/aac": true, "audio/x-wav": true,
	"application/pdf": true, "text/plain": true,
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".md", ".log", ".csv", ".ini", ".conf", ".yml", ".yaml", ".toml", ".go", ".py", ".c", ".h", ".cpp", ".java", ".sh", ".bat", ".rs", ".ts", ".sql", ".json", ".xml":
		return "text/plain; charset=utf-8"
	case ".m4a":
		return "audio/mp4"
	case ".opus":
		return "audio/ogg"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/webm"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func serveFile(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo, name string, inline bool) {
	ct := contentTypeFor(name)
	base, _, _ := mime.ParseMediaType(ct)
	h := w.Header()
	disp := "attachment"
	if inline && inlineTypes[base] {
		disp = "inline"
		if base != "application/pdf" {
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		} else {
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'self'")
		}
		h.Set("X-Frame-Options", "SAMEORIGIN")
	} else {
		ct = "application/octet-stream"
		if base == "text/plain" {
			ct = "text/plain; charset=utf-8"
		}
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": safeDispName(name)}))
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", fmt.Sprintf(`W/"%x-%x"`, st.Size(), st.ModTime().UnixNano()))
	h.Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, "", st.ModTime(), f) // Range, If-Range, 304; sendfile on plain TCP
}

func safeDispName(n string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, n)
}

// firstRequest reports whether r starts a download (not a resumed range), so
// downloads are counted and audited once.
func firstRequest(r *http.Request) bool {
	rg := r.Header.Get("Range")
	return rg == "" || strings.HasPrefix(rg, "bytes=0-")
}

// openForDownload opens a regular file of t, writing the error response if
// that is not possible.
func openForDownload(w http.ResponseWriter, t *target) (*os.File, os.FileInfo, bool) {
	f, st, err := fsjail.OpenRegular(t.root, t.rel)
	if fsjail.IsDirErr(err) {
		jerr(w, 400, "is a folder; use zip")
		return nil, nil, false
	}
	if err != nil {
		fsErr(w, err)
		return nil, nil, false
	}
	return f, st, true
}

func (a *App) hDownload(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlRead)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	f, st, good := openForDownload(w, t)
	if !good {
		return
	}
	defer f.Close()
	inline := q.Get("inline") == "1"
	if !inline && firstRequest(r) {
		a.auditf(c, "download", "%s:%s", t.m.Name, t.rel)
	}
	serveFile(w, r, f, st, path.Base(t.rel), inline)
}

// hZip streams a ZIP (stored, no compression => wire speed) of a folder or of
// selected names inside a folder. Nothing is buffered or written to disk.
func (a *App) hZip(w http.ResponseWriter, r *http.Request, c *Ctx) {
	q := r.URL.Query()
	t, code, msg := a.target(c, q.Get("m"), q.Get("p"), store.LvlRead)
	if code != 0 {
		jerr(w, code, msg)
		return
	}
	select {
	case a.zipSem <- struct{}{}:
		defer func() { <-a.zipSem }()
	default:
		jerr(w, 429, "too many archive downloads in progress")
		return
	}
	a.streamZip(w, r, c, t, q["n"])
}

// zipName maps a walked io/fs path p (at or beneath root) to its name inside
// the archive, under prefix.
func zipName(root, prefix, p string) string {
	rel := ""
	switch {
	case p == root:
	case root == ".":
		rel = p
	default:
		rel = strings.TrimPrefix(p, root+"/")
	}
	rel = strings.ReplaceAll(rel, "\\", "_") // a legal name byte on Unix, a separator in ZIP readers
	switch {
	case prefix == "":
		return rel
	case rel == "":
		return prefix
	}
	return prefix + "/" + rel
}

func describeNames(names []string) string {
	if len(names) <= 20 {
		return fmt.Sprint(names)
	}
	return fmt.Sprintf("[%d items]", len(names))
}

func (a *App) streamZip(w http.ResponseWriter, r *http.Request, c *Ctx, t *target, names []string) {
	base := fsjail.FSPath(t.rel)
	type item struct{ fsPath, prefix string }
	var roots []item
	archive := "download"
	if len(names) == 0 {
		st, err := t.root.Stat(t.os())
		if err != nil {
			fsErr(w, err)
			return
		}
		if !st.IsDir() {
			jerr(w, 400, "not a folder")
			return
		}
		archive = path.Base(t.rel)
		if t.rel == "/" {
			archive = t.m.Name
		}
		roots = []item{{base, ""}}
	} else {
		if len(names) > 10000 {
			jerr(w, 400, "too many items")
			return
		}
		for _, n := range names {
			if !fsjail.ValidSeg(n) {
				jerr(w, 400, "invalid name")
				return
			}
			roots = append(roots, item{path.Join(base, n), strings.ReplaceAll(n, "\\", "_")})
		}
		if len(names) == 1 {
			archive = names[0]
		}
	}
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeDispName(archive) + ".zip"}))
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	bw := bufio.NewWriterSize(w, 1<<20)
	zw := zip.NewWriter(bw)
	a.auditf(c, "zip", "%s:%s %s", t.m.Name, t.rel, describeNames(names))
	ctx := r.Context()
	fsys := t.root.FS()
	for _, it := range roots {
		err := fs.WalkDir(fsys, it.fsPath, func(p string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return nil
			}
			if fsjail.SkipEntry(d) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			arc := zipName(it.fsPath, it.prefix, p)
			if arc == "" {
				return nil
			}
			hdr := &zip.FileHeader{Name: arc, Method: zip.Store, Modified: info.ModTime()}
			if d.IsDir() {
				hdr.Name += "/"
				_, err := zw.CreateHeader(hdr)
				return err
			}
			hdr.UncompressedSize64 = uint64(info.Size()) // lets the writer pick ZIP64 up front
			f, err := fsys.Open(p)
			if err != nil {
				return nil
			}
			defer f.Close()
			zwr, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			_, err = fsjail.CopyBuffered(zwr, f)
			return err
		})
		if err != nil {
			return // client went away or write failed; connection is dropped
		}
	}
	if err := zw.Close(); err == nil {
		bw.Flush()
	}
}
