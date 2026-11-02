//go:build windows
package main
import (
 "bytes"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)
func TestTwoParallelBrowsersAreIsolated(t *testing.T){
 if os.Getenv("SURVEY_QA_E2E")!="1"{t.Skip("explicit synthetic browser QA only")}
 t.Setenv("SURVEY_QA_HEADLESS","1")
 root:=t.TempDir();for _,d:=range []string{"data","reports"}{if e:=os.MkdirAll(filepath.Join(root,d),0700);e!=nil{t.Fatal(e)}}
 var mu sync.Mutex; starts:=0;tokens:=map[string]bool{}; finished:=map[string]bool{};both:=make(chan struct{})
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/start":
   mu.Lock();starts++;id:=fmt.Sprint(starts);tokens[id]=true;if starts==2{close(both)};mu.Unlock()
   http.SetCookie(w,&http.Cookie{Name:"qa_worker",Value:id,Path:"/"})
   fmt.Fprint(w,`<html><body><section class="question"><h3>Choose an answer</h3><label><input type="radio" name="q" value="yes">Yes</label></section><button id="next" onclick="if(document.querySelector('input:checked'))fetch('/barrier').then(r=>{if(r.ok)location.href='/BCEndPages/en-Qual.html'})">Next</button></body></html>`)
  case "/barrier":
   select{case <-both:fmt.Fprint(w,"ready");case <-time.After(25*time.Second):http.Error(w,"second worker did not arrive",500)}
  case "/BCEndPages/en-Qual.html":
   if c,e:=r.Cookie("qa_worker");e==nil{mu.Lock();finished[c.Value]=true;mu.Unlock()};fmt.Fprint(w,"<html><body>Complete</body></html>")
  default:http.NotFound(w,r)
  }
 }));defer srv.Close()
 cfg:=AppConfig{Headless:true,BrowserMode:"external",Incognito:true,Screenshots:"none",memoryProbe:func()(uint64,bool){return 16*1024*1024*1024,true},MaxAttempts:2}
 var output bytes.Buffer
 view:=newConsoleRenderer(&output,true);view.width=func()int{return 80};view.startBatch(2,false);consoleUI=view;defer func(){consoleUI=nil}()
 batch:=runParallelBatch(root,srv.URL+"/start",2,2,cfg,openMemory(root,srv.URL+"/start"),nil)
 view.mu.Lock();attempts:=view.attempts;view.mu.Unlock();if attempts!=2{t.Fatalf("dashboard recorded %d attempts",attempts)}
 mu.Lock();defer mu.Unlock()
 if batch.Completed!=2||len(batch.Runs)!=2||len(tokens)!=2||len(finished)!=2{for _,r:=range batch.Runs{t.Logf("synthetic attempt %d status=%s errors=%v",r.Run,r.Status,r.Errors)};t.Fatalf("isolated completions failed: completed=%d runs=%d starts=%d cookies=%d",batch.Completed,len(batch.Runs),starts,len(finished))}
 a,b:=batch.Runs[0],batch.Runs[1];as,_:=time.Parse(time.RFC3339Nano,a.StartedAt);ae,_:=time.Parse(time.RFC3339Nano,a.FinishedAt);bs,_:=time.Parse(time.RFC3339Nano,b.StartedAt);be,_:=time.Parse(time.RFC3339Nano,b.FinishedAt)
 if !as.Before(be)||!bs.Before(ae){t.Fatalf("workers did not overlap: %+v %+v",a,b)}
 t.Logf("OVERLAP worker %d %s to %s; worker %d %s to %s; distinct cookies=%d",a.Worker,a.StartedAt,a.FinishedAt,b.Worker,b.StartedAt,b.FinishedAt,len(finished))
}

