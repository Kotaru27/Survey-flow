//go:build windows

package main
import (
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "sync/atomic"
 "testing"
)
func TestSeededQAScreenoutIsTerminal(t *testing.T){
 if os.Getenv("SURVEY_QA_E2E")!="1"{t.Skip("synthetic browser QA only")};t.Setenv("SURVEY_QA_HEADLESS","1")
 root:=t.TempDir();for _,d:=range []string{"data","reports","config"}{if err:=os.MkdirAll(filepath.Join(root,d),0700);err!=nil{t.Fatal(err)}}
 if err:=os.WriteFile(filepath.Join(root,"config","persona.json"),[]byte(`{"qaSeed":"synthetic-only","answerMode":"random"}`),0600);err!=nil{t.Fatal(err)}
 var starts atomic.Int32
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.URL.Path=="/start"{starts.Add(1);http.Redirect(w,r,"/BCEndPages/en-Term.html",http.StatusFound);return};fmt.Fprint(w,"<html><body>You do not qualify.</body></html>")}));defer srv.Close()
 cfg:=AppConfig{Headless:true,BrowserMode:"external",Incognito:true,Screenshots:"none",memoryProbe:func()(uint64,bool){return 16*gibBytes,true},MaxAttempts:1}
 result:=runParallelBatch(root,srv.URL+"/start",1,1,cfg,openMemory(root,srv.URL+"/start"),nil)
 if starts.Load()!=1||len(result.Runs)!=1||result.Runs[0].Status!="screenout_or_termination"{t.Fatalf("seeded screenout was retried or misclassified: starts=%d runs=%+v",starts.Load(),result.Runs)}
}
