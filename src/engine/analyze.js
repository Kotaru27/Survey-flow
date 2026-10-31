// QuestionAnalyzer: the page is read as a list of questions, each with a type and
// a group of controls - never as one bag of controls. adapters.js answers from this
// list and navigate.js refuses to press Next while a question on it is still open,
// so a question can no longer be answered while reported unanswered, or reported
// answered while a row of it was left empty.
// A choice control counts as answered when the page says so (checked input or
// aria-checked) or when the engine itself put it there: plenty of survey UIs are
// plain <label data-value> elements whose state lives in a framework we cannot
// read, and the Next gate has to trust what was actually clicked.
const isOn=e=>{if(!e)return false;
 try{if(e.matches&&e.matches('input')){
  const t=String(e.type||'').toLowerCase();
  if(t==='range')return e.getAttribute('data-ap-selected')==='1';
  if(t==='number')return String(e.value||'').trim()!=='';
  if(t==='radio'||t==='checkbox')return e.checked===true;
  if(e.checked===true)return true;
  return e.getAttribute&&e.getAttribute('data-ap-selected')==='1';
 }}catch{}
 // A framework's exposed state is authoritative. An earlier scripted click can
 // leave a stale local marker if a component rejected the event. That is NOT an
 // answered question and must never unlock the survey's Next button.
 if(e.hasAttribute?.('aria-checked'))return e.getAttribute('aria-checked')==='true';
 if(e.hasAttribute?.('aria-selected'))return e.getAttribute('aria-selected')==='true';
 return !!(e.getAttribute&&e.getAttribute('data-ap-selected')==='1')};
