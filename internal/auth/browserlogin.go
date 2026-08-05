package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/42BV/normatik-cli/internal/httpx"
)

// maxAuthResponseBytes caps how much of a (trusted-but-possibly-hostile) auth
// endpoint response body is read before decoding (NORMATIK-18).
const maxAuthResponseBytes = 1 << 20

// decodeAuthResponse decodes a bounded prefix of an auth-endpoint response body.
func decodeAuthResponse(r io.Reader, v any) error {
	return json.NewDecoder(io.LimitReader(r, maxAuthResponseBytes)).Decode(v)
}

// The /cli-auth endpoints live below the fixed /api context path, next to
// /public/v1. The generated transport does not cover this device-flow contract.

var browserLoginRandom = io.Reader(rand.Reader)

// ErrBrowserLoginUnsupported signals a backend without the /cli-auth endpoints
// (404/405 on start): an older server that only supports the paste path.
var ErrBrowserLoginUnsupported = errors.New("server does not support browser login (no /cli-auth endpoints)")

// ErrRateLimited signals a 429 on start or poll.
var ErrRateLimited = errors.New("rate limited (429)")

// ErrNonceInvalid signals a 410 on poll for a user-code that is unknown, expired, or
// was never approved (denied). The backend reports errorCode
// CLI_AUTH_NONCE_INVALID for these; no server-side key exists.
var ErrNonceInvalid = errors.New("approval expired, denied or already used (410)")

// ErrKeyAlreadyDelivered signals a 410 on poll whose body carries errorCode
// CLI_AUTH_KEY_ALREADY_DELIVERED: the flow WAS approved and its key was already
// handed out once. This is the transport-drop case: a later explicit poll receives
// this 410, so a usable key exists server-side and cannot be re-fetched. Terminal:
// caller must warn about a dangling key rather than report a plain expiry.
var ErrKeyAlreadyDelivered = errors.New("approval succeeded but the key was already delivered (410)")

// ErrKeyDeliveredUnreadable signals a 200 on poll whose body could not be
// decoded into a usable key (truncated/corrupt response, or missing apiKey).
// The backend consumes the nonce and wipes the raw key in the SAME transaction
// as the 200, so the key is already minted-and-spent: retrying only yields a
// 410. This is terminal, NOT transient — the caller must warn that a dangling
// key exists server-side rather than silently retry into an "expired" message.
var ErrKeyDeliveredUnreadable = errors.New("approval succeeded but the key response could not be read (200)")

// BrowserLoginStart is the decoded /cli-auth/start response plus the canonical
// site URL used by start and poll.
type BrowserLoginStart struct {
	BaseURL         string
	UserCode        string
	BrowserURL      string
	IntervalSeconds int
	deviceVerifier  string
}

// StartBrowserLogin POSTs to the fixed /api/cli-auth/start endpoint.
func StartBrowserLogin(ctx context.Context, site, keyNameSuggestion string, readOnly bool, hc *http.Client) (*BrowserLoginStart, error) {
	site = httpx.CanonicalSiteURL(site)
	if err := httpx.ValidateBaseURL(site); err != nil {
		return nil, err
	}
	if hc == nil {
		hc = httpx.NewClient(10 * time.Second)
	}
	verifierBytes := make([]byte, 32)
	if _, err := io.ReadFull(browserLoginRandom, verifierBytes); err != nil {
		return nil, fmt.Errorf("generate device verifier: %w", err)
	}
	deviceVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeDigest := sha256.Sum256([]byte(deviceVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	body, err := json.Marshal(map[string]any{
		"codeChallenge":     codeChallenge,
		"keyNameSuggestion": keyNameSuggestion,
		"readOnly":          readOnly,
	})
	if err != nil {
		return nil, err
	}
	endpoint := httpx.APIBaseURL(site) + "/cli-auth/start"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			UserCode        string `json:"userCode"`
			BrowserURL      string `json:"browserUrl"`
			IntervalSeconds int    `json:"intervalSeconds"`
		}
		decodeErr := decodeAuthResponse(resp.Body, &out)
		_ = resp.Body.Close()
		if decodeErr != nil || out.UserCode == "" || out.BrowserURL == "" {
			return nil, fmt.Errorf("unexpected /cli-auth/start response from %s", site)
		}
		return &BrowserLoginStart{
			BaseURL:         site,
			UserCode:        out.UserCode,
			BrowserURL:      out.BrowserURL,
			IntervalSeconds: out.IntervalSeconds,
			deviceVerifier:  deviceVerifier,
		}, nil
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		_ = resp.Body.Close()
		return nil, ErrBrowserLoginUnsupported
	case http.StatusTooManyRequests:
		_ = resp.Body.Close()
		return nil, ErrRateLimited
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, endpoint)
	}
}

// PollBrowserLogin POSTs the public user-code and private verifier to the base
// that start resolved.
// Returns ("", nil) while pending (202), the API key exactly once when approved
// (200) and ErrRateLimited on 429. A 410 maps by its errorCode:
// CLI_AUTH_KEY_ALREADY_DELIVERED -> ErrKeyAlreadyDelivered (a dangling key
// exists), anything else (or an absent/unparseable body) -> ErrNonceInvalid.
func PollBrowserLogin(ctx context.Context, start *BrowserLoginStart, hc *http.Client) (string, error) {
	if err := httpx.ValidateBaseURL(start.BaseURL); err != nil {
		return "", err
	}
	if hc == nil {
		hc = httpx.NewClient(10 * time.Second)
	}
	body, err := json.Marshal(map[string]string{
		"userCode":       start.UserCode,
		"deviceVerifier": start.deviceVerifier,
	})
	if err != nil {
		return "", err
	}
	endpoint := httpx.APIBaseURL(start.BaseURL) + "/cli-auth/poll"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Status string `json:"status"`
			APIKey string `json:"apiKey"`
		}
		// A 200 means the backend already consumed the flow and handed out the
		// key. Anything other than a clean APPROVED body with a key (decode error,
		// wrong status, empty key — or a spurious 200 from a proxy) means that key
		// is unusable/lost. Terminal, not transient (see ErrKeyDeliveredUnreadable).
		if err := decodeAuthResponse(resp.Body, &out); err != nil || out.Status != "APPROVED" || out.APIKey == "" {
			return "", ErrKeyDeliveredUnreadable
		}
		return out.APIKey, nil
	case http.StatusAccepted:
		return "", nil
	case http.StatusGone:
		// Two distinct 410s. CLI_AUTH_KEY_ALREADY_DELIVERED means the flow was
		// approved and its key already handed out once (a delivering 200 we
		// missed before this explicit follow-up poll) — a
		// dangling key exists. Anything else (expired/denied/unknown) is a plain
		// CLI_AUTH_NONCE_INVALID. An absent or unparseable body defaults to the
		// safe ErrNonceInvalid: never raise a false dangling-key warning.
		var out struct {
			ErrorCode string `json:"errorCode"`
		}
		if err := decodeAuthResponse(resp.Body, &out); err == nil && out.ErrorCode == "CLI_AUTH_KEY_ALREADY_DELIVERED" {
			return "", ErrKeyAlreadyDelivered
		}
		return "", ErrNonceInvalid
	case http.StatusTooManyRequests:
		return "", ErrRateLimited
	default:
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, endpoint)
	}
}
