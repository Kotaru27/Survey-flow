"""Representative DOM fixtures: run only with a cloud-installed Chromium and Python Playwright.
These are synthetic controls, not claims about the unknown live survey's HTML."""
from playwright.sync_api import sync_playwright
from pathlib import Path
import json, traceback
r=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
script='\n'.join((r/'src'/'engine'/f'{n}.js').read_text() for n in names)
script=script.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'Gender':'Female','Age':30,'MultiSelectCount':2,'OpenText':'Test response','AnswerMode':'persona'})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
STYLE='''<style>body{font:16px Arial;padding:30px}.question{margin:20px 0}.accordion-header{cursor:pointer;padding:12px;border:1px solid #ddd;display:block;background:#fafafa;width:320px}.panel{padding:10px;margin:0 5px}.panel[hidden]{display:none}label{display:block; padding:6px;}label input{width:18px;height:18px;margin-right:8px}strong.red{color:#e00000}</style>'''
def checkbox(t,name='p'):return f'<label><input type="checkbox" name="{name}">{t}</label>'
def radio(t,name='p'):return f'<label><input type="radio" name="{name}">{t}</label>'
def accordion(title,inner,opened=False):
  exp='true' if opened else 'false'; hide='' if opened else 'hidden'
  return f'''<section class="accordion-item"><button type="button" class="accordion-header" aria-expanded="{exp}" onclick="let p=this.nextElementSibling;let show=this.getAttribute('aria-expanded')==='false';this.setAttribute('aria-expanded',String(show));p.hidden=!show">{title}</button><div class="panel" {hide}>{inner}</div></section>'''
def pageWrap(inner):return '<!doctype html><html><head>'+STYLE+'</head><body><form><div class="question"><h3 class="question-title">Which products have you used?</h3>'+inner+'</div><button type="button" id="next" onclick="this.dataset.clicked=String(+(this.dataset.clicked||0)+1)">Next</button></form></body></html>'
checks=0

def step(page):
  data=page.evaluate(script)
  assert isinstance(data,dict), data
  return data

