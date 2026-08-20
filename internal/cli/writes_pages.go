package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/localfile"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

// ---- pages writes ----

func addPagesWrites(parent *cobra.Command) {
	var name, content, file, timezone string
	var version int64
	var dryRun bool
	var props, unsets []string
	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a page (--name/--content, --property, -f for propertyValues, --dry-run)",
		Example: "  normatik pages update 7 --name \"New title\"\n" +
			"  normatik pages update 7 --property \"Status=Approved\" --property \"Owner=email:a@b.nl\"\n" +
			"      # Owner is a USER_LIST property: pass email:<addr> for an account or ext:<name> for an external user (there is no separate 'user' type)\n" +
			"  normatik pages update 7 --unset-property \"Owner\"   # clears the stored value\n" +
			"  normatik pages update 7 -f page.json --property X=1  # flags layer over the form",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if err := command.CheckURLDryRun(d, cmd); err != nil {
				return err
			}
			var f api.PageEditForm
			if file != "" {
				loaded, lerr := loadForm[api.PageEditForm](file)
				if lerr != nil {
					d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
					return command.Handled(2)
				}
				f = loaded
			}
			if cmd.Flags().Changed("name") {
				f.Name = strPtr(name)
			}
			if cmd.Flags().Changed("content") {
				f.Content = strPtr(content)
			}
			if cmd.Flags().Changed("version") {
				f.Version = i64Ptr(version)
			}
			hasProps := len(props) > 0 || len(unsets) > 0
			hasFormProps := len(derefEditValues(f.PropertyValues)) > 0
			if dryRun {
				// Combination rule: with --content we validate the markdown first
				// (unchanged /content/validate behaviour); when there are property
				// values — from --property flags OR from a -f form — we print the
				// payload that would be sent. Never a write.
				if f.Content == nil && !hasProps && !hasFormProps {
					d.Printer.Message("Error [USAGE]: --dry-run needs --content/-f content and/or --property/-f propertyValues to preview")
					return command.Handled(2)
				}
				if f.Content != nil {
					if err := validateContent(cmd, d, *f.Content, i64Ptr(id), nil, false); err != nil {
						return err
					}
				}
				if hasProps {
					// Property flags route via PATCH — preview the resolved PagePatchForm.
					return previewPageProperties(cmd, d, id, f, props, unsets, timezone)
				}
				if hasFormProps {
					// A -f form (no flags) routes via PUT: preview the PageEditForm
					// itself. The optimistic-lock version is auto-filled from the page
					// at send when absent, so it may not appear in this preview.
					d.Printer.DryRun(f)
				}
				return nil
			}
			// A real update must change something: a --name/--content flag, property
			// flags, or a -f form. Without any, the PATCH reroute would send an empty
			// delta that the backend still materializes as a no-op carry-forward
			// revision (version churn) — refuse it up front. (Before the PATCH reroute
			// this hit a null-version PUT and 409'd by accident.)
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("content") && !hasProps && file == "" {
				d.Printer.Message("Error [USAGE]: nothing to update — pass --name/--content, --property/--unset-property, or -f <form>")
				return command.Handled(2)
			}
			// Route selection (both paths auto-fill the optimistic-lock version):
			// a -f form without property flags is a deliberate full-form PUT
			// replace; everything else — property flags OR a plain --name/--content
			// change — routes via the non-destructive PATCH path. PATCH backfills
			// untouched name/content and merges property values, whereas a
			// full-replace PUT silently wipes the omitted fields
			// (createRevisionForPage does no backfill), so a partial flag update
			// must never take the PUT path.
			if file != "" && !hasProps {
				return updatePagePut(cmd, d, id, f)
			}
			return updatePageProperties(cmd, d, id, f, props, unsets, timezone)
		},
	}
	update.Flags().StringVar(&name, "name", "", "new name")
	update.Flags().StringVar(&content, "content", "", "new markdown content")
	update.Flags().Int64Var(&version, "version", 0, "expected version (optimistic locking)")
	update.Flags().StringVarP(&file, "file", "f", "", "JSON file with PageEditForm (incl. propertyValues); full replace via PUT — omitted fields are cleared. Use --property for a non-destructive merge")
	update.Flags().StringArrayVar(&props, "property", nil, "set a property: \"name=value\" (repeatable; routes via PATCH; see `pages describe-properties`)")
	update.Flags().StringArrayVar(&unsets, "unset-property", nil, "clear a property by name (repeatable; routes via PATCH)")
	update.Flags().StringVar(&timezone, "timezone", "", "IANA zone for DATE_TIME properties (default: host local zone)")
	update.Flags().BoolVar(&dryRun, "dry-run", false, "preview without saving: validate --content via /content/validate and/or print the resolved --property PATCH payload")
	command.URLFlag(update)

	del := idWrite("delete <id>", "Delete a page (move to trash)", "normatik pages delete", "Page deleted (moved to trash).", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeletePage(ctx, id)
		})

	var reason string
	archive := &cobra.Command{
		Use: "archive <id>", Short: "Archive a page (--reason required)", Args: cobra.ExactArgs(1),
		Example: "  normatik pages archive 42 --reason \"Retentie verlopen\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			// MarkFlagRequired only rejects an ABSENT --reason; a blank or
			// whitespace-only value must fail here, before any request is sent
			// (ArchivePageForm.reason has minLength 1). Rendered via render.New
			// rather than command.Build so a missing base-URL/credential cannot
			// turn this usage error into an exit-78 auth error.
			if strings.TrimSpace(reason) == "" {
				output, _ := cmd.Flags().GetString("output")
				render.New(output).Message("Error [USAGE]: --reason must not be empty; archiving a page requires a reason.")
				return command.Handled(2)
			}
			f := api.ArchivePageForm{Reason: reason}
			return runWrite(cmd, "normatik pages archive", "Page archived.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ArchivePage(cmd.Context(), id, f)
			})
		},
	}
	archive.Flags().StringVar(&reason, "reason", "", "reason for archiving (required, non-empty)")
	_ = archive.MarkFlagRequired("reason")

	var parentID int64
	move := &cobra.Command{
		Use: "move <id>", Short: "Move a page (--parent or --root; omit = root)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.PageMoveForm{}
			if cmd.Flags().Changed("parent") {
				f.ParentId = i64Ptr(parentID)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.MovePage(cmd.Context(), id, f)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages move")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			writeResult(d, body, "Page moved.")
			return nil
		},
	}
	move.Flags().Int64Var(&parentID, "parent", 0, "new parent page id (omit for root)")
	move.Flags().Bool("root", false, "move to root (exclusive with --parent)")
	move.MarkFlagsMutuallyExclusive("root", "parent")
	command.URLFlag(move)

	sortCh := &cobra.Command{
		Use: "sort-children <parentId> <childId...>", Short: "Sort the children of a page", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			ids, ok := variadicIDs(args[1:])
			if !ok {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.SortChildren(cmd.Context(), pid, ids)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages sort-children")
			}
			if command.PrintURL(d, cmd, weburl.PageSortChildren(pid)) {
				return nil
			}
			writeResult(d, body, "Children sorted.")
			return nil
		},
	}
	command.URLFlag(sortCh)
	addWriteCommands(parent, update, del, archive, move, sortCh)
}

