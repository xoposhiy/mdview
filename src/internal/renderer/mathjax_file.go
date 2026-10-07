package renderer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// mathJaxFileName is the name the embedded MathJax gets on disk. It carries a
// hash of the script, so a new mdview.exe with another MathJax never loads a
// file an older exe left behind.
func mathJaxFileName() string {
	sum := sha256.Sum256([]byte(mathJaxScript))
	return "mathjax-" + hex.EncodeToString(sum[:6]) + ".js"
}

// unpackMathJax makes sure dir holds the embedded MathJax and returns the
// file's path. A file that is already there is used as it is.
//
// The script is written to a temporary file and then renamed, so a second
// mdview doing the same at the same moment never sees half a file. Copies
// left by other versions of mdview are removed; a removal that fails (the
// file is in use) is ignored.
func unpackMathJax(dir string) (string, error) {
	name := mathJaxFileName()
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && fi.Size() == int64(len(mathJaxScript)) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "mathjax-*.tmp")
	if err != nil {
		return "", err
	}
	_, werr := tmp.WriteString(mathJaxScript)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		if werr != nil {
			return "", werr
		}
		return "", cerr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		// Another mdview may have put the same file in place first and still
		// hold it open, which makes the rename fail on Windows.
		if fi, serr := os.Stat(path); serr == nil && fi.Size() == int64(len(mathJaxScript)) {
			return path, nil
		}
		return "", err
	}
	removeOldMathJax(dir, name)
	return path, nil
}

// removeOldMathJax deletes the MathJax files in dir other than keep.
func removeOldMathJax(dir, keep string) {
	old, _ := filepath.Glob(filepath.Join(dir, "mathjax-*.js"))
	for _, p := range old {
		if filepath.Base(p) != keep {
			os.Remove(p)
		}
	}
}
