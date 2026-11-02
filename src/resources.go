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
 if n==0 {probe:=availablePhysicalMemory;if c.memoryProbe!=nil{probe=c.memoryProbe};memory,ok:=probe();n=1;if ok&&memory>=4*gibBytes{n=2}}
 if visible {n=1};if n<1{n=1};if n>4{n=4};if total>0&&n>total{n=total};return n
}

// The reserve is kept in available memory, in addition to the estimated cost
// of a new browser. Failure to measure either metric keeps admission at one.
func adaptiveWorkerLimit(available, browserBytes uint64, cpuPercent float64, measured bool, maximum int) int {
 if maximum<2||!measured||browserBytes==0||cpuPercent<0||cpuPercent>85 {return 1}
 const reserve=2*gibBytes
 budget:=browserBytes+browserBytes/2
 if budget<256*1024*1024 {budget=256*1024*1024}
 if available<=reserve||available-reserve<budget{return 1}
 return 2 // Raise only after representative laptop benchmarks.
}
