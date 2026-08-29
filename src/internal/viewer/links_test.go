package viewer

import "testing"

func TestClassifyLink(t *testing.T) {
	const baseDir = `C:\docs`

	cases := []struct {
		name       string
		href       string
		wantKind   linkKind
		wantTarget string
	}{
		{"in-page anchor", "#section", linkInPage, "#section"},
		{"empty href", "", linkInPage, ""},
		{"https", "https://example.com/x", linkExternal, "https://example.com/x"},
		{"http", "http://example.com", linkExternal, "http://example.com"},
		{"mailto", "mailto:a@b.c", linkExternal, "mailto:a@b.c"},
		{"custom scheme", "obsidian://open?vault=x", linkExternal, "obsidian://open?vault=x"},
		{"relative md", "./other.md", linkViewable, `C:\docs\other.md`},
		{"nested md, mixed case ext", "sub/Other.MD", linkViewable, `C:\docs\sub\Other.MD`},
		{"absolute windows path is not a scheme", "C:/abs/x.md", linkViewable, `C:\abs\x.md`},
		{"markdown long extension", "notes.markdown", linkViewable, `C:\docs\notes.markdown`},
		{"local non-md file", "spec.pdf", linkExternal, `C:\docs\spec.pdf`},
		{"percent-decoded path", "other%20file.md", linkViewable, `C:\docs\other file.md`},
		{"fragment stripped from md", "other.md#sec", linkViewable, `C:\docs\other.md`},
		{"query stripped from local file", "docs/spec.pdf?v=2", linkExternal, `C:\docs\docs\spec.pdf`},
		{"relative image link opens in-app", "chart.png", linkViewable, `C:\docs\chart.png`},
		{"mixed case image extension", "photo.JPG", linkViewable, `C:\docs\photo.JPG`},
		{"unsupported image-like extension stays external", "scan.tiff", linkExternal, `C:\docs\scan.tiff`},

		// The renderer's wikilinks emit an already-resolved absolute path with
		// backslashes, so these two forms are what [[note]] and [[note#head]]
		// hand back through goOpenLink.
		{"backslash absolute md path", `C:\vault\note.md`, linkViewable, `C:\vault\note.md`},
		{"backslash absolute path with fragment", `C:\vault\note.md#head`, linkViewable, `C:\vault\note.md`},
		{"backslash absolute image path now opens in-app", `C:\vault\diagram.png`, linkViewable, `C:\vault\diagram.png`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, target := classifyLink(c.href, baseDir)
			if kind != c.wantKind {
				t.Errorf("classifyLink(%q) kind = %v, want %v", c.href, kind, c.wantKind)
			}
			if target != c.wantTarget {
				t.Errorf("classifyLink(%q) target = %q, want %q", c.href, target, c.wantTarget)
			}
		})
	}
}
