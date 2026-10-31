"""Negative browser checks: refuse invalid state; honor explicit red targets."""
from playwright.sync_api import sync_playwright
from pathlib import Path
import json
r=Path(__file__).resolve().parents[1]
script='\n'.join((r/'src'/'engine'/f'{n}.js').read_text() for n in ['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate'])
script=script.replace('const RAW=PERSONA;', 'const RAW={"Gender":"Female","Age":30,"MultiSelectCount":2,"OpenText":"QA synthetic","AnswerMode":"persona"};').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 def run(name,html,assertion):
  pg=b.new_page()
  pg.set_content('<style>body{font:18px Arial}.question{margin:20px}.red{color:#d00000}</style>'+html)
  try:
   data=pg.evaluate(script)
   assertion(pg,data)
   print('PASS',name)
  except AssertionError:
   print('FAIL',name,'action',data.get('action'),'unanswered',data.get('unanswered'),'pairs',data.get('selectedPairs'),'instructions',data.get('instructions'))
   raise
  finally:pg.close()
 def ignoredRadio(pg,d):
  assert d['unanswered']>0 and not pg.locator('#next').get_attribute('data-clicked')
  assert d['selectedPairs']==[], 'rejected choices must not enter the memory/report as successful answers'
 run('rejects-ignored-custom-radio','''<div class="question"><h3>Pick an option</h3><div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0" onclick="this.setAttribute('aria-checked','false')">Alpha</div><div role="radio" aria-checked="false" tabindex="0" onclick="this.setAttribute('aria-checked','false')">Beta</div></div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button>''',ignoredRadio)
 def dropdown(pg,d):
  assert pg.locator('select').input_value()!='placeholder' and d['unanswered']==0
 run('rejects-placeholder-nonempty-value','''<div class="question"><h3>Which category?</h3><select name="category" required><option value="placeholder">Please select an option</option><option value="one">First</option><option value="two">Second</option></select></div><button id="next" onclick="this.dataset.clicked=String(Number(this.dataset.clicked||0)+1)">Next</button>''',dropdown)
 def excludedOrdinal(pg,d):
  assert pg.get_by_label('Beta').is_checked() and not pg.get_by_label('Alpha').is_checked()
  assert d['instructions']['blocked']==0
 run('red-ordinal-exclusion','''<div class="question"><strong class="red">Select the second option, not the first</strong><label><input type="radio" name="choice">Alpha</label><label><input type="radio" name="choice">Beta</label></div><button id="next">Next</button>''',excludedOrdinal)
 b.close()
