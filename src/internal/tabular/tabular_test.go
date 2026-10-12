package tabular

import (
	"strings"
	"testing"

	"mdview/internal/renderer"
)

func TestIsTable(t *testing.T) {
	for ext, want := range map[string]bool{
		".csv": true, ".CSV": true, ".tsv": true, ".Tsv": true,
		".md": false, ".txt": false, "": false,
	} {
		if got := IsTable(ext); got != want {
			t.Errorf("IsTable(%q) = %v, want %v", ext, got, want)
		}
	}
}

func TestPrepareLeavesMarkdownAlone(t *testing.T) {
	in := []byte("# a,b\n")
	out, err := Prepare(`C:\d\x.md`, in)
	if err != nil || string(out) != string(in) {
		t.Fatalf("Prepare(.md) = %q, %v; want the input unchanged", out, err)
	}
}

func TestPrepareCSV(t *testing.T) {
	in := "\xef\xbb\xbfname,qty\r\n\"Smith, J\",3\r\n\"two\nlines\",\"say \"\"hi\"\"\"\r\n<b>x</b>\r\n"
	out, err := Prepare("t.csv", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "<table>\n<thead>\n<tr><th>name</th><th>qty</th></tr>\n</thead>\n<tbody>\n" +
		"<tr><td>Smith, J</td><td>3</td></tr>\n" +
		"<tr><td>two<br>lines</td><td>say &#34;hi&#34;</td></tr>\n" +
		"<tr><td>&lt;b&gt;x&lt;/b&gt;</td><td></td></tr>\n" +
		"</tbody>\n</table>\n"
	if string(out) != want {
		t.Errorf("Prepare(csv) =\n%s\nwant\n%s", out, want)
	}
}

func TestPrepareCSVSemicolons(t *testing.T) {
	out, err := Prepare("t.csv", []byte("a;b\n1,5;2,5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<td>1,5</td><td>2,5</td>") {
		t.Errorf("semicolon CSV not split at semicolons:\n%s", out)
	}
}

func TestPrepareTSV(t *testing.T) {
	out, err := Prepare("t.tsv", []byte("a\tb\tc\r\n\"q\tx\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<tr><td>&#34;q</td><td>x</td><td></td></tr>") {
		t.Errorf("TSV row not split at tabs or not padded:\n%s", out)
	}
}

func TestPrepareEmpty(t *testing.T) {
	out, err := Prepare("t.csv", nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("Prepare(empty csv) = %q, %v; want empty", out, err)
	}
}

// The table must reach the page as a table: goldmark must pass it through as
// one raw HTML block, even when a cell holds blank lines, Markdown syntax or
// a formula delimiter.
func TestRenderedTable(t *testing.T) {
	in := "h1,h2\n\"para\n\nbreak\",*not em* $x$ \\(y\\) [[link]]\n"
	src, err := Prepare("t.csv", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderer.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<thead>\n<tr><th>h1</th><th>h2</th></tr>",
		"<td>para<br><br>break</td>",
		"<td>*not em* $x$ &#92;(y&#92;) [[link]]</td>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "<p>") {
		t.Error("goldmark split the table into paragraphs")
	}
	if strings.Contains(page, "MathJax =") || strings.Contains(page, "mathjax") {
		t.Error("a table with no formulas loaded MathJax")
	}
}
