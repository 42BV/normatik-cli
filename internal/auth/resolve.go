package auth

import (
	"fmt"
	"os"
	"strings"

	"github.com/42BV/normatik-cli/internal/config"
	"github.com/42BV/normatik-cli/internal/httpx"
)

// Resolved is the outcome of credential resolution.
type Resolved struct {
	BaseURL string
	APIKey  string
	Profile string // resolved profile name ("" = none / env-only)
}

// Resolve determines the base-URL and API key.
//
// Site-URL precedence: --profile flag's profile URL > NORMATIK_BASE_URL env >
// active-profile URL. There is NO implicit localhost fallback: when nothing is
// configured BaseURL stays "" and callers must surface a clean CONFIG error
// (client commands) or prompt for it (login) — the CLI never silently talks to
// localhost. API-key precedence: NORMATIK_API_KEY env (strict, for CI/headless)
// > keychain for the resolved profile.
func Resolve(cfg *config.Config, profileFlag string) (Resolved, error) {
	r := Resolved{Profile: profileFlag}
	if r.Profile == "" {
		r.Profile = cfg.ActiveProfile
	}

	if profileFlag != "" {
		if p, ok := cfg.Profiles[profileFlag]; ok && p.BaseURL != "" {
			r.BaseURL = p.BaseURL
		}
	}
	if r.BaseURL == "" {
		if env := os.Getenv("NORMATIK_BASE_URL"); env != "" {
			r.BaseURL = env
		}
	}
	if r.BaseURL == "" && r.Profile != "" {
		if p, ok := cfg.Profiles[r.Profile]; ok && p.BaseURL != "" {
			r.BaseURL = p.BaseURL
		}
	}
	r.BaseURL = httpx.CanonicalSiteURL(r.BaseURL)

	if env := strings.TrimSpace(os.Getenv("NORMATIK_API_KEY")); env != "" {
		r.APIKey = env // strict: env wins over keychain (whitespace-only = absent)
	} else if r.Profile != "" {
		if k, err := GetKey(r.Profile); err == nil {
			r.APIKey = k
		}
	}
	if r.BaseURL != "" {
		if err := httpx.ValidateBaseURL(r.BaseURL); err != nil {
			return r, fmt.Errorf("invalid resolved environment: %w", err)
		}
	}
	return r, nil
}
