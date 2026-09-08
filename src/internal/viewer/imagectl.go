package viewer

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
	"syscall"
	"unsafe"
)

// Win32 bindings for the native image control. viewer.go already declares
// user32, kernel32 and the handful of procs it needs; everything else the
// control needs lives here.
var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procMoveWindow        = user32.NewProc("MoveWindow")
	procBringWindowToTop  = user32.NewProc("BringWindowToTop")
	procSetFocus2         = user32.NewProc("SetFocus")
	procGetFocus          = user32.NewProc("GetFocus")
	procInvalidateRect    = user32.NewProc("InvalidateRect")
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procGetClientRect     = user32.NewProc("GetClientRect")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procSetCapture        = user32.NewProc("SetCapture")
	procReleaseCapture    = user32.NewProc("ReleaseCapture")
	procScreenToClient    = user32.NewProc("ScreenToClient")
	procGetKeyState       = user32.NewProc("GetKeyState")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW   = user32.NewProc("CallWindowProcW")
	procFillRect          = user32.NewProc("FillRect")
	procDrawTextW         = user32.NewProc("DrawTextW")
	procLoadCursorW       = user32.NewProc("LoadCursorW")
	procSetTimer          = user32.NewProc("SetTimer")
	procKillTimer         = user32.NewProc("KillTimer")
	procOpenClipboard     = user32.NewProc("OpenClipboard")
	procEmptyClipboard    = user32.NewProc("EmptyClipboard")
	procSetClipboardData  = user32.NewProc("SetClipboardData")
	procCloseClipboard    = user32.NewProc("CloseClipboard")

	procSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procSetBrushOrgEx          = gdi32.NewProc("SetBrushOrgEx")
	procStretchDIBits          = gdi32.NewProc("StretchDIBits")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalFree   = kernel32.NewProc("GlobalFree")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
)

// defaultDPI is the DPI Windows calls 100% scaling.
const defaultDPI = 96

// dpiAwarenessPerMonitorV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2.
// It's one of the negative pseudo-handle constants Win32 defines for this
// API; -4 kept in a var (not a const) so the uintptr conversion below is a
// runtime sign-extension rather than a constant-overflow compile error.
var dpiAwarenessPerMonitorV2 = int32(-4)

// EnableDPIAwareness declares the process Per-Monitor-V2 DPI aware, so a
// GetClientRect on any window returns physical pixels: on a HiDPI monitor,
// 100% zoom in the image view means one image pixel per screen pixel
// instead of a blurry OS-stretched window. Must be called before any window
// is created.
func EnableDPIAwareness() {
	procSetProcessDpiAwarenessContext.Call(uintptr(dpiAwarenessPerMonitorV2))
}

// scaleForDPI converts a size in DPI-independent units to physical pixels for
// the monitor the window is on.
//
// Declaring the process DPI aware means every size Win32 takes is in physical
// pixels. Without this conversion the window would open 960x720 physical —
// two thirds the size it had before, on a display scaled to 150% — so the
// document would get a 640x480 viewport and need far more scrolling for the
// same text.
func scaleForDPI(hwnd uintptr, width, height int) (int, int) {
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return width, height
	}
	return width * int(dpi) / defaultDPI, height * int(dpi) / defaultDPI
}

