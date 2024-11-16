//go:build linux

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func FGInvoke(ret *int, fd uintptr) error {
	r, e := unix.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	if e != nil {
		return e
	}
	*ret = r
	return nil
}

func SetPrior() error {
	return unix.Setpriority(unix.PRIO_PGRP, 0, 2)
}
