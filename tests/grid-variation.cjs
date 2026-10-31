const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const s=fs.readFileSync('src/engine/helpers.js','utf8');const fn=s.slice(s.indexOf('function chooseFromGroup('),s.indexOf('\n}',s.indexOf('function chooseFromGroup('))+2);
let chosen=null,remembered='';const ctx={qkey:x=>String(x||'').toLowerCase(),vis:()=>true,isAvoid:()=>false,exhausted:new Set(),label:e=>e.label,preferredFor:()=>remembered,bestTier:(_q,_l,p)=>p.map((_,i)=>i),matchWant:(ls,w)=>ls.findIndex(x=>x.toLowerCase()===String(w).toLowerCase()),rnd:()=>0,usePersona:true,P:{YesNo:'Yes'},fireChoice:e=>(chosen=e,true),selected:[],pairs:[],pickedTrail:[]};vm.createContext(ctx);vm.runInContext(fn,ctx);
const options=['Yes','No'].map(label=>({label,disabled:false,getAttribute:()=>null}));
assert.equal(ctx.chooseFromGroup(options,'','yesno','','row',0),true);assert.equal(chosen.label,'No','avoid repeating the previous grid column when alternatives exist');
remembered='Yes';ctx.chooseFromGroup(options,'','yesno','','row',0);assert.equal(chosen.label,'Yes','retain an explicitly remembered answer');
console.log('PASS grid column variation and memory priority');
