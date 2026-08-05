package cli

import (
	"context"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

// variadicIDs parses positional args after the first into a []int64.
func variadicIDs(args []string) ([]int64, bool) {
	ids := make([]int64, 0, len(args))
	for _, a := range args {
		id, err := command.ParseID(a)
		if err != nil {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// ---- property descriptors writes ----

func addPropertyDescriptorWrites(c *cobra.Command) {
	var pageTypeID int64
	create := &cobra.Command{
		Use: "create -f form.json", Short: "Add a property descriptor (--page-type-id, -f form.json)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			file, _ := cmd.Flags().GetString("file")
			form, lerr := loadForm[api.PropertyDescriptorForm](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik property-descriptors create", "Property descriptor created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreatePropertyDescriptor(cmd.Context(), pageTypeID, form)
			}, func([]byte) string { return weburl.PageType(pageTypeID) }, "id", "name", "dataType")
		},
	}
	create.Flags().Int64Var(&pageTypeID, "page-type-id", 0, "page type id (required)")
	create.Flags().StringP("file", "f", "", "JSON file with the form (required)")
	_ = create.MarkFlagRequired("page-type-id")
	_ = create.MarkFlagRequired("file")
	command.URLFlag(create)

	update := idFormFileWrite[api.PropertyDescriptorForm]("update <id>", "Update a property descriptor (-f form.json)", "normatik property-descriptors update", "Property descriptor updated.",
		func(d *command.Deps, ctx context.Context, id int64, f api.PropertyDescriptorForm) ([]byte, *client.APIError) {
			return d.Client.UpdatePropertyDescriptor(ctx, id, f)
		})
	del := idWrite("delete <id>", "Delete a property descriptor", "normatik property-descriptors delete", "Property descriptor deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeletePropertyDescriptor(ctx, id)
		})

	swap := &cobra.Command{
		Use: "swap <fromId> <toId>", Short: "Swap two property descriptors", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, e1 := command.ParseID(args[0])
			to, e2 := command.ParseID(args[1])
			if e1 != nil || e2 != nil {
				return command.Handled(2)
			}
			return runWrite(cmd, "normatik property-descriptors swap", "Descriptors swapped.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SwapPropertyDescriptors(cmd.Context(), from, to)
			})
		},
	}
	sort := &cobra.Command{
		Use: "sort <pageTypeId> <id...>", Short: "Sort property descriptors of a page type", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ptID, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			ids, ok := variadicIDs(args[1:])
			if !ok {
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik property-descriptors sort", "Descriptors sorted.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SortPropertyDescriptors(cmd.Context(), ptID, ids)
			}, func([]byte) string { return weburl.PageType(ptID) })
		},
	}
	command.URLFlag(sort)
	dcSort := &cobra.Command{
		Use: "display-columns-sort <descriptorId> <id...>", Short: "Sort display-columns of a descriptor", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			descID, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			ids, ok := variadicIDs(args[1:])
			if !ok {
				return command.Handled(2)
			}
			return runWrite(cmd, "normatik property-descriptors display-columns-sort", "Display-columns sorted.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SortDisplayColumns(cmd.Context(), descID, ids)
			})
		},
	}
	addWriteCommands(c, create, update, del, swap, sort, dcSort)
}

// ---- work items writes (--page-id from the parent persistent flag) ----

