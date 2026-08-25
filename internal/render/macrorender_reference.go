package render

// Reference-macro renderers for the rich page renderer: macros that point at
// OTHER pages or at document structure — children, toc (client-side heading
// scan), include-page / excerpt-include (one inline level with a depth cap),
// property-chain and page-selector (shared reference-table shape).

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// errorLine renders a macro resolution error (filterError) in place of the
// list/table — a red warning line, mirroring the web's red alert box.
func errorLine(msg string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#b91c1c")).Render("⚠ " + msg)
}

// renderChildren renders ::children from its matched ChildrenInstanceData
// (ChildrenInstanceData.java): content mode lists each child page as a title
// line + summary. This deviates from ChildrenRenderer.tsx, which inlines the
// full child body content — bounded terminal output over web parity;
// properties mode renders the bordered reference table. filterError replaces
// the list with an error line.
func renderChildren(n *dnode, idx *macroIndex, width int) string {
	entry, ok := idx.entryFor("childrenInstances", n)
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	if fe := gstr(entry, "filterError"); fe != "" {
		return errorLine(fe)
	}
	if strings.EqualFold(gstr(entry, "mode"), "PROPERTIES") {
		return renderReferenceTable(entry, "(no child pages found)", idx)
	}
	pages := garr(entry, "pages")
	if len(pages) == 0 {
		return "(no child pages found)"
	}
	var blocks []string
	for _, it := range pages {
		pm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		title := firstStr(pm, "title", "name")
		if title == "" {
			title = "(untitled)"
		}
		line := "• " + lipgloss.NewStyle().Bold(true).Render(title)
		if sum := gstr(pm, "summary"); sum != "" {
			line += "\n  " + sum
		}
		blocks = append(blocks, line)
	}
	return strings.Join(blocks, "\n")
}

// renderPropertyChain renders ::property-chain{links=..} from
// macroData.propertyChains (keyed by the raw links string).
func renderPropertyChain(n *dnode, idx *macroIndex, width int) string {
	return renderReferenceMacro(n, idx, width, "propertyChains")
}

// renderPageSelector renders ::page-selector from macroData.pageSelector
// (SINGULAR map key, per MacroData.java).
func renderPageSelector(n *dnode, idx *macroIndex, width int) string {
	return renderReferenceMacro(n, idx, width, "pageSelector")
}

// renderReferenceMacro is the shared property-chain / page-selector path:
// both payloads (PropertyChainMacroData / PageSelectorMacroData) carry
// pageReferences + displayColumns + filterError with identical semantics.
func renderReferenceMacro(n *dnode, idx *macroIndex, width int, mapKey string) string {
	entry, ok := idx.entryFor(mapKey, n)
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	if fe := gstr(entry, "filterError"); fe != "" {
		return errorLine(fe)
	}
	return renderReferenceTable(entry, "(no pages found)", idx)
}

// renderReferenceTable renders the shared reference-table shape: a header row
// with the Name column plus the configured displayColumns (sorted by
// sortOrder, like PageReferenceTable.tsx), and per pageReference a row with
// the page name. Display cells come from idx.values, matched on
// propertyDescriptorName == column.name and formatted with propValue. A nil
// lookup, missing page, missing property or empty value stays an em-dash.
func renderReferenceTable(entry map[string]any, emptyNote string, idx *macroIndex) string {
	refs := garr(entry, "pageReferences")
	if len(refs) == 0 {
		return emptyNote
	}
	cols := make([]map[string]any, 0)
	for _, it := range garr(entry, "displayColumns") {
		if cm, ok := it.(map[string]any); ok {
			cols = append(cols, cm)
		}
	}
	sort.SliceStable(cols, func(i, j int) bool {
		return gnum(cols[i], "sortOrder") < gnum(cols[j], "sortOrder")
	})
	headers := []string{"Name"}
	for _, cm := range cols {
		headers = append(headers, gstr(cm, "name"))
	}
	var rows [][]string
	for _, it := range refs {
		rm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(headers))
		row[0] = firstStr(rm, "name", "title")
		pageID := pageRefID(rm)
		for i := 1; i < len(row); i++ {
			row[i] = referenceColumnValue(idx, pageID, gstr(cols[i-1], "name"))
		}
		rows = append(rows, row)
	}
	return asciiTable(headers, rows, nil)
}

func referenceColumnValue(idx *macroIndex, pageID int64, columnName string) string {
	pv := lookupPropertyValue(idx, pageID, columnName)
	if pv == nil {
		return "—"
	}
	return propValue(pv)
}

// lookupPropertyValue returns the first property row on pageID whose
// propertyDescriptorName equals columnName (SPA PageReferenceTable parity).
// Duplicate names on one page are not a real response-contract case
// (uk_property_descriptor_name is unique per page type, and a page has one
// type); first-match is recorded, not a second disambiguation key.
func lookupPropertyValue(idx *macroIndex, pageID int64, columnName string) map[string]any {
	if idx == nil || idx.values == nil || pageID == 0 || columnName == "" {
		return nil
	}
	for _, pv := range idx.values[pageID] {
		if gstr(pv, "propertyDescriptorName") == columnName {
			return pv
		}
	}
	return nil
}

