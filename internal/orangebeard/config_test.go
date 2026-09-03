package orangebeard

import (
	"path/filepath"
	"testing"
)

func TestConfig_SaveThenLoad_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := Config{
		Endpoint: "https://my-tenant.orangebeard.app",
		Token:    "abc-123-token",
		Project:  "my-project",
	}

	if err := SaveConfig(dir, want); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	got, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got != want {
		t.Errorf("LoadConfig() = %+v, want %+v", got, want)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("LoadConfig() error = nil, want an error for a missing config file")
	}
}

func TestConfigPath(t *testing.T) {
	got := configPath("/some/dir")
	want := filepath.Join("/some/dir", ".orangebeard", "config.env")
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}
