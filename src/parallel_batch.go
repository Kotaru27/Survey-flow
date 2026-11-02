package main

import (
 "os"
 "sync"
 "time"
)

// Reserve startup ports until Chrome has bound its debugging port.
var browserLaunchMu sync.Mutex

type workerResult struct{ worker,run int; started,finished time.Time; report RunReport }
type workerSample struct{worker,run int; bytes uint64; cpu float64; ok bool; at time.Time}

// runParallelBatch is an experimental opt-in path. Each attempt owns its browser
// and profile. Scheduling reserves a potential qualification for each active run,
// preventing concurrency from overshooting the requested successful target.
func runParallelBatch(root,u string,total,workers int,cfg AppConfig,mem *MemoryStore,logFile *os.File) BatchReport {
 if workers<1 {workers=1};if workers>4 {workers=4}
 batchSeq++
 if consoleUI!=nil {consoleUI.enableWorkers(workers)}
 batch:=BatchReport{Build:buildID,StartedAt:time.Now().Format(time.RFC3339),URL:u,Requested:total,StatusCounts:map[string]int{}}
 capAttempts:=batchAttemptLimit(total,cfg.MaxAttempts)
 results:=make(chan workerResult,workers)
 samples:=make(chan workerSample,workers*2)
 latest:=map[int]workerSample{}
 current:=map[int]int{}
 free:=make(chan int,workers);for i:=1;i<=workers;i++{free<-i}
 active,next:=0,1
 stopCh:=cfg.Stop;stopping:=false
 for {
  select {case <-stopCh: stopping=true;stopCh=nil;logLine(logFile,"Stop requested; finishing active surveys.");default:}
  limit:=workers
  probe:=availablePhysicalMemory;if cfg.memoryProbe!=nil{probe=cfg.memoryProbe}
  if available,ok:=probe();cfg.autoWorkers {
   var peak uint64;cpu:=float64(0);measured:=active>0
   for _,s:=range latest {if !s.ok||time.Since(s.at)>8*time.Second{measured=false};if s.bytes>peak{peak=s.bytes};if s.cpu>cpu{cpu=s.cpu}}
   if len(latest)!=active{measured=false}
   limit=adaptiveWorkerLimit(available,peak,cpu,ok&&measured,workers)
   if consoleUI!=nil{consoleUI.workerResources(available,cpu,ok&&measured)}
  } else if memoryWorkerCapacity(available,ok)<limit{limit=memoryWorkerCapacity(available,ok)}
  for !stopping && active<limit && next<=capAttempts && batch.Completed+active<total {
   w:=<-free; n:=next;next++;active++
   current[w]=n
   if consoleUI!=nil {consoleUI.workerStarted(w,n)}
   go func(worker,run int){
    started:=time.Now()
    browserLaunchMu.Lock();sess,status,err:=launchBrowser(root,logFile,cfg);browserLaunchMu.Unlock()
    sampleStop:=make(chan struct{})
    if cfg.autoWorkers&&status==""&&sess!=nil&&sess.cmd!=nil&&sess.cmd.Process!=nil {
     pid:=sess.cmd.Process.Pid
     go func(){prev:=currentCPU();tick:=time.NewTicker(2*time.Second);defer tick.Stop();for {select {case <-sampleStop:return;case <-tick.C:
      now:=currentCPU();cpu,cpuOK:=cpuUsage(prev,now);prev=now;bytes,memOK:=browserWorkingSet(pid)
      sample:=workerSample{worker,run,bytes,cpu,cpuOK&&memOK,time.Now()};select{case samples<-sample:default:}
     }}}()
    }
    report:=RunReport{Run:run,Status:status,URL:u,StartedAt:started.Format(time.RFC3339)}
    if status=="" {report=runCompleteWithProgress(root,u,readPersonaForRun(root,run),mem,logFile,run,total,cfg,sess,true,func(v float64){if consoleUI!=nil{consoleUI.workerProgress(worker,v)}})} else if err!=nil {report.Errors=append(report.Errors,err.Error())}
    close(sampleStop);sess.stop(logFile,true)
    results<-workerResult{worker,run,started,time.Now(),report}
   }(w,n)
  }
  if active==0 {break}
  var x workerResult
  select {case x=<-results: case s:=<-samples: if current[s.worker]==s.run{latest[s.worker]=s};continue;case <-stopCh: stopping=true;stopCh=nil;logLine(logFile,"Stop requested; finishing active surveys.");continue}
  active--;free<-x.worker
  delete(latest,x.worker)
  delete(current,x.worker)
  r:=x.report
  if r.Status=="completed"{batch.Completed++};batch.StatusCounts[r.Status]++
  name:=runReportName(x.run,total);saveReportAs(root,name,r)
  batch.Runs=append(batch.Runs,BatchRun{Run:x.run,Worker:x.worker,StartedAt:x.started.Format(time.RFC3339Nano),FinishedAt:x.finished.Format(time.RFC3339Nano),Status:r.Status,ReentryID:r.ReentryID,Steps:r.Steps,DurationMS:x.finished.Sub(x.started).Milliseconds(),Errors:r.Errors,Learned:r.Learned,Report:name})
  if consoleUI!=nil {consoleUI.workerFinished(x.worker,r.Status,batch.Completed,total)}
  if r.Status=="browser_missing"||r.Status=="port_error" {next=capAttempts+1}
 }
 batch.FinishedAt=time.Now().Format(time.RFC3339);batch.Windows=len(batch.Runs);batch.AvoidRules=mem.avoidCount(memoryFingerprint(u));saveBatchReport(root,batch)
 logLine(logFile,"Experimental batch finished: %d of %d qualifications, %d attempts",batch.Completed,total,len(batch.Runs))
 return batch
}
