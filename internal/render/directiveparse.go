package render

// Directive parser for the rich page renderer: builds a dnode tree from page
// content following the shared block grammar (directive-grammar corpus,
// NORM-321 R1–R7, K1–K3). Only ":::name{attrs}" at column 0 opens a container and
// only exactly ":::" at column 0 closes one; everything else — an indented
// ":::tab", a block-shaped line after a list or quote marker, "::pdf{id=1}" —
// stays text. Leaf directives ("::name{attrs}") are not recognised here: they
// stay in the #text runs and are resolved by the markdown render
// (renderTextBlock, leafLine). Inline directives (":name" in running text) stay
// inside #text runs and are resolved later by renderInline (inline.go).

import (
	"fmt"
	"sort"
	"strings"
)

type dnode struct {
	name  string // "" / "#text" for plain text runs; else directive name
	attrs map[string]string
	kids  []*dnode
	lines []string
	line  int // source line: the opener of a container, the first line of a text run
	end   int // source line of the closer of a container (0 while unclosed)
	// rejected marks a frame of an opener with a malformed attribute list (P6):
	// it is reported once, opens no block in the tree, but takes its own closer
	// (NORM-ivvltylb). Its lines stay text in the surrounding node.
	rejected bool
	// diagnostics lists, on the root only, what the grammar leaves over: blocks
	// that never close and closers without a block, with the shared messages.
	diagnostics []grammarMessage
}

// grammarMessage is a message of the shared block grammar, in the wording of
// the shared directive-grammar corpus.
type grammarMessage struct {
	line    int
	message string
}

// parseDirectives builds a tree: ":::name{attrs}" at column 0 opens a container
// and a bare ":::" at column 0 closes the innermost one (trailing whitespace
// does not count). Everything else is text appended to the current node, with
// the raw line kept. Markdown fences belong to the text pieces: they never
// hide a directive block boundary from this context-free parser.
//
// The :::code body gets no special tokenizer: block lines inside code are read
// by the same rules as everywhere else, so a line meant as code has to be
// written as "~:::name" or "~:::" (see EscapeCodeLines). "::marker" and
// "::pdf{id=1}" are plain code text. A fence inside a :::code body does not hide
// block lines (the grammar knows no code blocks).
func parseDirectives(content string) *dnode {
	root := &dnode{name: "#root"}
	stack := []*dnode{root}
	top := func() *dnode { return stack[len(stack)-1] }
	lineNo := 0
	lastCode := [2]int{} // open and close line of the last closed code block
	appendText := func(line string) {
		t := top()
		if len(t.kids) == 0 || t.kids[len(t.kids)-1].name != "#text" {
			t.kids = append(t.kids, &dnode{name: "#text", line: lineNo})
		}
		tn := t.kids[len(t.kids)-1]
		tn.lines = append(tn.lines, line)
	}
	for _, line := range strings.Split(content, "\n") {
		lineNo++
		if strings.TrimRight(line, " \t\r") == ":::" {
			if len(stack) > 1 {
				closed := top()
				stack = stack[:len(stack)-1]
				if closed.rejected {
					releaseRejected(top(), closed)
					appendText(line)
					continue
				}
				closed.end = lineNo
				if closed.name == "code" {
					lastCode = [2]int{closed.line, lineNo}
				}
				continue
			}
			message := fmt.Sprintf("Line %d: ':::' does not close a block.", lineNo)
			if lastCode[0] > 0 {
				message += fmt.Sprintf(" The last code block (line %d) ended on line %d; to show ':::' as code, write '~:::'.",
					lastCode[0], lastCode[1])
			}
			root.diagnostics = append(root.diagnostics, grammarMessage{lineNo, message})
			continue
		}
		name, attrs, ok, attrErr := parseOpenLine(line)
		if attrErr != "" {
			// Like the backend (ContentLexicalValidator): a malformed attribute
			// list is a message on its line; the line opens no block but its
			// frame takes the matching closer (NORM-ivvltylb).
			root.diagnostics = append(root.diagnostics, grammarMessage{lineNo, fmt.Sprintf("Line %d: %s", lineNo, attrErr)})
			appendText(line)
			stack = append(stack, &dnode{name: "#rejected", line: lineNo, rejected: true})
			continue
		}
		if ok {
			n := &dnode{name: name, attrs: attrs, line: lineNo}
			top().kids = append(top().kids, n)
			stack = append(stack, n)
			continue
		}
		appendText(line)
	}
	for i := len(stack) - 1; i >= 1; i-- {
		open := stack[i]
		if open.rejected {
			releaseRejected(stack[i-1], open)
			continue
		}
		root.diagnostics = append(root.diagnostics,
			grammarMessage{open.line, fmt.Sprintf("Block '%s' on line %d is not closed.", open.name, open.line)})
	}
	sort.SliceStable(root.diagnostics, func(i, j int) bool { return root.diagnostics[i].line < root.diagnostics[j].line })
	return root
}

