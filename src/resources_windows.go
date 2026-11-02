//go:build windows
package main
import (
 "syscall"
 "unsafe"
)
type memoryStatusEx struct {Length uint32; Load uint32; TotalPhys,AvailablePhys,TotalPage,AvailablePage,TotalVirtual,AvailableVirtual,AvailableExtendedVirtual uint64}
func availablePhysicalMemory()(uint64,bool){
 var s memoryStatusEx;s.Length=uint32(unsafe.Sizeof(s));ok,_,_:=syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&s)));return s.AvailablePhys,ok!=0
}
