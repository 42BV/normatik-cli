package render

// inlineDirectiveNames is the production known-set of inline macros that
// renderInline resolves (rich) and inlineToASCII placeholders (--plain).
// A Go map cannot be a const; this package-level var is the single source
// of truth (completeness tests refer to the same set).
var inlineDirectiveNames = map[string]bool{
	"pagelink":   true,
	"enum":       true,
	"color":      true,
	"jira-asset": true,
	"jira-issue": true,
}
