//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

type consoleCoord struct { X, Y int16 }
type consoleRect struct { Left, Top, Right, Bottom int16 }
type consoleInfo struct { Size, Cursor consoleCoord; Attributes uint16; Window consoleRect; Maximum consoleCoord }
func consoleWidth() int {
 var info consoleInfo
 proc:=syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleScreenBufferInfo")
 ok,_,_:=proc.Call(os.Stdout.Fd(),uintptr(unsafe.Pointer(&info)))
 if ok==0{return 80}; return int(info.Window.Right-info.Window.Left+1)
}
// Classic console windows can accept an icon; Windows Terminal owns its own window.
func setConsoleIcon() {
 if os.Getenv("WT_SESSION")!="" {return}
 kernel:=syscall.NewLazyDLL("kernel32.dll"); user:=syscall.NewLazyDLL("user32.dll")
 hwnd,_,_:=kernel.NewProc("GetConsoleWindow").Call(); if hwnd==0{return}
 module,_,_:=kernel.NewProc("GetModuleHandleW").Call(0)
 load:=user.NewProc("LoadImageW"); send:=user.NewProc("SendMessageTimeoutW")
 for _,size:=range []struct{kind,px uintptr}{{0,16},{1,32}} {
  icon,_,_:=load.Call(module,1,1,size.px,size.px,0);if icon!=0 {var ignored uintptr;send.Call(hwnd,0x80,size.kind,icon,2,200,uintptr(unsafe.Pointer(&ignored)))}
 }
}

// A console window remains a genuine console: no GUI or runtime dependency.
// Windows 10+ native cmd and Windows Terminal support truecolor after VT mode.
func prepareConsole() bool {
	setConsoleIcon()
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
	title, _ := syscall.UTF16PtrFromString("Survey Flow  |  QA Console")
	setTitle.Call(uintptr(unsafe.Pointer(title)))
	setCodePage.Call(65001)
	success, _, _ = setMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
	return success != 0
}
