package main

import (
	"os"
	"syscall"
)

// eviocgrab is EVIOCGRAB from linux/input.h.
const eviocgrab = 0x40044590

func grab(f *os.File) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), eviocgrab, 1); errno != 0 {
		return errno
	}
	return nil
}
