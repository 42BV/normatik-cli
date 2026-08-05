package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
)

// MacroDocs renders the full macro knowledge base (GET /content-macros/docs):
// the preamble as an indented block, the shared filter syntax under its own
// heading, then a table of the enabled macros, closed with a per-macro details
// hint. JSON mode dumps the body verbatim so the machine contract is unchanged.
func (p *Printer) MacroDocs(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	if pre := gtext(m, "preamble"); pre != "" {
		indentBlock(p, pre)
	}
	if fs := gtext(m, "filterSyntax"); fs != "" {
		fmt.Fprintln(p.Out, "\nFilter syntax:")
		indentBlock(p, fs)
	}
	if macros := garr(m, "macros"); macros != nil {
		fmt.Fprintln(p.Out)
		// The macros are nested under "macros" — re-marshal the bare array so
		// the generic List printer sees a recognizable list shape.
		raw, _ := json.Marshal(macros)
		p.List(raw, "directiveName", "form", "module", "summary")
	}
	if !p.Quiet {
		fmt.Fprintln(p.Err, "\nDetails: normatik macros docs <name>")
	}
}

// MacroDoc renders one macro's documentation (GET /content-macros/{name}/docs):
// name + form/module, summary, the examples one per line, and an attribute
// table (default '-' when the attribute has none). JSON mode dumps the body
// verbatim.
func (p *Printer) MacroDoc(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	name := gstr(m, "directiveName")
	if name == "" {
		name = "(unnamed)"
	}
	if fm := joinNonEmpty(" · ", gstr(m, "form"), gstr(m, "module")); fm != "" {
		fmt.Fprintf(p.Out, "%s  (%s)\n", name, fm)
	} else {
		fmt.Fprintln(p.Out, name)
	}
	if summary := gstr(m, "summary"); summary != "" {
		fmt.Fprintf(p.Out, "  %s\n", summary)
	}
	if examples := garr(m, "examples"); len(examples) > 0 {
		fmt.Fprintln(p.Out, "\nExamples:")
		for _, e := range examples {
			if s, ok := e.(string); ok && s != "" {
				fmt.Fprintf(p.Out, "  %s\n", terminalLine(s))
			}
		}
	}
	fmt.Fprintln(p.Out, "\nAttributes:")
	attrs := garr(m, "attributes")
	if len(attrs) == 0 {
		fmt.Fprintln(p.Out, "  (none)")
		return
	}
	tw := tabwriter.NewWriter(p.Out, 0, 2, 2, ' ', 0)
	if !p.Quiet {
		fmt.Fprintln(tw, "NAME\tTYPE\tREQUIRED\tDEFAULT\tDOC")
	}
	for _, a := range attrs {
		am, ok := a.(map[string]any)
		if !ok {
			continue
		}
		def := gstr(am, "def")
		if def == "" {
			def = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			gstr(am, "name"), gstr(am, "type"), cell(am["required"]), def, gstr(am, "doc"))
	}
	_ = tw.Flush()
}

// indentBlock prints a (possibly multi-line) text block indented by two spaces.
func indentBlock(p *Printer, text string) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		fmt.Fprintf(p.Out, "  %s\n", line)
	}
}
