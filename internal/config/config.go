// Package config resolves jellytop's server URL and API token from flags,
// the environment, and a small config file, in that order of precedence.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultURL is used when nothing else specifies a server.
const DefaultURL = "http://127.0.0.1:8096"

// Config holds everything jellytop needs to reach a server.
type Config struct {
	URL   string
	Token string
	// Source records where the token came from, for the error message when
	// authentication fails.
	Source string
}

// Path returns the config file location, honouring XDG_CONFIG_HOME.
func Path() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "jellytop", "config")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "jellytop", "config")
}

// ErrNoToken is returned when no API token could be found anywhere.
var ErrNoToken = errors.New("no API token configured")

// Load resolves configuration. Non-empty flag values win, then environment
// variables, then the config file.
func Load(flagURL, flagToken string) (*Config, error) {
	cfg := &Config{URL: DefaultURL}

	fileVals, err := readFile(Path())
	if err != nil {
		return nil, err
	}
	if v := fileVals["url"]; v != "" {
		cfg.URL, cfg.Source = v, "config file"
	}
	if v := fileVals["token"]; v != "" {
		cfg.Token, cfg.Source = v, "config file "+Path()
	}

	if v := os.Getenv("JELLYFIN_URL"); v != "" {
		cfg.URL = v
	}
	if v := os.Getenv("JELLYFIN_TOKEN"); v != "" {
		cfg.Token, cfg.Source = v, "$JELLYFIN_TOKEN"
	}

	if flagURL != "" {
		cfg.URL = flagURL
	}
	if flagToken != "" {
		cfg.Token, cfg.Source = flagToken, "--token"
	}

	if cfg.Token == "" {
		return nil, ErrNoToken
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		cfg.URL = "http://" + cfg.URL
	}
	return cfg, nil
}

// readFile parses a minimal "key = value" config file. Missing file is not an
// error; blank lines and # comments are ignored.
func readFile(path string) (map[string]string, error) {
	vals := map[string]string{}
	if path == "" {
		return vals, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return vals, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key = value", path, line)
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		vals[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return vals, sc.Err()
}

// SetupHint is printed when no token is configured.
func SetupHint() string {
	return strings.TrimSpace(fmt.Sprintf(`
jellytop needs a Jellyfin API key.

  1. In Jellyfin: Dashboard → Advanced → API Keys → +
  2. Then either export it:

       export JELLYFIN_URL=%s
       export JELLYFIN_TOKEN=<key>

     or write %s:

       url   = %s
       token = <key>
`, DefaultURL, Path(), DefaultURL))
}