const markSel=e=>{try{e.setAttribute('data-ap-selected','1')}catch{}};
const unmarkSel=e=>{try{e.removeAttribute('data-ap-selected')}catch{}};
// A question the engine deliberately does not answer (an identifier field without a
// value to put in it, a stray data-value chip that is not a question) must not hold
// the Next button hostage.
const skipQ=new Set();
const baseCache=new Map();
const baseKey=e=>{let k=baseCache.get(e);if(k===undefined){k=e.getAttribute('name')||stableQuestion(e);baseCache.set(e,k)}return k};
// One question is often rendered as several rows that all reuse the same name and
// the same question text ("for each of these brands, when do you consume it?").
// Keying only on name/text collapses every row into one group: it is answered once
// and then reported answered while the other rows stay empty, which is exactly how
// the Next button got pressed with unanswered questions on screen.
function splitRows(els){
 if(els.length<4)return null;
 // Categories inside an accordion are sections of one choice question, not matrix rows.
 if(typeof ACCORDIONS!=='undefined'&&ACCORDIONS.some(a=>a.panel&&a.panel.contains(els[0])))return null;
 let level=els.map(e=>e.parentElement);
 for(let d=0;d<8;d++){
  const order=[],idx=new Map();
  for(let i=0;i<els.length;i++){const a=level[i];if(!a)continue;if(!idx.has(a)){idx.set(a,order.length);order.push(a)}}
  if(order.length<2)return null;
  const size=new Array(order.length).fill(0);
  for(let i=0;i<els.length;i++){const a=level[i];if(!a)continue;size[idx.get(a)]++}
  // Two rows with two options each are also a valid matrix; the grouped
  // parent and >=2 controls per row guards keep flat choice lists intact.
  if(size.every(n=>n>=2))return els.map((_,i)=>{const a=level[i];return a?('r'+d+'.'+idx.get(a)):'r'+d+'.x'});
  level=level.map(a=>a?a.parentElement:null);
 }
 return null;
}
function ownerOf(els){
 const hasRole=r=>els.some(e=>/^(radio)$/i.test(String(e.getAttribute&&e.getAttribute('role')||'')));
 const hasType=t=>els.some(e=>{try{return e.matches&&e.matches('input[type="'+t+'"]')}catch{return false}});
 if(hasType('radio')||hasRole('radio'))return 'radio';
 if(hasType('checkbox')||els.some(e=>/^(checkbox|switch)$/i.test(String(e.getAttribute&&e.getAttribute('role')||''))))return 'check';
 return 'custom';
}
function kindOf(els,text){
 const hasType=t=>els.some(e=>{try{return e.matches&&e.matches('input[type="'+t+'"]')}catch{return false}});
 if(hasType('radio')||els.some(e=>/^(radio)$/i.test(String(e.getAttribute&&e.getAttribute('role')||''))) || els.some(e=>e.closest&&e.closest('[role="radiogroup"]')))return 'single';
 if(hasType('checkbox')||els.some(e=>/^(checkbox|switch)$/i.test(String(e.getAttribute&&e.getAttribute('role')||''))))return 'multi';
 if(/check all|tick all|select all|all that apply/i.test(text||''))return 'multi';
 return 'single';
}
// Called with no arguments it reads the whole page; called with a box (and
// includeHidden) it reads one interaction container such as a carousel slide so the
// model can count the items it still has to get through, including the ones that are
// rendered but not on screen yet.
function buildQuestions(box,includeHidden){
 const out=[];
 const keep=e=>includeHidden?true:vis(e);
 let choice=[],selList=[],txtList=[],rangeList=[];
 if(box&&box.querySelectorAll){
  const inBox=[...new Set(box.querySelectorAll('input,select,textarea,[role],[aria-checked],[data-value],[data-option],[data-answer],[data-code]'))].filter(keep);
  const choiceLike=e=>/^(radio|checkbox|option)$/i.test(String(e.getAttribute&&e.getAttribute('role')||''))||(e.hasAttribute&&(e.hasAttribute('aria-checked')||e.hasAttribute('data-value')||e.hasAttribute('data-option')||e.hasAttribute('data-answer')||e.hasAttribute('data-code')))||(e.matches&&(e.matches('input[type="radio"]')||e.matches('input[type="checkbox"]')));
  choice=inBox.filter(choiceLike);
  selList=inBox.filter(e=>e.tagName==='SELECT');
  txtList=inBox.filter(e=>textInputs.indexOf(e)>=0);
  rangeList=inBox.filter(e=>e.matches?.('input[type=range]'));
 }else{
  choice=[...new Set([...nativeRadios,...roleRadios,...nativeChecks,...roleChecks,...customChoices])].filter((e,i,a)=>a.indexOf(e)===i&&vis(e));
  selList=selects.filter(vis);
  txtList=textInputs.filter(vis);
  rangeList=ranges.filter(vis);
 }
 const groups=new Map();
 for(const e of choice){const b=baseKey(e);if(!groups.has(b))groups.set(b,[]);groups.get(b).push(e)}
 for(const [base,els] of groups){
  const sp=splitRows(els);
  const buckets=new Map();
  els.forEach((e,i)=>{const k=sp?sp[i]:'';if(!buckets.has(k))buckets.set(k,[]);buckets.get(k).push(e)});
  for(const [suffix,list] of buckets){
   const text=stableQuestion(list[0]);
   out.push({key:qkey(suffix?base+' #'+suffix:base),type:kindOf(list,text),owner:ownerOf(list),text,els:list,grid:!!sp});
  }
 }
 for(const s of selList){if(s.tagName!=='SELECT')continue;
  const key=qkey('select|'+(s.id||s.name||stableQuestion(s)));
  out.push({key,type:'select',owner:'select',text:stableQuestion(s),els:[s],grid:false});
 }
 for(const e of rangeList){
  if(e.disabled||e.readOnly)continue;
  out.push({key:qkey('range|'+(e.id||e.name||stableQuestion(e))),type:'range',owner:'range',text:stableQuestion(e),els:[e],grid:false});
 }
 for(const e of txtList){if(e.disabled||e.readOnly)continue;
  const key=qkey('text|'+(e.id||e.name||stableQuestion(e)));
  out.push({key,type:'text',owner:'text',text:questionText(e),els:[e],grid:false});
 }
 return out;
}
const questions=buildQuestions();
// Some accordion forms render "None of the above" as a standalone native radio,
// while category items are checkboxes. It is an exclusive fallback for the SAME
// question, not an extra mandatory single-choice question.
for(const q of questions){
 if(q.owner!=='radio'||q.els.length!==1)continue;
 const e=q.els[0],v=norm(label(e));
 if(!/^(none(?: of (?:the|these) (?:above|listed))?|not applicable)$/i.test(v))continue;
 const box=e.closest('.cf-question,.question,.question-container,fieldset,[data-question]');
 if(box&&box.querySelector('input[type="checkbox"],[role="checkbox"]')||ACCORDIONS.some(a=>[...a.content].some(c=>c.matches?.('input[type="checkbox"],[role="checkbox"]'))))q.exclusiveNone=true;
}
const redBoldInstructions=findRedBoldInstructions();
const INSTRUCTION_BLOCK={count:0,details:[]};
if(redBoldInstructions.length&&!questions.length){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push('red-instruction-with-no-visible-question');}
// Browsers may assign a non-empty value to the first <option> even when it is
// a placeholder (e.g. value='placeholder'>Please choose an option). A non-empty
// value alone is not proof the survey received a meaningful answer.
const selectAnswered=s=>{if(!s||s.selectedIndex<0)return false;
 const o=s.options&&s.options[s.selectedIndex];
 if(!o||o.disabled||String(o.value||'').trim()==='')return false;
 const t=norm(o.textContent||'');
 if(/^(?:please\s+)?(?:select|choose|pick)\b/i.test(t)||/^(?:[-–—]+|\.{3,})$/.test(t))return false;
 return !s.validity||s.validity.valid!==false;
};
const qAnswered=q=>{if(q.type==='select')return q.els.some(selectAnswered);
 if(q.type==='text')return q.els.some(e=>(e.isContentEditable?String(e.textContent||'').trim()!=='':String(e.value||'').trim()!=='')&&(!e.validity||e.validity.valid!==false));
 return !!q.els.some(isOn)};
