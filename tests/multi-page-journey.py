"""A connected synthetic survey journey. Does not access a live survey URL.
Requires installed Playwright and Chromium in the cloud test environment.
"""
from pathlib import Path
from playwright.sync_api import sync_playwright
import json

root=Path(__file__).resolve().parents[1]
mods=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
script='\n'.join((root/'src'/'engine'/f'{mod}.js').read_text() for mod in mods)
script=script.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'Gender':'Female','Age':30,'MultiSelectCount':2,'OpenText':'QA response','Number':30,'AnswerMode':'persona','YesNo':'Yes'})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')

# Each gate is enforced by the test website itself. The runner cannot advance by
# merely clicking Next without a correctly populated answer or interaction state.
def page(markup,gate,after=''):
 return {'html': '<section class="question"><h3>QA question</h3>'+markup+'</section>', 'check':gate,'after':after}
def radio(name,choices):
 return ''.join(f'<label><input type="radio" name="{name}" value="{i}">{t}</label>' for i,t in enumerate(choices))
def check(name,choices):
 return ''.join(f'<label><input type="checkbox" name="{name}" value="{i}">{t}</label>' for i,t in enumerate(choices))
def accordion(title,choices,name):
 return '<div class="accordion-item"><button type="button" class="accordion-header" aria-expanded="false" onclick="let open=this.getAttribute(\'aria-expanded\')!==\'true\';this.setAttribute(\'aria-expanded\',String(open));this.nextElementSibling.hidden=!open">'+title+'</button><div hidden>'+check(name,choices)+'</div></div>'

