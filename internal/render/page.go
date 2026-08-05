package render

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Page renders a page composite (GET /public/v1/pages/{id} with all expand
// sections) as an ASCII document. JSON mode dumps the composite verbatim.
// It is fully nil-safe: missing maps/arrays/pointers/omitempty fields are
// simply skipped — never a panic.
func (p *Printer) Page(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	fmt.Fprint(p.Out, renderPageASCII(m))
}

// PageGet renders the GET /public/v1/pages/{id} detail for `pages get`: the
// identifying key/value summary (id, name, pageTypeName, parentId) on top, then
// the property values rendered with the SAME formatter as `pages render` (via the
// shared renderPropertiesSection → propValue). This makes the intuitive read-back
// show property values instead of an empty screen. JSON mode stays byte-identical
// to the raw composite body (only human/table mode formats). Nil-safe: a page with
// no property values shows a "(no property values)" note, never a panic.
//
// When working is true, the property section is sourced from the WORKING revision
// (workingRevision.propertyValues) instead of the published values. On a workflow
// page a `pages update --property` lands on the working revision, but the default
// (published) read-back does not show it yet — so `--working` gives the agent a
// readable path to the value it just wrote. When there is no working revision the
// call falls back to the published (top-level) values with an explanatory note.
func (p *Printer) PageGet(body []byte, working bool, humanFields ...string) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	p.Raw(body, humanFields...) // top: flat identifying summary (unchanged)
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return
	}
	var b strings.Builder
	// The canonical descriptor order comes from the TOP-LEVEL composite
	// (availablePropertyDescriptors), which is present in both the published and
	// --working modes. The working revision submap has no availablePropertyDescriptors
	// of its own, but its propertyValues reuse the same descriptor names, so sorting
	// the working values against the top-level order is correct.
	order := descriptorOrder(m)
	// Pick the property source: the working revision when --working is set and one
	// exists, else the published (top-level) values. The "(no property values)"
	// fallback below is applied uniformly to whichever source is chosen, so an
	// empty page renders identically in both modes.
	src := m
	if working {
		// A present workingRevision is the source even when its propertyValues are
		// empty (e.g. after unsetting every property): showing the published values
		// there would be the same misleading read-back --working exists to prevent.
		// gmap returns nil for an absent/null workingRevision, which is the only case
		// that falls back to the published values.
		if wr := gmap(m, "workingRevision"); wr != nil {
			b.WriteString("\n(working revision - not yet published)\n")
			src = wr
		} else {
			b.WriteString("\n(no working revision - showing published values)\n")
		}
	}
	if !renderPropertiesSection(&b, src, order) {
		section(&b, "Properties")
		b.WriteString("  (no property values)\n")
	}
	fmt.Fprint(p.Out, b.String())
}

