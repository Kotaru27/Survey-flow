package main
import "testing"
func TestMemoryWorkerLimit(t *testing.T){const gib=uint64(1024*1024*1024);for _,c:=range []struct{n uint64;ok bool;want int}{{0,false,1},{5*gib,true,1},{6*gib,true,2},{30*gib,true,2}}{if got:=memoryWorkerLimit(c.n,c.ok);got!=c.want{t.Errorf("memoryWorkerLimit(%d,%v)=%d",c.n,c.ok,got)}}}

func TestMemoryWorkerCapacity(t *testing.T){for _,c:=range []struct{n uint64;ok bool;want int}{{0,false,1},{6*gibBytes,true,2},{7*gibBytes,true,3},{9*gibBytes,true,4}}{if got:=memoryWorkerCapacity(c.n,c.ok);got!=c.want{t.Errorf("capacity=%d want=%d",got,c.want)}}}

func TestStartupWorkerSelection(t *testing.T){
 high:=AppConfig{memoryProbe:func()(uint64,bool){return 8*gibBytes,true}};low:=AppConfig{memoryProbe:func()(uint64,bool){return 3*gibBytes,true}}
 if got:=workerCountForRun(high,false,10);got!=2{t.Errorf("high memory: %d",got)}
 if got:=workerCountForRun(low,false,10);got!=1{t.Errorf("low memory: %d",got)}
 if got:=workerCountForRun(high,true,10);got!=1{t.Errorf("visible mode: %d",got)}
 if got:=workerCountForRun(high,false,1);got!=1{t.Errorf("single target: %d",got)}
}

func TestAdaptiveWorkerAdmission(t *testing.T){
 const mb=uint64(1024*1024)
 tests:=[]struct{available,worker uint64;cpu float64;measured bool;want int}{
  {4*gibBytes,300*mb,20,true,2},
  {2*gibBytes+300*mb,300*mb,20,true,1},
  {4*gibBytes,300*mb,90,true,1},
  {4*gibBytes,300*mb,20,false,1},
  {4*gibBytes,0,20,true,1},
 }
 for _,tc:=range tests {if got:=adaptiveWorkerLimit(tc.available,tc.worker,tc.cpu,tc.measured,2);got!=tc.want{t.Errorf("admission %+v got %d",tc,got)}}
}
