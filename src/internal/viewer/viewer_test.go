package viewer

import "testing"

func TestTitleFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"README.md", "README.md — mdview"},
		{`C:\docs\notes.md`, "notes.md — mdview"},
		{"/home/user/doc.md", "doc.md — mdview"},
		{"file.md", "file.md — mdview"},
	}
	for _, c := range cases {
		got := TitleFromPath(c.path)
		if got != c.want {
			t.Errorf("TitleFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
