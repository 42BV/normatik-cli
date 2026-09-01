// Package problem decodes the Normatik public API's RFC-7807 ProblemDetail
// responses and maps them to actionable CLI output + exit codes.
//
// Design choice (the CLI's value-add): we decode the RAW error body into a
// generic map so that EVERY field — including hint fields the CLI does not yet
// know about — is preserved (Raw). Known fields are additionally lifted into
// typed fields for ergonomic access. Exit-code policy lives in one place
// (internal/catalog.ExitFor); the synthesized "next command" is a data-driven
// registry keyed by errorCode rather than a growing switch.
package problem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/42BV/normatik-cli/internal/catalog"
)

type Diagnostic struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
	Message   string `json:"message"`
}

// FieldError is one entry of a BindException-style errors[] array.
// Field is the server path (e.g. "description" or "values[0].value"), not a CLI flag.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Line formats the field error as "field: message". Nested paths stay as-is.
func (e FieldError) Line() string {
	switch {
	case e.Field == "":
		return e.Message
	case e.Message == "":
		return e.Field
	default:
		return e.Field + ": " + e.Message
	}
}

// Problem is a decoded ProblemDetail. Known fields are typed; Raw holds every
// field verbatim for forward-compatible surfacing.
type Problem struct {
	Status          int
	Title           string
	Detail          string
	ErrorCode       string
	Hint            string
	Reason          string
	ValidKeys       []string
	ValidNames      []string
	InvalidKeys     []string
	UnknownKeys     []string
	ValidValues     []string
	AllowedMethods  []string
	CurrentStatus   string
	RequestedAction string
	RequiredRole    string
	Field           string
	ReceivedValue   string
	MinValue        string
	MaxValue        string
	ReceivedZone    string
	EntityType      string
	CurrentVersion  string
	UsageCount      *int
	// RetryAfterSeconds and InTrash drive recovery suggestions.
	RetryAfterSeconds *int
	InTrash           bool
	Diagnostics       []Diagnostic
	Errors            []FieldError
	Raw               map[string]json.RawMessage
}

// Decode parses an error body. ok=false means the body is NOT a recognizable
// ProblemDetail (e.g. a Tomcat HTML 404) — callers treat that as a distinct
// "malformed backend response" class instead of pretending it has an errorCode.
func Decode(status int, body []byte) (*Problem, bool) {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	_, hasCode := raw["errorCode"]
	_, hasTitle := raw["title"]
	if !hasCode && !hasTitle {
		return nil, false
	}
	p := &Problem{Status: status, Raw: raw}
	p.Title = str(raw, "title")
	p.Detail = str(raw, "detail")
	p.ErrorCode = str(raw, "errorCode")
	p.Hint = str(raw, "hint")
	p.Reason = str(raw, "reason")
	p.ValidKeys = strs(raw, "validKeys")
	p.ValidNames = strs(raw, "validNames")
	p.InvalidKeys = strs(raw, "invalidKeys")
	p.UnknownKeys = strs(raw, "unknownKeys")
	p.ValidValues = strs(raw, "validValues")
	p.AllowedMethods = strs(raw, "allowedMethods")
	p.CurrentStatus = str(raw, "currentStatus")
	p.RequestedAction = str(raw, "requestedAction")
	p.RequiredRole = str(raw, "requiredRole")
	p.Field = str(raw, "field")
	p.ReceivedValue = scalarStr(raw, "receivedValue")
	p.MinValue = scalarStr(raw, "minValue")
	p.MaxValue = scalarStr(raw, "maxValue")
	p.ReceivedZone = str(raw, "receivedZone")
	p.EntityType = str(raw, "entityType")
	p.CurrentVersion = scalarStr(raw, "currentVersion")
	p.UsageCount = intPtr(raw, "usageCount")
	p.RetryAfterSeconds = intPtr(raw, "retryAfterSeconds")
	p.InTrash = boolVal(raw, "inTrash")
	if v, ok := raw["diagnostics"]; ok {
		_ = json.Unmarshal(v, &p.Diagnostics)
	}
	if v, ok := raw["errors"]; ok {
		_ = json.Unmarshal(v, &p.Errors)
	}
	return p, true
}