func renderPageASCII(m map[string]any) string {
	var b strings.Builder

	// Title box.
	name := gstr(m, "name")
	if name == "" {
		name = "(unnamed)"
	}
	bar := strings.Repeat("=", minInt(displayWidth(name)+2, 80))
	fmt.Fprintf(&b, "+%s+\n", bar)
	fmt.Fprintf(&b, "  %s\n", name)
	fmt.Fprintf(&b, "+%s+\n", bar)

	// Meta line.
	var meta []string
	if id := gnumStr(m, "id"); id != "" {
		meta = append(meta, "ID "+id)
	}
	if pt := pageTypeName(m); pt != "" {
		meta = append(meta, "Page type: "+pt)
	}
	if pn := gstr(m, "parentName"); pn != "" {
		meta = append(meta, "Parent: "+pn+parenSuffix(gnumStr(m, "parentId")))
	}
	if v := gnumStr(m, "version"); v != "" {
		meta = append(meta, "v"+v)
	}
	if len(meta) > 0 {
		fmt.Fprintln(&b, strings.Join(meta, "  -  "))
	}
	if crumbs := breadcrumbTrail(m); crumbs != "" {
		fmt.Fprintf(&b, "Breadcrumb: %s\n", crumbs)
	}

	// Properties.
	renderPropertiesSection(&b, m, descriptorOrder(m))

	// Content (markdown -> ASCII).
	section(&b, "Content")
	b.WriteString(markdownToASCII(gtext(m, "content")))
	b.WriteString("\n")

	// Expand sections (lists). Each renders only when present and non-empty.
	renderList(&b, m, "workflow", "Workflow", func(im map[string]any) string {
		return joinNonEmpty(" -> ", firstStr(im, "action", "name"), firstStr(im, "targetStatus", "status"))
	})
	renderList(&b, m, "attachments", "Attachments", func(im map[string]any) string {
		return joinNonEmpty("  ", firstStr(im, "fileName", "name", "filename"), sizeStr(im))
	})
	renderList(&b, m, "images", "Images", func(im map[string]any) string {
		return firstStr(im, "fileName", "name", "filename", "altText")
	})
	renderList(&b, m, "workItems", "Work items", func(im map[string]any) string {
		return joinNonEmpty("  ", firstStr(im, "title", "name", "summary"), bracket(firstStr(im, "status", "workItemTypeName")))
	})

	// Restriction (object).
	if r := gmap(m, "restriction"); r != nil {
		section(&b, "Restriction")
		fmt.Fprintf(&b, "  %s\n", summarizeRestriction(r))
	}

	// Partial expand failures (_errors map) — surface so agents know data is incomplete.
	if errs := gmap(m, "_errors"); len(errs) > 0 {
		section(&b, "Expand errors (partial)")
		for _, k := range sortedKeys(errs) {
			fmt.Fprintf(&b, "  %-16s %s\n", k, summarizeError(errs[k]))
		}
	}
	return b.String()
}

func section(b *strings.Builder, title string) {
	fmt.Fprintf(b, "\n-- %s --\n", title)
}

// descriptorOrder builds the canonical property ordering from the composite's
// top-level availablePropertyDescriptors: lower(name) -> first position. The
// backend returns propertyValues in RPV creation order (scrambled), so the table
// render sorts them against this index to match the descriptor order the UI shows.
// Duplicate names keep their FIRST position; an absent/empty availablePropertyDescriptors
// yields an empty map, which means no reordering.
func descriptorOrder(m map[string]any) map[string]int {
	descriptors := garr(m, "availablePropertyDescriptors")
	order := make(map[string]int, len(descriptors))
	for i, it := range descriptors {
		dm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		key := strings.ToLower(firstStr(dm, "name", "displayName"))
		if key == "" {
			continue
		}
		if _, exists := order[key]; !exists {
			order[key] = i
		}
	}
	return order
}

// propOrderIndex maps a propertyValue item to its canonical position in the
// descriptor order, or math.MaxInt (stably last) when it is not a map or its
// name is absent from the order. With an empty order every item returns
// math.MaxInt, so a stable sort preserves the original order.
func propOrderIndex(it any, order map[string]int) int {
	pm, ok := it.(map[string]any)
	if !ok {
		return math.MaxInt
	}
	key := strings.ToLower(firstStr(pm, "propertyDescriptorName", "descriptorName", "name", "label"))
	if idx, ok := order[key]; ok {
		return idx
	}
	return math.MaxInt
}

