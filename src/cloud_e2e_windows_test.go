//go:build windows

package main

import (
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
)

// Uses a local synthetic survey; no live survey or respondent data is involved.
func TestCloudBrowserRecoveryEndToEnd(t *testing.T) {
 if os.Getenv("SURVEY_QA_E2E")!="1" { t.Skip("explicit cloud browser QA only") }
 t.Setenv("SURVEY_QA_HEADLESS","1")
 root:=t.TempDir();if err:=os.MkdirAll(filepath.Join(root,"data"),0755);err!=nil{t.Fatal(err)}
 var mu sync.Mutex
 choices:=[]string{}
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  switch r.URL.Path {
  case "/", "/start":
   fmt.Fprint(w,`<!doctype html><html><body><section class="question"><h3>Do you use this product?</h3><label><input type="radio" name="usage" value="yes" checked>Yes</label><label><input type="radio" name="usage" value="no">No</label><p class="error" id="err" hidden>Yes is not allowed. Choose a different answer.</p></section><button id="next" onclick="let e=document.querySelector('input[name=usage]:checked');if(!e)return;if(e.value==='yes'){document.querySelector('#err').hidden=false}else{location.href='/choice'}">Next</button></body></html>`)
  case "/choice":
   fmt.Fprint(w,`<!doctype html><html><body><section class="question"><h3>What is your gender?</h3><label><input type="radio" name="gender" value="male" checked>Male</label><label><input type="radio" name="gender" value="female">Female</label></section><button id="next" onclick="let e=document.querySelector('input[name=gender]:checked');if(e)location.href='/route?choice='+e.value">Next</button></body></html>`)
  case "/route":
   v:=r.URL.Query().Get("choice");mu.Lock();choices=append(choices,v);mu.Unlock()
   dest:="/BCEndPages/en-Qual.html";if v=="male"{dest="/BCEndPages/en-Term.html"};http.Redirect(w,r,dest,http.StatusFound)
  case "/BCEndPages/en-Term.html":fmt.Fprint(w,"<html><body>You do not qualify.</body></html>")
  case "/BCEndPages/en-Qual.html":fmt.Fprint(w,"<html><body>Test complete.</body></html>")
  default:http.NotFound(w,r)
  }
 }))
 defer server.Close()
 cfg:=AppConfig{BrowserMode:"external",Incognito:true,Screenshots:"none"}
 sess,status,err:=launchBrowser(root,nil,cfg)
 if err!=nil||status!="" {t.Fatalf("browser launch failed: status=%s error=%v",status,err)}
 defer sess.stop(nil,true)
 persona:=Persona{Name:"Synthetic",Gender:"Male",Age:30,YesNo:"Yes",OpenText:"Test",Number:30,MultiSelectCount:1,AnswerMode:"persona",ReentryTemplate:"QA{uid}"}
 mem:=openMemory(root)
 report:=runComplete(root,server.URL+"/start",persona,mem,nil,1,1,cfg,sess,true)
 mu.Lock();gotChoices:=append([]string(nil),choices...);mu.Unlock()
 var changed bool
 for _,step:=range report.StepsLog {if strings.Contains(step.Action,"validation-changed"){changed=true}}
 if !changed {t.Errorf("validation was not recovered: status=%s errors=%v steps=%+v",report.Status,report.Errors,report.StepsLog)}
 if report.Status!="completed" {t.Errorf("did not reach qualifying end: status=%s errors=%v selections=%+v",report.Status,report.Errors,report.Selections)}
 if len(gotChoices)<2||gotChoices[0]!="male"||gotChoices[1]!="female" {t.Errorf("expected termination on male and retry with female, got %v",gotChoices)}
 rules:=openMemory(root).avoidFor(memoryFingerprint(server.URL+"/start"))
 if len(rules)!=1||rules[0].Option!="male" {t.Errorf("termination suspect was not persisted: %#v",rules)}
}
