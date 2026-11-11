# Survey Flow — QA verification and release conditions

## Scope
This is an authorized survey **QA/test-link** application, not a system for generating live-production respondent submissions. A successful compilation or synthetic automated test does not prove that a new study's custom widgets, redirects or respondent constraints are supported.

## Checks required before distributing a candidate
A green `Survey Flow Windows Console` GitHub Actions run is required for the exact final commit. CI executes:
1. Go unit tests and Windows synthetic end-to-end browser journeys.
2. JavaScript engine parsing and behavior regressions.
3. Playwright fixtures, including a 22-page synthetic path, validation/instruction recovery, selection-state checks, and opted-in multi-select replay.
4. Go race tests for report writes, respondent test IDs, browser workers and stop handling.
5. Windows executable compilation, embedded icon verification, study-rule scoping and console smoke checks.
6. Uploads: `SurveyFlow-Verified-EXE`, `SurveyFlow-Recovery-Console`, and test screenshot evidence.

Build and tests must run **after all code changes**; earlier successful commits are not a substitute.

## Hardened behaviors
- The launcher validates HTTP(S) links and requires approval for URLs without a recognizable test marker. Marker matching is a convenience, not proof of authorization.
- Unreadable study memory or malformed private study rules prevent a normal test run.
- Corrupt app configuration is reported instead of silently falling back to defaults.
- Success requires a real `Qual.html` URL on the supplied survey's origin.
- Archived run/batch reports have names that cannot collide merely because the in-memory batch counter resets at startup.
- Write failures are surfaced, and fatal statuses stop scheduling new parallel attempts.
- Verified multi-select sets are stored only from successfully submitted, complete DOM snapshots. Exact replay is opt-in and checks the actual selected state.
- Browser cookie/storage isolation executes once per attempt; automatic worker admission respects the established RAM reserve.

## Limits still requiring study-specific evidence
- Live Forsta/Confirmit survey paths have **not** been run by the cloud CI. Use only approved QA links and verify independently.
- Cross-origin qualification endpoints are deliberately not counted by the conservative origin check.
- Complex custom dropdowns, ranking/sorting, image-only hotspots, cross-origin frames and advanced drag/drop are not universally supported.
- Scripted/human verification must stop for manual review.
- Success memory does not infer the truthfulness of demographics or other survey responses; fixture personas must be legitimate and appropriately configured.
- The older Python implementation is maintained separately and is not part of the new Go release binary.

## Release decision
Release the executable only when the exact final commit has a successful CI workflow and downloadable artifacts. A failed/partial CI run is **not** a verified release. Manual Windows testing on approved QA survey links remains recommended before broader office rollout.
