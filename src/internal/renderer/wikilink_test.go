package renderer

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdview/internal/config"
)

// mkVault marks dir as an Obsidian vault root by creating dir/.obsidian.
func mkVault(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// mkDir creates dir and any missing parents, returning dir.
func mkDir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// requireNoVaultAbove skips the test when a real vault sits above dir. The walk
// runs all the way to the volume root, so such a vault would be a legitimate
// hit rather than a bug in vaultRoot.
func requireNoVaultAbove(t *testing.T, dir string) {
	t.Helper()
	if above := vaultRoot(dir); above != "" {
		t.Skipf("an actual vault exists at or above the temp dir: %s", above)
	}
}

// touch creates an empty file at path, creating parent directories as needed,
// and returns path.
func touch(t *testing.T, path string) string {
	t.Helper()
	mkDir(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVaultRootMarkerInSameDir(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	if got := vaultRoot(root); got != root {
		t.Errorf("vaultRoot(%q) = %q, want %q", root, got, root)
	}
}

func TestVaultRootMarkerSeveralLevelsUp(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	deep := mkDir(t, filepath.Join(root, "a", "b", "c"))
	if got := vaultRoot(deep); got != root {
		t.Errorf("vaultRoot(%q) = %q, want %q", deep, got, root)
	}
}

func TestVaultRootNoMarker(t *testing.T) {
	root := t.TempDir()
	requireNoVaultAbove(t, root)
	deep := mkDir(t, filepath.Join(root, "a", "b"))
	if got := vaultRoot(deep); got != "" {
		t.Errorf("vaultRoot(%q) = %q, want empty", deep, got)
	}
}

func TestVaultRootIgnoresObsidianFile(t *testing.T) {
	root := t.TempDir()
	requireNoVaultAbove(t, root)
	if err := os.WriteFile(filepath.Join(root, ".obsidian"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := vaultRoot(root); got != "" {
		t.Errorf("vaultRoot(%q) = %q, want empty: .obsidian is a file, not a directory", root, got)
	}
}

// checkResolve asserts that resolveWikilink(target, baseDir) returns wantHref
// and wantOK.
func checkResolve(t *testing.T, target, baseDir, wantHref string, wantOK bool) {
	t.Helper()
	href, ok := resolveWikilink(target, baseDir)
	if href != wantHref || ok != wantOK {
		t.Errorf("resolveWikilink(%q, %q) = (%q, %v), want (%q, %v)",
			target, baseDir, href, ok, wantHref, wantOK)
	}
}

func TestResolveBesideDocument(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, filepath.Join(dir, "note.md"))
	checkResolve(t, "note", dir, want, true)
}

func TestResolveExplicitSubfolder(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, filepath.Join(dir, "sub", "note.md"))
	checkResolve(t, "sub/note", dir, want, true)
}

func TestResolveFromVaultRoot(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	want := touch(t, filepath.Join(root, "projects", "spec.md"))
	docDir := mkDir(t, filepath.Join(root, "daily"))
	checkResolve(t, "projects/spec", docDir, want, true)
}

func TestResolveNearestWinsOverVaultRoot(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	touch(t, filepath.Join(root, "note.md"))
	docDir := mkDir(t, filepath.Join(root, "daily"))
	want := touch(t, filepath.Join(docDir, "note.md"))
	checkResolve(t, "note", docDir, want, true)
}

// A parent directory is searched whether or not a vault marker is present: the
// upward walk is bounded by maxParentLevels, not by .obsidian.
func TestResolveFromParentWithoutVaultMarker(t *testing.T) {
	root := t.TempDir()
	requireNoVaultAbove(t, root)
	want := touch(t, filepath.Join(root, "note.md"))
	docDir := mkDir(t, filepath.Join(root, "daily"))
	checkResolve(t, "note", docDir, want, true)
}

// deepDir returns root/l1/.../lN, the directory N levels below root.
func deepDir(t *testing.T, root string, levels int) string {
	t.Helper()
	dir := root
	for i := 1; i <= levels; i++ {
		dir = filepath.Join(dir, fmt.Sprintf("l%d", i))
	}
	return mkDir(t, dir)
}

// The fourth parent is still searched; the fifth is not.
func TestResolveFromFourthParent(t *testing.T) {
	root := t.TempDir()
	requireNoVaultAbove(t, root)
	want := touch(t, filepath.Join(root, "note.md"))
	checkResolve(t, "note", deepDir(t, root, maxParentLevels), want, true)
}

func TestResolveStopsAboveFourthParent(t *testing.T) {
	root := t.TempDir()
	requireNoVaultAbove(t, root)
	touch(t, filepath.Join(root, "note.md"))
	docDir := deepDir(t, root, maxParentLevels+1)
	checkResolve(t, "note", docDir, filepath.Join(docDir, "note.md"), false)
}

// Beyond the parent limit the vault root is still searched — that is the whole
// point of the marker.
func TestResolveFromVaultRootBeyondParentLimit(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	want := touch(t, filepath.Join(root, "note.md"))
	checkResolve(t, "note", deepDir(t, root, maxParentLevels+1), want, true)
}

// Nearest wins all the way up: a hit in the immediate parent beats one further
// above it.
func TestResolveNearestParentWins(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	touch(t, filepath.Join(root, "note.md"))
	want := touch(t, filepath.Join(root, "a", "note.md"))
	checkResolve(t, "note", mkDir(t, filepath.Join(root, "a", "b")), want, true)
}

func TestResolveMissingFileIsBroken(t *testing.T) {
	root := t.TempDir()
	docDir := mkDir(t, filepath.Join(root, "daily"))
	checkResolve(t, "missing", docDir, filepath.Join(docDir, "missing.md"), false)
}

func TestResolveExplicitMdExtensionNotDoubled(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, filepath.Join(dir, "note.md"))
	checkResolve(t, "note.md", dir, want, true)
}

func TestResolveNonMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, filepath.Join(dir, "diagram.png"))
	checkResolve(t, "diagram.png", dir, want, true)
}

