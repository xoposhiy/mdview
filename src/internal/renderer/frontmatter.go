package renderer

import (
	"bytes"
	"errors"
	"html/template"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// fmField is one row of the frontmatter properties table. Values holds a single
// element for scalars and one element per item for sequences; IsList records
// which of the two it was, so that a one-element sequence still renders as a
// pill rather than as plain text.
type fmField struct {
	Key    string
	Values []string
	IsList bool
}

// errNotMapping reports frontmatter that parsed as valid YAML but whose root is
// not a key/value mapping — most often a document that simply opens with a
// thematic break. Such a block is not frontmatter and is left in the body.
var errNotMapping = errors.New("frontmatter root is not a mapping")

// extractFrontmatter pulls an Obsidian-style YAML frontmatter block off the
// front of src, returning the properties table to place above the document and
// the remaining Markdown body. With no frontmatter present the source is
// returned untouched and the table is empty.
func extractFrontmatter(src []byte) (template.HTML, []byte) {
	fm, body := splitFrontmatter(src)
	if fm == nil {
		return "", src
	}

	fields, err := parseFrontmatter(fm)
	switch {
	case errors.Is(err, errNotMapping):
		// Not really frontmatter — leave the document alone.
		return "", src
	case err != nil:
		// Broken YAML: strip the block but show it verbatim rather than
		// silently dropping whatever the author wrote.
		return rawFrontmatter(fm), body
	}
	return renderFrontmatter(fields), body
}

// splitFrontmatter separates a leading YAML frontmatter block from the Markdown
// body. The opening `---` must be on the very first line, and a closing `---`
// or `...` must sit alone on a later line; otherwise the whole source comes
// back as the body with a nil frontmatter. A non-nil but empty frontmatter
// means the block was present and empty, so it still gets stripped.
func splitFrontmatter(src []byte) (fm, body []byte) {
	first, start := nextLine(src, 0)
	if trimTrailingSpace(first) != "---" {
		return nil, src
	}
	for i := start; i < len(src); {
		line, next := nextLine(src, i)
		if s := trimTrailingSpace(line); s == "---" || s == "..." {
			return src[start:i], src[next:]
		}
		i = next
	}
	return nil, src // no closing delimiter
}

// nextLine returns the line starting at i with its terminator (including a
// trailing CR) removed, plus the index at which the following line starts.
func nextLine(b []byte, i int) (line []byte, next int) {
	j := bytes.IndexByte(b[i:], '\n')
	if j < 0 {
		return bytes.TrimRight(b[i:], "\r"), len(b)
	}
	return bytes.TrimRight(b[i:i+j], "\r"), i + j + 1
}

func trimTrailingSpace(b []byte) string {
	return string(bytes.TrimRight(b, " \t"))
}

// parseFrontmatter turns a YAML frontmatter block into table rows in the order
// the keys appear in the file. It decodes into a yaml.Node rather than a map
// because Go maps have no stable order and the table must mirror the document.
func parseFrontmatter(fm []byte) ([]fmField, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil {
		return nil, err
	}
	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, nil
		}
		root = root.Content[0]
	}
	switch root.Kind {
	case 0:
		return nil, nil // empty or comment-only block
	case yaml.MappingNode:
		var fields []fmField
		flattenNode(root, "", &fields)
		return fields, nil
	default:
		return nil, errNotMapping
	}
}

// flattenNode walks a YAML value and appends one row per leaf. Nested mappings
// become dotted keys (`source.file`); sequences of scalars become a single row
// of pills; sequences holding anything richer are indexed (`refs[0].name`).
func flattenNode(n *yaml.Node, path string, out *[]fmField) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			flattenNode(n.Content[i+1], joinPath(path, n.Content[i].Value), out)
		}
	case yaml.SequenceNode:
		if vals, ok := scalarValues(n); ok {
			*out = append(*out, fmField{Key: path, Values: vals, IsList: true})
			return
		}
		for i, item := range n.Content {
			flattenNode(item, path+"["+strconv.Itoa(i)+"]", out)
		}
	case yaml.AliasNode:
		if n.Alias != nil {
			flattenNode(n.Alias, path, out)
		}
	default: // scalar, including an explicit or implicit null
		*out = append(*out, fmField{Key: path, Values: []string{n.Value}})
	}
}

// scalarValues reports the element values of a sequence, or ok=false if any
// element is itself a mapping or sequence.
func scalarValues(n *yaml.Node) (vals []string, ok bool) {
	vals = make([]string, 0, len(n.Content))
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, false
		}
		vals = append(vals, item.Value)
	}
	return vals, true
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// renderFrontmatter builds the two-column properties table. Values are plain
// text, never Markdown, so everything is escaped.
func renderFrontmatter(fields []fmField) template.HTML {
	if len(fields) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<table class="frontmatter"><tbody>`)
	for _, f := range fields {
		sb.WriteString("<tr><th>")
		sb.WriteString(template.HTMLEscapeString(f.Key))
		sb.WriteString("</th><td>")
		if f.IsList {
			for _, v := range f.Values {
				sb.WriteString(`<span class="fm-pill">`)
				sb.WriteString(template.HTMLEscapeString(v))
				sb.WriteString("</span>")
			}
		} else {
			sb.WriteString(template.HTMLEscapeString(strings.Join(f.Values, ", ")))
		}
		sb.WriteString("</td></tr>")
	}
	sb.WriteString("</tbody></table>")
	return template.HTML(sb.String())
}

// rawFrontmatter shows a frontmatter block that failed to parse as-is.
func rawFrontmatter(fm []byte) template.HTML {
	return template.HTML(`<pre class="frontmatter-raw">` +
		template.HTMLEscapeString(strings.TrimRight(string(fm), "\n")) + `</pre>`)
}
