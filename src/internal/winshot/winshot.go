// Package winshot saves a screenshot of a window to a PNG file. It exists so
// that an automated test run can look at what mdview actually put on screen,
// including content mdview does not draw itself (the WebView2 page) and
// content no browser can see (the native image control).
package winshot

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procPrintWindow         = user32.NewProc("PrintWindow")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
)

const (
	srcCopy = 0x00CC0020

	swRestore = 9

	// PW_RENDERFULLCONTENT asks Windows to include DirectComposition content
	// in a PrintWindow capture.
	pwRenderFullContent = 0x00000002

	biRGB        = 0
	dibRGBColors = 0
)

type rect struct{ Left, Top, Right, Bottom int32 }

type point struct{ X, Y int32 }

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

// Mode selects how the pixels are obtained.
type Mode string

const (
	// ModeScreen copies the pixels from the screen where the window is
	// currently displayed. It sees everything the user sees, so it is the
	// mode to trust, but it needs the window to be on top and not minimized.
	ModeScreen Mode = "screen"
	// ModePrintWindow asks the window to draw itself into an off-screen
	// buffer. It works when the window is covered, but WebView2 content
	// often comes out blank, so use it only for the native image view.
	ModePrintWindow Mode = "print"
)

// Options controls a capture.
type Options struct {
	Mode Mode
	// ClientOnly drops the title bar and the window frame.
	ClientOnly bool
	// Settle is how long to wait after raising the window, to let it repaint
	// before the pixels are read.
	Settle time.Duration
}

// CaptureWindow writes a PNG of the window at hwnd to path.
func CaptureWindow(hwnd uintptr, path string, opts Options) error {
	if hwnd == 0 {
		return errors.New("winshot: no window")
	}
	if opts.Mode == "" {
		opts.Mode = ModeScreen
	}
	if opts.Settle == 0 {
		opts.Settle = 300 * time.Millisecond
	}

	img, err := capture(hwnd, opts)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func capture(hwnd uintptr, opts Options) (*image.RGBA, error) {
	if opts.Mode == ModeScreen {
		raise(hwnd, opts.Settle)
	}

	origin, w, h, err := captureArea(hwnd, opts.ClientOnly)
	if err != nil {
		return nil, err
	}

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("winshot: GetDC failed")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, errors.New("winshot: CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	// A negative height gives a top-down buffer, which matches image.RGBA's
	// row order, so no row flipping is needed after the blit.
	hdr := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h),
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(
		screenDC,
		uintptr(unsafe.Pointer(&hdr)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbm == 0 || bits == nil {
		return nil, errors.New("winshot: CreateDIBSection failed")
	}
	defer procDeleteObject.Call(hbm)

	old, _, _ := procSelectObject.Call(memDC, hbm)
	defer procSelectObject.Call(memDC, old)

	switch opts.Mode {
	case ModePrintWindow:
		ret, _, _ := procPrintWindow.Call(hwnd, memDC, pwRenderFullContent)
		if ret == 0 {
			return nil, errors.New("winshot: PrintWindow failed")
		}
	default:
		ret, _, _ := procBitBlt.Call(
			memDC, 0, 0, uintptr(w), uintptr(h),
			screenDC, uintptr(origin.X), uintptr(origin.Y), srcCopy,
		)
		if ret == 0 {
			return nil, errors.New("winshot: BitBlt failed")
		}
	}

	return toRGBA(bits, w, h), nil
}

// captureArea returns the screen origin and the size of the region to grab.
func captureArea(hwnd uintptr, clientOnly bool) (point, int, int, error) {
	var r rect
	if clientOnly {
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	} else {
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	}
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return point{}, 0, 0, fmt.Errorf("winshot: empty window rect %dx%d", w, h)
	}

	origin := point{X: r.Left, Y: r.Top}
	if clientOnly {
		origin = point{}
		procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	}
	return origin, w, h, nil
}

// raise un-minimizes the window, brings it to the front and waits for it to
// repaint, so that a screen copy shows this window and not whatever was in
// front of it.
func raise(hwnd uintptr, settle time.Duration) {
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
	time.Sleep(settle)
}

// toRGBA copies a 32bpp top-down BGRA buffer into an image.RGBA.
func toRGBA(bits unsafe.Pointer, w, h int) *image.RGBA {
	src := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0] = src[i*4+2] // R
		img.Pix[i*4+1] = src[i*4+1] // G
		img.Pix[i*4+2] = src[i*4+0] // B
		img.Pix[i*4+3] = 255        // the screen has no alpha
	}
	return img
}
