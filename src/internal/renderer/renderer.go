package renderer

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed assets/template.html
var pageTemplate string

//go:embed assets/style.css
var styleCSS string

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
	Style   template.HTML
	Content template.HTML
}

// Render converts Markdown content to a complete HTML page.
func Render(content []byte) (string, error) {
	var body bytes.Buffer
	if err := md.Convert(content, &body); err != nil {
		return "", err
	}

	data := pageData{
		Style:   template.HTML(styleCSS),
		Content: template.HTML(body.String()),
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