// buildPropertyPatch resolves --property / --unset-property against the page's
// live metadata (a GET page response — availablePropertyDescriptors + version —
// never a write) into a PagePatchForm ready to send. Shared by the real PATCH
// path and the --dry-run preview. On a client-side validation failure it renders
// the error and returns the carried exit code.
func buildPropertyPatch(cmd *cobra.Command, d *command.Deps, id int64, f api.PageEditForm, props, unsets []string, timezone string) (api.PagePatchForm, *api.PublicPageCompositeResult, error) {
	page, apiErr := fetchPage(cmd.Context(), d.Client, id)
	if apiErr != nil {
		return api.PagePatchForm{}, nil, command.RenderError(d.Printer, apiErr, updateInvocation(id))
	}
	tz := timezone
	if tz == "" {
		tz = localZoneName()
	}
	pd := propertyDispatcher{lookup: newCachingLookup(newClientLookup(d.Client)), timezone: tz}
	values, unsetIDs, pferr := applyPropertyFlagsPatch(
		cmd.Context(), derefEditValues(f.PropertyValues), props, unsets,
		derefDescriptors(page.AvailablePropertyDescriptors), pd)
	if pferr != nil {
		return api.PagePatchForm{}, nil, renderPropError(d, pferr, updateInvocation(id))
	}

	patch := api.PagePatchForm{Name: f.Name, Content: f.Content}
	if len(values) > 0 {
		patch.PropertyValues = &values
	}
	if len(unsetIDs) > 0 {
		patch.UnsetPropertyDescriptorIds = &unsetIDs
	}
	patch.Version = patchVersion(f.Version, page)
	return patch, page, nil
}

