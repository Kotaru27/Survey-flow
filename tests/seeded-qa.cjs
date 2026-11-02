const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const h=fs.readFileSync('src/engine/header.js','utf8');
const code=h.slice(h.indexOf('const qaSeed='),h.indexOf('const shuffle='));
function stream(seed,run,step){let c=vm.createContext({RAW:{qaSeed:seed,qaRun:run,qaStep:step},Math,JSON});vm.runInContext(code+';globalThis.draw=rnd',c);return c.draw}
const draw=(seed,run,step)=>{const r=stream(seed,run,step);return Array.from({length:30},()=>r(7))};
assert.deepEqual(draw('qa',42,8),draw('qa',42,8),'same run/step must repeat');
assert.notDeepEqual(draw('qa',42,8),draw('qa',43,8),'distinct runs need independent streams');
assert.notDeepEqual(draw('qa',42,8),draw('qa',42,9),'steps need independent streams');
const a=stream('qa',1,1),b=stream('qa',2,1),combined=[];for(let i=0;i<20;i++)combined.push([a(7),b(7)]);
assert.deepEqual(combined.map(x=>x[0]),draw('qa',1,1).slice(0,20),'interleaving must not change worker stream');
const counts=[0,0,0,0];for(let i=1;i<=4000;i++)counts[stream('coverage',i,1)(4)]++;
for(const n of counts)assert.ok(n>850&&n<1150,`unbalanced synthetic seeded sample: ${counts}`);
console.log('PASS seeded replay, step isolation, worker interleaving, first-position coverage',counts);
