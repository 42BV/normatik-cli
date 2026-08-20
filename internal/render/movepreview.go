package render

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
)

// PageTypeMoveImpact renders PageTypeMoveImpactResult. JSON mode dumps the
// full body. Table mode shows scalars plus at-risk descriptors with default
// ADOPT — the generic Raw printer would hide those lists as "…".
func (p *Printer) PageTypeMoveImpact(body []byte) {
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
	for _, k := range []string{
		"affectedPagesCount",
		"workflowDirection",
		"workflowFlippedPages",
		"revisionsToBeDeleted",
		"snapshotKey",
	} {
		if v := impactScalar(m, k); v != "" {
			fmt.Fprintf(tw, "%s\t%s\n", k, v)
		}
	}
	_ = tw.Flush()

	fmt.Fprintln(p.Out)
	fmt.Fprintln(p.Out, "at-risk descriptors (default ADOPT)")
	items := garr(m, "atRiskDescriptors")
	if len(items) == 0 {
		fmt.Fprintln(p.Out, "—")
		return
	}
	at := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(at, "NAME\tTYPE\tSOURCE\tVALUES\tDECISION")
	for _, it := range items {
		im, ok := it.(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(at, "%s\t%s\t%s\t%s\tADOPT\n",
			firstStr(im, "descriptorName", "name"),
			firstStr(im, "type"),
			firstStr(im, "source"),
			atRiskValues(im),
		)
	}
	_ = at.Flush()
}

// atRiskValues picks the counter that describes what the move really touches
// for this at-risk descriptor (NORM-hrodzcmr): PAGE_OUTGOING (page_link) and
// USER_LIST (revision_user_link) report linksAffected; every other datatype lives in revision_property_value
// (revisionValuesAffected). valuesAffected (page-level property_value) is a legacy
// counter that is 0 for almost everything and is deliberately not shown.
func atRiskValues(im map[string]any) string {
	switch firstStr(im, "type") {
	case "PAGE_OUTGOING", "USER_LIST":
		return firstNumStr(im, "linksAffected", "valuesAffected")
	default:
		return firstNumStr(im, "revisionValuesAffected", "valuesAffected")
	}
}

// firstNumStr returns the first key that is present as a number; "0" if none.
func firstNumStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return gnumStr(m, k)
		}
	}
	return "0"
}

func impactScalar(m map[string]any, k string) string {
	if s := gstr(m, k); s != "" {
		return s
	}
	return gnumStr(m, k)
}
