package main

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "sync"
    "testing"
)

type fakeSurveyCDP struct {
    mu sync.Mutex
    pageURL string
    requested string
    id string
    wsURL string
    navigateCount int
    rejectNavigate bool
}

func (f *fakeSurveyCDP) current() string { f.mu.Lock();defer f.mu.Unlock();return f.pageURL }
func (f *fakeSurveyCDP) requests() int {f.mu.Lock();defer f.mu.Unlock();return f.navigateCount }

func serveFakeSurveyCDP(t *testing.T, originalURL string, targetID string) (*httptest.Server, *fakeSurveyCDP) {
    t.Helper()
    f:=&fakeSurveyCDP{pageURL:originalURL,requested:"https://survey.example.test/study.aspx",id:targetID}
    srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.URL.Path=="/json/list"{
            _=json.NewEncoder(w).Encode([]Target{{Id:f.id,URL:f.current(),Type:"page",WebSocketDebuggerURL:f.wsURL}})
            return
        }
        if r.URL.Path!="/devtools/page/test" {http.NotFound(w,r);return}
        hj,ok:=w.(http.Hijacker);if !ok {http.Error(w,"hijack not supported",500);return}
        conn,reader,err:=hj.Hijack();if err!=nil{return}
        defer conn.Close()
        _,_ = io.WriteString(conn,"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
        for {
            var hdr [2]byte
            if _,err=io.ReadFull(reader,hdr[:]);err!=nil{return}
            size:=int(hdr[1]&127)
            if size==126 {var x [2]byte;if _,err=io.ReadFull(reader,x[:]);err!=nil{return};size=int(binary.BigEndian.Uint16(x[:]))}
            if size==127{return}
            if size>100000{return}
            var mask [4]byte
            if hdr[1]&128 !=0 {if _,err=io.ReadFull(reader,mask[:]);err!=nil{return}}
            b:=make([]byte,size)
            if _,err=io.ReadFull(reader,b);err!=nil{return}
            if hdr[1]&128!=0 {for i:=range b{b[i]^=mask[i%4]}}
            var msg struct {
                ID int `json:"id"`
                Method string `json:"method"`
                Params map[string]any `json:"params"`
            }
            if json.Unmarshal(b,&msg)!=nil{continue}
            var payload any=map[string]any{"id":msg.ID,"result":map[string]any{}}
            switch msg.Method {
            case "Runtime.evaluate":
                payload=map[string]any{"id":msg.ID,"result":map[string]any{"result":map[string]any{"value":f.current()}}}
            case "Page.navigate":
                f.mu.Lock()
                f.navigateCount++
                if !f.rejectNavigate{f.pageURL=f.requested}
                reject:=f.rejectNavigate
                f.mu.Unlock()
                if reject{payload=map[string]any{"id":msg.ID,"error":map[string]any{"code":-32000,"message":"synthetic navigation rejected"}}}
            }
            res,_:=json.Marshal(payload)
            frame:=[]byte{0x81}
            if len(res)<126{frame=append(frame,byte(len(res)))}else{frame=append(frame,126,byte(len(res)>>8),byte(len(res)))}
            frame=append(frame,res...)
            if _,err=conn.Write(frame);err!=nil{return}
        }
    }))
    parsed,e:=url.Parse(srv.URL)
    if e!=nil{t.Fatal(e)}
    f.wsURL="ws://"+parsed.Host+"/devtools/page/test"
    return srv,f
}

func TestStartupNavigationAlreadyReachedSurveyWithoutSecondSubmission(t *testing.T) {
    wanted:="https://survey.example.test/study.aspx?test=1"
    srv,f:=serveFakeSurveyCDP(t,wanted,"tab-1")
    defer srv.Close()
    u,_:=url.Parse(srv.URL)
    port:=0
    _,_ = fmt.Sscanf(u.Port(),"%d",&port)
    sess:=&BrowserSession{port:port}
    conn,target,err:=reconnectAfterNavigationTimeout(sess,Target{Id:"tab-1"},wanted,20000,nil)
    if err!=nil{t.Fatal(err)}
    defer conn.c.Close()
    if target.Id!="tab-1"||sess.ws!=conn||f.requests()!=0 {t.Fatalf("recovery reused wrong tab or repeated navigation: tab=%s calls=%d",target.Id,f.requests())}
}

func TestStartupNavigationRetriesOnceWhenOriginalTabStillBlank(t *testing.T){
    srv,f:=serveFakeSurveyCDP(t,"about:blank","tab-1")
    defer srv.Close()
    u,_:=url.Parse(srv.URL);port:=0;_,_=fmt.Sscanf(u.Port(),"%d",&port)
    sess:=&BrowserSession{port:port}
    conn,_,err:=reconnectAfterNavigationTimeout(sess,Target{Id:"tab-1"},f.requested,30000,nil)
    if err!=nil{t.Fatal(err)}
    defer conn.c.Close()
    if f.requests()!=1||f.current()!=f.requested {t.Fatalf("expected one successful retry, requests=%d url=%s",f.requests(),f.current())}
}

func TestStartupRecoveryNeverNavigatesAnUnrelatedTab(t *testing.T){
    srv,f:=serveFakeSurveyCDP(t,"about:blank","other-tab")
    defer srv.Close()
    u,_:=url.Parse(srv.URL);port:=0;_,_=fmt.Sscanf(u.Port(),"%d",&port)
    sess:=&BrowserSession{port:port}
    _,_,err:=reconnectAfterNavigationTimeout(sess,Target{Id:"original-tab"},f.requested,40000,nil)
    if err==nil||!strings.Contains(err.Error(),"original survey tab"){t.Fatalf("must reject unrelated tab: %v",err)}
    if f.requests()!=0{t.Fatal("unexpected navigation on unrelated tab")}
}

func TestStartupNavigationRejectedIsReportedNotInventedAsRecovered(t *testing.T){
    srv,f:=serveFakeSurveyCDP(t,"about:blank","tab-1")
    f.rejectNavigate=true
    defer srv.Close()
    u,_:=url.Parse(srv.URL);port:=0;_,_=fmt.Sscanf(u.Port(),"%d",&port)
    sess:=&BrowserSession{port:port}
    conn,_,err:=reconnectAfterNavigationTimeout(sess,Target{Id:"tab-1"},f.requested,50000,nil)
    if conn!=nil{defer conn.c.Close()}
    if err==nil{t.Fatal("rejected navigation cannot be marked successful")}
    if f.requests()!=1{t.Fatalf("must retry at most once, got %d",f.requests())}
}

func TestNavigationMatchRequiresSameSchemeHostAndSurveyPath(t *testing.T){
    start:="https://survey.example.test/extwix/test_p123.aspx?test=1"
    for _,tc:=range []struct{url string;good bool}{
        {"https://survey.example.test/extwix/test_p123.aspx?test=2",true},
        {"https://survey.example.test/BCEndPages/en-Qual.html",true},
        {"https://survey.example.test/BCEndPages/en-Term.html",true},
        {"https://survey.example.test/other.aspx",false},
        {"https://another.example.test/extwix/test_p123.aspx",false},
        {"http://survey.example.test/extwix/test_p123.aspx",false},
        {"about:blank",false},
    }{
        if got:=navigationMatchesSurvey(tc.url,start);got!=tc.good{t.Errorf("for %q expected %t, got %t",tc.url,tc.good,got)}
    }
}
