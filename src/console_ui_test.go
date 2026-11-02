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
