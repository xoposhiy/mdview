package renderer

import (
	"bytes"
	_ "embed"
	"html/template"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed assets/template.html
var pageTemplate string

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		ghtml.WithUnsafe(),
	),
)

var tmpl = template.Must(template.New("page").Parse(pageTemplate))

type pageData struct {
	Content   template.HTML
	DarkTheme bool
	ScrollY   int
}

var imgSrcRe = regexp.MustCompile(`src="([^"]*)"`)

// resolveImageSrcs rewrites relative src= attributes to file:/// URLs so that
// WebView2's SetHtml (which has no base URL) can load local images.
func resolveImageSrcs(html, baseDir string) string {
	if baseDir == "" {
		return html
	}
	return imgSrcRe.ReplaceAllStringFunc(html, func(m string) string {
		src := m[5 : len(m)-1] // strip src=" and trailing "
		switch {
		case strings.HasPrefix(src, "http://"),
			strings.HasPrefix(src, "https://"),
			strings.HasPrefix(src, "file://"),
			strings.HasPrefix(src, "data:"):
			return m // already absolute, leave untouched
		}
		abs := src
		if !filepath.IsAbs(src) {
			abs = filepath.Join(baseDir, src)
		}
		url := "file:///" + strings.ReplaceAll(abs, "\\", "/")
		return `src="` + url + `"`
	})
}

// Render converts Markdown content to a complete HTML page (light theme).
func Render(content []byte) (string, error) {
	return RenderWithTheme(content, false, 0, "")
}

// RenderWithTheme converts Markdown content to a complete HTML page.
// baseDir is the directory of the source file; used to resolve relative image paths.
func RenderWithTheme(content []byte, dark bool, scrollY int, baseDir string) (string, error) {
	var body bytes.Buffer
	if err := md.Convert(content, &body); err != nil {
		return "", err
	}

	data := pageData{
		Content:   template.HTML(resolveImageSrcs(body.String(), baseDir)),
		DarkTheme: dark,
		ScrollY:   scrollY,
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
