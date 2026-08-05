package cli

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/42BV/normatik-cli/internal/auth"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/httpx"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/spf13/cobra"
)

// browserLoginTimeout matches the server-side device-flow TTL (10 min): polling past
// it can only ever return 410. Package var so tests can shorten it.
var browserLoginTimeout = 10 * time.Minute

// browserPollMinInterval is the lower bound on the server-advised poll cadence.
// Package var so tests can poll fast.
var browserPollMinInterval = time.Second

// danglingKeyNote warns that a key may exist server-side after an ambiguous
// failure (a 200 lost in transit, or a transport drop before a 410). "may have
// been" also covers a misbehaving proxy returning a spurious 200.
const danglingKeyNote = "  Note: if you approved this login, a new API key may have been created on the server. Check your Profile and revoke it if you did not expect it."

// openBrowser launches the OS default browser. Package var so tests can record
// the call instead of opening a real browser (same pattern as
// confirmProfileOverwrite).
var openBrowser = func(u string) error {
	if err := validateBrowserURL(u); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		// #nosec G204 -- fixed executable; u is a validated absolute HTTP(S) URL passed without a shell.
		return exec.Command("open", u).Start()
	case "windows":
		// #nosec G204 -- fixed executable; u is a validated absolute HTTP(S) URL passed without a shell.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		// #nosec G204 -- fixed executable; u is a validated absolute HTTP(S) URL passed without a shell.
		return exec.Command("xdg-open", u).Start()
	}
}

func validateBrowserURL(raw string) error {
	if err := httpx.ValidateBaseURL(raw); err != nil {
		return errors.New("refusing to open invalid browser URL")
	}
	return nil
}

// browserCapable reports whether opening a browser makes sense: an interactive
// stdout without SSH/CI markers. Browserless runs still poll — they only skip
// the open and print the approval URL. Package var so tests can pin it.
var browserCapable = func() bool {
	if !stdoutIsTTY() {
		return false
	}
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if os.Getenv("CI") == "true" {
		return false
	}
	return true
}

// loginKeyNameSuggestion builds the prefill key-name for the approval page.
func loginKeyNameSuggestion() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "CLI"
	}
	return "CLI — " + host
}

