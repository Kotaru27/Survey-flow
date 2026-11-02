//go:build windows

package main

import (
 "os"
 "testing"
 "time"
)
func TestWindowsProcessAndSystemMetrics(t *testing.T) {
 n,ok:=browserWorkingSet(os.Getpid());if !ok||n==0{t.Fatal("could not measure current process working set")}
 first:=currentCPU();time.Sleep(250*time.Millisecond);pct,ok:=cpuUsage(first,currentCPU())
 if !ok||pct<0||pct>100{t.Fatalf("invalid system CPU measurement: %.2f %v",pct,ok)}
 t.Logf("working set bytes=%d; system CPU=%.1f%%",n,pct)
}
