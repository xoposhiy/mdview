package viewer

import (
	"image/color"
	"syscall"
	"unsafe"

	"mdview/internal/applog"
)

// gwlpWndProc is the GWLP_WNDPROC index for [Get|Set]WindowLongPtrW.
const gwlpWndProc = -4

type winPoint struct{ X, Y int32 }

func loword(lp uintptr) uint16 { return uint16(lp & 0xFFFF) }
func hiword(lp uintptr) uint16 { return uint16((lp >> 16) & 0xFFFF) }

// xyFromLparam extracts client-area (x, y) from a mouse message's lParam.
func xyFromLparam(lp uintptr) (int32, int32) {
	return int32(int16(loword(lp))), int32(int16(hiword(lp)))
}

func isKeyDown(vk int) bool {
	ret, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(ret) < 0
}

func colorref(c color.RGBA) uintptr {
	return uintptr(uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16)
}

func otherBg(bg string) string {
	if bg == "black" {
		return "white"
	}
	return "black"
}

// installParentHook subclasses the top-level window at hwnd so onResize is
// called with the new client size on every WM_SIZE, and onActivate every time
// the window is activated or given the keyboard focus. Every message is
// forwarded to the original WndProc, so the window's own handling is
// untouched.
//
// onActivate runs after the original WndProc, not before: activating the
// window is what makes WebView2 pull the focus into its own child window, and
// the callback is there to take the focus back.
func installParentHook(hwnd uintptr, onResize func(w, h int), onActivate func()) {
	var oldProc uintptr
	newProc := syscall.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
		if msg == wmSize {
			onResize(int(loword(lp)), int(hiword(lp)))
		}
		// A message can reach the new WndProc before SetWindowLongPtrW has
		// returned the old one, in which case there is nothing to forward to.
		var ret uintptr
		if oldProc == 0 {
			ret, _, _ = procDefWindowProcW.Call(h, msg, wp, lp)
		} else {
			ret, _, _ = procCallWindowProcW.Call(oldProc, h, msg, wp, lp)
		}
		if msg == wmSetFocus || (msg == wmActivate && loword(wp) != waInactive) {
			onActivate()
		}
		return ret
	})
	idx := gwlpWndProc // copy into a variable: converting the constant directly overflows uintptr
	old, _, _ := procSetWindowLongPtrW.Call(hwnd, uintptr(idx), newProc)
	oldProc = old
}

func (c *imageControl) wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmGetDlgCode:
		// go-webview2's message loop runs every message through
		// IsDialogMessage (see webview.go's Run), which by default treats
		// arrow/tab keys as "move focus to the next dialog control" — exactly
		// the sibling-window navigation we don't want. Claiming all keys here
		// makes IsDialogMessage hand them to us as plain WM_KEYDOWN instead
		// of silently consuming every other press.
		return dlgcWantArrows | dlgcWantTab | dlgcWantAllKeys

	case wmPaint:
		c.onPaint()
		return 0

	case wmEraseBkgnd:
		return 1

	case wmSize:
		if c.zoom != nil {
			c.zoom.Layout(int(loword(lp)), int(hiword(lp)))
			// Only report while the image is actually on screen: the control
			// is resized along with the parent even while hidden, and
			// reporting then would put an image's zoom percentage into the
			// title of a Markdown document.
			if c.visible {
				c.reportZoom()
			}
		}
		return 0

	case wmMouseWheel:
		// WM_MOUSEWHEEL carries screen coordinates, unlike every other mouse
		// message here, so convert before handing them on.
		screenX, screenY := xyFromLparam(lp)
		pt := winPoint{X: screenX, Y: screenY}
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		c.wheelAt(int(int16(hiword(wp))), pt.X, pt.Y)
		return 0

	case wmLButtonDown:
		c.takeFocus()
		x, y := xyFromLparam(lp)
		c.beginDrag(x, y)
		procSetCapture.Call(hwnd)
		return 0

	case wmMouseMove:
		x, y := xyFromLparam(lp)
		c.dragTo(x, y)
		return 0

	case wmLButtonUp:
		c.endDrag()
		procReleaseCapture.Call()
		return 0

	case wmCaptureChanged:
		c.endDrag()
		return 0

	case wmMButtonDown:
		c.takeFocus()
		c.resetZoom()
		return 0

	case wmKeyDown:
		c.handleKeyDown(wp)
		return 0

	case wmTimer:
		switch wp {
		case toastTimerID:
			c.clearToast()
		case qualityTimerID:
			c.endFastScale()
		}
		return 0

	case wmDestroy:
		c.releaseBackBuffer()
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return ret
}

// The image view's actions, each one the whole of what a mouse gesture does.
// wndProc calls them for real input; the control interface calls the same
// methods, so an automated check drives the shipping code and not a copy.

