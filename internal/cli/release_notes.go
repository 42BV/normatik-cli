package cli

import (
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

// newReleaseNotesCmd wires the read-only `release-notes` group: `list` pages the
// published versions (fixed newest-first order) and `show <version>` prints one
// note's Markdown body. Neither leaf has a frontend route, so no --url flag.
func newReleaseNotesCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "release-notes",
		Short: "Release notes (list, show)",
		Long: "Release notes served by the Normatik Public API. `list` pages the published " +
			"versions (newest first); `show <version>` prints one release note's body.",
		RunE: command.UnknownSub,
	}
	c.AddCommand(newReleaseNotesListCmd(), newReleaseNotesShowCmd())
	return c
}

func newReleaseNotesListCmd() *cobra.Command {
	// Only page/size (like `pages search`): the backend fixes a semver-desc order
	// and 400s on a sort param, so exposing --sort would only invite INVALID_SORT.
	var page, size int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List release-note versions (newest first, paginated)",
		Example: "  normatik release-notes list\n" +
			"  normatik release-notes list --size 5\n" +
			"  normatik release-notes list --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, "normatik release-notes list", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListReleaseNotes(cmd.Context(), page, size)
			}, "version", "date")
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (one-based)")
	cmd.Flags().IntVar(&size, "size", 10, "items per page")
	return cmd
}

func newReleaseNotesShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <version>",
		Short: "Show one release note's body (Markdown rendered to ASCII)",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik release-notes show 1.2.0\n" +
			"  normatik release-notes show 1.2.0 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.GetReleaseNoteByVersion(cmd.Context(), args[0])
			if apiErr != nil {
				// The standard ProblemDetail render surfaces the 404
				// RELEASE_NOTE_NOT_FOUND hint from the catalog — no special-casing.
				return command.RenderError(d.Printer, apiErr, "normatik release-notes show "+args[0])
			}
			d.Printer.ReleaseNote(body)
			return nil
		},
	}
	return cmd
}
