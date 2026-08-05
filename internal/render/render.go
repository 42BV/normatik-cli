// Package render implements dual-mode output: a human table/text default and a
// stable machine-readable JSON mode (--output json). Data goes to stdout;
// errors and diagnostics go to stderr so `normatik ... | jq` stays clean.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/problem"
)

type Mode int

const (
	Table Mode = iota
	JSON
)

type Printer struct {
	Mode Mode
	Out  io.Writer
	Err  io.Writer
	// Fields, when non-empty, overrides the derived columns in table mode
	// (--fields id,name). It never affects JSON mode (verbatim contract).
	Fields []string
	// Quiet drops the header row and the pagination/meta footer in table mode.
	Quiet bool
}

func New(output string) *Printer {
	m := Table
	if output == "json" {
		m = JSON
	}
	return &Printer{Mode: m, Out: os.Stdout, Err: os.Stderr}
}

func (p *Printer) json(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// PageList renders a Page<PageListResult> as a table (or raw JSON). It keeps the
// nicely-labelled default columns, but honours --fields (routing through the
// generic, schema-aware List so projection + unknown-field warnings work) and
// --quiet (dropping the header + footer).
func (p *Printer) PageList(list *api.PagePageListResult) {
	if p.Mode == JSON {
		p.json(p.Out, list)
		return
	}
	if len(p.Fields) > 0 {
		if raw, err := json.Marshal(list); err == nil {
			p.List(raw)
			return
		}
	}
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	if !p.Quiet {
		fmt.Fprintln(tw, "ID\tNAME\tPAGE TYPE\tCHILDREN")
	}
	if list.Content != nil {
		for _, it := range *list.Content {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
				i64(it.Id), s(it.Name), s(it.PageTypeName), yesno(it.HasChildren))
		}
	}
	_ = tw.Flush()
	if !p.Quiet {
		fmt.Fprintf(p.Err, "\npage %d/%d · %d items total (size %d)\n",
			i32(list.Number), i32(list.TotalPages), i64(list.TotalElements), i32(list.Size))
	}
}

