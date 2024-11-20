//go:build !windows && !linux && !android

package main

func FGInvoke(ret *int, fd uintptr) error {
	return newError("UNSUPPORT")
}

func SetPrior() error {
	return newError("UNSUPPORT")
}
