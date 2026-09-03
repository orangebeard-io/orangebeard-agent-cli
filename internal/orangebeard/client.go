package orangebeard

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ErrNotSupported means the bulk-import route itself isn't mapped on this
// Orangebeard instance (a 404 whose body doesn't match the standard
// ErrorResponse shape) — distinct from a 404 raised by business logic (e.g.
// an unknown project), which surfaces as *UnexpectedStatusError instead.
var ErrNotSupported = fmt.Errorf("this Orangebeard instance doesn't support bulk import yet")

// ValidationError is a single violation found while validating a submitted
// document, as returned in a 400 response's validationErrors array.
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationFailedError is returned when the server rejects the whole
// document (HTTP 400); ValidationErrors always covers every violation found,
// not just the first.
type ValidationFailedError struct {
	Message          string
	ValidationErrors []ValidationError
}

func (e *ValidationFailedError) Error() string {
	return fmt.Sprintf("%s (%d validation error(s))", e.Message, len(e.ValidationErrors))
}

// ConflictError is returned when the request's idempotencyKey was already
// used with a different request body (HTTP 409).
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// UnexpectedStatusError is returned for any response the client doesn't have
// a more specific case for, including a business-logic 404 (e.g. unknown
// project) whose body parsed as the standard ErrorResponse shape.
type UnexpectedStatusError struct {
	StatusCode int
	Message    string // best-effort; empty if the body wasn't ErrorResponse-shaped
}

func (e *UnexpectedStatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("unexpected response (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("unexpected response (%d)", e.StatusCode)
}

// Client talks to Orangebeard's bulk test-run import endpoint.
type Client struct {
	Endpoint string // e.g. https://my-tenant.orangebeard.app
	Token    string // project access token
	Project  string

	// HTTPClient overrides the default HTTP client; nil uses http.DefaultClient.
	HTTPClient *http.Client
}

// Report submits a full test run in one call and returns the resulting
// testRunUUID. A 201 means the run was validated and enqueued — not that it
// has finished processing.
//
// If run.IdempotencyKey is empty, Report generates one so a retry after a
// dropped response reuses the same key instead of risking a duplicate run.
func (c *Client) Report(ctx context.Context, run BulkTestRun) (string, error) {
	if run.IdempotencyKey == "" {
		key, err := newIdempotencyKey()
		if err != nil {
			return "", fmt.Errorf("generating idempotency key: %w", err)
		}
		run.IdempotencyKey = key
	}

	body, err := json.Marshal(run)
	if err != nil {
		return "", fmt.Errorf("encoding request body: %w", err)
	}

	target := fmt.Sprintf("%s/listener/v3/%s/test-run/bulk", c.Endpoint, url.PathEscape(c.Project))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling %s: %w", target, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		var testRunUUID string
		if err := json.Unmarshal(respBody, &testRunUUID); err != nil {
			return "", fmt.Errorf("decoding testRunUUID: %w", err)
		}
		return testRunUUID, nil

	case http.StatusBadRequest:
		var payload struct {
			Message          string            `json:"message"`
			ValidationErrors []ValidationError `json:"validationErrors"`
		}
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return "", fmt.Errorf("decoding validation error response: %w", err)
		}
		return "", &ValidationFailedError{Message: payload.Message, ValidationErrors: payload.ValidationErrors}

	case http.StatusConflict:
		msg, _ := errorResponseMessage(respBody)
		return "", &ConflictError{Message: msg}

	case http.StatusNotFound:
		if msg, ok := errorResponseMessage(respBody); ok {
			return "", &UnexpectedStatusError{StatusCode: http.StatusNotFound, Message: msg}
		}
		return "", ErrNotSupported

	default:
		msg, _ := errorResponseMessage(respBody)
		return "", &UnexpectedStatusError{StatusCode: resp.StatusCode, Message: msg}
	}
}

// errorResponseMessage reports whether body matches Orangebeard's standard
// ErrorResponse shape ({"message": "..."}) and, if so, extracts the message.
func errorResponseMessage(body []byte) (string, bool) {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	if payload.Message == "" {
		return "", false
	}
	return payload.Message, true
}

func newIdempotencyKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// RFC 4122 version 4, variant 1.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