// typedKeys are the ProblemDetail members Decode lifts into typed fields, plus
// the RFC 9457 base members (type/instance/status/title/detail). Every other
// member the server sends is an "extra" (see Extras).
var typedKeys = map[string]bool{
	"type": true, "instance": true, "status": true, "title": true, "detail": true,
	"errorCode": true, "hint": true, "reason": true,
	"validKeys": true, "validNames": true, "invalidKeys": true, "unknownKeys": true,
	"validValues": true, "allowedMethods": true, "currentStatus": true,
	"requestedAction": true, "requiredRole": true, "field": true, "receivedValue": true,
	"minValue": true, "maxValue": true, "receivedZone": true, "entityType": true,
	"currentVersion": true, "usageCount": true, "retryAfterSeconds": true,
	"inTrash": true, "diagnostics": true, "errors": true,
}

// Extra is one server-sent ProblemDetail member the CLI has no typed field
// for (e.g. conflictingPageId, blockingDescriptors, totalCount). Value is the
// verbatim JSON so nothing is lost or reinterpreted.
type Extra struct {
	Key   string
	Value json.RawMessage
}

// Extras returns the untyped members of the decoded body in key order. This is
// the forward-compatibility contract of Raw: a new server hint field surfaces
// in the envelope (and, when scalar, in table mode) without a CLI release.
func (p *Problem) Extras() []Extra {
	if p == nil || len(p.Raw) == 0 {
		return nil
	}
	keys := make([]string, 0, len(p.Raw))
	for k := range p.Raw {
		if !typedKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]Extra, 0, len(keys))
	for _, k := range keys {
		out = append(out, Extra{Key: k, Value: p.Raw[k]})
	}
	return out
}

// ScalarText renders a JSON scalar (string, number, bool) as plain text and
// reports false for null, arrays and objects — table mode prints only scalars.
func (e Extra) ScalarText() (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(e.Value))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}

func (p *Problem) Error() string {
	if p.ErrorCode != "" {
		return fmt.Sprintf("%s: %s", p.ErrorCode, p.Detail)
	}
	return fmt.Sprintf("HTTP %d: %s", p.Status, p.Detail)
}

// ExitCode maps a problem to a stable, documented exit code (see `explain
// exit-codes`). The policy is centralized in catalog.ExitFor so the CLI and the
// generated catalogue can never disagree. Agents branch on the exit code first,
// then on the errorCode for precision.
func (p *Problem) ExitCode() int {
	return catalog.ExitFor(p.ErrorCode, p.Status)
}

// synthFunc builds a runnable "next command" (or advisory line) from a decoded
// problem. base is the failing invocation, e.g. "normatik pages list".
type synthFunc func(p *Problem, base string) string