with sync_playwright() as p:
 b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 def run(name,html,validate):
  global checks
  page=b.new_page()
  page.set_content(html)
  try:
   validate(page)
   print('PASS',name)
   checks+=1
  finally:page.close()
 def accMulti(pg):
  a=step(pg);print('acc step1',a['action'],a['unanswered'],a.get('accordion'),a.get('openQuestions'))
  assert 'accordion-expand' in a['action'] and a['unanswered']>0,a
  assert pg.locator('.accordion-header').first.get_attribute('aria-expanded')=='true'
  # Continue until target count met, without ever submitting an unanswered panel.
  for i in range(6):
   d=step(pg);print('acc step',i+2,d['action'],d['unanswered'],d.get('accordion'))
   if d['unanswered']==0:break
  assert sum(pg.locator('input:checked').count() for _ in [0])>=2
  assert pg.locator('#next').get_attribute('data-clicked') in ['1','2','3'], d
 run('accordion-multi',pageWrap(accordion('Laundry Products',checkbox('Liquid detergent')+checkbox('Detergent Bars')+checkbox('Powder detergent'))+accordion('Hair Care Products',checkbox('Shampoo')+checkbox('Conditioner'))+accordion('Skincare Products',checkbox('Face cream'))+radio('None of the above','none')) , accMulti)
 def structuredAccordion(pg):
  last=None
  for i in range(20):
   last=step(pg)
   if pg.locator('#next').get_attribute('data-clicked')=='1':break
   pg.wait_for_timeout(150)
  assert pg.locator('.accordion-header[aria-expanded=true]').count()<=1,last
  assert pg.locator('[data-choice]:checked').count()>=1,last
  assert pg.locator('#next').get_attribute('data-clicked')=='1',last
  assert last['instructions']['blocked']==0,last
  (r/'test-evidence').mkdir(exist_ok=True)
  pg.screenshot(path=str(r/'test-evidence'/'accordion-delayed-controls.png'))
 def structuredGroup(index,size):
  return f'''<section class="accordion-item"><button type="button" class="accordion-header" aria-controls="panel{index}" aria-expanded="false" onclick="openGroup({index},{size})">Group {index}</button><div class="panel" id="panel{index}" hidden></div></section>'''
 structuredScript='''<script>function openGroup(i,n){document.querySelectorAll('.panel').forEach(x=>x.hidden=true);document.querySelectorAll('.accordion-header').forEach(x=>x.setAttribute('aria-expanded','false'));let p=document.querySelector('#panel'+i), h=document.querySelector('[aria-controls=panel'+i+']');h.setAttribute('aria-expanded','true');p.hidden=false;if(!p.dataset.loaded){p.dataset.loaded='1';setTimeout(()=>{for(let j=0;j<n;j++){let l=document.createElement('label');l.innerHTML='<input type=checkbox name=items data-choice>Item '+i+'-'+j;p.appendChild(l)}},75)}}</script>'''
 run('accordion-many-groups-single-open-delayed-controls',pageWrap('<p>Select all that apply</p>'+''.join(structuredGroup(i,n) for i,n in enumerate([4,6,3,8,5],1))+radio('None of the above','none')+structuredScript),structuredAccordion)
 def duplicateHeaders(pg):
  d=None
  for _ in range(12):
   d=step(pg)
   if pg.locator('#next').get_attribute('data-clicked')=='1':break
  assert pg.locator('input:checked').count()==2,d
  assert pg.locator('#next').get_attribute('data-clicked')=='1',d
 sameGroup=lambda label: f'''<div><div class="group-title" onclick="this.nextElementSibling.hidden=!this.nextElementSibling.hidden">Group</div><div class="panel" hidden>{checkbox(label,'shared')}</div></div>'''
 run('accordion-repeated-headings-separate-groups',pageWrap('<p>Choose two items</p>'+sameGroup('Alpha')+sameGroup('Beta')),duplicateHeaders)
 def groupInstruction(pg):
  last=None
  for _ in range(12):
   last=step(pg)
   if pg.locator('#next').get_attribute('data-clicked')=='1':break
  assert pg.get_by_label('Trowel').is_checked(),last
  assert not pg.get_by_label('Paint').is_checked(),last
  assert pg.locator('#next').get_attribute('data-clicked')=='1',last
 run('accordion-named-group-minimum',pageWrap('<strong class="red">Must select atleast 1 GARDEN SUPPLIES</strong>'+accordion('Paint supplies',checkbox('Paint'))+accordion('Garden Supplies',checkbox('Trowel'))+radio('None of the above','none')),groupInstruction)
 def cfLikeGroup(title, options):
  controls=''.join(f"""<div class="cf-checkbox-answer"><div role="checkbox" tabindex="0" aria-checked="false" aria-label="{x}" onclick="this.setAttribute('aria-checked',this.getAttribute('aria-checked')==='true'?'false':'true')">{x}</div></div>""" for x in options)
  return f"""<div class="cf-collapsible-group cf-collapsing-panel"><div class="cf-collapsing-panel__title cf-collapsible-group__title" onclick="this.parentElement.classList.toggle('cf-collapsing-panel--open')">{title}</div><div class="cf-collapsing-panel__body"><div class="cf-collapsible-group__body">{controls}</div></div><div class="cf-collapsible-group__short-info"></div></div>"""
 def cfActualStructure(pg):
  last=None
  for _ in range(16):
   last=step(pg)
   if pg.locator('#next').get_attribute('data-clicked')=='1':break
  assert pg.get_by_label('Trowel').get_attribute('aria-checked')=='true',last
  assert pg.get_by_label('None of the above').get_attribute('aria-checked')=='false',last
  assert pg.locator('#next').get_attribute('data-clicked')=='1',last
  assert last['instructions']['blocked']==0,last
  assert last['accordion']['count']==2,last
 cfStyle='<style>.cf-collapsing-panel__title{cursor:pointer;padding:8px}.cf-collapsing-panel__body{height:0;overflow:hidden}.cf-collapsing-panel--open > .cf-collapsing-panel__body{height:auto}.cf-checkbox-answer [role=checkbox]{display:block;padding:8px}</style>'
 cfNone='<div class="cf-checkbox-answer"><div role="checkbox" aria-label="None of the above" aria-checked="false">None of the above</div></div>'
 run('cf-clipped-group-named-minimum-and-next',pageWrap(cfStyle+'<strong class="red">Must Select atleast 1 GARDEN SUPPLIES</strong>'+cfLikeGroup('Paint supplies',['Brush','Roller'])+cfLikeGroup('Garden Supplies',['Trowel','Rake'])+cfNone).replace('class="question"','class="question cf-question"'),cfActualStructure)
 def variedMulti(pg):
  outcomes=set()
  for _ in range(20):
   pg.set_content(pageWrap(''.join(checkbox(x) for x in ['Alpha','Beta','Gamma','Delta'])+checkbox('None of the above')))
   d=step(pg)
   checked=tuple(pg.locator('label').nth(i).locator('input').is_checked() for i in range(5))
   assert any(checked[:4]) and not checked[4],d
   outcomes.add(checked)
  assert len(outcomes)>1,outcomes
 run('multi-choice-varies-in-persona-mode',pageWrap(''),variedMulti)
 def redPosition(pg):
  d=step(pg); print('red position',d['action'],d.get('redHerring'),d.get('instructions'))
  assert pg.locator('input:checked').count()==1
  assert pg.locator('label').nth(1).locator('input').is_checked(),d
  assert d['instructions']['blocked']==0
 run('nested-red-bold',pageWrap('<p>Please <strong class="red">select</strong> the second option.</p>'+''.join(radio(x) for x in ['First','Second','Third','Fourth'])),redPosition)
 def redExact(pg):
  d=step(pg); print('exact',d['action'],d['instructions'])
  assert pg.get_by_label('Powder detergent').is_checked(),d
 run('red-exact-label',pageWrap('<strong class="red">Please select Powder detergent</strong>'+''.join(checkbox(x) for x in ['Liquid detergent','Detergent Bars','Powder detergent'])),redExact)
 def redMulti(pg):
  d=step(pg);print('red multi',d['action'],d['instructions']);
  assert pg.locator('label').nth(0).locator('input').is_checked()
  assert pg.locator('label').nth(2).locator('input').is_checked()
  assert pg.locator('input:checked').count()==2
 run('red-multiple-ordinals',pageWrap('<strong class="red">Select the first and third options</strong>'+''.join(checkbox(x) for x in ['Alpha','Beta','Gamma','Delta'])),redMulti)
 def unknown(pg):
  d=step(pg);print('unknown',d['action'],d['instructions'],d['unanswered'])
  assert pg.locator('input:checked').count()==0
  assert d['instructions']['blocked']>0 and d['unanswered']>0
  assert not pg.locator('#next').get_attribute('data-clicked')
 run('red-unknown-blocks',pageWrap('<strong class="red">Please select the striped triangle</strong>'+''.join(radio(x) for x in ['Alpha','Beta','Gamma'])),unknown)
 def redAccordion(pg):
  d=step(pg);print('acc red',d['action'],d['accordion'])
  assert pg.get_by_role('button',name='Hair Care Products').get_attribute('aria-expanded')=='true',d
  d=step(pg);print('acc red next',d['action'],d['instructions'])
  assert pg.get_by_label('Shampoo').is_checked(),d
 run('red-accordion-target',pageWrap('<strong class="red">Please select Shampoo</strong>'+accordion('Laundry Products',checkbox('Liquid detergent'),True)+accordion('Hair Care Products',checkbox('Shampoo')+checkbox('Conditioner'))),redAccordion)

 def ordinalAccordion(pg):
  d=step(pg); print('ordinal hidden step1',d['action'],d['unanswered'])
  assert pg.get_by_role('button',name='Hair Care Products').get_attribute('aria-expanded')=='true',d
  assert not pg.get_by_label('Liquid detergent').is_checked()
  d=step(pg); print('ordinal hidden step2',d['action'],d['unanswered'])
  assert pg.get_by_label('Conditioner').is_checked(),d
  d=step(pg);print('ordinal hidden step3',d['action'],d['unanswered'])
  assert pg.locator('input:checked').count()==1
  assert d['unanswered']==0,d
 run('ordinal-across-collapsed-categories',pageWrap('<strong class="red">Please select the fourth option</strong>'+accordion('Laundry Products',checkbox('Liquid detergent')+checkbox('Detergent Bars'))+accordion('Hair Care Products',checkbox('Shampoo')+checkbox('Conditioner'))),ordinalAccordion)
 def details(pg):
  d=step(pg);print('details step1',d['action'],d['accordion'])
  assert pg.locator('details').get_attribute('open') is not None,d
  d=step(pg);print('details step2',d['action'],d['unanswered'])
  assert pg.locator('details input:checked').count()>0,d
 run('native-details-disclosure',pageWrap('<details><summary>Bath Products</summary><section>'+checkbox('Soap')+checkbox('Shower gel')+'</section></details>'),details)
 def noConflict(pg):
  for _ in range(6):
   d=step(pg)
   if d['unanswered']==0:break
  assert not pg.get_by_label('None of the above').is_checked(),d
  assert pg.locator('input[type=checkbox]:checked').count()>=1,d
 run('none-exclusivity',pageWrap(accordion('Hair Care',checkbox('Shampoo')+checkbox('Conditioner'))+radio('None of the above','exclusive')),noConflict)

 def singleBoxes(pg):
  d=step(pg)
  for _ in range(5):
   if d['unanswered']==0:break
   d=step(pg)
  assert d['unanswered']==0,d
  assert pg.locator('input[type=checkbox]:checked').count()==1,d
 run('single-answer-checkbox-accordion',pageWrap(accordion('Laundry Products',checkbox('Liquid detergent')+checkbox('Detergent Bars'))+accordion('Skincare Products',checkbox('Face cream'))).replace('Which products have you used?','Please select one product'),singleBoxes)

 def lazyPanel(pg):
  first=step(pg)
  assert first['unanswered']>0 and 'accordion-expand' in first['action'],first
  second=step(pg)
  assert second['unanswered']>0 and not pg.locator('#next').get_attribute('data-clicked'),second
  pg.wait_for_timeout(750)
  third=step(pg)
  assert pg.get_by_label('Shampoo').is_checked(),third
 run('lazy-rendered-accordion',pageWrap('''<section class="accordion-item"><button type="button" class="accordion-header" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');let p=this.nextElementSibling;p.hidden=false;setTimeout(()=>{p.innerHTML='<label><input type=checkbox name=late>Shampoo</label>'},600)">Hair Care Products</button><div class="panel" hidden></div></section>'''),lazyPanel)
 # Regression: one answered page followed by a *generic* red/bold prompt used
 # to abort the run as needs_review after three holds.
 def consecutivePages(pg):
  one=step(pg)
  assert 'next' in one['action'] and pg.locator('#page2').is_visible(),one
  two=step(pg)
  assert two['instructions']['blocked']==0 and 'next' in two['action'],two
  assert pg.locator('#done').is_visible(),two
 run('two-pages-generic-red-continues',
   """<html><head>"""+STYLE+"""</head><body><div id="page1"><div class="question">Have you shopped this month?"""+radio('Yes','a')+radio('No','a')+"""</div></div><div id="page2" hidden><div class="question"><strong class="red">Please select one answer</strong><h3>Do you like online shopping?</h3>"""+radio('Yes','b')+radio('No','b')+"""</div></div><div id="done" hidden>Completed QA fixture</div><button id="next" onclick="let p=document.querySelector('#page1:not([hidden]),#page2:not([hidden])');if(p){p.hidden=true;let n=p.id==='page1'?document.querySelector('#page2'):document.querySelector('#done');n.hidden=false}">Next</button></body></html>""",consecutivePages)
 def genericMulti(pg):
  d=step(pg)
  assert d['instructions']['blocked']==0 and d['unanswered']==0,d
  assert pg.locator('input[type="checkbox"]:checked').count()>0,d
 run('generic-red-multiple-choice',pageWrap('<strong class="red">Select all that apply</strong>'+checkbox('Alpha')+checkbox('Beta')+checkbox('Gamma')),genericMulti)
 def slowNavigation(pg):
  first=step(pg)
  assert 'next' in first['action'] and pg.locator('#next').get_attribute('data-count')=='1',first
  again=step(pg)
  assert 'next-waiting' in again['action'] and pg.locator('#next').get_attribute('data-count')=='1',again
  pg.wait_for_timeout(750)
  third=step(pg)
  assert pg.locator('#second').is_visible() and pg.locator('#second input:checked').count()==1,third
  assert pg.locator('#next').get_attribute('data-count')=='2',third
 run('next-delayed-navigation-single-click',
  """<html><head>"""+STYLE+"""</head><body><div id="first" class="question">Pick first:"""+radio('A','a')+radio('B','a')+"""</div><div id="second" class="question" hidden>Pick second:"""+radio('C','b')+radio('D','b')+"""</div><button id="next" onclick="this.dataset.count=String(1+Number(this.dataset.count||0));if(this.dataset.count==='1')setTimeout(()=>{document.getElementById('first').hidden=true;document.getElementById('second').hidden=false},600)">Next</button></body></html>""",slowNavigation)
 def delayedMounted(pg):
  d=step(pg)
  assert pg.locator('input[type=radio]:checked').count()==1,d
  assert pg.locator('#next').get_attribute('data-clicked')=='1',d
 run('late-mounted-required-choice',
     """<html><head>"""+STYLE+"""</head><body><div class=question><h3 class=question-title>Have you tried this product?</h3><div id=choices></div></div><button id=next onclick="this.dataset.clicked=String(1+Number(this.dataset.clicked||0))">Next</button><script>setTimeout(()=>{document.getElementById('choices').innerHTML='<label><input type=radio name=delayed>Delayed yes</label><label><input type=radio name=delayed>Delayed no</label>'},600)</script></body></html>""",delayedMounted)
 def infoOnly(pg):
  d=step(pg)
  assert pg.locator('#next').get_attribute('data-clicked')=='1',d
  assert 'render-waiting' not in d['action'],d
 run('text-only-information-page-advances',
     """<html><head>"""+STYLE+"""</head><body><section class=question><p>We would like to learn about your experience. This survey takes a few minutes. Thank you for helping us.</p></section><button id=next onclick="this.dataset.clicked=1">Next</button></body></html>""",infoOnly)
 def spelledAlternative(pg):
  d=step(pg)
  assert pg.get_by_label('South Ridge').is_checked(),d
  assert d['instructions']['blocked']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 run('single-choice-or-one-exact-other-spacing',pageWrap('<span style="color:red">Please select Northshore OR South Ridge</span>'+radio('North Shore')+radio('South Ridge')+radio('Other')),spelledAlternative)
 def spacingOnly(pg):
  d=step(pg)
  assert pg.get_by_label('North Shore').is_checked(),d
  assert d['instructions']['blocked']==0,d
 run('single-choice-spacing-variant-only',pageWrap('<span style="color:red">Select Northshore OR Unknown place</span>'+radio('North Shore')+radio('Another place')),spacingOnly)
 def foreverEmpty(pg):
  d=step(pg)
  assert 'render-waiting' in d['action'] and d['unanswered']>0,d
  assert pg.locator('#next').get_attribute('data-clicked') is None,d
 run('unmounted-question-never-autosubmits',
     """<html><head>"""+STYLE+"""</head><body><section class=question><h3 class=question-title>Which product did you choose?</h3></section><button id=next onclick="this.dataset.clicked='1'">Next</button></body></html>""",foreverEmpty)
 # Regression for real-world red response-format instructions on a later page.
 def ordinaryIdentityRed(pg):
  a=step(pg)
  assert pg.locator('#id-page').is_visible(),a
  b=step(pg)
  assert b['instructions']['blocked']==0 and pg.locator('#finished').is_visible(),b
  assert pg.locator('#id-page input:checked').count()==1,b
 run('red-identity-format-after-previous-page',
  "<html><head>"+STYLE+"</head><body><section id=first-page class=question>Are you familiar with our products?"+radio('Yes','s1')+radio('No','s1')+"</section><section id=id-page class=question hidden><strong class=red>Please choose the option that best describes how you identify yourself.</strong><h3>How do you identify?</h3>"+radio('Woman','id')+radio('Man','id')+radio('Another identity','id')+"</section><section id=finished hidden>Test end</section><button id=next onclick=\"let x=document.querySelector('section:not([hidden])');x.hidden=true;(x.id==='first-page'?document.getElementById('id-page'):document.getElementById('finished')).hidden=false\">Next</button></body></html>",ordinaryIdentityRed)
 def oneAnswerOnly(pg):
  d=step(pg)
  assert d['instructions']['blocked']==0 and d['unanswered']==0,d
 run('red-mandatory-one-answer-message',pageWrap('<strong class="red">Please select one option from the following choices.</strong>'+radio('A')+radio('B')),oneAnswerOnly)
 def numberTarget(pg):
  d=step(pg)
  assert d['instructions']['blocked']==0,d
  assert pg.get_by_label('C').is_checked(),d
 run('red-numeric-option-target',pageWrap('<strong class="red">For quality control, select option 3.</strong>'+radio('A')+radio('B')+radio('C')),numberTarget)
 def numericAlternatives(pg):
  d=step(pg)
  assert pg.locator('input:checked').count()==1,d
  assert pg.get_by_label('E',exact=True).is_checked(),d
  assert d['instructions']['blocked']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
  (r/'test-evidence').mkdir(exist_ok=True)
  pg.screenshot(path=str(r/'test-evidence'/'numeric-instruction.png'))
 run('red-numeric-alternatives-before-option-noun',pageWrap('<strong class="red">Choose 5 OR 6 OR 7 responses</strong>'+''.join(radio(x) for x in ['A','B','C','D','E','F','G'])),numericAlternatives)
 def numberOutOfRange(pg):
  d=step(pg)
  assert d['instructions']['blocked']>0 and d['unanswered']>0,d
  assert pg.locator('#next').get_attribute('data-clicked') is None,d
 run('red-out-of-range-fails-closed',pageWrap('<strong class="red">Select option 9</strong>'+radio('A')+radio('B')+radio('C')),numberOutOfRange)
 def redPlainText(pg):
  d=step(pg)
  assert d['instructions']['blocked']>0 and d['unanswered']>0,d
  assert pg.locator('#next').get_attribute('data-clicked') is None,d
 run('red-not-bold-unknown-blocks',pageWrap('<span style="color:red">Please select the invisible triangle</span>'+radio('A')+radio('B')),redPlainText)
 def crossQuestion(pg):
  d=step(pg)
  assert pg.get_by_label('Female',exact=True).is_checked(),d
  assert pg.locator('select').input_value() in ['25','30','35','40'],d
  assert d['instructions']['blocked']==0 and pg.locator('#next').get_attribute('data-clicked')=='1',d
 run('cross-question-red-choice-and-age',
  '<html><head>'+STYLE+'</head><body><form><div class="question"><h3>How do you identify?</h3><span style="color:red">Please select Females only and age 25 - 40</span>'+radio('Male','g')+radio('Female','g')+'</div><div class="question"><h3>How old are you?</h3><select name="age"><option value="">Please select</option><option>18</option><option>25</option><option>30</option><option>35</option><option>40</option><option>50</option></select></div><button type="button" id="next" onclick="this.dataset.clicked=1">Next</button></form></body></html>',crossQuestion)
 def minimumRequired(pg):
  d=step(pg)
  assert pg.locator('input[type=checkbox]:checked').count()==3,d
  assert pg.get_by_label('Beta').is_checked(),d
  assert d['instructions']['blocked']==0,d
 run('minimum-count-includes-named-option',pageWrap('<span style="color:red">Select at least three options including Beta</span>'+checkbox('Alpha')+checkbox('Beta')+checkbox('Gamma')+checkbox('Delta')),minimumRequired)
 def explicitNotGeneric(pg):
  d=step(pg)
  assert d['instructions']['blocked']>0 and d['unanswered']>0,d
 run('red-specific-command-not-bypassed',pageWrap('<strong class="red">Please select the striped triangle, not just one answer</strong>'+radio('A')+radio('B')),explicitNotGeneric)
 def ambiguousBest(pg):
  d=step(pg)
  assert d['instructions']['blocked']>0 and d['unanswered']>0,d
 run('best-match-external-symbol-is-specific',pageWrap('<strong class="red">Select the option that best matches the blue symbol</strong>'+radio('Alpha')+radio('Beta')),ambiguousBest)
 b.close()
print('PASS',checks,'browser fixtures')