func TestAutoMeasuredWorkersOverlap(t *testing.T){
 if os.Getenv("SURVEY_QA_E2E")!="1"{t.Skip("explicit synthetic browser QA only")}
 t.Setenv("SURVEY_QA_HEADLESS","1")
 root:=t.TempDir();for _,d:=range []string{"data","reports"}{if e:=os.MkdirAll(filepath.Join(root,d),0700);e!=nil{t.Fatal(e)}}
 var mu sync.Mutex; starts:=0;tokens:=map[string]bool{}; finished:=map[string]bool{};both:=make(chan struct{})
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/start":
   mu.Lock();starts++;id:=fmt.Sprint(starts);tokens[id]=true;if starts==2{close(both)};mu.Unlock()
   http.SetCookie(w,&http.Cookie{Name:"qa_worker",Value:id,Path:"/"})
   fmt.Fprint(w,`<html><body><section class="question"><h3>Choose an answer</h3><label><input type="radio" name="q" value="yes">Yes</label></section><button id="next" onclick="if(document.querySelector('input:checked'))fetch('/barrier').then(r=>{if(r.ok)location.href='/BCEndPages/en-Qual.html'})">Next</button></body></html>`)
  case "/barrier":
   select{case <-both:fmt.Fprint(w,"ready");case <-time.After(25*time.Second):http.Error(w,"second worker did not arrive",500)}
  case "/BCEndPages/en-Qual.html":
   if c,e:=r.Cookie("qa_worker");e==nil{mu.Lock();finished[c.Value]=true;mu.Unlock()};fmt.Fprint(w,"<html><body>Complete</body></html>")
  default:http.NotFound(w,r)
  }
 }));defer srv.Close()
 cfg:=AppConfig{autoWorkers:true,Headless:true,BrowserMode:"external",Incognito:true,Screenshots:"none",memoryProbe:func()(uint64,bool){return 16*1024*1024*1024,true},MaxAttempts:2}
 var output bytes.Buffer
 view:=newConsoleRenderer(&output,true);view.width=func()int{return 80};view.startBatch(2,false);consoleUI=view;defer func(){consoleUI=nil}()
 batch:=runParallelBatch(root,srv.URL+"/start",2,2,cfg,openMemory(root,srv.URL+"/start"),nil)
 view.mu.Lock();attempts:=view.attempts;view.mu.Unlock();if attempts!=2{t.Fatalf("dashboard recorded %d attempts",attempts)}
 mu.Lock();defer mu.Unlock()
 if batch.Completed!=2||len(batch.Runs)!=2||len(tokens)!=2||len(finished)!=2{for _,r:=range batch.Runs{t.Logf("synthetic attempt %d status=%s errors=%v",r.Run,r.Status,r.Errors)};t.Fatalf("isolated completions failed: completed=%d runs=%d starts=%d cookies=%d",batch.Completed,len(batch.Runs),starts,len(finished))}
 a,b:=batch.Runs[0],batch.Runs[1];as,_:=time.Parse(time.RFC3339Nano,a.StartedAt);ae,_:=time.Parse(time.RFC3339Nano,a.FinishedAt);bs,_:=time.Parse(time.RFC3339Nano,b.StartedAt);be,_:=time.Parse(time.RFC3339Nano,b.FinishedAt)
 if !as.Before(be)||!bs.Before(ae){t.Fatalf("workers did not overlap: %+v %+v",a,b)}
 t.Logf("OVERLAP worker %d %s to %s; worker %d %s to %s; distinct cookies=%d",a.Worker,a.StartedAt,a.FinishedAt,b.Worker,b.StartedAt,b.FinishedAt,len(finished))
}


func TestParallelStopDrainsActiveBrowsers(t *testing.T){
 if os.Getenv("SURVEY_QA_E2E")!="1"{t.Skip("explicit synthetic browser QA only")};t.Setenv("SURVEY_QA_HEADLESS","1")
 root:=t.TempDir();for _,d:=range []string{"data","reports"}{if e:=os.MkdirAll(filepath.Join(root,d),0700);e!=nil{t.Fatal(e)}}
 var mu sync.Mutex;starts:=0;released:=make(chan struct{});stop:=make(chan os.Signal,1)
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){switch r.URL.Path{
 case "/start":mu.Lock();starts++;if starts==2{stop<-os.Interrupt;close(released)};mu.Unlock();select{case <-released:case <-time.After(25*time.Second):http.Error(w,"workers not ready",500);return};fmt.Fprint(w,`<html><body><section class="question"><h3>Choose an answer</h3><label><input type="radio" name="q" value="yes">Yes</label></section><button id="next" onclick="if(document.querySelector('input:checked'))location.href='/BCEndPages/en-Qual.html'">Next</button></body></html>`)
 case "/BCEndPages/en-Qual.html":fmt.Fprint(w,"<html><body>Complete</body></html>")
 default:http.NotFound(w,r)}}));defer srv.Close()
 cfg:=AppConfig{Headless:true,BrowserMode:"external",Incognito:true,Screenshots:"none",Stop:stop,memoryProbe:func()(uint64,bool){return 16*1024*1024*1024,true},MaxAttempts:4}
 batch:=runParallelBatch(root,srv.URL+"/start",4,2,cfg,openMemory(root,srv.URL+"/start"),nil)
 mu.Lock();gotStarts:=starts;mu.Unlock();if gotStarts!=2||len(batch.Runs)!=2||batch.Completed!=2{t.Fatalf("stop did not drain just two attempts: starts=%d runs=%d completed=%d",gotStarts,len(batch.Runs),batch.Completed)}
 entries,e:=os.ReadDir(filepath.Join(root,"data"));if e!=nil{t.Fatal(e)};for _,entry:=range entries{if strings.HasPrefix(entry.Name(),"browser-profile-"){t.Errorf("profile remains: %s",entry.Name())}}
 t.Log("PASS stop prevented new attempts and both active browsers completed and cleaned up")
}