// wheelAt zooms by delta wheel units (120 per notch) around the client-area
// point (x, y).
func (c *imageControl) wheelAt(delta int, x, y int32) {
	if c.zoom == nil {
		return
	}
	w, h := c.clientSize()
	c.zoom.ZoomAt(float64(delta)/120.0, int(x), int(y), w, h)
	c.beginFastScale()
	c.invalidate()
	c.reportZoom()
}

// beginDrag starts a pan gesture at the client-area point (x, y).
func (c *imageControl) beginDrag(x, y int32) {
	c.dragging = true
	c.dragX, c.dragY = x, y
}

// dragTo continues a pan gesture to (x, y). A no-op unless a drag is active.
func (c *imageControl) dragTo(x, y int32) {
	if !c.dragging || c.zoom == nil {
		return
	}
	w, h := c.clientSize()
	c.zoom.PanBy(float64(x-c.dragX), float64(y-c.dragY), w, h)
	c.dragX, c.dragY = x, y
	c.beginFastScale()
	c.invalidate()
}

// endDrag ends a pan gesture.
func (c *imageControl) endDrag() {
	c.dragging = false
	c.endFastScale()
}

// resetZoom returns to Fit.
func (c *imageControl) resetZoom() {
	if c.zoom == nil {
		return
	}
	c.zoom.Reset()
	c.layoutAndInvalidate()
}

// handleKeyDown reads the live modifier state for a real key press and hands
// the press to handleKey.
func (c *imageControl) handleKeyDown(vk uintptr) {
	c.handleKey(vk, isKeyDown(vkCtrl), isKeyDown(vkMenu))
}

// handleKey mirrors the Markdown view's JS keydown handler (see
// template.html) for the shortcuts that still apply to images.
func (c *imageControl) handleKey(vk uintptr, ctrlDown, altDown bool) {
	if altDown && vk == vkLeft {
		c.runAction(c.onBack)
		return
	}
	if vk == vkF5 {
		if c.onReload != nil {
			c.onReload()
		}
		return
	}
	if ctrlDown {
		switch vk {
		case 'R':
			if c.onReload != nil {
				c.onReload()
			}
		case 'C':
			c.copyToClipboard()
		case 'S':
			c.runAction(c.onSave)
		}
		return
	}
	if altDown {
		return
	}

	switch vk {
	case vkEscape:
		if c.onExit != nil {
			c.onExit()
		}
	case vkBack:
		c.runAction(c.onBack)
	case vkLeft:
		c.runAction(c.onPrev)
	case vkRight:
		c.runAction(c.onNext)
	case 'D':
		c.SetBg(otherBg(c.bg))
	case 'E':
		if c.onEditor != nil {
			c.onEditor()
		}
	case 'S':
		c.runAction(c.onSave)
	case 'O':
		c.runAction(c.onReveal)
	}
}

// runAction calls one of the control's callbacks and shows what it returns,
// so a failed navigation or save says why.
func (c *imageControl) runAction(fn func() string) {
	if fn == nil {
		return
	}
	if msg := fn(); msg != "" {
		c.showToast(msg)
	}
}

// onPaint draws one frame: the current background color, the visible part of
// the image scaled/panned per c.zoom, and the toast (if any) on top. It draws
// into an off-screen buffer and blits the finished frame in one go.
func (c *imageControl) onPaint() {
	if c.metrics != nil {
		defer c.metrics.track(stagePaint)()
	}

	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&ps)))

	w, h := c.clientSize()
	target := c.backBuffer(hdc, w, h)
	if target == 0 {
		target = hdc
	}

	bgBrush, _, _ := procCreateSolidBrush.Call(colorref(c.bgColor()))
	full := winRect{0, 0, int32(w), int32(h)}
	procFillRect.Call(target, uintptr(unsafe.Pointer(&full)), bgBrush)
	procDeleteObject.Call(bgBrush)

	c.drawImage(target, w, h)

	if c.toast != "" {
		c.drawToast(target, w, h)
	}

	if target != hdc {
		procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), target, 0, 0, srcCopy)
	}
}

