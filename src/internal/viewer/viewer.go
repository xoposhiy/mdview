package viewer

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"mdview/internal/applog"
	"mdview/internal/config"
	"mdview/internal/imgdecode"
	"mdview/internal/tabular"
	"mdview/internal/translate"

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

// The window's opening size, in DPI-independent units: what 960x720 meant
// before mdview declared itself DPI aware.
const (
	defaultWindowWidth  = 960
	defaultWindowHeight = 720
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

// TitleFromPathZoom is TitleFromPath with the current zoom level appended,
// used while an image is on screen.
func TitleFromPathZoom(path string, zoomPercent int) string {
	return filepath.Base(path) + " — mdview (" + strconv.Itoa(zoomPercent) + "%)"
}

// RenderFunc turns Markdown into a complete HTML page.
type RenderFunc func(content []byte, prefs config.Config, scrollY int, baseDir string) (string, error)

// Renderers are the two ways a Viewer needs a document turned into HTML.
// Display may point the page at files on this machine by absolute path,
// because only this WebView ever opens it. Standalone must inline everything:
// its output is written next to the Markdown file and is meant to be opened
// anywhere.
type Renderers struct {
	Display    RenderFunc
	Standalone RenderFunc
}

// Viewer manages the WebView2 window and, for image files, the native
// imageControl that replaces it on screen.
type Viewer struct {
	w        webview.WebView
	filePath string
	title    string
	prefs    config.Config
	scrollY  int
	history  history // documents visited before the current one
	tmpHTML  string  // temp file used for Navigate instead of SetHtml
	render   Renderers

	// img is created the first time an image is displayed, not at startup:
	// a session that only ever shows Markdown pays nothing for the image
	// view — no window class, no child window, and above all no subclassed
	// parent WndProc funnelling every window message through Go.
	img        *imageControl
	resizeHook bool
	metrics    *metricsStore
	evals      *evalRegistry

	// Selection translation. All three are touched only on the UI thread.
	// transSeq numbers the requests, so a late reply for an older request
	// or for a page that is gone is dropped.
	translator    translate.Translator
	translatorKey string
	transSeq      int
}

// navigateHTML writes html to a temp file and navigates to it via file://.
// This gives WebView2 a file origin so local images load correctly.
func (v *Viewer) navigateHTML(html string) {
	v.metrics.setSize("htmlBytes", len(html))

	write := v.metrics.track(stageWriteHTML)
	err := os.WriteFile(v.tmpHTML, []byte(html), 0644)
	write()
	if err != nil {
		applog.Write("navigateHTML: WriteFile failed: %v", err)
		v.w.SetHtml(html) // fallback
		return
	}

	defer v.metrics.track(stageNavigate)()
	url := "file:///" + strings.ReplaceAll(v.tmpHTML, "\\", "/")
	v.w.Navigate(url)
}

// setTitle sets the window title and remembers it, so the control interface
// can report what the user sees without asking Win32 for it.
func (v *Viewer) setTitle(title string) {
	v.title = title
	v.w.SetTitle(title)
}

// New creates a Viewer, binds hotkey handlers, and sets the initial HTML.
// render.Display is called on reload/navigate to convert file content to HTML.
func webviewDataPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mdview", "webview")
}

