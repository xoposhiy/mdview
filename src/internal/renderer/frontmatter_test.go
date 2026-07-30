package renderer

import (
	"strings"
	"testing"
)

// fieldSig renders fields as "key=a|b" lines so tests can assert on order and
// values in one readable string.
func fieldSig(fields []fmField) string {
	var sb strings.Builder
	for _, f := range fields {
		sb.WriteString(f.Key)
		sb.WriteString("=")
		sb.WriteString(strings.Join(f.Values, "|"))
		sb.WriteString("\n")
	}
	return sb.String()
}

// ----- splitFrontmatter -----

func TestSplitFrontmatterExtractsBlock(t *testing.T) {
	fm, body := splitFrontmatter([]byte("---\ntitle: Hello\n---\n\n# Body\n"))
	if string(fm) != "title: Hello\n" {
		t.Errorf("frontmatter = %q, want %q", fm, "title: Hello\n")
	}
	if strings.TrimSpace(string(body)) != "# Body" {
		t.Errorf("body = %q, want %q", body, "# Body")
	}
}

func TestSplitFrontmatterAbsentReturnsWholeSource(t *testing.T) {
	src := "# Just a heading\n\ntext\n"
	fm, body := splitFrontmatter([]byte(src))
	if fm != nil {
		t.Errorf("expected no frontmatter, got %q", fm)
	}
	if string(body) != src {
		t.Errorf("body = %q, want unchanged source", body)
	}
}

func TestSplitFrontmatterUnterminatedReturnsWholeSource(t *testing.T) {
	src := "---\ntitle: Hello\n\n# Body with no closing delimiter\n"
	fm, body := splitFrontmatter([]byte(src))
	if fm != nil {
		t.Errorf("expected no frontmatter for unterminated block, got %q", fm)
	}
	if string(body) != src {
		t.Errorf("body = %q, want unchanged source", body)
	}
}

func TestSplitFrontmatterAcceptsDotsTerminator(t *testing.T) {
	fm, body := splitFrontmatter([]byte("---\ntitle: Hello\n...\n# Body\n"))
	if string(fm) != "title: Hello\n" {
		t.Errorf("frontmatter = %q, want %q", fm, "title: Hello\n")
	}
	if strings.TrimSpace(string(body)) != "# Body" {
		t.Errorf("body = %q, want %q", body, "# Body")
	}
}

func TestSplitFrontmatterHandlesCRLF(t *testing.T) {
	fm, body := splitFrontmatter([]byte("---\r\ntitle: Hello\r\n---\r\n# Body\r\n"))
	if !strings.Contains(string(fm), "title: Hello") {
		t.Errorf("frontmatter = %q, want it to contain %q", fm, "title: Hello")
	}
	if !strings.Contains(string(body), "# Body") {
		t.Errorf("body = %q, want it to contain %q", body, "# Body")
	}
	if strings.Contains(string(body), "title: Hello") {
		t.Errorf("body still contains the frontmatter: %q", body)
	}
}

func TestSplitFrontmatterEmptyBlock(t *testing.T) {
	fm, body := splitFrontmatter([]byte("---\n---\n# Body\n"))
	if len(fm) != 0 {
		t.Errorf("frontmatter = %q, want empty", fm)
	}
	if strings.TrimSpace(string(body)) != "# Body" {
		t.Errorf("body = %q, want %q", body, "# Body")
	}
	if fm == nil {
		t.Error("expected non-nil (but empty) frontmatter so the block is still stripped")
	}
}

func TestSplitFrontmatterRequiresDelimiterOnFirstLine(t *testing.T) {
	src := "\n---\ntitle: Hello\n---\n"
	fm, _ := splitFrontmatter([]byte(src))
	if fm != nil {
		t.Errorf("delimiter was not on line 1; expected no frontmatter, got %q", fm)
	}
}

// ----- parseFrontmatter -----

