//go:build windows
package main
import (
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
 "time"
)
func TestSyntheticWorkerThroughput(t *testing.T){
 if os.Getenv("SURVEY_QA_E2E")!="1"{t.Skip("explicit synthetic browser QA only")};t.Setenv("SURVEY_QA_HEADLESS","1")
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){switch r.URL.Path{case "/start":fmt.Fprint(w,`<html><body><section class="question"><h3>Choose an answer</h3><label><input type="radio" name="q" value="yes">Yes</label></section><button id="next" onclick="if(document.querySelector('input:checked'))location.href='/BCEndPages/en-Qual.html'">Next</button></body></html>`);case "/BCEndPages/en-Qual.html":fmt.Fprint(w,"<html><body>Complete</body></html>");default:http.NotFound(w,r)}}));defer srv.Close()
 for workers:=1;workers<=4;workers++ {root:=t.TempDir();for _,d:=range []string{"data","reports"}{if e:=os.MkdirAll(filepath.Join(root,d),0700);e!=nil{t.Fatal(e)}};cfg:=AppConfig{Headless:true,BrowserMode:"external",Incognito:true,Screenshots:"none",memoryProbe:func()(uint64,bool){return 16*1024*1024*1024,true},MaxAttempts:4};start:=time.Now();b:=runParallelBatch(root,srv.URL+"/start",4,workers,cfg,openMemory(root,srv.URL+"/start"),nil);dur:=time.Since(start);if b.Completed!=4||len(b.Runs)!=4{t.Fatalf("workers=%d incomplete: %+v",workers,b)};t.Logf("SYNTHETIC workers=%d completions=4 elapsed_ms=%d",workers,dur.Milliseconds())}
}
