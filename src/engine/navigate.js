// The page is already settled when the runner got here, so this only waits for a late
// render: it returns as soon as the control count stops moving instead of always
// spending a fixed half second on every step. Frames, not timers, keep the wait honest -
// two animation frames is the cheapest proof that the DOM has stopped changing - but a
// frame is only a hint: a pane that is hidden or occluded is not painted at all, so every
// frame races a short timer and the frame count is capped.
await new Promise(resolve=>{
 let last=-1,stable=0,frames=0;
 const raf=window.requestAnimationFrame||(f=>setTimeout(f,16));
 const again=()=>{
  let fired=false;
  const go=()=>{if(fired)return;fired=true;clearTimeout(timer);tick()};
  const timer=setTimeout(go,20);
  raf(go);
 };
 const tick=()=>{
  const n=document.querySelectorAll('input,select,textarea,[role="radio"],[role="checkbox"],[role="option"],button').length;
  if(n===last){if(++stable>=2||++frames>90){resolve();return}}else{stable=0;frames=0;last=n}
  again();
 };
 tick();
});
ensureModel();
// The survey can take hundreds of milliseconds to mount the next page. Do not
// click Next again on the *same* visible page while its first navigation is in
// flight; repeated clicks can skip a question or submit twice. Full page loads
// naturally discard the window-scoped marker, while SPA transitions change the
// visible content fingerprint and clear it.
const navFingerprint=()=>location.href+'|'+norm(document.body?.innerText||'').slice(0,1800)+'|'+[...document.querySelectorAll('input,select,textarea')].filter(vis).slice(0,16).map(e=>String(e.name||e.id||e.type||'')).join(',');
const nowNavFP=navFingerprint();
const previousNav=window.__apNavIntent;
const waitingForNext=!!(previousNav&&previousNav.fp===nowNavFP&&Date.now()-previousNav.at<1800);
if(previousNav&&!waitingForNext)delete window.__apNavIntent;
const allButtons=[...new Set([...clickable,...native.filter(e=>/^(button|link|menuitem)$/i.test(e.getAttribute?.('role')||''))])].filter(vis);
const nextRegex=/^(next|continue|submit|finish|done|proceed|complete|go|confirm|save and continue|next page|go to next|>|>>|ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢)$/i;
// The carousel's own advance button says "next" too and sits inside the question box.
// The survey's Next never does, so the box is excluded from the search first and only
// taken back if there is no other way forward at all.
const inModelBox=e=>!!(pageModel&&pageModel.box&&pageModel.box.contains&&pageModel.box.contains(e));
const findNext=()=>allButtons.find(e=>nextRegex.test(txt(e))&&!e.disabled)||allButtons.find(e=>nextRegex.test((e.getAttribute('aria-label')||'').trim())&&!e.disabled)||null;
let next=findNext();
if(!next||inModelBox(next)){
 const outside=allButtons.filter(e=>!inModelBox(e));
 const hit=outside.find(e=>nextRegex.test(txt(e))&&!e.disabled)||outside.find(e=>nextRegex.test((e.getAttribute('aria-label')||'').trim())&&!e.disabled);
 if(hit)next=hit;
}
if(!next){const candidates=[...document.querySelectorAll('button,a,div,span,[tabindex],[role]')].filter(e=>vis(e)&&nextRegex.test(txt(e))&&txt(e).length<40);candidates.sort((a,b)=>a.querySelectorAll('*').length-b.querySelectorAll('*').length);next=candidates.find(e=>!inModelBox(e))||candidates[0]||null}
let unanswered=0;
const openQ=[];
refreshCustom();
// Some forms leave a page-wide error summary visible after the affected field
// has been corrected. Permit one re-submission only in the same step as a
// verified answer change, and only after every specific error has disappeared.
// An unchanged or still-specific error remains blocked.
const errorsAfterAnswers=readErrs();
const summaryOnly=errorsAfterAnswers.every(e=>/^(?:one or more questions require further input|please correct the errors below)[.!]?$/i.test(norm(e)));
if(valErr.length&&pairs.length&&summaryOnly){
 INSTRUCTION_BLOCK.details=INSTRUCTION_BLOCK.details.filter(x=>!['unmatched-validation','ambiguous-validation'].includes(x?.reason));
 INSTRUCTION_BLOCK.count=INSTRUCTION_BLOCK.details.length;
}
if(!completion&&!termination){
 for(const x of questions){if(qOpen(x)){unanswered++;if(openQ.length<8)openQ.push(x.key.slice(0,120))}}
 // The model can see further than the visible questions: a carousel keeps its other
 // slides in the DOM and a sequential list is not finished until every slot has its
 // own answer. Either one outranks the plain count so Next is never pressed early.
 const modelLeft=customUnanswered();
 if(modelLeft>unanswered)unanswered=modelLeft;
 // Some survey frameworks draw question headings before their answer controls.
 // Do not treat an empty inventory as a completed question during that gap.
 if(window.__apRenderPending){
  unanswered=Math.max(1,unanswered);
  if(openQ.length<8)openQ.push('question controls still rendering');
 }
 // While a choice accordion is opening, Next is not valid: new options have
 // not been inventoried yet, and a click on its heading is not an answer.
 if(accordion.pending>0||INSTRUCTION_BLOCK.count>0){
  unanswered=Math.max(1,unanswered);
  if(openQ.length<8)openQ.push('accordion: '+accordion.count+' categories, '+accordion.selected+' selected');
 }
 // A grid whose cells have no box yet is a question all the same: the survey refuses the
 // page until every brand on it is answered, so the brands still open are counted here
 // even though the control layer cannot see a single cell.
 const zeroLeft=(typeof window.__apZeroLeft==='number')?window.__apZeroLeft:0;
 if(zeroLeft>unanswered)unanswered=zeroLeft;
 // A widget owns its page, and the widget's own count is the list of prompts on it.
 // Once every brand, card or slide is placed, a control the plain scanner still counts
 // is the widget's own decoration (a hidden radio behind the board), not a question:
 // holding Next on those ended a run whose scale was already full.
 if(pageModel&&WIDGET_MODELS.has(pageModel.kind)&&modelLeft===0){
  // The circle widget owns only its own range controls. Other visible
  // questions on the page still need an answer before navigation.
  if(pageModel.kind==='emo_resonance')unanswered=questions.filter(x=>qOpen(x)&&!x.els.every(e=>pageModel.handles.includes(e))).length;
  else unanswered=0;
 }
}
// Ratings a widget committed in an earlier pass belong to this page's answer set: a widget
// answers one item per pass, and the pass that rated the last one is the pass that
// navigated away, so its reading never came back. They are parked on the page and handed
// over here; the runner empties the list only once it holds the result, so nothing is
// dropped and nothing is counted twice.
try{
 const pend=window.__apEmoPending;
 if(Array.isArray(pend))for(const it of pend){
  if(!pairs.some(p=>p.question===it.question&&p.option===it.option))pairs.push({question:it.question,option:it.option});
 }
}catch{}
action=[accordion.action,...acts].filter(Boolean).join('+');
// A carousel's own advance lives inside the question box, the survey's Next does not.
// While the box still has items the two must never be confused for each other.
const modelAdvance=(!completion&&!termination&&INSTRUCTION_BLOCK.count===0&&accordion.pending===0&&pageModel&&pageModel.kind==='carousel_grid'&&!pageModel.done)?customAdvance():null;
if(!completion&&!termination){
 if(window.__apRenderPending){action+=(action?'+':'')+'render-waiting'}
 else if(modelAdvance){if(fire(modelAdvance))action+=(action?'+':'')+'advance'}
 else if(waitingForNext&&unanswered===0){
  action+=(action?'+':'')+'next-waiting';
 }
 else if(next){
  if(INSTRUCTION_BLOCK.count===0&&accordion.pending===0&&unanswered===0&&(valErr.length===0||validationRecovery.changed||(needMore&&pairs.length>0)||(pairs.length>0&&summaryOnly))){
   const before=navFingerprint();
   if(fire(next)){
    window.__apNavIntent={fp:before,at:Date.now()};
    action+=(action?'+':'')+'next';
   }
  }
  else{action+=(action?'+':'')+'hold'}
 }
}
const chainOf=h=>{let n=h,out=[];for(let d=0;n&&d<5;d++,n=n.parentElement){out.push(n.tagName+'.'+String(n.getAttribute&&n.getAttribute('class')||'')+'['+String(n.getAttribute&&n.getAttribute('draggable')||'')+']'+(n.id?'#'+n.id:'')+'{'+String(n.getAttribute&&n.getAttribute('style')||'').slice(0,60)+'}')}return out.join(' < ')};
const detail=pageModel&&pageModel.handles?pageModel.handles.map(h=>((h.tagName||'')+'.'+String(cn(h).trim().split(/\s+/).slice(0,3).join('.'))+(isOn(h)?'*':'')+(vis(h)?'':'!')+'@'+Math.round(h.getBoundingClientRect().left)+','+Math.round(h.getBoundingClientRect().top)+' >> '+chainOf(h))).join(' ; ').slice(0,1400):'';
const scaleDetail=pageModel&&pageModel.kind==='scale_drop'&&pageModel.cards?(()=>{const tr=pageModel.track?pageModel.track.getBoundingClientRect():null;return 'TRACK '+String(cn(pageModel.track||''))+'|'+(tr?Math.round(tr.left)+','+Math.round(tr.top)+' '+Math.round(tr.width)+'x'+Math.round(tr.height):'none')+' ;; '+pageModel.cards.map(c=>{const r=c.getBoundingClientRect();return String(cn(c).trim().split(/\s+/).slice(0,4).join('.'))+'|'+txt(c).slice(0,12)+'|'+(/\bdropped\b/i.test(String(c.className||''))?'D':'-')+'|'+Math.round(r.left)+','+Math.round(r.top)+' '+Math.round(r.width)+'x'+Math.round(r.height)}).join(' ;; ')})().slice(0,1400):'';
const humanWall=()=>{try{
 const b=((document.body&&document.body.innerText)||'').toLowerCase();
 const t=/captcha|recaptcha|hcaptcha|i am not a robot|verify you are|verify that you|are you a human|press and hold|security check|checking your browser/.test(b);
 const f=[...document.querySelectorAll('iframe[src]')].some(e=>/captcha|recaptcha|hcaptcha|challenge/i.test(String(e.getAttribute('src')||'')));
 return t||f}catch{return false}};
