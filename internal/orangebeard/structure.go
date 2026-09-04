package orangebeard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StructureIndex is a per-project ledger of every testSetName / suite path /
// testName combination this project has successfully reported before. It
// carries no secrets (unlike Config) and is meant to be committed to version
// control: an agent starting a fresh session has no memory of what it named
// things last time, so it needs somewhere durable to look up the exact prior
// string instead of re-deriving a paraphrase — see the static-naming
// discipline in SKILL.md.
type StructureIndex struct {
	TestSets map[string]*TestSetNode `json:"testSets"`
}

// TestSetNode is everything recorded under one testSetName.
type TestSetNode struct {
	Suites       []*SuiteNode `json:"suites,omitempty"`
	LastReported string       `json:"lastReported"`
}

// SuiteNode is one suite's recorded name, its nested sub-suites, and the
// names of the tests reported directly inside it.
type SuiteNode struct {
	Name   string       `json:"name"`
	Suites []*SuiteNode `json:"suites,omitempty"`
	Tests  []string     `json:"tests,omitempty"`
}

// NewStructureIndex returns an empty index, ready to merge into.
func NewStructureIndex() *StructureIndex {
	return &StructureIndex{TestSets: map[string]*TestSetNode{}}
}

// MergeStructure records run's testSetName/suite-path/testName structure
// into idx, in place. Merging is additive and order-preserving: an existing
// suite or test is left where it already was and merged into (never
// reordered or removed), and a genuinely new sibling is appended after the
// existing ones. Call this only after a run has actually been accepted by
// the server — it's a ledger of what was reported, not what was attempted.
func MergeStructure(idx *StructureIndex, run BulkTestRun, now time.Time) {
	if idx.TestSets == nil {
		idx.TestSets = map[string]*TestSetNode{}
	}
	ts, ok := idx.TestSets[run.TestSetName]
	if !ok {
		ts = &TestSetNode{}
		idx.TestSets[run.TestSetName] = ts
	}
	ts.LastReported = now.UTC().Format(time.RFC3339)
	ts.Suites = mergeSuites(ts.Suites, run.Suites)
}

func mergeSuites(existing []*SuiteNode, incoming []Suite) []*SuiteNode {
	for _, in := range incoming {
		node := findSuite(existing, in.Name)
		if node == nil {
			node = &SuiteNode{Name: in.Name}
			existing = append(existing, node)
		}
		node.Suites = mergeSuites(node.Suites, in.Suites)
		node.Tests = mergeTests(node.Tests, in.Tests)
	}
	return existing
}

func findSuite(suites []*SuiteNode, name string) *SuiteNode {
	for _, s := range suites {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func mergeTests(existing []string, incoming []Test) []string {
	seen := make(map[string]bool, len(existing))
	for _, name := range existing {
		seen[name] = true
	}
	for _, in := range incoming {
		if !seen[in.TestName] {
			existing = append(existing, in.TestName)
			seen[in.TestName] = true
		}
	}
	return existing
}

func structurePath(dir string) string {
	return filepath.Join(dir, ".orangebeard", "reported-structure.json")
}

// LoadStructureIndex reads <dir>/.orangebeard/reported-structure.json. A
// missing file is not an error — it just means nothing has been reported
// from this project yet — and returns an empty index.
func LoadStructureIndex(dir string) (*StructureIndex, error) {
	data, err := os.ReadFile(structurePath(dir))
	if os.IsNotExist(err) {
		return NewStructureIndex(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", structurePath(dir), err)
	}
	idx := NewStructureIndex()
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", structurePath(dir), err)
	}
	if idx.TestSets == nil {
		idx.TestSets = map[string]*TestSetNode{}
	}
	return idx, nil
}

// SaveStructureIndex writes idx to <dir>/.orangebeard/reported-structure.json.
// Unlike config.env, this file holds no secret and is meant to be committed.
func SaveStructureIndex(dir string, idx *StructureIndex) error {
	path := structurePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
