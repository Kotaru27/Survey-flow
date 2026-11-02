package main
import "testing"
func TestMemoryWorkerLimit(t *testing.T){const gib=uint64(1024*1024*1024);for _,c:=range []struct{n uint64;ok bool;want int}{{0,false,1},{5*gib,true,1},{6*gib,true,2},{30*gib,true,2}}{if got:=memoryWorkerLimit(c.n,c.ok);got!=c.want{t.Errorf("memoryWorkerLimit(%d,%v)=%d",c.n,c.ok,got)}}}

func TestMemoryWorkerCapacity(t *testing.T){for _,c:=range []struct{n uint64;ok bool;want int}{{0,false,1},{6*gibBytes,true,2},{7*gibBytes,true,3},{9*gibBytes,true,4}}{if got:=memoryWorkerCapacity(c.n,c.ok);got!=c.want{t.Errorf("capacity=%d want=%d",got,c.want)}}}
