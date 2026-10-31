# Proposed public cloud build

Prepared from Survey_Flow_source_checkpoint_2_NOT_RELEASE.zip. The proposed public GitHub branch contains the application source, configuration, tests, and Windows build workflow in this archive. Default study-specific configuration was replaced with empty rules; real survey URLs in the red-herring tests were replaced with fictional test addresses. No supplied survey XML, run reports, screenshots, respondent IDs or live links are included in this archive. Git branches in this repository are publicly visible.

The workflow runs synthetic Playwright browser fixtures on a GitHub-hosted Linux runner, and Go and JavaScript checks plus a Windows build on a GitHub-hosted Windows runner. If those checks succeed it uploads a Windows console candidate artifact. It does not run an actual CF survey. Test or build failures must be corrected before a candidate is offered for user evaluation.
