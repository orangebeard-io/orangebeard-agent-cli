package orangebeard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file %s: %v", path, err)
	}
	return path
}

func TestHasAttachments(t *testing.T) {
	if hasAttachments(testRun()) {
		t.Error("hasAttachments() = true for a run with no attachments")
	}
	if !hasAttachments(testRunWithAttachment("shot1")) {
		t.Error("hasAttachments() = false for a run with a declared attachment")
	}
}

func TestHasAttachments_NestedUnderStep(t *testing.T) {
	run := testRun()
	run.Suites = []Suite{{
		Name: "Checkout",
		Tests: []Test{{
			TestName: "Guest checkout fails",
			Steps: []Step{{
				StepName: "Submit payment",
				Logs: []Log{{
					LogTime:     "2026-09-03T10:01:00Z",
					Message:     "trace",
					Attachments: []AttachmentRef{{AttachmentRef: "trace1", FileName: "trace.zip"}},
				}},
			}},
		}},
	}}
	if !hasAttachments(run) {
		t.Error("hasAttachments() = false for an attachment declared on a step's log")
	}
}

func TestCollectAttachments_JoinsByRef(t *testing.T) {
	path := writeTempFile(t, "shot.png", "fake-png-bytes")
	run := testRun()
	run.Suites = []Suite{{
		Name: "Checkout",
		Tests: []Test{{
			TestName: "Guest checkout fails",
			Logs: []Log{{
				LogTime:     "2026-09-03T10:01:00Z",
				Message:     "screenshot",
				Attachments: []AttachmentRef{{AttachmentRef: "shot1", FileName: "shot.png", Path: path}},
			}},
			Steps: []Step{{
				StepName: "Submit payment",
				Logs: []Log{{
					LogTime:     "2026-09-03T10:02:00Z",
					Message:     "trace",
					Attachments: []AttachmentRef{{AttachmentRef: "trace1", FileName: "trace.zip", Path: path}},
				}},
			}},
		}},
	}}
	resp := &BulkImportResponse{
		TestRunUUID: "run-uuid",
		Attachments: []AttachmentTarget{
			{AttachmentRef: "shot1", TestUUID: "test-uuid", LogUUID: "log-uuid-1"},
			{AttachmentRef: "trace1", TestUUID: "test-uuid", StepUUID: "step-uuid", LogUUID: "log-uuid-2"},
		},
	}

	pending, err := collectAttachments(run, resp)
	if err != nil {
		t.Fatalf("collectAttachments() error = %v, want nil", err)
	}
	if len(pending) != 2 {
		t.Fatalf("collectAttachments() = %d entries, want 2", len(pending))
	}
	// Sorted by AttachmentRef: "shot1" < "trace1".
	if pending[0].AttachmentRef != "shot1" || pending[0].LogUUID != "log-uuid-1" || pending[0].StepUUID != "" {
		t.Errorf("pending[0] = %+v, want shot1/log-uuid-1/no step", pending[0])
	}
	if pending[1].AttachmentRef != "trace1" || pending[1].StepUUID != "step-uuid" || pending[1].LogUUID != "log-uuid-2" {
		t.Errorf("pending[1] = %+v, want trace1/step-uuid/log-uuid-2", pending[1])
	}
}

func TestCollectAttachments_NoneDeclared_ReturnsEmpty(t *testing.T) {
	pending, err := collectAttachments(testRun(), &BulkImportResponse{TestRunUUID: "run-uuid"})
	if err != nil {
		t.Fatalf("collectAttachments() error = %v, want nil", err)
	}
	if len(pending) != 0 {
		t.Errorf("collectAttachments() = %v, want empty", pending)
	}
}

func TestCollectAttachments_MissingFromResponse_ReturnsError(t *testing.T) {
	run := testRunWithAttachment("shot1")
	resp := &BulkImportResponse{TestRunUUID: "run-uuid"} // server echoed nothing back

	_, err := collectAttachments(run, resp)
	if err == nil {
		t.Fatal("collectAttachments() error = nil, want an error for a ref missing from the response")
	}
	if !strings.Contains(err.Error(), "shot1") {
		t.Errorf("collectAttachments() error = %q, want it to name the missing ref %q", err, "shot1")
	}
}

