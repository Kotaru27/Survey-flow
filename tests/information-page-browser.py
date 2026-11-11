"""Synthetic browser checks for information pages and delayed questions."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

root=Path(__file__).resolve().parents[1]
parts=['header','detect','helpers','accordion','analyze','dispatch','adapters','adapters-custom','navigate']
engine='\n'.join((root/'src'/'engine'/f'{p}.js').read_text() for p in parts)
engine=engine.replace('const RAW=PERSONA;', 'const RAW='+json.dumps({'AnswerMode':'persona','OpenText':'Example'})+';').replace('const MEM=MEMORY;', 'const MEM={};').replace('const answered=__ANSWERED__;', 'const answered=0;')
base='''<style>body{padding:30px} .question{padding:20px} button{padding:12px}</style><input type="hidden" name="state" value="synthetic"><div class="question"><h3 class="question-title">%s</h3><div id="mount"></div></div><button id="next" onclick="this.dataset.clicked='yes'">Next</button>%s'''
with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'])
    page=browser.new_page()
    page.set_content(base % ('On the following pages you will see an exercise. You will be asked to indicate your preference.', ''))
    first=page.evaluate(engine)
    assert page.locator('#next').get_attribute('data-clicked') is None, first
    page.wait_for_timeout(800)
    for _ in range(5):
        last=page.evaluate(engine)
        if page.locator('#next').get_attribute('data-clicked')=='yes': break
        page.wait_for_timeout(400)
    assert page.locator('#next').get_attribute('data-clicked')=='yes',last
    print('PASS future information page advances after stability wait')
    page.set_content(base % ('Which option do you prefer?', '''<script>setTimeout(()=>{document.querySelector('#mount').innerHTML='<label><input type="radio" name="choice">Example</label>'},2500)</script>'''))
    first=page.evaluate(engine)
    assert page.locator('#next').get_attribute('data-clicked') is None,first
    page.wait_for_timeout(1900)
    for _ in range(5):
        last=page.evaluate(engine)
        if page.locator('#next').get_attribute('data-clicked')=='yes': break
        page.wait_for_timeout(300)
    assert page.locator('input[type=radio]').is_checked(),last
    assert page.locator('#next').get_attribute('data-clicked')=='yes',last
    print('PASS delayed current question is answered before advancing')
    # A legitimate text-only question/information screen can have a question-mark
    # heading but no answer inputs. It must advance after a bounded render grace.
    page.set_content(base % ('What happens next?', '<p>The following page will explain the next section.</p>'))
    page.evaluate('delete window.__apNavIntent; delete window.__apInfoWait')
    d=page.evaluate(engine)
    assert page.locator('#next').get_attribute('data-clicked') is None,d
    for attempt in range(8):
        page.wait_for_timeout(500)
        d=page.evaluate(engine)
        if page.locator('#next').get_attribute('data-clicked')=='yes': break
    assert page.locator('#next').get_attribute('data-clicked')=='yes',d
    assert d['action'].split('+')[-1]=='next',d
    print('PASS text-only question prompt with no answer controls advances after grace')
    # A Next action can be an icon whose accessible text is on title, not the glyph.
    page.set_content('<style>button{padding:20px}</style><p>Study information</p><button id="next" title="Next" onclick="this.dataset.clicked=\'yes\'"><span aria-hidden="true">›</span></button>')
    page.evaluate('delete window.__apNavIntent; delete window.__apInfoWait')
    d=page.evaluate(engine)
    assert page.locator('#next').get_attribute('data-clicked')=='yes',d
    print('PASS icon-only Next using title advances correctly')
    browser.close()
