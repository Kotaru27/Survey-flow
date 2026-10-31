//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

// A console window remains a genuine console: no GUI or runtime dependency.
// Windows 10+ native cmd and Windows Terminal support truecolor after VT mode.
func prepareConsole() bool {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel.NewProc("GetConsoleMode")
	setMode := kernel.NewProc("SetConsoleMode")
	setTitle := kernel.NewProc("SetConsoleTitleW")
	setCodePage := kernel.NewProc("SetConsoleOutputCP")
	handle := os.Stdout.Fd()
	var mode uint32
	success, _, _ := getMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if success == 0 {
		return false
	} // redirected/non-interactive output
	title, _ := syscall.UTF16PtrFromString("Survey Autopilot  |  QA Console")
	setTitle.Call(uintptr(unsafe.Pointer(title)))
	setCodePage.Call(65001)
	success, _, _ = setMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
	return success != 0
}