// drawImage blits the part of the image the viewport can show, scaled to the
// current zoom.
//
// Only that part is handed to GDI. At 800% the whole destination rectangle is
// tens of thousands of pixels wide, and asking GDI to scale all of it and
// then clip away everything off screen is what made zooming slow.
func (c *imageControl) drawImage(hdc uintptr, w, h int) {
	if c.packed == nil || c.zoom == nil || w <= 0 || h <= 0 {
		return
	}
	z := c.zoom

	sx0, sy0, sx1, sy1 := z.visibleRect(w, h)
	// The packed buffer is what is actually indexed below, so clamp to it
	// and not only to the size the zoom state was built from.
	sx1, sy1 = clampInt(sx1, 0, c.pw), clampInt(sy1, 0, c.ph)
	if sx1 <= sx0 || sy1 <= sy0 {
		return
	}
	dx0, dy0 := int32(z.viewportX(sx0)), int32(z.viewportY(sy0))
	dx1, dy1 := int32(z.viewportX(sx1)), int32(z.viewportY(sy1))

	// HALFTONE averages the pixels a downscale drops, which is what makes a
	// fitted photo look right, but it reads every source pixel and costs
	// tens of milliseconds on a large image. While a gesture is running, and
	// when upscaling (where it only blurs), use the cheap mode instead.
	mode := halftone
	if c.fastScale || z.scale >= 1 {
		mode = colorOnColor
	}
	procSetStretchBltMode.Call(hdc, uintptr(mode))
	procSetBrushOrgEx.Call(hdc, 0, 0, 0)

	// Pass whole rows starting at the first visible one, so the source
	// rectangle only has to be clipped horizontally: StretchDIBits measures
	// ySrc from the bottom of the bitmap, and a partial top-down DIB is the
	// one case where that is easy to get wrong.
	stride := c.pw * 4
	rows := c.packed[sy0*stride : sy1*stride]
	hdr := bitmapInfoHeader{
		Size:      uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:     int32(c.pw),
		Height:    -int32(sy1 - sy0), // negative: top-down source, matches c.packed's row order
		Planes:    1,
		BitCount:  32,
		SizeImage: uint32(len(rows)),
	}
	procStretchDIBits.Call(
		hdc,
		uintptr(dx0), uintptr(dy0), uintptr(dx1-dx0), uintptr(dy1-dy0),
		uintptr(sx0), 0, uintptr(sx1-sx0), uintptr(sy1-sy0),
		uintptr(unsafe.Pointer(&rows[0])),
		uintptr(unsafe.Pointer(&hdr)),
		0, // DIB_RGB_COLORS
		srcCopy,
	)
}

// drawToast paints a fixed dark bar with white text near the bottom of the
// control, readable regardless of the current letterbox background.
func (c *imageControl) drawToast(hdc uintptr, w, h int) {
	const barHeight = 36
	rect := winRect{Left: 20, Top: int32(h) - 20 - barHeight, Right: int32(w) - 20, Bottom: int32(h) - 20}
	if rect.Top < 0 {
		return
	}

	brush, _, _ := procCreateSolidBrush.Call(0x00333333)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
	procDeleteObject.Call(brush)

	procSetBkMode.Call(hdc, transparentBk)
	procSetTextColor.Call(hdc, 0x00FFFFFF)
	text, _ := syscall.UTF16PtrFromString(c.toast)
	autoLen := int32(-1) // DrawTextW: negative cchText means "null-terminated, compute length"
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(text)), uintptr(autoLen),
		uintptr(unsafe.Pointer(&rect)), uintptr(dtCenter|dtVCenter|dtSingleLine))
}

// copyToClipboard puts the whole current image (composited over the active
// letterbox background, at natural resolution, ignoring zoom/pan) on the
// clipboard as a standard device-independent bitmap.
func (c *imageControl) copyToClipboard() {
	if c.packed == nil {
		return
	}
	stride := c.pw * 4
	dataSize := stride * c.ph
	hdrSize := int(unsafe.Sizeof(bitmapInfoHeader{}))
	total := hdrSize + dataSize

	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(total))
	if hMem == 0 {
		applog.Write("copyToClipboard: GlobalAlloc failed")
		return
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		applog.Write("copyToClipboard: GlobalLock failed")
		return
	}

	hdr := (*bitmapInfoHeader)(unsafe.Pointer(ptr))
	*hdr = bitmapInfoHeader{
		Size:      uint32(hdrSize),
		Width:     int32(c.pw),
		Height:    int32(c.ph), // positive: bottom-up, for maximum clipboard compatibility
		Planes:    1,
		BitCount:  32,
		SizeImage: uint32(dataSize),
	}

	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr+uintptr(hdrSize))), dataSize)
	for y := 0; y < c.ph; y++ {
		srcRow := c.packed[y*stride : y*stride+stride]
		dstRow := dst[(c.ph-1-y)*stride : (c.ph-1-y)*stride+stride]
		copy(dstRow, srcRow)
	}
	procGlobalUnlock.Call(hMem)

	if ret, _, _ := procOpenClipboard.Call(c.hwnd); ret == 0 {
		procGlobalFree.Call(hMem)
		applog.Write("copyToClipboard: OpenClipboard failed")
		return
	}
	procEmptyClipboard.Call()
	if ret, _, _ := procSetClipboardData.Call(cfDIB, hMem); ret == 0 {
		applog.Write("copyToClipboard: SetClipboardData failed")
		procGlobalFree.Call(hMem)
	}
	procCloseClipboard.Call()
}
