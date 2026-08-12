package cli

import (
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/spf13/cobra"
)

// cascadeImpactFields are the status + published counts from
// CascadeImpactPreviewResult — the preview machine-contract surface.
// hiddenAffectedPageCount is the access-masked subset of the geraakte set (B9).
var cascadeImpactFields = []string{
	"totalCount", "activeCount", "archivedCount", "trashedCount", "publishedCount",
	"hiddenAffectedPageCount",
}

// ackPublishedFlag registers --acknowledge-published on cascade-archive/trash.
func ackPublishedFlag(cmd *cobra.Command, dest *bool) {
	cmd.Flags().BoolVar(dest, "acknowledge-published", false,
		"confirm published pages in the cascade may go offline (required by the server when publishedCount > 0)")
}

// includeSubtreeFlag registers --include-subtree (default true) for admin
// cascade-restore / cascade-unarchive and their previews.
func includeSubtreeFlag(cmd *cobra.Command, dest *bool) {
	cmd.Flags().BoolVar(dest, "include-subtree", true,
		"include cancelled descendants (default true)")
}

// addPagesCascadeWrites attaches cascade-archive and cascade-trash under pages.
func addPagesCascadeWrites(parent *cobra.Command) {
	var reason string
	var ackPublished bool
	archive := &cobra.Command{
		Use:   "cascade-archive <id>",
		Short: "Cascade-archive a page and lower-level descendants",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik pages cascade-archive 42 --reason \"Retentie\" --confirm\n" +
			"  normatik pages cascade-archive 42 --reason \"Retentie\" --confirm --acknowledge-published",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			if strings.TrimSpace(reason) == "" {
				output, _ := cmd.Flags().GetString("output")
				render.New(output).Message("Error [USAGE]: --reason must not be empty; cascade-archive requires a reason.")
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			f := api.CascadeArchiveForm{Reason: reason}
			if ackPublished {
				f.AcknowledgePublished = boolPtr(true)
			}
			return runWrite(cmd, "normatik pages cascade-archive", "Cascade archive completed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CascadeArchivePage(cmd.Context(), id, f)
			}, "correlationId")
		},
	}
	archive.Flags().StringVar(&reason, "reason", "", "reason for archiving (required, non-empty; applied to every page in the geraakte set)")
	_ = archive.MarkFlagRequired("reason")
	addSoftConfirm(archive)
	ackPublishedFlag(archive, &ackPublished)

	var trashAck bool
	trash := &cobra.Command{
		Use:   "cascade-trash <id>",
		Short: "Cascade-trash a page and lower-level descendants",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik pages cascade-trash 42 --confirm\n" +
			"  normatik pages cascade-trash 42 --confirm --acknowledge-published",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			f := api.CascadeTrashForm{}
			if trashAck {
				f.AcknowledgePublished = boolPtr(true)
			}
			return runWrite(cmd, "normatik pages cascade-trash", "Cascade trash completed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CascadeTrashPage(cmd.Context(), id, f)
			}, "correlationId")
		},
	}
	addSoftConfirm(trash)
	ackPublishedFlag(trash, &trashAck)

	addWriteCommands(parent, archive, trash)
}

// newPagesCascadeImpactCmd is the GET impact-preview for cascade-archive/trash.
func newPagesCascadeImpactCmd() *cobra.Command {
	var operation string
	cmd := &cobra.Command{
		Use:   "cascade-impact <id>",
		Short: "Impact preview for cascade-archive or cascade-trash",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik pages cascade-impact 42 --operation ARCHIVE\n" +
			"  normatik pages cascade-impact 42 --operation TRASH --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			op := strings.ToUpper(strings.TrimSpace(operation))
			if op != "ARCHIVE" && op != "TRASH" {
				output, _ := cmd.Flags().GetString("output")
				render.New(output).Message("Error [USAGE]: --operation must be ARCHIVE or TRASH.")
				return command.Handled(2)
			}
			return runObject(cmd, "normatik pages cascade-impact", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.PreviewPageCascadeImpact(cmd.Context(), id, op)
			}, cascadeImpactFields...)
		},
	}
	cmd.Flags().StringVar(&operation, "operation", "", "cascade operation to preview: ARCHIVE or TRASH (required)")
	_ = cmd.MarkFlagRequired("operation")
	return cmd
}

