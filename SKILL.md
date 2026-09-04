---
name: report-all-tests
description: Report manual/exploratory validation or ad-hoc test results into Orangebeard as a real, queryable test run. Use whenever you (the agent) have just finished checking something step-by-step — a browser QA pass, a manual API smoke check, an exploratory bug hunt — and the result should be recorded, not left in ephemeral chat. Also use for framework test output (pytest/jest/mvn/etc.) when that framework has no Orangebeard reporter configured in this project.
---

# report-all-tests

Coding agents that do manual/exploratory validation — step-by-step browser
checks, ad-hoc API probes, exploratory bug hunts — normally leave no trace:
the results live only in chat and vanish. This skill turns that work into a
first-class Orangebeard test run with one JSON document and one CLI call.

Use this skill when you have just finished a round of manual or exploratory
checks and the user would plausibly want an audit trail (e.g. "was X tested
before this shipped?").

## When framework test output is involved

If an automated framework (pytest, jest, mvn, etc.) already has a dedicated
Orangebeard listener **configured in this project**, let that listener report
— don't duplicate its output through this skill.

If it does **not** have one configured, that's not a reason to skip
reporting: report the framework's results through this skill instead, so the
run still gets recorded. In that case, **tell the user** a dedicated
framework-specific reporter/listener exists for next time (e.g. "this project
runs pytest but has no Orangebeard pytest listener configured — I reported
this run through the generic bulk-import skill instead; consider setting up
the pytest listener for automatic reporting going forward").

## Prerequisites

1. The `orangebeard-report` binary is built (`go build -o orangebeard-report .`
   in this repo) and on `PATH`, or invoked by its full path.

2. **This project must have its own Orangebeard setup — check every time,
   don't assume it carries over.** Configuration is per-project
   (`.orangebeard/config.env` in the project you're currently working in),
   so a project you configured before tells you nothing about a different
   project. Before your first `report` call in *this* project:
   - Check whether `.orangebeard/config.env` already exists here. If it
     does, you're done — skip straight to reporting.
   - If it doesn't, you must gather and store three things by running
     `orangebeard-report init --endpoint <url> --token <token> --project <name>`
     **before** doing anything else:
     - **`--endpoint`** — the full base URL, exactly as the user gives it to
       you, **including any tenant-specific path segment**. Orangebeard
       instances can route by tenant through the URL itself (e.g.
       `https://app-acc.orangebeard.io/orangebeard`, where `/orangebeard` is
       the tenant, not just a path) — the server derives the tenant from
       this, so truncating it to a bare host silently breaks routing. Ask
       for and store the exact URL; don't reconstruct or shorten it.
     - **`--project`** — the Orangebeard project name.
     - **`--token`** — the project access token.
   - **Ask the user** for whichever of these you don't already have. Never
     invent, guess, or reuse a value from a different project's setup.

## How to report a run

1. **Structure: `suites` → `tests` → `steps`.** A run has **at least one
   suite** — a test can never live directly on the run, it always belongs to
   a suite (the JSON schema doesn't even offer a top-level `tests` field on
   the run for this reason). Use suites to group by feature area; nest
   sub-suites if that grouping is itself hierarchical.

2. **Document intent, not just outcome.** A reviewer reading this run later
   (possibly not you, possibly months later) needs to understand *what* was
   being checked and *why*, not just PASSED/FAILED. Cover this through
   whichever combination fits:
   - `description` fields at run/suite/test level, and/or
   - suite/test/step names that make intent self-evident from structure
     alone (e.g. `"Guest checkout rejects an expired card"` beats
     `"test 3"`).
   - **If the test is coded** (you wrote or ran an actual script/assertion,
     not just clicked around), attach a log with the source code. Use
     `"logFormat": "MARKDOWN"` and fence it as a code block so it renders
     readably.
   - **Always log three things** for a test (or step) that did real work:
     what you *expected* to happen, what you *observed* while it ran, and
     what the *actual outcome* was. This is what makes a run useful for
     debugging later instead of just a pass/fail tally.
   - **A failed test needs at least one `"logLevel": "ERROR"` log** that a
     human can read and immediately understand what broke — not a bare
     stack trace dump with no framing. State what was expected, what
     happened instead, and (if known) why.

3. **Static-naming discipline — this is the part most likely to go wrong.**
   Orangebeard's history/trend view keys on
   `(testSetName, ordered suite-name path, testName)`. If you're reporting
   what is conceptually "the same" recurring check (e.g. a smoke-test suite
   run on every deploy), reuse **byte-identical** names across runs — don't
   let a rephrase ("Checkout flow" vs "Checkout Flow") or an LLM-generated
   variation silently start a new history line.

   **Never bake a date, time, version number, build tag, or any other
   run-specific detail into `testSetName`, a suite `name`, or a `testName`**
   — not even for a run you think is a one-off. `"manual-qa-checkout-
   2026-09-03"` or `"checkout-v2.1"` looks harmless once, but the moment you
   (or anyone) runs a similar check again, it can no longer be recognized as
   the same test — you've silently guaranteed a fresh history line every
   time, which is precisely what static naming exists to prevent. Use a
   stable, descriptive name instead (`"manual-qa-checkout"`), and put the
   *when* in `startTime`/`endTime` (already required) or a `description` —
   fields that are expected to vary, not identity fields.

   **You have no memory of what you named things last session — check
   before you invent.** `orangebeard-report` automatically maintains
   `.orangebeard/reported-structure.json`, a ledger of every testSetName /
   suite path / testName this project has reported before (committed to
   version control, unlike `config.env`). Before writing a `testSetName` or
   suite/test name for what might be a recurring check, read that file if it
   exists. If a close match is already there, reuse it exactly rather than
   composing a fresh string — a plausible-looking paraphrase is exactly how
   a history line silently breaks. If you're genuinely unsure whether this
   is the same check as a prior entry, ask the user rather than guessing.

4. **Always include three run-level attributes:**
   - `"key": "reference_url"` — a URL back to this agent session, if the
     environment exposes one (e.g. a Claude Code session link). Omit only if
     genuinely no such URL exists; never fabricate one.
   - `"key": "Agent"` — an identifying value for which agent produced this
     run, e.g. `"Claude"`, `"Copilot"`, `"Codex"`.
   - `"key": "stage"` — a hint of *where in the process* these tests ran,
     e.g. `"building"` (mid-build checks on work in progress),
     `"regression"` (regression testing), `"feature"` (testing one specific
     feature). Pick whatever value best describes the moment, in your own
     words if none of the examples fit — the point is giving a reader a
     quick sense of intent (a rough draft sanity check reads very
     differently from a pre-release regression pass) without having to
     infer it from context.

5. **Write the JSON to a file** and run:
   ```sh
   orangebeard-report report path/to/run.json
   ```

6. **Report the result to the user honestly.** A successful call prints
   "submitted — accepted and enqueued", not "done": Orangebeard processes
   runs asynchronously, so the run may not be queryable in the UI for a few
   seconds after this call returns. Don't tell the user the run "is in
   Orangebeard" as a completed fact in the same breath as the CLI call — say
   it was submitted.

7. **Handle known error shapes** rather than surfacing a raw stack trace:
   - Validation error (exit 1, lists every violation): fix the document and
     retry — this is almost always a schema mistake (missing required
     field, bad status enum value), not a transient failure.
   - "this Orangebeard instance doesn't support bulk import yet": this
     instance predates the bulk endpoint. Tell the user; don't retry.
   - Any other failure (network, auth): surface it plainly and ask the user
     how to proceed rather than silently giving up on the reporting step.

## Example

A run with a passing coded test (source logged as markdown, expected/
observed/actual logged) and a failing manual test (mandatory ERROR log),
both attributed to the agent and session that produced them:

```json
{
  "testSetName": "manual-qa-checkout",
  "startTime": "2026-09-03T14:00:00Z",
  "endTime": "2026-09-03T14:12:00Z",
  "description": "Manual + scripted verification of the checkout flow after the payment-provider migration",
  "attributes": [
    { "key": "reference_url", "value": "https://claude.ai/code/session_01UE9nGcxRysQYL1rCAc8qP1" },
    { "key": "Agent", "value": "Claude" },
    { "key": "stage", "value": "regression" }
  ],
  "suites": [
    {
      "name": "Checkout",
      "tests": [
        {
          "testName": "Guest checkout total includes tax for a saved-card purchase",
          "status": "PASSED",
          "startTime": "2026-09-03T14:00:05Z",
          "endTime": "2026-09-03T14:03:10Z",
          "logs": [
            {
              "logTime": "2026-09-03T14:00:05Z",
              "logLevel": "INFO",
              "logFormat": "MARKDOWN",
              "message": "Ran the following check:\n\n```js\nconst total = await checkout.getTotal();\nassert.equal(total, subtotal * 1.0825);\n```"
            },
            { "logTime": "2026-09-03T14:00:06Z", "logLevel": "INFO", "logFormat": "PLAIN_TEXT", "message": "Expected: total equals subtotal * 1.0825 (8.25% tax)" },
            { "logTime": "2026-09-03T14:03:09Z", "logLevel": "INFO", "logFormat": "PLAIN_TEXT", "message": "Observed: getTotal() returned 108.25 for a 100.00 subtotal" },
            { "logTime": "2026-09-03T14:03:10Z", "logLevel": "INFO", "logFormat": "PLAIN_TEXT", "message": "Actual outcome: assertion passed, tax calculated correctly" }
          ]
        },
        {
          "testName": "Guest checkout rejects an expired card",
          "status": "FAILED",
          "startTime": "2026-09-03T14:05:00Z",
          "endTime": "2026-09-03T14:08:00Z",
          "logs": [
            { "logTime": "2026-09-03T14:05:00Z", "logLevel": "INFO", "logFormat": "PLAIN_TEXT", "message": "Expected: submitting an expired card shows 'Card expired' and blocks the order" },
            { "logTime": "2026-09-03T14:07:58Z", "logLevel": "INFO", "logFormat": "PLAIN_TEXT", "message": "Observed: no error message appeared; order confirmation page loaded" },
            { "logTime": "2026-09-03T14:08:00Z", "logLevel": "ERROR", "logFormat": "PLAIN_TEXT", "message": "Actual outcome: FAILED — the expired-card check did not block checkout. An order was created for a card with a past expiry date instead of being rejected with 'Card expired'. Likely cause: expiry validation only runs client-side and was bypassed by submitting the form directly." }
          ]
        }
      ]
    }
  ]
}
```

See `README.md` for the complete JSON contract, config setup, and error
handling.
