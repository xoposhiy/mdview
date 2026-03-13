package viewer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"mdview/internal/config"

	webview "github.com/jchv/go-webview2"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procSendMessage   = user32.NewProc("SendMessageW")
	procLoadImage     = user32.NewProc("LoadImageW")
	procGetModHandle  = kernel32.NewProc("GetModuleHandleW")
	procShellExecute  = shell32.NewProc("ShellExecuteW")
)

const (
	imageIcon     = 1
	lrDefaultSize = 0x0040
	lrShared      = 0x8000
	wmSetIcon     = 0x0080
	iconSmall     = 0
	iconBig       = 1
)

func openInEditor(path string) {
	verb, _ := syscall.UTF16PtrFromString("edit")
	file, _ := syscall.UTF16PtrFromString(path)
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, 1)
}

func applyWindowIcon(hwnd unsafe.Pointer) {
	hInst, _, _ := procGetModHandle.Call(0)
	hIcon, _, _ := procLoadImage.Call(hInst, 1, imageIcon, 0, 0, lrDefaultSize|lrShared)
	if hIcon == 0 {
		return
	}
	procSendMessage.Call(uintptr(hwnd), wmSetIcon, iconSmall, hIcon)
	procSendMessage.Call(uintptr(hwnd), wmSetIcon, iconBig, hIcon)
}

// TitleFromPath returns the window title for the given file path.
func TitleFromPath(path string) string {
	return filepath.Base(path) + " — mdview"
}

// Viewer manages the WebView2 window.
type Viewer struct {
	w         webview.WebView
	filePath  string
	darkTheme bool
	renderFn  func([]byte, bool) (string, error)
}

// New creates a Viewer, binds hotkey handlers, and sets the initial HTML.
// renderFn is called on reload/navigate to convert file content to HTML.
func webviewDataPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mdview", "webview")
}

func New(filePath, initialHTML string, dark bool, renderFn func([]byte, bool) (string, error)) *Viewer {
	w := webview.NewWithOptions(webview.WebViewOptions{
		AutoFocus: true,
		DataPath:  webviewDataPath(),
	})
	w.SetTitle(TitleFromPath(filePath))
	w.SetSize(960, 720, webview.HintNone)

	v := &Viewer{w: w, filePath: filePath, darkTheme: dark, renderFn: renderFn}

	applyWindowIcon(w.Window())

	w.Bind("goExit", func() {
		w.Terminate()
	})

	w.Bind("goReload", func() {
		v.reload()
	})

	w.Bind("goNext", func() { v.navigate(+1) })
	w.Bind("goPrev", func() { v.navigate(-1) })
	w.Bind("goOpenEditor", func() { openInEditor(v.filePath) })

	w.Bind("goSaveTheme", func(dark bool) {
		v.darkTheme = dark
		theme := "light"
		if dark {
			theme = "dark"
		}
		config.Save(config.Config{Theme: theme})
	})

	w.SetHtml(initialHTML)
	return v
}

// Reload re-reads the file and updates the view. Safe to call from any goroutine.
func (v *Viewer) Reload() { v.reload() }

// Run starts the event loop. Blocks until the window is closed.
func (v *Viewer) Run() {
	v.w.Run()
	v.w.Destroy()
}

func (v *Viewer) reload() {
	content, err := os.ReadFile(v.filePath)
	if err != nil {
		return
	}
	html, err := v.renderFn(content, v.darkTheme)
	if err != nil {
		return
	}
	v.w.Dispatch(func() {
		v.w.SetHtml(html)
	})
}

func (v *Viewer) navigate(offset int) {
	dir := filepath.Dir(v.filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ".md" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	current := filepath.Base(v.filePath)
	idx := -1
	for i, f := range files {
		if strings.EqualFold(f, current) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	next := idx + offset
	if next < 0 || next >= len(files) {
		return
	}

	newPath := filepath.Join(dir, files[next])
	content, err := os.ReadFile(newPath)
	if err != nil {
		return
	}
	html, err := v.renderFn(content, v.darkTheme)
	if err != nil {
		return
	}
	v.filePath = newPath
	v.w.Dispatch(func() {
		v.w.SetTitle(TitleFromPath(newPath))
		v.w.SetHtml(html)
	})
}
