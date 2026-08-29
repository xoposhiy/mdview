// Package imgdecode decodes the raster image formats mdview can view
// (PNG, JPEG, BMP, GIF) and answers whether a given file extension is one
// of them.
package imgdecode

import (
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder (first frame only)
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp" // register BMP decoder
)

// Exts lists the file extensions mdview treats as viewable images, lower
// case and including the leading dot.
var Exts = []string{".png", ".jpg", ".jpeg", ".bmp", ".gif"}

// IsImage reports whether ext (as returned by filepath.Ext) names a
// supported image format.
func IsImage(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range Exts {
		if ext == e {
			return true
		}
	}
	return false
}

// Decode reads and decodes the image at path. The concrete format is
// detected from the file's contents, not its extension.
func Decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return img, nil
}
