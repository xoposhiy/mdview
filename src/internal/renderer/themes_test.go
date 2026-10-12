package renderer

import (
	"regexp"
	"testing"
)

// Every theme the page offers must have colours for the title bar, or that
// theme would silently get the default theme's title bar.
func TestEveryThemeHasColors(t *testing.T) {
	list := regexp.MustCompile(`(?s)var THEMES = \[(.*?)\];`).FindStringSubmatch(pageTemplate)
	if list == nil {
		t.Fatal("no THEMES list found in template.html")
	}
	ids := regexp.MustCompile(`\{ id: '([^']+)'`).FindAllStringSubmatch(list[1], -1)
	if len(ids) == 0 {
		t.Fatal("no THEMES entries found in template.html")
	}
	for _, m := range ids {
		if _, ok := themeColors[m[1]]; !ok {
			t.Errorf("theme %q has no --bg/--fg rule in template.html", m[1])
		}
	}
}

func TestThemeColorsFor(t *testing.T) {
	cases := []struct {
		id   string
		want ThemeColors
		dark bool
	}{
		{"light", ThemeColors{Bg: 0xfff2cf, Fg: 0x000000}, false},
		{"github", ThemeColors{Bg: 0xffffff, Fg: 0x1f2328}, false},
		{"dracula", ThemeColors{Bg: 0x282a36, Fg: 0xf8f8f2}, true},
		{"forest-dark", ThemeColors{Bg: 0x0d1612, Fg: 0xe0d8b8}, true},
		// The print rule sets white; it must not replace the default theme.
		{"", ThemeColors{Bg: 0xfff2cf, Fg: 0x000000}, false},
		{"no-such-theme", ThemeColors{Bg: 0xfff2cf, Fg: 0x000000}, false},
	}
	for _, c := range cases {
		got := ThemeColorsFor(c.id)
		if got != c.want {
			t.Errorf("ThemeColorsFor(%q) = %06x/%06x, want %06x/%06x", c.id, got.Bg, got.Fg, c.want.Bg, c.want.Fg)
		}
		if got.Dark() != c.dark {
			t.Errorf("ThemeColorsFor(%q).Dark() = %v, want %v", c.id, got.Dark(), c.dark)
		}
	}
}

func TestDarkThemes(t *testing.T) {
	want := map[string]bool{
		"light": false, "github": false, "solarized-light": false, "forest-light": false,
		"dark": true, "solarized-dark": true, "dracula": true, "nord": true, "forest-dark": true,
	}
	for id, dark := range want {
		if got := ThemeColorsFor(id).Dark(); got != dark {
			t.Errorf("%s: Dark() = %v, want %v", id, got, dark)
		}
	}
}