// --- toc (client-side heading scan) ------------------------------------------

type tocHeading struct {
	level int
	text  string
}

// renderToc renders ::toc by scanning the OWN content's headings client-side —
// the backend resolver is a no-op (TocMacroDataResolver: frontend DOM scan).
// Attr defaults per @AttrDoc: title "Table of Contents", minLevel 1, maxLevel 6.
// Headings inside ```-fences and :::code bodies do not count.
func renderToc(n *dnode, idx *macroIndex, _ int) string {
	title := n.attrs["title"]
	if title == "" {
		title = "Table of Contents"
	}
	minLevel := intAttrDefault(n.attrs["minLevel"], 1)
	maxLevel := intAttrDefault(n.attrs["maxLevel"], 6)
	var headings []tocHeading
	if idx.root != nil {
		collectHeadings(idx.root, &headings)
	}
	var lines []string
	for _, h := range headings {
		if h.level < minLevel || h.level > maxLevel {
			continue
		}
		indent := strings.Repeat("  ", h.level-minLevel)
		lines = append(lines, indent+"• "+renderInline(h.text, idx))
	}
	if len(lines) == 0 {
		return ruleHeader(title) + "\n(no headings found)"
	}
	return ruleHeader(title) + "\n" + strings.Join(lines, "\n")
}

// collectHeadings walks the directive tree collecting markdown headings from
// #text runs. :::code bodies are skipped entirely and ```-fenced lines within
// a text run are skipped via the same fence toggle renderTextBlock uses — a
// heading inside a fence is documentation, not document structure.
func collectHeadings(n *dnode, out *[]tocHeading) {
	if n.name == "code" {
		return
	}
	if n.name == "#text" {
		inFence := false
		for _, line := range n.lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			if m := headingRe.FindStringSubmatch(trimmed); m != nil {
				*out = append(*out, tocHeading{level: len(m[1]), text: m[2]})
			}
		}
		return
	}
	for _, k := range n.kids {
		collectHeadings(k, out)
	}
}

// intAttrDefault parses an int attr, falling back to def when absent/invalid.
func intAttrDefault(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// --- include-page / excerpt-include ------------------------------------------

// maxInlineIncludeDepth caps inline include rendering: the top page (depth 0)
// renders its includes inline, but an include nested INSIDE included content
// renders as a "— include: name —" placeholder. The backend delivers nested
// macroData up to 3 levels; one inline level keeps terminal output bounded.
const maxInlineIncludeDepth = 1

// renderIncludePage renders ::include-page{id=..} from macroData.includePages
// (IncludePageData: id, name, content, nested macroData).
func renderIncludePage(n *dnode, idx *macroIndex, width int) string {
	return renderIncludeLike(n, idx, width, "includePages", "Include", "(no content)")
}

// renderExcerptInclude renders ::excerpt-include from macroData.excerptIncludes
// (ExcerptIncludeData; key "self" for the id-less form — see macroDataKey).
func renderExcerptInclude(n *dnode, idx *macroIndex, width int) string {
	return renderIncludeLike(n, idx, width, "excerptIncludes", "Excerpt", "(no excerpt)")
}

// renderIncludeLike renders one level of inline include: a header rule with
// the included page's name, then its content rendered against a nested
// macroIndex over the macroData the backend delivered WITH the entry. Beyond
// the depth cap the include renders as a placeholder — never recursed.
func renderIncludeLike(n *dnode, idx *macroIndex, width int, mapKey, label, emptyNote string) string {
	entry, ok := idx.entryFor(mapKey, n)
	if idx.depth >= maxInlineIncludeDepth {
		return includePlaceholder(n, entry)
	}
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	head := ruleHeader(joinNonEmpty(": ", label, gstr(entry, "name")))
	content := gtext(entry, "content")
	if strings.TrimSpace(content) == "" {
		return head + "\n" + emptyNote
	}
	root := parseDirectives(content)
	nested := &macroIndex{
		raw:    gmap(entry, "macroData"),
		root:   root,
		depth:  idx.depth + 1,
		values: idx.values,
	}
	return head + "\n" + renderNodes(root.kids, nested, width)
}

// includePlaceholder is the depth-capped form of a nested include: the name
// comes from the (nested) macroData entry when the backend resolved it, else
// the directive's id attr.
func includePlaceholder(n *dnode, entry map[string]any) string {
	name := ""
	if entry != nil {
		name = gstr(entry, "name")
	}
	if name == "" {
		if id := n.attrs["id"]; id != "" {
			name = "page " + id
		} else {
			name = n.name
		}
	}
	return "— include: " + name + " —"
}