func New(filePath string, prefs config.Config, render Renderers) (*Viewer, error) {
	// Everything derived from the path (image src URLs, data-path, link
	// targets, the file list for arrow keys) assumes an absolute path. A
	// relative path from the terminal would otherwise be joined with the
	// document's directory a second time when a link or image is clicked.
	if abs, err := filepath.Abs(filePath); err == nil {
		filePath = abs
	}
	tmp, _ := os.CreateTemp("", "mdview_*.html")
	tmp.Close()
	v := &Viewer{
		filePath: filePath,
		prefs:    prefs,
		scrollY:  0,
		tmpHTML:  tmp.Name(),
		render:   render,
		metrics:  newMetricsStore(),
		evals:    newEvalRegistry(),
	}

	// Load the file before creating any window. A bad path or a corrupt file
	// then exits with a message on stderr instead of flashing an empty
	// window — and the file is still read and rendered exactly once.
	doc, err := v.load(filePath, 0)
	if err != nil {
		os.Remove(v.tmpHTML)
		return nil, err
	}

	w := webview.NewWithOptions(webview.WebViewOptions{
		AutoFocus: true,
		DataPath:  webviewDataPath(),
	})
	v.w = w
	width, height := scaleForDPI(uintptr(w.Window()), defaultWindowWidth, defaultWindowHeight)
	w.SetSize(width, height, webview.HintNone)
	v.setTitle(TitleFromPath(filePath))

	applyWindowIcon(w.Window())
	applyTitleBarTheme(uintptr(w.Window()), prefs.Theme)

	w.Bind("goEvalResult", func(id, value string) {
		v.evals.deliver(id, value)
	})

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
		case linkViewable:
			return v.open(target)
		default:
			return openExternal(target)
		}
	})

	w.Bind("goBack", func() string { return v.back() })
	w.Bind("goCanGoBack", func() bool { return v.history.canGoBack() })

	w.Bind("goSaveHTML", func() string { return v.saveHTMLAs() })

	w.Bind("goReveal", func() string { return revealInExplorer(v.filePath) })

	w.Bind("goOpenEditor", func() {
		applog.Write("goOpenEditor called, filePath=%s", v.filePath)
		openInEditor(v.filePath)
	})

	w.Bind("goSaveScroll", func(y int) { v.scrollY = y })

	w.Bind("goSaveTheme", func(theme string) {
		v.prefs.Theme = theme
		applyTitleBarTheme(uintptr(w.Window()), theme)
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

	w.Bind("goTranslate", v.goTranslate)

	v.display(doc)
	return v, nil
}

// imageView returns the image control, creating it on first use. Must run on
// the UI thread.
func (v *Viewer) imageView() *imageControl {
	if v.img != nil {
		return v.img
	}
	w := v.w
	c := newImageControl(uintptr(w.Window()), v.prefs.ImageBg)
	c.onNext = func() string { return v.navigate(+1) }
	c.onPrev = func() string { return v.navigate(-1) }
	c.onBack = func() string { return v.back() }
	c.onReload = func() { v.reload() }
	c.onExit = func() { w.Terminate() }
	c.onEditor = func() { openInEditor(v.filePath) }
	c.onSave = func() string { return v.saveImageAs() }
	c.onReveal = func() string { return revealInExplorer(v.filePath) }
	c.onBgChanged = func(bg string) {
		v.prefs.ImageBg = bg
		config.Save(v.prefs)
	}
	c.onZoom = func(percent int) {
		v.setTitle(TitleFromPathZoom(v.filePath, percent))
	}
	c.metrics = v.metrics
	v.img = c

	if !v.resizeHook {
		v.resizeHook = true
		installParentHook(uintptr(w.Window()),
			func(width, height int) { v.img.Resize(width, height) },
			func() {
				if v.img.visible {
					v.img.takeFocus()
				}
			})
	}
	return c
}

// saveHTMLAs asks the user where to put a standalone copy of the current
// document and writes it there. It returns a message for the user, or "" if
// the user cancelled.
//
// The dialog is modal, so this must run on the UI thread — which it does: it
// is called from a JS binding and from the image control's key handler.
func (v *Viewer) saveHTMLAs() string {
	ext := filepath.Ext(v.filePath)
	suggested := v.filePath[:len(v.filePath)-len(ext)] + ".html"
	outPath := saveFileDialog(uintptr(v.w.Window()), "Save as HTML", suggested,
		"HTML files (*.html)|*.html|All files (*.*)|*.*", "html")
	if outPath == "" {
		return ""
	}

	content, err := os.ReadFile(v.filePath)
	if err != nil {
		return "Error: " + err.Error()
	}
	content, err = tabular.Prepare(v.filePath, content)
	if err != nil {
		return "Error: " + err.Error()
	}
	html, err := v.render.Standalone(content, v.prefs, 0, filepath.Dir(v.filePath))
	if err != nil {
		return "Error: " + err.Error()
	}
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return "Error: " + err.Error()
	}
	return "Saved: " + filepath.Base(outPath)
}