const (
	wmDestroy        = 0x0002
	wmActivate       = 0x0006
	wmSetFocus       = 0x0007
	wmSize           = 0x0005
	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmClose          = 0x0010
	wmCaptureChanged = 0x0215
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmMButtonDown    = 0x0207
	wmMouseWheel     = 0x020A
	wmKeyDown        = 0x0100
	wmTimer          = 0x0113
	wmGetDlgCode     = 0x0087

	waInactive = 0

	dlgcWantArrows  = 0x0001
	dlgcWantTab     = 0x0002
	dlgcWantAllKeys = 0x0004

	vkBack     = 0x08
	vkEscape   = 0x1B
	vkSpace    = 0x20
	vkPageUp   = 0x21
	vkPageDown = 0x22
	vkEnd      = 0x23
	vkHome     = 0x24
	vkLeft     = 0x25
	vkUp       = 0x26
	vkRight    = 0x27
	vkDown     = 0x28
	vkF5       = 0x74
	vkCtrl     = 0x11
	vkMenu     = 0x12 // Alt

	wsChild   = 0x40000000
	wsVisible = 0x10000000

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	swHide    = 0
	swShow    = 5
	swRestore = 9

	halftone      = 4
	colorOnColor  = 3
	cfDIB         = 8
	gmemMoveable  = 0x0002
	dtCenter      = 0x0001
	dtVCenter     = 0x0004
	dtSingleLine  = 0x0020
	srcCopy       = 0x00CC0020
	transparentBk = 1
	idcArrow      = 32512
	toastTimerID  = 1
	toastMillis   = 2500
	// While the user zooms or pans, the image is scaled the fast way and a
	// quality repaint follows once the gesture stops for this long.
	qualityTimerID = 2
	qualityMillis  = 90
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

type paintStruct struct {
	Hdc        uintptr
	FErase     int32
	RcPaint    winRect
	FRestore   int32
	FIncUpdate int32
	Reserved   [32]byte
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

const imageClassNameStr = "mdview-image"

var (
	imageClassOnce sync.Once
	imageClassName *uint16

	// The window class is registered once per process, so its WndProc cannot
	// capture one control: it looks the owning control up by window handle
	// instead.
	imageControlsMu sync.Mutex
	imageControls   = map[uintptr]*imageControl{}
	// creatingControl owns the window currently being created. Windows sends
	// messages during CreateWindowExW, before the handle is known, so the
	// dispatcher hands those to this control.
	creatingControl *imageControl
)

// imageControlFor returns the control that owns hwnd, adopting the handle for
// the control being created if this is the first message it has seen.
func imageControlFor(hwnd uintptr) *imageControl {
	imageControlsMu.Lock()
	defer imageControlsMu.Unlock()
	if c := imageControls[hwnd]; c != nil {
		return c
	}
	if c := creatingControl; c != nil {
		imageControls[hwnd] = c
		return c
	}
	return nil
}

// imageDispatchProc is the window class's only WndProc.
func imageDispatchProc(hwnd, msg, wp, lp uintptr) uintptr {
	if c := imageControlFor(hwnd); c != nil {
		return c.wndProc(hwnd, msg, wp, lp)
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return ret
}

// registerImageClass registers the window class used by every imageControl.
func registerImageClass() {
	imageClassOnce.Do(func() {
		name, _ := syscall.UTF16PtrFromString(imageClassNameStr)
		imageClassName = name

		hInst, _, _ := procGetModHandle.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)

		wc := wndClassExW{
			CbSize: uint32(unsafe.Sizeof(wndClassExW{})),
			// CS_HREDRAW|CS_VREDRAW: without these, Windows only invalidates
			// the newly-exposed sliver on a resize, not the whole client
			// area. BeginPaint clips drawing to that invalid region, so
			// without this flag a resize leaves stale pixels around the
			// image (the resize artifacts bug).
			Style:         csHRedraw | csVRedraw,
			LpfnWndProc:   syscall.NewCallback(imageDispatchProc),
			HInstance:     hInst,
			HCursor:       cursor,
			LpszClassName: name,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

// imageControl is a native child window that displays a single raster image
// with Fit/zoom/pan, replacing the WebView2 content for the duration an
// image file is open. It is created once per Viewer and shown/hidden as the
// user navigates between Markdown and image files.
type imageControl struct {
	hwnd   uintptr
	parent uintptr

	src image.Image // the decoded image, as-is (may have transparency)

	bg     string // "black" or "white"
	packed []byte // bg-composited BGRA pixels, top-down, for StretchDIBits
	pw, ph int    // packed buffer dimensions

	zoom *zoomState

	visible      bool
	dragging     bool
	dragX, dragY int32
	toast        string

	// Off-screen buffer the client area is drawn into, kept between paints
	// and rebuilt only when the client size changes.
	bufDC, bufBmp, bufOldBmp uintptr
	bufW, bufH               int

	// fastScale is on while the user is zooming or panning: the image is
	// then scaled the cheap way, and a quality repaint follows once the
	// gesture stops.
	fastScale bool

	savedFocus uintptr

	// metrics is the owning Viewer's store; the image view records its
	// decode/pack/paint times there.
	metrics *metricsStore

	// Callbacks wired by the owning Viewer; all run on the UI thread.
	onNext, onPrev, onBack func() string
	onReload, onExit       func()
	onEditor               func()
	onSave, onReveal       func() string
	onBgChanged            func(bg string)
	onZoom                 func(percent int)
}

func newImageControl(parent uintptr, bg string) *imageControl {
	c := &imageControl{parent: parent, bg: bg}
	registerImageClass()

	var r winRect
	procGetClientRect.Call(parent, uintptr(unsafe.Pointer(&r)))
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w <= 0 {
		w = 960
	}
	if h <= 0 {
		h = 720
	}

	imageControlsMu.Lock()
	creatingControl = c
	imageControlsMu.Unlock()

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(imageClassName)),
		0,
		uintptr(wsChild),
		0, 0, uintptr(w), uintptr(h),
		parent,
		0,
		0,
		0,
	)

	imageControlsMu.Lock()
	creatingControl = nil
	if hwnd != 0 {
		imageControls[hwnd] = c
	}
	imageControlsMu.Unlock()

	c.hwnd = hwnd
	return c
}

// Resize matches the control to the parent's new client size. Called from
// the parent window's WM_SIZE via the resize hook installed in viewer.go.
func (c *imageControl) Resize(w, h int) {
	if c.hwnd == 0 {
		return
	}
	procMoveWindow.Call(c.hwnd, 0, 0, uintptr(w), uintptr(h), 1)
}

// Show raises the control above the (still-present, now-covered) WebView2
// content and gives it keyboard focus. A no-op if already visible, so
// re-displaying a new image (via SetImage) never redoes the
// show/raise/focus dance.
func (c *imageControl) Show() {
	if c.visible {
		return
	}
	c.visible = true
	c.savedFocus, _, _ = procGetFocus.Call()
	procShowWindow.Call(c.hwnd, swShow)
	procBringWindowToTop.Call(c.hwnd)
	procSetFocus2.Call(c.hwnd)
}

// takeFocus gives the control the keyboard focus, unless it already has it.
//
// Every time the window is activated again — alt-tab, a click after another
// app was in front, a modal dialog closing — WebView2 pulls the focus into
// its own child window, even while the image covers it. Key presses would
// then go to a page nobody can see, and the image view's shortcuts would
// stop working, so the control takes the focus back.
func (c *imageControl) takeFocus() {
	if c.hwnd == 0 {
		return
	}
	if cur, _, _ := procGetFocus.Call(); cur == c.hwnd {
		return
	}
	procSetFocus2.Call(c.hwnd)
}

// Hide restores keyboard focus to whatever held it before Show, so the
// WebView2 content underneath (now visible again) can receive input. A
// no-op if already hidden — every Markdown display calls this, so it must
// not touch focus/visibility (and risk disturbing WebView2) when there is
// nothing to undo.
func (c *imageControl) Hide() {
	if !c.visible {
		return
	}
	c.visible = false
	procShowWindow.Call(c.hwnd, swHide)
	if c.savedFocus != 0 {
		procSetFocus2.Call(c.savedFocus)
		c.savedFocus = 0
	}
}

// SetImage decodes buffer into the control's display state and lays it out
// against the control's current size.
func (c *imageControl) SetImage(img image.Image) {
	c.src = img
	b := img.Bounds()
	c.zoom = newZoomState(b.Dx(), b.Dy())
	c.rebuildPacked()
	c.layoutAndInvalidate()
}

// SetBg changes the letterbox/transparency background color and persists
// the choice via onBgChanged.
func (c *imageControl) SetBg(bg string) {
	if bg == c.bg {
		return
	}
	c.bg = bg
	if c.src != nil {
		c.rebuildPacked()
	}
	c.invalidate()
	if c.onBgChanged != nil {
		c.onBgChanged(bg)
	}
}

func (c *imageControl) bgColor() color.RGBA {
	if c.bg == "white" {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{0, 0, 0, 255}
}

// rebuildPacked composites c.src over the current background color into a
// top-down 32bpp BGRA buffer ready for StretchDIBits.
//
// This runs on every image load, so it works over the pixels as few times as
// it can: the background is filled only for an image that can be
// transparent, and RGBA is turned into the BGRA byte order GDI wants by
// swapping two bytes in place rather than copying into a second buffer.
func (c *imageControl) rebuildPacked() {
	if c.src == nil {
		return
	}
	if c.metrics != nil {
		defer c.metrics.track(stagePack)()
	}

	b := c.src.Bounds()
	w, h := b.Dx(), b.Dy()

	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	if isOpaque(c.src) {
		draw.Draw(canvas, canvas.Bounds(), c.src, b.Min, draw.Src)
	} else {
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(c.bgColor()), image.Point{}, draw.Src)
		draw.Draw(canvas, canvas.Bounds(), c.src, b.Min, draw.Over)
	}

	pix := canvas.Pix
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
	c.packed = pix
	c.pw, c.ph = w, h
}

// isOpaque reports whether img has no transparent pixel, so compositing it
// over a background is pointless work. Every stdlib image type answers this
// question; a type that does not is assumed to have transparency.
func isOpaque(img image.Image) bool {
	o, ok := img.(interface{ Opaque() bool })
	return ok && o.Opaque()
}

func (c *imageControl) clientSize() (int, int) {
	var r winRect
	procGetClientRect.Call(c.hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

func (c *imageControl) layoutAndInvalidate() {
	if c.zoom == nil {
		return
	}
	w, h := c.clientSize()
	c.zoom.Layout(w, h)
	c.invalidate()
	c.reportZoom()
}

func (c *imageControl) reportZoom() {
	if c.onZoom != nil && c.zoom != nil {
		c.onZoom(c.zoom.Percent())
	}
}

func (c *imageControl) invalidate() {
	procInvalidateRect.Call(c.hwnd, 0, 0)
}

// backBuffer returns a memory DC the size of the client area with an
// off-screen bitmap selected into it, creating one on first use and after a
// resize. Painting goes there and reaches the window as a single BitBlt, so
// a repaint never shows the background before the image (the black flash
// while zooming). It returns 0 if GDI refuses, and the caller then paints
// straight to the window.
func (c *imageControl) backBuffer(hdc uintptr, w, h int) uintptr {
	if c.bufDC != 0 && c.bufW == w && c.bufH == h {
		return c.bufDC
	}
	c.releaseBackBuffer()
	if w <= 0 || h <= 0 {
		return 0
	}
	dc, _, _ := procCreateCompatibleDC.Call(hdc)
	if dc == 0 {
		return 0
	}
	// Compatible with the window DC, not with dc: a memory DC starts out
	// holding a 1x1 monochrome bitmap, and a bitmap made from it would be
	// monochrome too.
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return 0
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	c.bufDC, c.bufBmp, c.bufOldBmp = dc, bmp, old
	c.bufW, c.bufH = w, h
	return dc
}

func (c *imageControl) releaseBackBuffer() {
	if c.bufDC == 0 {
		return
	}
	if c.bufOldBmp != 0 {
		procSelectObject.Call(c.bufDC, c.bufOldBmp)
	}
	if c.bufBmp != 0 {
		procDeleteObject.Call(c.bufBmp)
	}
	procDeleteDC.Call(c.bufDC)
	c.bufDC, c.bufBmp, c.bufOldBmp = 0, 0, 0
	c.bufW, c.bufH = 0, 0
}

// beginFastScale marks the start (or continuation) of a zoom/pan gesture and
// arms the timer that repaints at full quality once it stops.
func (c *imageControl) beginFastScale() {
	c.fastScale = true
	procSetTimer.Call(c.hwnd, qualityTimerID, qualityMillis, 0)
}

// endFastScale leaves fast scaling and repaints at full quality.
func (c *imageControl) endFastScale() {
	procKillTimer.Call(c.hwnd, qualityTimerID)
	if !c.fastScale {
		return
	}
	c.fastScale = false
	c.invalidate()
}

// showToast displays a short-lived message over the image, mirroring the
// Markdown view's HTML toast for the cases (native keyboard navigation
// while an image is on screen) that never touch the WebView.
func (c *imageControl) showToast(msg string) {
	if msg == "" {
		return
	}
	c.toast = msg
	c.invalidate()
	procSetTimer.Call(c.hwnd, toastTimerID, toastMillis, 0)
}

func (c *imageControl) clearToast() {
	procKillTimer.Call(c.hwnd, toastTimerID)
	if c.toast != "" {
		c.toast = ""
		c.invalidate()
	}
}
