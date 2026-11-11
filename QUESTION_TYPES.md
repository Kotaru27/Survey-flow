# Survey Flow — Question-type support and operating rules

Scope: automated interaction with **authorized survey QA/test links**. This matrix documents the implemented source paths, not an assertion that every survey platform/widget is compatible. Successful interaction requires the website to expose usable controls and to accept the resulting state.

| Type | Recognition | Answer path | Status / limitation |
|---|---|---|---|
| Accordion/disclosure categories — single and multiple | `aria-expanded`, native `<details>/<summary>`, accordion/collapse/category heading/panel patterns | Expand category before answering, rescan newly rendered controls, obey single vs multi instructions, treat "None of the above" as exclusive | Browser-tested on representative fixtures and a 22-page synthetic journey; actual screenshot HTML not supplied, so site-specific class/DOM variants still need verification |
| Single choice (radio) | `input[type=radio]`, `[role=radio]` | Separate question groups, match profile/options, click, verify checked or ARIA state | Implemented; framework-specific options may need manual review |
| Multiple choice | Native checkboxes, checkbox/switch ARIA role, checkbox wording | Select set of choices, respect mutually exclusive “None” | Implemented; minimum/maximum survey-specific counts may need manual review |
| Yes/no, Likert, satisfaction scales | Ordinary single-choice question group | Choose offered value using profile or configured selection mode | Implemented through generic single choice; no factual inference |
| Dropdown | HTML `select` | Choose enabled option, send change/input, verify `value` | Native implemented; custom combobox/dropdown not universal |
| Matrix / per-brand grid | Grouped rows sharing a question/name | Detect row containers and answer each row before Next | Two-by-two detection regression tested; real framework layouts remain unverified |
| Open text / essay | Text inputs, textareas, contenteditable paths | Type value with keyboard/input/change events | Implemented with constraint verification; cannot infer truthful free-text from question alone |
| Numeric | `input[type=number]` | Clamp persona value within native min, max, step | Implemented; included in connected synthetic browser test |
| Date, time, month | Native date/time/month inputs | Format native values, clamp within native min/max | Implemented; complex datepickers not universal |
| Native slider | `input[type=range]` | Move to allowed step; dispatch input/change; require deliberate selection marker | Implemented; included in connected synthetic browser test |
| Sequential free-text | Custom page model | Visit distinct text slots and avoid filling all with same answer | Implemented heuristic; platform-specific |
| Carousel grid | Custom page model | Keep track of hidden slides and advance only while work remains | Implemented heuristic; platform-specific |
| Draggable scale / cards / timeline | Custom page models | Pointer/drag events or requested browser mouse drag, verify model state | Partial; not a general ranking/drag-and-drop solver |
| Image options | Image alt/title/option labels where exposed | Match declared option text/image alternatives | Partial; image-only options without labels need human review |
| Ranking / sorting / hotspots / uploads | Nonstandard scripted widgets | No universal answer adapter | Unsupported unless matched by an existing specialized adapter |
| Attention or instructional checks | Explicit instructions, study-bound rules, local styled red text even with nested bold elements | Position/label matches, multiple specified choices, accordion hidden-option order, study-specific matching | Unrecognized or unverified instructions prevent navigation; do not guess or copy between studies |
| CAPTCHA, verification wall | Page text or challenge frames | Report for human intervention | Not automated |

## Failure-handling contract

1. Each step identifies question groups independently and gathers unresolved controls.
2. Native radio/checkbox selections are re-read from actual checked state. Old `data-ap-selected` markers cannot override unchecked controls.
3. Local red instructions take precedence only within the matching question container (or the single question on the page).
4. Study identity includes the full stable URL query. Attention rules must declare a matching `studyKey` or `studyUrl`. No old survey's rules or rejection history is copied into a newly seen study.
5. Never force the survey's Next/Submit with unresolved controls. After three nonproductive attempts, report `needs_review` with question IDs/remaining count instead of silently submitting or looping.
6. A *completed* status must come from the actual `Qual.html` URL, not a generic thank-you message; screen-outs and quotas are separate.
7. For a new study with unknown screening or attention rules, a reviewer must supply legitimate QA fixture expectations rather than the engine guessing.

## How to inspect a failed question

Open `reports/run-report.json` and look at the last `stepsLog` item: `questionType`, `questions`, `openQuestions`, `action`, `unanswered`, `validation`, `controls`, `interaction`, and `probe`. `data/launcher.log` has the attention-check scan. QA fixtures with actual anonymized HTML are needed to add reliable support for a new custom widget—do not hardcode its wording or answer.

## Build and release

Console only. The Windows executable is cross-compiled from the source ZIP; the
GitHub Actions workflow builds a native Windows console executable. Both require
release QA against an actual Windows session before treating real-site behavior
as verified. See QA_REPORT.md for tested versus untested paths.

## Accordion and red/bold instruction update (23 October 2026)

- The engine recognizes semantic disclosure panels, typical accordion headers, and category/group headings with visible hidden panels. It clicks the header, then re-reads the newly available choices, rather than treating collapsed options as absent questions.
- Hidden controls remain in the ordinal order of a red instruction (for example, fourth option), so an earlier hidden section cannot shift the answer index.
- A visually boxed "None of the above" implemented as a separate radio alongside checkbox categories is mutually exclusive rather than an additional required answer. A checkbox section labeled "select one" is limited to one choice.
- A red instruction split across nested markup is combined into a single phrase. Supported commands include explicit option text, simple ordinals and multiple specified ordinal choices. Unsupported phrases remain in `needs_review`; selection never falls back to an uninstructed guess.
- Panels that change to expanded before asynchronous choices render are kept pending instead of allowing Next.

**Important:** These checks used representative DOM fixtures constructed from the screenshot's structure, *not* the actual survey's HTML. A site that uses icon-only nonsemantic click handlers, inaccessible shadow DOM or a different accordion implementation may still require a safe, targeted adapter based on its anonymized HTML.

## Loading-state safety (24 October reliability update)

- **Delayed question inputs:** When a visible question prompt appears before
  its answer controls, the engine waits for controls instead of prematurely
  clicking Next. After the bounded timeout, it requests review.
- **Accordion options:** Opening a category and waiting for asynchronous
  choices does not count as an unanswered attempt. Unrendered panels still
  require review after a bounded wait.
- **Completion versus termination:** `Qual.html` marks a qualified finish;
  `Term.html`, quota/screenout pages and unknown redirects must not be
  treated as qualified.

## Recovery testing (28 October 2026)

- 22 consecutive browser-test pages cover radio, multi-choice, dropdown placeholder,
  textarea, numeric, date, range, independent matrix rows, three consecutive SPA
  accordion questions, explicit/general red instructions, native disclosure,
  ARIA radios, delayed answers, time/month/datetime, and mixed groups.
- 3 negative-focused browser tests confirm rejected ARIA clicks do not create
  fake answer records, non-empty placeholders are not treated as answers, and a
  negated ordinal target is excluded.
- Supported controls are not equivalent to universal compatibility. Custom
  canvases, image-only options, cross-origin iframes and advanced drag/drop
  remain unverified.
