const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const s=fs.readFileSync('src/engine/analyze.js','utf8');
const fn=s.slice(s.indexOf('const INSTR_ORD='),s.indexOf('\n}',s.indexOf('function instructionTargets('))+2);
const ctx={norm:s=>String(s||'').replace(/\s+/g,' ').trim(),vis:()=>true,ACCORDIONS:[],label:e=>e.name,generalSelectionInstruction:()=>false,document:{querySelectorAll:()=>[]}};
vm.createContext(ctx);vm.runInContext(fn,ctx);
function run(text,names,type='single'){const opts=names.map(name=>({name,disabled:false}));const out=ctx.instructionTargets({els:opts,type},{text});return {names:out.targets.map(x=>x.name).join('|'),reason:out.reason};}
for(const [text,names,type,expected] of [
 ['Select Females',['Male','Female'],'single','Female'],
 ['Select Female or Male',['Male','Female'],'single','Female'],
 ['Select Female and Male',['Male','Female'],'single',''],
 ['Select option 2 or 3 or 4',['a','b','c','d'],'single','b'],
 ['Select option 2 and 3',['a','b','c'],'single',''],
 ['Select option 2 and 3',['a','b','c'],'multi','b|c'],
 ['Select unknown',['Male','Female'],'single',''],
 ['Select Male',['Male','Female'],'single','Male'],
 ['Select Females',['Female','Female'],'single','']
]) {const got=run(text,names,type);assert.equal(got.names,expected,`${text}: ${JSON.stringify(got)}`);}
console.log('PASS 9 focused instruction cases');
