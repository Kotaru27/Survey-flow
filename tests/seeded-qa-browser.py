"""Synthetic browser checks for reproducible, isolated seeded answers."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
names=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{n}.js').read_text() for n in names)
html='''<style>label{display:block;padding:8px}</style><form><div class="question"><h3>Which product would you choose?</h3><label><input type="radio" name="q" value="a">Alpha</label><label><input type="radio" name="q" value="b">Beta</label><label><input type="radio" name="q" value="c">Gamma</label></div><button type="button" id="next">Next</button></form>'''
def answer(page,run):
 page.set_content(html)
 persona={'MultiSelectCount':2,'AnswerMode':'random','qaSeed':'synthetic-browser','qaRun':run,'qaStep':1}
 script=engine.replace('const RAW=PERSONA;','const RAW='+json.dumps(persona)+';').replace('const MEM=MEMORY;','const MEM={};').replace('const answered=__ANSWERED__;','const answered=0;')
 page.evaluate(script)
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
 browser.close()