// renderPropertiesSection writes the "Properties (n)" section for m's
// propertyValues, one "name = value" line per property via propValue (the shared
// per-dataType formatter: PERCENTAGE ×100, decimals rounding, page-reference and
// user-link names). The rows are rendered in canonical descriptor order (via
// order), with unknown/not-in-descriptors properties stably last; an empty order
// leaves the original order intact. Sorting happens on a local copy so the
// underlying composite data is never mutated. It returns false when there are no
// property values so callers decide what to show for the empty case. Shared by
// `pages render` (renderPageASCII) and `pages get` (PageGet) so the formatting
// never diverges.
func renderPropertiesSection(b *strings.Builder, m map[string]any, order map[string]int) bool {
	props := garr(m, "propertyValues")
	if len(props) == 0 {
		return false
	}
	sorted := make([]any, len(props))
	copy(sorted, props)
	sort.SliceStable(sorted, func(i, j int) bool {
		return propOrderIndex(sorted[i], order) < propOrderIndex(sorted[j], order)
	})
	section(b, fmt.Sprintf("Properties (%d)", len(sorted)))
	for _, it := range sorted {
		pm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(b, "  %-24s %s\n", firstStr(pm, "propertyDescriptorName", "descriptorName", "name", "label"), propValue(pm))
	}
	return true
}

// renderList renders a titled section for a JSON array under key, one line per
// item via summarize. Skipped entirely when the key is absent or the array empty.
func renderList(b *strings.Builder, m map[string]any, key, title string, summarize func(map[string]any) string) {
	items := garr(m, key)
	if len(items) == 0 {
		return
	}
	section(b, fmt.Sprintf("%s (%d)", title, len(items)))
	for _, it := range items {
		im, ok := it.(map[string]any)
		if !ok {
			fmt.Fprintf(b, "  - %s\n", terminalLine(fmt.Sprintf("%v", it)))
			continue
		}
		line := summarize(im)
		if line == "" {
			line = "(item)"
		}
		fmt.Fprintf(b, "  - %s\n", line)
	}
}

func breadcrumbTrail(m map[string]any) string {
	items := garr(m, "breadcrumbs")
	var names []string
	for _, it := range items {
		if im, ok := it.(map[string]any); ok {
			if n := firstStr(im, "name", "title"); n != "" {
				names = append(names, n)
			}
		}
	}
	return strings.Join(names, " > ")
}

func pageTypeName(m map[string]any) string {
	if pt := gmap(m, "pageType"); pt != nil {
		if n := firstStr(pt, "name", "displayName"); n != "" {
			return n
		}
	}
	return gstr(m, "pageTypeName")
}

func propValue(pm map[string]any) string {
	// dataType-specific scalar value fields (PropertyValueResult).
	if v := firstStr(pm, "textValue", "enumValueDisplay", "selectedPageTypeName", "dateTimeValue", "dateValue"); v != "" {
		return v
	}
	if raw, ok := pm["numericValue"]; ok && raw != nil {
		if f, isFloat := raw.(float64); isFloat {
			decimals, hasDecimals := decimalsOf(pm)
			return formatNumeric(f, gstr(pm, "numberFormat"), decimals, hasDecimals)
		}
		if s, isStr := raw.(string); isStr && s != "" {
			return terminalLine(s)
		}
	}
	// reference-type values: arrays of {name/displayName}.
	if names := nameList(pm, "pageReferences", "userLinks"); names != "" {
		return names
	}
	// generic fallbacks (displayValue/value/values) for non-Normatik shapes/tests.
	if dv := firstStr(pm, "displayValue"); dv != "" {
		return dv
	}
	if v, ok := pm["value"]; ok && isScalar(v) {
		return cell(v)
	}
	if vals := garr(pm, "values"); len(vals) > 0 {
		parts := make([]string, 0, len(vals))
		for _, v := range vals {
			parts = append(parts, cell(v))
		}
		return strings.Join(parts, ", ")
	}
	return "—"
}

// decimalsOf extracts the optional "decimals" scale from a PropertyValueResult.
// A JSON-null or absent "decimals" means "no fixed scale" and returns ok=false.
func decimalsOf(pm map[string]any) (int, bool) {
	if v, ok := pm["decimals"]; ok && v != nil {
		if f, isFloat := v.(float64); isFloat {
			return int(f), true
		}
	}
	return 0, false
}

