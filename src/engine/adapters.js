const pairs=[];
const acts=[];
let acted=0;
const MAXACT=45;
let action='';
// Resolve a named group only when the page exposes a unique matching panel.
// Comparisons ignore spacing and punctuation, not the group identity.
const groupKey=s=>String(s||'').toLocaleLowerCase().replace(/[^\p{L}\p{N}]/gu,'');
function namedGroupRule(instruction){
 const text=norm(instruction?.text||'').replace(/\batleast\b/ig,'at least');
 const m=text.match(/^(?:please\s+|must\s+)*(?:select|choose|pick|tick|check|mark)\s+(?:at\s*least)\s+(\d{1,2})\s+(.+?)[.!]?$/i);
 if(!m||!ACCORDIONS.length)return null;
 const name=groupKey(m[2]),matches=ACCORDIONS.filter(a=>groupKey(a.label)===name);
 if(!name||matches.length!==1)return null;
 return {panel:matches[0].panel,min:Number(m[1])};
}
const validationRecovery=(!completion&&!termination)?recoverValidatedChoice():{changed:false,reason:''};
if(validationRecovery.changed){acts.push('validation-changed');acted++}
else if(valErr.length&&(!needMore||validationRecovery.reason==='validation-no-alternative'||validationRecovery.reason==='validation-conflicts-with-instruction')){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:validationRecovery.reason,validation:valErr.slice(0,3)});}
 if(!completion&&!termination){
  for(const q of questions){
   if(q.noGuess)continue;
   if(q.owner==='text'||q.owner==='select')continue;
   const qtext=rhLocalText(q);
   const localHits=rhRulesMatching(qtext).filter(r=>rhRuleApplies(r,qtext)&&rhImageMatch(r,q));
   // A colored instruction applies only to its own question. Never apply a single
   // red sentence to every radio group on a multi-question page.
   const instruction=redInstructionFor(q);
   // A mix is a constraint over the carousel's cards, not a demand to select
   // both radio answers on one card. Derive counts from the survey's own
   // checked state so refreshes and existing answers remain authoritative.
   // A redundant "do not select all yes/no" constraint is compatible with a
   // mix. Reject other extra clauses rather than discarding instructions.
   const mixRule=instruction&&/^(?:(?:we\s+need\s+to|please|must)\s+)?(?:select|choose|pick)\s+(?:a\s+)?mix(?:ture)?\s+of\s+yes\s+and\s+no\s*(?:[.!;]+\s*(?:do\s+not|don['’]?t)\s+(?:select|choose|pick)\s+all\s+(?:yes|no)\s*)?[.!]?$/i.test(norm(instruction.text));
   // A filtered grid can contain just one row, so a mix is impossible. Where
   // the same instruction expressly forbids all-no, select the unique Yes.
   if(mixRule&&!pageModel&&q.owner==='radio'&&questions.filter(x=>x.owner==='radio').length===1&&/\b(?:do\s+not|don['’]?t)\s+(?:select|choose|pick)\s+all\s+no\b/i.test(instruction.text)){
    const opt=q.els.filter(e=>vis(e)&&!e.disabled),yes=opt.filter(e=>norm(e.textContent||'').toLowerCase()==='yes'),no=opt.filter(e=>norm(e.textContent||'').toLowerCase()==='no');
    if(opt.length===2&&yes.length===1&&no.length===1){
     q.noGuess=true;
     if(!isOn(yes[0])){
      if(isAvoid(q.key,label(yes[0]))||!fireChoice(yes[0])||!isOn(yes[0])){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'single-row-mix-unavailable',question:q.key});continue}
      const name=label(yes[0]);selected.push(name);pairs.push({question:q.key,option:qkey(name)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(name));acted++;
     }
     acts.push('single-row-not-all-no');continue;
    }
   }
   if(mixRule&&pageModel?.kind==='auto_carousel_grid'&&q.owner==='radio'){
    q.noGuess=true;
    const current=pageModel.slides.find(s=>s.getAttribute('aria-hidden')==='false'&&s.querySelector('[role="radio"],input[type="radio"]'));
    const options=q.els.filter(e=>current?.contains(e)&&vis(e));
    if(!options.length)continue;
    const response=e=>norm(e.textContent||label(e)).toLowerCase();
    const yes=options.filter(e=>response(e)==='yes'),no=options.filter(e=>response(e)==='no');
    if(yes.length!==1||no.length!==1){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'mix-options-ambiguous',question:q.key});continue}
    if(options.some(isOn))continue;
    const counts={yes:0,no:0};
    for(const slide of pageModel.slides)for(const e of slide.querySelectorAll('[role="radio"],input[type="radio"]'))if(isOn(e)){const value=response(e);if(value==='yes'||value==='no')counts[value]++}
    // Establish both responses early, then vary subsequent cards without a
    // fixed alternating pattern.
    const wanted=counts.yes+counts.no===0?(rnd(2)?yes[0]:no[0]):counts.yes===0?yes[0]:counts.no===0?no[0]:(rnd(2)?yes[0]:no[0]);
    if(isAvoid(q.key,label(wanted))){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'mix-conflicts-with-memory',question:q.key});continue}
    if(fireChoice(wanted)){const answer=label(wanted);selected.push(answer);pairs.push({question:q.key,option:qkey(answer)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(answer));acts.push('carousel-mix');acted++}
    else{INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'carousel-selection-unverified',question:q.key})}
    continue;
   }
   const groupRule=namedGroupRule(instruction);
   if(groupRule&&groupRule.panel){
    const members=q.els.filter(e=>groupRule.panel.contains(e));
    if(!members.length){q.noGuess=true;skipQ.add(q.key);continue}
    q.noGuess=true;
    const active=members.filter(e=>vis(e)&&!e.disabled);
    const already=members.filter(isOn).length;
    const available=active.filter(e=>!isOn(e)&&!isAvoid(q.key,label(e)));
    const need=Math.max(0,groupRule.min-already);
    if(need>available.length){if(accordion.pending===0){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'group-selection-unavailable',question:q.key})}continue}
    for(const e of available.slice(0,need)){if(fireChoice(e)&&isOn(e)){const name=label(e);selected.push(name);pairs.push({question:q.key,option:qkey(name)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(name));acted++}else{INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'group-selection-unverified',question:q.key})}}
    acts.push('instruction-group');continue;
   }
   let instructionForChoice=instruction;
   if(instruction&&parseAgeRange(instruction.text)){
    const ageQs=questions.filter(x=>(x.owner==='select'||x.owner==='text')&&ageQuestion(x));
    if(ageQs.length!==1){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'ambiguous-age-control',question:q.key,instruction:instruction.text});continue}
    instructionForChoice={...instruction,text:instructionWithAgeRemoved(instruction.text)};
   }
   const countRule=instruction&&q.type==='multi'?selectionCount(instruction.text):null;
   if(countRule){
    const usable=q.els.filter(e=>!e.disabled&&vis(e));
    const currently=usable.filter(isOn);
    if(countRule.min>countRule.max||countRule.max<1||currently.length>countRule.max||countRule.min>usable.length){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'selection-count-unavailable',question:q.key,instruction:instruction.text});continue}
    // Recognize one explicit included label only when its full label is unique.
    // Category, exclusion and compound constraints stay for review.
    if(/\b(from the|within the|except|excluding|other than)\b/i.test(instruction.text)){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'selection-count-additional-constraint',question:q.key,instruction:instruction.text});continue}
    let requiredTarget=null;
    if(/\b(including|include|must contain|along with|together with)\b/i.test(instruction.text)){
     const m=instruction.text.match(/\b(?:including|include|must contain|along with|together with)\s+(?:the\s+)?(.+?)[.!]?$/i);
     const want=norm(m?.[1]||'').toLowerCase().replace(/^["'“”‘’]+|["'“”‘’]+$/g,'').trim();
     // Accept a partial label only at a word boundary and only if it names
     // exactly one visible option. Never guess when the shorthand is ambiguous.
     const hits=usable.filter(e=>{const text=norm(label(e)).toLowerCase();return text===want||(want.length>=3&&text.startsWith(want+' '))});
     if(!want||hits.length!==1){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'required-option-not-identified',question:q.key,instruction:instruction.text});continue}
     requiredTarget=hits[0];
     if(isAvoid(q.key,label(requiredTarget))){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'instruction-conflicts-with-termination-memory',question:q.key});continue}
    }
    const needsRequired=requiredTarget&&!isOn(requiredTarget);
    if(needsRequired&&currently.length>=countRule.max){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'required-option-exceeds-maximum',question:q.key});continue}
    const minimum=Math.max(1,countRule.min);
    const needed=Math.max(0,minimum-currently.length-(needsRequired?1:0));
    const choices=usable.filter(e=>e!==requiredTarget&&!isOn(e)&&!isAvoid(q.key,label(e))&&!/^(none|none of|not applicable)\b/i.test(norm(label(e))));
    if(needed>choices.length){q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'selection-count-unavailable',question:q.key});continue}
    let ok=true;
    const targets=(needsRequired?[requiredTarget]:[]).concat(choices.slice(0,needed));
    for(const target of targets){
     if(!fireChoice(target)){ok=false;break}
     const title=label(target);selected.push(title);pairs.push({question:q.key,option:qkey(title)});pickedTrail.push(qkey(q.key)+'\u0000'+qkey(title));acted++;
    }
    if(!ok||usable.filter(isOn).length<countRule.min||usable.filter(isOn).length>countRule.max){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'selection-count-unverified',question:q.key})}
    q.noGuess=true;acts.push('instruction-count');continue;
   }
   let desired=instructionForChoice?instructionTargets(q,instructionForChoice):null;
   if(desired?.targets?.some(e=>isAvoid(q.key,label(e)))){
    if(q.type==='single'&&desired.reason==='ordinal-alternative'){
     const available=(desired.alternatives||[]).find(e=>!isAvoid(q.key,label(e)));
     desired={...desired,targets:available?[available]:[]};
    }
    if(q.type==='single'&&/^label-alternative/.test(desired.reason)){
     desired=instructionTargets({...q,els:q.els.filter(e=>!isAvoid(q.key,label(e)))},instructionForChoice);
    }
    if(!desired?.targets?.length||desired.targets.some(e=>isAvoid(q.key,label(e)))){
     q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'instruction-conflicts-with-termination-memory',question:q.key,instruction:instruction.text});continue;
    }
   }
   // Generic selection wording is not a target-specific red attention check.
   // Let the normal question adapter select a response while retaining the
   // tighter guard for explicit or unrecognised answer directives.
   if(desired&&desired.reason!=='general-instruction'){
    // An unrecognised red command must NEVER fall back to random radio/checkbox.
    if(!desired.targets.length){
     q.noGuess=true;INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:desired.reason,question:q.key.slice(0,140),instruction:instruction.text.slice(0,300),options:q.els.map(e=>norm(label(e)||e.getAttribute?.('aria-label')||'')).filter(Boolean).slice(0,12)});
     RH_LOG.push({q:qkey(q.key),did:'instruction-needs-review',reason:desired.reason,instruction:instruction.text.slice(0,180)});
     continue;
    }
    let applied=0,failed=false;
    for(const target of desired.targets){
     if(isOn(target))continue;
     if(!vis(target)){failed=true;break}
     if(!fireChoice(target)){failed=true;break}
     const title=label(target)||target.getAttribute?.('aria-label')||'';
     selected.push(title);pairs.push({question:q.key,option:qkey(title)});
     pickedTrail.push(qkey(q.key)+'\u0000'+qkey(title));applied++;
    }
    acts.push(failed?'instruction-verify':'instruction-followed');acted+=applied;
    if(failed||desired.targets.some(e=>!isOn(e))){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push('selection-unverified');}
    if(failed)RH_LOG.push({q:qkey(q.key),did:'instruction-verify-failed',instruction:instruction.text.slice(0,180)});
    else RH_LOG.push({q:qkey(q.key),did:'answered-instruction',count:desired.targets.length,matched:desired.reason});
    q.noGuess=true; // Never add extra choices via the ordinary adapter.
    continue;
   }

   const distinct={};
   for(const h of localHits){const k=rhWanted(h).join('|')+'#'+(h.index==null?'':h.index);distinct[k]=h}
   const localList=Object.keys(distinct).map(k=>distinct[k]);
   if(localList.length>1){
    // Two rules with different answers both fit this wording. Answering either one is a
    // coin toss on an attention check, so it is reported and left alone.
    RH_LOG.push({q:qkey(qtext.slice(0,60)),did:'AMBIGUOUS-RULE-NOT-GUESSED',text:norm(qtext).slice(0,140),options:opts0(q)});
    rhDiag(q.key,'ambiguous-not-guessed',{text:qtext.slice(0,140),options:opts0(q)});
    q.noGuess=true;
    continue;
   }
   const own=localList[0]||rhRuleFor(q,qtext);
   if(own?.strict)q.noGuess=true;
   // The id is checked on its own so the log can say which of the two ways it was found.
   const via=localList.length?'own-words':(rhRuleById(q)?'id':'none');
   const looks=rhIsCheck(qtext);
   if(!own&&!looks){
    // Not a check as far as this build can tell. Recorded so that "the pass walked past it"
    // is a line in the log rather than an absence, which is what made a matcher that never
    // fired look identical to a page that had no checks on it.
    rhDiag(q.key,'not-a-check',{text:qtext.slice(0,120)});
    continue;
   }
   const opts=rhOptions(q,qtext);
   if(!opts.length){
    // A rule matched but the options could not be read - a check drawn as pictures with no
    // alt text, or options that have not rendered yet. This is the state that used to end
    // in silence, with the check then answered at random by the ordinary radio path.
    RH_LOG.push({q:qkey(q.key),did:'NO-OPTIONS-DO-NOT-GUESS',text:norm(qtext).slice(0,160),via:via});
    rhDiag(q.key,'rule-matched-but-no-options-read',{text:qtext.slice(0,140),via:via});
    q.noGuess=true;
    continue;
   }
   const rule=own||rhRuleForCheck(q,qtext);
   const how=rule?(own?(via==='id'?'id':'own-words'):'nearest-rule'):'none';
   // A standalone trap list protects the check whatever else matched: the feelings scale is
   // kept off Angry/Happy/Sad even on a build whose key also has an answer for it.
   const guards=rhTrapRulesFor(q);
   if(!rule&&guards.length){
    let blocked=[];
    for(const g of guards){const t=rhTrap(g,opts);if(t&&blocked.indexOf(t)<0)blocked.push(t)}
    if(blocked.length){
     RH_LOG.push({q:qkey(q.key),did:'trap-avoided-unknown-check',avoided:blocked.map(o=>String(o.label||o.code)).join('/'),text:norm(qtext).slice(0,120)});
     rhDiag(q.key,'trap-list-applied-to-unknown-check',{text:qtext.slice(0,120),avoided:blocked.map(o=>String(o.label||o.code)).join('/')});
     if(!rhAnswered(q,qtext)){
      const safe=opts.find(o=>blocked.indexOf(o)<0);
      if(safe&&fireChoice(safe.el)){selected.push(safe.label||safe.code);acts.push('attention-check');acted++;RH_LOG.push({q:qkey(q.key),did:'answered-safe-option',answer:String(safe.label||safe.code)})}
     }
    }
   }
   if(!rule){
    // An attention check this build has no answer for. It is reported in full - the text
    // and every option - and left alone: an unrecognised check answered by chance is the
    // one mistake that ends the interview, and ending it teaches nothing.
    RH_LOG.push({q:qkey(q.key),did:'NO-RULE-DO-NOT-GUESS',text:norm(qtext).slice(0,160),options:opts.map(o=>String(o.label||o.code)).slice(0,10)});
    rhDiag(q.key,'looks-like-a-check-but-no-rule',{text:qtext.slice(0,160),options:opts.map(o=>String(o.label||o.code)).slice(0,10)});
    q.noGuess=true;
    continue;
   }
   // A trap list on the rule being answered from, plus any standalone trap list that covers
   // this wording: both keep their options off the table.
   let trap=rhTrap(rule,opts);
   for(const g of guards){if(g===rule)continue;const t=rhTrap(g,opts);if(t&&!trap)trap=t}
  if(trap){
   // A trap option is not the answer and must not be taken, but the check still needs one:
   // the safest option that is left is answered, and the log says which.
   RH_LOG.push({q:qkey(q.key),did:'trap-avoided',avoided:String(trap.label||trap.code),left:opts.filter(o=>o!==trap).length});
   rhDiag(q.key,'trap-avoided',{text:qtext.slice(0,120),avoided:String(trap.label||trap.code),how:how});
  }
  const pick=rhPick(rule,opts);
  if(!pick){
   const wantsAnswer=(rule.answer!=null&&String(rule.answer)!=='')||rule.index!=null;
   RH_LOG.push(wantsAnswer
    ?{q:qkey(q.key),did:'ANSWER-NOT-FOUND',want:String(rule.answer==null?'':rule.answer),options:opts.map(o=>String(o.label||o.code)).slice(0,8)}
    :{q:qkey(q.key),did:'trap-list-applied',note:String(rule.note||'')});
   rhDiag(q.key,wantsAnswer?'rule-matched-but-no-option-carries-that-answer':'trap-list-applied',
    {text:qtext.slice(0,140),how:how,want:String(rule.answer==null?'':rule.answer),options:opts.map(o=>String(o.label||o.code)).slice(0,8)});
   if(rule.strict){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'known-check-answer-unavailable',question:q.key})}
   continue;
  }
  if(rhAnswered(q,qtext)&&(!rule.strict||isOn(pick.el))){RH_LOG.push({q:qkey(q.key),did:'already-answered'});rhDiag(q.key,'already-answered',{text:qtext.slice(0,120),how:how});continue}
  if(!fireChoice(pick.el)){RH_LOG.push({q:qkey(q.key),did:'CLICK-FAILED',want:String(pick.label||pick.code)});rhDiag(q.key,'click-failed',{text:qtext.slice(0,120),how:how});continue}
  try{pick.el.setAttribute('data-ap-rh','1')}catch{}
  selected.push(pick.label||pick.code);
  pairs.push({question:q.key,option:qkey(pick.label||pick.code)});
  pickedTrail.push(qkey(q.key)+'\u0000'+qkey(pick.label||pick.code));
  acts.push('attention-check');acted++;
  RH_LOG.push({q:qkey(q.key),did:'answered',answer:String(pick.label||pick.code),note:String(rule.note||'')});
  rhDiag(q.key,'answered',{text:qtext.slice(0,140),how:how,answer:String(pick.label||pick.code)});
 }
  // A brand grid that the survey lays out with a script of its own can be left with no
  // box at all: every cell has zero size, so the control layer reads an empty question
  // and presses Next straight into a page the survey refuses ("Please select an answer").
  // The cells still answer a plain click without a box, so one brand is answered per step
  // - both renderings of it, because a responsive grid ships a mobile and a desktop copy -
  // and the brands still open are what hold the Next button.
  const pick0=opts=>opts&&opts.length?opts[0].el:null;
  const zeroCellGrids=()=>{
   const byBrand=new Map();
   for(const g of document.querySelectorAll('[role="radiogroup"][aria-required="true"]')){
    // A grid of logos is still a grid, but when the container is an attention check the
    // grid code must keep its hands off it: the key answers it, not the filler.
    const gq={key:qkey(String(g.id||'grid')),text:txt(g),els:g.firstElementChild?[g.firstElementChild]:[]};
    if(looksAttention(gq.text)&&(rhRuleFor(gq,gq.text)||rhRuleForCheck(gq,gq.text))){RH_LOG.push({q:qkey(txt(g).slice(0,80)),did:'grid-check-left-to-key'});continue}
    const opts=[...g.querySelectorAll('[role="radio"],.cf-radio,input[type="radio"]')];
    if(!opts.length)continue;
    if(opts.some(o=>o.getAttribute('aria-checked')==='true'||o.checked===true))continue;
    if(g.getBoundingClientRect().width>0)continue;
    const box=g.closest('.cf-question,.question,.question-container,fieldset,[role="group"],[data-question]');
    const br=box&&box.getBoundingClientRect();
    if(!br||br.width<=0||br.height<=0)continue;
    const brand=String(g.id||'').replace(/^(mobile|desktop)_/,'')+'|'+(g.getAttribute('aria-labelledby')||'');
    if(!byBrand.has(brand))byBrand.set(brand,[]);
    byBrand.get(brand).push(opts);
   }
   return byBrand;
  };
  const zeroGrids=zeroCellGrids();
  window.__apZeroLeft=zeroGrids.size;
  if(zeroGrids.size){
   const brand=[...zeroGrids.keys()][0];
   for(const opts of zeroGrids.get(brand)){
    // A cell that says the respondent never bought it, or bought it too long ago, is the
    // answer a random roll lands on now and then - and on a study that asks what people
    // bought lately it is a screen-out. Those cells are dropped and the rest are answered
    // with some variety, never the very first or the very last one.
    const cellText=o=>String((o.getAttribute&&o.getAttribute('aria-label'))||label(o)||'');
    const alive=opts.filter(o=>!/\bnever\b|12 months?|not (purchased|bought|used|consumed|in the last)|have not|haven'?t|purchased previously|don'?t know|not applicable|not at all|none of/i.test(cellText(o)));
    // Every row of the scale is its own question, so the rows are drawn independently -
    // but not blindly: a value another row of this same question already carries is not
    // drawn again, which is what makes the set of ratings look like a person's.
    const used=(window.__apCellUsed instanceof Map)?window.__apCellUsed:(window.__apCellUsed=new Map());
    const qk=qkey(stableQuestion(pick0(opts))||brand);
    const taken=used.get(qk)||new Set();
    const free=alive.filter(o=>!taken.has(cellText(o)));
    const pool=free.length>1?free:(alive.length>1?alive:opts);
    const pick=pool[Math.min(pool.length-1,1+rnd(Math.max(1,Math.floor((pool.length-1)*0.7))))];
    taken.add(cellText(pick));used.set(qk,taken);
    if(!pick||!fireChoice(pick))continue;
    selected.push(label(pick));
    pairs.push({question:qkey((stableQuestion(pick.closest('.cf-question,.question,fieldset,[role="group"],[data-question]'))||'grid')+' #'+brand),option:qkey(cellText(pick))});
    acts.push('grid-cell');acted++;
   }
  }
  // A page the drag model owns is a widget, not a list of controls: the brand cards on
  // a scale page are clickable, and clicking one puts the widget back where it started.
  const dragOnly=!!(pageModel&&(pageModel.kind==='scale_drop'||pageModel.kind==='timeline'));
