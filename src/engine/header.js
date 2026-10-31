(async()=>{
const RAW=PERSONA;
// A widget that ignores scripted events asks for a real mouse drag here, and the Go
// side performs it between steps. Declared up front so the adapters can fill it.
const dragRequests=[];
const pick=(pascal,camel)=>RAW[pascal]!==undefined&&RAW[pascal]!==null&&RAW[pascal]!==''?RAW[pascal]:RAW[camel];
const P=Object.assign({},RAW,{Name:pick('name','name'),Gender:pick('Gender','gender'),Age:pick('Age','age'),Country:pick('Country','country'),City:pick('City','city'),YesNo:pick('YesNo','yesNo'),Rating:pick('Rating','rating'),MultiSelectCount:pick('MultiSelectCount','multiSelectCount'),OpenText:pick('OpenText','openText'),Number:pick('Number','number'),AnswerMode:pick('AnswerMode','answerMode'),ReentryID:String(pick('ReentryID','reentryID')||'')});
const reentryPattern=/(reentry.?id|reentry|stores the id used for reentry|id used for reentry|test.?id|unique.?id|respondent.?id|panel(ist)?.?id|participant.?id)/i;
const norm=s=>String(s==null?'':s).replace(/\s+/g,' ').trim();
// Confirmit-style pages paint the shell first and render the question afterwards, so a
// step taken too early sees no controls at all (and then clicks Next on nothing).
// Wait until the document is complete and either a control is on screen or there is no
// hidden required control left to render.
async function renderedNow(){
 if(document.readyState!=='complete'){window.__apRenderPending=true;return false}
 try{
  const all=[...document.querySelectorAll('input,select,textarea,[role="radio"],[role="checkbox"],[role="option"]')];
  const shown=e=>{if(e.disabled)return false;const hasBox=x=>{const r=x.getBoundingClientRect();const s=getComputedStyle(x);return r.width>0&&r.height>0&&s.display!=='none'&&s.visibility!=='hidden'&&Number(s.opacity)!==0};return hasBox(e)||(e.matches?.('input[type=radio],input[type=checkbox]')&&[...(e.labels||[])].some(hasBox))};
  if(all.some(shown)){window.__apRenderPending=false;return true}
  // Boards, sliders and disclosure triggers can legitimately have no native controls.
  const widget=[...document.querySelectorAll('[role="slider"],[draggable="true"],[class*=slider i],[class*=card i],[class*=drag i],[class*=handle i],[class*=answer-container i],[class*=sliders-container i],summary,[aria-expanded="false"],.accordion-header')];
  if(widget.some(shown)){window.__apRenderPending=false;return true}
  const title=[...document.querySelectorAll('.question-title,.cf-question__title,[class*="question-title" i],fieldset legend,.question h2,.question h3')]
   .filter(shown).map(e=>norm(e.textContent||'')).find(Boolean)||'';
  // A page with a visible question but no answer controls is likely still mounting.
  // It must never submit Next as an apparently complete, empty page.
  // A generic title (e.g. 'Question 8') may sit above the actual prompt.
  // Inspect the question container too; otherwise an asynchronously mounted
  // control looks like an empty, completed page and Next is clicked too early.
  const bodyPrompt=[...document.querySelectorAll('.cf-question,.question,.question-container,[data-question],fieldset')]
   .filter(shown).map(e=>norm(e.textContent||'')).find(Boolean)||'';
  const prompt=(title+' '+bodyPrompt.slice(0,700)).trim();
  // A statement containing words such as 'would' or 'questions' is not by
  // itself evidence of an input. Keep waiting for direct questions and clear
  // answer instructions, including when their controls mount asynchronously.
  const direct=/\?|(?:^|[.!:]\s+)\s*(?:which|what|how|where|when|who|whose|why|have|has|do|does|did|are|is|would|could)\b/i;
  const directive=/\b(?:select|choose|pick|rate|rank|enter|indicate|tick|check)\b/i;
  const questionExpected=prompt.length>0&&(direct.test(prompt)||directive.test(prompt));
  window.__apRenderPending=questionExpected;
  if(questionExpected)return false;
  return !document.querySelector('[aria-required="true"],[required]');
 }catch{return true}
}
window.__apRenderPending=false;
for(let i=0;i<10;i++){
 if(await renderedNow())break;
 await new Promise(r=>setTimeout(r,90));
}
const MEM=MEMORY;
// The attention checks a study ends an interview over. The key lives in
// config/red-herring.json: an answer given here is answered the way the check expects, and
// an option the config marks as a trap is never taken, whatever the rest of the engine
// would have chosen. These answers are final - the memory does not reopen them and a
// later rejection does not rule them out - because a trap answer is the one mistake that
// cannot be learned back out of.
// Every rule the key holds. A rule that only lists traps names no question of its own -
// it says which option codes end the interview - so it is kept even though it has no
// text to match on. Dropping it here is what made "never Angry/Happy/Sad" unreachable:
// rhTrapOnly() reads this list, and a list the trap rules were filtered out of is always
// empty, so the guard never ran and the trap answer went in like any other.
const RH_ALL=(Array.isArray(MEM&&MEM.redHerring)?MEM.redHerring:[]).filter(r=>r&&typeof r==='object');
// The rules that name a question: matched by id, by the start of an id, or by a pattern
// over its text. A rule with none of those can still be a trap list (see above).
const RH_RULES=RH_ALL.filter(r=>r.id||r.idStarts||r.text||r.contains);
// A question that reads like an attention check is answered from the key or not at all.
// Guessing here is the one answer that cannot be learned back out of, so a check the file
// has no rule for is written down in full - its text and every option - and left
// unanswered until it has been reported, rather than answered by chance.
//
// Every phrase here is one that cannot occur in an ordinary question. That is the whole
// constraint: this list decides what gets REPORTED and what is held back from the random
// filler, so a phrase that also occurs in normal wording ("what colour is", "select one
// answer", "this year", "carefully") costs a real question its answer. Where a study needs
// one of those, the key says so itself with `when` on a trap list - see rhTrapWhen.
// Only unequivocal instruction/verification prompts are checks. Broad words such
// as "what year", "most appropriate", "click on" and "in the picture" are ordinary
// survey wording too; treating them as checks would leave legitimate questions blank.
const ATTENTION_RE=/\battention check\b|\bquality check\b|\b(?:select|choose|pick|click|mark|tick)\s+(?:the\s+)?(?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|last)\s+(?:answer|option|choice|item)\b|\bselect the correct answer\b|\b2\s*\+\s*2\b|\bodd one out\b|\bwhich one is different\b/i;
const looksAttention=t=>ATTENTION_RE.test(norm(t||''));
// The nearest rule by shared words: a check the file words differently still gets the
// answer the file already knows, instead of a coin toss. Two shared significant words are
// required - one is how "In which year are we now?" ends up answered with next year's
// answer because both sentences contain "year", which is the mistake this exists to stop.
const nearestRule=t=>{const words=norm(t||'').toLowerCase().split(/[^a-z0-9]+/).filter(w=>w.length>3&&!STOP.has(w));let best=null,score=0;for(const r of RH_RULES){const hay=norm([r.text,r.contains,r.note,r.answer,(r.answers||[]).join(' ')].join(' ')).toLowerCase();let sc=0;for(const w of words){if(hay.indexOf(w)>=0)sc++}if(sc>score){score=sc;best=r}}return score>=2?best:null};
const STOP=new Set(['which','what','answer','select','click','question','please','following','these','those','there','their','about','would','could','should','please']);
const rhQuestionIds=q=>{
 const out=[];
 const box=q&&q.els&&q.els[0]&&q.els[0].closest?q.els[0].closest('.cf-question,.question,.question-container,fieldset,[role="group"],[data-question],[id^=cq]'):null;
 const push=id=>{if(id&&out.indexOf(id)<0)out.push(id)};
 if(box&&box.id)push(box.id);
 for(const e of (q&&q.els)||[])push(e.id);
 for(const e of (q&&q.els)||[]){
  const lab=e.getAttribute&&(e.getAttribute('aria-labelledby')||'');
  for(const t of lab.split(/\s+/)){const el=document.getElementById(t);if(el)push(el.id)}
 }
 for(const e of (q&&q.els)||[]){
  let n=e.parentElement,d=0;
  while(n&&d<4){if(/^cq\d+$/i.test(n.id||''))push(n.id);n=n.parentElement;d++}
 }
 return out;
};
// A rule matches by exact id or by the start of an id - a study that numbers its checks
// cq11111121 instead of cq11111105 still matches. Ids only: they belong to one question
// and cannot bleed into a neighbour the way page text does.
const rhRuleById=q=>{
  const ids=rhQuestionIds(q).map(id=>String(id).toLowerCase());
  if(!ids.length)return null;
  for(const r of RH_RULES){
   if(r.id&&ids.some(id=>id===String(r.id).toLowerCase()))return r;
  }
  for(const r of RH_RULES){
   if(r.idStarts&&ids.some(id=>id.indexOf(String(r.idStarts).toLowerCase())===0))return r;
  }
  return null;
};
// The text a rule is matched against is the question's OWN wording, read from its own
// container upwards. The analyzer's q.text is whatever stableQuestion() made of the
// page, and on a page holding two checks that can be the same sentence for both - which
// is how "What year was last year?" gets answered with this year's answer.
const rhRuleText=q=>rhLocalText(q);
const rhImageMatch=(rule,q)=>{
 if(!rule.imageSrc)return true;
 const e=q?.els?.[0],box=e?.closest?.('.cf-question,.question,.question-container,fieldset,[data-question]');
 if(!box)return false;
 return [...box.querySelectorAll('img')].some(i=>String(i.currentSrc||i.src||'').split('?')[0].toLowerCase()===String(rule.imageSrc).toLowerCase());
};
const rhTextRuleFor=(q,text)=>{
  const t=norm(text||'').toLowerCase();
  if(!t)return null;
  for(const r of RH_RULES){
   if(!rhImageMatch(r,q))continue;
   if(r.text){
    try{if(new RegExp(String(r.text),'i').test(t))return r}catch{}
   }
  }
  for(const r of RH_RULES){
   if(!rhImageMatch(r,q))continue;
   if(r.contains&&t.indexOf(String(r.contains).toLowerCase())>=0)return r;
  }
  return null;
};
// The whole rule lookup for one question: its id first, then its own words. `text` is
// passed in so the caller reads it once and every decision on this question - lenient
// options, nearest rule, ambiguous check - is made against the same sentence.
const rhRuleFor=(q,text)=>{
  const byId=rhRuleById(q);
  if(byId)return byId;
  return rhTextRuleFor(q,text===undefined?rhRuleText(q):text);
};
// The options of a question, each with every name a reader could call it by: the option
// text, its aria-label, the value and the number its id ends in, the alt text of a picture
// inside it, and - for a colour swatch that has no text at all - the colour it is painted.
const COLOURS={purple:'#800080',violet:'#ee82ee',blue:'#0000ff',red:'#ff0000',green:'#008000',orange:'#ffa500',yellow:'#ffff00',black:'#000000',white:'#ffffff',grey:'#808080',gray:'#808080',pink:'#ffc0cb',brown:'#a52a2a'};
 const rhIdentity=o=>{
  if(!o)return '';
  const e=o.el;
  const parts=[];
  const add=v=>{const t=norm(String(v==null?'':v)).trim();if(t)parts.push(t)};
  try{add(label(e))}catch{}
  try{add(e.getAttribute&&e.getAttribute('aria-label'))}catch{}
  try{add(e.getAttribute&&e.getAttribute('title'))}catch{}
  try{add(e.getAttribute&&e.getAttribute('data-label'))}catch{}
  try{add(e.getAttribute&&e.getAttribute('data-value'))}catch{}
  try{add(e.getAttribute&&e.getAttribute('data-name'))}catch{}
  // A logo or a picture is named by its file as often as by its alt text: "audi.png" is
  // the answer to "click on Audi" when the image has no alt at all.
  try{for(const im of e.querySelectorAll?e.querySelectorAll('img,source,[style*=background]'):[]){
   add(im.getAttribute&&im.getAttribute('alt'));
   add(im.getAttribute&&im.getAttribute('title'));
   for(const a of ['src','srcset','data-src','data-image','data-original']){
    const v=im.getAttribute&&im.getAttribute(a);
    if(v){add(v);for(const part of String(v).split(/[, ]+/)){const f=part.split('/').pop();if(f)add(f.split('?')[0].replace(/\.[a-z0-9]{2,4}$/i,''))}}
   }
   const st=im.getAttribute&&im.getAttribute('style');
   if(st){add(st);const m=String(st).match(/url\(([^)]+)\)/);if(m){const f=m[1].split('/').pop();if(f)add(f.split('?')[0].replace(/\.[a-z0-9]{2,4}$/i,''))}}
  }}catch{}
  // "Please select the purple colour" is answered by an option with no words in it at
  // all: the colour is painted on something inside the option, and on a live page it is
  // painted by a stylesheet class, so the colour is in no attribute and not on the input.
  // Every colour the option and its first descendants are actually painted is therefore
  // part of the option's identity - otherwise this check is unreadable and gets guessed.
  try{
   const seen=[];
   const walk=n=>{if(!n||seen.length>12||seen.indexOf(n)>=0)return;seen.push(n);
    const cs=getComputedStyle(n);
    const bg=String(cs.backgroundColor||'');
    if(bg&&!/rgba?\(\s*0\s*,\s*0\s*,\s*0\s*,\s*0\s*\)|transparent/i.test(bg))add(bg);
    for(const k of n.children||[])walk(k);
   };
   walk(e);
  }catch{}
  try{add(txt(e))}catch{}
  try{add(e.value)}catch{}
  try{const m=String(e.id||'').match(/(\d+)\s*$/);if(m)add(m[1])}catch{}
  try{const s2=getComputedStyle(e);add(s2.backgroundColor);add(s2.color);add(e.getAttribute&&e.getAttribute('style'))}catch{}
  return parts.filter(Boolean).join(' | ');
 };
// Whether this question is an attention check at all decides how its options are read: a
// check the file has a rule for is read leniently, because a check drawn as logos or
// pictures has no visible box and would otherwise be skipped and left to chance.
const rhLenient=(q,text)=>!!(rhRuleFor(q,text)||rhRuleForCheck(q,text));
const rhOptions=(q,text)=>{
 const out=[];
 const els=(q&&q.els)||[];
 const lenient=rhLenient(q,text);
 for(const e of els){
  if(e.disabled)continue;
  if(!vis(e)&&!lenient)continue;
  if(lenient&&e.getBoundingClientRect&&(()=>{const r=e.getBoundingClientRect();return r.width===0&&r.height===0&&!e.closest('[role="radiogroup"],[role="group"],fieldset')})())continue;
  if(!((e.matches&&e.matches('input[type=radio],input[type=checkbox]'))||/^(radio|checkbox|option)$/i.test(String(e.getAttribute&&e.getAttribute('role')||''))||/label/i.test(String(e.className||''))))continue;
  const id=rhIdentity({el:e});
  if(!id)continue;
  const lab=norm(label(e)||txt(e)||'').trim();
  const code=String(e.value||'');
  const num=(String(e.id||'').match(/(\d+)\s*$/)||[])[1]||'';
  out.push({el:e,label:lab,code:code,num:num,id:id});
 }
 return out;
};
const rhAnswered=(q,text)=>rhOptions(q,text).some(o=>isOn(o.el));
// The rule a check is answered from: its own, or - when the file words the check
// differently - the nearest one by shared words. Null means this is an attention check
// this build has no answer for, and it must not be answered at all.
const rhRuleForCheck=(q,text)=>{
  const t=text===undefined?rhLocalText(q):text;
  const own=rhRuleFor(q,t);
  if(own)return own;
  if(!rhIsCheck(t))return null;
  return null; // unknown check must be reported, never borrowed from similar words
};
// What was done about an attention check, so the launcher log can show it. A check that
// was answered, and a check the config has no rule for, are both worth seeing: the second
// one is the whole answer to "why did it get screened out".
const RH_LOG=[];
// What the red-herring pass decided about EVERY question it looked at, including the
// ones it walked past. Without this the log cannot tell "no check on this page" from
// "a check was here and the key did not match it", and those two are fixed in
// completely different files - which is why a broken matcher stayed invisible for so
// long. One line per question, capped so a long grid cannot flood the log.
const RH_DIAG=[];
const rhDiag=(q,d,extra)=>{if(RH_DIAG.length>=40)return;const row={q:qkey(q),did:d};if(extra)for(const k in extra)row[k]=extra[k];RH_DIAG.push(row)};
// How a check was recognised: a rule matched its own words, an id, an id prefix, or -
// when the study words it differently - the nearest rule by shared words. Anything else
// on a question that reads like a check is a check this build must not guess at.
const rhHow=(own,near)=>own?(near?'nearest-rule':'rule'):'none';
// A rule may name one answer or several ("automobile" or "car"), and may point at the Nth
// option when the question is "select the fourth answer".
const rhWanted=rule=>{
 const list=[];
 if(rule.dynamic==='currentYear')list.push(String(new Date().getFullYear()));
 if(rule.dynamic==='previousYear')list.push(String(new Date().getFullYear()-1));
 if(Array.isArray(rule.answers))for(const a of rule.answers)list.push(String(a));
 if(rule.answer!=null&&String(rule.answer)!=="")list.push(String(rule.answer));
 return list.map(x=>x.trim()).filter(Boolean);
};
const rhColour=want=>{
 const hex=COLOURS[String(want).toLowerCase().trim()];
 if(!hex)return null;
 const r=parseInt(hex.slice(1,3),16),g=parseInt(hex.slice(3,5),16),b=parseInt(hex.slice(5,7),16);
 return [r,g,b];
};
const ORDINAL={first:1,second:2,third:3,fourth:4,fifth:5,sixth:6,seventh:7,eighth:8,ninth:9,tenth:10,last:0};
const rhHexToRgb=h=>{const m=String(h||'').trim().match(/^#?([0-9a-f]{6})$/i);if(!m)return null;const n=parseInt(m[1],16);return [(n>>16)&255,(n>>8)&255,n&255]};
// Every colour the option's identity carries, in the forms a page states a colour in:
// rgb()/rgba() as the computed style spells it, and #rrggbb as a stylesheet does. An
// option can carry more than one - a white cell with a purple swatch in it - so the
// answer to "select the purple colour" is "any of them is purple", not "the first one".
const rhRgbsOf=o=>{
  const all=String(o.id||'')+' '+String(o.label||'');
  const out=[];
  const re=/rgba?\((\d+),\s*(\d+),\s*(\d+)/gi;
  let m;
  while((m=re.exec(all))!==null)out.push([+m[1],+m[2],+m[3]]);
  const hex=/#([0-9a-f]{6})\b/gi;
  while((m=hex.exec(all))!==null){const c=rhHexToRgb(m[1]);if(c)out.push(c)}
  return out;
};
const rhRgbOf=o=>{const c=rhRgbsOf(o);return c.length?c[0]:null};
const rhWords=s=>norm(String(s||'')).toLowerCase().split(/[^a-z0-9]+/).filter(w=>w.length>0);
const rhPick=(rule,opts)=>{
 if(!opts.length)return null;
 if(rule.strict){
  const wants=rhWanted(rule).map(x=>norm(x).toLocaleLowerCase());
  const hits=opts.filter(o=>wants.includes(norm(o.label||'').toLocaleLowerCase()));
  return hits.length===1?hits[0]:null;
 }
 // "select the fourth answer" and friends: the position is the answer, whatever the
 // options happen to be called.
 if(rule.index==null){
  const joined=norm([rule.text,rule.contains,rule.note].join(' ')).toLowerCase();
  for(const w of Object.keys(ORDINAL))if(new RegExp('\\b'+w+'\\b').test(joined)&&!rhWanted(rule).length){
   const i=ORDINAL[w]===0?opts.length-1:ORDINAL[w]-1;
   if(i>=0&&i<opts.length)return opts[i];
  }
 }
 if(rule.index!=null){
  const i=Number(rule.index)-1;
  if(i>=0&&i<opts.length)return opts[i];
 }
 for(const want of rhWanted(rule)){
  const w=want.toLowerCase(),ww=rhWords(want);
  // A number is matched against what the option says, never against the number its id
  // happens to end in: "4" landing on an option whose id ends in 4 is how a correct check
  // gets answered wrongly.
  const numeric=/^\d+$/.test(w);
  let i=opts.findIndex(o=>String(o.label||'').trim().toLowerCase()===w);
  if(i>=0)return opts[i];
  i=opts.findIndex(o=>String(o.code||'').trim().toLowerCase()===w);
  if(i>=0)return opts[i];
  if(!numeric){
   i=opts.findIndex(o=>rhWords(o.label).join(' ')===w);
   if(i>=0)return opts[i];
   // The answer inside the option's own words: "Karate (no ball needed)" for "karate".
   i=opts.findIndex(o=>{const hay=rhWords(o.label).join(' ');return ww.length>0&&hay.indexOf(w)>=0});
   if(i>=0)return opts[i];
   i=opts.findIndex(o=>{const hay=rhWords(o.label).join(' ');return w.length>3&&hay.split(' ').indexOf(w)>=0});
   if(i>=0)return opts[i];
   // Every word of a multi-word answer present among the option's words.
   if(ww.length>1){
    i=opts.findIndex(o=>{const set=new Set(rhWords(o.label));return ww.every(x=>set.has(x))});
    if(i>=0)return opts[i];
   }
  }
  // A logo or a picture: the file name is part of the option's identity.
  if(!numeric){
   i=opts.findIndex(o=>{const hay=String(o.id||'').toLowerCase();return w.length>2&&hay.indexOf(w)>=0});
   if(i>=0)return opts[i];
  }
  const rgb=rhColour(want);
  if(rgb){
   i=opts.findIndex(o=>rhRgbsOf(o).some(c=>Math.abs(c[0]-rgb[0])<45&&Math.abs(c[1]-rgb[1])<45&&Math.abs(c[2]-rgb[2])<45));
   if(i>=0)return opts[i];
  }
 }
 return null;
};
// Returns the option the rule forbids, if the rule is a trap list, so the caller can keep
// it off the table.
// A rule that only lists traps protects any attention check it reaches, whatever its id.
// The rule a check is answered from, decided from the check's OWN text (see rhLocalText).
// The question's own wording is its TITLE when the markup has one, and only the whole
// container when it does not. A container holds the options too, so on a page where one
// option is called "Audi" or "Purple" the option text can satisfy a rule written for the
// question above it. Reading the title first keeps a rule about the question and a rule
// about one of its answers from ever meeting.
// The question's own wording - the sentence a rule in config/red-herring.json is written
// against. Reading it is the whole difficulty, and getting it wrong is why a matcher that
// works on paper recognises nothing on a live page:
//
//   * the nearest ancestor that merely LOOKS like a question is often the answer wrapper,
//     not the question - the radio sits under a div whose class contains "question" and
//     that div holds nothing but the options. "Male Female Other" is all the matcher gets,
//     and no rule written against "What gender do you associate yourself the most with?"
//     can ever match it.
//   * a container holds the options too, so option words ("Audi", "Purple") sit in the
//     same sentence a rule is matched against.
//
// So the sentence is the first of these that is really a sentence: the question's own title
// element, then any container up the chain with this question's option labels taken back
// out of it, and finally the analyzer's wider reading of the same question - accepted only
// when it demonstrably contains what we read here, which is what proves it is this question
// and not a neighbour's. A reading that is only option labels is never used while a wider
// one exists, because those labels are the answer space, not the question.
const RH_TEXT_CACHE=new WeakMap();
const RH_TITLE_SEL='.cf-question__title,.question-title,.questionTitle,[class*="question__title"],[class*="question-title"],[class*="questionTitle"],[class*="questionText"],[class*="question_label"],legend,h1,h2,h3,h4,h5,h6';
// The containers a question's sentence may sit in, nearest first.
const RH_BOX_SEL='.cf-question,.question,.question-container,fieldset,[role="radiogroup"],[role="group"],[data-question]';
const rhStripLabels=(text,labels)=>{
  let t=String(text||'');
  // Longest label first: "South East (excluding London)" has to go before "London", or the
  // shorter one is lifted out of the middle of the longer one and leaves "South East
  // (excluding )" behind in the sentence.
  const sorted=labels.map(l=>String(l||'').trim()).filter(l=>l.length>=2).sort((a,b)=>b.length-a.length);
  for(const s of sorted)t=t.split(s).join(' ');
  return norm(t);
};
// One option label is often another one with a word inside it - "London" and "South East
// (excluding London)" sit on the same question. Taking the labels back out of the container
// then leaves "South East (excluding )": damaged, not a sentence, and treating it as one
// hides the real wording. The test for that is not shape but content - leftover text is
// only a sentence if it brings at least one word none of the options said.
const rhIsSentence=(bare,labels)=>{
  const b=norm(bare||'');
  if(b.length<3)return false;
  const words=b.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean);
  if(!words.length)return false;
  const said=new Set();
  for(const l of labels){
    for(const w of norm(l||'').toLowerCase().split(/[^a-z0-9]+/))if(w)said.add(w);
  }
  return words.some(w=>!said.has(w));
};
const rhLocalText=q=>{
  const cached=RH_TEXT_CACHE.get(q);
  if(cached!==undefined)return cached;
  let out='';
  try{
   const e=(q&&q.els&&q.els[0])||null;
   if(!e)out=norm(q&&q.text||'');
   else{
    const labels=[];
    for(const x of (q.els||[])){try{const l=norm(label(x)||txt(x)||'');if(l)labels.push(l)}catch{}}
    // 1. the question's own title, if the markup has one
    const box=e.closest?e.closest(RH_BOX_SEL):null;
    let title='';
    try{const t=box&&box.querySelector?box.querySelector(RH_TITLE_SEL):null;if(t)title=norm(txt(t))}catch{}
    if(title.length>=3)out=title;
    let sentence='', firstWhole='';
    if(out.length<3){
     // 2. Walk up from the question's own container. At each level the text is read with
     //    this question's option labels taken back out of it, and the first level where
     //    anything is left over is the sentence. The walk matters: the nearest ancestor
     //    that looks like a question often holds the options and nothing else, and
     //    stopping there is what left every rule with no sentence to match.
     let node=(box&&box!==e)?box:(e.parentElement||e), d=0;
     while(node&&d<6){
      const whole=norm(txt(node)||'');
      if(whole&&!firstWhole)firstWhole=whole;
      const bare=rhStripLabels(whole,labels);
      if(rhIsSentence(bare,labels)){sentence=bare;break}
      node=node.parentElement;d++;
      if(node&&/^(FORM|BODY|HTML)$/.test(node.tagName||''))break;
     }
    }
    if(out.length<3)out=sentence;
    // 3. No sentence anywhere up the chain, so what we have is the option labels and
    //    nothing else. The analyzer reads the same question wider and does see the
    //    sentence, so take its reading - but only when it contains the labels we read
    //    ourselves, which is what proves it is this question and not a neighbour's.
    if(out.length<3){
     const qt=norm(q&&q.text||'');
     out=(qt.length>firstWhole.length&&(firstWhole===''||qt.indexOf(firstWhole)>=0))?qt:firstWhole;
    }
    if(out.length<3)out=norm(q&&q.text||'');
   }
  }catch{out=norm(q&&q.text||'')}
  RH_TEXT_CACHE.set(q,out);
  return out;
};
// A trap list belongs to one kind of check - the feelings scale whose "Angry/Happy/Sad"
// codes end the interview - and must not be applied to a year question whose fourth
// option happens to sit in a box coded 4. `when` scopes it to that wording; without it
// the list still applies, but only to a question that reads like a check at all.
const rhRuleApplies=(rule,text)=>{
  if(!rule)return false;
  if(rule.when){
   const t=norm(text||'');
   try{if(new RegExp(String(rule.when),'i').test(t))return true}catch{return false}
   return false;
  }
  return true;
};
// `when` also lets the key name a check the wording list cannot be trusted to catch -
// "Please select one answer. How do you feel right now?" says nothing an ordinary
// question never says, except the second half. A trap list that names its wording is
// therefore also what makes that wording count as a check.
const rhTrapWhen=t=>{
  const s=norm(t||'');
  if(!s)return false;
  for(const r of RH_ALL){
   if(!r.when||!Array.isArray(r.never)||!r.never.length)continue;
   try{if(new RegExp(String(r.when),'i').test(s))return true}catch{}
  }
  return false;
};
// The one decision that says "this is a check": either it reads like one, or the key
// says this exact wording is one.
const rhIsCheck=t=>looksAttention(t)||rhTrapWhen(t);
// Every rule whose words match this check. More than one rule with different answers means
// the check is worded ambiguously for this build, and an ambiguous check is reported rather
// than guessed at.
const rhRulesMatching=t=>{
 const text=norm(t||'').toLowerCase();
 const hits=[];
 for(const r of RH_RULES){
  if(r.id||r.idStarts)continue;
  let ok=false;
  if(r.text){try{ok=new RegExp(String(r.text),'i').test(text)}catch{ok=false}}
  if(!ok&&r.contains)ok=text.indexOf(String(r.contains).toLowerCase())>=0;
  if(ok)hits.push(r);
 }
 return hits;
};
const rhTrapOnly=()=>RH_ALL.filter(r=>Array.isArray(r.never)&&r.never.length&&r.answer==null&&!(Array.isArray(r.answers)&&r.answers.length)&&r.index==null&&!r.id&&!r.idStarts&&!r.text&&!r.contains);
// Every trap list that covers this question. A list with a `when` only covers the wording
// it names; a list without one covers any question the pass decided is a check.
const rhTrapRulesFor=q=>rhTrapOnly().filter(r=>rhRuleApplies(r,rhLocalText(q)));
const rhTrap=(rule,opts)=>{
  const never=Array.isArray(rule&&rule.never)?rule.never.map(x=>String(x).trim()):[];
  if(!never.length)return null;
  for(const o of opts){
   if(never.indexOf(o.code)>=0||never.indexOf(o.num)>=0)return o;
   const low=String(o.label||'').toLowerCase();
   for(const n of never)if(n.length>1&&low.indexOf(n.toLowerCase())>=0)return o;
  }
  return null;
};
const qkey=s=>((s||'').replace(/[ \t\n\r]+/g,' ').toLowerCase().slice(0,240)).trim();
const qpre=s=>qkey(s).slice(0,160);
const avoidRules=Array.isArray(MEM&&MEM.avoid)?MEM.avoid:[];
const avoidExact=new Set();
for(const a of avoidRules){const k=qkey(a&&a.question),o=qkey(a&&a.option);if(k&&o)avoidExact.add(k+'||'+o)}
const reopen=new Set((Array.isArray(MEM&&MEM.reopen)?MEM.reopen:[]).map(qkey));
// What has already completed this survey: those answers are tried first, unless the
// store has since learned to avoid them.
const preferMap=new Map();
for(const k in ((MEM&&MEM.prefer)||{})){const q=qkey(k),o=qkey((MEM.prefer||{})[k]);if(q&&o)preferMap.set(q,o)}
// Answer sets that already ended a run badly. pickedTrail is what this page has
// answered so far, so a profile that is being rebuilt is caught while it is built.
const bannedSets=Array.isArray(MEM&&MEM.banned)?MEM.banned.map(String):[];
const pickedTrail=[];
// How each answer the store knows has gone before: positive completed the survey,
// negative ended a run.
const scoreMap=new Map();
for(const q in ((MEM&&MEM.score)||{})){const row=(MEM.score||{})[q]||{};const m2=new Map();for(const o in row)m2.set(qkey(o),Number(row[o])||0);scoreMap.set(qkey(q),m2)}
// The profile being replayed while one question is varied.
const pinMap=new Map();
for(const q in ((MEM&&MEM.pin)||{}))pinMap.set(qkey(q),qkey((MEM.pin||{})[q]));
const probeQ=qkey(MEM&&MEM.probeQ||'');
const probeExcluded=new Set((()=>{const v=String((MEM&&MEM.probeExclude)||'').split(/\s+/).filter(Boolean);return v.map(qkey)})());
function preferredFor(q){const k=qkey(q);return k&&preferMap.has(k)?preferMap.get(k):''}
// rank orders the answers of a question: the profile that already worked comes first,
// a replayed answer keeps the run comparable, an answer the store has a record for is
// judged on that record, and the answer being probed away is taken off the table.
function rankOf(q,o){
 const k=qkey(q),ok=qkey(o);
 if(!k||!ok)return 0;
 let sc=0;
 const row=scoreMap.get(k);
 if(row&&row.has(ok))sc=row.get(ok)*10;
 if(pinMap.get(k)===ok)sc+=25;
 if(preferMap.get(k)===ok)sc+=15;
 if(k===probeQ&&probeExcluded.has(ok))sc-=1000;
 if(isAvoid(k,ok))sc-=1000;
 if(wouldRebuildBanned(k,ok))sc-=1000;
 return sc;
}
function bestTier(q,labels,pool){
 let top=-1e9;
 const sc=[];
 for(let i=0;i<pool.length;i++){const v=rankOf(q,labels[i]);sc.push(v);if(v>top)top=v}
 return sc.map((v,i)=>v===top?i:-1).filter(i=>i>=0);
}
function preferredFor(q){const k=qkey(q);return k&&preferMap.has(k)?preferMap.get(k):''}
function wouldRebuildBanned(q,o){
 const qk=qkey(q),ok=qkey(o);
 if(!qk||!ok||!bannedSets.length||!pickedTrail.length)return false;
 for(const raw of bannedSets){
  const parts=raw.split('\u0000').filter(Boolean);
  if(parts.length<2)continue;
  // Everything answered so far has to line up with this profile, in order.
  let pos=0,aligned=true;
  for(const t of pickedTrail){
   const cut=t.indexOf('\u0000');
   const tq=cut<0?t:t.slice(0,cut),to=cut<0?'':t.slice(cut+1);
   let found=-1;
   for(let i=pos;i+1<parts.length;i++){if(parts[i]===tq&&parts[i+1]===to){found=i;break}}
   if(found<0){aligned=false;break}
   pos=found+2;
  }
  if(!aligned)continue;
  for(let i=pos;i+1<parts.length;i++){if(parts[i]===qk&&parts[i+1]===ok)return true}
 }
 return false;
}
const exhausted=new Set();
const forceNext=!!(MEM&&MEM.forceNext);
function isAvoid(q,o){const k=qkey(q),op=qkey(o);if(!op)return false;return avoidExact.has(k+'||'+op)}
const vis=e=>{if(!e||e.disabled)return false;try{
 if(e.closest?.('.cf-carousel__content-item[aria-hidden="true"]'))return false;
 const shown=x=>{const r=x.getBoundingClientRect(),s=getComputedStyle(x);if(!(r.width>0&&r.height>0&&s.display!=='none'&&s.visibility!=='hidden'&&Number(s.opacity)!==0))return false;for(let p=x.parentElement;p;p=p.parentElement){const st=getComputedStyle(p),pr=p.getBoundingClientRect();if(/hidden|clip/.test(st.overflow+' '+st.overflowX+' '+st.overflowY)&&(r.right<=pr.left||r.left>=pr.right||r.bottom<=pr.top||r.top>=pr.bottom))return false}return true};
 if(shown(e))return true;
 // Styled native choices often hide the input and show its associated label.
 // Require a real visible label so hidden pages and collapsed panels stay hidden.
 if(e.matches?.('input[type=radio],input[type=checkbox]'))return [...(e.labels||[])].some(shown);
 return false;
}catch{return false}};
const cleanText=e=>{try{const c=e.cloneNode(true);for(const n of c.querySelectorAll('style,script,noscript,template'))n.remove();return norm(c.textContent||'')}catch{return norm(e&&e.textContent||'')}};
const txt=e=>norm(e?.innerText||cleanText(e)||e?.value||e?.getAttribute?.('aria-label')||e?.getAttribute?.('title')||'');
const esc=s=>{try{return CSS.escape(s)}catch{return String(s).replace(/[^a-zA-Z0-9_-]/g,'\\$&')}};
const rnd=n=>{n=Math.max(1,n|0);return Math.floor(Math.random()*n)};
const shuffle=a=>{for(let i=a.length-1;i>0;i--){const j=rnd(i+1);const t=a[i];a[i]=a[j];a[j]=t}return a};
const usePersona=/persona/i.test(String(P.AnswerMode||''));
const native=[...document.querySelectorAll('input,select,textarea,button,a,[role],[contenteditable="true"],[tabindex]')].filter(vis);
const selected=[];
const shadow=[];
function walk(root){for(const e of root.querySelectorAll?.('*')||[]){if(!vis(e))continue;shadow.push(e);if(e.shadowRoot)walk(e.shadowRoot)}}
try{walk(document)}catch{}
for(const e of shadow){if(!native.includes(e))native.push(e)}
const frames=[...document.querySelectorAll('iframe,frame')].filter(vis).map(f=>({src:f.src||'',name:f.name||'',sameOrigin:!!f.contentDocument}));
const roleRadios=native.filter(e=>/^(radio)$/i.test(e.getAttribute('role')||''));
const roleChecks=native.filter(e=>/^(checkbox|switch)$/i.test(e.getAttribute('role')||''));
const roleButtons=native.filter(e=>/^(button|link|menuitem|option)$/i.test(e.getAttribute('role')||''));
const customChoices=native.filter(e=>/^(radio|checkbox|option)$/i.test(e.getAttribute?.('role')||'')||e.hasAttribute?.('aria-checked')||e.hasAttribute?.('data-value')||e.hasAttribute?.('data-option')||e.hasAttribute?.('data-answer')||e.hasAttribute?.('data-code'));
const nativeRadios=native.filter(e=>e.matches?.('input[type="radio"]'));
const ranges=native.filter(e=>e.matches?.('input[type="range"]'));
const nativeChecks=native.filter(e=>e.matches?.('input[type="checkbox"]'));
const selects=native.filter(e=>e.tagName==='SELECT' || /^(combobox|listbox)$/i.test(e.getAttribute('role')||''));
const textInputs=native.filter(e=>e.matches?.('textarea,input:not([type]),input[type="text"],input[type="number"],input[type="email"],input[type="tel"],input[type="search"],input[type="url"],input[type="date"],input[type="time"],input[type="month"],input[type="datetime-local"]')||e.isContentEditable);
const clickable=[...new Set([...native.filter(e=>e.matches?.('button,a,[role="button"],[role="link"],[role="menuitem"],[role="option"],input[type="submit"],input[type="button"],[tabindex]:not([tabindex="-1"])')), ...roleButtons])].filter(vis);
