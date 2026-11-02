//go:build windows

package main

import (
 "syscall"
 "unsafe"
)

var metricsKernel=syscall.NewLazyDLL("kernel32.dll")
var metricsPSAPI=syscall.NewLazyDLL("psapi.dll")

type processEntry struct {
 Size, Usage, PID uint32
 Heap uintptr
 Module, Threads, Parent uint32
 Priority int32
 Flags uint32
 Exe [260]uint16
}
type memoryCounters struct {
 Size, PageFaults uint32
 PeakWorkingSet, WorkingSet, PeakPagedPool, PagedPool, PeakNonPagedPool, NonPagedPool, Pagefile, PeakPagefile uintptr
}
type fileTime struct { Low, High uint32 }
func (f fileTime) value() uint64 { return uint64(f.High)<<32|uint64(f.Low) }
type cpuSample struct { idle, kernel, user uint64; valid bool }
func currentCPU() cpuSample {
 var i,k,u fileTime
 ok,_,_:=metricsKernel.NewProc("GetSystemTimes").Call(uintptr(unsafe.Pointer(&i)),uintptr(unsafe.Pointer(&k)),uintptr(unsafe.Pointer(&u)))
 return cpuSample{i.value(),k.value(),u.value(),ok!=0}
}
func cpuUsage(a,b cpuSample) (float64,bool) {
 if !a.valid||!b.valid||b.kernel<a.kernel||b.user<a.user||b.idle<a.idle{return 0,false}
 total:=(b.kernel-a.kernel)+(b.user-a.user); idle:=b.idle-a.idle
 if total==0||idle>total{return 0,false};return float64(total-idle)*100/float64(total),true
}
func browserWorkingSet(pid int) (uint64,bool) {
 snap,_,_:=metricsKernel.NewProc("CreateToolhelp32Snapshot").Call(2,0)
 if snap==^uintptr(0){return 0,false}
 defer metricsKernel.NewProc("CloseHandle").Call(snap)
 var e processEntry;e.Size=uint32(unsafe.Sizeof(e))
 first:=metricsKernel.NewProc("Process32FirstW");next:=metricsKernel.NewProc("Process32NextW")
 ok,_,_:=first.Call(snap,uintptr(unsafe.Pointer(&e)))
 if ok==0{return 0,false}
 parents:=map[uint32]uint32{}
 for ok!=0 {parents[e.PID]=e.Parent;e.Size=uint32(unsafe.Sizeof(e));ok,_,_=next.Call(snap,uintptr(unsafe.Pointer(&e)))}
 root:=uint32(pid);if _,exists:=parents[root];!exists{return 0,false}
 tree:=map[uint32]bool{root:true};changed:=true
 for changed {changed=false;for child,parent:=range parents{if tree[parent]&&!tree[child]{tree[child]=true;changed=true}}}
 var total uint64
 for id:=range tree {
  handle,_,_:=metricsKernel.NewProc("OpenProcess").Call(0x1010,0,uintptr(id));if handle==0{return 0,false}
  var m memoryCounters;m.Size=uint32(unsafe.Sizeof(m))
  got,_,_:=metricsPSAPI.NewProc("GetProcessMemoryInfo").Call(handle,uintptr(unsafe.Pointer(&m)),uintptr(m.Size))
  metricsKernel.NewProc("CloseHandle").Call(handle)
  if got==0{return 0,false};total+=uint64(m.WorkingSet)
 }
 return total,total>0
}