// synth is the data-driven recovery registry: errorCode -> suggestion builder.
// Adding a recovery hint is a map entry, not a switch arm.
var synth = map[string]synthFunc{
	"INVALID_SORT": func(p *Problem, base string) string {
		if p.Reason == "MULTI_SORT_FORBIDDEN" {
			return base + " (use at most one --sort)"
		}
		if len(p.ValidKeys) > 0 {
			return fmt.Sprintf("%s --sort %s,asc", base, p.ValidKeys[0])
		}
		return ""
	},
	"EXPAND_UNKNOWN_KEY": func(p *Problem, base string) string {
		if len(p.ValidKeys) > 0 {
			return fmt.Sprintf("%s --expand %s", base, strings.Join(p.ValidKeys, ","))
		}
		return ""
	},
	"INVALID_REQUEST": func(p *Problem, _ string) string {
		// p.Field is a request-body field name (camelCase), not necessarily a
		// kebab-case CLI flag — phrase as a value list, not a --flag.
		if p.Field != "" && len(p.ValidValues) > 0 {
			return fmt.Sprintf("valid values for '%s': %s", p.Field, strings.Join(p.ValidValues, ", "))
		}
		return ""
	},
	"NUMBER_OUT_OF_RANGE": func(p *Problem, _ string) string {
		if p.MinValue != "" || p.MaxValue != "" {
			return fmt.Sprintf("value %s out of range [%s, %s]", p.ReceivedValue, dash(p.MinValue), dash(p.MaxValue))
		}
		return ""
	},
	"INVALID_TRANSITION": func(p *Problem, _ string) string {
		if p.CurrentStatus != "" {
			return fmt.Sprintf("normatik pages revisions snapshot <id>   # current status: %s", p.CurrentStatus)
		}
		return ""
	},
	// After a stale/concurrent write the safe next step is to re-read the current
	// state, review the other change, then retry — a blind retry would clobber it.
	// A `pages update <id>` invocation carries the id (updateInvocation), so emit a
	// runnable `pages get <id>` and surface the fresh version for the retry. Other
	// entities fall back to the generic advisory.
	"CONCURRENT_UPDATE": func(p *Problem, base string) string {
		if strings.HasPrefix(base, "normatik pages update ") {
			if id := argAfter(base, "update"); id != "" {
				if p.CurrentVersion != "" {
					return fmt.Sprintf("normatik pages get %s   # changed concurrently to version %s; review, then re-run your update with --version %s", id, p.CurrentVersion, p.CurrentVersion)
				}
				return fmt.Sprintf("normatik pages get %s   # changed concurrently; review, then re-run your update", id)
			}
		}
		if p.CurrentVersion != "" {
			return fmt.Sprintf("re-GET the resource (current version %s) and try again", p.CurrentVersion)
		}
		return "re-GET the resource, take the fresh version and try again"
	},
	"RATE_LIMIT_EXCEEDED": func(p *Problem, _ string) string {
		if p.RetryAfterSeconds != nil {
			return fmt.Sprintf("wait %ds and try again", *p.RetryAfterSeconds)
		}
		return ""
	},
	"PAGE_NOT_FOUND": func(p *Problem, _ string) string {
		if p.InTrash {
			// `trash` is a top-level command, not a `pages` subcommand.
			return "normatik trash restore <id>   # the page is in the trash"
		}
		return ""
	},
	"METHOD_NOT_ALLOWED": func(p *Problem, _ string) string {
		if len(p.AllowedMethods) > 0 {
			return "allowed methods: " + strings.Join(p.AllowedMethods, ", ")
		}
		return ""
	},
	// The server hint (test-locked) points at GET /public/v1/page-types; a CLI
	// user wants a runnable CLI command, so add a CLI-native complement.
	"INVALID_PARENT_PAGE_TYPE": func(p *Problem, _ string) string {
		return "normatik page-types get <parentId>   # check allowedChildTypes on the parent"
	},
	// The server hint (test-locked) points at raw REST (POST .../revisions/start);
	// a CLI user wants the runnable command.
	"REVISION_NOT_EDITABLE": func(p *Problem, _ string) string {
		return "normatik pages revisions start <id>   # create a STORED working revision, then retry"
	},
	// Server staticHint names the API field acknowledgePublished=true; the CLI
	// surface is the boolean flag --acknowledge-published on cascade-archive/trash.
	"PAGE_CASCADE_PUBLISHED_ACKNOWLEDGMENT_REQUIRED": func(p *Problem, base string) string {
		if base != "" {
			return base + " --acknowledge-published   # published pages in the cascade will go offline"
		}
		return "retry with --acknowledge-published   # published pages in the cascade will go offline"
	},
	// The wire problem carries validNames (bounded — the enabled directive
	// macros) but NOT the requested name; that comes from the failing
	// invocation ("normatik macros docs tok"), so the did-you-mean is a
	// closest-match of the invocation's <name> against validNames.
	"CONTENT_MACRO_NOT_FOUND": func(p *Problem, base string) string {
		if len(p.ValidNames) > 0 {
			if match := closestName(argAfter(base, "docs"), p.ValidNames); match != "" {
				return fmt.Sprintf("normatik macros docs %s   # did you mean this macro?", match)
			}
		}
		return "normatik macros docs   # list all enabled macros"
	},
	// UNKNOWN_DIRECTIVE is a DIAGNOSTIC code (an entry in diagnostics[]), not an
	// errorCode — an entry keyed "UNKNOWN_DIRECTIVE" would never fire. A blocked
	// save carries it under errorCode CONTENT_VALIDATION_FAILED, so THAT is the
	// synth key and we inspect p.Diagnostics. The diagnostic message format
	// ("Unknown directive 'x'. Did you mean 'y'? ...") is test-locked in the
	// backend (ContentSyntaxValidator), so plain string extraction is safe.
	"CONTENT_VALIDATION_FAILED": func(p *Problem, _ string) string {
		// Prefer the first UNKNOWN_DIRECTIVE that carries a did-you-mean: with
		// multiple unknown directives the concrete suggestion may sit on a later
		// diagnostic, and a runnable name beats the generic list variant.
		sawUnknownDirective := false
		for _, d := range p.Diagnostics {
			if d.Code != "UNKNOWN_DIRECTIVE" {
				continue
			}
			sawUnknownDirective = true
			if name := didYouMean(d.Message); name != "" {
				return fmt.Sprintf("normatik macros docs %s   # syntax and attributes for this macro", name)
			}
		}
		if sawUnknownDirective {
			return "normatik macros docs   # list all enabled macros and their syntax"
		}
		for _, d := range p.Diagnostics {
			if d.Code == "UNSUPPORTED_MARKDOWN_TABLE" {
				return "normatik macros docs table   # :::table syntax"
			}
			if d.Code == "UNSUPPORTED_MARKDOWN_FOOTNOTE" || d.Code == "UNSUPPORTED_MARKDOWN_TASK_LIST" {
				return "normatik macros docs   # supported markdown dialect"
			}
			if d.Code == "INVALID_FILTER_SYNTAX" {
				return "normatik macros docs   # filter syntax"
			}
		}
		// Other validation failures already carry their own diagnostics lines.
		return ""
	},
}