// releaseRejected moves what was read under a rejected opener (P6) to its
// parent: text runs merge with the parent's last text run, blocks follow.
func releaseRejected(parent, rejected *dnode) {
	for _, k := range rejected.kids {
		if k.name != "#text" {
			parent.kids = append(parent.kids, k)
			continue
		}
		if n := len(parent.kids); n > 0 && parent.kids[n-1].name == "#text" {
			parent.kids[n-1].lines = append(parent.kids[n-1].lines, k.lines...)
			continue
		}
		parent.kids = append(parent.kids, k)
	}
}

// parseOpenLine reads a container opener at column 0: ":::" directly followed
// by a name (a letter, then letters, digits, "_" or "-"), optionally an
// attribute block, and nothing else but whitespace. Anything else — "::::name",
// a leading space, trailing text — is not an opener. An attribute list that is
// not well formed (P6, see checkAttrs) gives attrErr, the message of the backend
// without its line number.
func parseOpenLine(line string) (name string, attrs map[string]string, ok bool, attrErr string) {
	if len(line) < 4 || !strings.HasPrefix(line, ":::") || !isASCIILetter(line[3]) {
		return "", nil, false, ""
	}
	i := 4
	for i < len(line) && isNameChar(line[i]) {
		i++
	}
	name = line[3:i]
	for i < len(line) && isBlankByte(line[i]) {
		i++
	}
	if i == len(line) {
		return name, map[string]string{}, true, ""
	}
	if line[i] != '{' {
		return "", nil, false, ""
	}
	if msg := checkAttrs(line, i); msg != "" {
		return "", nil, false, msg
	}
	attrs, end, closed := scanAttrs(line, i+1)
	if !closed {
		return "", nil, false, ""
	}
	if strings.TrimRight(line[end:], " \t\r") != "" {
		return "", nil, false, ""
	}
	return name, attrs, true, ""
}

// checkAttrs reads the attribute list that opens with the brace at s[open]
// the way AttributeParser#read does in the backend (P6): a name, optionally
// "=" and a value; a value is quoted (anything but the closing quote, a
// backslash escapes the next character) or unquoted (no whitespace and none of
// "'<=>`); "#id" is the only shorthand. It returns "" for a well-formed list,
// else the backend's message.
func checkAttrs(s string, open int) string {
	seen := map[string]bool{}
	pos := open + 1
	for {
		pos = skipBlanks(s, pos)
		if pos >= len(s) {
			return "Attributes are not closed: expected '}'."
		}
		c := s[pos]
		if c == '}' {
			return ""
		}
		var msg string
		switch c {
		case '.':
			return fmt.Sprintf("Class shorthand '%s' is not supported. Use #id or name=\"value\".", s[pos:shorthandEnd(s, pos+1)])
		case '#':
			pos, msg = checkIDShorthand(s, pos, seen)
		default:
			pos, msg = checkAttribute(s, pos, seen)
		}
		if msg != "" {
			return msg
		}
		if pos < len(s) && s[pos] != '}' && !isBlankByte(s[pos]) {
			return fmt.Sprintf("Unexpected '%c' in attributes.", s[pos])
		}
	}
}

