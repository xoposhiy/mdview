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
