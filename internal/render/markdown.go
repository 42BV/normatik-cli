package render

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// markdownToASCII renders Normatik markdown to a pragmatic, line-oriented ASCII
// representation for the terminal. It is intentionally dependency-free (no
// external markdown engine in v1) and best-effort:
//   - headings get underlines; inline emphasis/links are flattened;
//   - ```-fenced code is emitted verbatim, indented;
//   - leaf directives (::name) become a "[macro: name]" placeholder;
//   - container directives (:::name ... :::) become a single "[macro: name]"
//     placeholder and their body is omitted (never executed or recursed —
//     this is also how IncludePages are handled).
func markdownToASCII(md string) string {
	md = terminalText(md)
	if strings.TrimSpace(md) == "" {
		return "(no content)"
	}
	var out []string
	inFence := false
	inContainer := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		// Inside a container directive: swallow the body up to the closing ":::".
		if inContainer {
			if trimmed == ":::" {
				inContainer = false
			}
			continue
		}

		// Fenced code blocks (```) — emit verbatim, indented.
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, "    ----")
			continue
		}
		if inFence {
			out = append(out, "    "+line)
			continue
		}

		// Container directive open (:::name) — placeholder, body swallowed.
		if m := containerDirectiveRe.FindStringSubmatch(trimmed); m != nil {
			out = append(out, "[macro: "+m[1]+"]")
			inContainer = true
			continue
		}
		// Leaf directive (::name) — placeholder.
		if m := leafDirectiveRe.FindStringSubmatch(trimmed); m != nil {
			out = append(out, "[macro: "+m[1]+"]")
			continue
		}

		// Headings.
		if m := headingRe.FindStringSubmatch(trimmed); m != nil {
			level := len(m[1])
			text := inlineToASCII(m[2])
			switch level {
			case 1:
				out = append(out, text, strings.Repeat("=", displayWidth(text)))
			case 2:
				out = append(out, text, strings.Repeat("-", displayWidth(text)))
			default:
				out = append(out, strings.Repeat("#", level)+" "+text)
			}
			continue
		}

		// List items: normalise the bullet to "- " BEFORE inline flattening, so a
		// "* item" marker is not eaten by the emphasis stripper.
		if m := listItemRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1]+"- "+inlineToASCII(m[2]))
			continue
		}

		out = append(out, inlineToASCII(line))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

var (
	headingRe            = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listItemRe           = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	containerDirectiveRe = regexp.MustCompile(`^:::([a-zA-Z][\w-]*)`)
	leafDirectiveRe      = regexp.MustCompile(`^::([a-zA-Z][\w-]*)`)
	textDirectiveRe      = regexp.MustCompile(`:([a-zA-Z][\w-]*)(\[[^\]]*\])?(\{[^}]*\})?`)
	linkRe               = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	emphasisRe           = regexp.MustCompile(`(\*\*|__|\*|_|` + "`" + `)`)
)

// inlineToASCII flattens inline markdown: links become "text (url)", known
// inline text-directives become "[name]", unknown ones stay literal, and
// emphasis/code markers are stripped. The name is matched in full so a
// prefix of a known name (enumeration, colorful, pagelinks) stays literal.
func inlineToASCII(s string) string {
	s = linkRe.ReplaceAllString(s, "$1 ($2)")
	s = textDirectiveRe.ReplaceAllStringFunc(s, func(m string) string {
		name := textDirectiveRe.FindStringSubmatch(m)[1]
		if inlineDirectiveNames[name] {
			return "[" + name + "]"
		}
		return m
	})
	s = emphasisRe.ReplaceAllString(s, "")
	return s
}

// displayWidth returns the VISIBLE cell width of s, clamped so heading
// underlines never explode on very long titles. Inputs may carry lipgloss
// styling (renderTextBlock measures renderInline output for its heading
// underlines), so this must be ANSI-aware: counting runes would include the
// escape bytes and make underlines/padding too long. ansi.StringWidth ignores
// escape sequences and accounts for wide graphemes.
func displayWidth(s string) int {
	n := ansi.StringWidth(s)
	if n > 80 {
		return 80
	}
	if n == 0 {
		return 1
	}
	return n
}
