"""Synthetic attention-check rules; no study text or survey data."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
source='\n'.join((root/'src'/'engine'/f'{n}.js').read_text(encoding='utf-8') for n in names)
source=source.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':1,'AnswerMode':'random'})+';').replace('const answered=__ANSWERED__;', 'const answered=0;')
prompt='Synthetic verification: select the compatible items from this list.'
rule={'contains':prompt,'kind':'multi_exact','strict':True,'answers':['Alpha','Beta','Gamma','Delta'],'never':['One','Two','Three']}
engine=source.replace('const MEM=MEMORY;', 'const MEM='+json.dumps({'redHerring':[rule]})+';')
def html(options,checked=None,role=False,title=None):
 items=[]
 for label in options:
  if role:items.append('<div role="checkbox" aria-checked="%s" onclick="this.setAttribute(\'aria-checked\',this.getAttribute(\'aria-checked\')===\'true\'?\'false\':\'true\')">%s</div>'%('true' if label==checked else 'false',label))
  else:items.append('<label><input type=checkbox name=items %s>%s</label>'%('checked' if label==checked else '',label))
 heading='<h3>'+prompt+'</h3>' if not title else '<h3>'+title+'</h3><p>'+prompt+'</p>'
 return '<style>label,[role=checkbox]{display:block;padding:10px}</style><div class=question>'+heading+''.join(items)+'</div><button id=next onclick="this.dataset.count=String(1+Number(this.dataset.count||0))">Next</button>'
opts=['Three','Delta','Two','Alpha','Gamma','One','Beta']
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 for role in [False,True]:
  page=b.new_page();page.set_content(html(opts,'One',role));d=page.evaluate(engine)
  if role:
   selected=page.locator('[role=checkbox][aria-checked=true]').all_text_contents()
  else:
   selected=page.locator('label').filter(has=page.locator('input:checked')).all_text_contents()
  assert set(x.strip() for x in selected)==set(rule['answers']),d
  assert page.locator('#next').get_attribute('data-count')=='1',d
  print('PASS exact multi-check corrects bad selection and selects all safe answers', 'role' if role else 'native')
  page.close()
 page=b.new_page();page.set_content(html(opts[:-1]));d=page.evaluate(engine)
 assert page.locator('input:checked').count()==0 and page.locator('#next').get_attribute('data-count') is None,d
 assert d['instructions']['blocked']>0,d
 print('PASS incomplete answer inventory holds instead of guessing')
 page.close();page=b.new_page();page.set_content(html(opts,title='Internal heading'));d=page.evaluate(engine)
 assert page.locator('input:checked').count()==4 and page.locator('#next').get_attribute('data-count')=='1',d
 print('PASS visible prompt is matched when a separate heading is present')
 page.close();page=b.new_page();page.set_content(html(opts).replace(prompt,'A different synthetic question.'));d=page.evaluate(engine)
 assert page.locator('input:checked').count()!=4,d
 print('PASS unrelated question is not treated as the keyed check')
 b.close()
