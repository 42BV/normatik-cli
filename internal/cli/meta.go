package cli

import (
	"github.com/42BV/normatik-cli/internal/catalog"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Short:   "Show the owner of the active API key (GET /users/me)",
		Example: "  normatik whoami\n  normatik whoami --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.Me(cmd.Context())
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik whoami")
			}
			d.Printer.Raw(body, "id", "displayName", "email", "role", "workflowRole", "accessMode")
			return nil
		},
	}
}

func newExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain [error-code|exit-codes]",
		Short: "Explain an errorCode or the exit-code table (offline, full catalog)",
		Long: "Shows title, cause and recovery for an errorCode from the bundled catalog " +
			"(generated from ErrorCode.java + PublicApiHints.java — all codes, offline). " +
			"`explain` without an argument lists all codes; `explain exit-codes` shows the exit-code table.",
		Example: "  normatik explain INVALID_SORT\n" +
			"  normatik explain exit-codes\n" +
			"  normatik explain",
		// Complete error-code names (and the literal exit-codes) as the argument.
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return append([]string{"exit-codes"}, catalog.Names()...), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cmd.Printf("Known error codes (%d, offline catalog):\n", len(catalog.List()))
				for _, e := range catalog.List() {
					cmd.Printf("  %-40s %s\n", e.Name, e.Title)
				}
				cmd.Println("\nUsage: normatik explain <error-code>  ·  normatik explain exit-codes")
				return nil
			}
			arg := args[0]
			if arg == "exit-codes" {
				cmd.Println("Exit codes (agents branch on exit code first, then on errorCode):")
				for _, r := range catalog.ExitClasses() {
					cmd.Printf("  %-3d %s\n", r.Code, r.Meaning)
				}
				return nil
			}
			e, ok := catalog.Lookup(arg)
			if !ok {
				cmd.PrintErrf("Unknown code %q.\n", arg)
				if g := catalog.Closest(arg); g != "" {
					cmd.PrintErrf("  Did you mean %q?\n", g)
				}
				cmd.PrintErrln("  List all codes: normatik explain")
				return command.Handled(2)
			}
			cmd.Printf("%s — %s  (HTTP %d, exit %d)\n", e.Name, e.Title, e.Status, catalog.ExitFor(e.Name, e.Status))
			cmd.Printf("  Cause:    %s\n", e.UserMessage)
			switch e.HintKind {
			case "STATIC":
				cmd.Printf("  Recovery: %s\n", e.StaticHint)
			case "DYNAMIC":
				cmd.Printf("  Recovery: The error response carries structured hint fields specific to this error — read them and correct.\n")
			default:
				cmd.Printf("  Recovery: Read the detail field from the error response; this code provides no fixed hint.\n")
			}
			return nil
		},
	}
	return cmd
}