// updatePagePut performs a deliberate full-form page update via PUT (the -f form
// path only; plain --name/--content flag updates route via the non-destructive
// PATCH path to avoid wiping omitted fields). When the caller passed no --version
// it auto-fills the optimistic-lock version the same way the PATCH path does
// (patchVersion): the backend PUT rejects a null version on a non-workflow page
// (assertVersionMatches) yet validates a workflow page against PageRevision.version,
// so a version-less PUT tripped a spurious 409 CONCURRENT_UPDATE. An explicit
// --version (or -f version) always wins and skips the extra GET. PUT is
// full-replace (no backfill): callers must supply the complete intended form.
func updatePagePut(cmd *cobra.Command, d *command.Deps, id int64, f api.PageEditForm) error {
	if f.Version == nil {
		page, apiErr := fetchPage(cmd.Context(), d.Client, id)
		if apiErr != nil {
			return command.RenderError(d.Printer, apiErr, updateInvocation(id))
		}
		f.Version = patchVersion(f.Version, page)
	}
	body, apiErr := d.Client.UpdatePage(cmd.Context(), id, f)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, updateInvocation(id))
	}
	if command.PrintURL(d, cmd, weburl.Page(id)) {
		return nil
	}
	writeResult(d, body, "Page updated.", "id", "name")
	return nil
}

// updateInvocation is the failing-invocation string for `pages update <id>`
// errors. It carries the id so the CONCURRENT_UPDATE recovery synth can emit a
// runnable `pages get <id>` next step (see internal/problem).
func updateInvocation(id int64) string {
	return fmt.Sprintf("normatik pages update %d", id)
}

// updatePageProperties performs a property-aware page update via PATCH, using the
// shared resolver.
func updatePageProperties(cmd *cobra.Command, d *command.Deps, id int64, f api.PageEditForm, props, unsets []string, timezone string) error {
	patch, page, err := buildPropertyPatch(cmd, d, id, f, props, unsets, timezone)
	if err != nil {
		return err
	}
	body, apiErr := d.Client.PatchPage(cmd.Context(), id, patch)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, updateInvocation(id))
	}
	if command.PrintURL(d, cmd, weburl.Page(id)) {
		return nil
	}
	writeResult(d, body, "Page updated.", "id", "name")
	// On a workflow page the PATCH lands on the working revision, so the default
	// (published) `pages get` read-back does NOT show it yet — point the caller at
	// the working-revision view. A non-workflow page shows the value immediately,
	// so we confirm symmetrically that the change is already live.
	if page != nil && page.WorkflowEnabled != nil && *page.WorkflowEnabled {
		d.Printer.Message("Saved to the working revision (not yet published). View: normatik pages get %d --working", id)
	} else {
		d.Printer.Message("Saved. The change is live (this page type has no publishing workflow). View: normatik pages get %d", id)
	}
	return nil
}

// previewPageProperties resolves the same PATCH as updatePageProperties but prints
// it instead of sending it (--dry-run). No write ever happens. In table mode it
// prefixes a human-readable "name: old -> new" diff; JSON mode stays byte-identical
// (only the DryRun payload on stdout, no diff), so `--dry-run -o json | jq` is clean.
func previewPageProperties(cmd *cobra.Command, d *command.Deps, id int64, f api.PageEditForm, props, unsets []string, timezone string) error {
	patch, page, err := buildPropertyPatch(cmd, d, id, f, props, unsets, timezone)
	if err != nil {
		return err
	}
	if d.Printer.Mode != render.JSON {
		printDryRunDiff(d, page, props, unsets)
	}
	d.Printer.DryRun(patch)
	return nil
}

