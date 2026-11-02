//go:build !windows
package main
func availablePhysicalMemory()(uint64,bool){return 0,false}
