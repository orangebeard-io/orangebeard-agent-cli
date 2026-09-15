// Command orangebeard-report is a thin, dependency-free CLI that lets a
// coding agent report a whole test run (suites, tests, steps, logs) into
// Orangebeard with one JSON document and one HTTP call, via the bulk
// test-run import endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/orangebeard-io/orangebeard-agent-cli/internal/orangebeard"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "-v", "--version", "version":
		fmt.Println("orangebeard-report", version)
		return
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `orangebeard-report — report agent test runs into Orangebeard

Usage:
  orangebeard-report init --endpoint URL --token TOKEN --project NAME
      Writes connection details to .orangebeard/config.env (gitignore this).

  orangebeard-report report <path-to-bulk-run.json>
      Reads a bulk test-run JSON document, posts it in one call, and prints
      the resulting testRunUUID.

See README.md for the JSON document's shape and the static-naming rules
(testSetName / suite path / testName) that Orangebeard's history view relies
on staying identical across runs.
`)
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "Orangebeard endpoint, e.g. https://my-tenant.orangebeard.app (required)")
	token := fs.String("token", "", "Orangebeard project access token (required)")
	project := fs.String("project", "", "Orangebeard project name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var missing []string
	if *endpoint == "" {
		missing = append(missing, "--endpoint")
	}
	if *token == "" {
		missing = append(missing, "--token")
	}
	if *project == "" {
		missing = append(missing, "--project")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %v", missing)
	}

	cfg := orangebeard.Config{Endpoint: *endpoint, Token: *token, Project: *project}
	if err := orangebeard.SaveConfig(".", cfg); err != nil {
		return err
	}
	fmt.Println("Wrote .orangebeard/config.env — add .orangebeard/ to .gitignore, it holds a project access token.")
	return nil
}

func runReport(args []string) error {
	maybeNotifyUpdate()

	if len(args) != 1 {
		return errors.New("usage: orangebeard-report report <path-to-bulk-run.json>")
	}
	path := args[0]

	cfg, err := orangebeard.LoadConfig(".")
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var run orangebeard.BulkTestRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	client := &orangebeard.Client{Endpoint: cfg.Endpoint, Token: cfg.Token, Project: cfg.Project}
	ctx := context.Background()
	resp, err := client.Report(ctx, run)
	// ErrAttachmentsNotSupported is the one Report error that still carries a
	// usable response: the run itself was created (an old server just
	// ignores the unrecognized "attachments" field) — only the attachment
	// upload step is unavailable, so the submission is reported as a
	// success and attachments are skipped below instead of aborting.
	if err != nil && !errors.Is(err, orangebeard.ErrAttachmentsNotSupported) {
		return describeReportError(err)
	}

	fmt.Printf("Submitted — run %s accepted and enqueued.\n", resp.TestRunUUID)
	fmt.Println("It is not queryable yet; Orangebeard processes it asynchronously.")

	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err, "— declared attachments were not uploaded.")
	} else if uerr := uploadAttachments(ctx, client, run, resp); uerr != nil {
		fmt.Fprintln(os.Stderr, "warning: could not upload attachments:", uerr)
	}

	recordStructure(run)
	return nil
}

// uploadAttachments uploads every attachment the run declared and reports
// per-file success/failure. It's always safe to call — a run with nothing
// declared just does nothing. The run itself was already accepted by the
// time this runs, so an upload failure here is reported but doesn't fail
// the command.
func uploadAttachments(ctx context.Context, client *orangebeard.Client, run orangebeard.BulkTestRun, resp *orangebeard.BulkImportResponse) error {
	results, err := client.UploadAttachments(ctx, run, resp)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}

	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "warning: attachment %q (%s) failed to upload: %v\n", r.FileName, r.AttachmentRef, r.Err)
		} else {
			fmt.Printf("Uploaded attachment %q.\n", r.FileName)
		}
	}
	fmt.Printf("%d/%d attachment(s) uploaded.\n", len(results)-failed, len(results))
	fmt.Println("The server holds the run open only briefly for declared attachments — one that never arrives is dropped with a server-side warning, not a hard failure.")
	return nil
}

// recordStructure updates the per-project ledger of testSetName/suite-path/
// testName strings this project has reported before (.orangebeard/reported-
// structure.json), so a future session can reuse exact names instead of
// re-deriving a paraphrase. Only called after a successful submission. A
// failure here is non-fatal — the run itself was already accepted — so it's
// reported but doesn't fail the command.
func recordStructure(run orangebeard.BulkTestRun) {
	idx, err := orangebeard.LoadStructureIndex(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not update .orangebeard/reported-structure.json:", err)
		return
	}
	orangebeard.MergeStructure(idx, run, time.Now())
	if err := orangebeard.SaveStructureIndex(".", idx); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not update .orangebeard/reported-structure.json:", err)
	}
}

func describeReportError(err error) error {
	var valErr *orangebeard.ValidationFailedError
	if errors.As(err, &valErr) {
		msg := valErr.Message + "\n"
		for _, v := range valErr.ValidationErrors {
			msg += fmt.Sprintf("  - %s: %s\n", v.Path, v.Message)
		}
		return errors.New(msg)
	}

	var conflictErr *orangebeard.ConflictError
	if errors.As(err, &conflictErr) {
		return fmt.Errorf("idempotency conflict: %s", conflictErr.Message)
	}

	if errors.Is(err, orangebeard.ErrNotSupported) {
		return orangebeard.ErrNotSupported
	}

	return err
}
