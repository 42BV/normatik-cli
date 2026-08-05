package cli

import "charm.land/lipgloss/v2"

// Shared Normatik house-style text styles for interactive CLI output, mirroring
// theme.go / banner.go: kompas-oranje as the accent, a soft plum-grey for
// secondary text, and green to signal a completed authentication. lipgloss
// strips the colour automatically when the output is not a colour terminal.
var (
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97A54")).Bold(true) // kompas-oranje
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8290"))            // plum-grey
	styleStrong  = lipgloss.NewStyle().Bold(true)
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("#3F9E5A")).Bold(true) // geauthenticeerd
)
