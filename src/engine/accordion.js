// Accordion discovery is intentionally separate from answer selection. A collapsed
// panel must be expanded and re-scanned on the next CDP step; hidden choices must
// never be mistaken for a finished page. No category names are hardcoded.
const ACC_OPTION='input[type="checkbox"],input[type="radio"],[role="checkbox"],[role="radio"]';
// SPA surveys can replace the whole question without changing the URL or
// keeping a .question-title element. The old key then reused `seen` across
// successive accordion questions and eventually clicked Next with all panels
// closed. Fingerprint the prompt BEFORE the first accordion heading; option
// content is excluded so lazy-loading choices cannot reset the fingerprint.
const ACC_FIRST=document.querySelector('details > summary,[aria-expanded],.accordion-header,.accordion-trigger,[data-accordion-trigger]');
const ACC_ROOT=ACC_FIRST?.closest('.cf-question,.question,.question-container,fieldset,[data-question]');
const ACC_HEAD=norm(document.querySelector('.question-title,.cf-question__title,[class*="question-title" i]')?.textContent||'');
const ACC_PREFIX=ACC_ROOT&&ACC_FIRST?norm(String(ACC_ROOT.textContent||'').split(String(ACC_FIRST.textContent||''))[0]).slice(0,220):'';
const ACC_PAGE=location.href+'|'+String(ACC_ROOT?.id||ACC_ROOT?.getAttribute('data-question')||'')+'|'+(ACC_PREFIX||ACC_HEAD);

