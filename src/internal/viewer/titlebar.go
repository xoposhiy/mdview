package viewer

import (
	"syscall"
	"unsafe"

	"mdview/internal/renderer"
)

var (
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	// dwmwaUseImmersiveDarkMode asks DWM to draw the title bar and its
	// buttons dark. Windows 10 20H1 and later know it.
	dwmwaUseImmersiveDarkMode = 20
	// dwmwaCaptionColor and dwmwaTextColor set the exact colours of the
	// title bar and its text. Only Windows 11 knows them.
	dwmwaCaptionColor = 35
	dwmwaTextColor    = 36
)

// applyTitleBarTheme paints the window's title bar in the theme's page
// colours, so the title bar and the page look like one surface.
//
// The dark mode flag comes first and does the job on Windows 10, which
// ignores the two colour attributes; there the title bar is only dark or
// light. On a Windows version that knows none of the attributes the calls
// fail and the title bar stays as it was.
func applyTitleBarTheme(hwnd uintptr, theme string) {
	colors := renderer.ThemeColorsFor(theme)
	var dark uint32
	if colors.Dark() {
		dark = 1
	}
	setDwmAttribute(hwnd, dwmwaUseImmersiveDarkMode, dark)
	setDwmAttribute(hwnd, dwmwaCaptionColor, colorRef(colors.Bg))
	setDwmAttribute(hwnd, dwmwaTextColor, colorRef(colors.Fg))
}

func setDwmAttribute(hwnd uintptr, attr uintptr, value uint32) {
	procDwmSetWindowAttribute.Call(hwnd, attr, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
}

// colorRef converts 0xRRGGBB to a Win32 COLORREF, which is 0x00BBGGRR.
func colorRef(c renderer.RGB) uint32 {
	return uint32(c>>16&0xff) | uint32(c&0xff00) | uint32(c&0xff)<<16
}
