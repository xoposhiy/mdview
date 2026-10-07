package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mdview/internal/config"
)

const (
	docWithMath    = "# hi\n\nEuler wrote $e^{i\\pi} + 1 = 0$.\n"
	docWithoutMath = "# hi\n\nNo formulas here, just prose and `code`.\n"
)

// linkTag is what a page that links MathJax carries instead of the script.
const linkTag = `<script defer src=`

func TestPageWithoutMathHasNoMathJax(t *testing.T) {
	dir := t.TempDir()
	for name, render := range map[string]func([]byte, config.Config, int, string) (string, error){
		"standalone": RenderWithTheme,
		"linked":     RenderLinked(dir),
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
	// A session without formulas does not write MathJax anywhere.
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("no page had formulas, but %d file(s) were written", len(files))
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
	dir := filepath.Join(t.TempDir(), "not yet there")
	render := RenderLinked(dir)
	html, err := render([]byte(docWithMath), config.Default(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, mathJaxFileName())
	want := `<script defer src="` + fileURL(path) + `"></script>`
	if !strings.Contains(html, want) {
		t.Errorf("page does not link MathJax as %s", want)
	}
	// The whole point: the script itself is no longer in the page.
	if len(html) > 100_000 {
		t.Errorf("page is %d bytes; MathJax looks inlined as well", len(html))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the linked file was not written: %v", err)
	}
	if string(data) != mathJaxScript {
		t.Error("the linked file is not the embedded MathJax")
	}
}

func TestLinkedPageInlinesWhenTheFileCannotBeWritten(t *testing.T) {
	// A regular file where the folder should be: nothing can go inside it.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	render := RenderLinked(blocker)
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

func TestUnpackMathJaxKeepsAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path, err := unpackMathJax(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	again, err := unpackMathJax(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again != path {
		t.Errorf("second unpack returned %s, want %s", again, path)
	}
	if fi, _ := os.Stat(path); !fi.ModTime().Equal(old) {
		t.Error("an existing, complete file was written again")
	}
}

func TestUnpackMathJaxReplacesOtherVersions(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "mathjax-000000000000.js")
	truncated := filepath.Join(dir, mathJaxFileName())
	other := filepath.Join(dir, "config.json")
	for _, p := range []string{stale, truncated, other} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path, err := unpackMathJax(dir)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != mathJaxScript {
		t.Error("a truncated file was not replaced")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("the file of another MathJax version was not removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("a file that is not MathJax was removed")
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmps) != 0 {
		t.Errorf("temporary files left behind: %v", tmps)
	}
}

func TestUnpackMathJaxFromManyGoroutines(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := unpackMathJax(dir); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, mathJaxFileName())); string(data) != mathJaxScript {
		t.Error("the file is not the embedded MathJax after parallel unpacks")
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