if(!window.__apAccordionState||window.__apAccordionState.page!==ACC_PAGE)window.__apAccordionState={page:ACC_PAGE,seen:{},attempts:{},awaiting:{}};
const ACC_STATE=window.__apAccordionState;
function accTriggerCandidates(){
 const selectors='details > summary,[aria-expanded],[class*="category-header" i],[class*="group-title" i],[class*="accordion-header" i],[class*="accordion-trigger" i],[class*="accordion-toggle" i],[class*="collapse-toggle" i],[class*="expand-toggle" i],[class*="expander" i],[class*="category-title" i],[class*="group-header" i],[data-accordion-trigger]';
 return [...new Set(document.querySelectorAll(selectors))].filter(e=>vis(e)&&!e.disabled&&e.getAttribute('aria-disabled')!=='true');
}
function accModel(trigger){
 let item=trigger.closest('details,[class*="accordion-item" i],[data-accordion-item],[class*="collapsible" i],[role="treeitem"]');
 let panel=null;
 const controlled=trigger.getAttribute('aria-controls');
 if(controlled)panel=document.getElementById(controlled);
 if(!item)item=trigger.parentElement;
 if(!panel){
  const children=[...((item&&item.children)||[])];
  panel=children.find(e=>e!==trigger&&!e.contains(trigger)&&(
   e.matches?.('[role="region"],[class*="panel" i],[class*="content" i],[class*="body" i]')||e.querySelector?.(ACC_OPTION)))||null;
 }
 if(!panel&&trigger.nextElementSibling)panel=trigger.nextElementSibling;
 const expanded=trigger.closest('details')?.open===true||trigger.getAttribute('aria-expanded')==='true'||trigger.getAttribute('data-state')==='open';
 const closed=trigger.closest('details')?.open===false||trigger.getAttribute('aria-expanded')==='false'||trigger.getAttribute('data-state')==='closed'||trigger.getAttribute('data-expanded')==='false';
 const hiddenPanel=!!(panel&&(panel.hidden||panel.getAttribute('aria-hidden')==='true'||getComputedStyle(panel).display==='none'||getComputedStyle(panel).visibility==='hidden'));
 const content=panel?.querySelectorAll?.(ACC_OPTION)||[];
 // A generic expanded widget is not an accordion. Keep scope to actual choice groups,
 // explicit accordion structure, or a panel which holds choice controls.
 const structured=!!(controlled||trigger.closest('details,[class*="accordion" i],[class*="collapsible" i],[data-accordion-item]')||/accordion|collapse|expand|category-title|category-header|group-header|group-title/i.test(String(trigger.className||'')));
 const visuallyOpen=!!(panel&&content.length&&!hiddenPanel&&!closed);
 if(!structured||(!closed&&!expanded&&!hiddenPanel&&!visuallyOpen))return null;
 const label=norm(trigger.getAttribute('aria-label')||trigger.innerText||trigger.textContent||'').slice(0,140);
 if(!label||/^(next|continue|submit|finish|back)$/i.test(label))return null;
 const key=String(trigger.id||controlled||'')+'|'+label+'|'+[...document.querySelectorAll('summary,[aria-expanded],.accordion-header,.accordion-trigger')].indexOf(trigger);
 return {trigger,item,panel,key,label,expanded:(expanded||visuallyOpen)&&!hiddenPanel,closed:closed||hiddenPanel,content};
}
const ACCORDIONS=accTriggerCandidates().map(accModel).filter(Boolean);
const accQuestionRoot=ACCORDIONS[0]?.trigger.closest('.cf-question,.question,.question-container,fieldset,[data-question]');
const accInstructionText=norm(accQuestionRoot?.querySelector('legend,.question-title,[class*="question-title" i],h1,h2,h3,h4')?.textContent||'');
const ACC_SINGLE_CHOICE=/\b(?:select|choose|pick|mark|tick)\s+(?:only\s+)?one\b|\b(?:single|one)\s+answer\b|\bexactly\s+one\b/i.test(accInstructionText);
const accSelected=()=>ACCORDIONS.reduce((n,a)=>n+[...a.content].filter(e=>e.checked===true||e.getAttribute('aria-checked')==='true').length,0);
// The heading is not a choice; selected controls belong inside its panel.
function accordionPlan(){
 if(!ACCORDIONS.length)return {action:'',pending:0,count:0,selected:0};
const noneChecked=[...document.querySelectorAll('input[type="radio"],[role="radio"]')].some(e=>{
  const t=norm(label(e));return /^(none(?: of (?:the|these) (?:above|listed))?|not applicable)$/i.test(t)&&(e.checked===true||e.getAttribute('aria-checked')==='true');
 });
 for(const a of ACCORDIONS){
  const found=a.panel?.querySelectorAll?.(ACC_OPTION+' ,[data-option],[data-answer],[data-value]')?.length||0;
  if(found)delete ACC_STATE.awaiting[a.key];
 }
 const checked=accSelected();
 const exposed=ACCORDIONS.some(a=>a.expanded&&[...a.content].some(e=>vis(e)&&!e.checked&&e.getAttribute('aria-checked')!=='true'));
 const multi=ACCORDIONS.some(a=>[...a.content].some(e=>e.matches?.('input[type="checkbox"],[role="checkbox"]')))||/select all that apply|multiple answers|more than one/i.test(norm(document.body?.innerText||''));
 const wanted=ACC_SINGLE_CHOICE?1:multi?Math.max(1,Math.min(8,Number(P.MultiSelectCount)||1)):1;
 const redText=norm([...document.querySelectorAll('strong,b,[style*="color"],.red,.instruction')].filter(e=>{try{return getComputedStyle(e).color&&/^(rgb\(2[0-5]\d|red|#f)/i.test(getComputedStyle(e).color)}catch{return false}}).map(e=>e.textContent||'').join(' ')).toLowerCase();
 const list=[...document.querySelectorAll(ACC_OPTION)].filter(e=>ACCORDIONS.some(a=>a.panel?.contains(e)));
 const ordMatches=/\b(?:select|choose|pick|click|tick|mark|check)\b/i.test(redText)?[...redText.matchAll(/\b(first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|last)\b/g)].map(m=>m[1]):[];
 const positions={first:0,second:1,third:2,fourth:3,fifth:4,sixth:5,seventh:6,eighth:7,ninth:8,tenth:9};
 const ordinalOpts=ordMatches.map(x=>x==='last'?list[list.length-1]:list[positions[x]]).filter(Boolean);
 const targets=ACCORDIONS.filter(a=>{
  const names=[a.label,...[...a.content].map(e=>norm(label(e)||e.getAttribute('aria-label')||''))];
  return ordinalOpts.some(e=>a.panel?.contains(e))||names.some(name=>name.length>3&&redText.includes(name.toLowerCase()));
 });
const allRedOptions=[...new Set([...ordinalOpts,...list.filter(e=>{
  const name=norm(label(e)||e.getAttribute('aria-label')||'').toLowerCase();
  return name.length>3&&new RegExp('\\b'+name.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'\\b','i').test(redText);
 })])];
 const allRedChecked=allRedOptions.length>0&&allRedOptions.every(e=>e.checked===true||e.getAttribute('aria-checked')==='true');
 const enough=noneChecked||allRedChecked||(checked>=wanted&&targets.every(a=>ACC_STATE.seen[a.key])&&!allRedOptions.length);
 const loading=Object.keys(ACC_STATE.awaiting).filter(k=>ACC_STATE.awaiting[k]).length;
 const pending=enough?0:Math.max(loading,ACCORDIONS.filter(a=>a.closed&&!ACC_STATE.seen[a.key]).length);
 if(!pending)return {action:'',pending:0,count:ACCORDIONS.length,selected:checked};
 // Let the visible choices be answered before opening the next category. Otherwise
 // a single-open accordion collapses the first panel before its controls are read.
 if(exposed&&!targets.some(a=>a.closed&&!ACC_STATE.seen[a.key]))return {action:'',pending,count:ACCORDIONS.length,selected:checked};
 const candidates=targets.length?targets.filter(a=>a.closed&&!ACC_STATE.seen[a.key]):ACCORDIONS.filter(a=>a.closed&&!ACC_STATE.seen[a.key]);
 const item=candidates[0];
 if(!item)return {action:loading?'accordion-waiting':'',pending,count:ACCORDIONS.length,selected:checked};
 if((ACC_STATE.attempts[item.key]||0)>=2)return {action:'accordion-needs-review',pending,count:ACCORDIONS.length,selected:checked};
 ACC_STATE.attempts[item.key]=(ACC_STATE.attempts[item.key]||0)+1;
 const before=item.trigger.getAttribute('aria-expanded');
 fire(item.trigger);
 const changed=item.trigger.closest('details')?.open===true||item.trigger.getAttribute('aria-expanded')==='true'||(item.panel&&!item.panel.hidden&&getComputedStyle(item.panel).display!=='none');
 if(changed){ACC_STATE.seen[item.key]=true;if(!item.panel?.querySelector?.(ACC_OPTION+',[data-option],[data-answer],[data-value]'))ACC_STATE.awaiting[item.key]=true;return {action:'accordion-expand',pending:1,count:ACCORDIONS.length,selected:checked};}
 // Animation/framework updates may be asynchronous. Report the expansion attempt
 // and recheck on the next step instead of treating it as an answer.
 return {action:before==='false'?'accordion-expanding':'accordion-needs-review',pending:1,count:ACCORDIONS.length,selected:checked};
}
const accordion=accordionPlan();