pages=[
 page('Have you tried our synthetic QA product?'+radio('choice1',['Yes','No']),"!!document.querySelector('input[name=choice1]:checked')"),
 page('Choose any two hobbies.'+check('hobbies',['Music','Films','Books']),"document.querySelectorAll('input[name=hobbies]:checked').length>=1"),
 page('Which group best describes you?<select name="group" required><option value="placeholder">Please select an option</option><option value="first">First group</option><option value="second">Second group</option></select>',"document.querySelector('select').value!=='placeholder'"),
 page('Write a brief QA answer.<textarea required name="comment"></textarea>',"document.querySelector('textarea').value.trim().length>0"),
 page('Enter a synthetic count.<input type="number" required min="18" max="45" step="3" name="count">',"document.querySelector('input[name=count]').checkValidity() && document.querySelector('input[name=count]').value!==''"),
 page('Provide a synthetic birthday.<input type="date" required min="1980-01-01" max="2005-01-01" name="birthday">',"document.querySelector('input[name=birthday]').checkValidity() && !!document.querySelector('input[name=birthday]').value"),
 page('Move the score slider.<input type="range" name="score" min="0" max="10" step="2">',"document.querySelector('input[name=score]').dataset.apSelected==='1'"),
 page('Rate each item.<div class="matrix"><div class="matrix-row">A '+radio('matrixA',['Low','High'])+'</div><div class="matrix-row">B '+radio('matrixB',['Low','High'])+'</div></div>',"document.querySelectorAll('.matrix-row input:checked').length===2"),
 page('Which items are familiar?'+accordion('Home care',['Detergent','Soap'],'acc')+accordion('Personal care',['Shampoo','Moisturizer'],'acc'),"document.querySelectorAll('input[name=acc]:checked').length>=1"),
 page('Which items were purchased in another period?'+accordion('Home care',['Bleach','Dish soap'],'acc2')+accordion('Personal care',['Body lotion','Face wash'],'acc2'),"document.querySelectorAll('input[name=acc2]:checked').length>=1"),
 page('Which items were bought recently?'+accordion('Home care',['Sheets','Powder'],'acc3')+accordion('Personal care',['Serum','Cream'],'acc3'),"document.querySelectorAll('input[name=acc3]:checked').length>=1"),
 page('<p>Please <strong class="red">select</strong> the second option.</p>'+radio('ordinal',['One','Two','Three']),"document.querySelector('input[name=ordinal]:checked')?.value==='1'"),
 page('<strong class="red">Select Beta</strong>'+radio('exact',['Alpha','Beta','Gamma']),"document.querySelector('input[name=exact]:checked')?.value==='1'"),
 page('<strong class="red">Please choose the option that best describes yourself</strong>'+radio('general',['Choice A','Choice B']),"!!document.querySelector('input[name=general]:checked')"),
 page('Select a state.<div role="radiogroup"><div role="radio" aria-checked="false" tabindex="0" onclick="document.querySelectorAll(\'[role=radio]\').forEach(e=>e.setAttribute(\'aria-checked\',\'false\'));this.setAttribute(\'aria-checked\',\'true\')">Left</div><div role="radio" aria-checked="false" tabindex="0" onclick="document.querySelectorAll(\'[role=radio]\').forEach(e=>e.setAttribute(\'aria-checked\',\'false\'));this.setAttribute(\'aria-checked\',\'true\')">Right</div></div>',"!!document.querySelector('[role=radio][aria-checked=true]')"),
 page('Choose from details.<details><summary>Open choices</summary>'+check('disclosure',['Pear','Apple'])+'</details>',"document.querySelector('details').open && !!document.querySelector('details input:checked')"),
 page('Two separate questions.<fieldset><legend>First</legend>'+radio('qa',['Yes','No'])+'</fieldset><fieldset><legend>Second</legend>'+radio('qb',['Yes','No'])+'</fieldset>',"!!document.querySelector('input[name=qa]:checked') && !!document.querySelector('input[name=qb]:checked')"),
 page('Which delayed answer?<div id="late"></div>',"!!document.querySelector('#late input:checked')", "setTimeout(()=>document.getElementById('late').innerHTML='<label><input type=radio name=delayed>Loaded choice</label><label><input type=radio name=delayed>Alternate choice</label>',350)"),
 page('<strong class="red">Select the first and third options</strong>'+check('multiord',['First','Second','Third']),"document.querySelector('input[name=multiord][value=\"0\"]').checked && document.querySelector('input[name=multiord][value=\"2\"]').checked && document.querySelectorAll('input[name=multiord]:checked').length===2"),
 page('Enter a synthetic time.<input type="time" name="tm" required>',"!!document.querySelector('input[name=tm]').value"),
 page('Enter a synthetic month.<input type="month" name="mn" required>',"!!document.querySelector('input[name=mn]').value"),
 page('Enter a synthetic timestamp.<input type="datetime-local" name="ts" required>',"!!document.querySelector('input[name=ts]').value"),
]
html='''<!doctype html><html><head><meta charset="UTF-8"><style>body{font:17px Arial;margin:30px}.question{margin:20px 0}label{display:block;padding:5px}.accordion-header{padding:10px}.red{color:#dd0000}.matrix-row{display:block;padding:12px}button{padding:10px}</style></head><body><main id="host"></main><button id="next">Next</button><script>
const PAGES=__PAGES__;
window.__qa={index:0,accepted:[],rejected:[],attempts:[]};
function render(){const s=PAGES[window.__qa.index]; document.querySelector('#host').innerHTML=s.html; if(s.after) {new Function(s.after)();}}
document.addEventListener('click',evt=>{if(evt.target.id!=='next')return; const s=PAGES[window.__qa.index];const ok=(new Function('return ('+s.check+')'))(); window.__qa.attempts.push({page:window.__qa.index,ok});if(!ok){window.__qa.rejected.push(window.__qa.index);return;}window.__qa.accepted.push(window.__qa.index);window.__qa.index++;if(window.__qa.index>=PAGES.length){document.body.innerHTML='<h2>Final synthetic QA URL reached</h2>';window.__qa.done=true;return;}render();});
render();</script></body></html>'''.replace('__PAGES__',json.dumps(pages))
try:
 with sync_playwright() as p:
  b=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
  pg=b.new_page(viewport={'width':1280,'height':900})
  pg.on('pageerror',lambda exc: print('PAGE ERROR',str(exc)))
  pg.set_content(html, wait_until='load')
  steps=[];index=0;stalls=0
  for i in range(125):
   before=pg.evaluate('window.__qa?.index ?? -1')
   d=pg.evaluate(script)
   after=pg.evaluate('window.__qa?.index ?? -1')
   steps.append({'iteration':i+1,'page_before':before+1,'page_after':after+1,'action':d['action'],'unanswered':d['unanswered'],'blocking':d.get('instructions',{}).get('blocked',0)})
   if pg.evaluate('window.__qa.done === true'):break
   if after==before:stalls+=1
   else:stalls=0
   if stalls>=9:
    print('DEBUG',pg.evaluate('({qa:window.__qa,boxes:Array.from(document.querySelectorAll("input")).map(e=>({name:e.name,v:e.value,on:e.checked}))})'))
    print('STALLED',json.dumps(steps[-9:],indent=2));break
   pg.wait_for_timeout(130)
  accepted=pg.evaluate('window.__qa.accepted.length')
  rejected=pg.evaluate('window.__qa.rejected')
  result={'pages':len(pages),'accepted':accepted,'rejected_submissions':rejected,'url':pg.url,'engine_steps':len(steps),'steps':steps}
  (root/'tests'/'multi-page-journey-result.json').write_text(json.dumps(result,indent=2))
  print('JOURNEY',accepted,'/',len(pages),'accepted; rejected Next clicks',len(rejected),'synthetic end marker', pg.evaluate('window.__qa.done === true'))
  done=pg.evaluate('window.__qa.done === true')
  b.close()
  assert accepted==len(pages) and done,json.dumps(steps[-12:],indent=2)
  assert not rejected, 'engine tried Next before an answer passed page validation: '+str(rejected)
finally:pass
