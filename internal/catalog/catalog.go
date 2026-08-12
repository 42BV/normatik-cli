// Package catalog is the offline, embedded error-code knowledge base that
// powers `normatik explain` and the exit-code policy. It is generated from the
// backend sources (ErrorCode.java + PublicApiHints.java) by api/gen-catalog.mjs
// and embedded at build time, so the CLI explains every documented error
// without a network round-trip. `make verify` fails if the JSON drifts.
package catalog

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed errorcodes.json
var raw []byte

// Entry is one documented ErrorCode. HintKind is STATIC|DYNAMIC|NO_HINT —
// matching the backend's three-set discipline (PublicApiHints).
type Entry struct {
	Name        string `json:"name"`
	Status      int    `json:"status"`
	Title       string `json:"title"`
	UserMessage string `json:"userMessage"`
	HintKind    string `json:"hintKind"`
	ExitCode    int    `json:"exitCode"`
	StaticHint  string `json:"staticHint,omitempty"`
}

var (
	entries []Entry
	byName  map[string]Entry
)

func init() {
	if err := json.Unmarshal(raw, &entries); err != nil {
		panic("catalog: invalid embedded errorcodes.json: " + err.Error())
	}
	byName = make(map[string]Entry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
}

// List returns all entries, sorted by name (the generator sorts them).
func List() []Entry { return entries }

// Names returns every error-code name, sorted — used for shell completion.
func Names() []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

// Lookup finds an entry by code (case-insensitive).
func Lookup(code string) (Entry, bool) {
	e, ok := byName[strings.ToUpper(strings.TrimSpace(code))]
	return e, ok
}

// Closest returns the catalogued name with the smallest edit distance (<=3),
// for "did you mean?" suggestions on an unknown code. Empty if nothing is close.
func Closest(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	best, bestD := "", 4
	for _, e := range entries {
		if d := lev(code, e.Name); d < bestD {
			best, bestD = e.Name, d
		}
	}
	return best
}

// ExitClass is one row of the exit-code policy table.
type ExitClass struct {
	Code    int
	Meaning string
}

// ExitClasses is the documented exit-code table (`explain exit-codes`). It is
// the single human-readable companion to ExitFor.
func ExitClasses() []ExitClass {
	return []ExitClass{
		{0, "success"},
		{2, "usage error (wrong flags/args)"},
		{3, "validation/input (INVALID_SORT, CONTENT_VALIDATION_FAILED, 400/422)"},
		{4, "authentication (API_KEY_INVALID/EXPIRED, 401)"},
		{5, "forbidden/role (API_KEY_READ_ONLY, INSUFFICIENT_WORKFLOW_ROLE, 403)"},
		{6, "not found (404)"},
		{7, "conflict/state (CONCURRENT_UPDATE, INVALID_TRANSITION, *_IN_USE, 409)"},
		{65, "malformed response (server did not return a ProblemDetail)"},
		{75, "rate limited (RATE_LIMIT_EXCEEDED, 429) — respect retryAfterSeconds"},
		{78, "configuration (no API key)"},
		{130, "cancelled by the user (Ctrl-C/SIGTERM during an interactive flow)"},
		{1, "other/unknown error"},
	}
}

// authExit and forbiddenExit list the codes whose exit class is policy-driven
// rather than purely HTTP-status-driven (a 401/403 is ambiguous on its own).
var authExit = map[string]bool{
	"API_KEY_INVALID": true, "API_KEY_EXPIRED": true,
}
var forbiddenExit = map[string]bool{
	"API_KEY_READ_ONLY": true, "INSUFFICIENT_WORKFLOW_ROLE": true,
	"PAGE_ACCESS_DENIED": true, "PAGE_WRITE_ACCESS_DENIED": true, "PAGE_RESTRICTION_NOT_OWNER": true,
	"PAGE_CASCADE_WRITE_ACCESS_DENIED": true,
}

// ExitFor is the single source of truth for mapping an (errorCode, httpStatus)
// pair to a stable exit code. Both Problem.ExitCode and any future caller route
// through here. For a known code with no wire status we fall back to the
// catalogued status (defends against a proxy rewriting the HTTP status).
func ExitFor(code string, status int) int {
	code = strings.ToUpper(code)
	switch {
	case authExit[code]:
		return 4
	case forbiddenExit[code]:
		return 5
	case code == "RATE_LIMIT_EXCEEDED":
		return 75
	}
	if status == 0 {
		if e, ok := Lookup(code); ok {
			status = e.Status
		}
	}
	switch status {
	case 400, 422:
		return 3
	case 401:
		return 4
	case 403:
		return 5
	case 404:
		return 6
	case 409:
		return 7
	case 429:
		return 75
	}
	return 1
}

func lev(a, b string) int {
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
