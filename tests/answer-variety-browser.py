"""Synthetic browser coverage for answer variety; no real survey is contacted."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

root = Path(__file__).resolve().parents[1]
names = ['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine = '\n'.join((root/'src'/'engine'/f'{name}.js').read_text() for name in names)
engine = engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'MultiSelectCount':2,'AnswerMode':'random'})+';').replace('const answered=__ANSWERED__;', 'const answered=0;')
html = '''<style>label{display:block;padding:8px}</style><form><div class="question"><h3>Which product would you choose?</h3><label><input type="radio" name="q">Alpha</label><label><input type="radio" name="q">Beta</label><label><input type="radio" name="q">Gamma</label></div><button type="button" id="next">Next</button></form>'''
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, args=['--no-sandbox','--disable-dev-shm-usage'])
    page = browser.new_page()
    page.set_content(html)
    initial = page.evaluate(engine.replace('const MEM=MEMORY;', 'const MEM={};'))
    key = initial['selectedPairs'][0]['question']
    memory = {'prefer':{key:'Gamma'}, 'score':{key:{'Gamma':100}}}
    code = engine.replace('const MEM=MEMORY;', 'const MEM='+json.dumps(memory)+';')
    counts = {'Alpha':0, 'Beta':0, 'Gamma':0}
    for i in range(90):
        page.set_content(html)
        page.evaluate('(v)=>{Math.random=()=>v}', [0.1,0.5,0.9][i%3])
        result=page.evaluate(code)
        picked=page.locator('input:checked').evaluate_all('(xs)=>xs.map(x=>x.parentElement.innerText.trim())')
        assert len(picked)==1, (picked,result)
        counts[picked[0]]+=1
    assert all(counts[x]>=20 for x in counts), counts
    print('PASS synthetic single choice covers all options despite preferred history:', counts)
    browser.close()
