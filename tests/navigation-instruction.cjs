const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const nav=fs.readFileSync('src/engine/navigate.js','utf8').replace(/\r\n/g,'\n');
const start=nav.indexOf('if(!completion&&!termination){\n if(window.__apRenderPending)');
const end=nav.indexOf('\nconst chainOf=',start);
const code=nav.slice(start,end);
function run(block,force,unanswered,pending){let clicks=0;const c={completion:false,termination:false,valErr:[],summaryOnly:false,validationRecovery:{changed:false},needMore:false,pairs:[],window:{__apRenderPending:false},modelAdvance:null,waitingForNext:false,unanswered,forceNext:force,next:{},INSTRUCTION_BLOCK:{count:block},accordion:{pending},action:'',navFingerprint:()=>'',fire:()=>{clicks++;return true},Date};vm.createContext(c);vm.runInContext(code,c);return clicks}
assert.equal(run(1,false,0,0),0,'blocked instruction must prevent Next on a completed widget');
assert.equal(run(1,true,0,0),0,'forced Next must not bypass instructions');
assert.equal(run(0,true,0,1),0,'pending accordion must prevent Next');
assert.equal(run(0,true,2,0),0,'forced Next must not bypass unanswered controls');
assert.equal(run(0,false,0,0),1,'ready page can proceed');
console.log('PASS 5 instruction/navigation checks (mock DOM)');
