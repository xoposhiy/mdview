package renderer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
)

func renderMathOnly(src string) string {
	md := goldmark.New(goldmark.WithExtensions(Math))
	var buf bytes.Buffer
	_ = md.Convert([]byte(src), &buf)
	return buf.String()
}

func TestMathBlock(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
		// text that must NOT appear in the LaTeX output (confirms surrounding prose is intact)
		absent []string
	}{
		{
			name: "single-line block",
			src:  "$$\n\\frac{9}{10}\n$$\n",
			want: []string{`\[`, `\frac{9}{10}`, `\]`},
		},
		{
			name: "multi-line block",
			src:  "$$\na=1\nb=2\n$$\n",
			want: []string{`\[`, `a=1`, `b=2`, `\]`},
		},
		{
			name: "block interrupts paragraph",
			src:  "compute\n$$\n1+1\n$$\nresult\n",
			want: []string{`\[1+1\]`, "compute", "result"},
		},
		{
			name: "surrounding prose is not swallowed",
			src:  "before\n\n$$\nx\n$$\n\nafter\n",
			want: []string{"before", `\[x\]`, "after"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := renderMathOnly(c.src)
			for _, w := range c.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in:\n%s", w, html)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(html, a) {
					t.Errorf("should not contain %q in:\n%s", a, html)
				}
			}
		})
	}
}

// LaTeX's own delimiters: `\[...\]` for display math, `\(...\)` for inline.
// In Markdown `\[` and `\(` are escapes, so without the Math extension the
// backslashes vanish and MathJax never sees the formula.
func TestBackslashMath(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		want   []string
		absent []string
	}{
		{
			name: "display block on its own lines",
			src:  "\\[\n\\max U(\\text{current task})\n\\]\n",
			want: []string{`<div class="math-display">\[\max U(\text{current task})\]</div>`},
		},
		{
			name:   "display block keeps LaTeX intact",
			src:    "\\[\n\\max \\mathbb{E}\\left[\\sum_{t=0}^{T} U(x_t)\\right].\n\\]\n",
			want:   []string{`\left[\sum_{t=0}^{T} U(x_t)\right].`},
			absent: []string{"<em>"},
		},
		{
			name: "multi-line display block",
			src:  "\\[\na=1\nb=2\n\\]\n",
			want: []string{`\[a=1`, `b=2\]`},
		},
		{
			name: "closing delimiter after the last line of the formula",
			src:  "\\[\na=1 \\]\n",
			want: []string{`\[a=1 \]`},
		},
		{
			name: "display block on one line",
			src:  "\\[ x^2 \\]\n",
			want: []string{`<div class="math-display">\[ x^2 \]</div>`},
		},
		{
			name: "display block interrupts paragraph",
			src:  "compute\n\\[\n1+1\n\\]\nresult\n",
			want: []string{"compute", `\[1+1\]`, "result"},
		},
		{
			name:   "escaped brackets in prose stay prose",
			src:    "\\[1\\] see the reference\n",
			want:   []string{"[1] see the reference"},
			absent: []string{"math-display"},
		},
		{
			name: "inline math",
			src:  "where \\(x_1 + y_1\\) holds\n",
			want: []string{`where <span class="math-inline">\(x_1 + y_1\)</span> holds`},
		},
		{
			name: "inline math with a LaTeX line break",
			src:  "\\(a \\\\ b\\)\n",
			want: []string{`\(a \\ b\)`},
		},
		{
			name:   "unclosed escaped parenthesis stays prose",
			src:    "a \\(b c\n",
			want:   []string{"a (b c"},
			absent: []string{"math-inline"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := renderMathOnly(c.src)
			for _, w := range c.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in:\n%s", w, html)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(html, a) {
					t.Errorf("should not contain %q in:\n%s", a, html)
				}
			}
		})
	}
}
