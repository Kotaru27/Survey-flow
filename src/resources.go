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

func workerCountForRun(c AppConfig,visible bool,total int) int {
 n:=c.Workers
 if n==0 {probe:=availablePhysicalMemory;if c.memoryProbe!=nil{probe=c.memoryProbe};memory,ok:=probe();n=memoryWorkerLimit(memory,ok)}
 if visible {n=1};if n<1{n=1};if n>4{n=4};if total>0&&n>total{n=total};return n
}
