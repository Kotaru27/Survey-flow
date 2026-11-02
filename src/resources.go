package main
const gibBytes = uint64(1024 * 1024 * 1024)
// Admission uses a conservative estimate: reserve 4 GiB and budget 1 GiB
// per browser. This is not measured per-worker consumption or a CPU monitor.
func memoryWorkerCapacity(available uint64,ok bool) int {
 if !ok||available<5*gibBytes{return 1}
 capacity:=int((available-4*gibBytes)/gibBytes)
 if capacity<1{return 1};if capacity>4{return 4};return capacity
}
func memoryWorkerLimit(available uint64,ok bool) int {n:=memoryWorkerCapacity(available,ok);if n>2{return 2};return n}
func automaticWorkerLimit() int {available,ok:=availablePhysicalMemory();return memoryWorkerLimit(available,ok)}
