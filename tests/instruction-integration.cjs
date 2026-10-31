const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const a=fs.readFileSync('src/engine/analyze.js','utf8'),d=fs.readFileSync('src/engine/adapters.js','utf8');
const helper=a.slice(a.indexOf('function parseAgeRange('),a.indexOf('\n}',a.indexOf('function instructionTargets('))+2);
const initial=d.slice(d.indexOf('  for(const q of questions){'),d.indexOf('\n   const distinct=',d.indexOf('  for(const q of questions){')));
const select=d.slice(d.indexOf(" if(acted<MAXACT&&!dragOnly){for(const q of questions.filter(x=>x.owner==='select'))"),d.indexOf('\n // Native slider',d.indexOf(" if(acted<MAXACT&&!dragOnly){for(const q of questions.filter(x=>x.owner==='select'))")));
function state(instruction, labels, ageText='How old are you?', ages=['15','22','35','50'], type='single'){
 const controls=labels.map(name=>({name,checked:false,disabled:false,getAttribute(){return null}}));
 const q={key:'gender',type,owner:type==='multi'?'check':'radio',text:'Which identity?',els:controls};
 const options=ages.map(n=>({value:n,textContent:n,disabled:false}));
 const sel={tagName:'SELECT',options,selectedIndex:0,disabled:false,_value:options[0].value,dispatchEvent(){}, get value(){return this._value}, set value(v){this._value=v;this.selectedIndex=options.findIndex(x=>x.value===v)}};
 const aq={key:'select|age',owner:'select',type:'select',text:ageText,els:[sel]};
 const messages={count:0,details:[]};const inst={text:instruction};
 const ctx={norm:x=>String(x??'').replace(/\s+/g,' ').trim(),attrBlob:()=>'',vis:()=>true,label:e=>e.name,ACCORDIONS:[],document:{querySelectorAll:()=>[]},generalSelectionInstruction:()=>false,
   questions:[q,aq],redBoldInstructions:[inst],redInstructionFor:x=>x===q?inst:null,rhLocalText:()=>'',rhRulesMatching:()=>[],rhRuleApplies:()=>false,
   INSTRUCTION_BLOCK:messages,RH_LOG:[],fireChoice:e=>(e.checked=true),isOn:e=>!!e.checked,selected:[],pairs:[],pickedTrail:[],acts:[],acted:0,MAXACT:45,qkey:x=>String(x).toLowerCase(),
   completion:false,termination:false,dragOnly:false,selectAnswered:s=>s.selectedIndex>=0,reopen:new Set(),isAvoid:()=>false,exhausted:new Set(),questionText:()=>ageText,
   usePersona:true,P:{Age:35,Gender:'Male',Country:'Test'},preferredFor:()=>'',wouldRebuildBanned:()=>false,matchWant:(xs,w)=>xs.indexOf(w),rnd:()=>0,txt:o=>o.textContent,
   Event:class{constructor(n){this.type=n}}};
 vm.createContext(ctx);vm.runInContext(helper,ctx);vm.runInContext(initial+'\n }',ctx);vm.runInContext(select,ctx);
 return {ctx,controls,sel,messages,q,aq};
}
let t=state('Please select Females only and age 18 - 45',['Male','Female']);
assert.equal(t.controls[0].checked,false);assert.equal(t.controls[1].checked,true);assert.equal(t.sel.value,'35');assert.equal(t.messages.count,0);
t=state('Please select Cats only and age 25 - 40',['Dogs','Cat'], 'How old are you?', ['17','25','39','47']);
assert.equal(t.controls[1].checked,true);assert.ok(['25','39'].includes(t.sel.value));assert.equal(t.messages.count,0);
t=state('Please select Cats only and age 25 - 40',['Dogs','Cat'],'How old are you?',['17','18']);
assert.ok(t.messages.details.some(x=>x.reason==='age-option-unavailable'));
t=state('Please select Rabbits only and age 25 - 40',['Dogs','Cat']);
assert.ok(t.messages.count>0);assert.ok(!t.controls.some(x=>x.checked));
let c=t.ctx;assert.deepEqual(JSON.parse(JSON.stringify(c.parseAgeRange('age between 20 and 30'))),{min:20,max:30});assert.equal(c.parseAgeRange('age 50 - 20'),null);
assert.equal(c.instructionWithAgeRemoved('Select Zebras only and age 21 - 40'),'Select Zebras');
assert.deepEqual(JSON.parse(JSON.stringify(c.selectionCount('Please select more than six options'))),{min:7,max:null});
assert.deepEqual(JSON.parse(JSON.stringify(c.selectionCount('Select exactly 3 responses'))),{min:3,max:3});
assert.equal(c.selectionCount('Select option 3'),null);
assert.equal(c.parseAgeRange('not age 20 - 30'),null);
assert.deepEqual(JSON.parse(JSON.stringify(c.selectionCount('Select at least 2 and at most 4 options'))),{min:2,max:4});
assert.deepEqual(JSON.parse(JSON.stringify(c.selectionCount('Select between two and five options'))),{min:2,max:5});
t=state('Select at least 3 and at most 2 options',['Alpha','Beta','Gamma'],'Other question',['1'], 'multi');
assert.ok(!t.controls.some(x=>x.checked));assert.ok(t.messages.count>0);
t=state('Please select at least three options including Beta',['Alpha','Beta','Gamma','Delta'],'Other question',['1'], 'multi');
assert.equal(t.controls.filter(x=>x.checked).length,3);assert.equal(t.controls[1].checked,true);assert.equal(t.messages.count,0);
t=state('Please select at least two options including Missing',['Alpha','Beta'],'Other question',['1'], 'multi');
assert.ok(!t.controls.some(x=>x.checked));assert.ok(t.messages.details.some(x=>x.reason==='required-option-not-identified'));
t=state('Please select exactly one option',['Alpha','Beta'],'Other question',['1'], 'multi');
assert.equal(t.controls.filter(x=>x.checked).length,1);assert.equal(t.messages.count,0);
assert.equal(c.instructionWithAgeRemoved('Select Cats and age 21 - 40 and region North'),'Select Cats and age 21 - 40 and region North');
console.log('PASS cross-question, count and required-option checks (mock DOM)');
