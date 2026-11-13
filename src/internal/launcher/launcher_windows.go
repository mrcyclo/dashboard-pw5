// Package launcher starts programs on the PC for the apps widget, or brings
// their window to the front when they already run, and draws their icons.
package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procSHGetImageList = shell32.NewProc("SHGetImageList")

	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetIconInfo              = user32.NewProc("GetIconInfo")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procGetWindow                = user32.NewProc("GetWindow")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	procGetObjectW   = gdi32.NewProc("GetObjectW")
	procGetDIBits    = gdi32.NewProc("GetDIBits")
	procDeleteObject = gdi32.NewProc("DeleteObject")

	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	shgfiSysIconIndex = 0x4000
	// shilJumbo is the 256 px system image list.
	shilJumbo       = 4
	ildTransparent  = 1
	iImageListIcon  = 10
	iUnknownRelease = 2

	gwOwner        = 4
	gwlExStyle     = -20
	wsExToolWindow = 0x80
	dwmwaCloaked   = 14
	swRestore      = 9
	swShowNormal   = 1
	dibRGBColors   = 0
)

var iidImageList = ole.NewGUID("{46EB5926-582E-4017-9FDF-E8998DAA0950}")

type shFileInfo struct {
	icon        uintptr
	iconIndex   int32
	attributes  uint32
	displayName [260]uint16
	typeName    [80]uint16
}

type iconInfo struct {
	isIcon   int32
	xHotspot uint32
	yHotspot uint32
	mask     uintptr
	color    uintptr
}

type bitmap struct {
	kind       int32
	width      int32
	height     int32
	widthBytes int32
	planes     uint16
	bitsPixel  uint16
	bits       uintptr
}

type bitmapInfo struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
	colors        [4]byte
}

var errNoIcon = errors.New("launcher: no icon")

// withCOM runs fn on one OS thread with COM started, as the shell calls need.
func withCOM(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	// S_FALSE means COM was already started on this thread; it still needs the matching uninit.
	if oleErr, ok := err.(*ole.OleError); err == nil || (ok && oleErr.Code() == 1) {
		defer ole.CoUninitialize()
	}
	return fn()
}

// Icon draws the program's or shortcut's icon as a size x size gray PNG on white.
func Icon(path string, size int) ([]byte, error) {
	var img *image.NRGBA
	err := withCOM(func() error {
		var err error
		img, err = shellIcon(path)
		return err
	})
	if err != nil {
		return nil, err
	}
	box := opaqueBounds(img)
	if box.Empty() {
		return nil, errNoIcon
	}
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(canvas, fit(box, size), img, box, xdraw.Over, nil)
	gray := image.NewGray(canvas.Bounds())
	xdraw.Draw(gray, gray.Bounds(), canvas, image.Point{}, xdraw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, gray); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// fit centers a box of the source's shape in a size x size square.
func fit(src image.Rectangle, size int) image.Rectangle {
	w, h := src.Dx(), src.Dy()
	if w >= h {
		h = h * size / w
		w = size
	} else {
		w = w * size / h
		h = size
	}
	x, y := (size-w)/2, (size-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// opaqueBounds trims the transparent margin: the 256 px list draws a small
// icon in its top left corner when the program has no large one.
func opaqueBounds(img *image.NRGBA) image.Rectangle {
	box := image.Rectangle{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A < 16 {
				continue
			}
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return box
}

func shellIcon(path string) (*image.NRGBA, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var info shFileInfo
	ret, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), shgfiSysIconIndex)
	if ret == 0 {
		return nil, errNoIcon
	}
	var list unsafe.Pointer
	hr, _, _ := procSHGetImageList.Call(shilJumbo, uintptr(unsafe.Pointer(iidImageList)), uintptr(unsafe.Pointer(&list)))
	if hr != 0 || list == nil {
		return nil, errNoIcon
	}
	// A COM object starts with a pointer to its table of methods.
	vtable := *(**[11]uintptr)(list)
	defer syscall.SyscallN(vtable[iUnknownRelease], uintptr(list))
	var icon uintptr
	hr, _, _ = syscall.SyscallN(vtable[iImageListIcon], uintptr(list), uintptr(info.iconIndex), ildTransparent, uintptr(unsafe.Pointer(&icon)))
	if hr != 0 || icon == 0 {
		return nil, errNoIcon
	}
	defer procDestroyIcon.Call(icon)
	return iconPixels(icon)
}

func iconPixels(icon uintptr) (*image.NRGBA, error) {
	var ii iconInfo
	if ok, _, _ := procGetIconInfo.Call(icon, uintptr(unsafe.Pointer(&ii))); ok == 0 {
		return nil, errNoIcon
	}
	defer procDeleteObject.Call(ii.mask)
	if ii.color == 0 {
		return nil, errNoIcon
	}
	defer procDeleteObject.Call(ii.color)
	bgra, w, h, err := bitmapPixels(ii.color)
	if err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	hasAlpha := false
	for i := 0; i < w*h; i++ {
		b, g, r, a := bgra[i*4], bgra[i*4+1], bgra[i*4+2], bgra[i*4+3]
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
		hasAlpha = hasAlpha || a != 0
	}
	if hasAlpha {
		return img, nil
	}
	// Old icons carry no alpha; their mask is white where the icon is see-through.
	mask, mw, mh, err := bitmapPixels(ii.mask)
	if err != nil || mw != w || mh < h {
		return img, nil
	}
	for i := 0; i < w*h; i++ {
		if mask[i*4] == 0 {
			img.Pix[i*4+3] = 255
		}
	}
	return img, nil
}

// bitmapPixels reads a bitmap as top-down 32-bit BGRA rows.
func bitmapPixels(hbm uintptr) ([]byte, int, int, error) {
	var bm bitmap
	if n, _, _ := procGetObjectW.Call(hbm, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); n == 0 {
		return nil, 0, 0, errNoIcon
	}
	w, h := int(bm.width), int(bm.height)
	if w <= 0 || h <= 0 {
		return nil, 0, 0, errNoIcon
	}
	info := bitmapInfo{width: int32(w), height: -int32(h), planes: 1, bitCount: 32}
	info.size = uint32(unsafe.Offsetof(info.colors))
	pixels := make([]byte, w*h*4)
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	if n, _, _ := procGetDIBits.Call(dc, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), dibRGBColors); n == 0 {
		return nil, 0, 0, errNoIcon
	}
	return pixels, w, h, nil
}

// Target is the program a path starts: the path itself for an .exe, the
// shortcut's target for a .lnk to a program, and "" otherwise.
func Target(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe":
		return path
	case ".lnk":
	default:
		return ""
	}
	target := ""
	withCOM(func() error {
		target = shortcutTarget(path)
		return nil
	})
	if !strings.EqualFold(filepath.Ext(target), ".exe") {
		return ""
	}
	return target
}

