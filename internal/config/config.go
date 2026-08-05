// Package config persists non-secret CLI preferences: the active profile, the
// default output mode, and per-profile base-URLs. It lives at config.toml under
// os.UserConfigDir() (XDG on Linux, %AppData% on Windows, Application Support on
// macOS) with mode 0600. API keys are NEVER stored here — those go in the OS
// keychain (see internal/auth).
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/42BV/normatik-cli/internal/httpx"

	toml "github.com/pelletier/go-toml/v2"
)

// Profile is one environment: a base-URL (the matching API key lives in the keychain).
type Profile struct {
	BaseURL string `toml:"base_url"`
}

// Config is the on-disk shape of config.toml.
type Config struct {
	ActiveProfile string             `toml:"active_profile,omitempty"`
	Output        string             `toml:"output,omitempty"`
	Profiles      map[string]Profile `toml:"profiles,omitempty"`
}

// Path returns the config.toml location under the OS user config dir.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "normatik", "config.toml"), nil
}

func resolvePath() (string, error) {
	if env := os.Getenv("NORMATIK_CONFIG"); env != "" {
		return env, nil
	}
	return Path()
}

// Load reads config.toml. A missing file yields an empty (but usable) Config.
func Load() (*Config, error) {
	p, err := resolvePath()
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- p is deliberately the OS config path or the explicit NORMATIK_CONFIG override.
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, err
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	for name, profile := range c.Profiles {
		profile.BaseURL = httpx.CanonicalSiteURL(profile.BaseURL)
		c.Profiles[name] = profile
	}
	return &c, nil
}

// Save writes config.toml (file mode 0600, dir 0700). The 0600 is a POSIX
// best-effort and a no-op on Windows; config.toml deliberately holds NO secrets
// (API keys live in the OS keychain), so secret protection does not rely on it.
func (c *Config) Save() error {
	p, err := resolvePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// SetProfile upserts a profile's base-URL.
func (c *Config) SetProfile(name, baseURL string) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = Profile{BaseURL: httpx.CanonicalSiteURL(baseURL)}
}
