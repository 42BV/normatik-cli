package cli

import (
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

// definitionSpaceRequiredDetail and -Hint are the error of a work item type lookup without --space.
//
// A work item type slug is only unique within the global types plus the local types of one space, so every
// slug lookup names its space. Unlike the page scope contract (spacescope.go) this has no module-off default
// and no global fallback: also a global type, a single space or the Spaces module being off need --space.
const (
	definitionSpaceRequiredDetail = "--space is required: a work item type slug is only unique within the global types plus the local types of one space."
	definitionSpaceRequiredHint   = "Specify --space <key>, also for a global type. Use normatik spaces list to find available spaces."
)

// addDefinitionSpaceFlag adds the mandatory --space of a slug lookup to cmd.
func addDefinitionSpaceFlag(cmd *cobra.Command) {
	cmd.Flags().String(spaceFlagName, "", "space key (required): the global work item type or the one local to this space")
}

// requireDefinitionSpace returns the lowercase --space key of a slug lookup. Without it the command fails with a
// usage error (exit 2) before any request is made, so nothing is read or mutated.
func requireDefinitionSpace(cmd *cobra.Command, d *command.Deps) (string, error) {
	space := spaceOf(cmd)
	if space == "" {
		d.Printer.LocalError("USAGE", definitionSpaceRequiredDetail, definitionSpaceRequiredHint, 2)
		return "", command.Handled(2)
	}
	return space, nil
}