// runBrowserLogin is the default login path: start a server-side
// flow, let the user approve in the browser, poll for the freshly minted key
// and store it via the exact same tail as the paste path (finishLogin). The
// received key is never printed or logged. The environment URL is already
// resolved by the caller (resolveLoginURL — flag, env, active profile or a
// prompt) and is non-empty here; there is no implicit localhost fallback.
func runBrowserLogin(cmd *cobra.Command, p *render.Printer, profile, site string, readOnly, noBrowser bool) error {
	site = strings.TrimSpace(site)

	// Ctrl-C during the poll loop must abort cleanly: nothing is stored until
	// finishLogin, so cancelling here leaves no half state behind.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start, err := auth.StartBrowserLogin(ctx, site, loginKeyNameSuggestion(), readOnly, nil)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrBrowserLoginUnsupported):
			p.Message("Error [LOGIN_FAILED]: this server does not support browser login yet.")
			p.Message("  Try:  normatik login --url %s --paste  (paste an existing API key securely)", site)
			return command.Handled(4)
		case errors.Is(err, auth.ErrRateLimited):
			p.Message("Error [RATE_LIMIT_EXCEEDED]: too many login attempts — wait a moment and try again.")
			return command.Handled(75)
		default:
			p.Message("Error [LOGIN_FAILED]: %v", err)
			p.Message("  Hint: use an https:// environment URL and check whether the backend is reachable.")
			return command.Handled(4)
		}
	}

	// De URL wordt ALTIJD geprint; openen alleen wanneer gewenst én mogelijk.
	if noBrowser || !browserCapable() {
		p.Message("Open this link in your browser to approve the login:")
		p.Message("  %s", start.BrowserURL)
	} else if openErr := openBrowser(start.BrowserURL); openErr != nil {
		p.Message("Could not open a browser (%v).", openErr)
		p.Message("Open this link to approve the login: %s", start.BrowserURL)
	} else {
		p.Message("Your browser opens to approve this login.")
		p.Message("Open this link if the browser does not open: %s", start.BrowserURL)
	}

	interval := time.Duration(start.IntervalSeconds) * time.Second
	if interval < browserPollMinInterval {
		interval = browserPollMinInterval
	}
	deadline := time.Now().Add(browserLoginTimeout)
	p.Message("Waiting for approval (Ctrl-C to cancel)...")

	// Transient poll failures (5xx, a network blip, a per-call timeout) do NOT
	// abort the flow: the server-side authorization stays valid, so keep polling until
	// the deadline. Only an explicit 410 (expired/denied/used) or a ctx-cancel
	// ends early. A small budget bounds how many CONSECUTIVE transient failures
	// we tolerate, so a permanently-broken endpoint still exits instead of
	// hammering until the deadline.
	const maxTransientPollErrors = 5
	transientErrors := 0
	for {
		key, err := auth.PollBrowserLogin(ctx, start, nil)
		switch {
		case err == nil && key != "":
			return finishLogin(cmd, p, profile, start.BaseURL, key, true)
		case errors.Is(err, auth.ErrKeyAlreadyDelivered):
			// The backend confirmed (errorCode CLI_AUTH_KEY_ALREADY_DELIVERED) that
			// the flow was approved and its key already delivered once. A usable
			// key exists server-side and cannot be
			// re-fetched — terminal; warn about the dangling credential.
			p.Message("Error [LOGIN_FAILED]: approval succeeded but the key was already delivered and cannot be retrieved.")
			p.Message("%s", danglingKeyNote)
			return command.Handled(4)
		case errors.Is(err, auth.ErrNonceInvalid):
			// A plain 410 (CLI_AUTH_NONCE_INVALID): the flow is expired, denied or
			// unknown — no key was ever approved+delivered, so there is nothing to
			// revoke. The lost-delivery case is caught above via its own errorCode.
			p.Message("Error [LOGIN_FAILED]: the approval expired or was denied — start again with `normatik login`.")
			return command.Handled(4)
		case errors.Is(err, auth.ErrKeyDeliveredUnreadable):
			// The server approved and consumed the flow, but the key response
			// was unusable (truncated/wrong-status 200) — the key was minted and
			// cannot be recovered. Terminal; warn about the dangling credential.
			p.Message("Error [LOGIN_FAILED]: approval succeeded but the key could not be retrieved.")
			p.Message("%s", danglingKeyNote)
			return command.Handled(4)
		case ctx.Err() != nil:
			p.Message("Login cancelled — nothing was stored.")
			return command.Handled(130)
		case errors.Is(err, auth.ErrRateLimited):
			// Polling faster than the server allows: skip this round, keep waiting.
			transientErrors = 0
		case err != nil:
			// Transient failure: tolerate a bounded number in a row, then give up.
			// Repeated transport errors are the most likely moment a delivering 200
			// was lost, so the give-up path warns about a possible dangling key.
			transientErrors++
			if transientErrors > maxTransientPollErrors {
				p.Message("Error [LOGIN_FAILED]: %v", err)
				p.Message("  Hint: repeated errors while polling — check connectivity and run `normatik login` again.")
				p.Message("%s", danglingKeyNote)
				return command.Handled(4)
			}
		default:
			// Still pending (202): reset the transient budget.
			transientErrors = 0
		}
		if time.Now().After(deadline) {
			p.Message("Error [LOGIN_FAILED]: no approval within %s — start again with `normatik login`.", browserLoginTimeout)
			return command.Handled(4)
		}
		// Never wait past the deadline: a large server-advised interval (only its
		// lower bound is clamped above) must not overshoot the flow TTL.
		wait := interval
		if rem := time.Until(deadline); rem < wait {
			wait = rem
		}
		select {
		case <-ctx.Done():
			p.Message("Login cancelled — nothing was stored.")
			return command.Handled(130)
		case <-time.After(wait):
		}
	}
}
