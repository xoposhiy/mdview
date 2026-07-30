package renderer

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Wikilink support: recognise Obsidian's `[[target]]` and `[[target|label]]`,
// resolve the target against the document's directory and the vault root, and
// emit a plain anchor. Emitting an ordinary <a> is what makes the rest of the
// pipeline work untouched: the viewer's classifyLink resolves the href, and
// Viewer.open handles history and the "Not found" toast.

// vaultRoot walks up from dir and returns the first ancestor (dir itself
// included) that contains a .obsidian directory — the marker Obsidian itself
// uses for a vault root. It returns "" when no ancestor carries the marker.
//
// This is what lets a wikilink written relative to the vault root resolve
// without any configuration: a link in vault/daily/today.md to
// [[projects/spec]] means vault/projects/spec.md.
func vaultRoot(dir string) string {
	for d := dir; ; {
		info, err := os.Stat(filepath.Join(d, ".obsidian"))
		if err == nil && info.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d { // volume root
			return ""
		}
		d = parent
	}
}

// splitFragment splits a wikilink target at its first "#". The returned
// fragment keeps the "#" so it can be appended to an href unchanged.
func splitFragment(target string) (path, fragment string) {
	if i := strings.IndexByte(target, '#'); i >= 0 {
		return target[:i], target[i:]
	}
	return target, ""
}

func hasMarkdownExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// resolveWikilink turns a wikilink target into an href and reports whether the
// file it names exists.
//
// Candidates are tried verbatim and then with ".md" appended, first under
// baseDir and then under the vault root. The first existing file wins, so a
// note beside the document beats a same-named note at the vault root. Trying
// the target verbatim first is what makes [[diagram.png]] resolve to the image
// while [[v1.2 release notes]] — whose extension is meaningless — still finds
// its .md file, with no extension whitelist to maintain.
//
// When nothing exists the link is reported broken and the href is the ".md"
// candidate under baseDir: the file the author most likely meant, and the name
// the viewer's "Not found" toast will show.
//
// An empty baseDir means there is nothing to stat against — the href stays
// relative and the link is never reported broken, so that callers passing no
// directory do not get every link dimmed.
func resolveWikilink(target, baseDir string) (string, bool) {
	path, fragment := splitFragment(target)
	if path == "" {
		return fragment, true // [[#heading]] — an in-page anchor
	}

	candidates := []string{path}
	if !hasMarkdownExt(path) {
		candidates = append(candidates, path+".md")
	}
	// The href used when nothing exists, and the only href available without a
	// baseDir: always the Markdown spelling.
	fallback := candidates[len(candidates)-1]

	if baseDir == "" {
		return filepath.ToSlash(fallback) + fragment, true
	}

	bases := []string{baseDir}
	if root := vaultRoot(baseDir); root != "" && root != baseDir {
		bases = append(bases, root)
	}
	for _, base := range bases {
		for _, candidate := range candidates {
			abs := filepath.Join(base, candidate)
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				return abs + fragment, true
			}
		}
	}
	return filepath.Join(baseDir, fallback) + fragment, false
}

// splitAlias splits the inside of a wikilink into the link target and the text
// to display. The alias is everything after the first "|", as in Obsidian; with
// no alias, or an empty one, the target is shown as written.
func splitAlias(inner string) (target, label string) {
	target, label, found := strings.Cut(inner, "|")
	target = strings.TrimSpace(target)
	if !found {
		return target, target
	}
	if label = strings.TrimSpace(label); label == "" {
		label = target
	}
	return target, label
}

// baseDirKey carries the document's directory from RenderWithTheme down to the
// parser, which needs it to resolve targets against the filesystem. The md
// instance is a package-level singleton, so the value cannot live on it.
var baseDirKey = parser.NewContextKey()

func baseDirFromContext(pc parser.Context) string {
	dir, _ := pc.Get(baseDirKey).(string)
	return dir
}

var kindWikilink = gast.NewNodeKind("Wikilink")

type wikilinkNode struct {
	gast.BaseInline
	Label  string
	Href   string
	Broken bool // no file exists at Href
}

func (n *wikilinkNode) Kind() gast.NodeKind { return kindWikilink }

func (n *wikilinkNode) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type wikilinkParser struct{}

func (p *wikilinkParser) Trigger() []byte { return []byte{'['} }

func (p *wikilinkParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	line, _ := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	rest := line[2:]
	end := bytes.Index(rest, []byte("]]"))
	if end < 0 {
		return nil // unclosed on this line: leave the text alone
	}

	target, label := splitAlias(string(rest[:end]))
	if target == "" {
		return nil
	}

	block.Advance(2 + end + 2)
	href, ok := resolveWikilink(target, baseDirFromContext(pc))
	return &wikilinkNode{Label: label, Href: href, Broken: !ok}
}

type wikilinkRenderer struct{}

func (r *wikilinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindWikilink, r.render)
}

func (r *wikilinkRenderer) render(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	n := node.(*wikilinkNode)
	class := "wikilink"
	if n.Broken {
		class = "wikilink broken"
	}
	_, _ = w.WriteString(`<a class="` + class + `" href="`)
	_, _ = w.WriteString(template.HTMLEscapeString(n.Href))
	_, _ = w.WriteString(`">`)
	_, _ = w.WriteString(template.HTMLEscapeString(n.Label))
	_, _ = w.WriteString(`</a>`)
	return gast.WalkContinue, nil
}

type wikilinkExtension struct{}

// Wikilink is a goldmark extension that renders Obsidian-style `[[links]]`.
var Wikilink = &wikilinkExtension{}

func (e *wikilinkExtension) Extend(m goldmark.Markdown) {
	// Priority 150 is load-bearing: goldmark's own link parser sits at 200, so
	// a lower number is needed to see `[[` first. Code spans stay at 100 and
	// keep their precedence, which is what makes `` `[[foo]]` `` literal.
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&wikilinkParser{}, 150),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&wikilinkRenderer{}, 500),
	))
}
