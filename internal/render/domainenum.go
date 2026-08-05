package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
)

// DomainEnum renders a DomainEnumResult in table mode: id + name plus the allowed
// enum values, which the generic Raw printer would drop as a nested array (so the
// command looked broken — only `-o json` showed them). JSON mode dumps the body
// verbatim so the machine-readable contract is unchanged.
func (p *Printer) DomainEnum(body []byte) {
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
	if v := gnumStr(m, "id"); v != "" {
		fmt.Fprintf(tw, "id\t%s\n", v)
	}
	if v := gstr(m, "name"); v != "" {
		fmt.Fprintf(tw, "name\t%s\n", v)
	}
	fmt.Fprintf(tw, "values\t%s\n", domainEnumValues(m))
	_ = tw.Flush()
}

// domainEnumValues joins the display value of each entry in the "values" array
// (one line for the whole set), or an em-dash when the enum has no values.
func domainEnumValues(m map[string]any) string {
	items := garr(m, "values")
	if len(items) == 0 {
		return "—"
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		if im, ok := it.(map[string]any); ok {
			if v := firstStr(im, "value", "name", "displayName"); v != "" {
				names = append(names, v)
			}
		}
	}
	if len(names) == 0 {
		return "—"
	}
	return strings.Join(names, ", ")
}
