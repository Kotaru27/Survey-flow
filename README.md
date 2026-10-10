# Survey-flow

Application to test Confirmit / Forsta (CF/QRE) survey links.

**Rule-based, no AI.** The tool opens a survey *test link*, analyzes the HTML of
each page, answers it, and walks the survey until **you** say how many completes
you need. It reads the **red bold instructions** some questions carry, remembers
any selection that ended a run (Terminate / Quota full) and avoids it in future
runs.

> Scope: this is a **survey QA tool**. Use it on TEST links with an approved test
> persona — not to fabricate respondents in live fielding. The runner refuses URLs
> that do not look like test links unless you pass `--allow-live`.

---

## How it decides an answer (priority order, pure rules)

1. **Red (usually bold) instruction on the question** — parsed with pattern rules
   (`Select: "Yes"`, `Choose option B`, `Enter: 25`, `Select Apple and Banana`, …)
   and matched against the actual option labels. An instruction always wins.
2. **Persona** — your `persona.json` gives consistent test answers (age, gender,
   email, …) so a screener answer never contradicts a later one.
3. **Memory-safe strategy** — a random / first / round-robin pick among the options
   **not** remembered as ending a run.

After a validation message appears, the message itself is parsed and the answer is
corrected on retry (`select at least 4` → select 4, `between 18 and 25` → pick
inside that range). Learned minimums are stored in the memory file so later runs
don't repeat the same validation.

## End statuses (kept distinct)

| Status | Meaning | Remembered? |
|---|---|---|
| **Complete** | reached the programmed complete/thank-you page | counted towards your target |
| **Terminate** (screened/disqualified) | failed a screener or screen-out rule | the selections on the last page are remembered and **avoided** |
| **Quota full** | reached a full quota cell | remembered and avoided (disable with `--avoid terminate`) |
| **Validation** | answer missing/invalid | corrected from the message, never confused with a terminate |

Detection is text/URL heuristics (terminate / quota / complete phrasings), checked
in that order because end pages often still say "Thank you".

## Install

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

Optional browser engine (for JS-driven pages):

```bash
.venv/bin/pip install -r requirements-browser.txt
.venv/bin/playwright install chromium
```

## Usage

```bash
# how many completes do you need?
.venv/bin/python -m survey_flow run 'https://your-survey-test-link...' --completes 10

# deterministic option mapping (good for finding terminate points)
.venv/bin/python -m survey_flow run URL --completes 5 --strategy round-robin

# with a consistent test persona + custom memory file
.venv/bin/python -m survey_flow run URL --completes 3 \
    --persona persona.json --memory my_memory.json

# inspect what the tool sees on a page (debugging)
.venv/bin/python -m survey_flow analyze URL

# inspect / clear what it has learned
.venv/bin/python -m survey_flow memory show
.venv/bin/python -m survey_flow memory clear

# JS-heavy survey
.venv/bin/python -m survey_flow run URL --completes 3 --engine browser --headed
```

Key options for `run`:

| Option | Default | Purpose |
|---|---|---|
| `--completes N` | *required* | how many completes you need |
| `--strategy random\|first\|round-robin` | `random` | answer choice when no red instruction exists (fatal options always excluded) |
| `--memory PATH` | `survey_memory.json` | persistent terminate memory |
| `--persona PATH` | – | consistent test answers (see `persona.example.json`) |
| `--avoid terminate,quota_full` | both | which end statuses to remember & avoid |
| `--engine http\|browser` | `http` | plain-forms engine or Playwright |
| `--max-attempts N` | `60` | safety cap on whole runs |
| `--seed N` | – | reproducible `random` strategy |
| `--trace PATH` | `traces.jsonl` | every run's full answer path (QID/answers/status) for QA logs |
| `--allow-live` | off | override the test-link guard |

Exit code `0` = target reached, `2` = gave up before target, `3` = refused/misconfigured.

## What the analyzer understands

The question-type handling follows the CF/QRE field guide you provided:

* **Single answer** (radio/buttons) and **multi answer** (checkboxes, min/max
  selections, exclusive options)
* **SA/MA image pickers** (behaves as radio/checkbox choices)
* **Dropdowns** (placeholder `-- Select --` is never chosen)
* **SA grids / Likert / rating / NPS** — one answer per required row; numeric
  scales (0–10) matched by number
* **Ranking** — unique positions assigned to every item
* **Constant sum** — points distributed to hit the stated total
* **Open text / text lists** — persona or default text, honours min/max length
* **Open numeric / numeric grids** — generated inside min/max or `between X and Y`
* **Date / time / slider (range)** — clamped into the allowed window
* **Other-specify** — text box filled only when *Other* is selected
* **Consent / eligibility checkboxes** and text-only **instruction pages**
* **Exclusive options** (`None of these`, `Don't know`, `Prefer not to answer`, …)
  are never mixed into a multi-select and are avoided when a minimum applies

Red/bold instruction detection works off inline styles, `<font color>`, `<b>/<strong>`
and class names — no JavaScript or AI involved.

## The terminate memory

`survey_memory.json` (git-ignored) stores, per stable question signature:

```json
{
  "questions": {
    "QuestionDIV1": {
      "text": "Are you currently employed?",
      "seen": 5,
      "ends": {
        "l:no": {"label": "No", "count": 1, "last_status": "terminate",
                  "last_message": "you do not qualify ..."}
      },
      "learned": {"min_select": 4}
    }
  },
  "stats": {"runs": 5, "completes": 3, "terminates": 1, "quota_full": 1}
}
```

Options are identified by **normalized label** (not position), so answer
randomization between sessions doesn't break the memory. If every option of a
question eventually becomes fatal, the tool picks anyway and keeps going.

## Tests

```bash
.venv/bin/python -m pytest tests/ -q
```

The suite includes an end-to-end run against a mock Confirmit-style survey
(consent → screener terminate → red-instruction question → multi-select with
validation → quota-full dropdown → grid → open text → complete) verifying that
the runner reaches N completes, remembers the terminate and quota selections,
and follows the red instructions.

## Known limitations

* The default `http` engine submits classic forms; if a survey is fully
  JS-rendered, `analyze` will report an empty structure — use `--engine browser`.
* Ranking via drag-and-drop widgets and hotspot/image-map clicks may need the
  browser engine; custom scripted questions should be reported (QID + behaviour)
  rather than guessed.
* Which answer triggers a terminate is attributed to the **last page answered**
  before the end screen (the trace file keeps the full path for review).
