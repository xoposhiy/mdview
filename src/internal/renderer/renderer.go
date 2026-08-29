package renderer

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/url"
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
	HasMath     bool
	MathJax     template.JS
	MathJaxSrc  template.URL
}

// needsMathJax reports whether html holds anything MathJax would typeset.
//
// The Math extension emits `\(`/`\[` for every `$...$` it finds, and a document
// can carry the same delimiters inside raw HTML, so looking for them in the
// finished page catches both without the parser having to report anything.
//
// It errs towards yes: the delimiters can also sit in a code block, which
// MathJax skips but which still loads the script. That costs a page the old
// behaviour, where a miss would leave a formula unrendered.
func needsMathJax(html string) bool {
	return strings.Contains(html, `\(`) || strings.Contains(html, `\[`)
}

// fileURL turns a local path into a file:// URL. The path is percent-encoded,
// so a program installed under "C:\Program Files" still loads its assets.
func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // "C:/dir/f" -> "/C:/dir/f", so the URL gets file:///
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
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
//
// The page depends on no other file — MathJax is inlined — so it can be opened
// on another machine. That is what --export and "Save as HTML" need.
func RenderWithTheme(content []byte, prefs config.Config, scrollY int, baseDir string) (string, error) {
	return render(content, prefs, scrollY, baseDir, "")
}

// RenderLinked returns a render function that links MathJax from mathJaxPath
// instead of inlining it.
//
// Inlining puts 2 MB of script in every page. WebView2 then compiles it again
// on every navigation, and tearing the old page's copy down blocks the new
// page from starting: about 1.5 s per left/right keypress, measured, on
// documents with no formulas in them at all. A linked script is loaded from
// the same URL every time, so that cost is paid once.
//
// The pages this produces only work on the machine that wrote them, so use it
// for what the viewer displays, never for what it writes to disk. An empty
// mathJaxPath falls back to inlining, which is what keeps a copy of mdview.exe
// with no mathjax.js beside it able to show formulas.
func RenderLinked(mathJaxPath string) func([]byte, config.Config, int, string) (string, error) {
	src := ""
	if mathJaxPath != "" {
		src = fileURL(mathJaxPath)
	}
	return func(content []byte, prefs config.Config, scrollY int, baseDir string) (string, error) {
		return render(content, prefs, scrollY, baseDir, src)
	}
}

// render is the body shared by RenderWithTheme and RenderLinked. mathJaxSrc is
// the URL to load MathJax from, or "" to inline the script.
func render(content []byte, prefs config.Config, scrollY int, baseDir, mathJaxSrc string) (string, error) {
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

	html := resolveImageSrcs(body.String(), baseDir)

	data := pageData{
		Frontmatter: frontmatter,
		Content:     template.HTML(html),
		Theme:       prefs.Theme,
		Font:        prefs.Font,
		FontSize:    prefs.FontSize,
		ScrollY:     scrollY,
		HasMath:     needsMathJax(html) || needsMathJax(string(frontmatter)),
	}
	// A page with no formulas gets no MathJax at all — not the script, not the
	// configuration. Most documents are in that case, and it is the difference
	// between a left/right keypress taking 150 ms and taking 50 ms.
	if data.HasMath {
		if mathJaxSrc == "" {
			data.MathJax = template.JS(mathJaxScript)
		}
		data.MathJaxSrc = template.URL(mathJaxSrc)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
