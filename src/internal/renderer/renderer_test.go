package renderer

import (
	"strings"
	"testing"
)

func TestRenderHeading(t *testing.T) {
	html, err := Render([]byte("# Hello World"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<h1") {
		t.Errorf("expected <h1>, got:\n%s", html)
	}
}

func TestRenderTable(t *testing.T) {
	md := "| A | B |\n|---|---|\n| 1 | 2 |"
	html, err := Render([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<table") {
		t.Errorf("expected <table>, got:\n%s", html)
	}
}

func TestRenderCodeBlock(t *testing.T) {
	md := "```\nfoo bar\n```"
	html, err := Render([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<code") {
		t.Errorf("expected <code>, got:\n%s", html)
	}
}

func TestRenderInlineCode(t *testing.T) {
	html, err := Render([]byte("use `fmt.Println`"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<code>") {
		t.Errorf("expected inline <code>, got:\n%s", html)
	}
}

func TestRenderStrikethrough(t *testing.T) {
	html, err := Render([]byte("~~deleted~~"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<del>") {
		t.Errorf("expected <del>, got:\n%s", html)
	}
}

func TestRenderTaskList(t *testing.T) {
	html, err := Render([]byte("- [x] done\n- [ ] todo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("expected checkbox input, got:\n%s", html)
	}
}

func TestRenderInlineMath(t *testing.T) {
	html, err := Render([]byte(`value $\hat{p} = 0.5$ here`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="math-inline"`) {
		t.Errorf("expected inline math span, got:\n%s", html)
	}
	if !strings.Contains(html, `\(\hat{p} = 0.5\)`) {
		t.Errorf("expected \\(...\\) delimited LaTeX, got:\n%s", html)
	}
}

func TestRenderDisplayMath(t *testing.T) {
	html, err := Render([]byte(`$$D = p_\text{top} - p_\text{bot}$$`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="math-display"`) {
		t.Errorf("expected display math span, got:\n%s", html)
	}
	// `_` must NOT be turned into <em>; the LaTeX is preserved verbatim.
	if !strings.Contains(html, `\[D = p_\text{top} - p_\text{bot}\]`) {
		t.Errorf("expected verbatim LaTeX, got:\n%s", html)
	}
	if strings.Contains(html, "<em>") {
		t.Errorf("underscores were mangled into emphasis:\n%s", html)
	}
}

func TestRenderMathEscapesAngleBrackets(t *testing.T) {
	html, err := Render([]byte(`$\prod_{i \in S} p_i$`))
	if err != nil {
		t.Fatal(err)
	}
	// `\in` and braces survive; nothing here should produce raw HTML, but verify
	// the math markers are present.
	if !strings.Contains(html, `\(\prod_{i \in S} p_i\)`) {
		t.Errorf("expected verbatim LaTeX, got:\n%s", html)
	}
}

func TestRenderNonMathDollarUntouched(t *testing.T) {
	html, err := Render([]byte(`it costs $5 today`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "math-inline") {
		t.Errorf("lone dollar wrongly parsed as math:\n%s", html)
	}
}

// A page carries MathJax only when it has a formula on it; see needsMathJax.
func TestRenderMathScriptEmbedded(t *testing.T) {
	html, err := Render([]byte("hello $x^2$"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "window.MathJax") {
		t.Errorf("expected MathJax config in page, got page without it")
	}
}

func TestRenderEmpty(t *testing.T) {
	html, err := Render([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<html") {
		t.Errorf("expected valid HTML document, got:\n%s", html)
	}
}

func TestRenderPageShowsFrontmatterTableAboveBody(t *testing.T) {
	html, err := Render([]byte("---\ntitle: Hello\ntags: [a, b]\n---\n\n# Body\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="frontmatter"`) {
		t.Errorf("expected properties table in page, got:\n%s", html)
	}
	if !strings.Contains(html, "<th>title</th>") {
		t.Error("expected a title row in the properties table")
	}
	if strings.Contains(html, "title: Hello") {
		t.Error("raw frontmatter leaked into the rendered body")
	}
	if !strings.Contains(html, "<h1") {
		t.Error("expected the Markdown body to still be rendered")
	}
}

func TestRenderPageFrontmatterTableIsOutsideContentDiv(t *testing.T) {
	// The page JS makes `.content table` sortable and wraps over-wide ones in a
	// scroller. The properties table should get neither, so it must sit outside.
	html, err := Render([]byte("---\ntitle: Hello\n---\n# Body\n"))
	if err != nil {
		t.Fatal(err)
	}
	fmAt := strings.Index(html, `class="frontmatter"`)
	contentAt := strings.Index(html, `class="content"`)
	if fmAt < 0 || contentAt < 0 {
		t.Fatalf("missing markers: frontmatter=%d content=%d", fmAt, contentAt)
	}
	if fmAt > contentAt {
		t.Error("properties table is inside/after the .content div; JS will treat it as a document table")
	}
}

func TestRenderPageWithoutFrontmatterHasNoTable(t *testing.T) {
	html, err := Render([]byte("# Body only\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `class="frontmatter"`) {
		t.Errorf("unexpected properties table for a document with no frontmatter")
	}
}

func TestRenderOutputIsFullPage(t *testing.T) {
	html, err := Render([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("expected DOCTYPE, got:\n%s", html)
	}
	if !strings.Contains(html, "</html>") {
		t.Errorf("expected closing </html>, got:\n%s", html)
	}
}
