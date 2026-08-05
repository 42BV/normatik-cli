package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReleaseNote renders one release note (GET /public/v1/release-notes/{version})
// as an ASCII document: a version + date header, then the markdown body flattened
// to ASCII through the SAME renderer `pages render --plain` uses (markdownToASCII).
// A release note carries no macroData, so the plain markdown route — not the rich
// macro-resolving one — is the correct analog. JSON mode dumps the detail record
// verbatim. Nil-safe: a missing body renders "(no content)", never a panic.
func (p *Printer) ReleaseNote(body []byte) {
	if p.Mode == JSON {
		p.rawDump(body)
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		fmt.Fprintln(p.Out, terminalLine(string(body)))
		return
	}
	header := gstr(m, "version")
	if header == "" {
		header = "(unversioned)"
	}
	if date := gstr(m, "date"); date != "" {
		header += "  ·  " + date
	}
	fmt.Fprintln(p.Out, header)
	fmt.Fprintln(p.Out, strings.Repeat("=", displayWidth(header)))
	fmt.Fprintln(p.Out, markdownToASCII(gtext(m, "body")))
}
