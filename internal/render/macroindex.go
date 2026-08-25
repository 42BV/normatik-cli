package render

// macroData index for the rich page renderer: correlates a directive
// occurrence in the content with its entry in the composite's macroData maps.
// The key-builders in this file mirror the server's per-macro map keys exactly.

import (
	"sort"
	"strconv"
	"strings"
)

// macroIndex gives renderers access to the composite's macroData maps. Every
// map is reachable via dataMap/entryFor; renderers interpret the entries
// (map[string]any) themselves. root and depth carry per-content context:
// root is the parsed directive tree of the content this index belongs to
// (::toc scans it for headings) and depth is the include-nesting level
// (0 = top page; include-page/excerpt-include cap on it).
type macroIndex struct {
	raw         map[string]any // macroData of the composite (nil when absent)
	jiraRaw     map[string]any // guarded top-level jiraMacros (nil when absent)
	jiraEnabled bool           // true only when jiraMacros is present as an object
	root        *dnode         // parsed tree of the content this macroData belongs to
	depth       int            // include nesting depth (0 = top page)
	// values is an optional per-page property-values lookup (pageId → rows).
	// Nil means no injection: reference-table display cells stay em-dash.
	values map[int64][]map[string]any
}

func newMacroIndex(m map[string]any) *macroIndex {
	jira, jiraEnabled := m["jiraMacros"].(map[string]any)
	return &macroIndex{
		raw:         gmap(m, "macroData"),
		jiraRaw:     jira,
		jiraEnabled: jiraEnabled,
	}
}

// dataMap returns macroData[mapKey] as a map (nil when absent or not a map).
func (idx *macroIndex) dataMap(mapKey string) map[string]any {
	if idx.raw == nil {
		return nil
	}
	mm, _ := idx.raw[mapKey].(map[string]any)
	return mm
}

// jiraDataMap returns guarded jiraMacros[mapKey]. It deliberately never reads
// macroData: presence of the top-level jiraMacros object is the server-owned
// integration gate for every Jira renderer.
func (idx *macroIndex) jiraDataMap(mapKey string) map[string]any {
	if !idx.jiraEnabled || idx.jiraRaw == nil {
		return nil
	}
	mm, _ := idx.jiraRaw[mapKey].(map[string]any)
	return mm
}

// jiraEntryFor performs an exact lookup in the guarded Jira payload. Jira
// composite keys encode the complete directive configuration, so a miss must
// never fall back to an unrelated single entry.
func (idx *macroIndex) jiraEntryFor(mapKey, key string) (map[string]any, bool) {
	if key == "" {
		return nil, false
	}
	e, ok := idx.jiraDataMap(mapKey)[key].(map[string]any)
	return e, ok
}

// entryFor correlates a directive occurrence with its entry in
// macroData[mapKey]. First an exact lookup on the backend-mirrored key
// (macroDataKey); when that misses, the map holds exactly one entry AND the
// directive is composite-keyed, that entry is used (robust for
// single-instance pages when our key rebuild drifts). Id/value-keyed maps
// never fall back — a miss there means the backend omitted the entry
// (unresolved value, denied page) and the renderer must use its neutral
// fallback. Any other miss yields (nil, false).
func (idx *macroIndex) entryFor(mapKey string, n *dnode) (map[string]any, bool) {
	mm := idx.dataMap(mapKey)
	if len(mm) == 0 {
		return nil, false
	}
	if key := macroDataKey(n); key != "" {
		if e, ok := mm[key].(map[string]any); ok {
			return e, true
		}
	}
	if len(mm) == 1 && compositeKeyed(n.name) {
		for _, v := range mm {
			if e, ok := v.(map[string]any); ok {
				return e, true
			}
		}
	}
	return nil, false
}

// compositeKeyed reports whether a directive's macroData map is keyed by a
// CLI-rebuilt composite config key (childrenInstances, progressRings,
// propertyChains, pageSelector). Only there is the single-entry fallback
// safe: the backend emits one entry per directive config, so a lone entry on
// a single-instance page is that occurrence even when our key rebuild drifts.
// Id/value-keyed maps (pagelinks, enums, images, include-page,
// excerpt-include) omit unresolved or access-denied entries — a lone entry
// may belong to a different occurrence, so a key miss must stay a miss.
func compositeKeyed(name string) bool {
	switch name {
	case "children", "progress-ring", "property-chain", "page-selector":
		return true
	}
	return false
}