func TestUploadAttachments_Success(t *testing.T) {
	path := writeTempFile(t, "shot.png", "fake-png-bytes")
	run := testRunWithAttachment("shot1")
	run.Suites[0].Tests[0].Logs[0].Attachments[0].Path = path

	var gotPath, gotAuth, gotJSON, gotFileContent, gotFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotJSON = r.FormValue("json")
		f, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		gotFilename = header.Filename
		defer f.Close()
		buf := make([]byte, 64)
		n, _ := f.Read(buf)
		gotFileContent = string(buf[:n])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	resp := &BulkImportResponse{
		TestRunUUID: "run-uuid",
		Attachments: []AttachmentTarget{{AttachmentRef: "shot1", TestUUID: "test-uuid", LogUUID: "log-uuid"}},
	}

	results, err := c.UploadAttachments(context.Background(), run, resp)
	if err != nil {
		t.Fatalf("UploadAttachments() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Fatalf("UploadAttachments() = %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("results[0].Err = %v, want nil", results[0].Err)
	}
	if want := "/listener/v3/my-project/attachment"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	if want := "Bearer test-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
	if !strings.Contains(gotJSON, `"testUUID":"test-uuid"`) || !strings.Contains(gotJSON, `"logUUID":"log-uuid"`) || !strings.Contains(gotJSON, `"attachmentRef":"shot1"`) {
		t.Errorf("json field = %q, missing expected testUUID/logUUID/attachmentRef", gotJSON)
	}
	if gotFileContent != "fake-png-bytes" {
		t.Errorf("uploaded file content = %q, want %q", gotFileContent, "fake-png-bytes")
	}
	// Declared FileName is "fail.png" but the local path's basename is
	// "shot.png" (a temp file) - the multipart filename must follow the
	// declared name, not leak the local path's basename.
	if gotFilename != "fail.png" {
		t.Errorf("uploaded multipart filename = %q, want %q", gotFilename, "fail.png")
	}
}

func TestUploadAttachments_ContinuesPastPerFileFailure(t *testing.T) {
	goodPath := writeTempFile(t, "good.png", "ok")
	run := testRun()
	run.Suites = []Suite{{
		Name: "Checkout",
		Tests: []Test{{
			TestName: "Guest checkout fails",
			Logs: []Log{
				{LogTime: "2026-09-03T10:01:00Z", Message: "a", Attachments: []AttachmentRef{{AttachmentRef: "bad", FileName: "missing.png", Path: "/no/such/file.png"}}},
				{LogTime: "2026-09-03T10:02:00Z", Message: "b", Attachments: []AttachmentRef{{AttachmentRef: "good", FileName: "good.png", Path: goodPath}}},
			},
		}},
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	resp := &BulkImportResponse{
		TestRunUUID: "run-uuid",
		Attachments: []AttachmentTarget{
			{AttachmentRef: "bad", TestUUID: "test-uuid", LogUUID: "log-1"},
			{AttachmentRef: "good", TestUUID: "test-uuid", LogUUID: "log-2"},
		},
	}

	results, err := c.UploadAttachments(context.Background(), run, resp)
	if err != nil {
		t.Fatalf("UploadAttachments() error = %v, want nil (per-file failures shouldn't abort the batch)", err)
	}
	if len(results) != 2 {
		t.Fatalf("UploadAttachments() = %d results, want 2", len(results))
	}
	byRef := map[string]AttachmentResult{}
	for _, r := range results {
		byRef[r.AttachmentRef] = r
	}
	if byRef["bad"].Err == nil {
		t.Error(`results["bad"].Err = nil, want an open-file error`)
	}
	if byRef["good"].Err != nil {
		t.Errorf(`results["good"].Err = %v, want nil`, byRef["good"].Err)
	}
}

func TestUploadAttachments_ServerError(t *testing.T) {
	path := writeTempFile(t, "shot.png", "bytes")
	run := testRunWithAttachment("shot1")
	run.Suites[0].Tests[0].Logs[0].Attachments[0].Path = path

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Attachment too large"}`))
	}))
	defer srv.Close()

	c := newClient(t, srv)
	resp := &BulkImportResponse{
		TestRunUUID: "run-uuid",
		Attachments: []AttachmentTarget{{AttachmentRef: "shot1", TestUUID: "test-uuid", LogUUID: "log-uuid"}},
	}

	results, err := c.UploadAttachments(context.Background(), run, resp)
	if err != nil {
		t.Fatalf("UploadAttachments() error = %v, want nil", err)
	}
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("UploadAttachments() results = %+v, want one failed result", results)
	}
	if !strings.Contains(results[0].Err.Error(), "Attachment too large") {
		t.Errorf("results[0].Err = %v, want it to surface the server message", results[0].Err)
	}
}

func TestUploadAttachments_NoneDeclared_MakesNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	results, err := c.UploadAttachments(context.Background(), testRun(), &BulkImportResponse{TestRunUUID: "run-uuid"})
	if err != nil {
		t.Fatalf("UploadAttachments() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("UploadAttachments() = %v, want empty", results)
	}
	if called {
		t.Error("UploadAttachments() made an HTTP call for a run with no declared attachments")
	}
}
