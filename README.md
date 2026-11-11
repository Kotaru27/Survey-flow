# Survey Flow — Windows QA console

Survey Flow is a portable Go-based, rule-driven survey QA tool for **authorized test links**. It drives a browser through Chrome DevTools Protocol (CDP), analyzes questions with injected JavaScript, verifies selections before navigating, and records evidence of qualified and nonqualified outcomes.

## Run the application

1. Download **SurveyFlow-Recovery-Console** from a successful [Survey Flow Windows Console workflow](https://github.com/Kotaru27/Survey-flow/actions/workflows/windows-console-build.yml) and extract the ZIP to a writable directory. The **SurveyFlow-Verified-EXE** artifact contains only the executable.
2. Double-click `SurveyFlow.exe`. Chrome or Edge/Chromium must be available.
3. Paste an **approved QA/test survey URL**. For URLs without a recognizable test marker, the launcher requires a deliberate `TEST` confirmation; do not approve live fielding links.
4. Choose the target number of confirmed test qualifications. The console reports progress and stops on unverified or unsupported situations.

A qualified result requires the survey's explicit `Qual.html` completion URL on the **same origin** as the supplied link. Cross-origin completion redirects remain unverified until a study-specific approved destination is implemented. Generic thank-you text is not proof of qualification.

## QA evidence and state

- `reports/run-report.json`: latest run, with per-step question inventory, observed selection state, errors and validation details.
- `reports/batch-report.json`: latest batch; historical run and batch JSON files are also archived under unique names to avoid collisions across application launches.
- `data/launcher.log`: chronological activity and diagnostics.
- Study-specific persistent JSON memory: only verified successes are replayable, and unreadable memory blocks execution rather than being replaced.

### Optional exact multi-select replay

`config/app.json` includes `"replayVerifiedSets": false` by default. Changing this to `true` allows exact replay of **complete, verified checked-state multi-select sets from qualified test runs**. The option is deliberately off by default to preserve ordinary answer variety. Replay is skipped if options have changed or if current instructions, validation or existing pinned answers conflict.

## Limits and safety

This tool does **not** solve CAPTCHA, human verification challenges, inaccessible cross-origin frames, arbitrary image hotspots, file uploads, or every custom ranking/drag-and-drop control. Unknown red instructions and unresolved mandatory controls stop for review rather than being guessed. Tests use synthetic fixtures and do not establish correctness on every real Forsta/Confirmit survey.

The repository also retains an older Python implementation and `Testlink.bat`. The current Windows console is built from `src/` with `go build`; the older Python workflow is separate.

## Development and verification

- `go test ./src -count=1`
- `node tests/engine-regression.cjs`
- `python tests/validation-memory-browser.py` (requires Playwright and Chromium)
- CI runs additional browser fixtures, Windows end-to-end tests, concurrency/race checks, a Windows executable smoke test and icon embedding.

See `QUESTION_TYPES.md` for supported controls and `QA_REPORT.md` for evidence, known limitations and release conditions.