// saveImageAs asks the user where to put a copy of the image on screen and
// copies the file there, bytes as they are on disk: the same format, the same
// quality, no re-encoding.
func (v *Viewer) saveImageAs() string {
	src := v.filePath
	ext := strings.ToLower(filepath.Ext(src))
	filter := "All files (*.*)|*.*"
	if ext != "" {
		filter = "Image (*" + ext + ")|*" + ext + "|" + filter
	}
	outPath := saveFileDialog(uintptr(v.w.Window()), "Save image as", src,
		filter, strings.TrimPrefix(ext, "."))
	if outPath == "" {
		return ""
	}
	if strings.EqualFold(filepath.Clean(outPath), filepath.Clean(src)) {
		return "Same file: " + filepath.Base(src)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return "Error: " + err.Error()
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "Error: " + err.Error()
	}
	return "Saved: " + filepath.Base(outPath)
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

// loadedDoc is a file already turned into something displayable: a rendered
// HTML page, or a decoded image. Splitting loading from displaying is what
// lets the first file be loaded before the window exists.
type loadedDoc struct {
	path    string
	scrollY int
	html    string      // Markdown documents
	img     image.Image // images
}

// show loads path and displays it at the given scroll position (images ignore
// scrollY). It updates v.filePath and v.scrollY but does not touch history.
func (v *Viewer) show(path string, scrollY int) error {
	doc, err := v.load(path, scrollY)
	if err != nil {
		return err
	}
	v.display(doc)
	return nil
}

// load reads path and prepares it for display. Safe to call from any
// goroutine, and it touches no window, so it can run before one exists.
func (v *Viewer) load(path string, scrollY int) (loadedDoc, error) {
	if imgdecode.IsImage(filepath.Ext(path)) {
		decode := v.metrics.track(stageDecode)
		img, err := imgdecode.Decode(path)
		decode()
		if err != nil {
			return loadedDoc{}, err
		}
		b := img.Bounds()
		v.metrics.setSize("imagePixels", b.Dx()*b.Dy())
		return loadedDoc{path: path, img: img}, nil
	}

	read := v.metrics.track(stageReadFile)
	content, err := os.ReadFile(path)
	read()
	if err != nil {
		return loadedDoc{}, err
	}

	render := v.metrics.track(stageRender)
	content, err = tabular.Prepare(path, content)
	if err != nil {
		render()
		return loadedDoc{}, err
	}
	html, err := v.render.Display(content, v.prefs, scrollY, filepath.Dir(path))
	render()
	if err != nil {
		return loadedDoc{}, err
	}
	return loadedDoc{path: path, scrollY: scrollY, html: html}, nil
}

// display puts an already-loaded document on screen. The work itself runs on
// the UI thread, so display is safe to call from any goroutine.
func (v *Viewer) display(doc loadedDoc) {
	v.filePath = doc.path
	v.scrollY = doc.scrollY
	v.w.Dispatch(func() {
		v.transSeq++ // drop a translation that arrives for the old page
		if doc.img != nil {
			c := v.imageView()
			c.SetImage(doc.img)
			c.Show()
			return
		}
		if v.img != nil {
			v.img.Hide()
		}
		v.setTitle(TitleFromPath(doc.path))
		v.navigateHTML(doc.html)
	})
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
		if !e.IsDir() && isViewableExt(filepath.Ext(e.Name())) {
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