// macroDataKey rebuilds the backend's macroData map key for a directive node.
// Id-keyed macros use the numeric id attr directly; composite-keyed macros
// mirror the backend key-builders exactly (see the per-builder docs below).
// Returns "" when no key can be derived (entryFor then only has the
// single-entry fallback).
func macroDataKey(n *dnode) string {
	a := n.attrs
	switch n.name {
	case "pagelink", "image", "include-page":
		if id := a["id"]; id != "" {
			return id
		}
		return a["pageId"]
	case "file", "pdf":
		// FileMacroDataResolver keys fileAttachments by the numeric id attr
		// (both directives share the same map; PdfMacroDataResolver is a NoOp).
		return a["id"]
	case "page-tasks":
		// PageTasksMacroDataResolver keys pageTasks by the trimmed type slug
		// (DirectiveExtractor#parsePageTasksDirective: typeSlug.trim()).
		return strings.TrimSpace(a["type"])
	case "excerpt-include":
		// ExcerptIncludeMacroDataResolver keys by the page id, or "self" for
		// the id-less form (::excerpt-include → the current page's own excerpt).
		if id := a["id"]; id != "" {
			return id
		}
		if id := a["pageId"]; id != "" {
			return id
		}
		return "self"
	case "enum":
		// EnumMacroDataResolver: key = enumName + "::" + valueName.
		return a["enum"] + "::" + a["value"]
	case "children":
		return childrenKey(a)
	case "progress-ring":
		return progressRingKey(a)
	case "property-chain":
		// PropertyChainMacroReference: key = the raw links attribute string
		// (DirectiveExtractor#parsePropertyChainDirective passes linksStr as key).
		return a["links"]
	case "page-selector":
		return pageSelectorKey(a)
	}
	return ""
}

// childrenKey mirrors ChildrenMacroReference.buildKey:
// "mode=…|depth=…|leafOnly=…|columns=…|nameWidth=…|sort=…|filters=…".
// Attr resolution mirrors DirectiveExtractor#parseChildrenDirective: mode is
// "properties" only on exact match, depth via parseDepth ("all" → 10, invalid
// → 1), leafOnly only on exact "true", columns/filters raw, nameWidth via
// getInt (unparseable → empty).
func childrenKey(a map[string]string) string {
	mode := "content"
	if a["mode"] == "properties" {
		mode = "properties"
	}
	return "mode=" + mode +
		"|depth=" + strconv.Itoa(childrenDepth(a["depth"])) +
		"|leafOnly=" + strconv.FormatBool(a["leafOnly"] == "true") +
		"|columns=" + a["columns"] +
		"|nameWidth=" + intAttr(a["nameWidth"]) +
		"|sort=" + childrenSortString(a["sort"]) +
		"|filters=" + a["filters"]
}

// childrenDepth mirrors ChildrenMacroReference.parseDepth: blank → 1,
// "all" → MAX_CHILDREN_RENDER_DEPTH (10), numeric < 1 → 1, unparseable → 1.
func childrenDepth(s string) int {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 1
	}
	if strings.EqualFold(trimmed, "all") {
		return 10
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// childrenSortString mirrors ChildrenMacroReference.buildSortString fed by
// parseSortDescriptorName/parseSortDirection: bare "asc"/"desc" → lowercased
// direction; "Descriptor:asc" (split on the LAST colon) → "Descriptor:asc";
// anything else → "".
func childrenSortString(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	if strings.EqualFold(trimmed, "asc") || strings.EqualFold(trimmed, "desc") {
		return strings.ToLower(trimmed)
	}
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon < 1 || lastColon >= len(trimmed)-1 {
		return ""
	}
	dir := strings.ToLower(strings.TrimSpace(trimmed[lastColon+1:]))
	if dir != "asc" && dir != "desc" {
		return ""
	}
	return strings.TrimSpace(trimmed[:lastColon]) + ":" + dir
}

// progressRingKey mirrors ProgressRingMacroReference#buildKey:
// "{pageType}|{property}|{success values, trimmed, sorted, comma-joined}".
// pageType/property are trimmed (DirectiveExtractor#parseProgressRingDirective);
// both are required — without them the backend emits no reference, so no key.
func progressRingKey(a map[string]string) string {
	pageType := strings.TrimSpace(a["pageType"])
	property := strings.TrimSpace(a["property"])
	if pageType == "" || property == "" {
		return ""
	}
	var values []string
	for _, part := range strings.Split(a["success"], "|") {
		if t := strings.TrimSpace(part); t != "" {
			values = append(values, t)
		}
	}
	sort.Strings(values)
	return pageType + "|" + property + "|" + strings.Join(values, ",")
}

// pageSelectorKey mirrors PageSelectorMacroReference.buildKey:
// "filters|columns|nameWidth|sort|limit". Sort resolution mirrors
// DirectiveExtractor#parsePageSelectorDirective (case-insensitive desc/
// original, anything else → asc); limit is clamped to [1, MAX_LIMIT=1000],
// absent/unparseable → empty.
func pageSelectorKey(a map[string]string) string {
	sortDir := "asc"
	switch strings.ToLower(a["sort"]) {
	case "desc":
		sortDir = "desc"
	case "original":
		sortDir = "original"
	}
	limit := ""
	if n, err := strconv.Atoi(a["limit"]); err == nil {
		limit = strconv.Itoa(minInt(maxInt(n, 1), 1000))
	}
	return a["filters"] + "|" + a["columns"] + "|" + intAttr(a["nameWidth"]) + "|" + sortDir + "|" + limit
}

// intAttr mirrors DirectiveAttributes.getInt (Integer.parseInt, no trim):
// parseable → canonical decimal form, otherwise empty — the backend then
// renders "" into the composite key.
func intAttr(v string) string {
	n, err := strconv.Atoi(v)
	if err != nil {
		return ""
	}
	return strconv.Itoa(n)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// gnum reads a JSON number field as float64 (0 when absent or not a number).
func gnum(m map[string]any, k string) float64 {
	f, _ := m[k].(float64)
	return f
}

// gbool reads a JSON bool field (false when absent or not a bool).
func gbool(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}
