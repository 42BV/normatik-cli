package render

import (
	"encoding/json"
	"fmt"
)

// MacroScan renders a content-macro usage scan (macro -> pages). JSON mode dumps
// the full MacroScanResult verbatim; table mode lists the accessible pages and
// prints a restricted-access summary footer on stderr.
func (p *Printer) MacroScan(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var res struct {
		Accessible []map[string]any `json:"accessible"`
		Restricted map[string]any   `json:"restricted"`
	}
	if json.Unmarshal(body, &res) != nil {
		p.rawDump(body)
		return
	}
	if res.Accessible == nil {
		res.Accessible = []map[string]any{}
	}
	arr, _ := json.Marshal(res.Accessible)
	p.List(arr, "pageId", "name", "count")
	if r := res.Restricted; r != nil {
		pc, mc := numStr(r["pageCount"]), numStr(r["macroCount"])
		if pc != "0" || mc != "0" {
			fmt.Fprintf(p.Err, "\n%s page(s) hidden (no access), %s occurrence(s) not shown.\n", pc, mc)
		}
	}
}
