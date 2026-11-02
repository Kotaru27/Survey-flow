package main

import (
 "fmt"
 "encoding/json"
 "os"
 "path/filepath"
 "sync"
 "strings"
 "testing"
)

func TestConcurrentReentryIDsAreUnique(t *testing.T) {
 root:=t.TempDir();if err:=os.MkdirAll(filepath.Join(root,"data"),0700);err!=nil{t.Fatal(err)}
 const count=100
 var wg sync.WaitGroup
 ids:=make(chan string,count); errs:=make(chan error,count)
 for i:=0;i<count;i++ {wg.Add(1);go func(){defer wg.Done();id,_,err:=nextReentryID(root,Persona{ReentryTemplate:"QA-{seq}"});if err!=nil{errs<-err;return};ids<-id}()}
 wg.Wait();close(ids);close(errs);for err:=range errs{t.Error(err)}
 seen:=map[string]bool{};for id:=range ids{if seen[id]{t.Errorf("duplicate %s",id)};seen[id]=true};if len(seen)!=count{t.Errorf("got %d IDs, want %d",len(seen),count)}
 b,err:=os.ReadFile(filepath.Join(root,"data","reentry-history.txt"));if err!=nil{t.Fatal(err)};for id:=range seen{if !containsID(string(b),id){t.Error(fmt.Sprintf("missing %s",id))}}
}
func containsID(history,id string) bool {for _,line:=range strings.Split(history,"\n"){if line==id{return true}};return false}
func TestLeftSurveyUsesItsOwnOrigin(t *testing.T){
 cases:=[]struct{url,base string;want bool}{{"https://a.test/next","https://a.test/start",false},{"https://a.test/next","https://b.test/start",true},{"https://a.test/BCEndPages/en-Qual.html","https://a.test/start",true}}
 for _,c:=range cases {if got:=leftSurvey(c.url,c.base);got!=c.want{t.Errorf("leftSurvey(%q,%q)=%v",c.url,c.base,got)}}
}

func TestConcurrentReportWritesKeepValidJSON(t *testing.T) {
 root:=t.TempDir();if err:=os.MkdirAll(filepath.Join(root,"reports"),0700);err!=nil{t.Fatal(err)}
 const count=40; var wg sync.WaitGroup
 for i:=0;i<count;i++{wg.Add(1);go func(n int){defer wg.Done();r:=RunReport{Run:n,Status:"completed"};saveReport(root,r);saveReportAs(root,fmt.Sprintf("run-%02d.json",n),r)}(i)}
 wg.Wait()
 for i:=-1;i<count;i++ {name:="run-report.json";if i>=0{name=fmt.Sprintf("run-%02d.json",i)};b,e:=os.ReadFile(filepath.Join(root,"reports",name));if e!=nil{t.Error(e);continue};var v RunReport;if e=json.Unmarshal(b,&v);e!=nil{t.Errorf("%s: %v",name,e)};if i>=0&&v.Run!=i{t.Errorf("%s has run %d",name,v.Run)}}
}