func checkIDShorthand(s string, hash int, seen map[string]bool) (int, string) {
	end := shorthandEnd(s, hash+1)
	if end == hash+1 {
		return end, "Expected an id after '#'."
	}
	return end, markSeen(seen, "id")
}

func checkAttribute(s string, start int, seen map[string]bool) (int, string) {
	if !isAttrNameStart(s[start]) {
		return start, fmt.Sprintf("Unexpected '%c' in attributes.", s[start])
	}
	pos := start + 1
	for pos < len(s) && (isAttrNameStart(s[pos]) || s[pos] >= '0' && s[pos] <= '9' || s[pos] == '-' || s[pos] == '.') {
		pos++
	}
	name := s[start:pos]
	afterName := skipBlanks(s, pos)
	if afterName >= len(s) || s[afterName] != '=' {
		return pos, markSeen(seen, name)
	}
	pos = skipBlanks(s, afterName+1)
	if pos >= len(s) {
		return pos, "Attributes are not closed: expected '}'."
	}
	if q := s[pos]; q == '"' || q == '\'' {
		j := pos + 1
		for j < len(s) && s[j] != q {
			if s[j] == '\\' {
				j++
			}
			j++
		}
		if j >= len(s) {
			return j, fmt.Sprintf("Attribute '%s' has a value with an unclosed quote.", name)
		}
		return j + 1, markSeen(seen, name)
	}
	end := pos
	for end < len(s) && !isBlankByte(s[end]) && s[end] != '}' {
		if strings.IndexByte("\"'<=>`", s[end]) >= 0 {
			return end, fmt.Sprintf("Unexpected '%c' in attributes.", s[end])
		}
		end++
	}
	if end == pos {
		return end, fmt.Sprintf("Attribute '%s' has no value after '='.", name)
	}
	return end, markSeen(seen, name)
}

// markSeen records an attribute name and reports a name given more than once.
func markSeen(seen map[string]bool, name string) string {
	if seen[name] {
		return fmt.Sprintf("Attribute '%s' is given more than once.", name)
	}
	seen[name] = true
	return ""
}

func shorthandEnd(s string, start int) int {
	end := start
	for end < len(s) && !isBlankByte(s[end]) && strings.IndexByte("\"#'.<=>`}", s[end]) < 0 {
		end++
	}
	return end
}

func skipBlanks(s string, pos int) int {
	for pos < len(s) && isBlankByte(s[pos]) {
		pos++
	}
	return pos
}

func isAttrNameStart(c byte) bool { return isASCIILetter(c) || c == '_' || c == ':' }

// leafLine reads a leaf directive from a text line: optional markdown prefix
// (up to three spaces of indentation, or a list marker, or quote markers, with
// their own indentation), then "::name", an optional "[label]" and an optional
// "{attrs}", and nothing else. The prefix is returned so a renderer can keep a
// leaf inside its list item or quote. "::::name" and ":::name" are not leaves.
func leafLine(line string) (prefix, name string, attrs map[string]string, ok bool) {
	body := strings.TrimRight(line, " \t\r")
	i := leafPrefixEnd(body)
	rest := body[i:]
	if len(rest) < 3 || !strings.HasPrefix(rest, "::") || !isASCIILetter(rest[2]) {
		return "", "", nil, false
	}
	j := 3
	for j < len(rest) && isNameChar(rest[j]) {
		j++
	}
	name = rest[2:j]
	if j < len(rest) && rest[j] == '[' {
		end := strings.IndexByte(rest[j:], ']')
		if end < 0 {
			return "", "", nil, false
		}
		j += end + 1
	}
	for j < len(rest) && isBlankByte(rest[j]) {
		j++
	}
	attrs = map[string]string{}
	if j < len(rest) {
		if rest[j] != '{' {
			return "", "", nil, false
		}
		var closed bool
		var end int
		attrs, end, closed = scanAttrs(rest, j+1)
		if !closed || strings.TrimSpace(rest[end:]) != "" {
			return "", "", nil, false
		}
	}
	return body[:i], name, attrs, true
}

