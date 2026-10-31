//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Standard-library Windows mutex for the portable terminal-only build.
// No external packages or admin access are required.
var instanceMutex uintptr

func acquireSingleInstance(name string) bool {
	dll := syscall.NewLazyDLL("kernel32.dll")
	proc := dll.NewProc("CreateMutexW")
	utf16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return true
	}
	handle, _, callErr := proc.Call(0, 0, uintptr(unsafe.Pointer(utf16)))
	if handle == 0 {
		return true
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
		dll.NewProc("CloseHandle").Call(handle)
		return false
	}
	instanceMutex = handle
	return true
}
func activateExistingInstance() {
	fmt.Println("Survey Autopilot is already running. Switch to the open window.")
}