// Suggestion synthesizes a runnable "next command" from the structured hint
// fields, so an agent (or human) can self-correct in one loop.
func (p *Problem) Suggestion(base string) string {
	if fn, ok := synth[p.ErrorCode]; ok {
		return fn(p, base)
	}
	return ""
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// didYouMean extracts the suggested directive name from a backend
// UNKNOWN_DIRECTIVE diagnostic message ("Unknown directive 'x'. Did you mean
// 'y'? ..."). Defensive: returns "" when the marker or closing quote is absent.
func didYouMean(message string) string {
	const marker = "Did you mean '"
	i := strings.Index(message, marker)
	if i < 0 {
		return ""
	}
	rest := message[i+len(marker):]
	j := strings.Index(rest, "'")
	if j <= 0 {
		return ""
	}
	return rest[:j]
}

// argAfter returns the first whitespace-separated token following the given
// token in an invocation ("normatik macros docs tok" → "tok"), or "" when the
// invocation carried none.
func argAfter(invocation, token string) string {
	fields := strings.Fields(invocation)
	for i, f := range fields {
		if f == token && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// closestName returns the candidate with the smallest edit distance (<=3) to
// name; empty if nothing is close. Local Levenshtein on purpose: the cli
// package's helper would create an import cycle and catalog.Closest only
// searches the error-code catalogue, not arbitrary candidate lists.
func closestName(name string, candidates []string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	best, bestD := "", 4
	for _, c := range candidates {
		if d := editDistance(name, strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func str(m map[string]json.RawMessage, k string) string {
	if v, ok := m[k]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return ""
}

// scalarStr returns a field as a string whether the JSON value is a string or a
// number (receivedValue/min/max/currentVersion may be either).
func scalarStr(m map[string]json.RawMessage, k string) string {
	v, ok := m[k]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(v, &f) == nil {
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return ""
}

func strs(m map[string]json.RawMessage, k string) []string {
	if v, ok := m[k]; ok {
		var s []string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return nil
}

// intPtr decodes an integer field tolerantly: it accepts both JSON ints and
// integral floats (30 and 30.0) so a forward-compat decoder never silently
// drops a retryAfterSeconds/usageCount serialized as a float.
func intPtr(m map[string]json.RawMessage, k string) *int {
	v, ok := m[k]
	if !ok {
		return nil
	}
	var f float64
	if json.Unmarshal(v, &f) == nil {
		n := int(f)
		return &n
	}
	return nil
}

func boolVal(m map[string]json.RawMessage, k string) bool {
	if v, ok := m[k]; ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
	}
	return false
}