// printDryRunDiff prints a human-readable "name: old -> new" preview of the
// --property / --unset-property changes (table mode only). "old" is the current
// display value read from the page; a PATCH layers onto the working revision, so
// working-revision values are preferred when present, else the published values.
func printDryRunDiff(d *command.Deps, page *api.PublicPageCompositeResult, props, unsets []string) {
	source := dryRunPropertySource(page)
	d.Printer.Message("Dry-run - would change:")
	for _, p := range props {
		name, value, _ := strings.Cut(p, "=")
		d.Printer.Message("  %s: %s -> %s", name, currentPropertyDisplayByName(source, name), value)
	}
	for _, name := range unsets {
		d.Printer.Message("  %s: %s -> (unset)", name, currentPropertyDisplayByName(source, name))
	}
}

// dryRunPropertySource picks the property values a PATCH would layer onto: the
// working revision when it carries values (a PATCH edits the working revision),
// else the published top-level values. Only used for the --dry-run diff.
func dryRunPropertySource(page *api.PublicPageCompositeResult) []api.PropertyValueResult {
	if page == nil {
		return nil
	}
	// A present working revision is the source the PATCH layers onto, even when its
	// property list is empty (e.g. after unsetting everything): falling back to the
	// published values there would show a stale "old" the PATCH does not touch. Only
	// an ABSENT working revision falls back to the published values.
	if wr := page.WorkingRevision; wr != nil {
		if wr.PropertyValues != nil {
			return *wr.PropertyValues
		}
		return nil
	}
	if page.PropertyValues != nil {
		return *page.PropertyValues
	}
	return nil
}

// currentPropertyDisplayByName finds the property value with the given name
// (case-insensitive) in source and renders its current display, or "-" when absent.
func currentPropertyDisplayByName(source []api.PropertyValueResult, name string) string {
	for _, pv := range source {
		if strings.EqualFold(derefStr(pv.PropertyDescriptorName), name) {
			return currentPropertyDisplay(pv)
		}
	}
	return "-"
}

// currentPropertyDisplay renders a property value's current human display for the
// --dry-run diff: the first non-empty text-ish field, then numeric, then linked
// page/user names, else "-".
func currentPropertyDisplay(pv api.PropertyValueResult) string {
	for _, s := range []*string{pv.TextValue, pv.EnumValueDisplay, pv.SelectedPageTypeName, pv.DateTimeValue} {
		if s != nil && *s != "" {
			return *s
		}
	}
	if pv.DateValue != nil {
		return pv.DateValue.String()
	}
	if pv.NumericValue != nil {
		return fmt.Sprintf("%v", *pv.NumericValue)
	}
	if names := linkedRefNames(pv); len(names) > 0 {
		return strings.Join(names, ", ")
	}
	return "-"
}

