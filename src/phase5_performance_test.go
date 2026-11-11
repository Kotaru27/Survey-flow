package main

import (
 "os"
 "path/filepath"
 "testing"
)

func TestPhase5ConfigErrorsAreReportedInsteadOfIgnored(t *testing.T){
 root:=t.TempDir()
 if err:=os.MkdirAll(filepath.Join(root,"config"),0700);err!=nil{t.Fatal(err)}
 file:=filepath.Join(root,"config","app.json")
 if err:=os.WriteFile(file,[]byte("{invalid"),0600);err!=nil{t.Fatal(err)}
 invalid:=readAppConfig(root)
 if invalid.configError==""{t.Fatal("malformed JSON must stop the launcher instead of silently falling back")}
 if err:=os.WriteFile(file,[]byte("{\"replayVerifiedSets\":true,\"workers\":1}"),0600);err!=nil{t.Fatal(err)}
 valid:=readAppConfig(root)
 if valid.configError!=""||!valid.ReplayVerifiedSets||valid.Workers!=1{t.Fatalf("valid config should decode: %+v",valid)}
}

func TestPhase5AutomaticWorkersRespectConservativeMemoryReserve(t *testing.T){
 for _,tc:=range []struct{free uint64;want int}{
  {4*gibBytes,1},{5*gibBytes,1},{6*gibBytes,2},{8*gibBytes,2},
 }{
  cfg:=AppConfig{memoryProbe:func()(uint64,bool){return tc.free,true}}
  if got:=workerCountForRun(cfg,false,10);got!=tc.want{t.Errorf("free=%d auto workers=%d, want %d",tc.free,got,tc.want)}
 }
}
