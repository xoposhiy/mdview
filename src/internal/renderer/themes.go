package renderer

import (
	"regexp"
	"strconv"
)

// RGB is a colour as 0xRRGGBB.
type RGB uint32

// ThemeColors are the page background and text colours of one theme.
type ThemeColors struct {
	Bg, Fg RGB
}

// Dark reports whether the theme has a dark page: its background is closer
// to black than to white.
func (c ThemeColors) Dark() bool {
	r, g, b := c.Bg>>16&0xff, c.Bg>>8&0xff, c.Bg&0xff
	// Rec. 601 luma, scaled by 1000.
	return 299*r+587*g+114*b < 128*1000
}

// themeBlock matches a top-level theme rule in template.html: the bare
// `body {` rule that holds the default theme, or a `body[data-theme="id"] {`
// rule. Only rules at the start of a line count, so the indented `body` rule
// inside `@media print` does not.
var themeBlock = regexp.MustCompile(`(?m)^body(?:\[data-theme="([^"]+)"\])? \{([^}]*)\}`)

var (
	bgVar = regexp.MustCompile(`--bg:\s*#([0-9a-fA-F]{6})`)
	fgVar = regexp.MustCompile(`--fg:\s*#([0-9a-fA-F]{6})`)
)

// themeColors is parsed once from the embedded template, so the window's
// title bar uses exactly the colours the page uses.
var themeColors = parseThemeColors(pageTemplate)

// ThemeColorsFor returns the colours of the theme with the given id. An
// unknown or empty id gets the default theme's colours, as the page does.
func ThemeColorsFor(id string) ThemeColors {
	if c, ok := themeColors[id]; ok {
		return c
	}
	return themeColors[defaultThemeID]
}

// defaultThemeID is the id of the theme that the bare `body {` rule defines.
const defaultThemeID = "light"

func parseThemeColors(page string) map[string]ThemeColors {
	colors := map[string]ThemeColors{}
	for _, m := range themeBlock.FindAllStringSubmatch(page, -1) {
		id, body := m[1], m[2]
		if id == "" {
			id = defaultThemeID
		}
		bg, fg := bgVar.FindStringSubmatch(body), fgVar.FindStringSubmatch(body)
		if bg == nil || fg == nil {
			continue
		}
		if _, seen := colors[id]; seen {
			continue
		}
		colors[id] = ThemeColors{Bg: parseHex(bg[1]), Fg: parseHex(fg[1])}
	}
	return colors
}

func parseHex(s string) RGB {
	n, _ := strconv.ParseUint(s, 16, 32)
	return RGB(n)
}
