package main

import (
 "reflect"
 "testing"
)

func TestPhase4MultiSelectSetsPersistAndStayStudyScoped(t *testing.T){
 root:=t.TempDir()
 u:="https://survey.example.test/one?pid=QA741"
 fp:=memoryFingerprint(u)
 store:=openMemory(root,u)
 _,completed:=store.noteAttempt(fp,nil,"completed")
 if !completed{t.Fatal("multi-only qualified run was not recorded")}
 original:=map[string][]string{
   "which brands": {"Alpha","Gamma"},
   "not-enough": {"Alpha"},
   "invalid-duplicates": {"Alpha","alpha"},
 }
 if got:=store.noteCompletedSets(fp,original);got!=1{t.Fatalf("recorded %d verified groups, want 1",got)}
 if err:=store.save();err!=nil{t.Fatal(err)}
 reload:=openMemory(root,u)
 got:=reload.preferSetsFor(fp)
 if !reflect.DeepEqual(got["which brands"],[]string{"Alpha","Gamma"}){t.Fatalf("lost verified set: %#v",got)}
 got["which brands"][0]="MUTATED"
 if reload.preferSetsFor(fp)["which brands"][0]!="Alpha"{t.Fatal("caller mutated memory through shared slice")}
 other:="https://survey.example.test/other?pid=QA741"
 if sets:=openMemory(root,other).preferSetsFor(memoryFingerprint(other));len(sets)!=0{t.Fatalf("cross-study memory contamination: %v",sets)}
}

func TestPhase4UnqualifiedAttemptsCannotSaveSuccessfulSets(t *testing.T){
 s:=openMemory(t.TempDir())
 fp:=memoryFingerprint("https://survey.example.test/start?pid=QATEST")
 s.noteAttempt(fp,[]answerPair{{Question:"brands",Option:"Alpha"}},"screenout_or_termination")
 if n:=s.noteCompletedSets(fp,map[string][]string{"brands":{"Alpha","Beta"}});n!=0{t.Fatal("unqualified answer set recorded as successful")}
 if len(s.preferSetsFor(fp))!=0{t.Fatal("unqualified set available for replay")}
}

func TestPhase4VerifiedMultiSetsRequireSubmittedCheckedSnapshot(t *testing.T){
 good:=QuestionState{
  FullKey:"Select brands",Type:"multi",N:3,On:2,
  ObservedOptions:[]ObservedOption{
   {Label:"Alpha",Selected:true},
   {Label:"Beta",Selected:false},
   {Label:"Gamma",Selected:true},
  },
 }
 report:=StepReport{Step:5,Action:"checkbox+next",Unanswered:0,Questions:[]QuestionState{good}}
 if got:=verifiedMultiSets([]StepReport{report},1)["select brands"];!reflect.DeepEqual(got,[]string{"Alpha","Gamma"}){t.Fatalf("valid submitted snapshot: %v",got)}
 cases:=[]struct{name string;step StepReport}{
  {"old-attempt",StepReport{Step:4,Action:"checkbox+next",Questions:[]QuestionState{good}}},
  {"not-submitted",StepReport{Step:5,Action:"checkbox",Questions:[]QuestionState{good}}},
  {"unanswered",StepReport{Step:5,Action:"checkbox+next",Unanswered:1,Questions:[]QuestionState{good}}},
  {"validation",StepReport{Step:5,Action:"checkbox+next",Validation:[]string{"Invalid"},Questions:[]QuestionState{good}}},
  {"incomplete",StepReport{Step:5,Action:"checkbox+next",Questions:[]QuestionState{{FullKey:good.FullKey,Type:"multi",N:3,On:3,ObservedOptions:good.ObservedOptions}}}},
  {"grid",StepReport{Step:5,Action:"checkbox+next",Questions:[]QuestionState{{FullKey:good.FullKey,Type:"multi",Grid:true,N:3,On:2,ObservedOptions:good.ObservedOptions}}}},
 }
 for _,tc:=range cases{if got:=verifiedMultiSets([]StepReport{tc.step},5);len(got)>0{t.Errorf("%s falsely remembered %v",tc.name,got)}}
}