// A name whose filepath.Ext is meaningless (".2 release notes") must still
// resolve, which is what the try-verbatim-then-append-.md order buys.
func TestResolveNameContainingDots(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, filepath.Join(dir, "v1.2 release notes.md"))
	checkResolve(t, "v1.2 release notes", dir, want, true)
}

func TestResolveKeepsFragment(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "note.md"))
	checkResolve(t, "note#heading", dir, target+"#heading", true)
}

func TestResolveBrokenKeepsFragment(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	checkResolve(t, "missing#heading", root, filepath.Join(root, "missing.md")+"#heading", false)
}

// A fragment with no path is an in-page anchor; classifyLink routes a leading
// "#" href to linkInPage, so the page scrolls itself.
func TestResolveFragmentOnly(t *testing.T) {
	dir := t.TempDir()
	checkResolve(t, "#heading", dir, "#heading", true)
}

// With no baseDir there is nothing to stat against, so the href stays relative
// and the link is never reported broken.
func TestResolveWithoutBaseDir(t *testing.T) {
	checkResolve(t, "note", "", "note.md", true)
	checkResolve(t, "note.md", "", "note.md", true)
	checkResolve(t, "sub/note", "", "sub/note.md", true)
}

func renderIn(t *testing.T, markdown, baseDir string) string {
	t.Helper()
	html, err := RenderWithTheme([]byte(markdown), config.Default(), 0, baseDir)
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func mustContain(t *testing.T, html, want string) {
	t.Helper()
	if !strings.Contains(html, want) {
		t.Errorf("expected %q in output:\n%s", want, html)
	}
}

func mustNotContain(t *testing.T, html, unwanted string) {
	t.Helper()
	if strings.Contains(html, unwanted) {
		t.Errorf("did not expect %q in output:\n%s", unwanted, html)
	}
}

// anchor is the HTML a wikilink to href with the given label should produce.
func anchor(class, href, label string) string {
	return `<a class="` + class + `" href="` + template.HTMLEscapeString(href) + `">` + label + `</a>`
}

// mustNotLink asserts no wikilink anchor was produced. It looks for the opening
// tag rather than the bare word "wikilink", which also appears in the page's
// stylesheet.
func mustNotLink(t *testing.T, html string) {
	t.Helper()
	mustNotContain(t, html, `<a class="wikilink`)
}

func TestWikilinkRendersAnchor(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other]] here", dir)
	mustContain(t, html, anchor("wikilink", target, "other"))
}

