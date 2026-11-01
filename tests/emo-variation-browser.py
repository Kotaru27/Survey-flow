from playwright.sync_api import sync_playwright
from pathlib import Path
import json
r=Path(__file__).resolve().parents[1]
engine='\n'.join((r/'src'/'engine'/f'{n}.js').read_text() for n in ['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate'])
engine=engine.replace('const RAW=PERSONA;', 'const RAW={"Age":30,"MultiSelectCount":2,"AnswerMode":"persona"};').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
html='''<style>body{font:18px Arial}.answer-container.hidden{display:none}.answer-container{width:600px;height:120px}.slider{width:600px;height:80px}.grid{position:absolute;width:0;height:0;overflow:hidden}</style>
<div class="cf-question"><div>Move your circle away from or over the brand circle.</div><div class="cf-question__content"><div class="grid" id="grid"></div><div class="sliders-container" id="boxes"></div></div></div><button class="fake-next-button" id="next">Next</button>
<script>
for(let i=0;i<5;i++){
 const g=document.createElement('div');g.setAttribute('role','radiogroup');g.setAttribute('aria-required','true');
 for(let n=1;n<=100;n++){const c=document.createElement('div');c.setAttribute('role','radio');c.setAttribute('aria-checked','false');c.setAttribute('aria-label','Item '+i+' '+n);g.append(c)}document.querySelector('#grid').append(g);
 const b=document.createElement('div');b.className='answer-container '+(i?'hidden':'visible');b.innerHTML='<input class="slider" type="range" min="1" max="100" value="50"><div class="circle-answer">Item '+i+'</div>';
 const h=b.querySelector('input');h.addEventListener('input',()=>{if(window.reject)return;b.classList.add('answered');const cs=g.querySelectorAll('[role=radio]');cs.forEach((c,j)=>c.setAttribute('aria-checked',String(j+1===Number(h.value))))});document.querySelector('#boxes').append(b)
}
document.querySelector('#next').addEventListener('click',()=>{const a=[...document.querySelectorAll('.answer-container')],i=a.findIndex(e=>e.classList.contains('visible'));if(!a[i]?.classList.contains('answered')){document.body.dataset.invalid='1';return}if(i===a.length-1){document.body.dataset.done='1';return}a[i].classList.replace('visible','hidden');a[i+1].classList.replace('hidden','visible')});
</script>'''
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage']);page=b.new_page()
 page.set_content(html)
 for i in range(5):
  d=page.evaluate(engine)
  assert 'circle-score' in d['action'] and d['unanswered']==0,(i,d)
  assert page.locator('.answer-container.answered').count()==i+1,(i,d)
  if i<4: assert page.locator('.answer-container.visible').count()==1
  page.evaluate('delete window.__apNavIntent')
 scores=page.locator('.answer-container input').evaluate_all('(xs)=>xs.map(x=>Number(x.value))')
 assert len(set(scores))==5 and page.locator('body').get_attribute('data-done')=='1',scores
 assert page.locator('[role=radio][aria-checked=true]').count()==5
 print('PASS five distinct confirmed circle scores and navigation')
 page.set_content(html);page.evaluate('window.reject=true');d=page.evaluate(engine)
 assert d['unanswered']>0 and page.locator('body').get_attribute('data-done') is None and page.locator('.answer-container.answered').count()==0,d
 print('PASS rejected circle update blocks navigation')
 page.set_content(html);page.evaluate('''() => {const x=[...document.querySelectorAll('.slider')];for(let i=0;i<2;i++){x[i].value='50';x[i].dispatchEvent(new Event('input',{bubbles:true}));}document.querySelectorAll('.answer-container')[0].classList.replace('visible','hidden');document.querySelectorAll('.answer-container')[2].classList.replace('hidden','visible');delete window.__apNavIntent}''');d=page.evaluate(engine)
 assert d['unanswered']>0 and d['instructions']['blocked']>0 and page.locator('.answer-container.answered').count()==2,d
 print('PASS repeated prior scores block navigation')
 b.close()
