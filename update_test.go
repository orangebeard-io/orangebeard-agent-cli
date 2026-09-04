package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShouldCheckForUpdate_MissingFileReturnsTrue(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	if !shouldCheckForUpdate(cacheFile, time.Now()) {
		t.Error("shouldCheckForUpdate() = false, want true for a missing cache file")
	}
}

func TestShouldCheckForUpdate_RecentCheckReturnsFalse(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	now := time.Now()
	if err := recordUpdateChecked(cacheFile, now); err != nil {
		t.Fatalf("recordUpdateChecked() error = %v", err)
	}
	if shouldCheckForUpdate(cacheFile, now.Add(1*time.Hour)) {
		t.Error("shouldCheckForUpdate() = true, want false only 1h after a recorded check")
	}
}

func TestShouldCheckForUpdate_OldCheckReturnsTrue(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	now := time.Now()
	if err := recordUpdateChecked(cacheFile, now); err != nil {
		t.Fatalf("recordUpdateChecked() error = %v", err)
	}
	if !shouldCheckForUpdate(cacheFile, now.Add(25*time.Hour)) {
		t.Error("shouldCheckForUpdate() = false, want true 25h after a recorded check")
	}
}

func TestShouldCheckForUpdate_UnparsableFileReturnsTrue(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	if err := os.WriteFile(cacheFile, []byte("not a timestamp"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !shouldCheckForUpdate(cacheFile, time.Now()) {
		t.Error("shouldCheckForUpdate() = false, want true for a corrupt cache file")
	}
}

func TestUpdateNotice_SameVersionReturnsEmpty(t *testing.T) {
	if got := updateNotice("v1.2.3", "v1.2.3"); got != "" {
		t.Errorf("updateNotice() = %q, want empty for matching versions", got)
	}
}

func TestUpdateNotice_DifferentVersionReturnsNotice(t *testing.T) {
	got := updateNotice("v1.2.3", "v1.3.0")
	if got == "" {
		t.Fatal("updateNotice() = empty, want a notice for a newer version")
	}
	if !strings.Contains(got, "v1.3.0") || !strings.Contains(got, "v1.2.3") {
		t.Errorf("updateNotice() = %q, want it to mention both versions", got)
	}
}

func TestUpdateNotice_DevVersionAlwaysEmpty(t *testing.T) {
	if got := updateNotice("dev", "v1.3.0"); got != "" {
		t.Errorf("updateNotice() = %q, want empty for a dev (source) build", got)
	}
}

func TestCheckForUpdate_NotDue_DoesNotCallFetch(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	now := time.Now()
	if err := recordUpdateChecked(cacheFile, now); err != nil {
		t.Fatal(err)
	}
	called := false
	fetch := func() (string, error) { called = true; return "v9.9.9", nil }

	notice := checkForUpdate("v1.0.0", cacheFile, now.Add(1*time.Hour), fetch)
	if called {
		t.Error("fetch was called even though a check isn't due yet")
	}
	if notice != "" {
		t.Errorf("checkForUpdate() = %q, want empty when not due", notice)
	}
}

func TestCheckForUpdate_Due_CallsFetchAndReturnsNotice(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	called := false
	fetch := func() (string, error) { called = true; return "v2.0.0", nil }

	notice := checkForUpdate("v1.0.0", cacheFile, time.Now(), fetch)
	if !called {
		t.Error("fetch was not called even though no check has ever been recorded")
	}
	if notice == "" {
		t.Error("checkForUpdate() = empty, want a notice (v1.0.0 -> v2.0.0)")
	}
}

func TestCheckForUpdate_RecordsCheckEvenOnFetchFailure(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "last-update-check")
	fetch := func() (string, error) { return "", assertErr }

	notice := checkForUpdate("v1.0.0", cacheFile, time.Now(), fetch)
	if notice != "" {
		t.Errorf("checkForUpdate() = %q, want empty on fetch failure", notice)
	}
	if shouldCheckForUpdate(cacheFile, time.Now()) {
		t.Error("a failed fetch should still mark the check as done, so it isn't retried every single invocation")
	}
}

var assertErr = errors.New("simulated network failure")
