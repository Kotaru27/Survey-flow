"""Synthetic cloud-browser evidence. No live survey URLs or respondent data."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'test-evidence'
OUT.mkdir(exist_ok=True)
NAMES=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
ENGINE='\n'.join((ROOT/'src'/'engine'/f'{name}.js').read_text(encoding='utf-8') for name in NAMES)
STYLE='''<style>body{font:20px Arial;margin:40px;max-width:720px}.question{padding:24px;border:1px solid #ccd7e2;border-radius:12px}label{display:block;padding:12px}button{margin-top:20px;padding:12px 28px}.error{color:#b00020;font-weight:bold;padding:14px 0}#result{padding:16px;color:#146c43;font-weight:bold}.meta{font-size:15px;color:#555}</style>'''
PERSONA={'Gender':'Female','Age':30,'MultiSelectCount':1,'OpenText':'Test','Number':30,'AnswerMode':'persona'}
def script(memory=None):
 return ENGINE.replace('const RAW=PERSONA;', 'const RAW='+json.dumps(PERSONA)+';').replace('const MEM=MEMORY;', 'const MEM='+json.dumps(memory or {})+';').replace('const answered=__ANSWERED__;', 'const answered=0;')
def setpage(pg,body):pg.set_content('<html><head>'+STYLE+'</head><body>'+body+'</body></html>')
validation='''<h2>Validation recovery — synthetic test</h2><div class=question><h3>Pick a product</h3><label><input type=radio name=product value=a checked>Alpha</label><label><input type=radio name=product value=b>Beta</label><div class=error id=error hidden>Alpha is not allowed. Please choose another option.</div><div id=result hidden></div></div><button id=next onclick="this.dataset.count=String(1+Number(this.dataset.count||0));if(document.querySelector('input[name=product]:checked').value==='a'){document.querySelector('#error').hidden=false}else{document.querySelector('#error').hidden=true;let r=document.querySelector('#result');r.hidden=false;r.textContent='Accepted: Beta';}">Next</button><p class=meta id=count>Test uses synthetic options only.</p>'''
terminal='''<h2>Termination memory — synthetic test</h2><div class=question><h3>Select one option</h3><label><input type=radio name=product value=a>Alpha</label><label><input type=radio name=product value=b>Beta</label><div id=result hidden></div></div><button id=next onclick="let v=document.querySelector('input[name=product]:checked')?.value;let r=document.querySelector('#result');r.hidden=false;r.textContent=v==='a'?'Terminated after Alpha':'Accepted: Beta';">Next</button><p class=meta>Test uses synthetic options only.</p>'''
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 page=b.new_page(viewport={'width':1100,'height':760},device_scale_factor=1)
 setpage(page,validation);page.locator('#next').click();assert page.locator('#error').is_visible();page.screenshot(path=str(OUT/'01-validation-before.png'))
 d=page.evaluate(script());assert page.get_by_label('Beta').is_checked(),d;assert page.locator('#result').inner_text()=='Accepted: Beta',d;assert page.locator('#next').get_attribute('data-count')=='2',d;page.screenshot(path=str(OUT/'02-validation-after.png'))
 print('PASS validation changed Alpha to Beta and only submitted after change')
 page.close();page=b.new_page(viewport={'width':1100,'height':760},device_scale_factor=1)
 minimum='''<div class="question"><h3>Choose items</h3><label><input type=checkbox name=items checked>Alpha</label><label><input type=checkbox name=items>Beta</label><div class=error id=error>Please select at least two options.</div><div id=result hidden></div></div><button id=next onclick="this.dataset.count=String(1+Number(this.dataset.count||0));if(document.querySelectorAll('input[name=items]:checked').length>=2){document.querySelector('#error').hidden=true;let r=document.querySelector('#result');r.hidden=false;r.textContent='Accepted'}">Next</button>'''
 setpage(page,minimum);d=page.evaluate(script());assert page.get_by_label('Alpha').is_checked() and page.get_by_label('Beta').is_checked(),d;assert page.locator('#result').inner_text()=='Accepted',d;assert page.locator('#next').get_attribute('data-count')=='1',d
 print('PASS minimum-count validation added a choice and continued on the same page')
 page.close();page=b.new_page(viewport={'width':1100,'height':760},device_scale_factor=1)
 setpage(page,terminal);page.get_by_label('Alpha').check();d=page.evaluate(script());assert page.locator('#result').inner_text()=='Terminated after Alpha',d
 pairs=d.get('submittedPairs');assert len(pairs)==1 and pairs[0]['option']=='alpha',d;page.screenshot(path=str(OUT/'03-termination-first-run.png'))
 page.close();page=b.new_page(viewport={'width':1100,'height':760},device_scale_factor=1)
 setpage(page,terminal);rule={'question':pairs[0]['question'],'option':pairs[0]['option'],'reason':'screenout-suspect'};d=page.evaluate(script({'avoid':[rule]}));assert page.get_by_label('Beta').is_checked(),d;assert page.locator('#result').inner_text()=='Accepted: Beta',d;page.screenshot(path=str(OUT/'04-termination-next-run.png'))
 print('PASS next synthetic run avoided the prior submitted option when fed the recorded memory rule')
 page.close();page=b.new_page(viewport={'width':1100,'height':760},device_scale_factor=1)
 setpage(page,validation);page.locator('#next').click();page.locator('label').nth(1).evaluate('(e)=>e.remove()');d=page.evaluate(script());assert page.locator('#next').get_attribute('data-count')=='1',d;assert d['instructions']['blocked']>0,d
 print('PASS validation without an alternative did not repeatedly click Next')
 b.close()