// linkedRefNames collects the display names of a property's linked pages or users
// (whichever is populated) for the --dry-run diff.
func linkedRefNames(pv api.PropertyValueResult) []string {
	var names []string
	if pv.PageReferences != nil {
		for _, ref := range *pv.PageReferences {
			if n := derefStr(ref.Name); n != "" {
				names = append(names, n)
			}
		}
	}
	if pv.UserLinks != nil {
		for _, ul := range *pv.UserLinks {
			if n := derefStr(ul.DisplayName); n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

// patchVersion picks the optimistic-locking version for a PATCH. An explicit
// version (from --version or -f) always wins. Otherwise we auto-send Page.version
// for a NON-workflow page (PATCH checks Page.version) and ONLY suppress it for a
// confirmed workflow page: workflow PATCH (editWorkingRevision) validates against
// PageRevision.version, not Page.version, so auto-sending Page.version there would
// cause spurious 409 CONCURRENT_UPDATE errors (the user can still pass --version
// with a known revision version). An unknown/absent workflowEnabled falls back to
// sending the version — a visible 409 is safer than silently dropping optimistic
// locking on a concurrent edit.
func patchVersion(explicit *int64, page *api.PublicPageCompositeResult) *int64 {
	if explicit != nil {
		return explicit
	}
	if page.WorkflowEnabled != nil && *page.WorkflowEnabled {
		return nil
	}
	return page.Version
}

// addRevisionsWrites extends the pages revisions subtree with workflow writes.
func addRevisionsWrites(rc *cobra.Command) {
	start := idWriteURL("start <id>", "Start a new STORED working revision", "normatik pages revisions start", "Revision started.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.StartRevision(ctx, id)
		}, weburl.Page)

	var to, comment string
	transition := &cobra.Command{
		Use: "transition <id>", Short: "Perform a workflow transition (--to STORED|IN_REVIEW|...)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.TransitionForm{TargetStatus: api.TransitionFormTargetStatus(to)}
			if cmd.Flags().Changed("comment") {
				f.Comment = strPtr(comment)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.PerformTransition(cmd.Context(), id, f)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages revisions transition")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			writeResult(d, body, "Transition performed.")
			return nil
		},
	}
	transition.Flags().StringVar(&to, "to", "", "target status (required): STORED|IN_REVIEW|APPROVED|PUBLISHED")
	transition.Flags().StringVar(&comment, "comment", "", "optional comment")
	_ = transition.MarkFlagRequired("to")
	command.URLFlag(transition)

	restore := &cobra.Command{
		Use: "restore <id> <revisionNumber>", Short: "Restore an earlier version as a new publication", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, e1 := command.ParseID(args[0])
			rev, e2 := command.ParseID(args[1])
			if e1 != nil || e2 != nil {
				return command.Handled(2)
			}
			if rev < 1 || rev > math.MaxInt32 {
				output, _ := cmd.Flags().GetString("output")
				render.New(output).Message(
					"Error [USAGE]: invalid revisionNumber %q; expected range 1..%d.",
					args[1], math.MaxInt32,
				)
				return command.Handled(2)
			}
			revisionNumber := int32(rev)
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.RestoreVersion(cmd.Context(), id, revisionNumber)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages revisions restore")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			writeResult(d, body, "Version restored.")
			return nil
		},
	}
	// Restore has no local flags. Stop flag parsing after the page id so a
	// negative revision reaches the explicit range guard instead of being
	// interpreted as an unknown shorthand flag by pflag.
	restore.Flags().SetInterspersed(false)
	command.URLFlag(restore)

	discard := idWriteURL("discard <id>", "Discard the working revision (reverts to published, or PERMANENTLY deletes a draft-only page)", "normatik pages revisions discard", "Working revision discarded.", "hard",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DiscardWorkingRevision(ctx, id)
		}, weburl.Page)

	addWriteCommands(rc, start, transition, restore, discard)
}

// newPagesRestrictionCmd builds the `pages restriction` subtree.
func newPagesRestrictionCmd() *cobra.Command {
	rc := &cobra.Command{Use: "restriction", Short: "Page restrictions (create, remove, access, group-access)", RunE: command.UnknownSub}
	create := idWriteURL("create <pageId>", "Create a restriction on a page", "normatik pages restriction create", "Restriction created.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.CreateRestriction(ctx, id)
		}, weburl.Page)
	remove := idWriteURL("remove <pageId>", "Remove the restriction from a page", "normatik pages restriction remove", "Restriction removed.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.RemoveRestriction(ctx, id)
		}, weburl.Page)
	addWriteCommands(rc, create, remove, newRestrictionAccessCmd(false), newRestrictionAccessCmd(true))
	return rc
}

