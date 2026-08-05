package cli

import (
	"encoding/json"

	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

func newMacrosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "macros",
		Short: "Content macros: catalogue + docs + usage (list, docs, usage, scan)",
		Long: "Work with content macros. `list` shows the available macro catalogue; " +
			"`docs [<name>]` shows per-macro documentation (summary, attributes, defaults, examples) live from the backend; " +
			"`usage <pageId>` lists the macros a page uses; " +
			"`scan <macroName>` finds the pages that use a macro (admin key).",
		RunE: command.UnknownSub,
	}
	cmd.AddCommand(newMacrosListCmd())
	cmd.AddCommand(newMacrosDocsCmd())
	cmd.AddCommand(newMacrosUsageCmd())
	cmd.AddCommand(newMacrosScanCmd())
	return cmd
}

func newMacrosListCmd() *cobra.Command {
	var context string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the available content macros (--context PAGE_CONTENT|PAGE_PROPERTY|LANDING_PAGE|TASK_DESCRIPTION)",
		Example: "  normatik macros list\n  normatik macros list --context PAGE_CONTENT",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, "normatik macros list", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListContentMacros(cmd.Context(), context)
			}, "directiveName", "form", "module")
		},
	}
	cmd.Flags().StringVar(&context, "context", "", "render-context filter")
	return cmd
}

func newMacrosScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scan <macroName>",
		Short:   "Find pages that use a content macro (requires an admin API key)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik macros scan children\n  normatik macros scan table --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.ScanMacroUsage(cmd.Context(), args[0])
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik macros scan")
			}
			if command.PrintURL(d, cmd, weburl.AdminMacroScan()) {
				return nil
			}
			d.Printer.MacroScan(body)
			return nil
		},
	}
	command.URLFlag(cmd)
	return cmd
}

func newMacrosUsageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "usage <pageId>",
		Short:   "List the content macros a page uses (with counts)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik macros usage 5\n  normatik macros usage 5 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			id, perr := command.ParseID(args[0])
			if perr != nil {
				d.Printer.Message("Error [USAGE]: <pageId> must be a number, got %q", args[0])
				return command.Handled(2)
			}
			// Thin page GET (no expands) — content is enough to scan directives.
			body, apiErr := d.Client.GetPage(cmd.Context(), id, nil)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik macros usage")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			var page struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal(body, &page)
			counts := render.ScanDirectives(page.Content)
			out, _ := json.Marshal(counts)
			d.Printer.List(out, "directive", "count")
			return nil
		},
	}
	command.URLFlag(cmd)
	return cmd
}