const interaction=pageModel?{kind:String(pageModel.kind),total:pageModel.total|0,answered:pageModel.answered|0,done:pageModel.done?1:0,advance:pageModel.hasAdvance?1:0,ownsText:pageModel.ownsText?1:0,detail:scaleDetail||detail}:null;
// No sleeping after fire(next): a postback destroys this execution context while the promise
// is still pending and CDP rejects the whole call. Read the state synchronously instead and
// let Go wait for the document to settle before the next step.
const valErrAfter=readErrs();
const bodyAfter=norm((document.body&&document.body.innerText)||'');
// A page that refuses the answers while the engine sees everything answered means
// the analyzer is blind to something on it. Record the raw form controls so the
// next report shows exactly which field was never filled.
let probe=null;
if(valErrAfter.length&&!completion&&!termination){
  const meta={tag:'META',type:'',id:'',name:'counts',req:false,w:0,h:0,op:'',dis:'',vis:'',mark:'',val:''};
  meta.val='textInputs='+textInputs.length+' visText='+textInputs.filter(vis).length+' questions='+questions.length+' txtQs='+questions.filter(x=>x.owner==='text').length;
  const probeEls=[meta,...document.querySelectorAll('input,textarea,select')].slice(0,61);
  probe=probeEls.map(e=>{
   if(e.tag==='META')return e;
   const r=e.getBoundingClientRect(),s=getComputedStyle(e);
   const fs=e.closest?e.closest('fieldset[disabled]'):null;
   return {tag:e.tagName,type:String(e.type||''),id:String(e.id||'').slice(0,40),name:String(e.name||'').slice(0,40),req:!!(e.required||e.getAttribute('aria-required')==='true'),w:Math.round(r.width),h:Math.round(r.height),op:s.opacity,dis:s.display,vis:s.visibility,mark:String((e.disabled?'D':'')+(e.readOnly?'R':'')+(e.getAttribute('data-ap-selected')||'')),val:String(e.value||'').slice(0,40),
    cls:String((e.getAttribute&&e.getAttribute('class')||'')).slice(0,60),
    aria:String((e.getAttribute&&(e.getAttribute('aria-disabled')||e.getAttribute('data-disabled')||''))||''),
    inFs:fs?'y':'n',
    par:e.parentElement?((e.parentElement.tagName||'')+'.'+String(e.parentElement.getAttribute('class')||'').slice(0,50)):'',
    html:e.outerHTML.slice(0,220)};
  });
}
// Record only an unambiguous single-choice page submitted immediately before
// a termination. The outcome is a clue, not proof that the option caused it.
const submittedPairs=[];
if(/(?:^|\+)next(?:\+|$)/.test(action)){
 const eligible=questions.filter(x=>(x.type==='single'&&!x.grid)||(x.type==='select'&&!x.els[0]?.multiple));
 if(eligible.length===1){
  const x=eligible[0];let chosen=[];
  if(x.type==='select'){const el=x.els[0];if(selectAnswered(el))chosen=[txt(el.options[el.selectedIndex])];}
  else chosen=x.els.filter(isOn).map(label).filter(Boolean);
  if(chosen.length===1)submittedPairs.push({question:x.key,option:qkey(chosen[0])});
 }
}
const signature=JSON.stringify({url:location.href,title:document.title,q:q.slice(0,900),body:bodyAfter.slice(0,1400)});
return {signature,submittedPairs,redHerring:RH_LOG,redHerringScan:RH_DIAG,selected,selectedPairs:pairs,unanswered,exhausted:[...exhausted],returningSession,terminal,answered,dragRequests,url:location.href,title:document.title,question:q.slice(0,1400),questionType:ACCORDIONS.length?'accordion':pageModel?pageModel.kind:(questions.some(x=>x.grid)?'matrix':new Set(questions.map(x=>x.type)).size>1?'mixed':questions[0]?.type||'unknown'),action,interaction,accordion:{count:accordion.count,pending:accordion.pending,selected:accordion.selected,action:accordion.action},instructions:{blocked:INSTRUCTION_BLOCK.count,details:INSTRUCTION_BLOCK.details},needsHuman:humanWall(),validation:valErrAfter,completion,termination,controls:{radios:nativeRadios.length,roleRadios:roleRadios.length,checkboxes:nativeChecks.length,roleCheckboxes:roleChecks.length,selects:selects.length,texts:textInputs.length,clickables:clickable.length},questions:questions.slice(0,40).map(x=>({key:x.key.slice(0,140),type:x.type,n:x.els.length,on:(x.type==='select'||x.type==='text')?(qAnswered(x)?1:0):x.els.filter(isOn).length,grid:!!x.grid})),openQuestions:openQ,probe,inventory,bodyPreview:body.slice(0,1200)};
})()
