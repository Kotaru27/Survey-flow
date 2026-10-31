"""Synthetic tests of a numeric widget backed by a hidden answer field."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{n}.js').read_text() for n in names)
engine=engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':2,'AnswerMode':'persona','Number':30})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
def row(start,end,working=True):
 cells=''.join('<td onclick="%s"><span>%s</span></td>' % ("this.classList.add('selected');this.closest('.question').dataset.answer=this.textContent" if working else '',i) for i in range(start,end+1))
 return '<div class="question"><table><tr>'+cells+'</tr></table></div>'
def page_html(rows):
 return '<style>td{padding:12px;border:1px solid #ccc;cursor:pointer}.selected{background:blue;color:white}</style><div class="cf-question"><h3>How likely are you to recommend the item?</h3><div id="ISD">'+rows+'</div></div><button id="next" onclick="this.dataset.clicked=\'1\'">Next</button>'
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 page=b.new_page();page.set_content(page_html(row(0,10)));r=page.evaluate(engine)
 assert page.locator('#ISD .selected').count()==1,r
 assert page.locator('#next').get_attribute('data-clicked')=='1',r
 assert 'numeric-scale' in r['action'],r
 print('PASS numeric scale registers a selected value and continues')
 page=b.new_page();page.set_content(page_html(row(1,5)+row(1,5)));r=page.evaluate(engine)
 assert page.locator('#ISD .selected').count()==2,r
 assert page.locator('#next').get_attribute('data-clicked')=='1',r
 print('PASS multiple numeric rows are answered before continuing')
 page=b.new_page();page.set_content(page_html(row(0,10,False)));r=page.evaluate(engine)
 assert page.locator('#ISD .selected').count()==0,r
 assert page.locator('#next').get_attribute('data-clicked') is None,r
 assert r['instructions']['blocked']>0,r
 print('PASS ignored clicks are held for review')
 page=b.new_page();page.set_content(page_html(row(0,4).replace('<td onclick=', '<td onclick=',1).replace('<span>3</span>','<span>7</span>')));r=page.evaluate(engine)
 assert page.locator('#next').get_attribute('data-clicked') is None,r
 print('PASS ambiguous numeric range is held for review')
 b.close()
