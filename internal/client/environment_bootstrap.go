package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/42BV/normatik-cli/internal/httpx"
)

// EnvironmentBootstrapResult mirrors the backend's minimal, secret-free response
// to POST /environment-seed/bootstrap: it never echoes the secret or the
// temporary password back to the caller.
type EnvironmentBootstrapResult struct {
	UserID *int64  `json:"userId,omitempty"`
	Email  *string `json:"email,omitempty"`
}

// bootstrapTimeout bounds the one-time, unauthenticated bootstrap call — no
// retries, this is not meant to be a hot path.
const bootstrapTimeout = 30 * time.Second

// Bootstrap performs the anonymous, one-time POST /environment-seed/bootstrap on
// site. Deliberately a package-level function, not a *Client method: this
// endpoint precedes any API key (the target has none yet), so there is no
// *Client to build — api.NewClientWithResponses always attaches a Bearer
// request editor, which would be meaningless here. It reuses the same
// exec/decodeError shape as Client.exec so a rejection (e.g.
// ENVIRONMENT_BOOTSTRAP_REJECTED, RATE_LIMIT_EXCEEDED) renders through the
// normal command.RenderError path with its errorCode + hint, identically to
// every authenticated command.
func Bootstrap(ctx context.Context, site, secret, email, password string) (*EnvironmentBootstrapResult, *APIError) {
	if err := httpx.ValidateBaseURL(site); err != nil {
		return nil, fail(err)
	}
	reqBody, err := json.Marshal(struct {
		Secret   string `json:"secret"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}{Secret: secret, Email: email, Password: password})
	if err != nil {
		return nil, fail(err)
	}
	endpoint := httpx.APIBaseURL(site) + "/environment-seed/bootstrap"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fail(err)
	}
	req.Header.Set("Content-Type", "application/json")
	hc := httpx.NewClient(bootstrapTimeout)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fail(err)
	}
	defer func() { _ = resp.Body.Close() }()

	limit := errorResponseLimit
	if resp.StatusCode/100 == 2 {
		limit = jsonResponseLimit
	}
	body, rerr := readBounded(resp.Body, limit, resp.StatusCode)
	if rerr != nil {
		if apiErr := APIErrorFromRead(rerr); apiErr != nil {
			return nil, apiErr
		}
		return nil, fail(rerr)
	}
	if resp.StatusCode/100 != 2 {
		return nil, decodeError(resp.StatusCode, body)
	}
	var out EnvironmentBootstrapResult
	if len(body) > 0 {
		if jerr := json.Unmarshal(body, &out); jerr != nil {
			return nil, &APIError{Status: resp.StatusCode, Body: body, Malformed: true}
		}
	}
	return &out, nil
}
