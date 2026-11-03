const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const source=fs.readFileSync('src/engine/header.js','utf8').replace(/\r\n/g,'\n');
const start=source.indexOf('async function renderedNow()');
const end=source.indexOf('\nwindow.__apRenderPending=false;\nfor',start);
assert(start>=0&&end>start);
const code=source.slice(start,end);
function harness(prompt,options={}){
 let now=0, mounted=false;
 const el={disabled:false,textContent:prompt,getBoundingClientRect:()=>({width:20,height:20}),matches:()=>false};
 const control={...el,textContent:'answer'};
 const document={readyState:'complete',querySelectorAll(sel){
  if(sel.startsWith('input,select'))return mounted?[control]:[];
  if(sel.startsWith('[role="slider"]'))return [];
  if(sel.startsWith('.question-title'))return [];
  return [el];
 },querySelector(){return options.pending?{}:null}};
 const context={document,window:{},getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'}),norm:s=>String(s||'').replace(/\s+/g,' ').trim(),Date:{now:()=>now}};
 vm.createContext(context);vm.runInContext(code,context);
 return {check:()=>context.renderedNow(),advance:n=>now+=n,mount:()=>mounted=true,ctx:context};
}
(async()=>{
 let h=harness('On the following pages you will see an exercise. You will be asked to indicate your preference.');
 assert.equal(await h.check(),false,'information page must first settle');h.advance(1499);assert.equal(await h.check(),false);h.advance(1);assert.equal(await h.check(),true);
 for(const text of ['Which option do you prefer?', 'Please select an answer.', 'On the next page you will see a chart. Which product do you prefer?', 'On the following screens you will see examples. Please select a response.', 'On the next page you will see a chart, please select a response now.']){
  h=harness(text);await h.check();h.advance(5000);assert.equal(await h.check(),false,text);
 }
 h=harness('On the next page you will be asked to select a response.',{pending:true});await h.check();h.advance(5000);assert.equal(await h.check(),false,'pending structure blocks advance');
 h=harness('On the following pages you will be asked to select a response.');await h.check();h.advance(500);h.mount();assert.equal(await h.check(),true,'mounted controls are handled by ordinary engine');
 console.log('PASS render readiness: future information, mixed questions, pending controls, late mount');
})().catch(e=>{console.error(e);process.exitCode=1});
