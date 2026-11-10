package render

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// MoveToSpaceImpact renders MoveToSpaceImpactResult (preview and move). JSON
// mode dumps the body. Table mode shows allowed and pageCount, then the
// blockers and warnings: the generic Raw printer hides those lists, so a
// refused move did not say why (NORM-vnvimssy).
func (p *Printer) MoveToSpaceImpact(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	allowed := "no"
	if b, ok := m["allowed"].(bool); ok && b {
		allowed = "yes"
	}
	fmt.Fprintf(tw, "allowed\t%s\n", allowed)
	fmt.Fprintf(tw, "pageCount\t%s\n", gnumStr(m, "pageCount"))
	_ = tw.Flush()
	if blockers := garr(m, "blockers"); len(blockers) > 0 {
		fmt.Fprintln(p.Out)
		writeMoveToSpaceBlockers(p.Out, "", blockers)
	}
	if warnings := garr(m, "warnings"); len(warnings) > 0 {
		fmt.Fprintln(p.Out)
		fmt.Fprintln(p.Out, "warnings")
		for _, w := range warnings {
			if s, ok := w.(string); ok {
				fmt.Fprintf(p.Out, "  %s\n", terminalLine(s))
			}
		}
	}
}

// writeMoveToSpaceBlockers writes one row per blocker: the reason, the number
// of pages, and the page and type it names when the server does not mask them.
func writeMoveToSpaceBlockers(w io.Writer, indent string, blockers []any) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%sREASON\tPAGES\tPAGE\tTYPE\n", indent)
	for _, it := range blockers {
		b, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ := firstStr(b, "typeName")
		if space := firstStr(b, "typeSpaceName"); typ != "" && space != "" {
			typ += " (" + space + ")"
		}
		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\n", indent,
			terminalLine(firstStr(b, "reason")), gnumStr(b, "count"),
			dash(terminalLine(firstStr(b, "pageName"))), dash(terminalLine(typ)))
	}
	_ = tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
