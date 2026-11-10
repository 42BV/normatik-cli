package render

import "strings"

// EscapeCodeLines prepares the body of a :::code block for writing back (NORM-321
// P2). The block grammar reads ":::name" and ":::" at column 0 also inside code,
// so a line meant as code gets one tilde in front: ":::" becomes "~:::", "~:::"
// becomes "~~:::". Only a line that starts at column 0 with zero or more tildes
// directly followed by ":::" changes; "::marker", "~/bin" and " ~:::" stay as they
// are. Line endings are kept.
func EscapeCodeLines(code string) string {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, "~"), ":::") {
			lines[i] = "~" + line
		}
	}
	return strings.Join(lines, "\n")
}
