package orangebeard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AttachmentResult is the outcome of uploading one declared attachment. A
// failed upload is recorded in Err rather than aborting the rest of the
// batch — one bad file shouldn't cost the others.
type AttachmentResult struct {
	AttachmentRef string
	FileName      string
	Err           error
}

// pendingAttachment is one declared attachment joined with the UUIDs the
// server echoed back for it — everything uploadAttachment needs.
type pendingAttachment struct {
	TestRunUUID   string
	TestUUID      string
	StepUUID      string
	LogUUID       string
	AttachmentRef string
	FileName      string
	Path          string
	ContentType   string
}

// attachmentMetaData mirrors listener-api's AttachmentMetaData request DTO
// for the existing per-log /attachment endpoint, plus the attachmentRef
// this bulk-import flow adds so the server can match the upload against its
// pending-attachment bookkeeping.
type attachmentMetaData struct {
	TestRunUUID    string `json:"testRunUUID"`
	TestUUID       string `json:"testUUID"`
	StepUUID       string `json:"stepUUID,omitempty"`
	LogUUID        string `json:"logUUID"`
	AttachmentTime string `json:"attachmentTime"`
	AttachmentRef  string `json:"attachmentRef,omitempty"`
}

// hasAttachments reports whether run declares any attachments anywhere in
// its suite/test/step tree.
func hasAttachments(run BulkTestRun) bool {
	found := false
	walkLogs(run.Suites, func(l Log) {
		if len(l.Attachments) > 0 {
			found = true
		}
	})
	return found
}

// walkLogs calls fn for every Log in run's suite/test/step tree, at any
// nesting depth.
func walkLogs(suites []Suite, fn func(Log)) {
	for _, s := range suites {
		for _, t := range s.Tests {
			for _, l := range t.Logs {
				fn(l)
			}
			walkStepLogs(t.Steps, fn)
		}
		walkLogs(s.Suites, fn)
	}
}

func walkStepLogs(steps []Step, fn func(Log)) {
	for _, st := range steps {
		for _, l := range st.Logs {
			fn(l)
		}
		walkStepLogs(st.Steps, fn)
	}
}

// collectAttachments joins run's declared attachments with resp's echoed
// UUIDs by attachmentRef — no tree-walk correlation needed, since the
// server already returns exactly the fields a follow-up /attachment call
// needs, keyed by the ref the client itself assigned. A declared ref the
// response doesn't cover is a structural error (version/contract mismatch),
// not a per-file failure, so it's returned rather than silently dropped.
func collectAttachments(run BulkTestRun, resp *BulkImportResponse) ([]pendingAttachment, error) {
	declared := map[string]AttachmentRef{}
	walkLogs(run.Suites, func(l Log) {
		for _, a := range l.Attachments {
			declared[a.AttachmentRef] = a
		}
	})
	if len(declared) == 0 {
		return nil, nil
	}

	targets := make(map[string]AttachmentTarget, len(resp.Attachments))
	for _, t := range resp.Attachments {
		targets[t.AttachmentRef] = t
	}

	pending := make([]pendingAttachment, 0, len(declared))
	var missing []string
	for ref, decl := range declared {
		target, ok := targets[ref]
		if !ok {
			missing = append(missing, ref)
			continue
		}
		pending = append(pending, pendingAttachment{
			TestRunUUID:   resp.TestRunUUID,
			TestUUID:      target.TestUUID,
			StepUUID:      target.StepUUID,
			LogUUID:       target.LogUUID,
			AttachmentRef: ref,
			FileName:      decl.FileName,
			Path:          decl.Path,
			ContentType:   decl.ContentType,
		})
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("server response is missing %d declared attachment(s): %s", len(missing), strings.Join(missing, ", "))
	}

	sort.Slice(pending, func(i, j int) bool { return pending[i].AttachmentRef < pending[j].AttachmentRef })
	return pending, nil
}

// UploadAttachments uploads every attachment run declared, one per-log
// POST /attachment call using the UUIDs resp echoed back. It returns one
// AttachmentResult per declared attachment — a failed upload is recorded in
// that entry's Err rather than aborting the rest. The only top-level error
// is a structural mismatch between what was declared and what the server
// echoed back; a run with no declared attachments returns an empty slice
// and a nil error, so callers can call this unconditionally after Report.
func (c *Client) UploadAttachments(ctx context.Context, run BulkTestRun, resp *BulkImportResponse) ([]AttachmentResult, error) {
	pending, err := collectAttachments(run, resp)
	if err != nil {
		return nil, err
	}

	results := make([]AttachmentResult, 0, len(pending))
	for _, p := range pending {
		results = append(results, AttachmentResult{
			AttachmentRef: p.AttachmentRef,
			FileName:      p.FileName,
			Err:           c.uploadAttachment(ctx, p),
		})
	}
	return results, nil
}

// uploadAttachment streams p.Path's contents (rather than buffering the
// whole file) into a multipart POST /v3/{project}/attachment call, matching
// what AttachmentV3Controller expects: a "json" form field (attachmentMetaData)
// and an "attachment" file part.
func (c *Client) uploadAttachment(ctx context.Context, p pendingAttachment) error {
	file, err := os.Open(p.Path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", p.Path, err)
	}
	defer file.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		err := func() error {
			meta := attachmentMetaData{
				TestRunUUID:    p.TestRunUUID,
				TestUUID:       p.TestUUID,
				StepUUID:       p.StepUUID,
				LogUUID:        p.LogUUID,
				AttachmentTime: time.Now().UTC().Format(time.RFC3339),
				AttachmentRef:  p.AttachmentRef,
			}
			metaJSON, err := json.Marshal(meta)
			if err != nil {
				return fmt.Errorf("encoding attachment metadata: %w", err)
			}
			if err := writer.WriteField("json", string(metaJSON)); err != nil {
				return err
			}

			contentType := p.ContentType
			if contentType == "" {
				contentType = mimeTypeFor(p.FileName)
			}
			part, err := createFormFile(writer, "attachment", filepath.Base(p.Path), contentType)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, file); err != nil {
				return err
			}
			return writer.Close()
		}()
		pw.CloseWithError(err)
	}()

	target := fmt.Sprintf("%s/listener/v3/%s/attachment", c.Endpoint, url.PathEscape(c.Project))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, pr)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpResp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", target, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if httpResp.StatusCode == http.StatusCreated {
		return nil
	}
	if msg, ok := errorResponseMessage(respBody); ok {
		return fmt.Errorf("unexpected response (%d): %s", httpResp.StatusCode, msg)
	}
	return fmt.Errorf("unexpected response (%d)", httpResp.StatusCode)
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// createFormFile is mime/multipart.Writer.CreateFormFile with a caller-
// supplied Content-Type instead of the hardcoded application/octet-stream.
func createFormFile(w *multipart.Writer, fieldName, fileName, contentType string) (io.Writer, error) {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, quoteEscaper.Replace(fileName)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}

func mimeTypeFor(fileName string) string {
	if ct := mime.TypeByExtension(filepath.Ext(fileName)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
