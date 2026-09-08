package viewer

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"mdview/internal/applog"
)

// Shell and common-dialog calls that ask the user where a file goes, or show
// a file to the user in Explorer.

var (
	comdlg32             = syscall.NewLazyDLL("comdlg32.dll")
	procGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")
)

const (
	ofnOverwritePrompt = 0x00000002
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnExplorer        = 0x00080000
)

// openFileNameW is Win32's OPENFILENAMEW. Every field is here, in order,
// because the struct size is passed to the dialog and a short one is
// rejected.
type openFileNameW struct {
	StructSize        uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

// saveFileDialog shows the standard "Save as" dialog and returns the chosen
// path, or "" if the user cancelled.
//
// suggested is the path to start from: its directory is the folder the dialog
// opens in and its file name is the one already filled in. filter is a list
// of "label|pattern" pairs separated by "|", e.g.
// "HTML files (*.html)|*.html|All files (*.*)|*.*". defExt (without a dot) is
// appended when the user types a name with no extension.
//
// The dialog is modal and runs its own message loop, so it must be called on
// the UI thread.
func saveFileDialog(owner uintptr, title, suggested, filter, defExt string) string {
	// The dialog writes the chosen path back into this buffer.
	buf := make([]uint16, 4096)
	if name := filepath.Base(suggested); name != "" && name != "." {
		if u, err := syscall.UTF16FromString(name); err == nil && len(u) <= len(buf) {
			copy(buf, u)
		}
	}

	// Heap-allocated: the dialog runs a message loop, during which the
	// window procedures below call back into Go, and a struct on the stack
	// could be moved out from under Windows by a stack growth.
	ofn := new(openFileNameW)
	ofn.StructSize = uint32(unsafe.Sizeof(*ofn))
	ofn.HwndOwner = owner
	ofn.LpstrFilter = filterList(filter)
	ofn.LpstrFile = &buf[0]
	ofn.NMaxFile = uint32(len(buf))
	ofn.LpstrInitialDir = utf16Ptr(filepath.Dir(suggested))
	ofn.LpstrTitle = utf16Ptr(title)
	ofn.LpstrDefExt = utf16Ptr(defExt)
	ofn.Flags = ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir | ofnExplorer

	ret, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(ofn)))
	if ret == 0 {
		// Cancelled, or the dialog failed; either way there is nothing to
		// save to.
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// filterList turns "label|pattern|label|pattern" into the double-null
// terminated block of null-separated strings the dialog expects.
func filterList(spec string) *uint16 {
	var buf []uint16
	for _, part := range strings.Split(spec, "|") {
		u, err := syscall.UTF16FromString(part)
		if err != nil {
			return nil
		}
		buf = append(buf, u...) // UTF16FromString already ends each part with a null
	}
	buf = append(buf, 0)
	return &buf[0]
}

func utf16Ptr(s string) *uint16 {
	if s == "" {
		return nil
	}
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

// revealInExplorer opens the folder holding path and selects the file in it.
// It returns a message for the user, or "" on success.
func revealInExplorer(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	applog.Write("revealInExplorer: %s", abs)

	// Explorer needs the path quoted inside its own /select argument, so this
	// goes through ShellExecute (which takes the parameters as one string)
	// rather than exec.Command (which would quote the whole argument).
	verb, _ := syscall.UTF16PtrFromString("open")
	file, _ := syscall.UTF16PtrFromString("explorer.exe")
	params, err := syscall.UTF16PtrFromString(`/select,"` + abs + `"`)
	if err != nil {
		return "Cannot show in Explorer: " + filepath.Base(abs)
	}
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		swShowNormal,
	)
	if ret <= 32 {
		applog.Write("revealInExplorer: ShellExecute ret=%d", ret)
		return "Cannot show in Explorer: " + filepath.Base(abs)
	}
	return ""
}