// leafPrefixEnd returns the index where the content of a line starts after its
// markdown prefix: quote markers ("> "), a list marker ("- ", "* ", "+ ",
// "1. ", "1) ") or, without marker, at most three spaces of indentation (four
// or more is an indented code block, not a leaf).
func leafPrefixEnd(line string) int {
	i := 0
	for {
		j := i
		for j < len(line) && isBlankByte(line[j]) {
			j++
		}
		if j < len(line) && line[j] == '>' {
			i = j + 1
			if i < len(line) && line[i] == ' ' {
				i++
			}
			continue
		}
		if n := listMarkerLen(line[j:]); n > 0 {
			return j + n
		}
		if i == 0 && j > 3 {
			return 0
		}
		return j
	}
}

// listMarkerLen returns the length of a list marker plus the spaces after it at
// the start of s, or 0 when s does not start with one.
func listMarkerLen(s string) int {
	n := 0
	switch {
	case len(s) > 1 && strings.IndexByte("-*+", s[0]) >= 0 && s[1] == ' ':
		n = 2
	default:
		d := 0
		for d < len(s) && s[d] >= '0' && s[d] <= '9' {
			d++
		}
		if d == 0 || d+1 >= len(s) || (s[d] != '.' && s[d] != ')') || s[d+1] != ' ' {
			return 0
		}
		n = d + 2
	}
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isNameChar(c byte) bool {
	return isASCIILetter(c) || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func isBlankByte(c byte) bool { return c == ' ' || c == '\t' }

func isSpaceByte(c byte) bool { return isBlankByte(c) || c == '\n' || c == '\r' }

// parseAttrs parses a directive's {...} attribute block (the braces are
// optional: inline callers pass the text between them). Bare keys become
// boolean flags ("true"); a quoted empty string stays "".
func parseAttrs(s string) map[string]string {
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '{' {
		i++
	}
	attrs, _, _ := scanAttrs(s, i)
	return attrs
}

// scanAttrs reads the public directive attribute grammar character by
// character from s[i:] up to the closing "}": a key ([a-zA-Z_][a-zA-Z0-9_-]*)
// optionally followed by "=" and a double-quoted value, a single-quoted value
// (both with backslash escapes, so a "}" inside quotes does not end the block)
// or an unquoted value (up to whitespace or "}"). A bare key is a boolean flag
// (DirectiveAttributes#putFlag → "true"). It returns the attributes, the index
// after the closing "}" and whether that "}" was found; stray characters
// between attributes are skipped.
func scanAttrs(s string, i int) (attrs map[string]string, next int, closed bool) {
	attrs = map[string]string{}
	for i < len(s) {
		c := s[i]
		switch {
		case c == '}':
			return attrs, i + 1, true
		case isSpaceByte(c):
			i++
		case isASCIILetter(c) || c == '_':
			start := i
			for i < len(s) && isNameChar(s[i]) {
				i++
			}
			key := s[start:i]
			j := i
			for j < len(s) && isSpaceByte(s[j]) {
				j++
			}
			if j >= len(s) || s[j] != '=' {
				attrs[key] = "true"
				continue
			}
			j++
			for j < len(s) && isSpaceByte(s[j]) {
				j++
			}
			value, end := scanAttrValue(s, j)
			attrs[key] = value
			i = end
		default:
			i++
		}
	}
	return attrs, i, false
}

// scanAttrValue reads one attribute value starting at s[i] and returns it
// (unescaped when quoted) and the index after it.
func scanAttrValue(s string, i int) (string, int) {
	if i >= len(s) {
		return "", i
	}
	if q := s[i]; q == '"' || q == '\'' {
		j := i + 1
		for j < len(s) && s[j] != q {
			if s[j] == '\\' && j+1 < len(s) {
				j++
			}
			j++
		}
		if j >= len(s) {
			return unescapeAttr(s[i+1:]), len(s)
		}
		return unescapeAttr(s[i+1 : j]), j + 1
	}
	j := i
	for j < len(s) && !isSpaceByte(s[j]) && s[j] != '}' {
		j++
	}
	return s[i:j], j
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
