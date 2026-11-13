// Package winmedia talks to Windows: the system media session for the current
// track and playback commands, and Core Audio for the master volume.
package winmedia

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
)

const asyncTimeout = 5 * time.Second

var errAsyncTimeout = errors.New("winmedia: Windows did not answer in time")

// Host owns one OS thread with COM initialized. Every COM and WinRT call runs
// there, because COM apartments are per thread and goroutines move between threads.
type Host struct {
	work chan func()
}

func Start() (*Host, error) {
	h := &Host{work: make(chan func())}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// 1 is RO_INIT_MULTITHREADED.
		if err := ole.RoInitialize(1); err != nil {
			ready <- fmt.Errorf("winmedia: RoInitialize: %w", err)
			return
		}
		ready <- nil
		for fn := range h.work {
			fn()
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return h, nil
}

// run executes fn on the COM thread and turns a panic in the bindings into an
// error, so a misbehaving media app cannot take the tray app down.
func (h *Host) run(fn func() error) (err error) {
	done := make(chan struct{})
	h.work <- func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("winmedia: panic: %v", r)
			}
		}()
		err = fn()
	}
	<-done
	return err
}

func release(obj unsafe.Pointer) {
	if obj == nil {
		return
	}
	(*ole.IUnknown)(obj).Release()
}

// waitDone polls instead of registering a completion handler: it needs no
// parameterized delegate IIDs and the operations here finish in milliseconds.
func waitDone(op unsafe.Pointer) error {
	itf, err := (*ole.IUnknown)(op).QueryInterface(ole.NewGUID(foundation.GUIDIAsyncInfo))
	if err != nil {
		return err
	}
	defer itf.Release()
	info := (*foundation.IAsyncInfo)(unsafe.Pointer(itf))
	deadline := time.Now().Add(asyncTimeout)
	for {
		status, err := info.GetStatus()
		if err != nil {
			return err
		}
		if status == foundation.AsyncStatusCompleted {
			return nil
		}
		if status != foundation.AsyncStatusStarted {
			return fmt.Errorf("winmedia: async operation ended with status %d", status)
		}
		if time.Now().After(deadline) {
			return errAsyncTimeout
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func await(op *foundation.IAsyncOperation, err error) (unsafe.Pointer, error) {
	if err != nil {
		return nil, err
	}
	defer release(unsafe.Pointer(op))
	if err := waitDone(unsafe.Pointer(op)); err != nil {
		return nil, err
	}
	return op.GetResults()
}

// The helpers below call COM methods by vtable slot, for interfaces the bindings
// lack. Out parameters travel as unsafe.Pointer and become uintptr only inside
// the syscall expression: a uintptr computed earlier is not tracked by the
// runtime and can point at a stack that has since moved.

func vtable(obj unsafe.Pointer) *[32]uintptr {
	return *(**[32]uintptr)(obj)
}

func callOut(obj unsafe.Pointer, slot int, out unsafe.Pointer) error {
	hr, _, _ := syscall.SyscallN(vtable(obj)[slot], uintptr(obj), uintptr(out))
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

func callReadAsync(stream unsafe.Pointer, slot int, buffer unsafe.Pointer, count uint32, out unsafe.Pointer) error {
	hr, _, _ := syscall.SyscallN(vtable(stream)[slot], uintptr(stream), uintptr(buffer), uintptr(count), 0, uintptr(out))
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}