// addArchiveCascadeWrites attaches cascade-unarchive under archive.
func addArchiveCascadeWrites(c *cobra.Command) {
	var includeSubtree bool
	unarchive := &cobra.Command{
		Use:     "cascade-unarchive <pageId>",
		Short:   "Cascade-unarchive a page and optionally its cancelled descendants (admin)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik archive cascade-unarchive 42 --confirm\n  normatik archive cascade-unarchive 42 --confirm --include-subtree=false",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			f := api.CascadeSubtreeForm{IncludeSubtree: boolPtr(includeSubtree)}
			return runWrite(cmd, "normatik archive cascade-unarchive", "Cascade unarchive completed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CascadeUnarchivePage(cmd.Context(), id, f)
			}, "correlationId")
		},
	}
	addSoftConfirm(unarchive)
	includeSubtreeFlag(unarchive, &includeSubtree)
	addWriteCommands(c, unarchive)
}

// newArchiveCascadeImpactCmd is the GET impact-preview for cascade-unarchive.
func newArchiveCascadeImpactCmd() *cobra.Command {
	var includeSubtree bool
	cmd := &cobra.Command{
		Use:     "cascade-impact <pageId>",
		Short:   "Impact preview for cascade-unarchive (admin)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik archive cascade-impact 42\n  normatik archive cascade-impact 42 --include-subtree=false",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			return runObject(cmd, "normatik archive cascade-impact", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.PreviewAdminArchiveCascadeImpact(cmd.Context(), id, "UNARCHIVE", boolPtr(includeSubtree))
			}, cascadeImpactFields...)
		},
	}
	includeSubtreeFlag(cmd, &includeSubtree)
	return cmd
}

// addTrashCascadeWrites attaches cascade-restore and cascade-permanent-delete under trash.
func addTrashCascadeWrites(c *cobra.Command) {
	var includeSubtree bool
	var cascadeRestoreReason string
	restore := &cobra.Command{
		Use:   "cascade-restore <pageId>",
		Short: "Cascade-restore a page from trash and optionally its cancelled descendants (admin)",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik trash cascade-restore 42 --confirm\n" +
			"  normatik trash cascade-restore 42 --confirm --include-subtree=false\n" +
			"  normatik trash cascade-restore 42 --confirm --reason \"Nieuwe reden\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			f := api.CascadeRestoreForm{IncludeSubtree: boolPtr(includeSubtree)}
			if strings.TrimSpace(cascadeRestoreReason) != "" {
				r := cascadeRestoreReason
				f.Reason = &r
			}
			return runWrite(cmd, "normatik trash cascade-restore", "Cascade restore completed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CascadeRestoreFromTrash(cmd.Context(), id, f)
			}, "correlationId")
		},
	}
	addSoftConfirm(restore)
	includeSubtreeFlag(restore, &includeSubtree)
	restore.Flags().StringVar(&cascadeRestoreReason, "reason", "", "reason when restoring to archive (required by server if parent is archived)")

	perm := &cobra.Command{
		Use:     "cascade-permanent-delete <pageId>",
		Short:   "Cascade permanent-delete a trash root and its subtree (admin, irreversible)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik trash cascade-permanent-delete 42 --confirm=42",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmHard(cmd, d, args[0]); e != nil {
				return e
			}
			return runWrite(cmd, "normatik trash cascade-permanent-delete", "Cascade permanent-delete completed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CascadePermanentDeleteFromTrash(cmd.Context(), id)
			}, "correlationId")
		},
	}
	addHardConfirm(perm)

	addWriteCommands(c, restore, perm)
}

// newTrashCascadeImpactCmd is the GET impact-preview for cascade-restore / permanent-delete.
func newTrashCascadeImpactCmd() *cobra.Command {
	var operation string
	var includeSubtree bool
	cmd := &cobra.Command{
		Use:   "cascade-impact <pageId>",
		Short: "Impact preview for cascade-restore or cascade-permanent-delete (admin)",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik trash cascade-impact 42 --operation RESTORE\n" +
			"  normatik trash cascade-impact 42 --operation PERMANENT_DELETE",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			op := strings.ToUpper(strings.TrimSpace(operation))
			if op != "RESTORE" && op != "PERMANENT_DELETE" {
				output, _ := cmd.Flags().GetString("output")
				render.New(output).Message("Error [USAGE]: --operation must be RESTORE or PERMANENT_DELETE.")
				return command.Handled(2)
			}
			return runObject(cmd, "normatik trash cascade-impact", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.PreviewAdminTrashCascadeImpact(cmd.Context(), id, op, boolPtr(includeSubtree))
			}, cascadeImpactFields...)
		},
	}
	cmd.Flags().StringVar(&operation, "operation", "", "cascade operation to preview: RESTORE or PERMANENT_DELETE (required)")
	_ = cmd.MarkFlagRequired("operation")
	includeSubtreeFlag(cmd, &includeSubtree)
	return cmd
}
