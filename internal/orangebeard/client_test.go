package orangebeard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testRun() BulkTestRun {
	return BulkTestRun{
		TestSetName: "agent-manual-validation",
		StartTime:   "2026-09-03T10:00:00Z",
		EndTime:     "2026-09-03T10:05:00Z",
	}
}

func newClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return &Client{
		Endpoint:   srv.URL,
		Token:      "test-token",
		Project:    "my-project",
		HTTPClient: srv.Client(),
	}
}

func TestReport_Accepted(t *testing.T) {
	wantUUID := "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(wantUUID) // server returns a bare JSON string
	}))
	defer srv.Close()

	c := newClient(t, srv)
	gotUUID, err := c.Report(context.Background(), testRun())
	if err != nil {
		t.Fatalf("Report() error = %v, want nil", err)
	}
	if gotUUID != wantUUID {
		t.Errorf("Report() uuid = %q, want %q", gotUUID, wantUUID)
	}
	if want := "/listener/v3/my-project/test-run/bulk"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	if want := "Bearer test-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestReport_ValidationFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Arguments or content of the request is invalid!",
			"validationErrors": []map[string]string{
				{"path": "suites[0].tests[1].status", "message": "must not be blank"},
				{"path": "testSetName", "message": "must not be blank"},
			},
		})
	}))
	defer srv.Close()

	c := newClient(t, srv)
	_, err := c.Report(context.Background(), testRun())

	var valErr *ValidationFailedError
	if !asValidationFailed(err, &valErr) {
		t.Fatalf("Report() error = %v (%T), want *ValidationFailedError", err, err)
	}
	if len(valErr.ValidationErrors) != 2 {
		t.Errorf("ValidationErrors count = %d, want 2", len(valErr.ValidationErrors))
	}
	if valErr.ValidationErrors[0].Path != "suites[0].tests[1].status" {
		t.Errorf("ValidationErrors[0].Path = %q", valErr.ValidationErrors[0].Path)
	}
}

func TestReport_IdempotencyConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "idempotencyKey was already used with a different request body",
		})
	}))
	defer srv.Close()

	c := newClient(t, srv)
	_, err := c.Report(context.Background(), testRun())

	var conflictErr *ConflictError
	if !asConflict(err, &conflictErr) {
		t.Fatalf("Report() error = %v (%T), want *ConflictError", err, err)
	}
	if conflictErr.Message == "" {
		t.Error("ConflictError.Message is empty")
	}
}

func TestReport_NotFound_BusinessError(t *testing.T) {
	// A 404 whose body parses as the standard ErrorResponse shape (e.g. bad
	// project name) is an ordinary business error, not a capability gap.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Project not found!"})
	}))
	defer srv.Close()

	c := newClient(t, srv)
	_, err := c.Report(context.Background(), testRun())

	if err == ErrNotSupported {
		t.Fatalf("Report() error = ErrNotSupported, want a business UnexpectedStatusError")
	}
	var statusErr *UnexpectedStatusError
	if !asUnexpectedStatus(err, &statusErr) {
		t.Fatalf("Report() error = %v (%T), want *UnexpectedStatusError", err, err)
	}
	if statusErr.Message != "Project not found!" {
		t.Errorf("UnexpectedStatusError.Message = %q", statusErr.Message)
	}
}

func TestReport_NotFound_RouteUnmapped(t *testing.T) {
	// A 404 with a body that does NOT parse as ErrorResponse (empty, HTML, or
	// any framework default) means this Orangebeard instance predates the
	// bulk-import endpoint.
	for name, body := range map[string]string{
		"empty": "",
		"html":  "<html><body>404 Not Found</body></html>",
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			c := newClient(t, srv)
			_, err := c.Report(context.Background(), testRun())

			if err != ErrNotSupported {
				t.Fatalf("Report() error = %v, want ErrNotSupported", err)
			}
		})
	}
}

func TestReport_GeneratesIdempotencyKeyWhenMissing(t *testing.T) {
	var gotBody BulkTestRun
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode("00000000-0000-0000-0000-000000000000")
	}))
	defer srv.Close()

	c := newClient(t, srv)
	run := testRun()
	if run.IdempotencyKey != "" {
		t.Fatal("test setup: run.IdempotencyKey should start empty")
	}
	if _, err := c.Report(context.Background(), run); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if gotBody.IdempotencyKey == "" {
		t.Error("client did not generate an idempotencyKey for a request that omitted one")
	}
}

func TestReport_PreservesCallerSuppliedIdempotencyKey(t *testing.T) {
	var gotBody BulkTestRun
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode("00000000-0000-0000-0000-000000000000")
	}))
	defer srv.Close()

	c := newClient(t, srv)
	run := testRun()
	run.IdempotencyKey = "caller-chosen-key"
	if _, err := c.Report(context.Background(), run); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if gotBody.IdempotencyKey != "caller-chosen-key" {
		t.Errorf("IdempotencyKey = %q, want %q (caller-supplied key must survive untouched)", gotBody.IdempotencyKey, "caller-chosen-key")
	}
}

// --- errors.As helpers (avoids importing "errors" with unused-alias noise in every test) ---

func asValidationFailed(err error, target **ValidationFailedError) bool {
	if e, ok := err.(*ValidationFailedError); ok {
		*target = e
		return true
	}
	return false
}

func asConflict(err error, target **ConflictError) bool {
	if e, ok := err.(*ConflictError); ok {
		*target = e
		return true
	}
	return false
}

func asUnexpectedStatus(err error, target **UnexpectedStatusError) bool {
	if e, ok := err.(*UnexpectedStatusError); ok {
		*target = e
		return true
	}
	return false
}
