package httpx

import "net/http"

const defaultUserAgent = "normatik-cli/0.0.0-dev"

// userAgent is the process-wide product token. cli sets it after resolving
// the build version; tests that never import cli keep the development default.
var userAgent = defaultUserAgent

// SetUserAgent replaces the User-Agent sent on later requests. NewClient reads
// it per request, so a client built before this call still sends the new value.
func SetUserAgent(value string) {
	userAgent = value
}

// UserAgent returns the User-Agent the next request will send.
func UserAgent() string {
	return currentUserAgent()
}

func currentUserAgent() string {
	return userAgent
}

// userAgentTransport stamps User-Agent on a clone and delegates to base.
// The request handed to RoundTrip is left unchanged.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("User-Agent", currentUserAgent())
	return t.base.RoundTrip(clone)
}
