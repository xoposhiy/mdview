package renderer

import (
	"bytes"
	_ "embed"
	"html/template"
	"path/filepath"
	"regexp"
	"strings"

	"mdview/internal/config"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed assets/template.html
var pageTemplate string

//go:embed assets/mathjax.js
var mathJaxScript string

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM, Math, Wikilink),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		ghtml.WithUnsafe(),
	),
)

var tmpl = template.Must(template.New("page").Parse(pageTemplate))

type pageData struct {
	Frontmatter template.HTML
	Content     template.HTML
	Theme       string
	Font        string
	FontSize    int
	ScrollY     int
	MathJax     template.JS
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

// Render converts Markdown content to a complete HTML page (default prefs).
func Render(content []byte) (string, error) {
	return RenderWithTheme(content, config.Default(), 0, "")
}

// RenderWithTheme converts Markdown content to a complete HTML page styled
// according to prefs (theme, font, size). baseDir is the directory of the
// source file; used to resolve relative image paths. Empty/zero pref fields
// fall back to defaults so partial or stray callers still produce a valid page.
func RenderWithTheme(content []byte, prefs config.Config, scrollY int, baseDir string) (string, error) {
	def := config.Default()
	if prefs.Theme == "" {
		prefs.Theme = def.Theme
	}
	if prefs.Font == "" {
		prefs.Font = def.Font
	}
	if prefs.FontSize == 0 {
		prefs.FontSize = def.FontSize
	}

	frontmatter, markdown := extractFrontmatter(content)

	// baseDir travels to the wikilink parser through the parse context: md is a
	// package-level singleton, so it cannot carry per-document state.
	ctx := parser.NewContext()
	ctx.Set(baseDirKey, baseDir)

	var body bytes.Buffer
	if err := md.Convert(markdown, &body, parser.WithContext(ctx)); err != nil {
		return "", err
	}

	data := pageData{
		Frontmatter: frontmatter,
		Content:     template.HTML(resolveImageSrcs(body.String(), baseDir)),
		Theme:       prefs.Theme,
		Font:        prefs.Font,
		FontSize:    prefs.FontSize,
		ScrollY:     scrollY,
		MathJax:     template.JS(mathJaxScript),
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
