package main

import (
 "os"
 "path/filepath"
 "reflect"
 "testing"
)
func TestSeededQAPersonaReproducible(t *testing.T){
 root:=t.TempDir();dir:=filepath.Join(root,"config");if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 input:=`{"qaSeed":"synthetic-seed","answerMode":"persona","variedPerRun":true}`
 if err:=os.WriteFile(filepath.Join(dir,"persona.json"),[]byte(input),0600);err!=nil{t.Fatal(err)}
 first:=readPersonaForRun(root,7)
 for i:=0;i<20;i++{if got:=readPersonaForRun(root,7);!reflect.DeepEqual(first,got){t.Fatal("same synthetic seed and run changed profile")}}
 variants:=map[string]bool{};for i:=1;i<=30;i++{p:=readPersonaForRun(root,i);variants[p.City]=true};if len(variants)<2{t.Fatal("different runs produced no profile variation")}
}