func addWorkItemsWrites(c *cobra.Command) {
	pageID := func(cmd *cobra.Command) int64 { v, _ := cmd.Flags().GetInt64("page-id"); return v }

	var typeSlug, desc string
	create := &cobra.Command{
		Use: "create", Short: "Create a work item (--type-slug, --description)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.WorkItemCreateForm{TypeSlug: typeSlug}
			if cmd.Flags().Changed("description") {
				f.Description = strPtr(desc)
			}
			return runWriteURL(cmd, "normatik work-items create", "Work item created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreateWorkItem(cmd.Context(), pageID(cmd), f)
			}, func([]byte) string { return weburl.Page(pageID(cmd)) }, "id", "typeSlug")
		},
	}
	create.Flags().StringVar(&typeSlug, "type-slug", "", "work-item-type slug (required)")
	create.Flags().StringVar(&desc, "description", "", "description")
	_ = create.MarkFlagRequired("type-slug")
	command.URLFlag(create)

	var edesc string
	edit := &cobra.Command{
		Use: "edit <workItemId>", Short: "Update a work item (--description)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.WorkItemEditForm{}
			if cmd.Flags().Changed("description") {
				f.Description = strPtr(edesc)
			}
			return runWriteURL(cmd, "normatik work-items edit", "Work item updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.EditWorkItem(cmd.Context(), pageID(cmd), wid, f)
			}, func([]byte) string { return weburl.Page(pageID(cmd)) })
		},
	}
	edit.Flags().StringVar(&edesc, "description", "", "new description")
	command.URLFlag(edit)

	del := &cobra.Command{
		Use: "delete <workItemId>", Short: "Delete a work item", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, perr := command.ParseID(args[0])
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
			body, apiErr := d.Client.DeleteWorkItem(cmd.Context(), pageID(cmd), wid)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik work-items delete")
			}
			if command.PrintURL(d, cmd, weburl.Page(pageID(cmd))) {
				return nil
			}
			writeResult(d, body, "Work item deleted.")
			return nil
		},
	}
	addSoftConfirm(del)
	command.URLFlag(del)

	var text string
	comment := &cobra.Command{
		Use: "comment <workItemId>", Short: "Post a comment (--text)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik work-items comment", "Comment posted.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CommentWorkItem(cmd.Context(), pageID(cmd), wid, api.WorkItemCommentForm{Text: text})
			}, func([]byte) string { return weburl.Page(pageID(cmd)) })
		},
	}
	comment.Flags().StringVar(&text, "text", "", "comment text (required)")
	_ = comment.MarkFlagRequired("text")
	command.URLFlag(comment)

	var toValueID int64
	var tcomment string
	transition := &cobra.Command{
		Use: "transition <workItemId>", Short: "Perform a transition (--to-value-id)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.WorkItemTransitionForm{ToValueId: toValueID}
			if cmd.Flags().Changed("comment") {
				f.Comment = strPtr(tcomment)
			}
			return runWriteURL(cmd, "normatik work-items transition", "Transition performed.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.TransitionWorkItem(cmd.Context(), pageID(cmd), wid, f)
			}, func([]byte) string { return weburl.Page(pageID(cmd)) })
		},
	}
	transition.Flags().Int64Var(&toValueID, "to-value-id", 0, "target status value id (required, see work-items transitions)")
	transition.Flags().StringVar(&tcomment, "comment", "", "optional note")
	_ = transition.MarkFlagRequired("to-value-id")
	command.URLFlag(transition)

	addWriteCommands(c, create, edit, del, comment, transition)
}

// ---- work-item-types writes (slug-keyed) ----