const accordionMore=q=>q.owner==='check'&&!q.noGuess&&ACCORDIONS.length&&!ACC_SINGLE_CHOICE&&q.els.some(e=>ACCORDIONS.some(a=>a.panel?.contains(e)))&&accSelected()<Math.max(1,Math.min(8,Number(P.MultiSelectCount)||1))&&q.els.some(e=>!e.disabled&&vis(e)&&!isOn(e));
const qOpen=q=>!(q.owner==='check'&&ACCORDIONS.length&&ACC_SINGLE_CHOICE&&accSelected()>0)&&!q.exclusiveNone&&!((q.owner==='check')&&questions.some(n=>n.exclusiveNone&&qAnswered(n)))&&!skipQ.has(q.key)&&(!qAnswered(q)||accordionMore(q))&&q.els.some(e=>!e.disabled&&vis(e));
// A lone choice control only earns an answer (instead of being treated as page
// furniture and skipped) when it sits under something that reads like a question.
const looksLikeQuestion=q=>!!q.grid||/\?|\brequired\b|\bselect\b|\bchoose\b|\btick\b|\bcheck\b|\bagree\b|\brate\b|\bplease\b/i.test(q.text||'')||q.els.some(e=>/^(radio|checkbox|option)$/i.test(String(e.getAttribute&&e.getAttribute('role')||'')));
// Read *local*, visible instructional emphasis, not a page-wide directive.
// Only leaf text nodes are considered to prevent duplicating inherited red styles.
function redColor(value){
 const v=String(value||'').toLowerCase().trim();
 let channels=null;
 const rgb=v.match(/^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)/);
 if(rgb)channels=rgb.slice(1,4).map(Number);
 if(/^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i.test(v)){
  const h=v.slice(1);
  channels=h.length===3?[0,1,2].map(i=>parseInt(h[i]+h[i],16)):[0,2,4].map(i=>parseInt(h.slice(i,i+2),16));
 }
 if(v==='red')channels=[255,0,0];
 return !!channels&&channels[0]>=120&&channels[0]>channels[1]*1.5&&channels[0]>channels[2]*1.5;
}
// Survey forms often render ordinary selection guidance in bold red (especially
// required-field messages). Only directives that identify a *particular* option
// belong in the attention-instruction path. A format prompt is not an answer key.
function generalSelectionInstruction(raw){
 const t=norm(raw||'').toLowerCase().replace(/[.!?:;]+$/,'').replace(/\s+/g,' ').trim();
 if(!t)return false;
 // Never dilute a target-specific command, even if it also mentions "one answer".
 if(/\b(?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|last|\d+(?:st|nd|rd|th))\s+(?:option|answer|response|choice|item)\b|\b(?:option|answer|response|choice|item)\s*(?:number|no\.?|#)?\s*\d+\b/i.test(t))return false;
 if(/\b(?:ignore|regardless of|attention check|quality check|to show you are reading|please prove|for testing purposes|do not|don't|never|avoid|except|instead of|always select|only select)\b/i.test(t))return false;
 // Conventional prompts or required-field messages describe *how many/how* to
 // answer, not *which* particular response to select.
 const selection=/\b(?:select|choose|pick|tick|mark|check|indicate|answer|respond|click|tap|press)\b/i.test(t);
 if(!selection)return false;
 if(/\b(?:select|choose|pick|tick|mark|check|indicate|provide)\s+(?:only\s+|exactly\s+|just\s+)?(?:one|single|multiple|one or more|more than one|all|any|at least one|up to \d+|between \d+ and \d+)\s*(?:(?:of the|of these|from the|of)\s+)?(?:following\s+)?(?:options?|answers?|responses?|choices?|items?)\b/i.test(t))return true;
 if(/\b(?:select|choose|pick|tick|mark|check)\s+(?:one|a single|an?|all|any|those|each|as many as apply)\s*(?:that apply|which apply|as apply|of the following|from the list|from below|per row|for each row|for each statement)?\s*$/i.test(t))return true;
 if(/\b(?:select|choose|pick|tick|mark|check)\s+(?:only\s+|exactly\s+)?(?:one|a single|all|any|at least one|one or more|up to \d+)\b/i.test(t)&&/\b(?:that apply|which apply|for each|per row|in each row|from the following|from the list|from the options|below|answer|option|choice|response|item)\b/i.test(t))return true;
 if(/\b(?:select|choose|pick|tick|mark|check|indicate)\s+(?:(?:the|an?|whichever)\s+)?(?:option|answer|response|choice|statement)\s+(?:(?:below|from below)\s+)?(?:that|which)\s+(?:best|most|closely|accurately|applies|reflects|describes|matches|represents|corresponds)\b/i.test(t)&&/\b(?:you|your|yourself|opinion|experience|preference|situation|identity)\b/i.test(t))return true;
 if(/\b(?:select|choose|pick|tick|mark|check|indicate)\s+(?:your|how you|whether you|which of the following|all that apply|any that apply)\b/i.test(t))return true;
 if(/\b(?:please\s+)?(?:select|choose|provide|indicate)\s+(?:a|an|one)\s+(?:valid\s+)?(?:answer|response|choice|option)\s*(?:to continue|before continuing|to proceed|below|from the list)?$/i.test(t))return true;
 return false;
}
function findRedBoldInstructions(){
 const out=[];const unique=new Set();
 // Text may be split across <strong>, <span> and nested font/color wrappers. Read
 // the smallest visible phrase around a red fragment; never pull answer labels
 // from a surrounding question/form into the instruction.
 const nodes=[...document.querySelectorAll('strong,b,span,p,label,div,em,font,h1,h2,h3,h4,h5,h6,[role="note"]')];
 for(const e of nodes){
  if(!vis(e))continue;
  let css;try{css=getComputedStyle(e)}catch{continue}
  if(!redColor(css.color))continue;
  // Red instructions need not be bold. Exclude explicit validation/error regions.
  if(e.closest('[role=\"alert\"],[aria-live=\"assertive\"],.error,.errors,.validation-error,[class*=\"validation-error\"]'))continue;
  if(e.querySelector('input,select,textarea,[role="radio"],[role="checkbox"]'))continue;
  let phrase=e;
  let t=norm(e.textContent||'');
  // "Please <strong style=color:red>select</strong> the second answer".
  if(!/\b(select|choose|pick|click|tick|mark|check|answer|respond|indicate|tap|press|must|ignore)\b/i.test(t)||t.length<10){
   let p=e.parentElement;
   for(let depth=0;p&&depth<3;depth++,p=p.parentElement){
    if(/^(FORM|BODY|HTML)$/.test(p.tagName||''))break;
    if(p.querySelector('input,select,textarea,[role="radio"],[role="checkbox"]'))break;
    const parentText=norm(p.textContent||'');
    if(parentText.length>400)break;
    if(/\b(select|choose|pick|click|tick|mark|check|answer|respond|indicate|tap|press|must|ignore)\b/i.test(parentText)){
     phrase=p;t=parentText;break;
    }
   }
  }
  if(t.length<10||t.length>400||!/\b(select|choose|pick|click|tick|mark|check|answer|respond|indicate|tap|press|must|ignore)\b/i.test(t))continue;
  const key=t+'|'+(phrase.closest('.cf-question,.question,.question-container,fieldset,[role="group"],[data-question]')?.id||'');
  if(unique.has(key))continue;
  unique.add(key);out.push({text:t,el:phrase,redEl:e});
 }
 // A shorter matching fragment is preferred to a surrounding paragraph.
 out.sort((a,b)=>a.text.length-b.text.length);
 return out;
}
// A red instruction is a directive, not a preference. Only a uniquely owned
// question may act on it; otherwise stop for review rather than guessing.
function redInstructionFor(q){
 const e=q.els[0];if(!e)return null;
 const owner=e.closest?.('.cf-question,.question,.question-container,fieldset,[data-question]');
 const local=redBoldInstructions.filter(i=>owner&&owner.contains(i.el));
 if(local.length)return local[0];
 // Some frameworks put the question's heading immediately *outside* its fieldset.
 // Accept it only when the ancestor does not contain another question's control.
 let node=owner||e.parentElement;
 for(let depth=0;node&&depth<3;depth++,node=node.parentElement){
  if(/^(FORM|BODY|HTML)$/.test(node.tagName||''))break;
  const matches=redBoldInstructions.filter(i=>node.contains(i.el));
  if(matches.length===1){
   const others=questions.some(other=>other!==q&&other.els.some(el=>node.contains(el)));
   if(!others)return matches[0];
  }
 }
 if(questions.length===1&&redBoldInstructions.length===1)return redBoldInstructions[0];
 return null;
}
// Conservative cross-question instruction parsing. These helpers return null when
// a range or selection count cannot be read with confidence.
function parseAgeRange(raw){
 const t=norm(raw).toLowerCase();
 if(/\b(?:not|outside|exclude|excluding|except|avoid)\b/.test(t))return null;
 const patterns=[/\b(?:age|aged)\s*(?:between\s*|of\s*|from\s*)?(\d{1,3})\s*(?:-|–|—|to|and)\s*(\d{1,3})\b/,
 /\b(?:between\s+)?(\d{1,3})\s*(?:-|–|—|to|and)\s*(\d{1,3})\s*(?:years?\s+old|years?\s+of\s+age)\b/];
 for(const re of patterns){const m=t.match(re);if(m){const lo=Number(m[1]),hi=Number(m[2]);return lo<=hi&&hi<=130?{min:lo,max:hi}:null}}
 return null;
}
function ageQuestion(q){return /\bage\b|\bhow old\b|\byears? old\b/i.test((q.text||'')+' '+attrBlob(q.els?.[0]));}
function relatedAgeRule(q){
 if(!ageQuestion(q))return null;
 const matches=redBoldInstructions.filter(i=>/\bage\b|\baged\b|\byears? old\b/i.test(i.text||''));
 if(matches.length!==1)return matches.length?{error:'ambiguous-age-instruction'}:null;
 const ageQs=questions.filter(x=>(x.owner==='select'||x.owner==='text')&&ageQuestion(x));
 if(ageQs.length!==1||ageQs[0]!==q)return {error:'ambiguous-age-control'};
 const range=parseAgeRange(matches[0].text);
 return range?{range}: {error:'unsupported-age-instruction'};
}
function selectionCount(raw){
 const t=norm(raw).toLowerCase().replace(/\batleast\b/g,'at least');
 if(!/\b(select|choose|pick|tick|check|mark)\b/.test(t))return null;
 const words={one:1,two:2,three:3,four:4,five:5,six:6,seven:7,eight:8,nine:9,ten:10};
 const number=x=>/^\d+$/.test(x)?Number(x):words[x];
 let min=0,max=Infinity,found=false;
 const re=/\b(at least|more than|over|at most|no more than|up to|exactly)\s+(\d{1,2}|one|two|three|four|five|six|seven|eight|nine|ten)\b/g;
 for(const m of t.matchAll(re)){
  const n=number(m[2]);if(!Number.isFinite(n)||n>50)return null;found=true;
  if(m[1]==='more than'||m[1]==='over')min=Math.max(min,n+1);
  else if(m[1]==='at least')min=Math.max(min,n);
  else if(m[1]==='exactly'){min=Math.max(min,n);max=Math.min(max,n)}
  else max=Math.min(max,n);
 }
 const b=t.match(/\bbetween\s+(\d{1,2}|one|two|three|four|five|six|seven|eight|nine|ten)\s+and\s+(\d{1,2}|one|two|three|four|five|six|seven|eight|nine|ten)\s+(?:options?|answers?|responses?|choices?|items?)\b/);
 if(b){found=true;min=Math.max(min,number(b[1]));max=Math.min(max,number(b[2]))}
 return found?{min,max}:null;
}

function instructionWithAgeRemoved(text){
 // This only separates a recognized age constraint, never arbitrary trailing text.
 return norm(text).replace(/\s+\band\s+(?:the\s+)?(?:age|aged)\s*(?:between\s*|of\s*|from\s*)?\d{1,3}\s*(?:-|–|—|to|and)\s*\d{1,3}\b\s*[.!]?$/i,'').replace(/\s+only\s*[.!]?$/i,'');
}
const INSTR_ORD={first:0,second:1,third:2,fourth:3,fifth:4,sixth:5,seventh:6,eighth:7,ninth:8,tenth:9};
function instructionTargets(q,instruction){
 const text=norm(instruction.text||'');
 let options=q.els.filter(e=>!e.disabled&&vis(e));
 // When a product section is collapsed, using only *visible* options changes
 // the meaning of "third" or "last". Preserve the DOM order of hidden choices.
 if(ACCORDIONS.length&&q.els.length){
  const first=q.els[0];const name=first.getAttribute('name');
  const roots=ACCORDIONS.map(a=>a.panel).filter(Boolean);
  const scope=first.closest('.cf-question,.question,.question-container,fieldset,[data-question]');
  const full=[...document.querySelectorAll(ACC_OPTION)].filter(e=>!e.disabled&&(!scope||scope.contains(e))&&(
    (name&&e.getAttribute('name')===name)||(!name&&e.getAttribute('role')===first.getAttribute('role')&&roots.some(p=>p.contains(e)))));
  if(full.length>options.length)options=full;
 }
 if(!options.length)return {targets:[],reason:'no-visible-options'};
 const labels=options.map(e=>norm(label(e)||e.getAttribute?.('aria-label')||''));
 const lower=text.toLowerCase();
 // Red/bold text can be the ordinary response format ("Please select one answer")
 // rather than an attention instruction naming a particular answer. Earlier we
 // classified this as unmatched, blocked every adapter, then the Go runner
 // terminated the entire batch after three identical holds.
 // Only generic cardinality/format wording can take the regular answer path;
 // specific ordinals, exact labels, negations and unknown directives still block.
 if(generalSelectionInstruction(text))return {targets:[],reason:'general-instruction'};
 // A negative instruction alone is not permission to choose some other answer.
 if(/\b(?:do not|don't|never|avoid)\s+(?:select|choose|pick|click|tick|mark|check)\b/i.test(text))
  return {targets:[],reason:'negative-instruction-needs-positive-target'};
 // In 'select the second option, not the first', the prohibited ordinal
 // is not a second target. Preserve the positive command and exclude the
 // explicitly negated position instead of blocking a valid single-select.
 const ordinal=[...lower.matchAll(/\b(first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|last)\b/g)]
  .filter(m=>!/(?:\bnot|\bexcept|\bexcluding|\bavoid)\s+(?:the\s+)?$/i.test(lower.slice(Math.max(0,m.index-30),m.index)))
  .map(m=>m[1]);
 // Numeric targets need an explicit option/answer noun: "select option 3" is
 // positional; "select 3 options" is cardinality, NOT a positional instruction.
 const numeric=[];
 for(const m of lower.matchAll(/\b(?:options?|answers?|responses?|choices?|items?)\s*(?:number|no\.?|#)?\s*(\d{1,2}(?:\s*(?:,|\bor\b|\band\b)\s*\d{1,2})*)\b/g)){
  for(const n of m[1].match(/\d{1,2}/g)||[])numeric.push(Number(n)-1);
 }
 // In a single-choice question, "select 2 or 3 options" identifies
 // alternative positions. For multi-select it can mean a selection count,
 // so leave that interpretation to the count handling instead.
 if(q.type==='single'){
  for(const m of lower.matchAll(/^\s*(?:please\s+)?(?:select|choose|pick|click|tick|mark|check)\s+(?:the\s+)?(\d{1,2}(?:\s*(?:,|\bor\b)\s*\d{1,2})+)\s*(?:options?|answers?|responses?|choices?|items?)\s*[.!]?\s*$/g)){
   if(/\bor\b/.test(m[1]))for(const n of m[1].match(/\d{1,2}/g)||[])numeric.push(Number(n)-1);
  }
 }
 for(const m of lower.matchAll(/\b(\d{1,2})(?:st|nd|rd|th)\s+(?:option|answer|response|choice|item)\b/g))numeric.push(Number(m[1])-1);
 const ordTargets=ordinal.map(o=>o==='last'?options[options.length-1]:options[INSTR_ORD[o]])
  .concat(numeric.map(i=>options[i]));
 if(ordinal.length||numeric.length){
  if(ordTargets.some(e=>!e))return {targets:[],reason:'ordinal-option-not-found'};
  if(ordTargets.length>1&&q.type==='single'){
   if(/\bor\b/i.test(text)&&!/\band\b/i.test(text))return {targets:[ordTargets[0]],alternatives:[...new Set(ordTargets)],reason:'ordinal-alternative'};
   return {targets:[],reason:'multiple-targets-in-single-select'};
  }
  return {targets:[...new Set(ordTargets)],reason:'ordinal'};
 }
 const none=/\b(?:none of (?:the |these )?(?:above|listed)|none)\b/i.test(text);
 if(none){const target=options.find((_,i)=>/\bnone\b/i.test(labels[i]));if(target)return {targets:[target],reason:'none'};}
 // Explicit option words, including quoted answers, take precedence. Avoid
 // substring matching (e.g. Male should never select Female).
 const command=lower.match(/\b(?:select|choose|pick|click|tick|mark|check)\b[\s:,-]*(?:only\s+|the\s+)?([\s\S]+)$/);
 if(!command)return {targets:[],reason:'no-command'};
 const tail=command[1].replace(/\s+(?:option|answer|choice|item)s?\s*\.?$/,'').trim();
 const normLabel=v=>norm(v).toLowerCase().replace(/[.!?]+$/,'');
 const chunks=tail.split(/\s*(?:,|\band\b|\bor\b)\s*/i).map(x=>normLabel(x.replace(/^[\s'"“”‘’]+|[\s'"“”‘’]+$/g,''))).filter(Boolean);
 // Pure OR means any explicitly and uniquely matched alternative is valid.
 // Prefer an exact label; permit only whitespace differences as a fallback.
 // Do not infer conditional or negative clauses from approximate text.
 if(q.type==='single'&&/\bor\b/i.test(tail)&&!/\b(?:and|not|unless|except|excluding|without|if|when)\b/i.test(tail)&&chunks.length>1&&chunks.every(c=>/^[\p{L}\p{N}\s'’.\-]+$/u.test(c))){
  for(const loose of [false,true]){
   const matches=[];
   for(const chunk of chunks){
    const normalized=loose?chunk.replace(/\s+/g,''):chunk;
    const hits=options.filter((_,i)=>{let l=normLabel(labels[i]);if(loose)l=l.replace(/\s+/g,'');return l===normalized});
    if(hits.length===1)matches.push(hits[0]);
   }
   if(matches.length)return {targets:[matches[0]],reason:loose?'label-alternative-spacing':'label-alternative'};
  }
 }
 const matches=[];
 for(const chunk of chunks){
  // Only accept a unique exact label or its simple English plural. Whole
  // strings are compared so Male never matches Female.
  const hits=options.filter((_,i)=>{const l=normLabel(labels[i]);return l===chunk||(l.length>2&&chunk===l+'s')});
  if(hits.length===1)matches.push(hits[0]);
 }
 if(matches.length===chunks.length&&chunks.length>0){
  if(matches.length>1&&q.type==='single'){
   if(/\bor\b/i.test(tail)&&!/\band\b/i.test(tail))return {targets:[matches[0]],reason:'label-alternative'};
   return {targets:[],reason:'multiple-targets-in-single-select'};
  }
  return {targets:[...new Set(matches)],reason:'explicit-label'};
 }
 // "Please choose Powder detergent." (the whole target follows the verb).
 const whole=normLabel(tail.replace(/^[\s'"“”‘’]+|[\s'"“”‘’]+$/g,''));
 const direct=options.find((_,i)=>normLabel(labels[i])===whole);
 if(direct)return {targets:[direct],reason:'exact-label'};
 return {targets:[],reason:'instruction-not-matched'};
}

// Recover only when a displayed validation identifies one rejected selection.
// Unknown, conflicting or exhausted cases stay on the page for review.
function recoverValidatedChoice(){
 if(!valErr.length)return {changed:false,reason:''};
 // Some validations state which answer is allowed rather than naming the
 // rejected one. Only act on an explicit positive command in a local error
 // belonging to exactly one single-choice group.
 const errorSelector='[role="alert"],[aria-live="assertive"],.error,.errors,.validation-error,[class*="error" i],[class*="validation" i]';
 const errorNodes=[...new Set([...document.querySelectorAll(errorSelector)].filter(vis).flatMap(e=>{
  const items=[...e.querySelectorAll('li')].filter(vis);
  return items.length?items:[e];
 }))];
 const commands=errorNodes.map(e=>({el:e,text:norm(e.textContent||'')})).filter(x=>/^(?:please\s+)?(?:select|choose|pick|tick|mark)\s+.{1,120}[.!]?$/i.test(x.text));
 const matches=[];
 for(const command of commands){
  const owner=command.el.closest('.cf-question,.question,.question-container,fieldset,[data-question]');
  if(!owner)continue;
  const local=questions.filter(q=>q.owner==='radio'&&q.els.some(e=>owner.contains(e)));
  if(local.length!==1)continue;
  const q=local[0],inst=redInstructionFor(q);
  if(inst&&!generalSelectionInstruction(inst.text))continue;
  const parsed=instructionTargets(q,{text:command.text});
  if(!parsed?.targets?.length||!['explicit-label','exact-label','label-alternative','label-alternative-spacing','ordinal','ordinal-alternative'].includes(parsed.reason))continue;
  const target=parsed.targets[0];
  if(!target||!vis(target)||isOn(target)||isAvoid(q.key,label(target)))continue;
  matches.push({q,target});
 }
 if(matches.length===1){
  const {q,target}=matches[0];
  if(!fireChoice(target)||!isOn(target))return {changed:false,reason:'validation-change-unverified'};
  const name=norm(label(target));selected.push(name);pairs.push({question:q.key,option:qkey(name)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(name));
  q.noGuess=true;return {changed:true,reason:'validation-answer-changed'};
 }
 const message=norm(valErr.join(' ')).toLowerCase();
 const selectedNow=[];
 for(const q of questions){
  if(q.owner==='radio'||q.owner==='check'){
   for(const e of q.els.filter(isOn))selectedNow.push({q,el:e,name:norm(label(e))});
  }else if(q.owner==='select'){
   const e=q.els[0];if(e&&selectAnswered(e))selectedNow.push({q,el:e,name:norm(txt(e.options[e.selectedIndex]))});
  }
 }
 const named=selectedNow.filter(x=>x.name.length>=3&&message.includes(x.name.toLowerCase()));
 let hit=named.length===1?named[0]:null;
 if(!hit&&named.length===0&&selectedNow.length===1&&!needMore&&/\b(?:invalid|not valid|not allowed|not accepted|unavailable|cannot|another answer|another option|another choice)\b/i.test(message))hit=selectedNow[0];
 if(!hit)return {changed:false,reason:named.length>1?'ambiguous-validation':'unmatched-validation'};
 const q=hit.q,inst=redInstructionFor(q);
 if(inst&&!generalSelectionInstruction(inst.text))return {changed:false,reason:'validation-conflicts-with-instruction'};
 const allLabels=q.owner==='select'?[...hit.el.options].map(txt):q.els.map(label);
 const id=location.pathname+'|'+qkey(q.key)+'|'+allLabels.map(qkey).join('|');
 const store=window.__apRejectedValidation||(window.__apRejectedValidation={});
 const rejected=store[id]||(store[id]=[]);
 if(!rejected.includes(qkey(hit.name)))rejected.push(qkey(hit.name));
 let target=null,ok=false;
 if(q.owner==='radio'||q.owner==='check'){
  const available=q.els.filter(e=>vis(e)&&!e.disabled&&!isOn(e)&&!rejected.includes(qkey(label(e)))&&!isAvoid(q.key,label(e)));
  if(!available.length)return {changed:false,reason:'validation-no-alternative'};
  target=available[0];
  if(q.owner==='check'){
   fire(hit.el);unmarkSel(hit.el);
   if(isOn(hit.el))return {changed:false,reason:'validation-uncheck-failed'};
  }
  ok=fireChoice(target)&&isOn(target)&&!isOn(hit.el);
  if(ok){const name=norm(label(target));selected.push(name);pairs.push({question:q.key,option:qkey(name)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(name));}
 }else if(q.owner==='select'){
  const e=hit.el,old=String(e.value),rule=relatedAgeRule(q);
  if(rule?.error)return {changed:false,reason:rule.error};
  const possible=[...e.options].filter(o=>!o.disabled&&String(o.value||'')!==''&&!/^(?:please\s+)?(?:select|choose|pick)\b/i.test(norm(txt(o)))&&!rejected.includes(qkey(txt(o)))&&!isAvoid(q.key,txt(o))&&(!rule?.range||(/^\d{1,3}$/.test(norm(txt(o)))&&Number(txt(o))>=rule.range.min&&Number(txt(o))<=rule.range.max)));
  if(!possible.length)return {changed:false,reason:'validation-no-alternative'};
  target=possible[0];e.value=target.value;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));
  ok=String(e.value)!==old&&String(e.value)===String(target.value)&&selectAnswered(e);
  if(ok){const name=norm(txt(target));selected.push(name);pairs.push({question:q.key,option:qkey(name)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(name));}
 }
 if(!ok)return {changed:false,reason:'validation-change-unverified'};
 q.noGuess=true;return {changed:true,reason:'validation-answer-changed'};
}
