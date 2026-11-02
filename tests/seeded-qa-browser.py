"""Synthetic browser checks for reproducible, isolated seeded answers."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{n}.js').read_text() for n in names)
html='''<style>label{display:block;padding:8px}</style><form><div class="question"><h3>Which product would you choose?</h3><label><input type="radio" name="q" value="a">Alpha</label><label><input type="radio" name="q" value="b">Beta</label><label><input type="radio" name="q" value="c">Gamma</label></div><button type="button" id="next">Next</button></form>'''
def step(page,run,markup):
 page.set_content(markup)
 persona={'MultiSelectCount':2,'AnswerMode':'random','qaSeed':'synthetic-browser','qaRun':run,'qaStep':1}
 script=engine.replace('const RAW=PERSONA;','const RAW='+json.dumps(persona)+';').replace('const MEM=MEMORY;','const MEM={};').replace('const answered=__ANSWERED__;','const answered=0;')
 return page.evaluate(script)
def answer(page,run):
 result=step(page,run,html)
 assert len(result['questions'])==1,result
 options=result['questions'][0]['observedOptions']
 assert [(o['position'],o['value']) for o in options]==[(1,'a'),(2,'b'),(3,'c')],options
 assert sum(1 for o in options if o['selected'])==1,options
 selected=page.locator('input:checked')
 assert selected.count()==1
 return selected.input_value()
with sync_playwright() as p:
 browser=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
 c1=browser.new_context();c2=browser.new_context();a=c1.new_page();b=c2.new_page()
 counts={'a':0,'b':0,'c':0}
 for run in range(1,91):
  x=answer(a,run);counts[x]+=1
  if run<=15: assert answer(b,run)==x,('seed mismatch',run)
 assert all(n>=12 for n in counts.values()),counts
 print('PASS seeded synthetic browser choices and separate-session replay:',counts)
 select='''<style>select{display:block;padding:8px}</style><form><div class="question"><h3>Which category would you choose?</h3><select name="q" required><option value="">Please select</option><option value="a">Alpha</option><option value="b">Beta</option><option value="c">Gamma</option></select></div><button type="button" id="next">Next</button></form>'''
 dropdown={'a':0,'b':0,'c':0}
 for run in range(1,91):
  step(a,run,select);v=a.locator('select').input_value();assert v in dropdown;dropdown[v]+=1
 assert min(dropdown.values())>0,dropdown
 print('PASS seeded dropdown option coverage:',dropdown)
 multi='''<style>label{display:block;padding:8px}</style><form><div class="question"><h3>Which products do you use?</h3><label><input type="checkbox" name="q" value="a">Alpha</label><label><input type="checkbox" name="q" value="b">Beta</label><label><input type="checkbox" name="q" value="c">Gamma</label><label><input type="checkbox" name="q" value="none">None of the above</label></div><button type="button" id="next">Next</button></form>'''
 multi_counts={'a':0,'b':0,'c':0,'none':0}
 for run in range(1,91):
  step(a,run,multi);selected=a.locator('input:checked').evaluate_all('(xs)=>xs.map(x=>x.value)')
  assert selected and not ('none' in selected and len(selected)>1),selected
  for v in selected:multi_counts[v]+=1
 assert min(multi_counts.values())>0,multi_counts
 print('PASS seeded multi-choice coverage and exclusivity:',multi_counts)
 browser.close()
