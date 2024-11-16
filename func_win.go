//go:build windows

package main

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func FGInvoke(ret *int, fd uintptr) error {
	r, e := windows.GetsockoptInt(windows.Handle(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	if e != nil {
		return e
	}
	*ret = r
	return nil
}

func SetPrior() error {
	handle := windows.CurrentProcess()
	return windows.SetPriorityClass(handle, windows.BELOW_NORMAL_PRIORITY_CLASS)
}
