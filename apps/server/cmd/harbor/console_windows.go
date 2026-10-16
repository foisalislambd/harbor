//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	// A windowed exe has no console. Reattach when it is started from a terminal
	// so `harbor push` and `harbor serve` can still print.
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	r, _, _ := attach.Call(uintptr(^uint32(0))) // ATTACH_PARENT_PROCESS
	if r == 0 {
		return
	}
	if out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		os.Stdout = os.NewFile(uintptr(out), "stdout")
	}
	if errOut, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE); err == nil {
		os.Stderr = os.NewFile(uintptr(errOut), "stderr")
	}
}

func interactiveConsole() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || handle == 0 || handle == windows.InvalidHandle {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(handle, &mode) == nil
}
