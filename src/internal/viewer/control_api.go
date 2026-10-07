package viewer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"mdview/internal/applog"
	"mdview/internal/config"
	"mdview/internal/winshot"
)

// This file implements control.API: the outside-the-window interface used by
// automated checks. Every method routes through the same functions the real
// keyboard and mouse route through, so a check driven from here exercises the
// shipping code path and not a parallel one written for tests.

var (
	procIsIconic            = user32.NewProc("IsIconic")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// uiTimeout bounds every wait on the UI thread. A control request must never
// hang a test run: if the message loop is stuck, say so instead.
const uiTimeout = 10 * time.Second

// ImageState describes what the native image control is showing.
type ImageState struct {
	Visible     bool    `json:"visible"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	ZoomPercent int     `json:"zoomPercent"`
	Fit         bool    `json:"fit"`
	PanX        float64 `json:"panX"`
	PanY        float64 `json:"panY"`
	Bg          string  `json:"bg"`
	Toast       string  `json:"toast"`
}

// ViewerState is a snapshot of everything a check needs to know about what
// is on screen.
type ViewerState struct {
	File         string        `json:"file"`
	Mode         string        `json:"mode"` // "markdown" or "image"
	Title        string        `json:"title"`
	ScrollY      int           `json:"scrollY"`
	HistoryDepth int           `json:"historyDepth"`
	ClientWidth  int           `json:"clientWidth"`
	ClientHeight int           `json:"clientHeight"`
	Prefs        config.Config `json:"prefs"`
	Image        *ImageState   `json:"image,omitempty"`
}

// dispatchSync runs fn on the UI thread and waits for it to finish.
func (v *Viewer) dispatchSync(fn func()) error {
	done := make(chan struct{})
	v.w.Dispatch(func() {
		defer close(done)
		fn()
	})
	select {
	case <-done:
		return nil
	case <-time.After(uiTimeout):
		return errors.New("timed out waiting for the UI thread")
	}
}

// State implements control.API.
func (v *Viewer) State() any {
	// The translation settings in v.prefs are the ones from startup; goTranslate
	// reads the file on every request, so report what the file holds now. Never
	// report the key itself, only whether there is one.
	tr := config.Load().Translate
	if tr.DeepLKey != "" {
		tr.DeepLKey = "(set)"
	}
	var st ViewerState
	v.dispatchSync(func() {
		st = ViewerState{
			File:         v.filePath,
			Mode:         v.mode(),
			Title:        v.title,
			ScrollY:      v.scrollY,
			HistoryDepth: len(v.history),
			Prefs:        v.prefs,
		}
		st.Prefs.Translate = tr
		st.ClientWidth, st.ClientHeight = clientSizeOf(uintptr(v.w.Window()))
		if img := v.img; img != nil && img.zoom != nil {
			st.Image = &ImageState{
				Visible:     img.visible,
				Width:       img.zoom.imgW,
				Height:      img.zoom.imgH,
				ZoomPercent: img.zoom.Percent(),
				Fit:         img.zoom.fit,
				PanX:        img.zoom.panX,
				PanY:        img.zoom.panY,
				Bg:          img.bg,
				Toast:       img.toast,
			}
		}
	})
	return st
}

// mode reports which view is on screen. It reads UI-thread state, so every
// caller must already be on the UI thread.
func (v *Viewer) mode() string {
	if v.imageOnScreen() {
		return "image"
	}
	return "markdown"
}

func (v *Viewer) imageOnScreen() bool { return v.img != nil && v.img.visible }

// Metrics implements control.API.
func (v *Viewer) Metrics() any { return v.metrics.snapshot() }

// Key implements control.API. While an image is on screen the key goes to the
// image control's own handler; otherwise it is dispatched as a keydown event
// into the page, where the Markdown view's JS handler picks it up.
func (v *Viewer) Key(name string, mods []string) error {
	ctrl, alt, shift := parseMods(mods)
	vk, jsKey, ok := lookupKey(name)
	if !ok {
		return fmt.Errorf("unknown key %q", name)
	}
	js := fmt.Sprintf(
		`document.dispatchEvent(new KeyboardEvent('keydown',`+
			`{key:%s,ctrlKey:%t,altKey:%t,shiftKey:%t,bubbles:true,cancelable:true}))`,
		jsString(jsKey), ctrl, alt, shift)

	// Which view is on screen is UI-thread state, so choose the target on the
	// UI thread too.
	return v.dispatchSync(func() {
		if v.imageOnScreen() {
			v.img.handleKey(uintptr(vk), ctrl, alt)
			return
		}
		v.w.Eval(js)
	})
}

// Mouse implements control.API. Mouse actions only apply to the image view.
func (v *Viewer) Mouse(action string, x, y, dx, dy, delta int) error {
	var onScreen bool
	err := v.dispatchSync(func() {
		if onScreen = v.imageOnScreen(); !onScreen {
			return
		}
		c := v.img
		switch action {
		case "wheel":
			c.wheelAt(delta, int32(x), int32(y))
		case "down":
			c.beginDrag(int32(x), int32(y))
		case "move":
			c.dragTo(int32(x), int32(y))
		case "up":
			c.endDrag()
		case "middle":
			c.resetZoom()
		case "drag":
			c.beginDrag(int32(x), int32(y))
			c.dragTo(int32(x+dx), int32(y+dy))
			c.endDrag()
		default:
			applog.Write("Mouse: unknown action %q", action)
		}
	})
	if err != nil {
		return err
	}
	if !onScreen {
		return errors.New("mouse actions apply to the image view only")
	}
	return nil
}

// evalWrapper runs one expression in the page and sends the value back
// through the goEvalResult binding, because webview.Eval by itself is
// fire-and-forget. A promise is waited for, so a check can measure something
// that takes time (frames painted, MathJax finishing) and not just read a
// value that is already there.
//
// Format arguments, in order: result id, the expression, result id again.
const evalWrapper = `(function(){` +
	`function send(v){try{goEvalResult(%[1]s,JSON.stringify(v===undefined?null:v));}` +
	`catch(e){goEvalResult(%[1]s,JSON.stringify({error:String(e)}));}}` +
	`try{var r=(%[2]s);` +
	`if(r&&typeof r.then==="function"){r.then(send,function(e){send({error:String(e)})});}` +
	`else{send(r);}}` +
	`catch(e){send({error:String(e)});}})()`

// Eval implements control.API: it runs js in the page and returns its value,
// JSON-encoded.
func (v *Viewer) Eval(js string, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	id, ch := v.evals.begin()
	defer v.evals.end(id)

	wrapped := fmt.Sprintf(evalWrapper, jsString(id), js)
	var onScreen bool
	if err := v.dispatchSync(func() {
		if onScreen = v.imageOnScreen(); !onScreen {
			v.w.Eval(wrapped)
		}
	}); err != nil {
		return nil, err
	}
	if onScreen {
		return nil, errors.New("no page to evaluate: an image is on screen")
	}

	select {
	case raw := <-ch:
		if raw == "" {
			raw = "null"
		}
		return json.RawMessage(raw), nil
	case <-time.After(timeout):
		return nil, errors.New("the page did not answer in time")
	}
}

// Open implements control.API.
func (v *Viewer) Open(path string) (string, error) {
	var msg string
	err := v.dispatchSync(func() { msg = v.open(path) })
	return msg, err
}

// Resize implements control.API.
func (v *Viewer) Resize(w, h int) error {
	return v.dispatchSync(func() { v.w.SetSize(w, h, 0) })
}

// Focus implements control.API.
func (v *Viewer) Focus() error {
	hwnd := uintptr(v.w.Window())
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
	return nil
}

// Screenshot implements control.API.
//
// The capture must not run on the UI thread: it waits for the window to
// repaint, and a wait on the UI thread would block the very repaint it waits
// for.
func (v *Viewer) Screenshot(path string, clientOnly bool, mode string) error {
	return winshot.CaptureWindow(uintptr(v.w.Window()), path, winshot.Options{
		Mode:       winshot.Mode(mode),
		ClientOnly: clientOnly,
	})
}

// Quit implements control.API.
//
// Terminate posts WM_QUIT to the calling thread, so it only ends the message
// loop when it runs on the UI thread. Dispatch (not dispatchSync) is what
// gets it there: the loop stops as a result, so there is nothing to wait for.
func (v *Viewer) Quit() error {
	v.w.Dispatch(func() { v.w.Terminate() })
	return nil
}

func clientSizeOf(hwnd uintptr) (int, int) {
	var r winRect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// evalRegistry matches each Eval call to the goEvalResult callback that
// answers it.
type evalRegistry struct {
	mu      sync.Mutex
	next    int
	pending map[string]chan string
}

func newEvalRegistry() *evalRegistry {
	return &evalRegistry{pending: map[string]chan string{}}
}

func (r *evalRegistry) begin() (string, chan string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	id := fmt.Sprintf("e%d", r.next)
	ch := make(chan string, 1)
	r.pending[id] = ch
	return id, ch
}

func (r *evalRegistry) end(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
}

// deliver hands a result to the waiting Eval call. Unknown ids are dropped:
// they belong to a call that already timed out.
func (r *evalRegistry) deliver(id, value string) {
	r.mu.Lock()
	ch := r.pending[id]
	r.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- value:
	default:
	}
}

func parseMods(mods []string) (ctrl, alt, shift bool) {
	for _, m := range mods {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "ctrl", "control":
			ctrl = true
		case "alt":
			alt = true
		case "shift":
			shift = true
		}
	}
	return
}

// keyNames maps the names the control interface accepts to a Win32 virtual
// key code and the value the browser reports as KeyboardEvent.key. Both views
// must answer to the same name, or a test would have to know which view is on
// screen.
var keyNames = map[string]struct {
	vk int
	js string
}{
	"left":      {vkLeft, "ArrowLeft"},
	"right":     {vkRight, "ArrowRight"},
	"up":        {vkUp, "ArrowUp"},
	"down":      {vkDown, "ArrowDown"},
	"escape":    {vkEscape, "Escape"},
	"esc":       {vkEscape, "Escape"},
	"backspace": {vkBack, "Backspace"},
	"f5":        {vkF5, "F5"},
	"home":      {vkHome, "Home"},
	"end":       {vkEnd, "End"},
	"pageup":    {vkPageUp, "PageUp"},
	"pagedown":  {vkPageDown, "PageDown"},
	"space":     {vkSpace, " "},
}

func lookupKey(name string) (int, string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if k, ok := keyNames[name]; ok {
		return k.vk, k.js, true
	}
	// Single printable character: letters and digits reach the native view as
	// their upper-case ASCII code, which is also their virtual key code.
	if len(name) == 1 {
		ch := name[0]
		if ch >= 'a' && ch <= 'z' {
			return int(ch - 'a' + 'A'), name, true
		}
		if ch >= '0' && ch <= '9' {
			return int(ch), name, true
		}
		if ch == '?' {
			return int('?'), "?", true
		}
	}
	return 0, "", false
}

// jsString quotes s as a JavaScript string literal.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
