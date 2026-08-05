package render

import (
	"regexp"
	"sort"
	"strings"
)

// DirectiveCount is one macro/directive name and how often it occurs in a page's
// content.
type DirectiveCount struct {
	Directive string `json:"directive"`
	Count     int    `json:"count"`
}

// inlineDirectiveScanRe matches an inline directive (:name[...] or :name{...}).
// It REQUIRES a [label] or {attrs} suffix so a plain colon in prose ("Note:foo")
// is not mistaken for a macro — stricter than textDirectiveRe used for rendering.
var inlineDirectiveScanRe = regexp.MustCompile(`:([a-zA-Z][\w-]*)(?:\[[^\]]*\]|\{[^}]*\})`)

// ScanDirectives counts content-macro usage in raw page markdown, by directive
// name, across all three forms:
//   - container (block):  :::name{...}      (a bare ":::" closes, not counted)
//   - leaf (block):       ::name{...}
//   - inline:             :name[...] / :name{...}  (within a line)
//
// Block-directive lines are counted once and not re-scanned for inline matches.
// Result is sorted by count desc, then name, for stable output.
func ScanDirectives(content string) []DirectiveCount {
	counts := map[string]int{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ":::" { // container close
			continue
		}
		if m := containerDirectiveRe.FindStringSubmatch(trimmed); m != nil {
			counts[m[1]]++
			continue
		}
		if m := leafDirectiveRe.FindStringSubmatch(trimmed); m != nil {
			counts[m[1]]++
			continue
		}
		for _, m := range inlineDirectiveScanRe.FindAllStringSubmatch(line, -1) {
			counts[m[1]]++
		}
	}
	out := make([]DirectiveCount, 0, len(counts))
	for name, c := range counts {
		out = append(out, DirectiveCount{Directive: name, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Directive < out[j].Directive
	})
	return out
}
