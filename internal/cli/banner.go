package cli

import (
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// ANSI-Shadow "NORMATIK" wordmark, rendered above root --help on a TTY.
var bannerRows = []string{
	`███╗   ██╗ ██████╗ ██████╗ ███╗   ███╗ █████╗ ████████╗██╗██╗  ██╗`,
	`████╗  ██║██╔═══██╗██╔══██╗████╗ ████║██╔══██╗╚══██╔══╝██║██║ ██╔╝`,
	`██╔██╗ ██║██║   ██║██████╔╝██╔████╔██║███████║   ██║   ██║█████╔╝ `,
	`██║╚██╗██║██║   ██║██╔══██╗██║╚██╔╝██║██╔══██║   ██║   ██║██╔═██╗ `,
	`██║ ╚████║╚██████╔╝██║  ██║██║ ╚═╝ ██║██║  ██║   ██║   ██║██║  ██╗`,
	`╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝   ╚═╝╚═╝  ╚═╝`,
}

// banner builds the coloured wordmark: oranje→terracotta verloop + tagline.
// lipgloss strips colour automatically when the output is not a colour terminal.
func banner() string {
	bands := []string{"#D97A54", "#D97A54", "#A2543F", "#A2543F", "#9A4E3B", "#9A4E3B"}
	var b strings.Builder
	b.WriteString("\n")
	for i, row := range bannerRows {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(bands[i]))
		b.WriteString("  " + st.Render(row) + "\n")
	}
	diamond := lipgloss.NewStyle().Foreground(lipgloss.Color("#D97A54")).Render("◆")
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8290")).Render("the CLI for the Normatik Public API")
	b.WriteString("                " + diamond + "  " + tag + "\n")
	return b.String()
}

// shouldShowBanner: only on a real terminal, only for the root help / bare run,
// never when piped (agents/CI) or when --no-banner is passed.
func shouldShowBanner(args []string) bool {
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	for _, a := range args {
		if a == "--no-banner" {
			return false
		}
	}
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "-h", "--help", "help":
		return true
	}
	return false
}
