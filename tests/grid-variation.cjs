const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const s=fs.readFileSync('src/engine/helpers.js','utf8');const fn=s.slice(s.indexOf('function chooseFromGroup('),s.indexOf('\n}',s.indexOf('function chooseFromGroup('))+2);
let chosen=null,remembered='';const ctx={qkey:x=>String(x||'').toLowerCase(),vis:()=>true,isAvoid:()=>false,exhausted:new Set(),label:e=>e.label,preferredFor:()=>remembered,bestTier:(_q,_l,p)=>p.map((_,i)=>i),matchWant:(ls,w)=>ls.findIndex(x=>x.toLowerCase()===String(w).toLowerCase()),rnd:()=>0,usePersona:true,P:{YesNo:'Yes'},fireChoice:e=>(chosen=e,true),selected:[],pairs:[],pickedTrail:[]};vm.createContext(ctx);vm.runInContext(fn,ctx);
const options=['Yes','No'].map(label=>({label,disabled:false,getAttribute:()=>null}));
assert.equal(ctx.chooseFromGroup(options,'','yesno','','row',0),true);assert.equal(chosen.label,'Yes','ordinary grid rows may repeat a valid preceding column');
remembered='Yes';ctx.chooseFromGroup(options,'','yesno','','row',0);assert.equal(chosen.label,'Yes','retain an explicitly remembered answer');
remembered='';let state=123456789;ctx.rnd=n=>{state^=state<<13;state^=state>>>17;state^=state<<5;return (state>>>0)%n};
const counts={Yes:0,No:0};for(let i=0;i<10000;i++){ctx.chooseFromGroup(options,'','yesno','','row',0);counts[chosen.label]++}
assert.ok(counts.Yes>4700&&counts.Yes<5300,`previous row must not exclude a valid option: ${JSON.stringify(counts)}`);
console.log('Synthetic grid after: previous column 0, selections',counts,'; previous code excluded Yes entirely');
console.log('PASS independent grid selection and explicit replay pin');
