function label(e){if(!e)return '';let id=e.id;if(id){let l=document.querySelector('label[for="'+esc(id)+'"]');if(l)return txt(l)}let l=e.closest?.('label');if(l)return txt(l);let labelled=e.getAttribute?.('aria-label');if(labelled)return labelled;let al=e.getAttribute?.('aria-labelledby');if(al){return al.split(/\s+/).map(id=>document.getElementById(id)).filter(Boolean).map(txt).join(' ')}return txt(e.parentElement).slice(0,200)}
function questionContainer(e){return e?.closest?.('fieldset,[role="radiogroup"],[role="group"],[role="presentation"],.question,.question-container,[class*="question"],[class*="Question"],form,[data-question]') || e?.parentElement || e}
function questionText(e){let c=questionContainer(e);let t=txt(c);if(t.length<20 && c?.parentElement)t=txt(c.parentElement);return t.slice(0,900)}
// Total by contract: it is called with whatever an analyzer or an adapter happened to
// find, including nothing at all, and one missing element must not take the whole engine
// down with it. A page the engine cannot read is a run that never finishes.
function stableQuestion(e){if(!e||!e.nodeType)return '';let c=e?.closest?.('.cf-question,.questionText,[id^="cq"],[class*="question__"],[class*="question__title i"]')||e?.closest?.('fieldset,[role="radiogroup"],[role="group"],[role="presentation"],.question,.question-container,[class*="question"],[class*="Question"],form,[data-question]')||e?.parentElement||e||e;let src=c;try{const cl=c.cloneNode(true);for(const n of cl.querySelectorAll('style,script,noscript,template,[role="alert"],[aria-live="assertive"],.error,.errors,.validation-error,[class*="error"],[class*="Error"],[class*="validation"],[class*="Validation"]'))n.remove();src=cl}catch{}let t=norm(String(src.textContent||''));if(t.length<20&&c?.parentElement){const p=c.parentElement;let ps='';try{const cl2=p.cloneNode(true);for(const n of cl2.querySelectorAll('style,script,noscript,template'))n.remove();ps=String(cl2.textContent||'')}catch{ps=String(p.textContent||'')}if(norm(ps).length>=t.length)t=norm(ps)}if(t.length<25){let up=c,best=t;for(let i=0;i<4&&up;i++){up=up.parentElement;if(!up||/^(FORM|BODY|HTML)$/.test(up.tagName||''))break;let x='';try{const cl3=up.cloneNode(true);for(const n of cl3.querySelectorAll('style,script,noscript,template'))n.remove();x=norm(String(cl3.textContent||''))}catch{x=norm(String(up.textContent||''))}if(x.length>best.length)best=x}if(best.length>=t.length)t=best}
if(t.length<20)t=questionText(e);return t.slice(0,900)}
function attrBlob(e){const out=[];const put=v=>{const s=v==null?'':String(v).trim();if(s&&s.length<200)out.push(s)};let n=e,d=0;while(n&&n.nodeType===1&&d<6&&!/^(FORM|BODY|HTML|HEAD)$/.test(n.tagName)){put(n.id);for(const a of ['name','placeholder','title','aria-label','aria-labelledby','aria-errormessage','data-name','data-label','data-question'])put(n.getAttribute&&n.getAttribute(a));n=n.parentElement;d++}try{put(label(e))}catch{}return out.join(' ')}
const contexts=[...new Set(native.map(questionText).filter(t=>t.length>3))].slice(0,10);
let q=contexts.join(' | ');
const inventory={nativeInputs:native.length,nativeRadios:nativeRadios.length,nativeCheckboxes:nativeChecks.length,roleRadios:roleRadios.length,roleCheckboxes:roleChecks.length,customChoices:customChoices.length,selects:selects.length,textInputs:textInputs.length,clickables:clickable.length,iframes:frames.length,frameInfo:frames.slice(0,5),sampleInteractive:native.filter(e=>vis(e)&&txt(e)).slice(0,40).map(e=>({tag:e.tagName,role:e.getAttribute('role')||'',type:e.getAttribute('type')||'',text:label(e).slice(0,120),name:e.getAttribute('name')||'',id:e.id||'',tabindex:e.getAttribute('tabindex')||'',value:e.getAttribute('data-value')||''}))};
function fire(el){try{el.scrollIntoView({block:'center',inline:'center'})}catch{} try{el.click();return true}catch{} try{for(const t of ['pointerdown','mousedown','pointerup','mouseup','click'])el.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,view:window}));return true}catch{return false}}
// A dispatched click is not an accepted answer. When native checked state or
// ARIA selection state is exposed, require it to agree before recording a pair
// or clearing the Next gate. Custom data-only widgets remain best-effort.
function fireChoice(e){
 if(!fire(e))return false;
 const hasNative=e.matches?.('input[type="radio"],input[type="checkbox"]');
 const hasAria=e.hasAttribute?.('aria-checked')||e.hasAttribute?.('aria-selected');
 if(hasNative&&!isOn(e)){
  const lab=[...(e.labels||[])].find(vis);
  if(lab)fire(lab);
 }
 if((hasNative||hasAria)&&!isOn(e))return false;
 markSel(e);
 return true;
}
function visibleLabelCandidates(container){return [...container.querySelectorAll?.('label,[role="radio"],[role="checkbox"],[role="option"],button,a,[tabindex]')||[]].filter(vis)}
// matchWant finds the option a persona answer should land on: exact text first, then
// a whole-word match so "Male" never picks "Female" or "woman". Returns -1 when the
// persona value is not offered, which lets the caller fall back to a random option -
// one survey offering "18-24" and the next offering "25" must both stay answerable.
function matchWant(labels,want,kind){
 if(!want)return -1;
 const w=String(want).toLowerCase().trim();if(!w)return -1;
 const norm=labels.map(x=>String(x).toLowerCase().trim());
 let i=norm.indexOf(w);if(i>=0)return i;
 const esc=w.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 const re=new RegExp('(^|[^a-z])'+esc+'([^a-z]|$)');
 i=norm.findIndex(s=>re.test(s));if(i>=0)return i;
 if(kind==='gender')return -1;
 return norm.findIndex(s=>s.indexOf(w)>=0);
}
function chooseFromGroup(items,want,kind,qt,key,previousColumn){
  const k=key||qkey(qt||'');
  const active=items.filter(e=>!e.disabled&&vis(e)&&!(e.matches?.('input')&&e.checked)&&e.getAttribute?.('aria-checked')!=='true');if(!active.length)return false;
 let pool=active.filter(e=>!isAvoid(k,label(e)));
 if(!pool.length){exhausted.add(k);return false}
 const labels=pool.map(label);
 // Everything the store knows about this question narrows the field to its best
 // answers first: the profile that completed, the profile being replayed, and the
 // answers with the better record. Only inside that field do persona and chance apply,
 // and a profile that already failed is never rebuilt here.
 const remembered=preferredFor(k);
 const tier=bestTier(k,labels,pool);
 const short=tier.length? tier.map(i=>pool[i]) : pool;
 const shortLabels=short.map(label);
 let pick=-1;
 const tryText=(t,kd)=>{if(!t)return -1;const i=matchWant(shortLabels,t,kd);return i};
 if(remembered)pick=tryText(remembered,kind);
 // "Kinetic yes", "BSN no": when the options carry the name of the thing being asked
 // about, this is one question per item rather than one yes/no question, and the persona's
 // flat answer would put the same word on every row. The profiles that completed this
 // survey said yes to roughly 45% of the brands they were asked about - a person knows
 // some brands and not others. A row of identical answers is what a screen-out reads as,
 // so the mix is drawn here instead, and the persona answer is left for real yes/no.
 // "Kinetica yes" / "Kinetica no" is one question per item: the options differ only in
 // the yes/no word. "Yes, definitely" / "No, not really" is an ordinary yes/no question
 // whose options happen to contain the words - the rest of the text has to be the SAME on
 // every option before this counts as the per-item shape, or a real yes/no question would
 // be answered at random instead of from the persona.
 if(pick<0&&want)pick=tryText(want,kind);
 if(pick<0&&usePersona&&kind==='yesno'&&previousColumn===undefined)pick=tryText(P.YesNo,kind);
 if(pick<0){
  const choices=short.map((e,i)=>({e,i})).filter(x=>previousColumn===undefined||items.indexOf(x.e)!==previousColumn);
  pick=(choices.length?choices:short.map((e,i)=>({e,i})))[rnd(choices.length||short.length)].i;
 }
 const chosen=short[pick];
 const ok=fireChoice(chosen);if(ok){selected.push(label(chosen));pairs.push({question:k,option:qkey(label(chosen))});pickedTrail.push(qkey(k)+'\u0000'+qkey(label(chosen)))}return ok;
}
// writeValue puts a string into a field the way a browser does it, so a framework that
// reads the value setter (React, Vue) still sees the change: assigning .value alone is
// invisible to those and the field reports itself empty again on the next pass.
// typeInto types the answer key by key instead of pasting it. An open answer that is
// pasted in one shot is invisible to the surveys that listen for typing (auto-save,
// counters, per-keystroke validation), and those reject the page with
// "one or more questions require further input" while the field looks filled.
function typeInto(e,v){
 if(!e)return false;
 const s=String(v==null?'':v);
 try{e.focus()}catch{}
 const proto=e.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
 const set=Object.getOwnPropertyDescriptor(proto,'value')?.set;
 const put=val=>{try{if(set)set.call(e,val);else e.value=val}catch{}};
 const key=(type,ch)=>{try{const o={key:ch,code:'Key'+(ch.toUpperCase()===' '?'Space':ch.toUpperCase()),keyCode:ch.toUpperCase().charCodeAt(0),which:ch.toUpperCase().charCodeAt(0),bubbles:true,cancelable:true};const C=/^(keydown|keyup)$/.test(type)?KeyboardEvent:Event;e.dispatchEvent(new C(type,o))}catch{}};
 if(e.isContentEditable){
  try{e.textContent='';for(const ch of s){e.textContent=(e.textContent||'')+ch;e.dispatchEvent(new InputEvent('input',{bubbles:true,data:ch,inputType:'insertText'}))}}catch{}
  e.dispatchEvent(new Event('change',{bubbles:true}));return true;
 }
 key('keydown','a');
 if(String(e.value||'')!==''){put('');e.dispatchEvent(new InputEvent('input',{bubbles:true,data:null,inputType:'deleteContentBackward'}));key('keyup','a')}
 let cur='';
 for(const ch of s){
  key('keydown',ch);
  cur+=ch;put(cur);
  e.dispatchEvent(new InputEvent('input',{bubbles:true,data:ch,inputType:'insertText'}));
  key('keyup',ch);
 }
 e.dispatchEvent(new Event('change',{bubbles:true}));
 e.dispatchEvent(new Event('blur',{bubbles:true}));
 return true;
}
function writeValue(e,v){
 if(!e)return false;
 const s=String(v==null?'':v);
 try{
  if(e.isContentEditable){e.focus();e.textContent=s;e.dispatchEvent(new InputEvent('input',{bubbles:true,data:s,inputType:'insertText'}));e.dispatchEvent(new Event('change',{bubbles:true}));return true}
  const proto=e.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
  const setter=Object.getOwnPropertyDescriptor(proto,'value')?.set;
  if(setter){setter.call(e,s)}else{e.value=s}
  e.focus();
  e.dispatchEvent(new Event('input',{bubbles:true}));
  e.dispatchEvent(new Event('change',{bubbles:true}));
  e.dispatchEvent(new Event('blur',{bubbles:true}));
  return true;
 }catch{return false}
}
// dragTo walks a pointer (and an HTML5 drag sequence) from the handle's own position to
// the target point in visible steps. A single synthetic jump is ignored by every
// slider and drop zone that listens for a drag over time, which is exactly how the
// emo-resonance circle and the timeline cards are moved.
function dragTo(h,x,y){
 if(!h)return false;
 // Everything the caller measured was measured before this scroll, so every point
 // (start included) is shifted by exactly as much as the document moved.
 let sy0=0,sy1=0;
 try{sy0=window.scrollY||0;h.scrollIntoView({block:'center',inline:'center'});sy1=window.scrollY||0}catch{}
 const moved=Math.round(sy1-sy0);
 let r;
 try{r=h.getBoundingClientRect()}catch{return false}
 let sx=Math.round(r.left+Math.max(1,r.width/2));
 let sy=Math.round(r.top+Math.max(1,r.height/2))-moved;
 let ex=Math.round(x)-moved,ey=Math.round(y)-moved;
 // elementFromPoint only sees the viewport: scroll the drop point back into it, moving
 // every point by that same amount, or a slot below the fold is never under the cursor.
 try{
  const vw=window.innerWidth||document.documentElement.clientWidth||1024;
  const vh=window.innerHeight||document.documentElement.clientHeight||768;
  if(ex<20||ey<20||ex>vw-20||ey>vh-20){
   const before=window.scrollY||0;
   window.scrollBy(0,Math.round(ey-vh/2));
   const d=Math.round((window.scrollY||0)-before);
   sy-=d;ex-=d;ey-=d;
  }
 }catch{}
 const send=(t,type,cx,cy,extra)=>{
  if(!t)return;
  const o={bubbles:true,cancelable:true,view:window,clientX:cx,clientY:cy,screenX:cx,screenY:cy,button:0,buttons:1,pointerId:1,pointerType:'mouse',isPrimary:true,composed:true};
  if(extra)for(const k in extra)o[k]=extra[k];
  try{
   const C=/^pointer/.test(type)?window.PointerEvent:/^(drag|drop)/.test(type)?window.DragEvent:MouseEvent;
   if(!C)throw new Error('no event');
   t.dispatchEvent(new C(type,o));
   return;
  }catch{}
  try{t.dispatchEvent(new MouseEvent(type,{bubbles:true,cancelable:true,view:window,clientX:cx,clientY:cy,button:0,buttons:1}))}catch{}
 };
 let dt=null;
 try{dt=new DataTransfer()}catch{}
 const dd=dt?{dataTransfer:dt}:null;
 const par=h.parentElement||null;
 const touch=(t,type,cx,cy)=>{try{
  const tk=new Touch({identifier:1,target:t,clientX:cx,clientY:cy,pageX:cx,pageY:cy});
  t.dispatchEvent(new TouchEvent(type,{bubbles:true,cancelable:true,touches:type==='touchend'?[]:[tk],targetTouches:type==='touchend'?[]:[tk],changedTouches:[tk],view:window}));
 }catch{}};
 send(h,'pointerdown',sx,sy);
 send(h,'mousedown',sx,sy);
 touch(h,'touchstart',sx,sy);
 if(par){send(par,'pointerdown',sx,sy);send(par,'mousedown',sx,sy);touch(par,'touchstart',sx,sy)}
 send(h,'dragstart',sx,sy,dd);
 const dx=ex-sx,dy=ey-sy,steps=6;
 for(let i=1;i<=steps;i++){
  const px=Math.round(sx+dx*i/steps),py=Math.round(sy+dy*i/steps);
  send(document,'pointermove',px,py);
  send(document,'mousemove',px,py);
  send(document,'drag',px,py,dd);
  touch(document,'touchmove',px,py);
  // A widget that captures the pointer keeps listening on the element it was pressed
  // on, so the handle (and the box that holds it) see the whole movement too.
  send(h,'pointermove',px,py);
  send(h,'mousemove',px,py);
  touch(h,'touchmove',px,py);
  if(par){send(par,'pointermove',px,py);send(par,'mousemove',px,py);touch(par,'touchmove',px,py)}
  const el=document.elementFromPoint(px,py);
  if(el&&el!==document){
   send(el,'pointermove',px,py);
   send(el,'mousemove',px,py);
   send(el,'dragenter',px,py,dd);
   send(el,'dragover',px,py,dd);
  }
 }
 const t=document.elementFromPoint(ex,ey)||h;
 send(t,'pointermove',ex,ey);
 send(t,'mousemove',ex,ey);
 send(t,'dragover',ex,ey,dd);
 send(t,'drop',ex,ey,dd);
 send(t,'pointerup',ex,ey);
 send(t,'mouseup',ex,ey);
 send(h,'pointerup',ex,ey);
 send(h,'mouseup',ex,ey);
 touch(h,'touchend',ex,ey);
 if(par){send(par,'pointerup',ex,ey);send(par,'mouseup',ex,ey)}
 send(h,'dragend',ex,ey,dd);
 send(t,'click',ex,ey);
 return true;
}
