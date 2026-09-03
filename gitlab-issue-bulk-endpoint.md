# GitLab issue to file (orangebeard-io/team-soju)

Set iteration to current (`/iteration --current`) and add the `TODO` label per org convention.

## Title
Add generic bulk test-run import endpoint (POST /listener/v3/{projectName}/test-run/bulk)

## Description

### Problem
AI coding agents (and other non-framework callers) that perform manual/exploratory validation or ad-hoc test runs have no way to report results into Orangebeard except by driving the full start/finish REST sequence one call per item. This is complex, error-prone client-side work. listener-api already has `POST /v3/test-tool/junit/import`, which internally parses a JUnit XML file and drives the full start-run -> start-suite -> start-test -> log -> finish-test -> finish-run sequence server-side from ONE HTTP call, reusing TestRunV3Service/SuiteV3Service/TestV3Service/StepV3Service/LogV3Service. This issue generalizes that same internal logic behind a generic JSON parser instead of an XML parser, so any caller (not just JUnit-XML-emitting frameworks) gets the same one-call convenience.

### Endpoint
`POST /listener/v3/{projectName}/test-run/bulk`

Auth: inherits existing project-token auth (`Authorization: Bearer <token>`, `@PreAuthorize(HAS_TOKEN_ACCESS_TO_PROJECT)`) — no new auth mechanism.

#### Request body
Field names mirror the existing StartTestRun/StartSuiteRQ/StartTest/StartStep/LogRQ schemas 1:1. No client-supplied entity IDs — server mints every UUID, exactly like every other V3 endpoint.

```jsonc
{
  "idempotencyKey": "client-generated-uuid",       // optional
  "testSetName": "my-app-manual-validation",       // required, static identity key
  "startTime": "2026-09-02T10:00:00Z",             // required
  "endTime": "2026-09-02T10:05:00Z",               // required
  "description": "optional",
  "attributes": [{ "key": "branch", "value": "main" }],
  "suites": [                                       // optional, ordered, recursive
    {
      "name": "Checkout",
      "description": "optional", "attributes": [],
      "suites": [ /* nested sub-suites, recursive */ ],
      "tests": [
        {
          "testName": "Guest can complete purchase",
          "testType": "TEST",                          // TEST | BEFORE | AFTER, default TEST
          "status": "PASSED",                           // required, one of PASSED|FAILED|SKIPPED|STOPPED|TIMED_OUT
          "startTime": "...", "endTime": "...",
          "description": "optional", "attributes": [],
          "logs": [{ "logTime": "...", "message": "...", "logLevel": "INFO", "logFormat": "PLAIN_TEXT" }],
          "steps": [
            { "stepName": "Fill shipping form", "status": "PASSED",
              "startTime": "...", "endTime": "...", "logs": [],
              "steps": [ /* nested sub-steps */ ] }
          ]
        }
      ]
    }
  ]
}
```

#### Responses
- `201`: `{ "testRunUUID": "..." }` — accepted-and-enqueued (see Async note below), NOT processed/queryable yet.
- `400`: standard `ErrorResponse` schema extended with a `validationErrors` array of `{path, message}` covering ALL violations found (not just the first). An empty-but-structurally-valid document (`suites: []` or omitted) is accepted as a legitimate empty run, not a validation error.
- `409`: idempotency key reused with a different request body (ErrorResponse shape, no validationErrors array).

### Critical design constraints

1. **Async architecture (verified against TestRunV3Service.java)**: the whole V3 API is enqueue-then-return via Pub/Sub (`commonQueueService.queueRun(...)`), not synchronous. `finishTestRun` blocks on `ensureNoItemsInProgress`, which checks published-vs-consumed message counters. This endpoint MUST follow the same pattern: **validate the entire tree synchronously first**, and only if validation passes, enqueue the full sequence of start/log/finish messages, returning 201 immediately after enqueueing (not after Pub/Sub consumption). A malformed deep tree discovered mid-enqueue (if validation were skipped or partial) would leave an unrecoverable half-created run — this is why full-tree validation before any enqueueing is non-negotiable.

2. **History/trend identity**: Orangebeard's history continuity keys on `(project, testSetName, ordered suite-name path, testName)` (confirmed in TestRepository.getFullPath and test-results-api's history query). This endpoint doesn't need new logic here — it's inherited automatically by reusing the existing service calls — but it's why testSetName/suite names/test names are treated as required, static identity fields rather than optional metadata.

3. **BEFORE/AFTER identity carve-out** (prior confirmed learning, confidence 10/10): BEFORE/AFTER hook items commonly reuse generic names (e.g. `BeforeAll`) across genuinely different instances in the same suite. The existing retry-collapsing identity logic (TestRunStatusDeriver) is TEST-type-only. This endpoint must only emit items via the existing service calls and must NOT invent any new dedupe/collapse logic — BEFORE/AFTER items must be emitted as distinct items even when same-named.

4. **Idempotency**: optional client-generated `idempotencyKey`; server dedupes repeated requests carrying the same key within a bounded window (proposed default: 24h, TBD-confirm) per project, returning the original testRunUUID instead of creating a second run. Reusing a key with a different body returns 409 (never silently honored, never silently duplicated).

5. **Capability detection (client-side, informs response design)**: no separate capability/version endpoint needed. A calling client should be able to distinguish "route doesn't exist on this instance" from "route exists but request was rejected" by checking whether a 404 body parses as the standard ErrorResponse shape (business-logic 404, e.g. bad project) vs. any other body (framework-default/unmapped route). No special server work needed beyond ensuring 404s from actual business logic use the standard ErrorResponse shape (should already be the case).

6. **Payload limits** (confirmed): max 150,000 total items across suites+tests+steps combined — logs deliberately excluded from the count, since a single step can legitimately carry many log lines and that's not the tree-shape growth the limit exists to bound (raised from an initial 10,000 default — realistic sessions run ~10k tests x 15 steps each), max nesting depth 20 shared across suite and step nesting combined (not independent budgets). Rejected as a 400 validation error, not silent truncation, before any enqueueing. The per-violation message states the actual count, the limit, and what counts toward it.

### Out of scope for this issue
- Attachments (v1 requires a follow-up per-log `POST /attachment` call using returned UUIDs, same as today).
- The client-side skill/CLI that will call this endpoint (tracked separately, not blocked on this issue's exact implementation details beyond the contract above).
- API versioning/OpenAPI regen process specifics (open question, needs investigation of this repo's existing process).

### Definition of done
- `POST /listener/v3/{projectName}/test-run/bulk` implemented per the contract above, TDD'd (unit tests first).
- Component test proving the eventual TestRunFinishedEvent contains expected suites/tests/logs (assert via polling against the async event, not synchronously against the HTTP response).
- Explicit test proving BEFORE/AFTER items with duplicate names are emitted as distinct items, not collapsed.
- Test proving all validation errors in a tree are returned together in one 400 response, not just the first.
- Test proving idempotency key reuse (same body -> same testRunUUID returned, no duplicate run; different body -> 409).

Full design doc with cross-model (Codex) review and 3 rounds of adversarial spec review: `~/.gstack/projects/report-all-tests/tom-unknown-design-20260902-190628.md`.