// Raw renders an arbitrary JSON body: pretty in JSON mode, or a flat key/value
// summary of selected fields in table mode.
func (p *Printer) Raw(body []byte, humanFields ...string) {
	if p.Mode == JSON {
		var v any
		if json.Unmarshal(body, &v) == nil {
			p.json(p.Out, v)
		} else {
			fmt.Fprintln(p.Out, terminalLine(string(body)))
		}
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	// --fields overrides; otherwise the per-command fields; otherwise derive
	// identifying scalar keys so the user never sees an empty screen.
	humanFields = p.resolveFields(humanFields, deriveObjectFields(m), rowKeys([]map[string]any{m}))
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	for _, f := range humanFields {
		if val, ok := m[f]; ok && val != nil {
			fmt.Fprintf(tw, "%s\t%s\n", f, cell(val))
		}
	}
	_ = tw.Flush()
}

// Problem renders a decoded ProblemDetail with hint + a synthesized next command.
// suggestion is the runnable "Try:" line (may be empty).
func (p *Printer) Problem(pr *problem.Problem, suggestion string) {
	if p.Mode == JSON {
		p.json(p.Err, map[string]any{"error": envelope(pr, suggestion)})
		return
	}
	fmt.Fprintf(p.Err, "Error [%s] (HTTP %d): %s\n", terminalLine(pr.ErrorCode), pr.Status, terminalLine(pr.Detail))
	if pr.Hint != "" {
		fmt.Fprintf(p.Err, "  Hint: %s\n", terminalLine(pr.Hint))
	}
	if len(pr.ValidKeys) > 0 {
		fmt.Fprintf(p.Err, "  Valid keys: %v\n", terminalLines(pr.ValidKeys))
	}
	if len(pr.ValidNames) > 0 {
		fmt.Fprintf(p.Err, "  Valid names: %v\n", terminalLines(pr.ValidNames))
	}
	if pr.CurrentStatus != "" {
		fmt.Fprintf(p.Err, "  Current status: %s\n", terminalLine(pr.CurrentStatus))
	}
	if pr.RequiredRole != "" {
		fmt.Fprintf(p.Err, "  Required role: %s\n", terminalLine(pr.RequiredRole))
	}
	if pr.RetryAfterSeconds != nil {
		fmt.Fprintf(p.Err, "  Retry-after: %ds\n", *pr.RetryAfterSeconds)
	}
	for _, d := range pr.Diagnostics {
		fmt.Fprintf(p.Err, "  %d:%d %s %s: %s\n", d.Line, d.Column, terminalLine(d.Severity), terminalLine(d.Code), terminalLine(d.Message))
	}
	if suggestion != "" {
		fmt.Fprintf(p.Err, "  Try:  %s\n", terminalLine(suggestion))
	}
}

// Malformed renders a non-ProblemDetail error response (e.g. Tomcat HTML 404).
func (p *Printer) Malformed(status int, body []byte) {
	if p.Mode == JSON {
		p.json(p.Err, map[string]any{"error": map[string]any{
			"code": "MALFORMED_RESPONSE", "status": status,
			"detail": "server did not return a ProblemDetail", "exitCode": 65,
		}})
		return
	}
	fmt.Fprintf(p.Err, "Error: HTTP %d without ProblemDetail (no errorCode).\n", status)
	fmt.Fprintf(p.Err, "  Body: %s\n", terminalLine(truncate(body, 200)))
}

func (p *Printer) Message(format string, a ...any) {
	clean := make([]any, len(a))
	for i, value := range a {
		clean[i] = terminalArg(value)
	}
	fmt.Fprintf(p.Err, format+"\n", clean...)
}

// DryRun prints a resolved write payload that WOULD be sent (pretty JSON on
// stdout) followed by a "no changes were saved" note on stderr. Used by the
// --dry-run property preview so an agent can inspect the exact form the CLI
// resolved without any write ever happening. The note lives on stderr so
// `normatik ... --dry-run -o json | jq` still sees only the payload.
func (p *Printer) DryRun(payload any) {
	p.json(p.Out, payload)
	fmt.Fprintln(p.Err, "dry-run: no changes were saved")
}

// List renders a list response — either a Page<X> envelope (content[] + paging)
// or a bare JSON array — as a table in table mode, or verbatim in JSON mode.
// fields selects/orders the columns; empty derives identifying columns from the
// first row. Nested objects/arrays are skipped as columns (use --output json).
func (p *Printer) List(body []byte, fields ...string) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	rows, meta := extractRows(body)
	if rows == nil {
		p.Raw(body, fields...) // not a recognizable list shape
		return
	}
	fields = p.resolveFields(fields, deriveFields(rows), rowKeys(rows))
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	if !p.Quiet {
		header := make([]string, len(fields))
		for i, f := range fields {
			header[i] = strings.ToUpper(f)
		}
		fmt.Fprintln(tw, strings.Join(header, "\t"))
	}
	for _, r := range rows {
		cells := make([]string, len(fields))
		for i, f := range fields {
			cells[i] = cell(r[f])
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	_ = tw.Flush()
	if meta != "" && !p.Quiet {
		fmt.Fprintf(p.Err, "\n%s\n", meta)
	}
}

// resolveFields picks the columns to render: an explicit --fields override
// (p.Fields) wins over the per-command default and the derived columns. When
// --fields names a key that appears in NO row, we warn on stderr rather than
// silently rendering an empty column.
func (p *Printer) resolveFields(cmdFields, derived []string, present map[string]bool) []string {
	// User-supplied --fields wins and is validated (warn on a field absent from
	// every row). A command's built-in default field list may legitimately
	// over-request optional fields, so it is NOT warned about — only projected.
	if len(p.Fields) > 0 {
		var missing []string
		for _, f := range p.Fields {
			if !present[f] {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(p.Err, "warning: field(s) %v not present in the response (column stays empty)\n", missing)
		}
		return p.Fields
	}
	if len(cmdFields) == 0 {
		return derived
	}
	return cmdFields
}

func rowKeys(rows []map[string]any) map[string]bool {
	present := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			present[k] = true
		}
	}
	return present
}

func (p *Printer) rawDump(body []byte) {
	var v any
	if json.Unmarshal(body, &v) == nil {
		// Valid JSON: json.Encoder escapet control-bytes in string-waarden al.
		p.json(p.Out, v)
	} else {
		// NORMATIK-19 (CWE-150): een malformed 2xx-body die niet als JSON parst
		// werd letterlijk naar de terminal geschreven; ESC/OSC/BEL/CR kon zo de
		// terminalweergave beinvloeden. Sanitize diagnostische bytes (behoud LF).
		fmt.Fprintln(p.Out, terminalText(string(body)))
	}
}

// extractRows pulls the row objects out of a Page<X> envelope or a bare array,
// plus a one-line pagination footer when present.
func extractRows(body []byte) ([]map[string]any, string) {
	var arr []map[string]any
	if json.Unmarshal(body, &arr) == nil && arr != nil {
		return arr, ""
	}
	var env map[string]any
	if json.Unmarshal(body, &env) != nil {
		return nil, ""
	}
	content, ok := env["content"].([]any)
	if !ok {
		return nil, ""
	}
	rows := make([]map[string]any, 0, len(content))
	for _, it := range content {
		if m, ok := it.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	meta := ""
	if num, ok := env["number"]; ok {
		meta = fmt.Sprintf("page %v/%v · %v items total", numStr(num), numStr(env["totalPages"]), numStr(env["totalElements"]))
	}
	return rows, meta
}

var preferredFields = []string{"id", "slug", "name", "displayName", "title", "email", "status", "role", "workflowRole", "pageTypeName", "parentId"}

func deriveFields(rows []map[string]any) []string {
	if len(rows) == 0 {
		return []string{"id"}
	}
	return deriveObjectFields(rows[0])
}

// deriveObjectFields picks identifying scalar columns: preferred fields first (in
// order), then the remaining scalar keys (sorted), capped so wide objects stay
// readable. Nested objects/arrays are never columns (use --output json).
func deriveObjectFields(m map[string]any) []string {
	const maxCols = 6
	seen := map[string]bool{}
	var out []string
	for _, f := range preferredFields {
		if v, ok := m[f]; ok && isScalar(v) {
			out = append(out, f)
			seen[f] = true
		}
	}
	var rest []string
	for k, v := range m {
		if !seen[k] && isScalar(v) {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		if len(out) >= maxCols {
			break
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		out = []string{"id"}
	}
	return out
}

func cell(v any) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case string:
		return terminalLine(t)
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case float64:
		return numStr(t)
	default:
		return "…" // nested object/array: use --output json
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, bool, float64, nil:
		return true
	}
	return false
}

func numStr(v any) string {
	switch t := v.(type) {
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case nil:
		return "?"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ProfileInfo is the per-profile view for `auth list`: the base-URL plus
// whether the OS keychain holds a key for it. It never carries key material.
type ProfileInfo struct {
	BaseURL string `json:"baseUrl"`
	HasKey  bool   `json:"hasKey"`
}

// Profiles renders the known environment profiles (name + base-URL + key
// presence), marking the active one. API keys are never shown. JSON mode emits
// {active, profiles: {name: {baseUrl, hasKey}}}.
func (p *Printer) Profiles(active string, profiles map[string]ProfileInfo) {
	if p.Mode == JSON {
		p.json(p.Out, map[string]any{"active": active, "profiles": profiles})
		return
	}
	if len(profiles) == 0 {
		fmt.Fprintln(p.Out, "(no profiles — log in with `normatik login`)")
		return
	}
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ACTIVE\tPROFILE\tBASE-URL\tKEY")
	for _, n := range names {
		mark := ""
		if n == active {
			mark = "*"
		}
		key := "–"
		if profiles[n].HasKey {
			key = "✓"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, n, profiles[n].BaseURL, key)
	}
	_ = tw.Flush()
}

func envelope(pr *problem.Problem, suggestion string) map[string]any {
	e := map[string]any{
		"code": pr.ErrorCode, "status": pr.Status,
		"title": pr.Title, "detail": pr.Detail, "exitCode": pr.ExitCode(),
	}
	put := func(k string, v any) { e[k] = v }
	if pr.Hint != "" {
		put("hint", pr.Hint)
	}
	if pr.Reason != "" {
		put("reason", pr.Reason)
	}
	if len(pr.ValidKeys) > 0 {
		put("validKeys", pr.ValidKeys)
	}
	if len(pr.InvalidKeys) > 0 {
		put("invalidKeys", pr.InvalidKeys)
	}
	if len(pr.ValidNames) > 0 {
		put("validNames", pr.ValidNames)
	}
	if len(pr.UnknownKeys) > 0 {
		put("unknownKeys", pr.UnknownKeys)
	}
	if pr.CurrentStatus != "" {
		put("currentStatus", pr.CurrentStatus)
	}
	if pr.RequiredRole != "" {
		put("requiredRole", pr.RequiredRole)
	}
	if pr.Field != "" {
		put("field", pr.Field)
	}
	if len(pr.ValidValues) > 0 {
		put("validValues", pr.ValidValues)
	}
	if pr.ReceivedValue != "" {
		put("receivedValue", pr.ReceivedValue)
	}
	if pr.MinValue != "" {
		put("minValue", pr.MinValue)
	}
	if pr.MaxValue != "" {
		put("maxValue", pr.MaxValue)
	}
	if len(pr.AllowedMethods) > 0 {
		put("allowedMethods", pr.AllowedMethods)
	}
	if pr.CurrentVersion != "" {
		put("currentVersion", pr.CurrentVersion)
	}
	if pr.RetryAfterSeconds != nil {
		put("retryAfterSeconds", *pr.RetryAfterSeconds)
	}
	if len(pr.Diagnostics) > 0 {
		put("diagnostics", pr.Diagnostics)
	}
	if suggestion != "" {
		put("nextCommand", suggestion)
	}
	return e
}

func s(p *string) string {
	if p == nil {
		return ""
	}
	return terminalLine(*p)
}
func i32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
func i64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
func yesno(p *bool) string {
	if p != nil && *p {
		return "yes"
	}
	return "—"
}
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
