package cli

import (
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

func newSpecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spec",
		Short: "Download the live public-API OpenAPI document (GET /v3/api-docs/public-api)",
		Long: "Fetches the live Normatik Public API OpenAPI document from the configured " +
			"environment and prints it as JSON on stdout. This is the machine-readable " +
			"contract for agents. The path sits outside /public/v1.",
		Example: "  normatik spec\n  normatik spec --output json | jq '.paths | keys'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.GetPublicApiSpec(cmd.Context())
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik spec")
			}
			d.Printer.JSONDocument(body)
			return nil
		},
	}
}
