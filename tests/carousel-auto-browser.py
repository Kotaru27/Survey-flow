"""Synthetic auto-advancing carousel regression; no survey content."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

ROOT=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((ROOT/'src'/'engine'/f'{name}.js').read_text() for name in names)
engine=engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':2,'AnswerMode':'persona','YesNo':'Yes'})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
def slide(i):
 return f"""<div class="cf-carousel__content-item" aria-hidden="{'false' if i==0 else 'true'}"><div role="radiogroup">
 <div role="radio" aria-checked="false" aria-label="Item {i} Yes" onclick="answer(this)">Yes</div>
 <div role="radio" aria-checked="false" aria-label="Item {i} No" onclick="answer(this)">No</div>
 </div></div>"""
html='''<style>[aria-hidden=true]{display:none}[role=radio]{padding:10px;cursor:pointer}font{color:red}</style>
<div class="cf-question cf-question--carousel-grid"><p>Choose a response for each item. <font color="red">we need to select mix of yes and no</font></p><div class="cf-carousel">'''+''.join(slide(i) for i in range(4))+'''</div></div><button id=next onclick="this.dataset.clicked='1'">Next</button>
<script>function answer(e){let s=e.closest('.cf-carousel__content-item');e.setAttribute('aria-checked','true');let n=s.nextElementSibling;if(n){s.setAttribute('aria-hidden','true');n.setAttribute('aria-hidden','false')}}</script>'''
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 page=b.new_page();page.set_content(html)
 for i in range(10):
  result=page.evaluate(engine)
  count=page.locator('[role=radio][aria-checked=true]').count()
  if count<4: assert page.locator('#next').get_attribute('data-clicked') is None, result
  if page.locator('#next').get_attribute('data-clicked')=='1':break
 assert page.locator('[role=radio][aria-checked=true]').count()==4,result
 chosen=page.locator('[role=radio][aria-checked=true]').all_text_contents()
 assert 'Yes' in chosen and 'No' in chosen,chosen
 assert page.locator('#next').get_attribute('data-clicked')=='1',result
 print('PASS auto-advancing carousel answered every card, mixed responses, then clicked page Next',chosen)
 page.close();page=b.new_page()
 longhtml=html.replace(''.join(slide(i) for i in range(4)),''.join(slide(i) for i in range(23)))
 page.set_content(longhtml)
 for _ in range(35):
  result=page.evaluate(engine)
  n=page.locator('[role=radio][aria-checked=true]').count()
  if n<23: assert page.locator('#next').get_attribute('data-clicked') is None,result
  if page.locator('#next').get_attribute('data-clicked')=='1': break
 assert page.locator('[role=radio][aria-checked=true]').count()==23,result
 assert page.locator('#next').get_attribute('data-clicked')=='1',result
 print('PASS 23-card carousel answered all cards before page Next')
 seen=set()
 for trial in range(12):
  page.close();page=b.new_page()
  page.set_content(html.replace('<font color="red">we need to select mix of yes and no</font>',''))
  for _ in range(8):
   result=page.evaluate(engine)
   if page.locator('#next').get_attribute('data-clicked')=='1':break
  assert page.locator('[role=radio][aria-checked=true]').count()==4,result
  assert page.locator('#next').get_attribute('data-clicked')=='1',result
  seen.update(page.locator('[role=radio][aria-checked=true]').all_text_contents())
 assert seen=={'Yes','No'},seen
 print('PASS carousel without a mix instruction does not always use the persona choice',seen)
 b.close()
