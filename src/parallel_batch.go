package main

import (
 "os"
 "sync"
 "time"
)

// Reserve startup ports until Chrome has bound its debugging port.
var browserLaunchMu sync.Mutex

type workerResult struct{ worker,run int; started,finished time.Time; report RunReport }

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
 free:=make(chan int,workers);for i:=1;i<=workers;i++{free<-i}
 active,next:=0,1
 for {
  for active<workers && next<=capAttempts && batch.Completed+active<total {
   w:=<-free; n:=next;next++;active++
   if consoleUI!=nil {consoleUI.workerStarted(w,n)}
   go func(worker,run int){
    started:=time.Now()
    browserLaunchMu.Lock();sess,status,err:=launchBrowser(root,logFile,cfg);browserLaunchMu.Unlock()
    report:=RunReport{Run:run,Status:status,URL:u,StartedAt:started.Format(time.RFC3339)}
    if status=="" {report=runCompleteWithProgress(root,u,readPersona(root),mem,logFile,run,total,cfg,sess,true,func(v float64){if consoleUI!=nil{consoleUI.workerProgress(worker,v)}})} else if err!=nil {report.Errors=append(report.Errors,err.Error())}
    sess.stop(logFile,true)
    results<-workerResult{worker,run,started,time.Now(),report}
   }(w,n)
  }
  if active==0 {break}
  x:=<-results;active--;free<-x.worker
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