// An option a red-herring rule marks as a trap is off the table for every other path too:
// the check is answered from the rule, and nothing else may hand it the wrong box. A
// standalone trap list counts as well, or the feelings scale would be answered at random
// by exactly the path this is meant to protect.
 const rhBlocked=e=>{const q=questions.find(x=>x.els&&x.els.indexOf(e)>=0);if(!q)return false;const t=rhLocalText(q);const opts=rhOptions(q,t);if(!opts.length)return false;const rules=[rhRuleFor(q,t)].concat(rhTrapRulesFor(q));for(const r of rules){if(rhRuleApplies(r,t)&&rhTrap(r,opts))return true}return false};
 // A question the key has answered is closed to every other path: the ordinary radio,
 // checkbox and grid code must not put a second, different answer on an attention check.
 // q.noGuess closes the ones the key reported but refused to answer - no rule, ambiguous
 // rule, unreadable options - so the generic path cannot pick one for it either.
const rhSettled=q=>!!(q.noGuess||(q.els&&q.els.length&&(rhRuleFor(q)||rhRuleForCheck(q)||rhTrapRulesFor(q).length)&&rhAnswered(q)));
// A separate exclusive radio may appear before lazily mounted accordion
// checkboxes. Do not treat it as a standalone required radio while the
// surrounding accordion choices are still being discovered.
const pendingAccordionExclusive=q=>!!(ACCORDIONS.length&&accQuestionRoot&&q.els.length&&q.els.every(e=>accQuestionRoot.contains(e)&&/^(?:none(?: of (?:the|these) (?:above|listed))?|not applicable)$/i.test(norm(label(e)))));
let radioQs=dragOnly?[]:questions.filter(q=>q.owner==='radio'&&!q.exclusiveNone&&!pendingAccordionExclusive(q)&&!q.noGuess&&!rhSettled(q));
 // One row of a matrix per step. A grid the survey re-reads after every click is answered
 // row by row, and Next belongs to the page: it is not pressed until the rows are done.
 let gridRow=false,gridCell=false;
 if(radioQs.length){for(const q of radioQs){
   if(acted>=MAXACT)break;
   const g=q.els.filter(vis).filter(e=>!rhBlocked(e));if(!g.length)continue;
   const qt=q.text;
   const already=g.some(isOn);
   if(already&&!reopen.has(q.key)&&!g.some(e=>isOn(e)&&isAvoid(q.key,label(e))))continue;
   if(!g.some(e=>!isOn(e)&&!isAvoid(q.key,label(e)))){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'no-safe-alternative',question:q.key});continue}
   if(q.grid&&gridRow)continue;
   const c=qt.toLowerCase();
   const kind=/gender|sex/.test(c)?'gender':/yes|no|agree|disagree|would you|have you|do you|did you/.test(c)?'yesno':/age|year/.test(c)?'age':/city|town|where do you go|which region|where do you live|county|\/state\b/.test(c)?'city':'middle';
   let want=usePersona?(kind==='gender'?P.Gender:kind==='age'?String(P.Age):kind==='city'?P.City:''):'';
   const previous=q.grid?questions.filter(x=>x!==q&&x.grid&&x.owner==='radio'&&x.els.length===q.els.length).map(x=>x.els.findIndex(isOn)).filter(i=>i>=0).pop():pageModel?.kind==='auto_carousel_grid'?-1:undefined;
   if(chooseFromGroup(g,want,kind,qt,q.key,previous)){acts.push('radio');acted++}
   if(q.grid)gridRow=true;
  }}
 let checkQs=dragOnly?[]:questions.filter(q=>q.owner==='check'&&!(ACC_SINGLE_CHOICE&&ACCORDIONS.length&&accSelected()>0)&&!questions.some(n=>n.exclusiveNone&&qAnswered(n))&&!q.noGuess&&!rhSettled(q));
 if(checkQs.length&&acted<MAXACT){for(const q of checkQs){
   if(acted>=MAXACT)break;
   const g=q.els.filter(vis).filter(e=>!rhBlocked(e));if(!g.length)continue;
   const qt=q.text,qk=q.key;
   let prevOn=[];
   if(reopen.has(qk)){prevOn=g.filter(e=>!e.disabled&&isOn(e)).map(label);for(const e of g){if(!e.disabled&&isOn(e)){fire(e);unmarkSel(e)}}}
   if(g.some(e=>!e.disabled&&isOn(e))&&!needMore&&!accordionMore(q))continue;
   if(q.grid&&gridCell)continue;
  const active=g.filter(e=>!e.disabled&&vis(e)&&!isOn(e));
  if(!active.length)continue;
const context=qt.toLowerCase();
   // "None" and "not applicable" are valid exclusive choices. Do not assume
   // a qualifying answer based on occupation, purchase history or products.
   const exclusive=e=>/^(none(?: of (?:the |these )?(?:above|listed))?|not applicable|no one|nobody)$/i.test(norm(label(e)));
   let cand=active.slice();
  let usable=cand.filter(e=>!isAvoid(qk,label(e)));
  if(!usable.length&&cand.length){exhausted.add(qk);INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'no-safe-alternative',question:qk});continue}
  // A remembered answer for this question leads the selection, so a survey that has
  // completed before is submitted the way it completed.
  const remembered=preferredFor(qk);
  if(remembered){
   const pick=usable.filter(e=>matchWant([label(e)],remembered,'exact')>=0&&!wouldRebuildBanned(qk,label(e)));
   if(pick.length)usable=[...pick,...usable.filter(e=>pick.indexOf(e)<0)];
  }
  usable=usable.filter(e=>!wouldRebuildBanned(qk,label(e)));
  if(!usable.length){exhausted.add(qk);INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'no-safe-alternative',question:qk});continue}
  if(prevOn.length){const diff=usable.filter(e=>prevOn.indexOf(label(e))<0);if(diff.length)usable=diff}
  if(/none of the above|none above/i.test(context)&&!usable.length){usable=active.slice(0,1)}
  if(!usable.length)continue;
