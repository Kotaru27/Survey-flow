//go:build windows
package main
import("os";"path/filepath";"testing";"time")
func TestSyntheticLogWriteTiming(t *testing.T){
 root:=t.TempDir();measure:=func(syncEach bool)time.Duration{
  f,err:=os.CreateTemp(root,"log-*");if err!=nil{t.Fatal(err)};defer os.Remove(f.Name());defer f.Close()
  start:=time.Now();for i:=0;i<80;i++{if _,err=f.WriteString("synthetic QA log entry\n");err!=nil{t.Fatal(err)};if syncEach {if err=f.Sync();err!=nil{t.Fatal(err)}}};if err=f.Sync();err!=nil{t.Fatal(err)};return time.Since(start)
 }
 t.Logf("synthetic 80 writes: sync each=%s, sync once=%s; folder=%s",measure(true),measure(false),filepath.Base(root))
}