// formatNumeric renders a numeric PropertyValue for human-readable output, mirroring the
// frontend NumberValueBadge: a PERCENTAGE value is shown as value*100 with a "%" suffix,
// a NUMBER (or absent format) as-is.
//
// Rounding choice: when "decimals" is configured we round to that scale via
// strconv.FormatFloat, which rounds half-to-even (banker's rounding). This is a deliberate
// round (not a trim) so the CLI matches the backend, whose CalculatedEvaluatorService
// scales CALCULATED values with RoundingMode.HALF_EVEN. Consequence: 12.5 at decimals=0
// renders "12" (nearest even), not "13".
//
// Without a decimals config we trim trailing zeros instead of forcing a scale. The value
// is first round-tripped through a 15-significant-digit format to strip float noise (e.g.
// 0.9*100 = 90.00000000000001 -> "90") and always rendered in fixed notation so tiny
// fractions do not appear in scientific notation (e.g. 0.00001 -> "0.00001").
//
// Only this human render formats; JSON output stays raw.
func formatNumeric(value float64, numberFormat string, decimals int, hasDecimals bool) string {
	display := value
	suffix := ""
	if numberFormat == "PERCENTAGE" {
		display = value * 100
		suffix = "%"
	}
	if hasDecimals && decimals >= 0 {
		return strconv.FormatFloat(display, 'f', decimals, 64) + suffix
	}
	cleaned, _ := strconv.ParseFloat(strconv.FormatFloat(display, 'g', 15, 64), 64)
	return strconv.FormatFloat(cleaned, 'f', -1, 64) + suffix
}

// nameList joins the name/displayName of each object in the first non-empty of
// the given array keys (used for page-reference and user-link property values).
func nameList(pm map[string]any, keys ...string) string {
	for _, k := range keys {
		items := garr(pm, k)
		if len(items) == 0 {
			continue
		}
		var names []string
		for _, it := range items {
			if im, ok := it.(map[string]any); ok {
				if n := firstStr(im, "name", "displayName", "title"); n != "" {
					names = append(names, n)
				}
			}
		}
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	return ""
}

func summarizeRestriction(r map[string]any) string {
	var parts []string
	if t := firstStr(r, "type", "restrictionType"); t != "" {
		parts = append(parts, t)
	}
	if entries := garr(r, "accessEntries"); len(entries) > 0 {
		parts = append(parts, fmt.Sprintf("%d access entries", len(entries)))
	}
	if owner := firstStr(r, "ownerName", "owner"); owner != "" {
		parts = append(parts, "owner: "+owner)
	}
	if len(parts) == 0 {
		return "(restricted)"
	}
	return strings.Join(parts, "  ·  ")
}

func summarizeError(v any) string {
	if em, ok := v.(map[string]any); ok {
		return joinNonEmpty(": ", firstStr(em, "errorCode", "code"), firstStr(em, "detail", "message"))
	}
	return terminalLine(fmt.Sprintf("%v", v))
}

// --- nil-safe accessors -----------------------------------------------------

func gstr(m map[string]any, k string) string {
	return layoutLine(gtext(m, k))
}

func gtext(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return terminalText(s)
		}
	}
	return ""
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := gstr(m, k); s != "" {
			return s
		}
	}
	return ""
}

func gnumStr(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		switch t := v.(type) {
		case float64:
			return numStr(t)
		case string:
			return terminalLine(t)
		}
	}
	return ""
}

func garr(m map[string]any, k string) []any {
	if v, ok := m[k]; ok {
		if a, ok := v.([]any); ok {
			return a
		}
	}
	return nil
}

func gmap(m map[string]any, k string) map[string]any {
	if v, ok := m[k]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sizeStr(im map[string]any) string {
	if s := gnumStr(im, "fileSize"); s != "" {
		return "(" + s + " bytes)"
	}
	if s := gnumStr(im, "size"); s != "" {
		return "(" + s + " bytes)"
	}
	return ""
}

func bracket(s string) string {
	if s == "" {
		return ""
	}
	return "[" + s + "]"
}

func parenSuffix(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

func joinNonEmpty(sep string, parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