func addWorkItemTypesWrites(c *cobra.Command) {
	var name, slug string
	var domainEnumID, defaultStatusID int64
	create := &cobra.Command{
		Use: "create", Short: "Create a work item type (--name, --slug)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.WorkItemTypeCreateForm{}
			if cmd.Flags().Changed("name") {
				f.Name = strPtr(name)
			}
			if cmd.Flags().Changed("slug") {
				f.Slug = strPtr(slug)
			}
			if cmd.Flags().Changed("domain-enum-id") {
				f.DomainEnumId = i64Ptr(domainEnumID)
			}
			if cmd.Flags().Changed("default-status-value-id") {
				f.DefaultStatusValueId = i64Ptr(defaultStatusID)
			}
			return runWriteURL(cmd, "normatik work-item-types create", "Work item type created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreateWorkItemType(cmd.Context(), f)
			}, func(body []byte) string { return weburl.AdminWorkItemType(responseSlug(body)) }, "slug", "name")
		},
	}
	create.Flags().StringVar(&name, "name", "", "name")
	create.Flags().StringVar(&slug, "slug", "", "slug")
	create.Flags().Int64Var(&domainEnumID, "domain-enum-id", 0, "linked domain enum id")
	create.Flags().Int64Var(&defaultStatusID, "default-status-value-id", 0, "default status value id")
	command.URLFlag(create)

	update := &cobra.Command{
		Use: "update <slug> -f form.json", Short: "Update a work item type (-f form.json)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			file, _ := cmd.Flags().GetString("file")
			f, lerr := loadForm[api.WorkItemTypeUpdateForm](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik work-item-types update", "Work item type updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateWorkItemType(cmd.Context(), args[0], f)
			}, func([]byte) string { return weburl.AdminWorkItemType(args[0]) })
		},
	}
	update.Flags().StringP("file", "f", "", "JSON file with the form (required)")
	_ = update.MarkFlagRequired("file")
	command.URLFlag(update)

	// Delete points at the bare list route (mapping table): the detail page
	// is gone after the delete.
	del := &cobra.Command{
		Use: "delete <slug>", Short: "Delete a work item type", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmHard(cmd, d, args[0]); e != nil {
				return e
			}
			body, apiErr := d.Client.DeleteWorkItemType(cmd.Context(), args[0])
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik work-item-types delete")
			}
			if command.PrintURL(d, cmd, weburl.AdminWorkItemTypes()) {
				return nil
			}
			writeResult(d, body, "Work item type deleted.")
			return nil
		},
	}
	addHardConfirm(del)
	command.URLFlag(del)

	transitions := &cobra.Command{Use: "transitions", Short: "Transitions of a work item type (add, update, remove)", RunE: command.UnknownSub}
	addT := &cobra.Command{
		Use: "add <typeSlug> -f form.json", Short: "Add a transition (-f form.json)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			file, _ := cmd.Flags().GetString("file")
			f, lerr := loadForm[api.WorkItemTransitionCreateForm](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik work-item-types transitions add", "Transition added.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.AddWorkItemTransition(cmd.Context(), args[0], f)
			}, func([]byte) string { return weburl.AdminWorkItemType(args[0]) })
		},
	}
	addT.Flags().StringP("file", "f", "", "JSON file with the form (required)")
	_ = addT.MarkFlagRequired("file")
	command.URLFlag(addT)
	updT := &cobra.Command{
		Use: "update <typeSlug> <transitionId> -f form.json", Short: "Update a transition (-f form.json)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, perr := command.ParseID(args[1])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			file, _ := cmd.Flags().GetString("file")
			f, lerr := loadForm[api.WorkItemTransitionUpdateForm](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik work-item-types transitions update", "Transition updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateWorkItemTransition(cmd.Context(), args[0], tid, f)
			}, func([]byte) string { return weburl.AdminWorkItemType(args[0]) })
		},
	}
	updT.Flags().StringP("file", "f", "", "JSON file with the form (required)")
	_ = updT.MarkFlagRequired("file")
	command.URLFlag(updT)
	remT := &cobra.Command{
		Use: "remove <typeSlug> <transitionId>", Short: "Remove a transition", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, perr := command.ParseID(args[1])
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
			body, apiErr := d.Client.RemoveWorkItemTransition(cmd.Context(), args[0], tid)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik work-item-types transitions remove")
			}
			if command.PrintURL(d, cmd, weburl.AdminWorkItemType(args[0])) {
				return nil
			}
			writeResult(d, body, "Transition removed.")
			return nil
		},
	}
	command.URLFlag(remT)
	addSoftConfirm(remT)
	transitions.AddCommand(addT, updT, remT)
	addWriteCommands(c, create, update, del, transitions)
}
