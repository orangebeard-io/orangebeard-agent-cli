# orangebeard-report

A dependency-free CLI that lets a coding agent (or any script) report a whole
test run — suites, tests, steps, and logs, arbitrarily nested — into
[Orangebeard](https://orangebeard.io) with one JSON document and one HTTP
call, instead of hand-rolling the real-time start/finish REST sequence.

It's a thin wrapper around Orangebeard's bulk test-run import endpoint:
`POST /listener/v3/{projectName}/test-run/bulk`
(orangebeard-io/team-soju#4781).

## Install

**Download a prebuilt binary** (no Go toolchain needed) from the
[latest release](https://github.com/orangebeard-io/orangebeard-agent-cli/releases/latest) —
a zero-dependency static binary for linux/darwin/windows (amd64+arm64).
Extract the archive and put `orangebeard-report` (or `orangebeard-report.exe`
on Windows) on your `PATH`.

**Or build from source** (requires Go 1.22+):

```sh
go install github.com/orangebeard-io/orangebeard-agent-cli@latest
```

```sh
git clone https://github.com/orangebeard-io/orangebeard-agent-cli
cd orangebeard-agent-cli
go build -o orangebeard-report .
```

Every merge to `main` cuts a new release automatically (`.github/workflows/release.yml`).

## Usage

```sh
# Once per project — writes .orangebeard/config.env (gitignore this: it holds a token)
orangebeard-report init --endpoint https://my-tenant.orangebeard.app --token <project-token> --project my-project

# Submit a run
orangebeard-report report path/to/run.json
```

A `201` means the run was validated and **enqueued**, not that it has
finished processing — Orangebeard's V3 API is asynchronous end to end. The
CLI's success message reflects that ("submitted", never "done").

## The JSON document

Field names mirror Orangebeard's real-time API 1:1. No IDs are supplied by
the caller — the server mints every UUID.

```jsonc
{
  "idempotencyKey": "…",                    // optional — the CLI generates one if omitted
  "testSetName": "my-app-manual-validation", // required, static (see below)
  "startTime": "2026-09-03T10:00:00Z",       // required
  "endTime": "2026-09-03T10:05:00Z",         // required
  "description": "optional",
  "attributes": [                            // see "Required attributes" below
    { "key": "reference_url", "value": "…" },
    { "key": "Agent", "value": "Claude" },
    { "key": "stage", "value": "regression" }
  ],
  "suites": [                                // at least one suite; tests never live directly on the run
    {
      "name": "Checkout",                    // required per suite
      "description": "optional",
      "attributes": [],
      "suites": [ /* nested sub-suites, recursive */ ],
      "tests": [
        {
          "testName": "Guest can complete purchase", // required
          "testType": "TEST",                // TEST | BEFORE | AFTER, default TEST
          "status": "PASSED",                // required unless the test has steps; one of PASSED|FAILED|SKIPPED|STOPPED|TIMED_OUT
          "startTime": "…", "endTime": "…",
          "description": "optional",
          "attributes": [],
          "logs": [{ "logTime": "…", "message": "…", "logLevel": "INFO", "logFormat": "PLAIN_TEXT" }],
          "steps": [                          // optional, ordered, recursive
            { "stepName": "Fill shipping form", "status": "PASSED",
              "startTime": "…", "endTime": "…", "logs": [],
              "steps": [ /* nested sub-steps */ ] }
          ]
        }
      ]
    }
  ]
}
```

`logLevel` is one of `DEBUG | INFO | WARN | ERROR`. `logFormat` is one of
`PLAIN_TEXT | HTML | MARKDOWN` — use `MARKDOWN` to log source code as a
fenced code block.

### Required attributes

Every run should carry these three run-level `attributes`:

- **`reference_url`** — a link back to the agent session that produced this
  run, if the environment exposes one. Omit if none exists; never fabricate
  one.
- **`Agent`** — an identifying value for which agent produced the run (e.g.
  `"Claude"`, `"Copilot"`, `"Codex"`).
- **`stage`** — where in the process these tests ran, e.g. `"building"`
  (mid-build checks on work in progress), `"regression"` (regression
  testing), `"feature"` (testing one specific feature). Free-form — pick
  whatever value best describes the moment.

### Documenting intent, not just outcome

A run should be legible to someone who wasn't there when it ran:

- Use `description` fields and/or self-evident suite/test/step naming to
  convey what was being checked and why.
- If a test is coded, log its source (`logFormat: "MARKDOWN"`, fenced as a
  code block).
- Log the expected outcome, what was observed, and the actual outcome for
  each test/step that did real work.
- A `FAILED` test requires at least one `logLevel: "ERROR"` log stating,
  in terms a human can act on, what broke and why — not a bare stack trace.

See `SKILL.md` for a worked example combining all of the above.

### Static-naming discipline — read this before generating a document

Orangebeard's history/trend continuity keys on
**`(project, testSetName, ordered suite-name path, testName)`**. If any of
those change between runs, that test's timeline breaks — Orangebeard sees a
brand-new test, not a continuation. When an agent generates this document
across multiple sessions or runs for the "same" logical test suite:

- `testSetName`, suite `name`s, and `testName`s must be **byte-identical**
  across runs — not paraphrased, not re-derived per run.
- `BEFORE`/`AFTER` items are the one exception: reusing the same name (e.g.
  `BeforeAll`) across genuinely different instances is expected and normal —
  Orangebeard does not collapse them.

### Errors

- **400** — the whole document was rejected; every violation is listed
  together (`path` + `message`), not just the first.
- **409** — the `idempotencyKey` was already used with a different request
  body (a genuine retry always resends the identical body).
- **404** — the CLI distinguishes an ordinary business 404 (e.g. unknown
  project — printed as-is) from a 404 that means *this Orangebeard instance
  predates the bulk-import endpoint* (printed as "this Orangebeard instance
  doesn't support bulk import yet").

## Development

TDD: `internal/orangebeard` has full unit-test coverage of the HTTP
response-handling branches (`client_test.go`, via `httptest.Server`) and the
config round-trip (`config_test.go`). Run:

```sh
go test ./...
```

## Scope / out of scope

Current scope is `init` + `report`, matching the approved design's 48-hour
cut. Explicitly deferred: a `junit` subcommand (thin wrapper around the
pre-existing `POST /v3/test-tool/junit/import`), attachments, and
package-manager publishing. Tracked in orangebeard-io/team-soju#4783.

Distributed from [github.com/orangebeard-io/orangebeard-agent-cli](https://github.com/orangebeard-io/orangebeard-agent-cli),
vendored by pinned git ref or a tagged release — see #4783.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
