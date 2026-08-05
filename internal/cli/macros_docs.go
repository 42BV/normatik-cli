package cli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

func newMacrosDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs [<name>]",
		Short: "Per-macro documentation (summary, attributes, defaults, examples) — live from the backend",
		Long: "Shows the content-macro documentation the backend serves live — the knowledge base " +
			"for writing directives in page content. Without an argument it prints the full knowledge " +
			"base: the preamble on the directive forms, the shared filter syntax and a table of every " +
			"enabled macro. With a name it prints that macro's documentation: summary, examples and an " +
			"attribute table with types, defaults and docs. Discovery path: `normatik macros docs` to " +
			"find a macro, then `normatik macros docs <name>` for its details.",
		Args: cobra.MaximumNArgs(1),
		Example: "  normatik macros docs\n" +
			"  normatik macros docs toc\n" +
			"  normatik macros docs toc --output json",
		ValidArgsFunction: completeMacroDocNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				body, apiErr := d.Client.GetContentMacroDocs(cmd.Context())
				if apiErr != nil {
					return command.RenderError(d.Printer, apiErr, "normatik macros docs")
				}
				d.Printer.MacroDocs(body)
				return nil
			}
			body, apiErr := d.Client.GetContentMacroDocsByName(cmd.Context(), args[0])
			if apiErr != nil {
				// The invocation carries the requested name so the
				// CONTENT_MACRO_NOT_FOUND synth can build a did-you-mean
				// against the validNames the backend returns.
				return command.RenderError(d.Printer, apiErr, "normatik macros docs "+args[0])
			}
			d.Printer.MacroDoc(body)
			return nil
		},
	}
}

// completeMacroDocNames best-effort completes <name> from the live docs
// collection. Completion must never surface an error: any failure (no config,
// no key, backend down, unexpected body) yields no suggestions. It deliberately
// avoids command.Build, which prints config errors — noise a completion
// context must not produce.
func completeMacroDocNames(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	_, res, err := command.Resolve(cmd)
	if err != nil || res.APIKey == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	c, cerr := client.New(res.BaseURL, res.APIKey)
	if cerr != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// Completion must fail silently AND fast: without this deadline a black-holed
	// host would freeze the user's shell for the generic 120s client timeout.
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
	defer cancel()
	body, apiErr := c.GetContentMacroDocs(ctx)
	if apiErr != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var docs struct {
		Macros []struct {
			DirectiveName string `json:"directiveName"`
		} `json:"macros"`
	}
	if json.Unmarshal(body, &docs) != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(docs.Macros))
	for _, m := range docs.Macros {
		if m.DirectiveName != "" {
			names = append(names, m.DirectiveName)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