func TestParseFrontmatterPreservesKeyOrder(t *testing.T) {
	fields, err := parseFrontmatter([]byte("zebra: 1\napple: 2\nmango: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "zebra=1\napple=2\nmango=3\n"
	if got := fieldSig(fields); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseFrontmatterFlattensNestedMap(t *testing.T) {
	fm := "title: T\nsource:\n  file: a.pdf\n  version: \"V 1.0\"\nstatus: current\n"
	fields, err := parseFrontmatter([]byte(fm))
	if err != nil {
		t.Fatal(err)
	}
	want := "title=T\nsource.file=a.pdf\nsource.version=V 1.0\nstatus=current\n"
	if got := fieldSig(fields); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseFrontmatterFlattensDeeplyNestedMap(t *testing.T) {
	fields, err := parseFrontmatter([]byte("a:\n  b:\n    c: deep\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldSig(fields); got != "a.b.c=deep\n" {
		t.Errorf("got %q, want %q", got, "a.b.c=deep\n")
	}
}

func TestParseFrontmatterFlowSequence(t *testing.T) {
	fields, err := parseFrontmatter([]byte("tags: [handbook, sdt, scope]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("want 1 field, got %d: %v", len(fields), fields)
	}
	if !fields[0].IsList {
		t.Error("tags should be marked as a list")
	}
	if got := fieldSig(fields); got != "tags=handbook|sdt|scope\n" {
		t.Errorf("got %q", got)
	}
}

func TestParseFrontmatterBlockSequence(t *testing.T) {
	fields, err := parseFrontmatter([]byte("tags:\n  - one\n  - two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !fields[0].IsList {
		t.Error("tags should be marked as a list")
	}
	if got := fieldSig(fields); got != "tags=one|two\n" {
		t.Errorf("got %q", got)
	}
}

func TestParseFrontmatterSingleValueSequenceIsStillAList(t *testing.T) {
	fields, err := parseFrontmatter([]byte("tags: [solo]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !fields[0].IsList {
		t.Error("a one-element sequence should still render as a list")
	}
}

func TestParseFrontmatterScalarIsNotAList(t *testing.T) {
	fields, err := parseFrontmatter([]byte("title: Hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fields[0].IsList {
		t.Error("a scalar should not be marked as a list")
	}
}

func TestParseFrontmatterQuotedStringWithSpecialChars(t *testing.T) {
	fields, err := parseFrontmatter([]byte(`title: "Regs: for Software, Data & Tech"` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldSig(fields); got != "title=Regs: for Software, Data & Tech\n" {
		t.Errorf("got %q", got)
	}
}

func TestParseFrontmatterKeepsDateAsWrittenInFile(t *testing.T) {
	fields, err := parseFrontmatter([]byte("valid_from: 2026-09-01\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldSig(fields); got != "valid_from=2026-09-01\n" {
		t.Errorf("got %q, want the date verbatim, not a reformatted timestamp", got)
	}
}

func TestParseFrontmatterNullValueKeepsRow(t *testing.T) {
	fields, err := parseFrontmatter([]byte("title: T\nstatus:\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldSig(fields); got != "title=T\nstatus=\n" {
		t.Errorf("got %q, want the empty status row preserved", got)
	}
}

func TestParseFrontmatterSequenceOfMaps(t *testing.T) {
	fm := "refs:\n  - name: a\n    page: 1\n  - name: b\n    page: 2\n"
	fields, err := parseFrontmatter([]byte(fm))
	if err != nil {
		t.Fatal(err)
	}
	want := "refs[0].name=a\nrefs[0].page=1\nrefs[1].name=b\nrefs[1].page=2\n"
	if got := fieldSig(fields); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseFrontmatterEmptyReturnsNoFields(t *testing.T) {
	fields, err := parseFrontmatter([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 0 {
		t.Errorf("want no fields, got %v", fields)
	}
}

func TestParseFrontmatterMalformedReturnsError(t *testing.T) {
	if _, err := parseFrontmatter([]byte("tags: [unclosed\nfoo: bar\n")); err == nil {
		t.Error("expected an error for malformed YAML, got nil")
	}
}

func TestParseFrontmatterNonMappingRootReturnsError(t *testing.T) {
	if _, err := parseFrontmatter([]byte("just a bare string\n")); err == nil {
		t.Error("expected an error when the root node is not a mapping, got nil")
	}
}

// ----- renderFrontmatter -----

func TestRenderFrontmatterBuildsTableRows(t *testing.T) {
	html := string(renderFrontmatter([]fmField{{Key: "title", Values: []string{"Hello"}}}))
	if !strings.Contains(html, `class="frontmatter"`) {
		t.Errorf("expected a .frontmatter table, got:\n%s", html)
	}
	if !strings.Contains(html, "<th>title</th>") {
		t.Errorf("expected key cell, got:\n%s", html)
	}
	if !strings.Contains(html, "<td>Hello</td>") {
		t.Errorf("expected value cell, got:\n%s", html)
	}
}

func TestRenderFrontmatterListValuesBecomePills(t *testing.T) {
	f := []fmField{{Key: "tags", Values: []string{"a", "b"}, IsList: true}}
	html := string(renderFrontmatter(f))
	if strings.Count(html, `class="fm-pill"`) != 2 {
		t.Errorf("expected 2 pills, got:\n%s", html)
	}
}

func TestRenderFrontmatterEscapesHTMLInValues(t *testing.T) {
	f := []fmField{{Key: "title", Values: []string{`<script>alert(1)</script>`}}}
	html := string(renderFrontmatter(f))
	if strings.Contains(html, "<script>") {
		t.Errorf("value was not escaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("expected escaped value, got:\n%s", html)
	}
}

func TestRenderFrontmatterEscapesHTMLInKeys(t *testing.T) {
	f := []fmField{{Key: `<b>k</b>`, Values: []string{"v"}}}
	if html := string(renderFrontmatter(f)); strings.Contains(html, "<b>") {
		t.Errorf("key was not escaped:\n%s", html)
	}
}

func TestRenderFrontmatterEscapesHTMLInPills(t *testing.T) {
	f := []fmField{{Key: "tags", Values: []string{`<img src=x>`}, IsList: true}}
	if html := string(renderFrontmatter(f)); strings.Contains(html, "<img") {
		t.Errorf("pill value was not escaped:\n%s", html)
	}
}

func TestRenderFrontmatterNoFieldsRendersNothing(t *testing.T) {
	if html := string(renderFrontmatter(nil)); html != "" {
		t.Errorf("expected empty output for no fields, got %q", html)
	}
}

// ----- extractFrontmatter (entry point) -----

func TestExtractFrontmatterReturnsTableAndStrippedBody(t *testing.T) {
	table, body := extractFrontmatter([]byte("---\ntitle: Hello\n---\n# Body\n"))
	if !strings.Contains(string(table), "<th>title</th>") {
		t.Errorf("expected properties table, got:\n%s", table)
	}
	if strings.Contains(string(body), "title: Hello") {
		t.Errorf("frontmatter leaked into the body: %q", body)
	}
}

func TestExtractFrontmatterAbsentReturnsNoTable(t *testing.T) {
	src := "# Body only\n"
	table, body := extractFrontmatter([]byte(src))
	if table != "" {
		t.Errorf("expected no table, got:\n%s", table)
	}
	if string(body) != src {
		t.Errorf("body = %q, want unchanged", body)
	}
}

func TestExtractFrontmatterEmptyBlockRendersNoTableButStripsBlock(t *testing.T) {
	table, body := extractFrontmatter([]byte("---\n---\n# Body\n"))
	if table != "" {
		t.Errorf("expected no table for empty frontmatter, got:\n%s", table)
	}
	if strings.Contains(string(body), "---") {
		t.Errorf("expected the empty block to be stripped, got %q", body)
	}
}

func TestExtractFrontmatterMalformedYAMLShowsRawBlock(t *testing.T) {
	table, body := extractFrontmatter([]byte("---\ntags: [unclosed\n---\n# Body\n"))
	if !strings.Contains(string(table), "<pre") {
		t.Errorf("expected a <pre> fallback for malformed YAML, got:\n%s", table)
	}
	if !strings.Contains(string(table), "tags: [unclosed") {
		t.Errorf("expected the raw YAML to be shown, got:\n%s", table)
	}
	if strings.Contains(string(body), "tags: [unclosed") {
		t.Errorf("malformed frontmatter leaked into the body: %q", body)
	}
}

func TestExtractFrontmatterLeadingThematicBreakIsNotFrontmatter(t *testing.T) {
	// A document opening with a thematic break must not be eaten as frontmatter.
	src := "---\n\nSome introductory text.\n\n---\n\n# Body\n"
	table, body := extractFrontmatter([]byte(src))
	if table != "" {
		t.Errorf("expected no table, got:\n%s", table)
	}
	if string(body) != src {
		t.Errorf("body = %q, want unchanged source", body)
	}
}
