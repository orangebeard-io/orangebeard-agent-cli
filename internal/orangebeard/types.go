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
	LogTime   string `json:"logTime"`
	Message   string `json:"message"`
	LogLevel  string `json:"logLevel,omitempty"`
	LogFormat string `json:"logFormat,omitempty"`
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
