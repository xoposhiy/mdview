package renderer

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Math support: recognise `$...$` and `\(...\)` (inline), `$$...$$` and
// `\[...\]` (display), and emit raw nodes so goldmark does not apply Markdown
// formatting to the LaTeX (e.g. `_`, `\`, `%`). The LaTeX is wrapped in
// MathJax's default delimiters (`\(...\)` / `\[...\]`) and typeset client-side
// by the embedded MathJax script.

var kindMath = gast.NewNodeKind("Math")

type mathNode struct {
	gast.BaseInline
	Display bool
	Value   []byte
}

func (n *mathNode) Kind() gast.NodeKind { return kindMath }

func (n *mathNode) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type mathParser struct{}

func (p *mathParser) Trigger() []byte { return []byte{'$'} }

func (p *mathParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 {
		return nil
	}

	display := line[1] == '$'
	delimLen := 1
	if display {
		delimLen = 2
	}

	rest := line[delimLen:]
	closeIdx := -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '\\': // escaped char (e.g. \$, \%) — skip the next byte
			i++
		case '$':
			if display {
				if i+1 < len(rest) && rest[i+1] == '$' {
					closeIdx = i
				}
			} else {
				closeIdx = i
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		return nil
	}

	content := rest[:closeIdx]
	if len(content) == 0 {
		return nil
	}
	// For inline math, require non-space adjacent to the delimiters so prose
	// like "cost $5 or $10" is not mistaken for math.
	if !display && (content[0] == ' ' || content[len(content)-1] == ' ') {
		return nil
	}

	block.Advance(delimLen + closeIdx + delimLen)

	n := &mathNode{Display: display}
	n.Value = make([]byte, len(content))
	copy(n.Value, content)
	return n
}

type mathRenderer struct{}

func (r *mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMath, r.render)
	reg.Register(kindMathBlock, r.renderBlock)
}

func (r *mathRenderer) render(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	n := node.(*mathNode)
	open, close := `\(`, `\)`
	class := "math-inline"
	if n.Display {
		open, close = `\[`, `\]`
		class = "math-display"
	}
	_, _ = w.WriteString(`<span class="` + class + `">`)
	_, _ = w.WriteString(open)
	// Escape HTML metacharacters; MathJax reads the (decoded) text content.
	template.HTMLEscape(w, n.Value)
	_, _ = w.WriteString(close)
	_, _ = w.WriteString(`</span>`)
	return gast.WalkContinue, nil
}

// mathBlockNode holds the raw LaTeX content of a display block: $$\n...\n$$
// or \[\n...\n\].
var kindMathBlock = gast.NewNodeKind("MathBlock")

type mathBlockNode struct {
	gast.BaseBlock
	Value []byte
	// closed is set when the opening line already held the whole formula.
	closed bool
}

func (n *mathBlockNode) Kind() gast.NodeKind { return kindMathBlock }
func (n *mathBlockNode) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

// mathBlockParser handles fenced display math: $$ on its own line, content, $$ on its own line.
type mathBlockParser struct{}

