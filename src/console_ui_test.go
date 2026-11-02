package main
import ("bytes";"strings";"testing";"time")
func TestDashboardInPlaceAndOutcomes(t *testing.T){
 var b bytes.Buffer;r:=newConsoleRenderer(&b,true);r.width=func()int{return 80};r.startBatch(50,false)
 for i:=1;i<=50;i++{r.startRun(i,i-1);r.lastDraw=time.Time{};r.setSurveyProgress(37);r.runFinished(i,"completed",i,50)}
 s:=b.String();if strings.Count(s,"\n")!=1{t.Fatalf("dashboard scrolled: %d",strings.Count(s,"\n"))};if !strings.Contains(s,"50/50 qualified")||!strings.Contains(s,"100%")||!strings.Contains(s,"0%")||!strings.Contains(s,"37%") {t.Fatalf("missing states: %s",s)}
}
func TestDashboardUnknownFailureAndNarrow(t *testing.T){
 var b bytes.Buffer;r:=newConsoleRenderer(&b,true);r.width=func()int{return 30};r.startBatch(2,false);r.startRun(1,0);r.lastDraw=time.Time{};r.setSurveyProgress(-1);r.runFinished(1,"screenout",0,2)
 if !strings.Contains(b.String(),"--") {t.Fatal("missing indeterminate state")};if strings.Count(b.String(),"\n")!=1{t.Fatal("unexpected scrolling")}
}
func TestRedirectedOutputIsConcise(t *testing.T){
 var b bytes.Buffer;r:=newConsoleRenderer(&b,false);r.startBatch(50,false);for i:=0;i<50;i++{r.startRun(i+1,i);r.setSurveyProgress(45);r.runFinished(i+1,"completed",i+1,50)}
 r.finish(BatchReport{Requested:50,Completed:1,Runs:[]BatchRun{{Status:"completed"}}});s:=b.String();if strings.Contains(s,"\x1b")||strings.Contains(s,"[")||strings.Count(s,"\n")>2{t.Fatalf("bad redirected output %q",s)}
}
func TestSafeConsoleTextDropsControls(t *testing.T){s:=safeConsoleText("bad\x1b[2J\ntext",100);if strings.ContainsAny(s,"\x1b\n\r"){t.Fatal(s)}}
func TestParallelDashboardSeparatesAttemptsAndQualifications(t *testing.T){
 var out bytes.Buffer;r:=newConsoleRenderer(&out,true);r.width=func()int{return 80};r.startBatch(5,false);r.enableWorkers(2);r.workerStarted(1,1);r.workerStarted(2,2);r.lastDraw=time.Time{};r.workerProgress(1,45);r.workerFinished(1,"screenout_pattern",0,5);r.workerStarted(1,3)
 r.mu.Lock();defer r.mu.Unlock();if r.attempts!=1||r.screenouts!=1||r.completed!=0||r.workerViews[1].done!=1||r.workerViews[1].percent!=0||r.workerViews[2].run!=2 {t.Fatalf("invalid counters or reset: %+v",r)}
 s:=out.String();if !strings.Contains(s,"QUALIFICATIONS")||!strings.Contains(s,"W1")||!strings.Contains(s,"W2")||!strings.Contains(s,"45%")||!strings.Contains(s,"0/5"){t.Fatalf("missing worker output %q",s)}
}
