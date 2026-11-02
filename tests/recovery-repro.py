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
 def mixedCards(pg,d):
  assert len(d['questions'])==1 and d['questions'][0]['n']==3,d
  assert pg.locator('[role=radio][aria-checked=true]').count()==1,d
  assert d['unanswered']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 run('mixed-image-text-radio-cards','''<div class="question"><h3>Which choice?</h3><div role="radiogroup"><div role="presentation"><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)"><img alt="Illustration" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==">A long description for a pictured choice</div></div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">Other option</div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">Unsure</div></div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button><script>function choose(e){document.querySelectorAll('[role=radio]').forEach(x=>x.setAttribute('aria-checked','false'));e.setAttribute('aria-checked','true')}</script>''',mixedCards)
 run('mixed-cards-with-question-owner','''<div class="question"><h3>Which choice?</h3><div role="presentation"><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)"><img alt="Illustration" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==">A long description for a pictured choice</div></div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">Other option</div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">Unsure</div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button><script>function choose(e){document.querySelectorAll('[role=radio]').forEach(x=>x.setAttribute('aria-checked','false'));e.setAttribute('aria-checked','true')}</script>''',mixedCards)
 def distinctGroups(pg,d):
  assert len(d['questions'])==2 and all(q['n']==2 for q in d['questions']),d
  assert pg.locator('[role=radio][aria-checked=true]').count()==2,d
 run('distinct-radio-groups-remain-separate','''<div class="question"><h3>Two separate questions</h3><div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">A</div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">B</div></div><div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">C</div><div role="radio" aria-checked="false" tabindex="0" onclick="choose(this)">D</div></div></div><button id="next">Next</button><script>function choose(e){e.closest('[role=radiogroup]').querySelectorAll('[role=radio]').forEach(x=>x.setAttribute('aria-checked','false'));e.setAttribute('aria-checked','true')}</script>''',distinctGroups)
 # Mirrors the actual observed hierarchy: IDs on the question, individual
 # answer wrappers and control labels; images hide text in descendant spans.
 def realShape(pg,d):
  assert len(d['questions'])==1 and d['questions'][0]['n']==5,d
  assert pg.locator('[role=radio][aria-checked=true]').count()==1,d
  assert d['unanswered']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 card="""<div class="cf-list__item"><div class="cf-button-answer" id="cqFake_{i}"><div id="cqFake_{i}_control" class="cf-button cf-button-answer__button" role="radio" aria-checked="false" tabindex="0" aria-labelledby="cqFake_{i}_text" onclick="choose(this)"><div class="cf-button-answer__text" id="cqFake_{i}_text">{content}</div></div></div></div>"""
 imgs=[f'<center><img width="50" height="30" alt="Synthetic item {i}" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw=="><span style="display:none">Synthetic item {i}</span></center>' for i in range(3)]
 cards=''.join(card.format(i=i,content=c) for i,c in enumerate(imgs+['Other answer','Unsure']))
 shape="""<div class="cf-question cf-question--single" id="cqFake"><div id="cqFake_text" class="cf-question__text">Which would you prefer?</div><div id="cqFake_instruction" class="cf-question__instruction">Choose one answer</div><div id="cqFake_content" class="cf-question__content"><div class="cf-list cf-list--narrow" role="radiogroup" aria-required="true">"""+cards+"""</div></div></div><button id="next" onclick="this.dataset.clicked=String(Number(this.dataset.clicked||0)+1)">Next</button><script>function choose(e){document.querySelectorAll('[role=radio]').forEach(x=>x.setAttribute('aria-checked','false'));e.setAttribute('aria-checked','true')}</script>"""
 run('observed-structural-card-shape',shape,realShape)
 def rowsOfCards(pg,d):
  assert len(d['questions'])==1 and d['questions'][0]['n']==4,d
  assert pg.locator('[role=radio][aria-checked=true]').count()==1,d
  assert d['unanswered']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 run('one-radio-group-with-visual-rows','''<div class="question"><h3>Choose one</h3><div role="radiogroup"><div class="row"><div role="radio" tabindex="0" aria-checked="false" onclick="choose(this)">A</div><div role="radio" tabindex="0" aria-checked="false" onclick="choose(this)">B</div></div><div class="row"><div role="radio" tabindex="0" aria-checked="false" onclick="choose(this)">C</div><div role="radio" tabindex="0" aria-checked="false" onclick="choose(this)">D</div></div></div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button><script>function choose(e){document.querySelectorAll('[role=radio]').forEach(x=>x.setAttribute('aria-checked','false'));e.setAttribute('aria-checked','true')}</script>''',rowsOfCards)
 def separateUnanswered(pg,d):
  assert len(d['questions'])==2 and d['unanswered']>=1,d
  assert not pg.locator('#next').get_attribute('data-clicked'),d
 run('separate-unanswered-group-blocks-next','''<div class="question"><h3>Two independent selections</h3><div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0" onclick="this.setAttribute('aria-checked','true')">A</div><div role="radio" aria-checked="false" tabindex="0" onclick="this.setAttribute('aria-checked','true')">B</div></div><div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0">C</div><div role="radio" aria-checked="false" tabindex="0">D</div></div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button>''',separateUnanswered)
 def cardChecks(pg,d):
  checks=pg.locator('[role=checkbox]')
  assert len(d['questions'])==1 and d['questions'][0]['n']==6,d
  assert pg.locator('[role=checkbox][aria-checked=true]').count()>=1,d
  assert checks.nth(5).get_attribute('aria-checked')=='false',d
  assert d['unanswered']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 cards=''.join('<div class="cf-list__item"><div id="cqFake_'+str(i)+'"><div role="checkbox" tabindex="0" aria-checked="false" aria-label="'+('Nessuna di queste risposte' if i==5 else 'Option '+str(i))+'" onclick="toggle(this)">'+('None' if i==5 else '<img width="24" height="24" alt="Option" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw=="><span style="display:none">Option '+str(i)+'</span>')+'</div></div></div>' for i in range(6))
 run('checkbox-cards-are-one-question','<div class="cf-question" id="cqFake"><h3>Choose all that apply</h3><div role="group">'+cards+'''</div></div><button id="next" onclick="this.dataset.clicked='1'">Next</button><script>function toggle(e){e.setAttribute('aria-checked',e.getAttribute('aria-checked')==='true'?'false':'true')}</script>''',cardChecks)
 def dropdown(pg,d):
  assert pg.locator('select').input_value()!='placeholder' and d['unanswered']==0
 run('rejects-placeholder-nonempty-value','''<div class="question"><h3>Which category?</h3><select name="category" required><option value="placeholder">Please select an option</option><option value="one">First</option><option value="two">Second</option></select></div><button id="next" onclick="this.dataset.clicked=String(Number(this.dataset.clicked||0)+1)">Next</button>''',dropdown)
 def excludedOrdinal(pg,d):
  assert pg.get_by_label('Beta').is_checked() and not pg.get_by_label('Alpha').is_checked()
  assert d['instructions']['blocked']==0
 run('red-ordinal-exclusion','''<div class="question"><strong class="red">Select the second option, not the first</strong><label><input type="radio" name="choice">Alpha</label><label><input type="radio" name="choice">Beta</label></div><button id="next">Next</button>''',excludedOrdinal)
 b.close()