func newRestrictionAccessCmd(group bool) *cobra.Command {
	use := "access"
	short := "User access on a restriction (add, update, remove)"
	if group {
		use = "group-access"
		short = "Group access on a restriction (add, update, remove)"
	}
	ac := &cobra.Command{Use: use, Short: short, RunE: command.UnknownSub}
	var subjectID int64
	var level string
	subjectFlag := "user-id"
	if group {
		subjectFlag = "group-id"
	}
	add := &cobra.Command{
		Use: "add <pageId>", Short: "Add access", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			var body []byte
			var apiErr *client.APIError
			if group {
				body, apiErr = d.Client.AddGroupAccess(cmd.Context(), pid, api.PageRestrictionGroupAccessForm{GroupId: subjectID, AccessLevel: api.PageRestrictionGroupAccessFormAccessLevel(level)})
			} else {
				body, apiErr = d.Client.AddAccess(cmd.Context(), pid, api.PageRestrictionAccessForm{UserId: subjectID, AccessLevel: api.PageRestrictionAccessFormAccessLevel(level)})
			}
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages restriction "+use+" add")
			}
			if command.PrintURL(d, cmd, weburl.Page(pid)) {
				return nil
			}
			writeResult(d, body, "Access added.")
			return nil
		},
	}
	add.Flags().Int64Var(&subjectID, subjectFlag, 0, "id of the "+map[bool]string{true: "group", false: "user"}[group]+" (required)")
	add.Flags().StringVar(&level, "level", "", "access level (required)")
	_ = add.MarkFlagRequired(subjectFlag)
	_ = add.MarkFlagRequired("level")
	command.URLFlag(add)

	upd := &cobra.Command{
		Use: "update <pageId> <accessId>", Short: "Update access level", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, e1 := command.ParseID(args[0])
			aid, e2 := command.ParseID(args[1])
			if e1 != nil || e2 != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			var body []byte
			var apiErr *client.APIError
			if group {
				body, apiErr = d.Client.UpdateGroupAccess(cmd.Context(), pid, aid, api.PageRestrictionGroupAccessForm{GroupId: subjectID, AccessLevel: api.PageRestrictionGroupAccessFormAccessLevel(level)})
			} else {
				body, apiErr = d.Client.UpdateAccess(cmd.Context(), pid, aid, api.PageRestrictionAccessForm{UserId: subjectID, AccessLevel: api.PageRestrictionAccessFormAccessLevel(level)})
			}
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages restriction "+use+" update")
			}
			if command.PrintURL(d, cmd, weburl.Page(pid)) {
				return nil
			}
			writeResult(d, body, "Access updated.")
			return nil
		},
	}
	upd.Flags().Int64Var(&subjectID, subjectFlag, 0, "id of the subject")
	upd.Flags().StringVar(&level, "level", "", "new access level (required)")
	_ = upd.MarkFlagRequired("level")
	command.URLFlag(upd)

	rem := &cobra.Command{
		Use: "remove <pageId> <accessId>", Short: "Remove access", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, e1 := command.ParseID(args[0])
			aid, e2 := command.ParseID(args[1])
			if e1 != nil || e2 != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			var body []byte
			var apiErr *client.APIError
			if group {
				body, apiErr = d.Client.RemoveGroupAccess(cmd.Context(), pid, aid)
			} else {
				body, apiErr = d.Client.RemoveAccess(cmd.Context(), pid, aid)
			}
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages restriction "+use+" remove")
			}
			if command.PrintURL(d, cmd, weburl.Page(pid)) {
				return nil
			}
			writeResult(d, body, "Access removed.")
			return nil
		},
	}
	addSoftConfirm(rem)
	command.URLFlag(rem)
	addWriteCommands(ac, add, upd, rem)
	return ac
}

// addTrashWrites extends the trash subtree with restore/purge and cascade writes.
func addTrashWrites(c *cobra.Command) {
	var restoreReason string
	restore := &cobra.Command{
		Use:   "restore <pageId>",
		Short: "Restore a page from the trash (optional --reason when parent is archived)",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik trash restore 42\n" +
			"  normatik trash restore 42 --reason \"Nieuwe reden\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik trash restore", "Page restored.",
				func(d *command.Deps) ([]byte, *client.APIError) {
					return d.Client.RestoreFromTrash(cmd.Context(), id, restoreReason)
				},
				func([]byte) string { return weburl.Page(id) },
			)
		},
	}
	restore.Flags().StringVar(&restoreReason, "reason", "", "reason when restoring to archive (required by server if parent is archived)")
	command.URLFlag(restore)
	// Purge points at the bare trash list route (mapping table: "trash
	// list / purge" -> /admin/trash), the same pattern as users delete.
	purge := idWriteURL("purge <pageId>", "Delete a page PERMANENTLY from the trash (irreversible)", "normatik trash purge", "Page permanently deleted.", "hard",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.PurgeFromTrash(ctx, id)
		}, func(int64) string { return weburl.AdminTrash() })
	addWriteCommands(c, restore, purge)
	addTrashCascadeWrites(c)
}

// addArchiveWrites extends the archive subtree with restore, delete and cascade-unarchive.
// Restore maps to server unarchive; delete moves the page to trash (soft).
func addArchiveWrites(c *cobra.Command) {
	restore := idWriteURL("restore <pageId>", "Restore an archived page (unarchive)", "normatik archive restore", "Page restored from the archive.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.UnarchivePage(ctx, id)
		}, weburl.Page)
	// No confirm flag: machine-call convention for admin archive→trash (same as
	// archive restore). Points --url at the trash list after the move.
	del := idWriteURL("delete <pageId>", "Move an archived page to the trash", "normatik archive delete", "Page moved from the archive to the trash.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeleteArchivedPage(ctx, id)
		}, func(int64) string { return weburl.AdminTrash() })
	addWriteCommands(c, restore, del)
	addArchiveCascadeWrites(c)
}

