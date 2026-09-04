package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	latestReleaseAPI    = "https://api.github.com/repos/orangebeard-io/orangebeard-agent-cli/releases/latest"
	updateCheckInterval = 24 * time.Hour
)

// maybeNotifyUpdate runs the whole rate-limited update check and prints a
// notice to stderr if a newer release exists. Silent on any failure (no
// network, GitHub down, corrupt cache, dev build) — an update check must
// never be disruptive to the actual command the agent is running.
func maybeNotifyUpdate() {
	cacheFile, err := defaultUpdateCacheFile()
	if err != nil {
		return
	}
	fetch := func() (string, error) {
		client := &http.Client{Timeout: 3 * time.Second}
		return fetchLatestRelease(latestReleaseAPI, client)
	}
	if notice := checkForUpdate(version, cacheFile, time.Now(), fetch); notice != "" {
		fmt.Fprintln(os.Stderr, notice)
	}
}

func defaultUpdateCacheFile() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "orangebeard-report", "last-update-check"), nil
}

// checkForUpdate performs one rate-limited check: if due, it calls fetch for
// the latest release tag and returns a notice string (empty if not due, or
// up to date, or fetch failed). A due check is always recorded as done —
// including on fetch failure — so a network hiccup doesn't force a retry on
// every single invocation; it just waits for the next daily window.
func checkForUpdate(currentVersion, cacheFile string, now time.Time, fetch func() (string, error)) string {
	if !shouldCheckForUpdate(cacheFile, now) {
		return ""
	}
	_ = recordUpdateChecked(cacheFile, now)

	latest, err := fetch()
	if err != nil {
		return ""
	}
	return updateNotice(currentVersion, latest)
}

func shouldCheckForUpdate(cacheFile string, now time.Time) bool {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return true
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}
	return now.Sub(last) >= updateCheckInterval
}

func recordUpdateChecked(cacheFile string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cacheFile, []byte(now.UTC().Format(time.RFC3339)), 0o644)
}

// updateNotice returns a human-readable notice if currentVersion is behind
// latestTag, or "" if they match or currentVersion isn't a real release
// build (e.g. "dev" for a from-source build with no injected version).
func updateNotice(currentVersion, latestTag string) string {
	if currentVersion == "" || currentVersion == "dev" || latestTag == "" {
		return ""
	}
	if strings.TrimPrefix(currentVersion, "v") == strings.TrimPrefix(latestTag, "v") {
		return ""
	}
	return fmt.Sprintf(
		"A newer orangebeard-report is available: %s (you have %s). Get it: https://github.com/orangebeard-io/orangebeard-agent-cli/releases/latest",
		latestTag, currentVersion,
	)
}

func fetchLatestRelease(apiURL string, client *http.Client) (string, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, apiURL)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}
