package orangebeard

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMergeStructure_NewTestSetCreatesTree(t *testing.T) {
	idx := NewStructureIndex()
	run := BulkTestRun{
		TestSetName: "manual-qa-checkout",
		Suites: []Suite{
			{
				Name: "Checkout",
				Tests: []Test{
					{TestName: "Guest can complete purchase"},
					{TestName: "Guest checkout rejects an expired card"},
				},
			},
		},
	}
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)

	MergeStructure(idx, run, now)

	ts, ok := idx.TestSets["manual-qa-checkout"]
	if !ok {
		t.Fatal("testSetName not recorded")
	}
	if ts.LastReported != "2026-09-04T10:00:00Z" {
		t.Errorf("LastReported = %q", ts.LastReported)
	}
	if len(ts.Suites) != 1 || ts.Suites[0].Name != "Checkout" {
		t.Fatalf("Suites = %+v", ts.Suites)
	}
	if got := ts.Suites[0].Tests; len(got) != 2 || got[0] != "Guest can complete purchase" || got[1] != "Guest checkout rejects an expired card" {
		t.Errorf("Tests = %v", got)
	}
}

func TestMergeStructure_DuplicateTestIsNotDuplicated(t *testing.T) {
	idx := NewStructureIndex()
	run := BulkTestRun{
		TestSetName: "manual-qa-checkout",
		Suites: []Suite{
			{Name: "Checkout", Tests: []Test{{TestName: "Guest can complete purchase"}}},
		},
	}
	MergeStructure(idx, run, time.Now())
	MergeStructure(idx, run, time.Now())

	tests := idx.TestSets["manual-qa-checkout"].Suites[0].Tests
	if len(tests) != 1 {
		t.Fatalf("Tests = %v, want exactly 1 (deduped)", tests)
	}
}

func TestMergeStructure_NewTestAppendsToExistingSuite(t *testing.T) {
	idx := NewStructureIndex()
	first := BulkTestRun{
		TestSetName: "manual-qa-checkout",
		Suites:      []Suite{{Name: "Checkout", Tests: []Test{{TestName: "A"}}}},
	}
	second := BulkTestRun{
		TestSetName: "manual-qa-checkout",
		Suites:      []Suite{{Name: "Checkout", Tests: []Test{{TestName: "B"}}}},
	}
	MergeStructure(idx, first, time.Now())
	MergeStructure(idx, second, time.Now())

	tests := idx.TestSets["manual-qa-checkout"].Suites[0].Tests
	if len(tests) != 2 || tests[0] != "A" || tests[1] != "B" {
		t.Fatalf("Tests = %v, want [A B] (existing order preserved, new appended)", tests)
	}
}

func TestMergeStructure_NewSiblingSuiteAppended(t *testing.T) {
	idx := NewStructureIndex()
	first := BulkTestRun{TestSetName: "ts", Suites: []Suite{{Name: "Checkout"}}}
	second := BulkTestRun{TestSetName: "ts", Suites: []Suite{{Name: "Login"}}}
	MergeStructure(idx, first, time.Now())
	MergeStructure(idx, second, time.Now())

	suites := idx.TestSets["ts"].Suites
	if len(suites) != 2 || suites[0].Name != "Checkout" || suites[1].Name != "Login" {
		t.Fatalf("Suites = %+v", suites)
	}
}

func TestMergeStructure_NestedSubSuitesMergeRecursively(t *testing.T) {
	idx := NewStructureIndex()
	run := BulkTestRun{
		TestSetName: "ts",
		Suites: []Suite{
			{
				Name: "Checkout",
				Suites: []Suite{
					{Name: "Guest", Tests: []Test{{TestName: "Pay with card"}}},
				},
			},
		},
	}
	MergeStructure(idx, run, time.Now())

	// A second run adds a test to the nested sub-suite and a sibling sub-suite.
	run2 := BulkTestRun{
		TestSetName: "ts",
		Suites: []Suite{
			{
				Name: "Checkout",
				Suites: []Suite{
					{Name: "Guest", Tests: []Test{{TestName: "Pay with PayPal"}}},
					{Name: "Member", Tests: []Test{{TestName: "Pay with saved card"}}},
				},
			},
		},
	}
	MergeStructure(idx, run2, time.Now())

	checkout := idx.TestSets["ts"].Suites[0]
	if len(checkout.Suites) != 2 {
		t.Fatalf("Checkout.Suites = %+v, want 2 nested suites", checkout.Suites)
	}
	guest := checkout.Suites[0]
	if guest.Name != "Guest" || len(guest.Tests) != 2 {
		t.Fatalf("Guest = %+v", guest)
	}
	member := checkout.Suites[1]
	if member.Name != "Member" || len(member.Tests) != 1 {
		t.Fatalf("Member = %+v", member)
	}
}

func TestStructureIndex_SaveThenLoad_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	idx := NewStructureIndex()
	run := BulkTestRun{
		TestSetName: "ts",
		Suites:      []Suite{{Name: "Checkout", Tests: []Test{{TestName: "A"}}}},
	}
	MergeStructure(idx, run, time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))

	if err := SaveStructureIndex(dir, idx); err != nil {
		t.Fatalf("SaveStructureIndex() error = %v", err)
	}

	got, err := LoadStructureIndex(dir)
	if err != nil {
		t.Fatalf("LoadStructureIndex() error = %v", err)
	}
	if got.TestSets["ts"].Suites[0].Tests[0] != "A" {
		t.Errorf("round-tripped index = %+v", got)
	}
}

func TestLoadStructureIndex_MissingFileReturnsEmptyIndex(t *testing.T) {
	dir := t.TempDir()
	idx, err := LoadStructureIndex(dir)
	if err != nil {
		t.Fatalf("LoadStructureIndex() error = %v, want nil (missing file is not an error)", err)
	}
	if idx == nil || len(idx.TestSets) != 0 {
		t.Errorf("LoadStructureIndex() = %+v, want empty index", idx)
	}
}

func TestStructurePath(t *testing.T) {
	got := structurePath("/some/dir")
	want := filepath.Join("/some/dir", ".orangebeard", "reported-structure.json")
	if got != want {
		t.Errorf("structurePath() = %q, want %q", got, want)
	}
}
