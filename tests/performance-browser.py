"""Compare equivalent page scans on a synthetic busy survey page."""
from pathlib import Path
import json,re,time,statistics
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{n}.js').read_text() for n in names)
engine=engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':1,'AnswerMode':'persona','OpenText':'Example','Number':30})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
old="const contexts=[...new Set(native.map(questionText).filter(t=>t.length>3))].slice(0,10);\nlet q=contexts.join(' | ');"
before,count=re.subn(r'// The output contains at most ten distinct contexts\.[\s\S]*?let q=contexts\.join\(\x27 \| \x27\);',lambda _:old,engine,count=1)
assert count==1
# Decorative areas have different question containers and enough content to expose
# repeated text extraction. They contain no private survey content.
decor=''.join(f'<div class="question">Decoration {i} unrelated layout content</div>' for i in range(850))
html='<div class="question"><h3>Choose a synthetic answer?</h3><label><input type=radio name=pick value=a>Alpha</label></div>'+decor+'<button id=next onclick="this.dataset.clicked=\'yes\'">Next</button>'
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 pg=b.new_page(viewport={'width':1024,'height':768})
 results={}
 for name,code in [('baseline',before),('optimized',engine)]:
  samples=[]
  for _ in range(4):
   pg.set_content(html)
   start=time.perf_counter();d=pg.evaluate(code);samples.append((time.perf_counter()-start)*1000)
   assert pg.locator('input[name=pick]').is_checked(),d
   assert pg.locator('#next').get_attribute('data-clicked')=='yes',d
  results[name]=statistics.median(samples)
 print('PASS equivalent synthetic page selected the same answer and continued')
 print('TIMING synthetic median ms:', {k:round(v,1) for k,v in results.items()})
 b.close()
