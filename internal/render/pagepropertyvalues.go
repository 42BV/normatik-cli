package render

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
)

type pagePropertyValuesRow struct {
	PageId         int64            `json:"pageId"`
	PageName       string           `json:"pageName"`
	PropertyValues []map[string]any `json:"propertyValues"`
}

// PagePropertyValues renders GET /public/v1/pages/property-values.
// JSON mode re-serializes the body via rawDump. Table mode prints one row per
// page × property (PAGE ID, PAGE, PROPERTY, VALUE). A page without properties
// gets one row with PROPERTY "—" and an empty VALUE. --quiet drops the header.
func (p *Printer) PagePropertyValues(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var rows []pagePropertyValuesRow
	if err := json.Unmarshal(body, &rows); err != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	if !p.Quiet {
		fmt.Fprintln(tw, "PAGE ID\tPAGE\tPROPERTY\tVALUE")
	}
	for _, row := range rows {
		name := terminalLine(row.PageName)
		if len(row.PropertyValues) == 0 {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", row.PageId, name, "—", "")
			continue
		}
		for _, pv := range row.PropertyValues {
			prop := firstStr(pv, "propertyDescriptorName")
			if prop == "" {
				prop = "—"
			}
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", row.PageId, name, terminalLine(prop), propValue(pv))
		}
	}
	_ = tw.Flush()
}
