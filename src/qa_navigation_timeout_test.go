package main

import (
 "errors"
 "net"
 "testing"
 "time"
)

func TestQANavigationTimeoutIsScoped(t *testing.T) {
 if got:=cdpResponseTimeout("Page.navigate");got!=5*time.Second{t.Fatalf("navigation deadline %v",got)}
 for _,method:=range []string{"Runtime.enable","Page.enable","Runtime.evaluate","Page.captureScreenshot"}{
  if got:=cdpResponseTimeout(method);got!=12*time.Second{t.Errorf("%s timeout changed: %v",method,got)}
 }
}

func TestCDPFrameTimeoutIsEnforced(t *testing.T) {
 server,client:=net.Pipe()
 defer server.Close()
 defer client.Close()
 w:=&WS{c:client}
 start:=time.Now()
 _,err:=w.recvWithin(70*time.Millisecond)
 var networkErr net.Error
 if !errors.As(err,&networkErr)||!networkErr.Timeout(){t.Fatalf("expected a network timeout, got %v",err)}
 if elapsed:=time.Since(start);elapsed>time.Second{t.Fatalf("socket deadline was not enforced: %v",elapsed)}
}
