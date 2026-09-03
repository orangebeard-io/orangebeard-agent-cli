---
name: report-all-tests
description: Report manual/exploratory validation or ad-hoc test results into Orangebeard as a real, queryable test run. Use whenever you (the agent) have just finished checking something step-by-step — a browser QA pass, a manual API smoke check, an exploratory bug hunt — and the result should be recorded, not left in ephemeral chat.
---

# report-all-tests

Coding agents that do manual/exploratory validation — step-by-step browser
checks, ad-hoc API probes, exploratory bug hunts — normally leave no trace:
the results live only in chat and vanish. This skill turns that work into a
first-class Orangebeard test run with one JSON document and one CLI call.

Use this skill when you have just finished a round of manual or exploratory
checks and the user would plausibly want an audit trail (e.g. "was X tested
before this shipped?").

Do **not** use this for output from an existing automated test framework
(pytest, jest, mvn, etc.) — those already have dedicated Orangebeard
listeners that report automatically. This skill is for work an agent did
itself, that has no other reporter.

## Prerequisites

1. The `orangebeard-report` binary is built (`go build -o orangebeard-report .`
   in this repo) and on `PATH`, or invoked by its full path.
2. `.orangebeard/config.env` exists in the target project (run
   `orangebeard-report init --endpoint <url> --token <token> --project <name>`
   once if not). If it's missing and you don't have the endpoint/token,
   **ask the user** rather than guessing — do not invent credentials.

## How to report a run

1. **Build the JSON document** describing what you checked. Map your work
   onto `suites` → `tests` → `steps` (see `README.md` for the full schema).
   A single flat list of tests with no suite nesting is fine for a quick
   check; use suites when there's a natural grouping (e.g. by feature area).

2. **Static-naming discipline — this is the part most likely to go wrong.**
   Orangebeard's history/trend view keys on
   `(testSetName, ordered suite-name path, testName)`. If you're reporting
   what is conceptually "the same" recurring check (e.g. a smoke-test suite
   run on every deploy), reuse **byte-identical** names across runs — don't
   let a rephrase ("Checkout flow" vs "Checkout Flow") or an LLM-generated
   variation silently start a new history line. If this is a one-off
   exploratory session with no recurring identity, a descriptive
   `testSetName` (e.g. `"manual-qa-2026-09-03"`) is fine — static naming only
   matters once you intend the *same* test to be tracked over time.

3. **Write the JSON to a file** and run:
   ```sh
   orangebeard-report report path/to/run.json
   ```

4. **Report the result to the user honestly.** A successful call prints
   "submitted — accepted and enqueued", not "done": Orangebeard processes
   runs asynchronously, so the run may not be queryable in the UI for a few
   seconds after this call returns. Don't tell the user the run "is in
   Orangebeard" as a completed fact in the same breath as the CLI call — say
   it was submitted.

5. **Handle known error shapes** rather than surfacing a raw stack trace:
   - Validation error (exit 1, lists every violation): fix the document and
     retry — this is almost always a schema mistake (missing required
     field, bad status enum value), not a transient failure.
   - "this Orangebeard instance doesn't support bulk import yet": this
     instance predates the bulk endpoint. Tell the user; don't retry.
   - Any other failure (network, auth): surface it plainly and ask the user
     how to proceed rather than silently giving up on the reporting step.

## Example

```json
{
  "testSetName": "manual-qa-checkout-2026-09-03",
  "startTime": "2026-09-03T14:00:00Z",
  "endTime": "2026-09-03T14:12:00Z",
  "description": "Manual browser walkthrough of the checkout flow after the payment-provider migration",
  "suites": [
    {
      "name": "Checkout",
      "tests": [
        {
          "testName": "Guest can complete purchase with a saved card",
          "status": "PASSED",
          "startTime": "2026-09-03T14:00:05Z",
          "endTime": "2026-09-03T14:03:10Z",
          "steps": [
            { "stepName": "Add item to cart", "status": "PASSED", "startTime": "2026-09-03T14:00:05Z", "endTime": "2026-09-03T14:00:20Z" },
            { "stepName": "Complete payment", "status": "PASSED", "startTime": "2026-09-03T14:02:00Z", "endTime": "2026-09-03T14:03:10Z" }
          ]
        }
      ]
    }
  ]
}
```

See `README.md` for the complete JSON contract, config setup, and error
handling.
