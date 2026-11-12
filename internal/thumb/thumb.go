// Package thumb makes small JPEG thumbnails of photos with the standard
// library only, and keeps the on-disk thumbnail cache within a size budget.
package thumb

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif" // register decoders
	"image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	Edge      = 320        // longest edge in pixels (crisp 160px tiles on HiDPI)
	MaxPixels = 40_000_000 // decompression-bomb guard, checked before decoding
	MaxBytes  = 64 << 20   // larger source files are not thumbnailed
	CacheMax  = 512 << 20  // on-disk cache budget
)

var exts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true}

// Supported reports whether files with this (lower-case) extension get thumbnails.
func Supported(ext string) bool { return exts[ext] }

var ErrNoThumb = errors.New("no thumbnail")

// Make decodes an image and returns a JPEG thumbnail, upright per EXIF.
func Make(f io.ReadSeeker, ext string) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrNoThumb
	}
	orient := 1
	if ext == ".jpg" || ext == ".jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		orient = jpegOrientation(io.LimitReader(f, 256<<10))
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, err
	}
	dst := orientImage(downscale(src, Edge), orient)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// downscale shrinks src so its longest edge is at most edge, averaging a 3x3
// grid of samples per output pixel. Transparency is flattened onto white.
func downscale(src image.Image, edge int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw, dh := sw, sh
	if sw > edge || sh > edge {
		if sw >= sh {
			dw, dh = edge, max(1, sh*edge/sw)
		} else {
			dw, dh = max(1, sw*edge/sh), edge
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	const n = 3
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sr, sg, sb uint64
			for j := 0; j < n; j++ {
				sy := b.Min.Y + min(sh-1, (y*n+j)*sh/(dh*n))
				for i := 0; i < n; i++ {
					sx := b.Min.X + min(sw-1, (x*n+i)*sw/(dw*n))
					r, g, bl, al := src.At(sx, sy).RGBA()
					sr += uint64(r + 0xffff - al)
					sg += uint64(g + 0xffff - al)
					sb += uint64(bl + 0xffff - al)
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8(sr / (n * n) >> 8)
			dst.Pix[o+1] = uint8(sg / (n * n) >> 8)
			dst.Pix[o+2] = uint8(sb / (n * n) >> 8)
			dst.Pix[o+3] = 0xff
		}
	}
	return dst
}

// orientImage applies an EXIF orientation (1-8) so phone photos stand upright.
func orientImage(src *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dst.PixOffset(dx, dy):][:4], src.Pix[src.PixOffset(x, y):][:4])
		}
	}
	return dst
}

// jpegOrientation reads the EXIF orientation tag from a JPEG's APP1 segment.
func jpegOrientation(r io.Reader) int {
	br := bufio.NewReader(r)
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil || soi != [2]byte{0xFF, 0xD8} {
		return 1
	}
	for {
		var m [4]byte
		if _, err := io.ReadFull(br, m[:]); err != nil || m[0] != 0xFF {
			return 1
		}
		n := int(m[2])<<8 | int(m[3])
		if m[1] == 0xDA || m[1] == 0xD9 || n < 2 {
			return 1
		}
		seg := make([]byte, n-2)
		if _, err := io.ReadFull(br, seg); err != nil {
			return 1
		}
		if m[1] == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
	}
}

func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	cnt := int(bo.Uint16(t[off:]))
	for i := 0; i < cnt; i++ {
		e := off + 2 + i*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// CleanCache keeps the thumbnail cache in dir under maxBytes by deleting the
// least recently used files.
func CleanCache(dir string, maxBytes int64) {
	type tf struct {
		p    string
		size int64
		mod  time.Time
	}
	var files []tf
	var total int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			files = append(files, tf{p, info.Size(), info.ModTime()})
			total += info.Size()
		}
		return nil
	})
	if total <= maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= maxBytes*8/10 {
			break
		}
		if os.Remove(f.p) == nil {
			total -= f.size
		}
	}
}
