//go:build windows

package launcher

import (
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// makeShortcut writes a .lnk to target, the way Windows Explorer would.
func makeShortcut(link, target string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	result, err := oleutil.CallMethod(shell, "CreateShortcut", link)
	if err != nil {
		return err
	}
	shortcut := result.ToIDispatch()
	defer shortcut.Release()
	if _, err := oleutil.PutProperty(shortcut, "TargetPath", target); err != nil {
		return err
	}
	_, err = oleutil.CallMethod(shortcut, "Save")
	return err
}
