package render

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
)

// ArchivedPageView renders GET /pages/admin/archive/{id} (ArchivedPageViewResult)
// in table mode: flattens the nested page object so name / page-type show as
// scalars. Nested page detail is "…" under Raw, so a dedicated renderer is
// needed. JSON mode dumps the body verbatim.
func (p *Printer) ArchivedPageView(body []byte) {
	p.renderNestedPageView(body, []string{"archivedAt", "archivedReason", "parentId"})
}

// TrashedPageView renders GET /pages/admin/trash/{id} (TrashedPageViewResult)
// in table mode: flattens the nested page object so name / page-type show as
// scalars. Trash is reasonless — only deletedAt and parentId sit next to page.
// JSON mode dumps the body verbatim.
func (p *Printer) TrashedPageView(body []byte) {
	p.renderNestedPageView(body, []string{"deletedAt", "parentId"})
}

func (p *Printer) renderNestedPageView(body []byte, metaKeys []string) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	page, _ := m["page"].(map[string]any)
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	if page != nil {
		if v := gnumStr(page, "id"); v != "" {
			fmt.Fprintf(tw, "id\t%s\n", v)
		}
		if v := gstr(page, "name"); v != "" {
			fmt.Fprintf(tw, "name\t%s\n", v)
		}
		if v := nestedName(page, "pageType"); v != "" {
			fmt.Fprintf(tw, "pageTypeName\t%s\n", v)
		}
	}
	for _, key := range metaKeys {
		if val, ok := m[key]; ok && val != nil {
			fmt.Fprintf(tw, "%s\t%s\n", key, cell(val))
		}
	}
	_ = tw.Flush()
}

// nestedName returns obj[key].name when key is a nested object, else "".
func nestedName(m map[string]any, key string) string {
	obj, ok := m[key].(map[string]any)
	if !ok {
		return ""
	}
	return gstr(obj, "name")
}
