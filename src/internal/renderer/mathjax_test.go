package renderer

import (
	"strings"
	"testing"

	"mdview/internal/config"
)

const (
	docWithMath    = "# hi\n\nEuler wrote $e^{i\\pi} + 1 = 0$.\n"
	docWithoutMath = "# hi\n\nNo formulas here, just prose and `code`.\n"
)

// linkTag is what a page that links MathJax carries instead of the script.
const linkTag = `<script defer src=`

func TestPageWithoutMathHasNoMathJax(t *testing.T) {
	for name, render := range map[string]func([]byte, config.Config, int, string) (string, error){
		"standalone": RenderWithTheme,
		"linked":     RenderLinked(`C:\mdview\mathjax.js`),
	} {
		t.Run(name, func(t *testing.T) {
			html, err := render([]byte(docWithoutMath), config.Default(), 0, "")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(html, linkTag) {
				t.Error("page has no formulas but links MathJax")
			}
			if strings.Contains(html, "window.MathJax") {
				t.Error("page has no formulas but still configures MathJax")
			}
			if len(html) > 100_000 {
				t.Errorf("page is %d bytes; MathJax looks inlined", len(html))
			}
		})
	}
}

func TestStandalonePageWithMathInlinesMathJax(t *testing.T) {
	html, err := RenderWithTheme([]byte(docWithMath), config.Default(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, linkTag) {
		t.Error("standalone page links MathJax; it must inline it")
	}
	if len(html) < len(mathJaxScript) {
		t.Errorf("page is %d bytes, smaller than the %d-byte script it should contain", len(html), len(mathJaxScript))
	}
}

func TestLinkedPageWithMathReferencesTheFile(t *testing.T) {
	render := RenderLinked(`C:\Program Files\mdview\mathjax.js`)
	html, err := render([]byte(docWithMath), config.Default(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := `<script defer src="file:///C:/Program%20Files/mdview/mathjax.js"></script>`
	if !strings.Contains(html, want) {
		t.Errorf("page does not link MathJax as %s", want)
	}
	// The whole point: the script itself is no longer in the page.
	if len(html) > 100_000 {
		t.Errorf("page is %d bytes; MathJax looks inlined as well", len(html))
	}
}

func TestLinkedPageWithoutAFileInlines(t *testing.T) {
	render := RenderLinked("")
	html, err := render([]byte(docWithMath), config.Default(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, linkTag) {
		t.Error("no mathjax.js to link, but the page links one anyway")
	}
	if len(html) < len(mathJaxScript) {
		t.Error("no mathjax.js to link, and the script was not inlined either")
	}
}

func TestNeedsMathJax(t *testing.T) {
	cases := []struct {
		name string
		html string
		want bool
	}{
		{"plain prose", "<p>nothing to see</p>", false},
		{"inline math", `<p><span class="math">\(x\)</span></p>`, true},
		{"display math", `<div class="math-display">\[x\]</div>`, true},
		// A false positive: MathJax skips <code>, but the script still loads.
		// Accepted, because the other way round loses a formula.
		{"delimiter in code", "<pre><code>printf(\"\\\\(\")</code></pre>", true},
	}
	for _, c := range cases {
		if got := needsMathJax(c.html); got != c.want {
			t.Errorf("%s: needsMathJax(%q) = %v, want %v", c.name, c.html, got, c.want)
		}
	}
}

// Raw HTML is passed through untouched, so a document can carry MathJax
// delimiters the Math extension never saw. The page must still load MathJax.
func TestRawHTMLMathIsDetected(t *testing.T) {
	html, err := RenderWithTheme([]byte(`<div>\[ x^2 \]</div>`), config.Default(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "window.MathJax") {
		t.Error("formula written as raw HTML, but the page does not load MathJax")
	}
}

func TestFileURL(t *testing.T) {
	cases := []struct{ path, want string }{
		{`C:\work\mdview\mathjax.js`, "file:///C:/work/mdview/mathjax.js"},
		{`C:\Program Files\mdview\mathjax.js`, "file:///C:/Program%20Files/mdview/mathjax.js"},
		{`C:\заметки\mathjax.js`, "file:///C:/%D0%B7%D0%B0%D0%BC%D0%B5%D1%82%D0%BA%D0%B8/mathjax.js"},
	}
	for _, c := range cases {
		if got := fileURL(c.path); got != c.want {
			t.Errorf("fileURL(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
