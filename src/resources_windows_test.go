//go:build windows
package main
import "testing"
func TestWindowsAvailableMemory(t *testing.T){n,ok:=availablePhysicalMemory();if !ok||n==0{t.Fatalf("available memory unavailable: %d %v",n,ok)};t.Logf("cloud available memory: %d MiB",n/(1024*1024))}
