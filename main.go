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
	testRunUUID, err := client.Report(context.Background(), run)
	if err != nil {
		return describeReportError(err)
	}

	fmt.Printf("Submitted — run %s accepted and enqueued.\n", testRunUUID)
	fmt.Println("It is not queryable yet; Orangebeard processes it asynchronously.")
	return nil
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
