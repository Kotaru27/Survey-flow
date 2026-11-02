"""Synthetic grid batching, fallback and timing tests."""
from pathlib import Path
import json,time
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{n}.js').read_text() for n in names)
engine=engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':2,'AnswerMode':'random'})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
assert 'const gridLimit=gridCanBatch?8:1;' in engine
baseline=engine.replace('const gridLimit=gridCanBatch?8:1;','const gridLimit=1;',1)
def row(i,kind='radio'):
 return '<div class="row" data-row="%s">'%i+''.join('<label><input type="%s" onclick="choose(this)">%s</label>'%(kind,x) for x in ('Low','Mid','High'))+'</div>'
def html(kind='radio',extra='',script=''):
 return '<style>label{display:inline-block;padding:5px}.row{padding:5px}</style><div class=question><h3>Rate each item</h3>'+extra+''.join(row(i,kind) for i in range(12))+'</div><button id=next onclick="this.dataset.clicked=\'yes\'">Next</button><script>function choose(e){if(e.type===\'radio\')for(const x of e.closest(\'.row\').querySelectorAll(\'input\'))if(x!==e)x.checked=false;'+script+'}</script>'
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage']);page=b.new_page()
 timings={}
 for name,code in [('baseline',baseline),('optimized',engine)]:
  page.set_content(html());page.evaluate('delete window.__apNavIntent');start=time.perf_counter();calls=0
  while calls<20:
   d=page.evaluate(code);calls+=1
   if page.locator('#next').get_attribute('data-clicked')=='yes':break
  rows=page.locator('.row').count();filled=page.locator('.row input:checked').count()
  assert rows==12 and filled==12 and page.locator('#next').get_attribute('data-clicked')=='yes',d
  timings[name]=(calls,round((time.perf_counter()-start)*1000))
 assert timings['optimized'][0]<timings['baseline'][0],timings
 print('PASS single grid: all rows answered; passes, elapsed ms:',timings)
 page.set_content(html('checkbox'));page.evaluate('delete window.__apNavIntent')
 for i in range(20):
  d=page.evaluate(engine)
  if page.locator('#next').get_attribute('data-clicked')=='yes':break
 assert page.locator('.row').count()==12 and page.locator('.row').filter(has=page.locator('input:checked')).count()==12,d
 print('PASS multi grid: every row has a selection')
 for instruction,relation in [('Select the same option for every row','same'),('Select a different option from the previous row','different')]:
  page.set_content(html(extra='<p>'+instruction+'</p>'));page.evaluate('delete window.__apNavIntent')
  for i in range(20):
   d=page.evaluate(engine)
   if page.locator('#next').get_attribute('data-clicked')=='yes':break
  values=page.locator('.row').evaluate_all('(rows)=>rows.map(r=>Array.from(r.querySelectorAll("input")).findIndex(x=>x.checked))')
  assert len(values)==12 and all(v>=0 for v in values) and page.locator('#next').get_attribute('data-clicked')=='yes',(relation,values,d)
  if relation=='same':assert len(set(values))==1,(relation,values,d)
  else:assert all(a!=b for a,b in zip(values,values[1:])),(relation,values,d)
  print('PASS explicit grid relation and navigation:',relation)
 page.set_content(html(extra='<p>Select the same option for every row</p>'));page.evaluate('delete window.__apNavIntent')
 page.locator('.row').nth(0).locator('input').nth(0).check();page.locator('.row').nth(1).locator('input').nth(1).check()
 d=page.evaluate(engine)
 assert page.locator('#next').get_attribute('data-clicked') is None and any(x.get('reason')=='grid-relation-existing-conflict' for x in d['instructions']['details'] if isinstance(x,dict)),d
 print('PASS conflicting existing grid answers block submission')
 page.set_content(html(extra='<strong style="color:red">Select the same option for every row</strong>'));page.evaluate('delete window.__apNavIntent')
 for i in range(20):
  d=page.evaluate(engine)
  if page.locator('#next').get_attribute('data-clicked')=='yes':break
 values=page.locator('.row').evaluate_all('(rows)=>rows.map(r=>Array.from(r.querySelectorAll("input")).findIndex(x=>x.checked))')
 assert all(v>=0 for v in values) and len(set(values))==1 and page.locator('#next').get_attribute('data-clicked')=='yes',(values,d)
 print('PASS red grid consistency instruction and navigation')
 page.set_content(html(extra='<p>Select the same option for every row</p>'));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 # selectedPairs carries the full key; the diagnostic questions list truncates it.
 keys=[x['question'] for x in d['selectedPairs']]
 assert len(keys)>=2,d
 page.set_content(html(extra='<p>Select the same option for every row</p>'));page.evaluate('delete window.__apNavIntent')
 memory={'avoid':[{'question':keys[1],'option':x} for x in ('Low','Mid','High')]}
 constrained=engine.replace('const MEM={};','const MEM='+json.dumps(memory)+';',1)
 d=page.evaluate(constrained)
 assert page.locator('#next').get_attribute('data-clicked') is None and page.locator('input:checked').count()==0 and d['instructions']['blocked']>0,d
 print('PASS no common memory-safe grid answer blocks submission')
 page.set_content(html(extra='<strong style="color:red">Please select one answer per row</strong>'));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 assert page.locator('input:checked').count()==1 and page.locator('#next').get_attribute('data-clicked') is None,d
 print('PASS instruction-bearing grid uses one-row path')
 page.set_content(html(script="if(e.closest('.row').dataset.row==='0')queueMicrotask(()=>document.querySelector('[data-row=1]').remove())"));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 assert page.locator('#next').get_attribute('data-clicked') is None,d
 print('PASS changing grid cannot submit early')
 page.set_content(html(script="if(e.closest('.row').dataset.row==='0')queueMicrotask(()=>{let x=document.createElement('input');x.required=true;document.querySelector('.question').appendChild(x)})"));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 assert page.locator('input[required]').count()==1 and page.locator('#next').get_attribute('data-clicked') is None,d
 assert 'render-waiting' in d['action'],d
 print('PASS newly mounted required input blocks navigation')
 page.set_content(html(script="if(e.closest('.row').dataset.row==='0')queueMicrotask(()=>{let x=document.createElement('div');x.className='error';x.textContent='Invalid selection';document.querySelector('.question').appendChild(x)})"));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 assert page.locator('.error').is_visible() and page.locator('#next').get_attribute('data-clicked') is None,d
 assert d['instructions']['blocked']>0,d
 print('PASS validation appearing during a grid batch blocks navigation')
 page.set_content(html(script="if(e.closest('.row').dataset.row==='0')queueMicrotask(()=>{let x=document.createElement('strong');x.style.color='red';x.textContent='Please select the second answer';document.querySelector('.question').appendChild(x)})"));page.evaluate('delete window.__apNavIntent');d=page.evaluate(engine)
 assert page.locator('#next').get_attribute('data-clicked') is None and page.locator('input:checked').count()==1,d
 assert 'render-waiting' in d['action'],d
 print('PASS a newly revealed red instruction stops grid batching')
 b.close()
