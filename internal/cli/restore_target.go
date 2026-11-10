package cli

import (
	"fmt"
	"strings"

	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

// The admin restore routes (trash restore/cascade-restore, archive
// restore/cascade-unarchive) bring a page back to its ORIGINAL space. Only when
// that space was deleted does the server accept an explicit destination; while
// it exists every target field is refused with RESTORE_TARGET_SPACE_NOT_ALLOWED,
// also the same space. The CLI forwards what it is given and never fills a target
// in itself, so a flag can not turn a restore into a move.

type restoreTargetFlags struct {
	space  string
	parent int64
	root   bool
}

func addRestoreTargetFlags(cmd *cobra.Command, t *restoreTargetFlags) {
	cmd.Flags().StringVar(&t.space, "target-space", "", "key of the space to restore into; only accepted while the original space was deleted (then with --target-parent or --target-root)")
	cmd.Flags().Int64Var(&t.parent, "target-parent", 0, "id of the page to restore under in --target-space")
	cmd.Flags().BoolVar(&t.root, "target-root", false, "restore as a root page of --target-space")
}

// resolveRestoreTarget turns the --target-* flags into the request target. An
// unknown key is refused here; every other rule (original space still exists,
// parent in the target space, rights) is the server's.
func resolveRestoreTarget(cmd *cobra.Command, d *command.Deps, t *restoreTargetFlags) (client.RestoreTarget, error) {
	var target client.RestoreTarget
	if cmd.Flags().Changed("target-parent") {
		target.ParentID = i64Ptr(t.parent)
	}
	if cmd.Flags().Changed("target-root") {
		target.Root = boolPtr(t.root)
	}
	if strings.TrimSpace(t.space) == "" {
		return target, nil
	}
	sp, ok, apiErr := d.Client.ResolveSpace(cmd.Context(), t.space, true)
	if apiErr != nil {
		return target, command.RenderError(d.Printer, apiErr, cmd.CommandPath())
	}
	if !ok {
		return target, spaceNotFound(d, t.space)
	}
	target.SpaceID = i64Ptr(sp.Id)
	return target, nil
}

// spaceNotFound renders the not-found of a space key and returns its exit code.
func spaceNotFound(d *command.Deps, key string) error {
	d.Printer.LocalError("SPACE_NOT_FOUND",
		fmt.Sprintf("no space with key %q.", strings.ToLower(strings.TrimSpace(key))),
		"Use normatik spaces list to find available spaces.", 6)
	return command.Handled(6)
}
