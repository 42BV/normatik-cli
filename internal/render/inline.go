package render

// Inline rendering for the rich page renderer: inline macros (:enum,
// :pagelink, :color), markdown emphasis/links/inline-code and the shared pill
// helpers. renderInline resolves everything inside a single line of running
// text; block structure is handled by renderTextBlock (macrorender.go).

import (
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	inlineEnumRe      = regexp.MustCompile(`:enum\{([^}]*)\}`)
	inlinePageRe      = regexp.MustCompile(`:pagelink(?:\[([^\]]*)\])?\{([^}]*)\}`)
	inlineJiraAssetRe = regexp.MustCompile(`:jira-asset\{([^}]*)\}`)
	inlineJiraIssueRe = regexp.MustCompile(`:jira-issue\{([^}]*)\}`)
	inlineCodeRe      = regexp.MustCompile("`([^`\n]+)`")
	strikeRe          = regexp.MustCompile(`~~([^~]+)~~`)
	// The {…} attribute block is optional: ":color[label]" without attrs must
	// keep its label (the web renders the children plain when color is absent —
	// DirectiveMarkdownRenderer.tsx directive-color fallback).
	inlineColorRe = regexp.MustCompile(`:color\[([^\]]*)\](\{[^}]*\})?`)
	boldRe        = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe      = regexp.MustCompile(`\*([^*]+)\*`)
	mdLinkRe      = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	sgrResetRe    = regexp.MustCompile("\x1b\\[0?m")
)

func renderInline(s string, idx *macroIndex) string {
	// Inline code spans are verbatim leaves (InlineCodeMacroDataResolver: "it
	// cannot contain other marks and always nests innermost"): stash their
	// content behind NUL placeholders before any other pass so no enum/
	// pagelink/emphasis resolution happens inside, and restore them styled at
	// the very end (after the emphasis/strike passes).
	var codeSpans []string
	s = inlineCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		codeSpans = append(codeSpans, inlineCodeRe.FindStringSubmatch(m)[1])
		return "\x00" + strconv.Itoa(len(codeSpans)-1) + "\x00"
	})
	s = inlineEnumRe.ReplaceAllStringFunc(s, func(tok string) string {
		a := parseAttrs(inlineEnumRe.FindStringSubmatch(tok)[1])
		return idx.enumPill(a["enum"], a["value"])
	})
	s = inlinePageRe.ReplaceAllStringFunc(s, func(tok string) string {
		mm := inlinePageRe.FindStringSubmatch(tok)
		a := parseAttrs(mm[2])
		if mm[1] != "" {
			a["label"] = mm[1] // preferred [label] form (see `macros docs pagelink`)
		}
		return idx.pageLink(a)
	})
	s = inlineJiraAssetRe.ReplaceAllStringFunc(s, func(tok string) string {
		a := parseAttrs(inlineJiraAssetRe.FindStringSubmatch(tok)[1])
		return idx.jiraAssetInline(a)
	})
	s = inlineJiraIssueRe.ReplaceAllStringFunc(s, func(tok string) string {
		a := parseAttrs(inlineJiraIssueRe.FindStringSubmatch(tok)[1])
		return idx.jiraIssueInline(a)
	})
	s = mdLinkRe.ReplaceAllString(s, "$1 ($2)")
	// :color must resolve before the emphasis passes so its ANSI output sits
	// INSIDE the emphasis span — canonical nesting per StrikeMacroDataResolver
	// @MacroDoc: strike nests outside color. Running it after would let the
	// strike pass tear the directive syntax apart per grapheme (leaking it as
	// literal text).
	s = inlineColorRe.ReplaceAllStringFunc(s, func(m string) string {
		mm := inlineColorRe.FindStringSubmatch(m)
		return colorLabel(mm[1], parseAttrs(mm[2])["color"])
	})
	s = boldRe.ReplaceAllStringFunc(s, func(m string) string {
		return lipgloss.NewStyle().Bold(true).Render(boldRe.FindStringSubmatch(m)[1])
	})
	s = italicRe.ReplaceAllStringFunc(s, func(m string) string {
		return lipgloss.NewStyle().Italic(true).Render(italicRe.FindStringSubmatch(m)[1])
	})
	// Strikethrough is hand-rolled: lipgloss renders it per grapheme and is
	// not ANSI-aware, so embedded SGR sequences (a resolved :color or enum
	// pill inside ~~…~~) would be torn apart byte-by-byte and leak as literal
	// text. Emit one whole-span pair (9 on, 29 off) and re-arm strike after
	// any embedded full reset so the rest of the span stays struck.
	s = strikeRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := sgrResetRe.ReplaceAllString(strikeRe.FindStringSubmatch(m)[1], "${0}\x1b[9m")
		return "\x1b[9m" + inner + "\x1b[29m"
	})
	// Unknown inline :name sequences stay literal (same norm as the UI).
	codeStyle := lipgloss.NewStyle().Faint(true)
	for i, span := range codeSpans {
		s = strings.Replace(s, "\x00"+strconv.Itoa(i)+"\x00", codeStyle.Render(span), 1)
	}
	return s
}

// colorLabel renders a :color[label]{color="#RRGGBB"} label with a lipgloss
// foreground in that color (ColorMacroDataResolver @AttrDoc: hex #RRGGBB).
// An empty or unparseable color (lipgloss.Color also accepts ANSI numbers but
// no color names) renders the label plain.
func colorLabel(label, c string) string {
	if c == "" {
		return label
	}
	col := lipgloss.Color(c)
	if _, none := col.(lipgloss.NoColor); none {
		return label
	}
	return lipgloss.NewStyle().Foreground(col).Render(label)
}

func (idx *macroIndex) enumPill(enum, value string) string {
	e, _ := idx.entryFor("enums", &dnode{name: "enum", attrs: map[string]string{"enum": enum, "value": value}})
	color := ""
	label := value
	if e != nil {
		if c := gstr(e, "color"); c != "" {
			color = c
		}
		if v := gstr(e, "value"); v != "" {
			label = v
		}
	}
	return pill(label, color)
}

// pill renders a bold padded badge: background in the given hex color with a
// contrast-picked foreground (fgFor), or reverse-video when no color is known.
// Shared by the inline enum pill and the page-tasks status column.
func pill(label, color string) string {
	st := lipgloss.NewStyle().Padding(0, 1).Bold(true)
	if color != "" {
		st = st.Background(lipgloss.Color(color)).Foreground(lipgloss.Color(fgFor(color)))
	} else {
		st = st.Reverse(true)
	}
	return st.Render(label)
}

func (idx *macroIndex) pageLink(a map[string]string) string {
	key := a["id"]
	if key == "" {
		key = a["pageId"]
	}
	// A caller-supplied link text wins; without one the page name is shown
	// (PagelinkMacroDataResolver: "without a label the page name is shown").
	// `text` is the documented legacy attribute variant of the [label].
	label := a["label"]
	if label == "" {
		label = a["text"]
	}
	if label != "" {
		return "→ " + label
	}
	if pl, ok := idx.entryFor("pagelinks", &dnode{name: "pagelink", attrs: a}); ok {
		if n := firstStr(pl, "name", "title", "pageName"); n != "" {
			return "→ " + n
		}
	}
	return "→ page " + key
}

// fgFor returns a readable foreground (black/white) for a hex background.
func fgFor(hex string) string {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return "#000000"
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 0)
	g, _ := strconv.ParseInt(h[2:4], 16, 0)
	bl, _ := strconv.ParseInt(h[4:6], 16, 0)
	// perceived luminance
	lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl))
	if lum > 150 {
		return "#111111"
	}
	return "#FFFFFF"
}
