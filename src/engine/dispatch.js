// Interaction dispatch: a page is read as ONE interaction model, chosen in priority
// order - drag/drop first, then grid/carousel, then sequential open text - and never
// as a bag of controls. The model that wins owns the page's navigation, so a carousel
// advance is never confused with the survey's Next and a second open-text slot never
// receives a copy of the first answer.
const SEQ_MAX=3;
const SEQ_MAX_WHEN_BLOCKED=6;
const EMO_VALUES=[0.72,0.84,0.64,0.78,0.88,0.68,0.86,0.62,0.80,0.74,0.82,0.66];
// Each brand on an emo-resonance scale is dragged to its own spot. The widget shows one
// brand at a time, so a position taken from a per-step counter handed every brand the
// same offset - eight identical ratings, which is not how anyone answers. Spots are
// drawn from the pool at random and are not reused while the pool still has one free.
const emoSpot=()=>{
 const used=(window.__apEmoSpots instanceof Set)?window.__apEmoSpots:(window.__apEmoSpots=new Set());
 let pick=0;
 for(let n=0;n<EMO_VALUES.length;n++){const c=EMO_VALUES[rnd(EMO_VALUES.length)];if(!used.has(c)){pick=c;break}}
 if(!pick)pick=EMO_VALUES[rnd(EMO_VALUES.length)];
 used.add(pick);
 return pick;
};
const OPEN_POOL=['Reliable and easy to use','Good value for the price','I see it often in stores','Easy to find in shops','Good quality for everyday use','Works well for my routine','Offers useful features','A brand I would recommend','Reasonably priced overall','Consistent quality every time','Convenient to buy','Fits what I am looking for'];
function openTextAnswer(used){
 const seen=new Set(used.map(v=>norm(v).toLowerCase()));
 const base=norm(P.OpenText||'').replace(/[.\s]+$/,'');
 for(const c of [base,...OPEN_POOL]){const v=norm(c).replace(/[.\s]+$/,'');if(v&&!seen.has(v.toLowerCase()))return v}
 let i=2,v=base||'Test response';
 while(seen.has(v.toLowerCase())&&i<40){v=(base||'Test response')+' '+i;i++}
 return v;
}
function dragPool(){
 return [...document.querySelectorAll('[draggable="true"],[role="slider"],input[type="range"],[class*=slider],[class*=Slider],[class*=handle],[class*=Handle],[class*=knob],[class*=Knob],[class*=thumb],[class*=Thumb],[class*=drag],[class*=Drag],[class*=card],[class*=Card],[class*=slot],[class*=Slot],[class*=timeline],[class*=Timeline],[class*=track],[class*=Track],[class*=circle],[class*=Circle],[class*=bubble],[class*=dot],svg circle,ellipse')].filter(e=>vis(e));
}
// classAttr reads the class of an SVG node too, where .className is an object and a
// string test on it silently never matches.
const cn=e=>String(e&&e.getAttribute&&e.getAttribute('class')||'')+' '+(e&&e.className&&e.className.baseVal!==undefined?String(e.className.baseVal):'');
function trackOf(h){
 let n=h.parentElement,d=0;
 while(n&&d<6){const r=n.getBoundingClientRect(),hr=h.getBoundingClientRect();if(r.width>hr.width*1.6&&r.width>40)return n;n=n.parentElement;d++}
 return h.parentElement||h;
}
// A scale page holds a row of cards under a line and asks the respondent to lift each
// one onto the line. The cards are draggable items, not handles, and the line has no
// text of its own, so it is found as the widest thin band above them.
function detectScaleDrop(pool){
 const found=pool.filter(e=>/timeline-item|ui-draggable|scale-item|brand-item|drag-item/i.test(cn(e))&&vis(e));
 // The row that holds the cards and the bar they are dropped on share the "timeline"
 // prefix. A container is told apart by its own class word, not by a substring: the
 // cards are "item" while the row around them is "items".
 const structural=/(^|[\s_-])(items|container|bar|line|axis|track|wrapper|inner|list|row|group|holder)([\s_-]|$)/i;
 const cards=found.filter(e=>{
  const words=String(cn(e)).trim().split(/\s+/);
  if(words.some(w=>structural.test(w)))return false;
  const r=e.getBoundingClientRect();
  return r.width>=40&&r.height>=40;
 });
 if(cards.length<2)return null;
 const boxEl=(cards[0].closest?cards[0].closest('.cf-question,.question,.question-container,fieldset,[role="group"],[data-question]'):null)||document.body;
 const unplaced=cards.filter(c=>!/\bdropped\b/i.test(String(c.className||'')));
 const row=(unplaced.length?unplaced:cards);
 // A card that has been placed sits on the line, so the line is measured against the
 // cards still waiting below it, not against the highest card on the page.
 const top=Math.min(...row.map(c=>c.getBoundingClientRect().top));
 let band=null,bw=0,bt=0;
 for(const e of boxEl.querySelectorAll('div,span,hr,ol,ul,svg')){
  if(e.contains(cards[0]))continue;
  const r=e.getBoundingClientRect();
  if(r.width<150||r.height<=0||r.height>70||r.top>top-2)continue;
  if(txt(e).length>40)continue;
  if(!band||r.width>bw||(r.width===bw&&r.top<bt)){band=e;bw=r.width;bt=r.top}
 }
 if(!band){
  // Still no line above the cards: take the widest thin empty bar anywhere in the
  // question, which is the scale itself once a card has been dropped onto it.
  for(const e of boxEl.querySelectorAll('div,span,hr,svg')){
   if(e.contains(cards[0]))continue;
   const r=e.getBoundingClientRect();
   if(r.width<150||r.height<=0||r.height>70)continue;
   if(txt(e).length>40)continue;
   if(!band||r.width>bw){band=e;bw=r.width}
  }
 }
 if(!band)return null;
 return {kind:'scale_drop',cards,track:band,hasAdvance:false,total:cards.length,answered:0,done:false};
}
function detectTimeline(pool){
 if(!/\btimeline\b|drag (the )?(cards|items|blocks)|place (the )?(cards|items)|order the following|chronolog/i.test(lower))return null;
 const cards=pool.filter(e=>/card/i.test(cn(e))||e.getAttribute('draggable')==='true');
 if(cards.length<2)return null;
 const zones=pool.filter(e=>/slot|position|zone|timeline|track|year|line/i.test(cn(e))&&cards.indexOf(e)<0);
 if(!zones.length)return null;
 return {kind:'timeline',cards,zones,hasAdvance:false,total:cards.length,answered:0,done:false};
}
function detectEmo(pool){
 // The same widget answers a "move your circle" question and a "rate the brands on
 // this scale" one: the wording changes, the boxes do not. Cards that belong to the
 // scale are not sliders, though, and are never treated as one.
 if(pool.some(e=>/timeline-item|ui-draggable|scale-item|brand-item|drag-item/i.test(cn(e))&&vis(e)&&e.getBoundingClientRect().width>=40))return null;
 // Generic answer containers and ordinary "scale from" prompts also occur on
 // basic questions. A hidden widget elsewhere on the page is not evidence of
 // an active circle question.
 const shaped=/(move|drag)\s+your\s+(circle|slider)|circle[^.]{0,60}represent|move your circle|show how you feel about|distance between|line representing a scale/i.test(lower);
 const circleWidget=[...document.querySelectorAll('.sliders-container')].find(e=>vis(e)&&[...e.querySelectorAll('.answer-container input[type="range"]')].some(vis));
 if(!shaped&&!circleWidget)return null;
 const handles=pool.filter(e=>/slider|handle|knob|thumb|dot|circle|bubble|drag/i.test(cn(e))||e.getAttribute('role')==='slider'||e.getAttribute('draggable')==='true'||(e.matches&&e.matches('input[type="range"]')));
 if(!handles.length)return null;
 // The page wraps the slider and the brand disc in containers whose own class also
 // matches. Dragging a wrapper moves nothing and then counts as an answer, so only the
 // innermost element of each match is a handle.
 const leaves=handles.filter(h=>!handles.some(o=>o!==h&&h.contains&&h.contains(o)));
 // The disc the circle lands in and the little drag icon are decoration: they match the
 // class search but hold no answer. A real slider (a range input or a role=slider) is
 // the only control that can be moved, so it is the only one counted.
 const byRole=leaves.filter(e=>e.getAttribute('draggable')==='true'||String(e.getAttribute&&e.getAttribute('role')||'').toLowerCase()==='slider');
 const byName=leaves.filter(e=>/drag|handle|thumb|knob|grip/i.test(cn(e)));
 const byRange=leaves.filter(e=>e.matches&&e.matches('input[type="range"]'));
 // Range inputs are the authoritative value-bearing controls of the circle
 // widget. Its animated drag-icon can temporarily become visible and must not
 // replace the range merely because it matches the word 'drag'.
 const circleRanges=byRange.filter(e=>e.closest?.('.sliders-container .answer-container'));
 const list=circleRanges.length?circleRanges:(byRole.length?byRole:(byName.length?byName:(byRange.length?byRange:leaves)));
 if(!list.length)return null;
 const box=(circleWidget||list[0])?.closest?.('.cf-question,.question,.question-container,fieldset,[data-question]')||circleWidget||null;
 return {kind:'emo_resonance',box,handles:list,hasAdvance:false,total:list.length,answered:0,done:false};
}
function boxGroups(box,includeHidden){
 if(!box||!box.querySelectorAll)return {total:0,answered:0};
 const all=buildQuestions(box,includeHidden).filter(q=>q.owner==='radio'||q.owner==='check'||q.owner==='custom');
 let els=[];
 for(const q of all)for(const e of q.els)if(includeHidden||vis(e))els.push(e);
 els=[...new Set(els)];
 // The carousel is counted in slides, not in questions: a slide that shows one answer
 // at a time is a single question to the analyzer, and counting it as one would call
 // the carousel finished after its first item. An item is answered when every
 // question inside it is, not when every control is - a radio group only ever has
 // one control on.
 const items=coarseGroups(box,els);
 if(items&&items.length>=2){
  const owner=new Map();
  for(const q of all)for(const e of q.els)owner.set(e,q);
  const flags=items.map(g=>{const qs=[];for(const e of g){const q=owner.get(e);if(q&&qs.indexOf(q)<0)qs.push(q)}return qs.length>0&&qs.every(qAnswered)?1:0});
  return {total:items.length,answered:flags.reduce((a,b)=>a+b,0),items:flags};
 }
 return {total:all.length,answered:all.filter(qAnswered).length};
}
// Walks up from the controls to the widest split that still separates them into at
// least two groups inside the box - the slide/row level - and stops when the levels
// only merge everything back together. A level made of <label> elements is the answer
// options of one question, not the items of a list, so it is passed over.
function coarseGroups(box,els){
 if(els.length<2)return null;
 let level=els.map(e=>e.parentElement),best=null;
 for(let d=0;d<10;d++){
  const order=[],idx=new Map();
  for(let i=0;i<els.length;i++){const a=level[i];if(!a)continue;if(!idx.has(a)){idx.set(a,order.length);order.push(a)}}
  if(order.length<2)break;
  if(!order.every(a=>box.contains(a)))break;
  if(order.every(a=>a&&a.tagName==='LABEL')){level=level.map(a=>a?a.parentElement:null);continue}
  const groups=[];
  for(const a of order){const g=els.filter((_,i)=>level[i]===a);if(g.length)groups.push(g)}
  if(groups.length<2)break;
  best=groups;
  level=level.map(a=>a?a.parentElement:null);
 }
 return best;
}
// A reentry/identifier field is not one of the open-answer slots: it has its own value
// (the unique test ID) and must stay with the generic text block even when a
// sequential list owns the rest of the page's text.
const isReentryQ=q=>reentryPattern.test(q.text||'')||q.els.some(e=>reentryPattern.test(attrBlob(e)));
function detectCarousel(){
 // This carousel changes cards automatically after a choice and may hide both
 // navigation arrows. Count its real answer state, including off-screen cards.
 for(const box of document.querySelectorAll('.cf-question--carousel-grid')){
  const slides=[...box.querySelectorAll('.cf-carousel__content-item')].filter(s=>s.querySelector('[role="radio"],input[type="radio"]'));
  if(slides.length<2)continue;
  const answered=slides.filter(s=>[...s.querySelectorAll('[role="radio"],input[type="radio"]')].some(isOn)).length;
  return {kind:'auto_carousel_grid',box,slides,total:slides.length,answered,done:answered===slides.length,hasAdvance:false};
 }
 const cand=[...document.querySelectorAll('[class*=carousel i],[class*=swiper i],[class*=slide i],[class*=arrow i],[aria-label*=next i],[aria-label*=previous i],[aria-label*=prev i],[class*=next i],[class*=prev i]')].filter(e=>vis(e)&&!e.disabled);
 // The list that holds the slides matches "slide" just as its own arrows do, and
 // taking it for the advance means clicking a div that does nothing while the real
 // arrow is never pressed. Only a leaf control that wraps no answers qualifies.
 const nodes=cand.filter(e=>e.matches('button,a,[role="button"],input[type="button"],input[type="submit"],[tabindex],[aria-label]')&&!e.querySelector('input,select,textarea,[role="radio"],[role="checkbox"],[role="option"]'));
 let box=null,adv=null;
 for(const n of nodes){const b=n.closest('.question,.question-container,fieldset,[role="group"],[data-question]');if(b){box=b;adv=n;break}}
 if(!box||!adv)return null;
 const g=boxGroups(box,true);
 if(g.total<2)return null;
 return {kind:'carousel_grid',box,adv,hasAdvance:true,total:g.total,answered:g.answered,done:g.answered>=g.total};
}
function detectSequential(){
 const vt=questions.filter(q=>q.owner==='text'&&!isReentryQ(q)&&q.els.some(e=>vis(e)&&!e.disabled));
 if(!vt.length)return null;
 const clue=/what else|anything else|anything more|and what else|please add|tell me more|anything further|anything at all|anything you|next one|another (one|answer|brand|entry)/i.test(lower+' | '+q);
 // Two visible text fields are the ordinary "first name / last name" shape, not a
 // list: only take that pair over when the page itself asks for one more answer.
 if(vt.length<2||(vt.length===2&&!clue))return null;
 return {kind:'sequential_open_text',textQs:vt,hasAdvance:false,total:vt.length,answered:vt.filter(qAnswered).length,done:false};
}
// The models whose page is one widget rather than a list of controls. Their item count
// is the list of prompts on the page, so it - and nothing the plain scanner makes of the
// widget's own markup - decides whether Next may be pressed.
const WIDGET_MODELS=new Set(['emo_resonance','scale_drop','timeline','carousel_grid','auto_carousel_grid']);
function detectPageModel(){
 const pool=dragPool();
 return detectScaleDrop(pool)||detectTimeline(pool)||detectEmo(pool)||detectCarousel()||detectSequential();
}
function applyModel(m){if(m)m.ownsText=m.kind==='sequential_open_text';return m}
// The generic text block is switched off only for a sequential list: everything else
// still answers ordinary controls, while navigation is always owned by the model.
let pageModel=applyModel(detectPageModel());
// The model is looked for again when the first pass found nothing: carousels and drag
// handles are often rendered by a script that has not run yet when the engine boots.
function ensureModel(){if(!pageModel)pageModel=applyModel(detectPageModel());return pageModel}
function customAdvance(){
 const m=pageModel;if(!m||!m.box)return null;
 if(m.adv&&vis(m.adv)&&!m.adv.disabled)return m.adv;
 const btns=[...m.box.querySelectorAll('button,a,[role="button"],[tabindex]')].filter(e=>vis(e)&&!e.disabled);
 return btns.find(e=>/^(next|continue|next brand|next item|›|»|→|>)$/i.test(txt(e)))||btns.find(e=>/next|forward|arrow/i.test(txt(e)+' '+(e.getAttribute('aria-label')||'')))||null;
}
// Counts are re-read from the page every step: a carousel reveals one item at a time
// and a slot only counts as done once the answer is actually in it.
function refreshCustom(){
 const m=pageModel;if(!m)return;
 if(m.kind==='auto_carousel_grid'){
  m.slides=m.slides.filter(s=>s.isConnected!==false);
  m.total=m.slides.length;
  m.answered=m.slides.filter(s=>[...s.querySelectorAll('[role="radio"],input[type="radio"]')].some(isOn)).length;
  m.done=m.total>0&&m.answered===m.total;
 }else if(m.kind==='carousel_grid'){
  if(m.box&&m.box.isConnected!==false){
   const g=boxGroups(m.box,true);
   m.total=g.total;m.answered=g.answered;m.items=g.items||[];
  }
  m.hasAdvance=!!customAdvance();
  m.done=m.total>0&&m.answered>=m.total;
 }else if(m.kind==='sequential_open_text'){
  m.textQs=(m.textQs||[]).filter(q=>q.els.some(e=>vis(e)&&!e.disabled&&!e.readOnly));
  m.total=m.textQs.length;
  m.answered=m.textQs.filter(qAnswered).length;
  m.done=m.total>0&&m.answered>=m.total;
 }else if(m.kind==='emo_resonance'){
  m.handles=(m.handles||[]).filter(vis);
  m.total=m.handles.length;
  m.answered=m.handles.filter(isOn).length;
  m.done=m.total>0&&m.answered>=m.total;
 }else if(m.kind==='scale_drop'){
  m.cards=(m.cards||[]).filter(vis);
  m.total=m.cards.length;
  m.answered=m.cards.filter(c=>/\bdropped\b/i.test(String(c.className||''))||isOn(c)).length;
  m.done=m.total>0&&m.answered>=m.total;
 }else if(m.kind==='timeline'){
  m.cards=(m.cards||[]).filter(vis);
  m.total=m.cards.length;
  m.answered=m.cards.filter(isOn).length;
  m.done=m.total>0&&m.answered>=m.total;
 }
}
function customUnanswered(){
 const m=pageModel;if(!m)return 0;
 if(m.done)return 0;
 return Math.max(0,(m.total||0)-(m.answered||0));
}
