// Package httpx centralizes the CLI's outbound HTTP security policy.
package httpx

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

const allowHTTPEnv = "NORMATIK_ALLOW_HTTP"

// CanonicalSiteURL normalizes the user-facing environment URL. A legacy
// trailing /api context path is stripped because transports own that prefix.
func CanonicalSiteURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return trimmed
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/api") {
		u.Path = strings.TrimSuffix(u.Path, "/api")
		u.RawPath = ""
	}
	return strings.TrimRight(u.String(), "/")
}

// APIBaseURL adds Normatik's fixed API context path to a canonical site URL.
func APIBaseURL(site string) string {
	canonical := CanonicalSiteURL(site)
	if canonical == "" {
		return ""
	}
	u, err := url.Parse(canonical)
	if err != nil || u.Host == "" {
		return canonical + "/api"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api"
	u.RawPath = ""
	return u.String()
}

// ValidateBaseURL allows HTTPS everywhere and plaintext HTTP only for literal
// loopback targets when the development opt-in is exactly "1".
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid base URL: use an absolute https:// URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if os.Getenv(allowHTTPEnv) != "1" {
			return fmt.Errorf("insecure base URL: use https:// (loopback development requires %s=1)", allowHTTPEnv)
		}
		if !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("insecure base URL: %s=1 permits only localhost, 127.0.0.0/8 and ::1; use https://", allowHTTPEnv)
		}
		return nil
	default:
		return fmt.Errorf("invalid base URL scheme %q: use https://", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// NewClient creates an HTTP client that follows redirects only within the
// exact original origin (scheme, hostname and effective port).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: sameOriginRedirect,
	}
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("redirect blocked after 10 hops")
	}
	if len(via) == 0 || sameOrigin(via[0].URL, req.URL) {
		return nil
	}
	return fmt.Errorf("redirect blocked: target origin differs from the authenticated request origin")
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
