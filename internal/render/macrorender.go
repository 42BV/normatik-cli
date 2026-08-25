package render

// Rich page renderer (`pages render`, default mode): renders page content
// with macros resolved from the composite's macroData (enum pills, tables,
// pagelinks) instead of the `--plain` [macro: name] placeholders. This file
// holds the entry point (PageRich), the renderer registry and the structural
// renderers (table, code, expand, tabs/tab, excerpt); the parser lives in
// directiveparse.go, the macroData correlation in macroindex.go, reference
// macros in macrorender_reference.go, data macros in macrorender_data.go and
// inline resolution in inline.go.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// asciiTable renders a bordered ASCII table (rounded outer border + row/column
// separators) via lipgloss/table. headers may be empty (key/value grids). colW
// optionally sets per-column target widths.
func asciiTable(headers []string, rows [][]string, colW []int) string {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderRow(true).
		BorderColumn(true).
		Wrap(true)
	if len(headers) > 0 {
		up := make([]string, len(headers))
		for i, h := range headers {
			// Header cells may already carry lipgloss styling (renderInline
			// resolves **bold** etc. before the cell reaches this table).
			// ToUpper over those escape bytes would turn \x1b[1m into \x1b[1M
			// — and CSI ...M is the DL (delete line) control, which wipes the
			// header row on a real terminal. Strip the styling first: the
			// header row is re-styled bold by the StyleFunc below anyway.
			up[i] = strings.ToUpper(terminalLine(ansi.Strip(h)))
		}
		t = t.Headers(up...)
	}
	cleanRows := make([][]string, len(rows))
	for i, row := range rows {
		cleanRows[i] = make([]string, len(row))
		for j, cell := range row {
			cleanRows[i][j] = layoutLine(cell)
		}
	}
	t = t.Rows(cleanRows...)
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		st := lipgloss.NewStyle().Padding(0, 1)
		if row == table.HeaderRow {
			st = st.Bold(true)
		}
		if col < len(colW) {
			st = st.Width(colW[col])
		}
		return st
	})
	return t.String()
}

// PageRich renders a page composite with macros resolved to ASCII layout.
// The rich output goes through a colorprofile.Writer at this single choke
// point: lipgloss v2 emits ANSI styling unconditionally, so the writer
// downsamples/strips it at the edge — a non-TTY destination (pipe, redirect,
// test buffer) gets plain text, NO_COLOR/CLICOLOR/CLICOLOR_FORCE are
// honoured, and a real terminal keeps its detected color profile. JSON mode
// stays the raw composite, untouched.
func (p *Printer) PageRich(body []byte) {
	p.pageRich(body, nil)
}

// PageRichWithValues is the same human-rich path as PageRich, with an optional
// per-page property-values lookup for reference-table cells. JSON mode still
// dumps the original composite and ignores the lookup.
func (p *Printer) PageRichWithValues(body []byte, values map[int64][]map[string]any) {
	p.pageRich(body, values)
}

func (p *Printer) pageRich(body []byte, values map[int64][]map[string]any) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	fmt.Fprint(colorprofile.NewWriter(p.Out, os.Environ()), renderPageRich(m, values))
}

func renderPageRich(m map[string]any, values map[int64][]map[string]any) string {
	var b strings.Builder
	idx := newMacroIndex(m)
	idx.values = values

	// Title box + meta + breadcrumb + properties (reuse the existing helpers).
	name := gstr(m, "name")
	if name == "" {
		name = "(unnamed)"
	}
	bar := strings.Repeat("=", minInt(displayWidth(name)+2, 80))
	fmt.Fprintf(&b, "+%s+\n  %s\n+%s+\n", bar, name, bar)
	var meta []string
	if id := gnumStr(m, "id"); id != "" {
		meta = append(meta, "ID "+id)
	}
	if pn := gstr(m, "parentName"); pn != "" {
		meta = append(meta, "Parent: "+pn+parenSuffix(gnumStr(m, "parentId")))
	}
	if len(meta) > 0 {
		fmt.Fprintln(&b, strings.Join(meta, "  -  "))
	}
	if crumbs := breadcrumbTrail(m); crumbs != "" {
		fmt.Fprintf(&b, "Breadcrumb: %s\n", crumbs)
	}
	var detail [][]string
	if pt := pageTypeName(m); pt != "" {
		detail = append(detail, []string{"Page Type", pt}) // page metadata, like the web
	}
	for _, it := range garr(m, "propertyValues") {
		if pm, ok := it.(map[string]any); ok {
			detail = append(detail, []string{firstStr(pm, "propertyDescriptorName", "descriptorName", "name"), propValue(pm)})
		}
	}
	if len(detail) > 0 {
		section(&b, "Details")
		b.WriteString(asciiTable(nil, detail, []int{24, 50}) + "\n")
	}

	// Content — the rich part.
	section(&b, "Content")
	root := parseDirectives(gtext(m, "content"))
	idx.root = root // ::toc scans this tree for the page's own headings
	b.WriteString(renderNodes(root.kids, idx, 78))
	b.WriteString("\n")
	return b.String()
}

