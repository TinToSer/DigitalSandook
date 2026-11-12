package thumb

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// jpegWithOrientation encodes a w x h JPEG and splices in an EXIF APP1 segment.
func jpegWithOrientation(w, h, orient int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(orient), 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), append(seg, payload...)...), b[2:]...)
}

func TestThumbOrientationAndSize(t *testing.T) {
	b := jpegWithOrientation(800, 400, 6) // landscape pixels, "rotate 90 CW" tag
	out, err := Make(bytes.NewReader(b), ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 160 || cfg.Height != 320 {
		t.Fatalf("thumb is %dx%d, want 160x320 (rotated, longest edge %d)", cfg.Width, cfg.Height, Edge)
	}
}

func TestThumbRejectsDecompressionBomb(t *testing.T) {
	// a valid PNG header that claims 100000 x 100000 pixels (40 GB decoded)
	var png bytes.Buffer
	png.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 100000)
	binary.BigEndian.PutUint32(ihdr[4:], 100000)
	ihdr[8], ihdr[9] = 8, 6
	binary.Write(&png, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	png.Write(chunk)
	binary.Write(&png, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	if _, err := Make(bytes.NewReader(png.Bytes()), ".png"); err != ErrNoThumb {
		t.Fatalf("expected ErrNoThumb for a decompression bomb, got %v", err)
	}
}

func TestExifOrientationShortInput(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("I"), []byte("II*\x00"), []byte("MM\x00*\x00\x00\x00\xff")} {
		if got := exifOrientation(b); got != 1 {
			t.Fatalf("exifOrientation(%q) = %d", b, got)
		}
	}
}
