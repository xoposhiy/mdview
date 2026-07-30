package viewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"mdview/internal/applog"
	"mdview/internal/config"

	webview "github.com/jchv/go-webview2"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procSendMessage  = user32.NewProc("SendMessageW")
	procLoadImage    = user32.NewProc("LoadImageW")
	procGetModHandle = kernel32.NewProc("GetModuleHandleW")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

const (
	imageIcon     = 1
	lrDefaultSize = 0x0040
	lrShared      = 0x8000
	wmSetIcon     = 0x0080
	iconSmall     = 0
	iconBig       = 1
	swShowNormal  = 1
)

// openExternal hands target to the shell, which routes URLs to the default
// browser and local files to their registered application. It returns a
// message for the user, or "" on success.
func openExternal(target string) string {
	applog.Write("openExternal: %s", target)

	verb, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		applog.Write("openExternal: bad target: %v", err)
		return "Cannot open: " + target
	}
	ret, _, _ := procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	if ret <= 32 {
		applog.Write("openExternal: ShellExecute ret=%d", ret)
		return "Cannot open: " + target
	}
	return ""
}

func openInEditor(path string) {
	applog.Write("openInEditor: path=%s", path)

	// 1. Respect %EDITOR% / %VISUAL% env vars.
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if ed := os.Getenv(env); ed != "" {
			ed = strings.Trim(strings.TrimSpace(ed), `"`)
			applog.Write("openInEditor: using %s=%s", env, ed)
			cmd := exec.Command(ed, path)
			if err := cmd.Start(); err != nil {
				applog.Write("openInEditor: exec error: %v", err)
			}
			return
		}
	}

	// 2. Try ShellExecute "edit" verb.
	verb, _ := syscall.UTF16PtrFromString("edit")
	file, _ := syscall.UTF16PtrFromString(path)
	ret, _, _ := procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	applog.Write("openInEditor: ShellExecute edit ret=%d", ret)
	if ret > 32 {
		return
	}

	// 3. Fallback: notepad.
	applog.Write("openInEditor: falling back to notepad")
	if err := exec.Command("notepad.exe", path).Start(); err != nil {
		applog.Write("openInEditor: notepad error: %v", err)
	}
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
	w        webview.WebView
	filePath string
	prefs    config.Config
	scrollY  int
	history  history // documents visited before the current one
	tmpHTML  string  // temp file used for Navigate instead of SetHtml
	renderFn func([]byte, config.Config, int, string) (string, error)
}