let n;
   if(needMore){n=1}else{
     const max=Math.max(1,Math.min(Number(P.MultiSelectCount)||1,usable.length));
     if(!remembered)shuffle(usable);
     n=1+rnd(max);
   }
   // One cell of a checkbox matrix is one box: a grid row is not a "select N" list.
   if(ACC_SINGLE_CHOICE&&ACCORDIONS.length)n=1;
   if(accordionMore(q))n=Math.min(n,Math.max(1,Math.max(1,Math.min(8,Number(P.MultiSelectCount)||1))-accSelected()));
   if(usable.some(exclusive)){
    const ordinary=usable.filter(e=>!exclusive(e));
    usable=ordinary.length?ordinary:[usable.find(exclusive)];
    n=Math.min(n,usable.length);
   }
let did=false;
   for(const e of usable.slice(0,n)){if(fireChoice(e)){selected.push(label(e));pairs.push({question:qk,option:qkey(label(e))});pickedTrail.push(qk+'\u0000'+qkey(label(e)));did=true}}
   if(did){acts.push('checkbox');acted++}
   if(q.grid)gridCell=true;
  }}
 if(acted<MAXACT&&!dragOnly){for(const q of questions.filter(x=>x.owner==='custom')){
  if(acted>=MAXACT)break;
  const g=q.els.filter(vis).filter(e=>!rhBlocked(e));
  if(!g.length)continue;
  const usable0=g.filter(e=>{const t=label(e);return t.length>0&&!/^(next|continue|submit|finish|done|back|previous)$/i.test(t)});
  // Nothing readable to click is not a question the engine can answer, so it must
  // not hold the Next button either.
  if(!usable0.length){skipQ.add(q.key);continue}
  if(usable0.length===1&&!looksLikeQuestion(q)){skipQ.add(q.key);continue}
  const qt=q.text,qk=q.key;
  const on=isOn;
  let prevOn=[];
  if(reopen.has(qk)){prevOn=usable0.filter(e=>!e.disabled&&on(e)).map(label);for(const e of usable0){if(!e.disabled&&on(e)){fire(e);unmarkSel(e)}}}
  const active=usable0.filter(e=>!on(e));
  let usable=active.filter(e=>!isAvoid(qk,label(e)));
  if(!usable.length&&active.length){exhausted.add(qk);INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'no-safe-alternative',question:qk});continue}
  if(prevOn.length){const diff=usable.filter(e=>prevOn.indexOf(label(e))<0);if(diff.length)usable=diff}
  if(!usable.length)continue;
  const context=qt.toLowerCase();
  const kind=/gender|sex/.test(context)?'gender':/yes|no|agree|disagree|would you|have you|do you|did you/.test(context)?'yesno':/city|town|where do you go|which region|where do you live|county|\/state\b/.test(context)?'city':'middle';
  const want=usePersona?(kind==='gender'?P.Gender:kind==='yesno'?P.YesNo:kind==='city'?P.City:''):'';
  if(chooseFromGroup(usable,want,kind,qt,qk)){acts.push('custom-choice');acted++}
 }}
 if(acted<MAXACT&&!dragOnly){for(const q of questions.filter(x=>x.owner==='select')){
  if(acted>=MAXACT)break;
  const s=q.els[0];
  if(!s||s.tagName!=='SELECT'||!vis(s))continue;
  const qt=q.text,qk=q.key;
  let opts=[...s.options].filter(o=>!o.disabled&&String(o.value||'').trim()!==''&&!/^(?:please\s+)?(?:select|choose|pick)\b/i.test(norm(txt(o)))&&!/^(?:[-–—]+|\.{3,})$/.test(norm(txt(o))));
  if(!opts.length){skipQ.add(qk);continue}
  const have=selectAnswered(s);
  const ageRule=relatedAgeRule(q);
  if(ageRule?.error){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:ageRule.error,question:qk});continue}
  const ageValue=o=>/^\d{1,3}$/.test(norm(txt(o)))?Number(norm(txt(o))):NaN;
  if(ageRule?.range){opts=opts.filter(o=>{const n=ageValue(o);return Number.isFinite(n)&&n>=ageRule.range.min&&n<=ageRule.range.max});if(!opts.length){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'age-option-unavailable',question:qk});continue}}
  if(have&&!reopen.has(qk)&&!isAvoid(qk,txt(s.options[s.selectedIndex]))&&(!ageRule||opts.some(o=>o===s.options[s.selectedIndex])))continue;
  const prevVal=have?String(s.value):'';
  let pool=opts.filter(o=>!isAvoid(qk,txt(o)));
  if(!pool.length){exhausted.add(qk);INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'no-safe-alternative',question:qk});continue}
  if(prevVal&&reopen.has(qk)){const diff=pool.filter(o=>String(o.value)!==prevVal);if(diff.length)pool=diff}
 // A dropdown often carries no question text of its own - the field is named
 // "cq8010_input" and the sentence above it is the question - so the surrounding
 // question is read as well before deciding what the list is.
