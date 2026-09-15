// Package orangebeard is a client for Orangebeard's bulk test-run import
// endpoint (POST /listener/v3/{projectName}/test-run/bulk).
package orangebeard

// Attribute is a free-form key/value tag attachable to a run, suite, or test.
type Attribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Log is a single log line attached to a test or step.
type Log struct {
	LogTime     string          `json:"logTime"`
	Message     string          `json:"message"`
	LogLevel    string          `json:"logLevel,omitempty"`
	LogFormat   string          `json:"logFormat,omitempty"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

// AttachmentRef declares a local file to be uploaded as an attachment to
// this log once the bulk call returns. AttachmentRef must be unique across
// the whole document — the server echoes it back in the response so the
// client can correlate the declaration with the minted testUUID/stepUUID/
// logUUID it needs for the follow-up /attachment call. Path is only
// meaningful to the client; the server ignores it.
type AttachmentRef struct {
	AttachmentRef string `json:"attachmentRef"`
	FileName      string `json:"fileName"`
	Path          string `json:"path"`
	ContentType   string `json:"contentType,omitempty"`
}

// Step is a single step within a test, optionally nested under parent steps.
type Step struct {
	StepName    string `json:"stepName"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
	Logs        []Log  `json:"logs,omitempty"`
	Steps       []Step `json:"steps,omitempty"`
}

// Test is a single test (or BEFORE/AFTER hook) within a suite.
type Test struct {
	TestName    string      `json:"testName"`
	TestType    string      `json:"testType,omitempty"`
	Status      string      `json:"status,omitempty"`
	StartTime   string      `json:"startTime"`
	EndTime     string      `json:"endTime"`
	Description string      `json:"description,omitempty"`
	Attributes  []Attribute `json:"attributes,omitempty"`
	Logs        []Log       `json:"logs,omitempty"`
	Steps       []Step      `json:"steps,omitempty"`
}

// Suite is a named grouping of tests and/or nested sub-suites.
type Suite struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Attributes  []Attribute `json:"attributes,omitempty"`
	Suites      []Suite     `json:"suites,omitempty"`
	Tests       []Test      `json:"tests,omitempty"`
}

// BulkTestRun is the full document posted to the bulk-import endpoint.
type BulkTestRun struct {
	IdempotencyKey string      `json:"idempotencyKey,omitempty"`
	TestSetName    string      `json:"testSetName"`
	StartTime      string      `json:"startTime"`
	EndTime        string      `json:"endTime"`
	Description    string      `json:"description,omitempty"`
	Attributes     []Attribute `json:"attributes,omitempty"`
	Suites         []Suite     `json:"suites,omitempty"`
}

// BulkImportResponse is the bulk-import endpoint's 201 response. Attachments
// is only populated when the request declared at least one; when the
// request declared none, the server's response is (and stays) a bare JSON
// string, which Report decodes into TestRunUUID directly.
type BulkImportResponse struct {
	TestRunUUID string             `json:"testRunUUID"`
	Attachments []AttachmentTarget `json:"attachments,omitempty"`
}

// AttachmentTarget is the server's echo of one declared AttachmentRef,
// carrying the minted UUIDs a follow-up /attachment call needs. StepUUID is
// only set when the log's parent is a step rather than a test directly.
type AttachmentTarget struct {
	AttachmentRef string `json:"attachmentRef"`
	TestUUID      string `json:"testUUID"`
	StepUUID      string `json:"stepUUID,omitempty"`
	LogUUID       string `json:"logUUID"`
}