// --- rendering --------------------------------------------------------------

// macroRenderer renders a single directive node (leaf or container) to a block.
type macroRenderer func(n *dnode, idx *macroIndex, width int) string

// macroRenderers dispatches directive names to their renderer. Names without an
// entry fall back to renderUnknownMacro (labelled generic block). Populated in
// init() because the renderers recurse via renderNodes back into this map,
// which the compiler rejects as an initialization cycle on a composite literal.
//
// Completeness is guarded by TestMacroCompleteness (macrocompleteness_test.go)
// against the generated contentmacros.json: every BASIC directive macro
// allowed in PAGE_CONTENT must have an entry here, be resolved inline by
// renderInline (pagelink/enum/color) or be a structural container child
// (tab under :::tabs; header/row/cell under :::table are not ContentMacros).
var macroRenderers map[string]macroRenderer

func init() {
	macroRenderers = map[string]macroRenderer{
		"table":           renderTable,
		"code":            func(n *dnode, _ *macroIndex, _ int) string { return renderCode(n) },
		"expand":          renderExpand,
		"excerpt":         renderExcerpt,
		"tabs":            renderTabs,
		"tab":             renderTab,
		"children":        renderChildren,
		"toc":             renderToc,
		"include-page":    renderIncludePage,
		"excerpt-include": renderExcerptInclude,
		"property-chain":  renderPropertyChain,
		"page-selector":   renderPageSelector,
		"progress-ring":   renderProgressRing,
		"page-tasks":      renderPageTasks,
		"attachments":     renderAttachments,
		"file":            renderFile,
		"pdf":             renderPdf,
		"image":           renderImage,
		"jira-assets":     renderJiraAssets,
		"jira-issues":     renderJiraIssues,
	}
}

func renderNodes(nodes []*dnode, idx *macroIndex, width int) string {
	var out []string
	for _, n := range nodes {
		if n.name == "#text" {
			out = append(out, renderTextBlock(strings.Join(n.lines, "\n"), idx))
			continue
		}
		renderer, ok := macroRenderers[n.name]
		if !ok {
			renderer = renderUnknownMacro
		}
		out = append(out, renderer(n, idx, width))
	}
	return strings.Join(out, "\n")
}

// renderUnknownMacro is the generic fallback for directives without a
// registered renderer: a labelled block, with any inner content below it.
func renderUnknownMacro(n *dnode, idx *macroIndex, width int) string {
	label := "[macro: " + n.name + "]"
	inner := renderNodes(n.kids, idx, width)
	if strings.TrimSpace(inner) == "" {
		return label
	}
	return label + "\n" + inner
}

// renderTextBlock renders plain markdown (headings, lists, paragraphs) with
// inline macros resolved. ```-fenced code is emitted verbatim (indented, like
// markdownToASCII and renderCode) — no inline resolution inside a fence.
func renderTextBlock(s string, idx *macroIndex) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var out []string
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, "    ----")
			continue
		}
		if inFence {
			out = append(out, "    "+line)
			continue
		}
		if m := headingRe.FindStringSubmatch(trimmed); m != nil {
			text := renderInline(m[2], idx)
			switch len(m[1]) {
			case 1:
				out = append(out, text, strings.Repeat("=", displayWidth(text)))
			case 2:
				out = append(out, text, strings.Repeat("-", displayWidth(text)))
			default:
				out = append(out, strings.Repeat("#", len(m[1]))+" "+text)
			}
			continue
		}
		if m := listItemRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1]+"- "+renderInline(m[2], idx))
			continue
		}
		out = append(out, renderInline(line, idx))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// renderCode renders :::code{lang title lineNumbers}: an optional header rule
// with "lang · title" above the verbatim body (attrs per CodeMacroDataResolver
// @MacroDoc: lang, title, lineNumbers + legacy showLineNumbers — bare flags
// parse to "true"). The body stays verbatim; lineNumbers adds a gutter.
func renderCode(n *dnode) string {
	var lines []string
	for _, k := range n.kids {
		if k.name == "#text" {
			lines = append(lines, k.lines...)
		}
	}
	numbered := n.attrs["lineNumbers"] == "true" || n.attrs["showLineNumbers"] == "true"
	var b strings.Builder
	if head := joinNonEmpty(" · ", n.attrs["lang"], n.attrs["title"]); head != "" {
		b.WriteString(ruleHeader(head) + "\n")
	}
	b.WriteString("    ----\n")
	for i, l := range lines {
		if numbered {
			fmt.Fprintf(&b, "    %3d │ %s\n", i+1, l)
		} else {
			b.WriteString("    " + l + "\n")
		}
	}
	b.WriteString("    ----")
	return b.String()
}

