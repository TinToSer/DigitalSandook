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
	"sync"
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

var readerPool = sync.Pool{New: func() any { return bufio.NewReaderSize(nil, 256<<10) }}

// Make decodes an image and returns a JPEG thumbnail, upright per EXIF.
func Make(f io.ReadSeeker, ext string) ([]byte, error) {
	br := readerPool.Get().(*bufio.Reader)
	defer func() {
		br.Reset(nil)
		readerPool.Put(br)
	}()

	br.Reset(f)
	cfg, _, err := image.DecodeConfig(br)
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
		br.Reset(io.LimitReader(f, 256<<10))
		orient = jpegOrientation(br)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	br.Reset(f)
	src, _, err := image.Decode(br)
	if err != nil {
		return nil, err
	}
	dst := orientImage(downscale(src, Edge), orient)
	buf := bytes.NewBuffer(make([]byte, 0, 16<<10))
	if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clampByte(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func ycbcrToRGB(y, cb, cr uint8) (uint8, uint8, uint8) {
	yy1 := int32(y) * 0x10101
	cb1 := int32(cb) - 128
	cr1 := int32(cr) - 128
	return clampByte((yy1 + 91881*cr1) >> 16),
		clampByte((yy1 - 22554*cb1 - 46802*cr1) >> 16),
		clampByte((yy1 + 116130*cb1) >> 16)
}

// downscale shrinks src so its longest edge is at most edge using a fast
// two-pass separable area-averaging (box) filter with direct pixel-slice fast
// paths (zero interface dispatch per pixel). Transparency is flattened onto white.
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

	// Pass 1: horizontal downscale (sw x sh -> dw x sh, 3 bytes RGB per pixel)
	tmp := make([]uint8, dw*sh*3)
	xStarts := make([]int, dw)
	xEnds := make([]int, dw)
	for x := 0; x < dw; x++ {
		x0 := x * sw / dw
		x1 := (x + 1) * sw / dw
		if x1 <= x0 {
			x1 = min(sw, x0+1)
		}
		xStarts[x] = x0
		xEnds[x] = x1
	}

	switch im := src.(type) {
	case *image.YCbCr:
		horizYCbCr(im, b, dw, sh, xStarts, xEnds, tmp)
	case *image.RGBA:
		horizRGBA(im, b, dw, sh, xStarts, xEnds, tmp)
	case *image.NRGBA:
		horizNRGBA(im, b, dw, sh, xStarts, xEnds, tmp)
	case *image.Paletted:
		horizPaletted(im, b, dw, sh, xStarts, xEnds, tmp)
	default:
		horizGeneric(src, b, dw, sh, xStarts, xEnds, tmp)
	}

	// Pass 2: vertical downscale (dw x sh -> dw x dh into *image.RGBA)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	rowStride := dw * 3
	for y := 0; y < dh; y++ {
		y0 := y * sh / dh
		y1 := (y + 1) * sh / dh
		if y1 <= y0 {
			y1 = min(sh, y0+1)
		}
		cnt := uint32(y1 - y0)
		dstRow := dst.Pix[y*dst.Stride:]
		for x := 0; x < dw; x++ {
			var sr, sg, sb uint32
			col := x * 3
			for sy := y0; sy < y1; sy++ {
				o := sy*rowStride + col
				sr += uint32(tmp[o])
				sg += uint32(tmp[o+1])
				sb += uint32(tmp[o+2])
			}
			do := x * 4
			dstRow[do] = uint8(sr / cnt)
			dstRow[do+1] = uint8(sg / cnt)
			dstRow[do+2] = uint8(sb / cnt)
			dstRow[do+3] = 0xff
		}
	}
	return dst
}

func horizYCbCr(im *image.YCbCr, b image.Rectangle, dw, sh int, xStarts, xEnds []int, tmp []uint8) {
	for y := 0; y < sh; y++ {
		sy := b.Min.Y + y
		yRowOff := (sy - im.Rect.Min.Y) * im.YStride
		cRowIdx := sy - im.Rect.Min.Y
		switch im.SubsampleRatio {
		case image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440:
			cRowIdx >>= 1
		}
		cRowOff := cRowIdx * im.CStride
		dstRow := tmp[y*dw*3:]
		for x := 0; x < dw; x++ {
			x0, x1 := xStarts[x], xEnds[x]
			var sr, sg, sb uint32
			for sx := x0; sx < x1; sx++ {
				ix := b.Min.X + sx - im.Rect.Min.X
				yi := yRowOff + ix
				ci := cRowOff
				switch im.SubsampleRatio {
				case image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio440:
					ci += ix
				case image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420:
					ci += ix >> 1
				case image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410:
					ci += ix >> 2
				}
				r, g, bl := ycbcrToRGB(im.Y[yi], im.Cb[ci], im.Cr[ci])
				sr += uint32(r)
				sg += uint32(g)
				sb += uint32(bl)
			}
			cnt := uint32(x1 - x0)
			o := x * 3
			dstRow[o] = uint8(sr / cnt)
			dstRow[o+1] = uint8(sg / cnt)
			dstRow[o+2] = uint8(sb / cnt)
		}
	}
}

func horizRGBA(im *image.RGBA, b image.Rectangle, dw, sh int, xStarts, xEnds []int, tmp []uint8) {
	for y := 0; y < sh; y++ {
		srcRow := im.Pix[(b.Min.Y+y-im.Rect.Min.Y)*im.Stride:]
		xBase := (b.Min.X - im.Rect.Min.X) * 4
		dstRow := tmp[y*dw*3:]
		for x := 0; x < dw; x++ {
			x0, x1 := xStarts[x], xEnds[x]
			var sr, sg, sb uint32
			for sx := x0; sx < x1; sx++ {
				p := xBase + sx*4
				// RGBA is alpha-premultiplied; flatten onto white (255 - a)
				inv := uint32(255 - srcRow[p+3])
				sr += uint32(srcRow[p]) + inv
				sg += uint32(srcRow[p+1]) + inv
				sb += uint32(srcRow[p+2]) + inv
			}
			cnt := uint32(x1 - x0)
			o := x * 3
			dstRow[o] = uint8(sr / cnt)
			dstRow[o+1] = uint8(sg / cnt)
			dstRow[o+2] = uint8(sb / cnt)
		}
	}
}

func horizNRGBA(im *image.NRGBA, b image.Rectangle, dw, sh int, xStarts, xEnds []int, tmp []uint8) {
	for y := 0; y < sh; y++ {
		srcRow := im.Pix[(b.Min.Y+y-im.Rect.Min.Y)*im.Stride:]
		xBase := (b.Min.X - im.Rect.Min.X) * 4
		dstRow := tmp[y*dw*3:]
		for x := 0; x < dw; x++ {
			x0, x1 := xStarts[x], xEnds[x]
			var sr, sg, sb uint32
			for sx := x0; sx < x1; sx++ {
				p := xBase + sx*4
				a := uint32(srcRow[p+3])
				inv := 255 - a
				sr += (uint32(srcRow[p])*a)/255 + inv
				sg += (uint32(srcRow[p+1])*a)/255 + inv
				sb += (uint32(srcRow[p+2])*a)/255 + inv
			}
			cnt := uint32(x1 - x0)
			o := x * 3
			dstRow[o] = uint8(sr / cnt)
			dstRow[o+1] = uint8(sg / cnt)
			dstRow[o+2] = uint8(sb / cnt)
		}
	}
}

func horizPaletted(im *image.Paletted, b image.Rectangle, dw, sh int, xStarts, xEnds []int, tmp []uint8) {
	var pal [256][3]uint8
	for i, c := range im.Palette {
		if i >= 256 {
			break
		}
		r, g, bl, al := c.RGBA()
		pal[i] = [3]uint8{uint8((r + 0xffff - al) >> 8), uint8((g + 0xffff - al) >> 8), uint8((bl + 0xffff - al) >> 8)}
	}
	for y := 0; y < sh; y++ {
		srcRow := im.Pix[(b.Min.Y+y-im.Rect.Min.Y)*im.Stride:]
		xBase := b.Min.X - im.Rect.Min.X
		dstRow := tmp[y*dw*3:]
		for x := 0; x < dw; x++ {
			x0, x1 := xStarts[x], xEnds[x]
			var sr, sg, sb uint32
			for sx := x0; sx < x1; sx++ {
				c := pal[srcRow[xBase+sx]]
				sr += uint32(c[0])
				sg += uint32(c[1])
				sb += uint32(c[2])
			}
			cnt := uint32(x1 - x0)
			o := x * 3
			dstRow[o] = uint8(sr / cnt)
			dstRow[o+1] = uint8(sg / cnt)
			dstRow[o+2] = uint8(sb / cnt)
		}
	}
}

func horizGeneric(src image.Image, b image.Rectangle, dw, sh int, xStarts, xEnds []int, tmp []uint8) {
	for y := 0; y < sh; y++ {
		sy := b.Min.Y + y
		dstRow := tmp[y*dw*3:]
		for x := 0; x < dw; x++ {
			x0, x1 := xStarts[x], xEnds[x]
			var sr, sg, sb uint64
			for sx := x0; sx < x1; sx++ {
				r, g, bl, al := src.At(b.Min.X+sx, sy).RGBA()
				sr += uint64(r + 0xffff - al)
				sg += uint64(g + 0xffff - al)
				sb += uint64(bl + 0xffff - al)
			}
			cnt := uint64(x1 - x0)
			o := x * 3
			dstRow[o] = uint8((sr / cnt) >> 8)
			dstRow[o+1] = uint8((sg / cnt) >> 8)
			dstRow[o+2] = uint8((sb / cnt) >> 8)
		}
	}
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
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
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