func TestWikilinkAlias(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other|the label]]", dir)
	mustContain(t, html, anchor("wikilink", target, "the label"))
}

// Obsidian treats the alias as everything after the first "|".
func TestWikilinkAliasKeepsLaterPipes(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other|a|b]]", dir)
	mustContain(t, html, anchor("wikilink", target, "a|b"))
}

func TestWikilinkEmptyAliasFallsBackToTarget(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other|]]", dir)
	mustContain(t, html, anchor("wikilink", target, "other"))
}

func TestWikilinkTrimsSurroundingSpaces(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[ other | the label ]]", dir)
	mustContain(t, html, anchor("wikilink", target, "the label"))
}

func TestWikilinkSubfolderLabelShowsPath(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "sub", "other.md"))
	html := renderIn(t, "see [[sub/other]]", dir)
	mustContain(t, html, anchor("wikilink", target, "sub/other"))
}

func TestWikilinkBrokenGetsBrokenClass(t *testing.T) {
	dir := t.TempDir()
	html := renderIn(t, "see [[missing]]", dir)
	mustContain(t, html, anchor("wikilink broken", filepath.Join(dir, "missing.md"), "missing"))
}

func TestWikilinkKeepsFragment(t *testing.T) {
	dir := t.TempDir()
	target := touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other#heading]]", dir)
	mustContain(t, html, anchor("wikilink", target+"#heading", "other#heading"))
}

func TestWikilinkResolvesFromVaultRoot(t *testing.T) {
	root := t.TempDir()
	mkVault(t, root)
	target := touch(t, filepath.Join(root, "projects", "spec.md"))
	docDir := mkDir(t, filepath.Join(root, "daily"))
	html := renderIn(t, "see [[projects/spec]]", docDir)
	mustContain(t, html, anchor("wikilink", target, "projects/spec"))
}

func TestWikilinkEscapesLabel(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "see [[other|a <b> & c]]", dir)
	mustContain(t, html, `>a &lt;b&gt; &amp; c</a>`)
	// The whole page is searched, and the embedded MathJax bundle happens to
	// contain "<b>", so assert on the unescaped label as a whole instead.
	mustNotContain(t, html, "a <b> & c")
}

func TestWikilinkLiteralInCodeSpan(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "use `[[other]]` verbatim", dir)
	mustContain(t, html, "<code>[[other]]</code>")
	mustNotLink(t, html)
}

func TestWikilinkLiteralInFencedBlock(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "```\n[[other]]\n```", dir)
	mustContain(t, html, "[[other]]")
	mustNotLink(t, html)
}

func TestWikilinkLiteralWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, md := range []string{"[[]]", "[[ ]]", "[[|label]]"} {
		html := renderIn(t, md, dir)
		mustNotLink(t, html)
	}
}

func TestWikilinkLiteralWhenUnclosed(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "other.md"))
	html := renderIn(t, "an unclosed [[other link", dir)
	mustNotLink(t, html)
}

func TestWikilinkTwoOnOneLine(t *testing.T) {
	dir := t.TempDir()
	first := touch(t, filepath.Join(dir, "one.md"))
	second := touch(t, filepath.Join(dir, "two.md"))
	html := renderIn(t, "[[one]] and [[two]]", dir)
	mustContain(t, html, anchor("wikilink", first, "one"))
	mustContain(t, html, anchor("wikilink", second, "two"))
}

// The dimming rule lives in the page's inline stylesheet; without it a broken
// wikilink is indistinguishable from a working one.
func TestBrokenWikilinkIsStyled(t *testing.T) {
	html := renderIn(t, "text", t.TempDir())
	mustContain(t, html, "a.wikilink.broken {")
}

// The wikilink parser is registered ahead of goldmark's own link parser, so
// guard against it swallowing ordinary links.
func TestNormalMarkdownLinkStillWorks(t *testing.T) {
	dir := t.TempDir()
	html := renderIn(t, "see [text](other.md) and [ref][1]\n\n[1]: http://example.com", dir)
	mustContain(t, html, `<a href="other.md">text</a>`)
	mustContain(t, html, `<a href="http://example.com">ref</a>`)
}
