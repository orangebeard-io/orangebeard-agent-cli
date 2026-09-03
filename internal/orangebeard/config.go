package orangebeard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the connection details a report needs: where to send it, how
// to authenticate, and which project it belongs to.
type Config struct {
	Endpoint string
	Token    string
	Project  string
}

func configPath(dir string) string {
	return filepath.Join(dir, ".orangebeard", "config.env")
}

// SaveConfig writes cfg to <dir>/.orangebeard/config.env. The file holds a
// project access token, so it's created with owner-only permissions and the
// caller is expected to gitignore the .orangebeard directory.
func SaveConfig(dir string, cfg Config) error {
	path := configPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "ORANGEBEARD_ENDPOINT=%s\n", cfg.Endpoint)
	fmt.Fprintf(&b, "ORANGEBEARD_TOKEN=%s\n", cfg.Token)
	fmt.Fprintf(&b, "ORANGEBEARD_PROJECT=%s\n", cfg.Project)

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// LoadConfig reads <dir>/.orangebeard/config.env, written by SaveConfig (or
// hand-edited in the same KEY=value shape).
func LoadConfig(dir string) (Config, error) {
	path := configPath(dir)
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w (run `init` first)", path, err)
	}
	defer f.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := Config{
		Endpoint: values["ORANGEBEARD_ENDPOINT"],
		Token:    values["ORANGEBEARD_TOKEN"],
		Project:  values["ORANGEBEARD_PROJECT"],
	}
	var missing []string
	if cfg.Endpoint == "" {
		missing = append(missing, "ORANGEBEARD_ENDPOINT")
	}
	if cfg.Token == "" {
		missing = append(missing, "ORANGEBEARD_TOKEN")
	}
	if cfg.Project == "" {
		missing = append(missing, "ORANGEBEARD_PROJECT")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%s is missing: %s", path, strings.Join(missing, ", "))
	}
	return cfg, nil
}
