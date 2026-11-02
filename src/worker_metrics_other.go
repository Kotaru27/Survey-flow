//go:build !windows

package main
type cpuSample struct{}
func currentCPU() cpuSample{return cpuSample{}}
func cpuUsage(a,b cpuSample)(float64,bool){return 0,false}
func browserWorkingSet(pid int)(uint64,bool){return 0,false}