// navigateHTML writes html to a temp file and navigates to it via file://.
// This gives WebView2 a file origin so local images load correctly.
func (v *Viewer) navigateHTML(html string) {
	if err := os.WriteFile(v.tmpHTML, []byte(html), 0644); err != nil {
		applog.Write("navigateHTML: WriteFile failed: %v", err)
		v.w.SetHtml(html) // fallback
		return
	}
	url := "file:///" + strings.ReplaceAll(v.tmpHTML, "\\", "/")
	v.w.Navigate(url)
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

func New(filePath, initialHTML string, prefs config.Config, renderFn func([]byte, config.Config, int, string) (string, error)) *Viewer {
	w := webview.NewWithOptions(webview.WebViewOptions{
		AutoFocus: true,
		DataPath:  webviewDataPath(),
	})
	w.SetTitle(TitleFromPath(filePath))
	w.SetSize(960, 720, webview.HintNone)

	tmp, _ := os.CreateTemp("", "mdview_*.html")
	tmp.Close()
	v := &Viewer{w: w, filePath: filePath, prefs: prefs, scrollY: 0, tmpHTML: tmp.Name(), renderFn: renderFn}

	applyWindowIcon(w.Window())

	w.Bind("goExit", func() {
		w.Terminate()
	})

	w.Bind("goReload", func() {
		v.reload()
	})

	w.Bind("goNext", func() string { return v.navigate(+1) })
	w.Bind("goPrev", func() string { return v.navigate(-1) })

	// goOpenLink receives the raw href attribute, not the anchor's resolved
	// href: the page is served from a temp file, so relative hrefs would
	// otherwise resolve against %TEMP% instead of the document's directory.
	w.Bind("goOpenLink", func(href string) string {
		kind, target := classifyLink(href, filepath.Dir(v.filePath))
		applog.Write("goOpenLink: href=%s kind=%d target=%s", href, kind, target)
		switch kind {
		case linkInPage:
			return ""
		case linkMarkdown:
			return v.open(target)
		default:
			return openExternal(target)
		}
	})

	w.Bind("goBack", func() string { return v.back() })
	w.Bind("goCanGoBack", func() bool { return v.history.canGoBack() })

	w.Bind("goSaveHTML", func() string {
		content, err := os.ReadFile(v.filePath)
		if err != nil {
			return "Error: " + err.Error()
		}
		html, err := v.renderFn(content, v.prefs, 0, filepath.Dir(v.filePath))
		if err != nil {
			return "Error: " + err.Error()
		}
		ext := filepath.Ext(v.filePath)
		outPath := v.filePath[:len(v.filePath)-len(ext)] + ".html"
		if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
			return "Error: " + err.Error()
		}
		return "Saved: " + filepath.Base(outPath)
	})

	w.Bind("goOpenEditor", func() {
		applog.Write("goOpenEditor called, filePath=%s", v.filePath)
		openInEditor(v.filePath)
	})

	w.Bind("goSaveScroll", func(y int) { v.scrollY = y })

	w.Bind("goSaveTheme", func(theme string) {
		v.prefs.Theme = theme
		config.Save(v.prefs)
	})

	w.Bind("goSaveFont", func(font string) {
		v.prefs.Font = font
		config.Save(v.prefs)
	})

	w.Bind("goSaveFontSize", func(size int) {
		v.prefs.FontSize = size
		config.Save(v.prefs)
	})

	v.navigateHTML(initialHTML)
	return v
}

// Reload re-reads the file and updates the view. Safe to call from any goroutine.
func (v *Viewer) Reload() { v.reload() }

// Run starts the event loop. Blocks until the window is closed.
func (v *Viewer) Run() {
	v.w.Run()
	v.w.Destroy()
	os.Remove(v.tmpHTML)
}

func (v *Viewer) reload() { v.show(v.filePath, v.scrollY) }

// show renders path into the window at the given scroll position. It updates
// v.filePath and v.scrollY but does not touch history.
func (v *Viewer) show(path string, scrollY int) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	html, err := v.renderFn(content, v.prefs, scrollY, filepath.Dir(path))
	if err != nil {
		return err
	}
	v.filePath = path
	v.scrollY = scrollY
	v.w.Dispatch(func() {
		v.w.SetTitle(TitleFromPath(path))
		v.navigateHTML(html)
	})
	return nil
}

// open records the current document in history, then shows path at the top.
// History is only pushed once the load succeeds, so a broken link leaves both
// the displayed document and the history untouched.
func (v *Viewer) open(path string) string {
	prevPath, prevScroll := v.filePath, v.scrollY
	if err := v.show(path, 0); err != nil {
		applog.Write("open: %s: %v", path, err)
		return "Not found: " + filepath.Base(path)
	}
	v.history.push(prevPath, prevScroll)
	return ""
}

// back returns to the previously visited document at the scroll position it was
// left at. A no-op when there is no history.
func (v *Viewer) back() string {
	entry, ok := v.history.pop()
	if !ok {
		return ""
	}
	if err := v.show(entry.path, entry.scrollY); err != nil {
		applog.Write("back: %s: %v", entry.path, err)
		return "Not found: " + filepath.Base(entry.path)
	}
	return ""
}

func (v *Viewer) navigate(offset int) string {
	dir := filepath.Dir(v.filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
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
		return ""
	}

	next := idx + offset
	if next < 0 || next >= len(files) {
		return ""
	}

	return v.open(filepath.Join(dir, files[next]))
}