// ruleHeader renders a "── label ────" heading rule in the box-drawing house
// style, with the label bold.
func ruleHeader(label string) string {
	fill := 46 - displayWidth(label) - 4
	if fill < 2 {
		fill = 2
	}
	return "── " + lipgloss.NewStyle().Bold(true).Render(label) + " " + strings.Repeat("─", fill)
}

// renderExpand renders :::expand{title} as a disclosure block: a "▸ Title"
// header with the body always rendered below it, lightly indented — a terminal
// has no click-to-open. Title default mirrors ExpandRenderer.tsx ('Expand').
func renderExpand(n *dnode, idx *macroIndex, width int) string {
	title := n.attrs["title"]
	if title == "" {
		title = "Expand"
	}
	head := lipgloss.NewStyle().Bold(true).Render("▸ " + title)
	body := indentLines(renderNodes(n.kids, idx, width-2), "  ")
	if strings.TrimSpace(body) == "" {
		return head
	}
	return head + "\n" + body
}

// renderExcerpt renders :::excerpt{name}: the excerpt marker box from the web
// (ExcerptRenderer.tsx: a labelled quote panel with the body inline) becomes a
// rule header — "Excerpt" plus the optional name attr — with the body rendered
// below it. The body is regular content, so macros inside it resolve normally.
func renderExcerpt(n *dnode, idx *macroIndex, width int) string {
	head := ruleHeader(joinNonEmpty(": ", "Excerpt", n.attrs["name"]))
	body := renderNodes(n.kids, idx, width)
	if strings.TrimSpace(body) == "" {
		return head
	}
	return head + "\n" + body
}

// renderTabs renders a :::tabs container sequentially: every :::tab kid becomes
// a labelled section — a terminal has no tab strip, so all tabs are visible.
// Non-tab kids are skipped (TabsRenderer.tsx also only consumes tab children);
// zero tabs render nothing (TabsRenderer returns null).
func renderTabs(n *dnode, idx *macroIndex, width int) string {
	var blocks []string
	for _, k := range n.kids {
		if k.name == "tab" {
			blocks = append(blocks, renderTab(k, idx, width))
		}
	}
	return strings.Join(blocks, "\n\n")
}

// renderTab renders one :::tab{title}: rule header + body. Title default
// mirrors TabsRenderer.tsx ('Tab').
func renderTab(n *dnode, idx *macroIndex, width int) string {
	title := n.attrs["title"]
	if title == "" {
		title = "Tab"
	}
	head := ruleHeader(title)
	body := renderNodes(n.kids, idx, width)
	if strings.TrimSpace(body) == "" {
		return head
	}
	return head + "\n" + body
}

// indentLines prefixes every non-empty line of s with prefix.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// renderTable renders a :::table (header/row > cell) as a minimalist ASCII table
// — no cell borders, just a header separator, matching the web look. Column
// widths come from the `widths` attr (ratios), scaled to `width`.
func renderTable(n *dnode, idx *macroIndex, width int) string {
	var header []string
	var rows [][]string
	for _, k := range n.kids {
		switch k.name {
		case "header":
			header = cellsOf(k, idx)
		case "row":
			rows = append(rows, cellsOf(k, idx))
		}
	}
	cols := len(header)
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return "(empty table)"
	}
	colW := columnWidths(n.attrs["widths"], cols, width-4*cols)
	return asciiTable(header, rows, colW)
}

func cellsOf(rowNode *dnode, idx *macroIndex) []string {
	var cells []string
	for _, c := range rowNode.kids {
		if c.name == "cell" {
			var txt []string
			for _, t := range c.kids {
				if t.name == "#text" {
					txt = append(txt, strings.TrimSpace(strings.Join(t.lines, " ")))
				}
			}
			cells = append(cells, renderInline(strings.TrimSpace(strings.Join(txt, " ")), idx))
		}
	}
	return cells
}

func columnWidths(spec string, cols, total int) []int {
	w := make([]int, cols)
	gaps := 2 * (cols - 1)
	usable := total - gaps
	if usable < cols {
		usable = cols
	}
	ratios := make([]float64, cols)
	sumR := 0.0
	if spec != "" {
		for i, part := range strings.Split(spec, ",") {
			if i >= cols {
				break
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err == nil {
				ratios[i] = v
				sumR += v
			}
		}
	}
	if sumR == 0 {
		for i := range ratios {
			ratios[i] = 1
		}
		sumR = float64(cols)
	}
	for i := range w {
		w[i] = int(float64(usable) * ratios[i] / sumR)
		if w[i] < 6 {
			w[i] = 6
		}
	}
	return w
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}