// newContentCmd builds the `content validate` command (READ_ONLY-safe dry-run).
func newContentCmd() *cobra.Command {
	c := &cobra.Command{Use: "content", Short: "Content tools (validate)", RunE: command.UnknownSub}
	var content, file string
	var targetPageID, targetPageTypeID int64
	validate := &cobra.Command{
		Use: "validate", Short: "Validate markdown content (dry-run, READ_ONLY-safe)",
		Example: "  normatik content validate --content '::children{depth=1}' --target-page-type-id 199",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body := content
			if file != "" {
				// NORMATIK-21: no-follow, regular-file-only, size-bounded read via localfile.
				data, ferr := localfile.ReadBounded(file, maxFormFileBytes)
				if ferr != nil {
					d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, ferr)
					return command.Handled(2)
				}
				body = string(data)
			}
			if body == "" {
				d.Printer.Message("Error [USAGE]: provide --content or -f <file>")
				return command.Handled(2)
			}
			var tp, tpt *int64
			if cmd.Flags().Changed("target-page-id") {
				tp = i64Ptr(targetPageID)
			}
			if cmd.Flags().Changed("target-page-type-id") {
				tpt = i64Ptr(targetPageTypeID)
			}
			return validateContent(cmd, d, body, tp, tpt, true)
		},
	}
	validate.Flags().StringVar(&content, "content", "", "markdown content")
	validate.Flags().StringVarP(&file, "file", "f", "", "file with content")
	validate.Flags().Int64Var(&targetPageID, "target-page-id", 0, "validate in the context of an existing page")
	validate.Flags().Int64Var(&targetPageTypeID, "target-page-type-id", 0, "validate in the context of a page type")
	c.AddCommand(validate)
	return c
}

// validateContent calls /content/validate and renders the diagnostics. When
// announce is set (the standalone `content validate` command) it makes success
// and failure unambiguous for scripts/agents (NORM-jaoljqtl): an explicit
// "OK - 0 diagnostics" line in table mode (quiet-suppressible) and a non-zero
// exit (3) on at least one ERROR-severity diagnostic. The --dry-run reuse passes
// announce=false to keep its preview semantics (diagnostics shown, exit 0).
// JSON output stays verbatim so `... -o json | jq` is unaffected.
func validateContent(cmd *cobra.Command, d *command.Deps, content string, targetPageID, targetPageTypeID *int64, announce bool) error {
	f := api.PublicContentValidationForm{Content: content, TargetPageId: targetPageID, TargetPageTypeId: targetPageTypeID}
	body, apiErr := d.Client.ValidateContent(cmd.Context(), f)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, "normatik content validate")
	}
	d.Printer.Raw(body) // JSON mode: verbatim body (diagnostics[] included, pipe-clean)
	if !announce {
		return nil
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Line     int    `json:"line"`
			Column   int    `json:"column"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		// A 2xx body we cannot parse is not a verifiable success — never claim OK.
		d.Printer.Message("Could not parse the validation response (%d bytes).", len(body))
		return command.Handled(65)
	}
	errCount := 0
	for _, dg := range res.Diagnostics {
		if strings.EqualFold(dg.Severity, string(api.ERROR)) {
			errCount++
		}
	}
	// Table mode: Raw does not render the diagnostics array, so signal the outcome
	// explicitly — an "OK - 0 diagnostics" line on a clean run (quiet-suppressible),
	// or each diagnostic (same shape as the ProblemDetail renderer) so a failing run
	// is never silent. JSON mode already carries everything verbatim above.
	if d.Printer.Mode != render.JSON {
		if len(res.Diagnostics) == 0 {
			if !d.Printer.Quiet {
				d.Printer.Message("OK - 0 diagnostics")
			}
		} else {
			for _, dg := range res.Diagnostics {
				d.Printer.Message("%d:%d %s %s: %s", dg.Line, dg.Column, dg.Severity, dg.Code, dg.Message)
			}
		}
	}
	// At least one ERROR-severity diagnostic → invalid content; exit non-zero so a
	// script/agent can branch on the exit code instead of parsing the JSON body.
	if errCount > 0 {
		return command.Handled(3)
	}
	return nil
}
