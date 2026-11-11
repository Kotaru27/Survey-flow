package main

import (
 "bufio"
 "bytes"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestPhase2QATestLinkGate(t *testing.T) {
 cases:=[]struct{url string;recognized bool}{
  {"https://survey.example.test/start?pid=QA123",true},
  {"http://127.0.0.1:8765/survey",true},
  {"https://survey.example.com/testing?pid=QA",true},
  {"https://survey.example.com/survey?test=1",true},
  {"https://survey.example.com/survey?pid=QA",false},
  {"https://survey.example.com/contest",false},
 }
 for _,tc:=range cases {if got:=explicitlyMarkedTestLink(tc.url);got!=tc.recognized{t.Errorf("%s: recognized=%v want %v",tc.url,got,tc.recognized)}}
 var sink bytes.Buffer
 view:=newConsoleRenderer(&sink,false)
 url:="https://survey.example.com/survey?pid=QA"
 if confirmAuthorizedTestLink(bufio.NewReader(strings.NewReader("NO\n")),view,url){t.Fatal("unapproved URL was allowed")}
 if !confirmAuthorizedTestLink(bufio.NewReader(strings.NewReader("TEST\n")),view,url){t.Fatal("explicit QA confirmation was refused")}
 if !confirmAuthorizedTestLink(bufio.NewReader(strings.NewReader("")),view,"https://survey.example.test/one"){t.Fatal("test domain should not require extra confirmation")}
}

func TestPhase2CompletionCannotCrossUnapprovedOrigin(t *testing.T) {
 start:="https://survey.example.test/start?pid=QA"
 for _,tc:=range []struct{url string;want bool}{
  {"https://survey.example.test/BCEndPages/en-Qual.html",true},
  {"https://survey.example.test/Qual.html?done=1",true},
  {"https://other.example.test/BCEndPages/en-Qual.html",false},
  {"http://survey.example.test/BCEndPages/en-Qual.html",false},
  {"https://survey.example.test/BCEndPages/en-Term.html",false},
  {"https://survey.example.test/thank-you",false},
 }{if got:=isQualifiedCompletionForSurvey(tc.url,start);got!=tc.want{t.Errorf("%q: qualify=%v want %v",tc.url,got,tc.want)}}
}

func TestPhase2ReportArchiveNamesSurviveRestart(t *testing.T){
 a:=reportNameFor("processA",1,1,1)
 b:=reportNameFor("processB",1,1,1)
 c:=reportNameFor("processA",2,1,1)
 d:=reportNameFor("processA",1,1,2)
 for _,other:=range []string{b,c,d}{if a==other{t.Fatalf("report naming collision: %s",a)}}
 root:=t.TempDir()
 for _,name:=range []string{a,b}{
  if err:=saveReportAs(root,name,RunReport{Status:"completed"});err!=nil{t.Fatalf("archive %s: %v",name,err)}
  if _,err:=os.Stat(filepath.Join(root,"reports",name));err!=nil{t.Fatal(err)}
 }
 if err:=saveReport(root,RunReport{Status:"completed"});err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(filepath.Join(root,"reports","run-report.json"));err!=nil{t.Fatal(err)}
}

func TestPhase2ReportWriteErrorsAreReturned(t *testing.T) {
 root:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(root,"reports"),[]byte("not a directory"),0600);err!=nil{t.Fatal(err)}
 if err:=saveReportAs(root,"run-1.json",RunReport{Status:"completed"});err==nil{t.Fatal("report write failure was silently swallowed")}
 if err:=saveBatchReport(root,BatchReport{});err==nil{t.Fatal("batch write failure was silently swallowed")}
 if !fatalStatus("report_write_error"){t.Fatal("report persistence failure should stop further runs")}
}

func TestPhase2UnreadableStudyMemoryIsDetected(t *testing.T) {
 root:=t.TempDir()
 url:="https://survey.example.test/start?pid=QA"
 path:=studyMemoryPath(root,url)
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("{corrupted"),0600);err!=nil{t.Fatal(err)}
 mem:=openMemory(root,url)
 if !mem.readFailed{t.Fatal("corrupt study memory must be reported to the launcher")}
 if err:=mem.save();err==nil{t.Fatal("corrupt study memory must never be overwritten")}
}
