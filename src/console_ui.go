package main

import (
 "fmt"
 "io"
 "strings"
 "sync"
 "time"
 "unicode"
 "unicode/utf8"
)

var consoleUI *consoleRenderer
const (
 ink="\x1b[38;2;225;233;238m"; muted="\x1b[38;2;127;144;154m"; mint="\x1b[38;2;107;229;192m"; azure="\x1b[38;2;135;194;245m"; amber="\x1b[38;2;250;200;115m"; rose="\x1b[38;2;255;128;140m"; bold="\x1b[1m"; reset="\x1b[0m"
)
func safeConsoleText(s string, limit int) string {
 s=strings.Map(func(r rune) rune {if unicode.IsControl(r)||r=='\u2028'||r=='\u2029' {return ' '};return r},s)
 s=strings.Join(strings.Fields(s)," "); if limit<2||utf8.RuneCountInString(s)<=limit{return s};return string([]rune(s)[:limit-1])+"…"
}
type consoleRenderer struct {
 mu sync.Mutex
 out io.Writer
 colored bool
 target, run, completed, percent, tick int
 running, visible, active, finished bool
 result string
 started, lastDraw time.Time
 width func() int
}
func newConsoleRenderer(out io.Writer, colored bool)*consoleRenderer{return &consoleRenderer{out:out,colored:colored,percent:-1,width:consoleWidth}}
func(r *consoleRenderer) stylize(c,s string) string {if r.colored{return c+s+reset};return s}
func(r *consoleRenderer) println(s string){_,_=fmt.Fprintln(r.out,s)}
func(r *consoleRenderer) header(){r.mu.Lock();defer r.mu.Unlock();r.println("\n  "+r.stylize(mint+bold,"SURVEY AUTOPILOT"));r.println("")}
func(r *consoleRenderer) section(title,detail string){r.mu.Lock();defer r.mu.Unlock();r.println("  "+r.stylize(muted,safeConsoleText(detail,72)))}
func(r *consoleRenderer) prompt(label,hint string){r.mu.Lock();defer r.mu.Unlock();r.println("\n  "+r.stylize(ink+bold,label));if hint!=""{r.println("  "+r.stylize(muted,hint))};_,_=fmt.Fprint(r.out,"  "+r.stylize(mint,"› "))}
func(r *consoleRenderer) detail(label,value string){r.mu.Lock();defer r.mu.Unlock();r.println("  "+r.stylize(muted,label+": ")+safeConsoleText(value,50))}
func(r *consoleRenderer) event(kind,color,s string){r.mu.Lock();defer r.mu.Unlock();if r.active {r.result="Attention required; see log";r.drawLocked(true);return};r.println("  "+r.stylize(color,kind)+"  "+safeConsoleText(s,70))}
func(r *consoleRenderer) startBatch(total int,visible bool){r.mu.Lock();defer r.mu.Unlock();r.target=total;r.visible=visible;r.started=time.Now();r.active=true;r.run=0;r.percent=-1;if r.colored {r.println("");r.drawLocked(true)}}
func(r *consoleRenderer) startRun(n,completed int){r.mu.Lock();defer r.mu.Unlock();r.run=n;r.completed=completed;r.percent=0;r.tick=0;r.running=true;r.finished=false;r.result="";r.drawLocked(true)}
func(r *consoleRenderer) setSurveyProgress(v float64){r.mu.Lock();defer r.mu.Unlock();if !r.running{return};if v>=0&&v<=100 {r.percent=int(v)} else {r.percent=-1};r.tick++;r.drawLocked(false)}
func(r *consoleRenderer) drawLocked(force bool){
 if !r.colored{return};if !force&&time.Since(r.lastDraw)<200*time.Millisecond{return};r.lastDraw=time.Now()
 width:=r.width();if width<=0 {width=80};if width<16{width=16}
 mode:="BG";if r.visible{mode="VISIBLE"}; state:=fmt.Sprintf("Survey %d · %s",r.run,mode);if r.run==0{state="Preparing"}
 pct:="--";bar:="";if r.percent>=0{pct=fmt.Sprintf("%d%%",r.percent)}
 suffix:=fmt.Sprintf("  %s  |  %d/%d qualified  |  %s",pct,r.completed,r.target,time.Since(r.started).Round(time.Second).String());if r.finished{suffix+="  "+r.result}
 available:=width-utf8.RuneCountInString(state)-utf8.RuneCountInString(suffix)-4
 if available>=5 {n:=available-2;if n>18{n=18};if r.percent>=0{fill:=r.percent*n/100;bar="["+strings.Repeat("#",fill)+strings.Repeat("-",n-fill)+"]"}else{pos:=r.tick%n;run:=[]rune(strings.Repeat("-",n));run[pos]='>';bar="["+string(run)+"]"}}
 line:=fmt.Sprintf("%s %s%s",state,bar,suffix);if r.run==0{line="Preparing · "+mode};line=safeConsoleText(line,width-1)
 _,_=fmt.Fprint(r.out,"\r\x1b[2K"+r.stylize(mint,line))
}
func(r *consoleRenderer) runFinished(n int,status string,done,total int){r.mu.Lock();defer r.mu.Unlock();r.run=n;r.completed=done;r.target=total;r.running=false;r.finished=true;r.percent=100;r.result=safeConsoleText(status,24);r.drawLocked(true)}
func statusColor(status string) string {if status=="completed"{return mint};return amber}
func outcomeCounts(batch BatchReport)(complete,terminated,incomplete int){for _,run:=range batch.Runs{switch strings.ToLower(strings.TrimSpace(run.Status)){case "completed":complete++;case "screenout_pattern","screenout_or_termination":terminated++;default:incomplete++}};return}
func(r *consoleRenderer) finish(batch BatchReport){r.mu.Lock();defer r.mu.Unlock();complete,terminated,incomplete:=outcomeCounts(batch);if r.colored {_,_=fmt.Fprintln(r.out)};r.active=false;r.println(fmt.Sprintf("  Completed %d / %d  ·  Screenouts %d  ·  Other %d",complete,batch.Requested,terminated,incomplete));if complete<batch.Requested{r.println("  Target not reached. See reports for details.")}}
func(r *consoleRenderer) onLog(s string){
 if strings.HasPrefix(s,"[") {r.mu.Lock();defer r.mu.Unlock();if r.running&&r.percent==0{r.percent=-1};r.tick++;r.drawLocked(false)}
}