func shortcutTarget(link string) string {
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return ""
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	defer shell.Release()
	result, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return ""
	}
	shortcut := result.ToIDispatch()
	defer shortcut.Release()
	target, err := oleutil.GetProperty(shortcut, "TargetPath")
	if err != nil {
		return ""
	}
	return target.ToString()
}

func sameProgram(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b)))
}

// Open brings the program's window to the front, restoring it when it is
// minimized, or starts path when no window of target is open.
func Open(path, target string) error {
	if hwnd := findWindow(target); hwnd != 0 {
		focus(hwnd)
		return nil
	}
	dir := filepath.Dir(path)
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(path), nil, windows.StringToUTF16Ptr(dir), swShowNormal)
}

// findWindow returns the frontmost window a user would see in the taskbar for the program.
func findWindow(target string) windows.HWND {
	if target == "" {
		return 0
	}
	var found windows.HWND
	callback := syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		if !taskbarWindow(hwnd) || !sameProgram(processPath(hwnd), target) {
			return 1
		}
		found = hwnd
		return 0
	})
	windows.EnumWindows(callback, nil)
	return found
}

func taskbarWindow(hwnd windows.HWND) bool {
	if !windows.IsWindowVisible(hwnd) {
		return false
	}
	if owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner); owner != 0 {
		return false
	}
	if length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd)); length == 0 {
		return false
	}
	index := int32(gwlExStyle)
	style, _, _ := procGetWindowLongW.Call(uintptr(hwnd), uintptr(index))
	if style&wsExToolWindow != 0 {
		return false
	}
	// Suspended Store apps keep cloaked windows that look visible but are not shown.
	var cloaked uint32
	procDwmGetWindowAttribute.Call(uintptr(hwnd), dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked))
	return cloaked == 0
}

func processPath(hwnd windows.HWND) string {
	var pid uint32
	procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(process, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// focus restores and raises a window. Windows lets a background program take
// the foreground only while it shares input with the window in front, so this
// thread joins that window's input for the call.
func focus(hwnd windows.HWND) {
	if iconic, _, _ := procIsIconic.Call(uintptr(hwnd)); iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	front, _, _ := procGetForegroundWindow.Call()
	frontThread, _, _ := procGetWindowThreadProcessId.Call(front, 0)
	ownThread := windows.GetCurrentThreadId()
	if frontThread != 0 && uint32(frontThread) != ownThread {
		procAttachThreadInput.Call(uintptr(ownThread), frontThread, 1)
		defer procAttachThreadInput.Call(uintptr(ownThread), frontThread, 0)
	}
	procSetForegroundWindow.Call(uintptr(hwnd))
	procBringWindowToTop.Call(uintptr(hwnd))
}

// DisplayName is the name Windows shows for a program, from its version
// information, or the file name without its extension.
func DisplayName(path string) string {
	fallback := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return fallback
	}
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return fallback
	}
	info := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&info[0])); err != nil {
		return fallback
	}
	var translation unsafe.Pointer
	var length uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&info[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&translation), &length); err != nil || length < 4 {
		return fallback
	}
	// The first language and code page the file names, as two 16-bit numbers.
	pair := (*[2]uint16)(translation)
	key := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, pair[0], pair[1])
	var text unsafe.Pointer
	if err := windows.VerQueryValue(unsafe.Pointer(&info[0]), key, unsafe.Pointer(&text), &length); err != nil || length == 0 {
		return fallback
	}
	name := strings.TrimSpace(windows.UTF16PtrToString((*uint16)(text)))
	if name == "" {
		return fallback
	}
	return name
}

// Windows is the launcher the desk uses on this PC.
type Windows struct{}

func (Windows) Icon(path string, size int) ([]byte, error) { return Icon(path, size) }
func (Windows) Target(path string) string                  { return Target(path) }
func (Windows) DisplayName(path string) string             { return DisplayName(path) }
func (Windows) Open(path, target string) error             { return Open(path, target) }
