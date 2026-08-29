package imgdecode

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
)

func TestIsImage(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".png", true}, {".PNG", true},
		{".jpg", true}, {".jpeg", true},
		{".bmp", true}, {".gif", true},
		{".md", false}, {".webp", false}, {"", false},
	}
	for _, c := range cases {
		if got := IsImage(c.ext); got != c.want {
			t.Errorf("IsImage(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	return img
}

func TestDecode(t *testing.T) {
	dir := t.TempDir()
	src := testImage()

	encoders := map[string]func(*bytes.Buffer) error{
		"pic.png": func(b *bytes.Buffer) error { return png.Encode(b, src) },
		"pic.jpg": func(b *bytes.Buffer) error { return jpeg.Encode(b, src, nil) },
		"pic.bmp": func(b *bytes.Buffer) error { return bmp.Encode(b, src) },
		"pic.gif": func(b *bytes.Buffer) error { return gif.Encode(b, src, nil) },
	}

	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := encode(&buf); err != nil {
				t.Fatalf("encode: %v", err)
			}
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
				t.Fatalf("write: %v", err)
			}

			img, err := Decode(path)
			if err != nil {
				t.Fatalf("Decode(%q): %v", path, err)
			}
			b := img.Bounds()
			if b.Dx() != 4 || b.Dy() != 3 {
				t.Errorf("Decode(%q) size = %dx%d, want 4x3", path, b.Dx(), b.Dy())
			}
		})
	}
}

func TestDecodeMissingFile(t *testing.T) {
	if _, err := Decode(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Error("Decode of a missing file: want error, got nil")
	}
}
