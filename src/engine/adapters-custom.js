// NPS template: each visible numeric row belongs to a hidden open question.
// Inventory displayed choices, click one, and require observable selection state.
// Never infer a stored answer from a click alone.
if(!completion&&!termination){
 const npsRoot=document.querySelector('#ISD');
 const npsRows=npsRoot?[...npsRoot.querySelectorAll('.question')].filter(vis):[];
 if(npsRoot&&vis(npsRoot)&&!npsRows.length){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'numeric-scale-not-rendered'});}
 for(const row of npsRows){
  if(!vis(row))continue;
  const leaves=[...row.querySelectorAll('button,label,span,div,li,td,[role="radio"],[role="button"]')].filter(e=>vis(e)&&/^\d{1,2}$/.test(norm(e.textContent||''))&&![...e.children].some(c=>/^\d{1,2}$/.test(norm(c.textContent||''))));
  const found=new Map();
  for(const e of leaves){
   const number=Number(norm(e.textContent));
   let target=e.closest('button,label,li,td,[role="radio"],[role="button"],[data-value]')||e;
   if(!row.contains(target))target=e;
   if(!found.has(number))found.set(number,[]);
   if(!found.get(number).includes(target))found.get(number).push(target);
  }
  const nums=[...found.keys()].sort((a,b)=>a-b);
  if(nums.length<3||nums.length>21||nums.some((n,i)=>i>0&&n!==nums[i-1]+1)||nums.some(n=>found.get(n).length!==1)){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'numeric-scale-options-ambiguous'});continue}
  const choices=nums.map(n=>({n,el:found.get(n)[0]}));
  const qkeyNps=qkey(stableQuestion(row)||'numeric scale');
  const state=e=>{
   const nodes=[e,...e.querySelectorAll('input,[aria-checked],[aria-pressed],[aria-selected]')];
   return nodes.some(x=>x.checked===true||x.getAttribute('aria-checked')==='true'||x.getAttribute('aria-pressed')==='true'||x.getAttribute('aria-selected')==='true'||/(?:^|[\s_-])(?:selected|active|checked)(?:$|[\s_-])/i.test(String(x.className||'')));
  };
  const owner=row.closest('.cf-question,.question-container,fieldset,[data-question]')||npsRoot;
  if(redBoldInstructions.some(i=>owner.contains(i.el)&&!generalSelectionInstruction(i.text))){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'numeric-scale-instruction-needs-review',question:qkeyNps});continue}
  if(choices.some(c=>state(c.el))){window.__apRenderPending=false;continue}
  const available=choices.filter(c=>!isAvoid(qkeyNps,String(c.n)));
  if(!available.length){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'numeric-scale-no-safe-choice',question:qkeyNps});continue}
  const chosen=available[rnd(available.length)],before=chosen.el.getAttribute('style')||'',beforeClass=String(chosen.el.className||'');
  fire(chosen.el);
  await new Promise(resolve=>setTimeout(resolve,80));
  const styleChanged=(chosen.el.getAttribute('style')||'')!==before||String(chosen.el.className||'')!==beforeClass;
  if(!state(chosen.el)&&!styleChanged){INSTRUCTION_BLOCK.count++;INSTRUCTION_BLOCK.details.push({reason:'numeric-scale-selection-unverified',question:qkeyNps});continue}
  window.__apRenderPending=false;
  selected.push(String(chosen.n));pairs.push({question:qkeyNps,option:String(chosen.n)});pickedTrail.push(qkeyNps+'\u0000'+chosen.n);acts.push('numeric-scale');acted++;
 }
}
// Custom adapters: the pages whose interaction is not a row of controls. They run off
// the model dispatch.js chose, so a second open-text slot never receives a copy of the
// first answer, the emo-resonance circle is actually dragged rather than clicked, and a
// timeline card is dropped into a slot instead of being left on the board.
if(!completion&&!termination&&pageModel){
 const m=pageModel;
 if(m.kind==='sequential_open_text'){
  const used=[];
  for(const q of m.textQs){
   if(acted>=MAXACT)break;
   const e=q.els[0];
   if(!e||e.disabled||e.readOnly||!vis(e))continue;
   if(reopen.has(q.key)&&String(e.value||'').trim()){typeInto(e,'');used.push('')}
   const cur=String(e.value||'').trim();
   if(cur){used.push(cur);continue}
   const v=openTextAnswer(used);
   if(!typeInto(e,v))continue;
   const got=String(e.value||'').trim();
   if(!got||!qAnswered(q)){acts.push('seq-text-mismatch');continue}
   used.push(got);
   selected.push(got);
   pairs.push({question:q.key,option:qkey(got)});
   acts.push('seq-text');
   acted++;
  }
}else if(m.kind==='emo_resonance'){
  let i=0;
  // The spot a brand was dropped at, kept on that brand's own box. The widget shows one
  // brand at a time, so the drop that rated it happened in an earlier pass and this is
  // the only place the answer is still readable from - without it a rated brand left no
  // answer behind at all.
  const emoFrac=b=>{try{const v=Number(b&&b.getAttribute('data-ap-frac'));return v>0?v:0}catch{return 0}};
  const recordEmo=(h,frac)=>{
   const pct=Math.round(frac*100);
   const qt=(stableQuestion(trackOf(h))||q||'emo resonance').slice(0,200);
   selected.push(pct+'%');
   pairs.push({question:qkey(qt),option:qkey(String(pct))});
   // Pressing Next destroys the execution context the answer was read in, so a rating is
   // lost from the report whenever the pass that rated a brand is the pass that navigated
   // away. It is parked on the page until a pass that survives picks it up and the runner
   // has the result in hand.
   const pend=window.__apEmoPending||(window.__apEmoPending=[]);
   if(!pend.some(x=>x.question===qkey(qt)&&x.option===qkey(String(pct))))pend.push({question:qkey(qt),option:qkey(String(pct))});
  };
  for(const h of m.handles){
   if(acted>=MAXACT)break;
   if(!h||!vis(h))continue;
   if(isOn(h)){i++;continue}
   const box=(h.closest?h.closest('[class*=answer-container i],[class*=sliders-container i],[class*=emo i]'):null)||trackOf(h);
// Only the survey's own state says the brand was rated: the box it marks "answered".
    // The circle's position is read relative to that box, so a scroll or a re-layout -
    // which move box and handle together - can never read as the survey answering.
    const answeredByBox=()=>{try{return !!(box&&(/\banswered\b/i.test(String(box.className||''))||box.getAttribute('data-answered')==='1'))}catch{return false}};
    if(answeredByBox()){markSel(h);const f=emoFrac(box);if(f>0)recordEmo(h,f);i++;continue}
    const sig=()=>{try{const hr=h.getBoundingClientRect(),br=box?box.getBoundingClientRect():null;return(box?String(box.getAttribute&&box.getAttribute('style')||'')+'|'+String(box.className||''):'')+'#'+String(h.getAttribute&&h.getAttribute('style')||'')+'#'+(br?Math.round(hr.left-br.left)+','+Math.round(hr.top-br.top):Math.round(hr.left)+','+Math.round(hr.top))}catch{return''}};
    const pos0=sig();
    let seen=null;
    try{seen=box?box.getAttribute('data-ap-pos0'):null}catch{}
    if(seen===null&&box){try{box.setAttribute('data-ap-pos0',pos0)}catch{}}
    else if(box&&pos0!==seen){markSel(h);const f=emoFrac(box);if(f>0)recordEmo(h,f);i++;continue}
    const frac=emoSpot();
    i++;
    try{if(box)box.setAttribute('data-ap-frac',String(frac))}catch{}
    let ok=false,hx=0,hy=0,tx=0,ty=0;
    if(h.matches&&h.matches('input[type="range"]')){
     // A range input behind a scripted slider keeps its own copy of the position, so a
     // value assignment moves the field without the survey ever seeing it: the thumb
     // itself is dragged instead, and only a drag that changes nothing falls back.
     const mn=Number(h.min||0),mx=Number(h.max||100);
     const want=Math.round(mn+(mx-mn)*frac);
     const holder=h.parentElement&&h.parentElement.closest?h.parentElement.closest('[class*=slider i],[class*=Slider i],[class*=track i],[class*=Track i]'):null;
     const thumb=holder&&holder.querySelector?holder.querySelector('[class*=handle i],[class*=thumb i],[class*=knob i],.ui-slider-handle'):null;
     const tr2=trackOf(thumb||h);
     const trr=tr2.getBoundingClientRect(),hr=(thumb||h).getBoundingClientRect();
     ty=trr.height>hr.height?trr.top+trr.height/2:hr.top+hr.height/2;
     tx=trr.left+hr.width/2+Math.max(1,trr.width-hr.width)*frac;
     hx=hr.left+Math.max(1,hr.width/2);hy=hr.top+Math.max(1,hr.height/2);
     const before=String(h.value||'');
     dragTo(thumb||h,tx,ty);
     if(String(h.value||'')===before){
      const proto=HTMLInputElement.prototype,set=Object.getOwnPropertyDescriptor(proto,'value')?.set;
      if(set)set.call(h,String(want));else h.value=String(want);
      h.dispatchEvent(new Event('input',{bubbles:true}));
      h.dispatchEvent(new Event('change',{bubbles:true}));
     }
     ok=String(h.value||'')!=='';
    }else{
     // This widget answers one horizontal move of the circle along its own row. The
     // distance is measured as the room the box actually leaves on each side, because a
     // target past the edge of the box (or high in it, where the brand disc sits) is a
     // drop the survey ignores - and a drop past the edge is not even on the page.
     // Attempts alternate sides, so a brand that did not take the first drop is not
     // offered the very same one again.
     const tr=trackOf(h);
     const trr=tr.getBoundingClientRect(),hr=h.getBoundingClientRect();
     hx=hr.left+Math.max(1,hr.width/2);hy=hr.top+Math.max(1,hr.height/2);
     const roomR=Math.max(0,trr.right-hr.width-hx),roomL=Math.max(0,hx-hr.width-trr.left);
     const reach=Math.max(16,Math.min(trr.width*frac,Math.max(roomL,roomR)));
     const tryN=(h.__apEmoTry=(h.__apEmoTry||0)+1);
     const dir=(tryN%2===0||roomR>=reach)?1:-1;
     tx=hx+dir*Math.min(reach,dir>0?roomR:roomL);ty=hy;
     ok=dragTo(h,tx,ty);
    }
    if(answeredByBox()){markSel(h);recordEmo(h,frac);
     acts.push('drag');
     acted++;
     continue;
    }
    // A widget that positions itself from a CSS variable or a pointer capture ignores
    // events built in the page, and a circle that moved inside the page is not a brand the
    // survey recorded either way. Until the widget's own state says answered, the handle
    // stays open and a real mouse drag is asked for instead - the coordinates are the
    // ones measured right now, and the engine is run again the moment the drop lands.
    dragRequests.push({x1:Math.round(hx),y1:Math.round(hy),x2:Math.round(tx),y2:Math.round(ty)});
    acts.push('drag');acted++;
   }
 }else if(m.kind==='scale_drop'){
  // Each card is held, carried up and let go over its own spot on the line, the way a
  // respondent does it. One card per step: every drop re-lays the page out, so the
  // next card is measured fresh instead of using coordinates that have moved.
  const cards=(m.cards||[]).filter(vis);
  const placed=cards.filter(c=>/\bdropped\b/i.test(String(c.className||'')));
  // Every card is tried in turn: the line refuses a card that lands on an occupied
  // spot, and retrying the same card over and over would never get past it.
  let target=null,bestN=1e9;
  for(const c of cards){
   if(isOn(c)||/\bdropped\b/i.test(String(c.className||'')))continue;
   const n=Number(c.getAttribute('data-ap-lift')||0);
   if(n<bestN){bestN=n;target=c}
  }
  if(target){
   const tr=(m.track&&m.track.isConnected!==false&&vis(m.track))?m.track:(cards[0].parentElement||m.track);
   const trr=tr.getBoundingClientRect(),cr=target.getBoundingClientRect();
   if(trr.width&&cr.width){
    // The line is where a card already sits; before the first one lands the band is
    // the best guess available.
    let y=Math.round(Math.max(4,Math.min(trr.bottom-4,trr.top+trr.height/2)));
    if(placed.length){
     const rr=placed[0].getBoundingClientRect();
     y=Math.round(rr.top+rr.height/2);
    }
    const idx=cards.indexOf(target);
    const x=Math.round(trr.left+trr.width*(idx+1)/(cards.length+1));
    try{target.setAttribute('data-ap-lift',String(bestN+1))}catch{}
    dragRequests.push({x1:Math.round(cr.left+cr.width/2),y1:Math.round(cr.top+cr.height/2),x2:x,y2:y});
    acts.push('lift');acted++;
   }
  }
 }else if(m.kind==='timeline'){
  const zones=m.zones.filter(vis);
  if(!zones.length){skipQ.add('timeline')}
  let zi=0;
  for(const c of m.cards){
   if(acted>=MAXACT)break;
   if(!c||!vis(c)||isOn(c))continue;
   const z=zones[zi%zones.length];
   if(!z)break;
   zi++;
   const zr=z.getBoundingClientRect();
   if(!zr.width||!zr.height)continue;
   const x=zr.left+zr.width/2;
   const y=zr.top+zr.height/2;
if(!dragTo(c,x,y))continue;
    // Same rule as the circle: when the card is still on the board after the drag the
    // board ignored it, so a real mouse drag is asked for before it counts.
    if(!(z.contains&&z.contains(c))){
     const cr=c.getBoundingClientRect();
     dragRequests.push({x1:Math.round(cr.left+Math.max(1,cr.width/2)),y1:Math.round(cr.top+Math.max(1,cr.height/2)),x2:Math.round(x),y2:Math.round(y)});
     acts.push('drop');acted++;
     continue;
    }
    markSel(c);
   const lbl=String(c.getAttribute('aria-label')||txt(c)||'card').slice(0,120);
   const qt=(stableQuestion(z)||q||'timeline').slice(0,200);
   selected.push(lbl);
   pairs.push({question:qkey(qt),option:qkey(lbl)});
   acts.push('drop');
   acted++;
  }
 }
}
