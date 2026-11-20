package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unsafe"
)

var (
	procGetWindowPlacement       = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement       = user32.NewProc("SetWindowPlacement")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procSetWindowLongW           = user32.NewProc("SetWindowLongW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsWindowEnabled          = user32.NewProc("IsWindowEnabled")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

const (
	swShowMinimized   = 2
	swShowMaximized   = 3
	swShowNoActivate  = 4
	swShowMinNoActive = 7
	gwlExStyle        = -20
	wsExNoActivate    = 0x08000000
)

// windowPlacement matches Win32 WINDOWPLACEMENT. Normal holds the restored
// window bounds in workspace coordinates, even while minimized or maximized.
type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	Min, Max               winPoint
	Normal                 winRect
}

type WindowState struct {
	File      string
	ScrollY   int
	Placement windowPlacement
	Visible   bool
}

var ErrWindowBusy = errors.New("window is busy")

func (v *Viewer) CaptureWindow() (WindowState, error) {
	hwnd := uintptr(v.w.Window())
	if enabled, _, _ := procIsWindowEnabled.Call(hwnd); enabled == 0 {
		return WindowState{}, ErrWindowBusy
	}
	var state WindowState
	var image bool
	var seq int
	var placementErr error
	if err := v.dispatchSync(func() {
		state.File, state.ScrollY = v.filePath, v.scrollY
		image, seq = v.imageOnScreen(), v.transSeq
		state.Placement.Length = uint32(unsafe.Sizeof(state.Placement))
		if ok, _, err := procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&state.Placement))); ok == 0 {
			placementErr = err
		}
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		state.Visible = visible != 0
	}); err != nil {
		return WindowState{}, err
	}
	if placementErr != nil || image {
		return state, placementErr
	}

	// The page reports scroll changes with a debounce. Read the live value
	// during handoff so a scroll just before publication is also restored.
	raw, err := v.Eval("Math.round(window.scrollY)", 500*time.Millisecond)
	var scroll int
	if err == nil && json.Unmarshal(raw, &scroll) == nil {
		if err := v.dispatchSync(func() {
			if seq == v.transSeq && state.File == v.filePath {
				state.ScrollY = scroll
			}
		}); err != nil {
			return WindowState{}, err
		}
	}
	return state, nil
}

func (v *Viewer) PrepareHandoff(file string, pid int) (bool, error) {
	var foreground, busy bool
	err := v.dispatchSync(func() {
		hwnd := uintptr(v.w.Window())
		enabled, _, _ := procIsWindowEnabled.Call(hwnd)
		busy = v.filePath != file || enabled == 0
		if busy {
			return
		}
		active, _, _ := procGetForegroundWindow.Call()
		foreground = active == hwnd
		if foreground {
			procAllowSetForegroundWindow.Call(uintptr(pid))
		}
	})
	if err == nil && busy {
		err = ErrWindowBusy
	}
	return foreground, err
}

func (v *Viewer) prepareReplacement(state WindowState) error {
	hwnd := uintptr(v.w.Window())
	procShowWindow.Call(hwnd, swHide)
	if state.Placement.Length != 0 {
		state.Placement.ShowCmd = swHide
		if ok, _, err := procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&state.Placement))); ok == 0 {
			return err
		}
	}
	return nil
}

func (v *Viewer) WaitReady(ctx context.Context) error {
	for {
		var image bool
		if err := v.dispatchSync(func() { image = v.imageOnScreen() }); err != nil {
			return err
		}
		if image {
			return nil
		}
		const ready = `document.readyState === "complete" && document.querySelector(".content") !== null`
		if result, err := v.Eval(ready, 500*time.Millisecond); err == nil && string(result) == "true" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (v *Viewer) ShowReplacement(state WindowState, foreground bool) error {
	if !state.Visible {
		return nil
	}
	var placementErr error
	if err := v.dispatchSync(func() {
		hwnd := uintptr(v.w.Window())
		placement := state.Placement
		if placement.Length == 0 {
			procShowWindow.Call(hwnd, swShowNoActivate)
		} else {
			placementErr = showPlacement(hwnd, placement, foreground)
		}
		if placementErr != nil {
			return
		}
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			placementErr = errors.New("replacement window is still hidden")
			return
		}
		if foreground {
			procSetForegroundWindow.Call(hwnd)
		}
	}); err != nil {
		return err
	}
	return placementErr
}

func showPlacement(hwnd uintptr, placement windowPlacement, foreground bool) error {
	switch placement.ShowCmd {
	case swShowMinimized:
		placement.ShowCmd = swShowMinNoActive
	case swShowMaximized:
		// SW_SHOWMAXIMIZED normally activates the window. Keep a
		// background replacement from taking focus during that call.
		if !foreground {
			index := int32(gwlExStyle)
			style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(index))
			procSetWindowLongW.Call(hwnd, uintptr(index), style|wsExNoActivate)
			defer procSetWindowLongW.Call(hwnd, uintptr(index), style)
		}
	default:
		placement.ShowCmd = swShowNoActivate
	}
	if ok, _, err := procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement))); ok == 0 {
		return err
	}
	return nil
}