let c=(qt+' '+questionText(s)).toLowerCase();
   // An age band offered as a dropdown is the persona's age, not a roll of the dice: a
   // study that screens on age then screens the tool out on an age nobody in the profile
   // has. The band is matched by its own text, so "30" still finds "29 - 34".
   const isAge=/\bage\b|\byears? old\b|how old/.test(c);
   let wanted=usePersona?(/gender|sex/.test(c)?P.Gender:isAge?String(P.Age):/country|nationality/.test(c)?P.Country:''):'';
   let o=null;
   // A select that has taken this survey to the end page keeps that value.
   const remembered=preferredFor(qk);
   if(remembered){
    const ri=matchWant(pool.map(txt),remembered,'exact');
    if(ri>=0&&!wouldRebuildBanned(qk,txt(pool[ri])))o=pool[ri];
   }
   if(!o&&wanted){const i=matchWant(pool.map(txt),wanted,isAge?'exact':/gender|sex/.test(c)?'gender':'country');if(i>=0)o=pool[i]}
   o=o||pool[rnd(pool.length)];
  s.value=o.value;s.dispatchEvent(new Event('input',{bubbles:true}));s.dispatchEvent(new Event('change',{bubbles:true}));
  // A dropdown that ignores the value was never answered: recording it would put an
  // answer in the report that the survey does not hold, and the next step would answer
  // the same question again on top of it.
  if(String(s.value)!==String(o.value)||!selectAnswered(s)){
   acts.push('select-stuck');acted++;
   continue;
  }
  selected.push(txt(o));pairs.push({question:qk,option:qkey(txt(o))});pickedTrail.push(qk+'\u0000'+qkey(txt(o)));acts.push('select');acted++;
 }}
 // Native slider values have a browser default, which does not mean a user
 // answered the question. Move the slider and dispatch input/change explicitly.
 if(acted<MAXACT && !(pageModel&&pageModel.kind==='emo_resonance')){
  for(const rq of questions.filter(x=>x.owner==='range')){
   const el=rq.els[0];
   if(!el||!vis(el)||el.disabled||el.readOnly||qAnswered(rq))continue;
   const lo=Number.isFinite(Number(el.min))&&el.min!==''?Number(el.min):0;
   const hi=Number.isFinite(Number(el.max))&&el.max!==''?Number(el.max):100;
   if(hi<lo)continue;
   let value=Math.min(hi,Math.max(lo, Number.isFinite(Number(P.Number))?Number(P.Number):(lo+hi)/2));
   const st=Number(el.step);
   if(el.step!=='any'&&Number.isFinite(st)&&st>0)value=lo+Math.round((value-lo)/st)*st;
   value=Math.min(hi,Math.max(lo,value));
   writeValue(el,String(value));
   if(String(el.value)===String(value)){
    markSel(el);selected.push(String(value));pairs.push({question:rq.key,option:qkey(String(value))});acts.push('range');acted++;
   }
  }
 }
 // A sequential list owns the ordinary open-answer slots, but the reentry ID still
 // belongs to this block: it is the unique test ID, never one of the list's answers.
 if(acted<MAXACT){for(const q of questions.filter(x=>x.owner==='text'&&(!(pageModel&&pageModel.ownsText)||isReentryQ(x)))){
  if(acted>=MAXACT)break;
const e=q.els[0];if(!e||e.disabled||e.readOnly||!vis(e))continue;
   const qt=q.text,qk=q.key;
  const checkText=rhLocalText(q),checkRule=rhRuleFor(q,checkText);
  if(checkRule?.kind==='text'){
   const answers=rhWanted(checkRule);
   if(answers.length!==1){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'known-check-answer-unavailable',question:qk});continue}
   const value=answers[0];
   if(String(e.value||'')!==value)typeInto(e,value);
   if(String(e.value||'')!==value||(e.validity&&e.validity.valid===false)){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'known-check-answer-unverified',question:qk});continue}
   selected.push(value);pairs.push({question:qk,option:qkey(value)});acts.push('attention-check-text');acted++;continue;
  }
   let c=qt.toLowerCase();
  const hasId=String(P.ReentryID||'').trim()!=='';
  const qjunk=!/[a-z]{3,}/.test(c)||/[{};]|\.cf-[a-z]/.test(c);
  const bareBox=qjunk&&textInputs.filter(vis).length===1&&(e.required||e.getAttribute('aria-required')==='true')&&answered<=2;
  const fieldIsId=reentryPattern.test(c)||reentryPattern.test(attrBlob(e))||(bareBox&&hasId);
  // A required respondent ID cannot be skipped: keep Next blocked and report it.
  if(fieldIsId&&!hasId){if(!e.required&&e.getAttribute('aria-required')!=='true')skipQ.add(qk);continue}
  const ageRule=relatedAgeRule(q);
  if(ageRule?.error){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:ageRule.error,question:qk});continue}
  if(!fieldIsId&&String(e.value||'').trim()&&(!ageRule||Number(e.value)>=ageRule.range.min&&Number(e.value)<=ageRule.range.max))continue;
   // A closed question with an open box under it ("Other, please specify", a comment
   // field next to a rating) is one question, not two: the free text belongs to the
   // answer that was just chosen, so it is only typed when the page asks for it - it
   // is marked required, or the page has already refused these answers once.
   const box=(e.closest?e.closest('.cf-question,.question,.question-container,fieldset,[role="group"],[data-question]'):null)||e.parentElement;
   const combo=!!(box&&box.querySelector('input[type="radio"],input[type="checkbox"],select,[role="radio"],[role="checkbox"]'));
   const required=!!(e.required||e.getAttribute('aria-required')==='true'||e.getAttribute('aria-required')==='true');
   if(combo&&!required&&!fieldIsId&&!(MEM&&MEM.needOpen)){skipQ.add(qk);continue}
  const isId=fieldIsId&&hasId;
  let v=isId?P.ReentryID:/age|year/.test(c)?String(P.Age):/country|nationality/.test(c)?P.Country:/email/.test(c)?'test.respondent@example.invalid':/phone|mobile/.test(c)?'9999999999':P.OpenText;
  if(ageRule?.range){const n=Number(P.Age);v=String(Math.max(ageRule.range.min,Math.min(ageRule.range.max,Number.isFinite(n)?n:ageRule.range.min)))}
  // An open answer never ends in a full stop.
  if(!isId)v=String(v).replace(/[.\s]+$/,'')||v;
  if(e.matches?.('input[type="number"]')){
   const low=e.min!==''?Number(e.min):NaN,high=e.max!==''?Number(e.max):NaN;
   let n=ageRule?.range?Number(v):Number(P.Number);
   if(!Number.isFinite(n))n=0;
   if(Number.isFinite(low))n=Math.max(n,low);
   if(Number.isFinite(high))n=Math.min(n,high);
   const step=Number(e.step);
   if(e.step!=='any'&&Number.isFinite(step)&&step>0)n=(Number.isFinite(low)?low:0)+Math.round((n-(Number.isFinite(low)?low:0))/step)*step;
   if(Number.isFinite(low))n=Math.max(n,low);
   if(Number.isFinite(high))n=Math.min(n,high);
   if(ageRule?.range&&(n<ageRule.range.min||n>ageRule.range.max)){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'age-range-conflicts-with-field',question:qk});continue}
   v=String(n);
  }
  else if(e.matches?.('input[type="date"]')){
   const age=Math.max(1,Math.min(120,Number(P.Age)||30));
   v=String(new Date().getFullYear()-Math.floor(age))+'-06-15';
  }
  else if(e.matches?.('input[type="time"]'))v='12:00';
  else if(e.matches?.('input[type="datetime-local"]'))v='1990-06-15T12:00';
  else if(e.matches?.('input[type="month"]'))v='1990-06';
  // Native date/time and free-text constraints must not silently invalidate the response.
  if(/^(date|time|datetime-local|month)$/.test(e.type||'')){
   if(e.min&&String(v)<String(e.min))v=e.min;
   if(e.max&&String(v)>String(e.max))v=e.max;
  }
  if(!isId&&Number.isInteger(e.maxLength)&&e.maxLength>=0)v=String(v).slice(0,e.maxLength);
  typeInto(e,v);
  const stuck=String(e.value)===String(v)&&(!e.validity||e.validity.valid!==false);
  if(stuck){selected.push(String(v));pairs.push({question:qk,option:qkey(String(v))})}
  acts.push(isId?(stuck?'reentryID':'reentryID_mismatch'):(stuck?'text':'text_mismatch'));
  acted++;
 }}
}
