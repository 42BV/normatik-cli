package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/42BV/normatik-cli/internal/httpx"
)

// Detect probes <site>/api/public/v1/users/me and returns the canonical site URL
// when the key authenticates. Every Normatik environment uses the /api prefix.
func Detect(ctx context.Context, site, apiKey string, hc *http.Client) (string, error) {
	site = httpx.CanonicalSiteURL(site)
	if err := httpx.ValidateBaseURL(site); err != nil {
		return "", err
	}
	if hc == nil {
		hc = httpx.NewClient(10 * time.Second)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpx.APIBaseURL(site)+"/public/v1/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return site, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("API key rejected (401) at %s", site)
	}
	return "", fmt.Errorf("Normatik API probe failed with status %d at %s/api", resp.StatusCode, site)
}
