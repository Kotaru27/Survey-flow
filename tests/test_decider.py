import json

from survey_flow.analyzer import analyze
from survey_flow.decider import Decider, Persona, constraints_from_validations
from survey_flow.memory import TerminateMemory

SA_HTML = """<html><body><form method=post>
<div id="QuestionDIV1">
  <p class="QuestionText">Are you employed?</p>
  <label><input type="radio" name="q1" value="1"> No</label>
  <label><input type="radio" name="q1" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

INSTR_HTML = """<html><body><form method=post>
<div id="QuestionDIV2">
  <p class="QuestionText">Driving license?</p>
  <span style="color:#f00"><b>Select: "Yes"</b></span>
  <label><input type="radio" name="q2" value="1"> No</label>
  <label><input type="radio" name="q2" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

MA_HTML = """<html><body><form method=post>
<div id="Q3">
  <p class="QuestionText">Choose some services</p>
  <label><input type="checkbox" name="q3_1" value="1"> A</label>
  <label><input type="checkbox" name="q3_2" value="2"> B</label>
  <label><input type="checkbox" name="q3_3" value="3"> C</label>
  <label><input type="checkbox" name="q3_4" value="4"> D</label>
  <label><input type="checkbox" name="q3_5" value="5"> None of these</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

NUM_HTML = """<html><body><form method=post>
<div id="Q4">
  <p class="QuestionText">What is your age? Enter a number between 18 and 25</p>
  <input type="number" name="age">
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

CSUM_HTML = """<html><body><form method=post>
<div id="Q5">
  <p class="QuestionText">Allocate 100 points among these</p>
  Price: <input type="number" name="p1"><br>
  Quality: <input type="number" name="p2"><br>
  Service: <input type="number" name="p3">
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

RANK_HTML = """<html><body><form method=post>
<div id="Q6">
  <p class="QuestionText">Rank the fruits</p>
  <table>
    <tr><td>Apple</td>
        <td><label><input type="radio" name="r_1" value="1"> 1</label></td>
        <td><label><input type="radio" name="r_1" value="2"> 2</label></td>
        <td><label><input type="radio" name="r_1" value="3"> 3</label></td></tr>
    <tr><td>Banana</td>
        <td><label><input type="radio" name="r_2" value="1"> 1</label></td>
        <td><label><input type="radio" name="r_2" value="2"> 2</label></td>
        <td><label><input type="radio" name="r_2" value="3"> 3</label></td></tr>
    <tr><td>Cherry</td>
        <td><label><input type="radio" name="r_3" value="1"> 1</label></td>
        <td><label><input type="radio" name="r_3" value="2"> 2</label></td>
        <td><label><input type="radio" name="r_3" value="3"> 3</label></td></tr>
  </table>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

