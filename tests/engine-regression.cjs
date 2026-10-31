const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),assert=require('node:assert/strict'),cp=require('node:child_process');
const root=path.resolve(__dirname,'..','src','engine');
const scriptFiles=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate'];
const source=scriptFiles.map(name=>fs.readFileSync(path.join(root,name+'.js'),'utf8')).join('\n');
const checked=source.replaceAll('PERSONA','{}').replaceAll('MEMORY','{}').replaceAll('__ANSWERED__','0');
const parse=cp.spawnSync(process.execPath,['--check','-'],{input:checked,encoding:'utf8'});
assert.equal(parse.status,0,parse.stderr||'combined script must parse');
let tests=1;
const analyze=fs.readFileSync(path.join(root,'analyze.js'),'utf8');
const header=fs.readFileSync(path.join(root,'header.js'),'utf8');
const detect=fs.readFileSync(path.join(root,'detect.js'),'utf8');
function context(code,defs={}){const state=vm.createContext({ ...defs,Set,Map });vm.runInContext(code,state);return state;}
const r1=analyze.substring(analyze.indexOf('function redColor('),analyze.indexOf('function findRedBoldInstructions('));
const red=context(r1);
for(const c of ['red','#ff0000','#f00','rgb(225, 18, 25)','rgba(200, 0, 0, 1)']){assert.equal(red.redColor(c),true,c);tests++;}
for(const c of ['rgb(0, 0, 0)','rgb(100, 1, 1)','#008000','#444444','transparent']){assert.equal(red.redColor(c),false,c);tests++;}
const a1=analyze.slice(0,analyze.indexOf('const markSel='));
const inputState=context(a1+'\nglobalThis.check=isOn;');
function fakeInput(type,checked,mark,aria){return{type,checked,matches(sel){return sel==='input'},getAttribute(key){return key==='data-ap-selected'?mark:key==='aria-checked'?aria:null}}}
assert.equal(inputState.check(fakeInput('radio',false,'1')),false,'unchecked native radio must not inherit stale marker');tests++;
assert.equal(inputState.check(fakeInput('checkbox',true,null)),true);tests++;
assert.equal(inputState.check(fakeInput('range',false,null)),false,'default range value is not an answer');tests++;
assert.equal(inputState.check(fakeInput('range',false,'1')),true);tests++;
const part=analyze.slice(analyze.indexOf('function splitRows('),analyze.indexOf('function ownerOf('));
const model=context(part);
const rootNode={parentElement:null},row1={parentElement:rootNode},row2={parentElement:rootNode};
const controls=[row1,row1,row2,row2].map(x=>({parentElement:{parentElement:x}}));
const rows=model.splitRows(controls);
assert.ok(rows&&rows[0]===rows[1]&&rows[2]===rows[3]&&rows[1]!==rows[2],'matrix rows must not collapse');tests++;
const listParent={parentElement:null};
const flatControls=Array.from({length:4},()=>({parentElement:{parentElement:listParent}}));
assert.equal(model.splitRows(flatControls),null,'ordinary four-choice radio list must not be split into rows');tests++;
const row3={parentElement:rootNode};
const grid3=[row1,row1,row2,row2,row3,row3].map(x=>({parentElement:{parentElement:x}}));
const gridNames=model.splitRows(grid3);
assert.ok(gridNames&&new Set(gridNames).size===3,'three-row matrix should split into three row groups');tests++;
const pattern=header.match(/const ATTENTION_RE=([^;]+);/)[1];
const attn=vm.runInNewContext(pattern);
assert.equal(attn.test('In which year were you born?'),false,'ordinary year question must not be blocked');tests++;
assert.equal(attn.test('Which option is most appropriate?'),false,'ordinary preference must not be blocked');tests++;
assert.equal(attn.test('Please select the second answer.'),true);tests++;
const local=analyze.substring(analyze.indexOf('function redInstructionFor('));
const func=local.match(/^function redInstructionFor\(q\)\{[\s\S]*?\n\}/)?.[0];
assert.ok(func,'local instruction matcher must exist');
const parentA={contains(e){return e==='a'}},parentB={contains(e){return e==='b'}};
const i=context(func,{questions:[{},{}],redBoldInstructions:[{el:'a',text:'Select the second option'}]});
const qa={els:[{closest(){return parentA}}]},qb={els:[{closest(){return parentB}}]};
assert.equal(i.redInstructionFor(qa).text,'Select the second option');tests++;
assert.equal(i.redInstructionFor(qb),null,'red instruction must not leak into another question');tests++;
const answeredFn=analyze.slice(analyze.indexOf('const qAnswered='),analyze.indexOf('\nconst qOpen='));
const chk=context(answeredFn+'\nglobalThis.qa=qAnswered;', {isOn(){return false}});
assert.equal(chk.qa({type:'text',els:[{value:'1',validity:{valid:false}}]}),false,'invalid numeric/text input must remain unanswered');tests++;
assert.equal(chk.qa({type:'text',els:[{value:'1',validity:{valid:true}}]}),true,'valid text input may be counted');tests++;
const qualExpr=detect.match(/const endQual=([^;]+);/)?.[1];
assert.ok(qualExpr,'qualification URL detection must exist');
for(const [url,expected] of [
 ['https://example.com/BCEndPages/EN-Qual.html',true],
 ['https://example.com/BCEndPages/Qual.html?x=1',true],
 ['https://example.com/BCEndPages/EN-Term.html',false],
 ['https://example.com/BCEndPages/EN-Qual.html-old',false],
 ['https://example.com/page?msg=thank%20you',false]
]){
 assert.equal(vm.runInNewContext(qualExpr,{location:{href:url}}),expected,url);tests++;
}
assert.match(detect,/const completion=!termination&&endQual;/,'completion must require qualification URL');tests++;
console.log('PASS '+tests+' engine syntax and regression assertions');
