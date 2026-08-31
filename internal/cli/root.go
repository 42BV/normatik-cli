// Package cli wires the cobra command tree. Discoverability features:
//   - auto --help on every (sub)command, with examples
//   - did-you-mean suggestions for mistyped commands (cobra built-in)
//   - global --output table|json with a stable error envelope
//   - stable, documented exit codes (see `normatik explain exit-codes`)
//
// Shared machinery (deps, handled errors, error-render, resource registry) lives
// in internal/command; this package only wires commands together.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/42BV/normatik-cli/internal/command"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

const developmentVersion = "0.0.0-dev"

// version is normally set at build time (-ldflags) and surfaced via fang's
// --version. `go install module@vX.Y.Z` cannot provide linker flags, but its
// binary embeds the module version in BuildInfo; use that version only while
// the linked value is still the development fallback.
var version = developmentVersion

func init() {
	info, ok := debug.ReadBuildInfo()
	version = resolvedVersion(version, info, ok)
}

func resolvedVersion(linked string, info *debug.BuildInfo, buildInfoAvailable bool) string {
	if linked != developmentVersion || !buildInfoAvailable || info == nil {
		return linked
	}
	if moduleVersion := info.Main.Version; moduleVersion != "" && moduleVersion != "(devel)" {
		return moduleVersion
	}
	return linked
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "normatik",
		Short: "Normatik CLI — agent-discoverable client for the Normatik Public API",
		Long: "normatik is the command-line interface for the Normatik Public API (/public/v1).\n" +
			"Every error carries an errorCode + hint; use `normatik explain <code>` to recover.",
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
	}
	root.PersistentFlags().StringP("output", "o", "table", "output format: table|json")
	root.PersistentFlags().String("profile", "", "use a specific environment profile (otherwise the active profile)")
	root.PersistentFlags().Bool("no-banner", false, "hide the banner on --help")
	root.PersistentFlags().StringSlice("fields", nil, "select/order columns in table mode (e.g. --fields id,name)")
	root.PersistentFlags().BoolP("quiet", "q", false, "compact output (no header/pagination footer)")
	root.PersistentFlags().Bool("no-input", false, "non-interactive: destructive actions without --confirm fail with exit 2 (no prompt)")
	root.SuggestionsMinimumDistance = 2

	// Hand-written commands.
	root.AddCommand(newPagesCmd())
	root.AddCommand(newMacrosCmd())
	root.AddCommand(newReleaseNotesCmd())
	root.AddCommand(newWhoamiCmd())
	root.AddCommand(newSpecCmd())
	root.AddCommand(newExplainCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newBaseURLCmd())
	// Registry-driven resource commands (root stays constant as resources grow).
	for _, c := range command.Commands() {
		root.AddCommand(c)
	}
	annotateURLHelp(root)     // group-help discoverability for the local --url flag
	registerCompletions(root) // static shell-completion for known flag values
	return root
}

// Main builds and runs the root command and returns the process exit code.
// fang adds styled help, --version, completion and styled errors; we keep our
// own structured API-error rendering + exit-code mapping by passing a custom
// error handler that skips errors we already rendered ourselves.
func Main() int {
	root := newRoot()
	if shouldShowBanner(os.Args[1:]) {
		fmt.Fprint(os.Stdout, banner())
	}
	err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(version),
		fang.WithErrorHandler(errorHandler),
		fang.WithColorSchemeFunc(normatikColorScheme),
	)
	if err == nil {
		return 0
	}
	var he *command.HandledError
	if errors.As(err, &he) {
		return he.Code
	}
	// cobra usage/parse error (already styled by fang via DefaultErrorHandler)
	return 2
}

// errorHandler lets fang style genuine cobra errors (unknown command, bad flag),
// but stays silent for our HandledError — those were already printed by the
// command itself as a structured error/hint envelope.
func errorHandler(w io.Writer, styles fang.Styles, err error) {
	var he *command.HandledError
	if errors.As(err, &he) {
		return
	}
	fang.DefaultErrorHandler(w, styles, err)
}