DROP_HTML = """<html><body><form method=post>
<div id="Q7">
  <p class="QuestionText">Choose your region</p>
  <select name="region">
    <option value="">-- Select --</option>
    <option value="1">North</option>
    <option value="2">South</option>
    <option value="3">East</option>
  </select>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""


def decide(html, decider, attempt=0, vmsgs=None):
    page = analyze(html, "http://x/test")
    decider.decide_page(page, attempt=attempt, validation_msgs=vmsgs or [])
    return page


def chosen(page):
    out = {}
    for q in page.questions:
        out[q.sig] = [f.label for f in q.chosen_options if f.selected or f.new_value]
        if q.kind == "dropdown":
            out[q.sig] = [o.label for sel in q.fields if sel.type == "select-one"
                          for o in sel.options if sel.new_value == o.value and o.value]
        out[q.sig] += [f"{f.name}={f.new_value}" for f in q.scalars if f.new_value is not None]
    return out


def test_strategy_first():
    page = decide(SA_HTML, Decider(TerminateMemory(":memory:"), strategy="first"))
    assert chosen(page)["QuestionDIV1"] == ["No"]


def test_memory_avoids_terminate():
    mem = TerminateMemory(":memory:")
    mem.mark_end("QuestionDIV1", "Are you employed?",
                 [{"label": "No", "value": "1"}], "terminate")
    for _ in range(10):
        page = decide(SA_HTML, Decider(mem, strategy="round-robin"))
        assert chosen(page)["QuestionDIV1"] == ["Yes"]


def test_instruction_beats_memory_and_strategy():
    mem = TerminateMemory(":memory:")
    mem.mark_end("QuestionDIV2", "license", [{"label": "Yes", "value": "2"}], "terminate")
    page = decide(INSTR_HTML, Decider(mem, strategy="first"))
    assert chosen(page)["QuestionDIV2"] == ["Yes"]


def test_multi_validation_raises_min():
    page = decide(MA_HTML, Decider(TerminateMemory(":memory:"), strategy="first"))
    assert chosen(page)["Q3"] == ["A"]          # default min = 1
    page = decide(MA_HTML, Decider(TerminateMemory(":memory:"), strategy="first"),
                  attempt=1, vmsgs=["Please select at least 4 options"])
    assert len(chosen(page)["Q3"]) == 4
    assert "None of these" not in chosen(page)["Q3"]   # exclusive kept out


def test_multi_never_mixes_exclusive():
    mem = TerminateMemory(":memory:")
    dec = Decider(mem, strategy="round-robin")
    for _ in range(20):
        page = decide(MA_HTML, Decider(mem, strategy="round-robin"),
                      attempt=1, vmsgs=["Please select at least 4 options"])
        got = chosen(page)["Q3"]
        if "None of these" in got:
            assert len(got) == 1


def test_number_from_instruction_range():
    page = decide(NUM_HTML, Decider(TerminateMemory(":memory:"), strategy="random", seed=1))
    age = int(chosen(page)["Q4"][0].split("=")[1])
    assert 18 <= age <= 25


def test_number_from_validation_range():
    page = decide(NUM_HTML, Decider(TerminateMemory(":memory:"), strategy="random", seed=3),
                  attempt=1, vmsgs=["Number must be between 18 and 25"])
    age = int(chosen(page)["Q4"][0].split("=")[1])
    assert 18 <= age <= 25


def test_constant_sum_totals_100():
    page = decide(CSUM_HTML, Decider(TerminateMemory(":memory:"), strategy="first"))
    got = chosen(page)["Q5"]
    total = sum(int(v.split("=")[1]) for v in got)
    assert total == 100


def test_ranking_assigns_unique_ranks():
    page = decide(RANK_HTML, Decider(TerminateMemory(":memory:"), strategy="first"))
    ranks = []
    for q in page.questions:
        for f in q.chosen_options:
            ranks.append((f.name, f.value))
    # one selection per row, values unique
    values = sorted(v for _, v in ranks)
    assert values == ["1", "2", "3"]


def test_dropdown_skips_placeholder_and_learns():
    mem = TerminateMemory(":memory:")
    page = decide(DROP_HTML, Decider(mem, strategy="first"))
    assert chosen(page)["Q7"] == ["North"]
    mem.mark_end("Q7", "region", [{"label": "North", "value": "1"}], "terminate")
    page = decide(DROP_HTML, Decider(mem, strategy="first"))
    assert chosen(page)["Q7"] == ["South"]


def test_persona_values():
    persona = Persona({"keywords": {"age": 34, "gender": "Male"}, "text": "because QA"})
    dec = Decider(TerminateMemory(":memory:"), persona=persona, strategy="first")
    page = decide(NUM_HTML, dec)
    assert chosen(page)["Q4"] == ["age=34"]

    gender_html = SA_HTML.replace("Are you employed?", "What is your gender?") \
        .replace("> No<", "> Female<").replace("> Yes<", "> Male<") \
        .replace('value="1"', 'value="1"').replace("value=1", "value=1")
    page = decide(gender_html, dec)
    assert chosen(page)["QuestionDIV1"] == ["Male"]


def test_constraints_from_validations():
    c = constraints_from_validations([
        "Please select at least 4 options",
        "Number must be between 18 and 25",
        "Minimum length of 30 required",
    ])
    assert c["min_select"] == 4
    assert c["range"] == (18.0, 25.0)
    assert c["min_length"] == 30


def test_memory_stats_persist_to_json(tmp_path):
    path = tmp_path / "m.json"
    mem = TerminateMemory(str(path))
    mem.mark_end("q", "t", [{"label": "No", "value": "1"}], "terminate")
    mem.save()
    data = json.loads(path.read_text())
    assert data["questions"]["q"]["ends"]["l:no"]["last_status"] == "terminate"


def test_learned_min_persists_across_deciders(tmp_path):
    """A validation message teaches the memory; a fresh decider starts at k=4."""
    mem = TerminateMemory(str(tmp_path / "m.json"))
    page = analyze(MA_HTML, "http://x/test")
    Decider(mem, strategy="first").decide_page(
        page, attempt=1, validation_msgs=["Please select at least 4 options"]
    )
    assert mem.learned("Q3", "min_select") == 4

    page2 = analyze(MA_HTML, "http://x/test")
    Decider(mem, strategy="first").decide_page(page2, attempt=0, validation_msgs=[])
    assert len(chosen(page2)["Q3"]) == 4
