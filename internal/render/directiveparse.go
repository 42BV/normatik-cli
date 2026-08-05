package render

// Directive parser for the rich page renderer: builds a dnode tree from page
// content. ":::name{attrs}" opens a container, "::name{attrs}" is a leaf and
// everything else is text; inline directives (":name" in running text) stay
// inside #text runs and are resolved later by renderInline (inline.go).

import (
	"regexp"
	"strings"
)

type dnode struct {
	name  string // "" / "#text" for plain text runs; else directive name
	attrs map[string]string
	kids  []*dnode
	lines []string
}

var (
	openRe = regexp.MustCompile(`^:::([a-zA-Z][\w-]*)\s*(\{[^}]*\})?\s*$`)
	leafRe = regexp.MustCompile(`^::([a-zA-Z][\w-]*)(\[[^\]]*\])?\s*(\{[^}]*\})?\s*$`)
	// attrRe matches the public directive attribute grammar: a key optionally
	// followed by "=" and a double-quoted value, single-quoted
	// value (both with backslash escapes) or unquoted value (up to
	// whitespace/'}'). A bare key without "=value" is a boolean flag
	// (DirectiveAttributes#putFlag → "true").
	attrRe = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_-]*)\s*(?:=\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'|([^\s}]+)))?`)
)

// parseDirectives builds a tree: ":::name{attrs}" opens a container, a bare ":::"
// closes the innermost, a line that is only a leaf directive ("::name{attrs}")
// becomes a leaf node (name + attrs, no kids), and everything else is text
// appended to the current node. Inline text directives (":name" inside a
// sentence) stay within #text lines; renderInline resolves those. Lines inside
// a ```-fenced code block are always text — directive syntax in a fence is
// documentation, not a macro to execute (mirrors markdownToASCII's inFence).
// The body of a :::code container is raw for the same reason: code is the only
// macro with preservesRawContent on the web (ContentMacro.ts), so every body
// line up to the closing ":::" stays verbatim text — directive-shaped lines
// included.
func parseDirectives(content string) *dnode {
	root := &dnode{name: "#root"}
	stack := []*dnode{root}
	top := func() *dnode { return stack[len(stack)-1] }
	appendText := func(line string) {
		t := top()
		if len(t.kids) == 0 || t.kids[len(t.kids)-1].name != "#text" {
			t.kids = append(t.kids, &dnode{name: "#text"})
		}
		tn := t.kids[len(t.kids)-1]
		tn.lines = append(tn.lines, line)
	}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Raw-content container body (:::code): everything up to the closing
		// ":::" is verbatim text — directive-shaped lines are content here,
		// not macros (web: preservesRawContent on 'code'). Checked before the
		// fence toggle so a ``` inside a code body stays plain text too.
		if top().name == "code" {
			if trimmed == ":::" {
				stack = stack[:len(stack)-1]
				continue
			}
			appendText(line)
			continue
		}
		// Fenced code blocks: the fence markers and everything between them
		// stay verbatim text; renderTextBlock emits them as a code block.
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			appendText(line)
			continue
		}
		if inFence {
			appendText(line)
			continue
		}
		if trimmed == ":::" {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if mm := openRe.FindStringSubmatch(trimmed); mm != nil {
			n := &dnode{name: mm[1], attrs: parseAttrs(mm[2])}
			top().kids = append(top().kids, n)
			stack = append(stack, n)
			continue
		}
		// Leaf directive: the whole line is "::name{attrs}" (the "::" prefix
		// cannot match a ":::" container open — the third colon blocks the
		// name group). Not pushed on the stack: leaves have no body.
		if mm := leafRe.FindStringSubmatch(trimmed); mm != nil {
			top().kids = append(top().kids, &dnode{name: mm[1], attrs: parseAttrs(mm[3])})
			continue
		}
		// text line — append to a trailing #text child (preserves order)
		appendText(line)
	}
	return root
}

// parseAttrs parses a directive's {...} attribute block. Submatch indices
// (not strings) distinguish a non-participating group from a quoted empty
// string, so key="" stays "" while a bare flag becomes "true".
func parseAttrs(s string) map[string]string {
	attrs := map[string]string{}
	for _, loc := range attrRe.FindAllStringSubmatchIndex(s, -1) {
		key := s[loc[2]:loc[3]]
		switch {
		case loc[4] >= 0: // double-quoted value
			attrs[key] = unescapeAttr(s[loc[4]:loc[5]])
		case loc[6] >= 0: // single-quoted value
			attrs[key] = unescapeAttr(s[loc[6]:loc[7]])
		case loc[8] >= 0: // unquoted value
			attrs[key] = s[loc[8]:loc[9]]
		default: // bare key → boolean flag (AttributeParser: putFlag)
			attrs[key] = "true"
		}
	}
	return attrs
}

// unescapeAttr mirrors AttributeParser#unescapeAttribute: drops the backslash
// of an escape pair (\' → ', \" → ", \\ → \); a trailing lone backslash stays.
func unescapeAttr(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			i++
		}
		b.WriteByte(v[i])
	}
	return b.String()
}
