package main
import (
 "fmt"
 "sort"
 "strings"
 "time"
 "unicode/utf8"
)
type workerView struct {run,done,percent,tick int; active bool}
func(r *consoleRenderer) enableWorkers(n int){r.mu.Lock();defer r.mu.Unlock();r.parallel=true;r.rows=1;r.workerViews=map[int]*workerView{};for i:=1;i<=n;i++{r.workerViews[i]=&workerView{percent:-1}};r.drawLocked(true)}
func(r *consoleRenderer) workerStarted(id,run int){r.mu.Lock();defer r.mu.Unlock();v:=r.workerViews[id];if v==nil{return};v.run=run;v.percent=0;v.active=true;r.drawLocked(true)}
func(r *consoleRenderer) workerProgress(id int,p float64){r.mu.Lock();defer r.mu.Unlock();v:=r.workerViews[id];if v==nil||!v.active{return};if p>=0&&p<=100{v.percent=int(p)}else{v.percent=-1};v.tick++;r.drawLocked(false)}
func(r *consoleRenderer) workerResources(available uint64,cpu float64,valid bool){r.mu.Lock();defer r.mu.Unlock();if valid{r.resourceText=fmt.Sprintf(" · RAM free %.1f GB · CPU %.0f%%",float64(available)/float64(gibBytes),cpu)}else{r.resourceText=" · Resource sampling"};r.drawLocked(false)}
func(r *consoleRenderer) workerFinished(id int,status string,complete,total int){r.mu.Lock();defer r.mu.Unlock();v:=r.workerViews[id];if v==nil{return};v.percent=100;v.done++;r.completed=complete;r.target=total;r.attempts++;switch strings.ToLower(status){case "completed":case "screenout_pattern","screenout_or_termination":r.screenouts++;case "quota","quota_full":r.quotas++;default:r.failures++};r.drawLocked(true);v.active=false}
func progressBar(p,tick,width int) string {if width<1{return ""};if p<0 {s:=[]rune(strings.Repeat("-",width));s[tick%width]='>';return "["+string(s)+"] --"};if p>100{p=100};if p<0{p=0};n:=p*width/100;return fmt.Sprintf("[%s%s] %d%%",strings.Repeat("#",n),strings.Repeat("-",width-n),p)}
func(r *consoleRenderer) drawWorkersLocked(force bool){
 if !r.colored{return};if !force&&time.Since(r.lastDraw)<200*time.Millisecond{return};r.lastDraw=time.Now();width:=r.width();if width<2{width=2};barWidth:=12;if width<50{barWidth=6}
 p:=0;if r.target>0{p=100*r.completed/r.target};mode:="BACKGROUND";if r.visible{mode="VISIBLE"}
 lines:=[]string{fmt.Sprintf("SURVEY AUTOPILOT  ·  %s",mode),fmt.Sprintf("QUALIFICATIONS %s  %d/%d",progressBar(p,0,barWidth),r.completed,r.target)}
 ids:=make([]int,0,len(r.workerViews));for id:=range r.workerViews{ids=append(ids,id)};sort.Ints(ids);active:=0
 for _,id:=range ids{v:=r.workerViews[id];if !v.active{continue};active++;lines=append(lines,fmt.Sprintf("W%d %s  Survey %d · Done %d",id,progressBar(v.percent,v.tick,barWidth),v.run,v.done))}
 lines=append(lines,fmt.Sprintf("Attempts %d · Screenout %d · Quota %d · Other %d",r.attempts,r.screenouts,r.quotas,r.failures),fmt.Sprintf("Workers %d active · Elapsed %s%s",active,time.Since(r.started).Round(time.Second),r.resourceText))
 old:=r.rows;if old<1{old=1};if old>1{fmt.Fprintf(r.out,"\r\x1b[%dA",old-1)}else{fmt.Fprint(r.out,"\r")};count:=len(lines);if old>count{count=old}
 for i:=0;i<count;i++{fmt.Fprint(r.out,"\x1b[2K");if i<len(lines){line:=safeConsoleText(lines[i],width-1);if utf8.RuneCountInString(line)>width-1{line=string([]rune(line)[:width-1])};fmt.Fprint(r.out,r.stylize(mint,line))};if i<count-1{fmt.Fprint(r.out,"\n")}}
 if count>len(lines){fmt.Fprintf(r.out,"\r\x1b[%dA",count-len(lines))};r.rows=len(lines)
}