func (p *mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (p *mathBlockParser) Open(parent gast.Node, reader text.Reader, pc parser.Context) (gast.Node, parser.State) {
	line, _ := reader.PeekLine()
	if string(bytes.TrimRight(line, "\r\n")) != "$$" {
		return nil, parser.NoChildren
	}
	// Do not advance — the framework calls AdvanceLine() after Open returns.
	return &mathBlockNode{}, parser.NoChildren
}

func (p *mathBlockParser) Continue(node gast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if string(bytes.TrimRight(line, "\r\n")) == "$$" {
		// Advance past "$$" but not the newline; AdvanceLine() handles the rest.
		// Without this, openBlocks re-triggers on the same "$" line and opens a new block.
		reader.Advance(len(bytes.TrimRight(line, "\r\n")))
		return parser.Close
	}
	node.(*mathBlockNode).Value = append(node.(*mathBlockNode).Value, line...)
	// Do not advance — the framework calls AdvanceLine() after Continue returns.
	return parser.Continue | parser.NoChildren
}

func (p *mathBlockParser) Close(node gast.Node, reader text.Reader, pc parser.Context) {}
func (p *mathBlockParser) CanInterruptParagraph() bool { return true }
func (p *mathBlockParser) CanAcceptIndentedLine() bool { return false }

func (r *mathRenderer) renderBlock(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	n := node.(*mathBlockNode)
	_, _ = w.WriteString(`<div class="math-display">`)
	_, _ = w.WriteString(`\[`)
	template.HTMLEscape(w, bytes.TrimRight(n.Value, "\r\n"))
	_, _ = w.WriteString(`\]`)
	_, _ = w.WriteString(`</div>`)
	return gast.WalkContinue, nil
}

// LaTeX's own delimiters. In Markdown `\[` and `\(` are backslash escapes, so
// these parsers must claim them before goldmark drops the backslash.
var (
	displayOpen  = []byte(`\[`)
	displayClose = []byte(`\]`)
)

// displayMathBlockParser handles display math in LaTeX delimiters: `\[` on its
// own line, the formula, then a line that ends with `\]`; or `\[ ... \]` on
// one line. A line that starts with `\[` but closes it in the middle, such as
// `\[1\] see the reference`, is prose with escaped brackets and stays prose.
type displayMathBlockParser struct{}

func (p *displayMathBlockParser) Trigger() []byte { return []byte{'\\'} }

func (p *displayMathBlockParser) Open(parent gast.Node, reader text.Reader, pc parser.Context) (gast.Node, parser.State) {
	line, _ := reader.PeekLine()
	rest, ok := bytes.CutPrefix(bytes.TrimSpace(line), displayOpen)
	if !ok {
		return nil, parser.NoChildren
	}
	if len(bytes.TrimSpace(rest)) == 0 {
		return &mathBlockNode{}, parser.NoChildren
	}
	formula, ok := bytes.CutSuffix(rest, displayClose)
	if !ok || bytes.Contains(formula, displayClose) {
		return nil, parser.NoChildren
	}
	return &mathBlockNode{Value: bytes.Clone(formula), closed: true}, parser.NoChildren
}

func (p *displayMathBlockParser) Continue(node gast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*mathBlockNode)
	if n.closed {
		return parser.Close
	}
	line, _ := reader.PeekLine()
	content := bytes.TrimRight(line, " \t\r\n")
	if formula, ok := bytes.CutSuffix(content, displayClose); ok {
		n.Value = append(n.Value, formula...)
		// Consume the line, or goldmark offers its `\` to openBlocks again.
		reader.Advance(len(content))
		return parser.Close
	}
	n.Value = append(n.Value, line...)
	return parser.Continue | parser.NoChildren
}

func (p *displayMathBlockParser) Close(node gast.Node, reader text.Reader, pc parser.Context) {}
func (p *displayMathBlockParser) CanInterruptParagraph() bool { return true }
func (p *displayMathBlockParser) CanAcceptIndentedLine() bool { return false }

// inlineMathParser handles inline math in LaTeX delimiters, `\(...\)`, on a
// single line.
type inlineMathParser struct{}

func (p *inlineMathParser) Trigger() []byte { return []byte{'\\'} }

func (p *inlineMathParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 || line[1] != '(' {
		return nil
	}
	rest := line[2:]
	closeIdx := -1
	for i := 0; i+1 < len(rest); i++ {
		if rest[i] != '\\' {
			continue
		}
		if rest[i+1] == ')' {
			closeIdx = i
			break
		}
		i++ // a LaTeX command or `\\`: its next byte cannot start `\)`
	}
	if closeIdx <= 0 {
		return nil
	}
	block.Advance(2 + closeIdx + 2)
	return &mathNode{Value: bytes.Clone(rest[:closeIdx])}
}

type mathExtension struct{}

// Math is a goldmark extension that renders LaTeX math via MathJax.
var Math = &mathExtension{}

func (e *mathExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&mathParser{}, 500),
		util.Prioritized(&inlineMathParser{}, 500),
	))
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(&mathBlockParser{}, 500),
		util.Prioritized(&displayMathBlockParser{}, 500),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&mathRenderer{}, 500),
	))
}
