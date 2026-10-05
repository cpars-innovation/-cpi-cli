---
name: cpi-review
description: Review an SAP Cloud Integration flow or a change to one against .cpi/conventions.md, the plan, the tenant's check and design guidelines and general CPI good practice, and report findings by severity. Use when the user asks for a review, before a release or transport, or after cpi-build finished a larger change.
---

# Review an integration flow

Delegate the review to the **cpi-reviewer** agent so that it judges the result with a fresh
context and read-only tools. Give it:

- the flow ID(s) and the local folder(s) of the files,
- what changed (the git diff range or "new flow"),
- the plan file if there is one (`.cpi/plans/<FlowId>.md`),
- test results you have (case table from cpi-test),
- the full text of [review-checklist.md](review-checklist.md) (the agent cannot read plugin files).

When it returns, present its findings to the user grouped by severity. Do not fix anything
in the same step: ask the user which findings to address, then use cpi-build.

If the user wants the review without the agent, or there is no cpi-reviewer agent (agents
other than Claude Code install only the skills), do the review yourself: follow the checklist in
[review-checklist.md](review-checklist.md), use only read tools (no uploads, deployments or file
changes), judge the result rather than the intention, and report findings by severity.
