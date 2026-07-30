package viewer

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type linkKind int

const (
	linkInPage linkKind = iota
	linkMarkdown
	linkExternal
)

// schemeRe matches a URL scheme. The scheme must be at least two characters
// long so that a Windows absolute path such as "C:/docs/x.md" is not mistaken
// for a "C:" scheme.
var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]+:`)

// classifyLink decides what should happen when the user clicks a link whose raw
// href attribute is href. baseDir is the directory of the document being
// viewed; relative local paths resolve against it.
//
// The returned target is the href unchanged for in-page anchors and for URLs
// with a scheme, and an absolute filesystem path for local files.
func classifyLink(href, baseDir string) (linkKind, string) {
	if href == "" || strings.HasPrefix(href, "#") {
		return linkInPage, href
	}
	if schemeRe.MatchString(href) {
		return linkExternal, href
	}

	path := href
	if i := strings.IndexAny(path, "#?"); i >= 0 {
		path = path[:i]
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)

	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return linkMarkdown, path
	}
	return linkExternal, path
}
