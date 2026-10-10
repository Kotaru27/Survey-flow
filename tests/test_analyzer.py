from survey_flow.analyzer import analyze
from survey_flow.models import Outcome


SA_PAGE = """<html><body><form method=post action=/s>
<div id="QuestionDIV9">
  <p class="QuestionText">Are you currently employed?</p>
  <span style="color:#f00"><b>Select: "Yes"</b></span>
  <label><input type="radio" name="q1" value="1"> No</label>
  <label><input type="radio" name="q1" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

MA_PAGE = """<html><body><form method=post>
<div id="Q2">
  <p class="QuestionText">Pick at least 3 services</p>
  <label><input type="checkbox" name="q2_1" value="1"> Email</label>
  <label><input type="checkbox" name="q2_2" value="2"> Search</label>
  <label><input type="checkbox" name="q2_3" value="3"> Video</label>
  <label><input type="checkbox" name="q2_4" value="4"> Chat</label>
  <label><input type="checkbox" name="q2_5" value="5"> None of these</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

GRID_PAGE = """<html><body><form method=post>
<div id="QG">
  <p class="QuestionText">Rate these</p>
  <table>
    <tr><td>Speed</td>
        <td><label><input type="radio" name="g_1" value="1"> Poor</label></td>
        <td><label><input type="radio" name="g_1" value="2"> Good</label></td></tr>
    <tr><td>Value</td>
        <td><label><input type="radio" name="g_2" value="1"> Poor</label></td>
        <td><label><input type="radio" name="g_2" value="2"> Good</label></td></tr>
  </table>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

RANK_PAGE = """<html><body><form method=post>
<div id="QR">
  <p class="QuestionText">Rank the fruits, best first</p>
  <table>
    <tr><td>Apple</td>
        <td><label><input type="radio" name="r_1" value="1"> 1</label></td>
        <td><label><input type="radio" name="r_1" value="2"> 2</label></td></tr>
    <tr><td>Banana</td>
        <td><label><input type="radio" name="r_2" value="1"> 1</label></td>
        <td><label><input type="radio" name="r_2" value="2"> 2</label></td></tr>
  </table>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

DROPDOWN_PAGE = """<html><body><form method=post>
<div id="QD">
  <p class="QuestionText">Choose your region</p>
  <select name="qd">
    <option value="">-- Select --</option>
    <option value="1">North</option>
    <option value="2">South</option>
  </select>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

NUMBER_PAGE = """<html><body><form method=post>
<div id="QN">
  <p class="QuestionText">What is your age? Enter a number between 18 and 99</p>
  <input type="number" name="age" min="18" max="99">
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

TEXT_PAGE = """<html><body><form method=post>
<div id="QT">
  <p class="QuestionText">Why do you use our service?</p>
  <input type="text" name="why" maxlength="80">
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

TERMINATE_IN_WORD_PAGE = """<html><body><form method=post>
<div id="QX">
  <p class="QuestionText">Have you ever been terminated from a job?</p>
  <label><input type="radio" name="qx" value="1"> No</label>
  <label><input type="radio" name="qx" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

COMPLETE_PAGE = "<html><body><h1>Thank you</h1><p>Your survey is now complete.</p></body></html>"
TERMINATE_PAGE = "<html><body><h1>Survey terminated</h1><p>You do not qualify.</p></body></html>"
QUOTA_PAGE = "<html><body><h1>Quota full</h1><p>The quota has been reached.</p></body></html>"
VALIDATION_PAGE = SA_PAGE.replace(
    '<form method=post', '<div class="error">Please answer this question</div><form method=post'
)


def test_single_choice_with_red_instruction():
    page = analyze(SA_PAGE, "http://x/test")
    assert page.outcome == Outcome.QUESTION
    assert len(page.questions) == 1
    q = page.questions[0]
    assert q.kind == "single"
    assert q.sig == "QuestionDIV9"
    assert any('Select: "Yes"' in i for i in q.instructions), q.instructions
    labels = [f.label for f in q.fields if f.type == "radio"]
    assert labels == ["No", "Yes"]


def test_multi_min_and_exclusive():
    page = analyze(MA_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "multi"
    assert q.min_select == 3
    excl = [f for f in q.fields if f.exclusive]
    assert [f.label for f in excl] == ["None of these"]


def test_grid():
    page = analyze(GRID_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "grid_single"
    rows = {f.name for f in q.fields if f.type == "radio"}
    assert rows == {"g_1", "g_2"}
    # row labels attached
    first = next(f for f in q.fields if f.name == "g_1")
    assert first.row_label == "Speed"


def test_ranking_detected():
    page = analyze(RANK_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "ranking"


def test_dropdown_options_attached():
    page = analyze(DROPDOWN_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "dropdown"
    sel = q.fields[0]
    labels = [o.label for o in sel.options]
    assert labels == ["-- Select --", "North", "South"]
    assert sel.options[0].is_placeholder_option
    assert not sel.options[1].is_placeholder_option


def test_number_bounds():
    page = analyze(NUMBER_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "number"
    f = q.scalars[0]
    assert f.min == "18" and f.max == "99"


def test_text_limits():
    page = analyze(TEXT_PAGE, "http://x/test")
    q = page.questions[0]
    assert q.kind == "text"
    assert q.scalars[0].maxlength == 80


def test_terminate_word_in_question_is_not_end_status():
    page = analyze(TERMINATE_IN_WORD_PAGE, "http://x/survey")
    assert page.outcome == Outcome.QUESTION


def test_end_pages():
    assert analyze(COMPLETE_PAGE, "http://x/").outcome == Outcome.COMPLETE
    assert analyze(TERMINATE_PAGE, "http://x/").outcome == Outcome.TERMINATE
    assert analyze(QUOTA_PAGE, "http://x/").outcome == Outcome.QUOTA_FULL


def test_validation_page():
    page = analyze(VALIDATION_PAGE, "http://x/test")
    assert page.outcome == Outcome.VALIDATION
    assert page.messages


def test_interstitial():
    html = """<html><body><form method=post>
      <p>Welcome</p><input type="submit" value="Start">
      </form></body></html>"""
    page = analyze(html, "http://x/test")
    assert page.outcome == Outcome.INTERSTITIAL


def test_submit_detected():
    page = analyze(SA_PAGE, "http://x/test")
    assert page.submits
    assert page.submits[0].label == "Next"
    assert page.form_method == "POST"


def test_hidden_fields_captured():
    html = SA_PAGE.replace("</form>", '<input type="hidden" name="sid" value="abc"></form>')
    page = analyze(html, "http://x/test")
    assert ("sid", "abc") in page.hiddens
