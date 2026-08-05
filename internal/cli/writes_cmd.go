package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/weburl"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"
)

func writeResult(d *command.Deps, body []byte, success string, fields ...string) {
	if strings.TrimSpace(string(body)) == "" {
		d.Printer.Message("%s", success)
		return
	}
	d.Printer.Raw(body, fields...)
}

// idWrite builds a write command taking one positional <id>. confirm ∈ {none,soft,hard}.
func idWrite(use, short, invocation, success, confirm string, fn func(*command.Deps, context.Context, int64) ([]byte, *client.APIError)) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if confirm == "soft" {
				if e := confirmSoft(cmd, d); e != nil {
					return e
				}
			} else if confirm == "hard" {
				if e := confirmHard(cmd, d, args[0]); e != nil {
					return e
				}
			}
			body, apiErr := fn(d, cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			writeResult(d, body, success)
			return nil
		},
	}
	if confirm == "soft" {
		addSoftConfirm(cmd)
	} else if confirm == "hard" {
		addHardConfirm(cmd)
	}
	return cmd
}

// formFileWrite builds a create/update command that loads its body from -f form.json.
func formFileWrite[T any](use, short, invocation, success string, fn func(*command.Deps, context.Context, T) ([]byte, *client.APIError)) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: use, Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			form, lerr := loadForm[T](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			body, apiErr := fn(d, cmd.Context(), form)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			writeResult(d, body, success)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON file containing the form (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// idFormFileWrite is formFileWrite with a leading positional <id>.
func idFormFileWrite[T any](use, short, invocation, success string, fn func(*command.Deps, context.Context, int64, T) ([]byte, *client.APIError)) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			form, lerr := loadForm[T](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			body, apiErr := fn(d, cmd.Context(), id, form)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			writeResult(d, body, success)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON file containing the form (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// responseID extracts the numeric "id" field from a create response body, to
// build the --url of the just-created resource (CR-besluit: writes execute
// first, then the URL comes from the response — never resolved before the
// write happened). A body that fails to unmarshal (impossible on a successful
// response by contract) yields the zero id.
func responseID(body []byte) int64 {
	var v struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(body, &v)
	return v.ID
}

// responseSlug is responseID's slug counterpart — work-item-types is the only
// slug-keyed route in the mapping table (CR-besluit 3).
func responseSlug(body []byte) string {
	var v struct {
		Slug string `json:"slug"`
	}
	_ = json.Unmarshal(body, &v)
	return v.Slug
}

// runWriteURL is runWrite (writehelpers.go) with --url support: the write
// always executes first, then when --url is set urlPath(body) is resolved and
// printed instead of the normal write result (CR-besluit: never resolve --url
// before the write happened). urlPath ignores body for commands whose touched
// resource is already known from args (e.g. update); create commands read the
// new resource's id/slug from the response via responseID/responseSlug.
func runWriteURL(cmd *cobra.Command, invocation, successMsg string, fn func(*command.Deps) ([]byte, *client.APIError), urlPath func(body []byte) string, fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if command.PrintURL(d, cmd, urlPath(body)) {
		return nil
	}
	writeResult(d, body, successMsg, fields...)
	return nil
}

// idWriteURL is idWrite with --url support; see runWriteURL. urlPath
// builds the frontend path from the id already known from args — e.g. a
// delete prints the list route (the detail page is gone), an
// activate/reactivate prints the still-existing detail route (mapping
// table). Also used by the pages-family writes in writes_pages.go
// (revisions start, restriction create/remove, trash restore/purge).
// confirm ∈ {none,soft,hard}.
func idWriteURL(use, short, invocation, success, confirm string, fn func(*command.Deps, context.Context, int64) ([]byte, *client.APIError), urlPath func(id int64) string) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if confirm == "soft" {
				if e := confirmSoft(cmd, d); e != nil {
					return e
				}
			} else if confirm == "hard" {
				if e := confirmHard(cmd, d, args[0]); e != nil {
					return e
				}
			}
			body, apiErr := fn(d, cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			if command.PrintURL(d, cmd, urlPath(id)) {
				return nil
			}
			writeResult(d, body, success)
			return nil
		},
	}
	if confirm == "soft" {
		addSoftConfirm(cmd)
	} else if confirm == "hard" {
		addHardConfirm(cmd)
	}
	command.URLFlag(cmd)
	return cmd
}

// formFileWriteURL is formFileWrite with --url support; see runWriteURL.
func formFileWriteURL[T any](use, short, invocation, success string, fn func(*command.Deps, context.Context, T) ([]byte, *client.APIError), urlPath func(body []byte) string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: use, Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			form, lerr := loadForm[T](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			body, apiErr := fn(d, cmd.Context(), form)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			if command.PrintURL(d, cmd, urlPath(body)) {
				return nil
			}
			writeResult(d, body, success)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON file containing the form (required)")
	_ = cmd.MarkFlagRequired("file")
	command.URLFlag(cmd)
	return cmd
}

// idFormFileWriteURL is idFormFileWrite with --url support; see runWriteURL.
func idFormFileWriteURL[T any](use, short, invocation, success string, fn func(*command.Deps, context.Context, int64, T) ([]byte, *client.APIError), urlPath func(id int64, body []byte) string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			form, lerr := loadForm[T](file)
			if lerr != nil {
				d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, lerr)
				return command.Handled(2)
			}
			body, apiErr := fn(d, cmd.Context(), id, form)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			if command.PrintURL(d, cmd, urlPath(id, body)) {
				return nil
			}
			writeResult(d, body, success)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON file containing the form (required)")
	_ = cmd.MarkFlagRequired("file")
	command.URLFlag(cmd)
	return cmd
}

// ---- users ----

func addUsersWrites(c *cobra.Command) {
	var dn, email string
	create := &cobra.Command{
		Use: "create", Short: "Create an external user", Example: "  normatik users create --display-name \"Jan\" --email jan@x.nl",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.ExternalUserForm{DisplayName: dn}
			if email != "" {
				e := openapi_types.Email(email)
				f.Email = &e
			}
			return runWriteURL(cmd, "normatik users create", "User created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreateExternalUser(cmd.Context(), f)
			}, func(body []byte) string { return weburl.AdminUser(responseID(body)) }, "id", "displayName", "email")
		},
	}
	create.Flags().StringVar(&dn, "display-name", "", "display name (required)")
	create.Flags().StringVar(&email, "email", "", "email address")
	_ = create.MarkFlagRequired("display-name")
	command.URLFlag(create)

	var udn, uemail, urole, uwf string
	var setPassword bool
	update := &cobra.Command{
		Use: "update <id>", Short: "Update a user (admin)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.UserForm{DisplayName: udn, Email: openapi_types.Email(uemail), Role: api.UserFormRole(urole)}
			if setPassword {
				// NORMATIK-12: the secret is read via a hidden prompt or bounded stdin,
				// never from argv (no shell history / ps / CI-tracing exposure).
				pw, rerr := readNewUserPassword()
				if rerr != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", rerr)
					return command.Handled(2)
				}
				f.Password = strPtr(pw)
			}
			if cmd.Flags().Changed("workflow-role") {
				wr := api.UserFormWorkflowRole(uwf)
				f.WorkflowRole = &wr
			}
			return runWriteURL(cmd, "normatik users update", "User updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateUser(cmd.Context(), id, f)
			}, func([]byte) string { return weburl.AdminUser(id) }, "id", "displayName", "email", "role")
		},
	}
	update.Flags().StringVar(&udn, "display-name", "", "display name (required)")
	update.Flags().StringVar(&uemail, "email", "", "email address (required)")
	update.Flags().StringVar(&urole, "role", "", "role (required): USER|ADMIN")
	update.Flags().BoolVar(&setPassword, "set-password", false, "set a new password; read from a hidden prompt or piped stdin, never from argv")
	update.Flags().StringVar(&uwf, "workflow-role", "", "workflow role (optional)")
	_ = update.MarkFlagRequired("display-name")
	_ = update.MarkFlagRequired("email")
	_ = update.MarkFlagRequired("role")
	command.URLFlag(update)

	// Delete points at the bare list route (mapping table): the detail page
	// is gone after a soft-delete, unlike reactivate which lands back on it.
	del := idWriteURL("delete <id>", "Delete a user (soft, admin)", "normatik users delete", "User deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeleteUser(ctx, id)
		}, func(int64) string { return weburl.AdminUsers() })
	react := idWriteURL("reactivate <id>", "Reactivate a soft-deleted user", "normatik users reactivate", "User reactivated.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.ReactivateUser(ctx, id)
		}, weburl.AdminUser)

	var replacement int64
	perm := &cobra.Command{
		Use: "permanent-delete <id>", Short: "Delete a user PERMANENTLY (irreversible)", Args: cobra.ExactArgs(1),
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
			f := api.PermanentDeleteForm{}
			if cmd.Flags().Changed("replacement-owner-id") {
				f.ReplacementOwnerId = i64Ptr(replacement)
			}
			body, apiErr := d.Client.PermanentDeleteUser(cmd.Context(), id, f)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik users permanent-delete")
			}
			writeResult(d, body, "User permanently deleted.")
			return nil
		},
	}
	perm.Flags().Int64Var(&replacement, "replacement-owner-id", 0, "replacement owner for orphaned resources")
	addHardConfirm(perm)
	addWriteCommands(c, create, update, del, react, perm)
}

// ---- groups ----

func addGroupsWrites(c *cobra.Command) {
	bindGroup := func(cmd *cobra.Command, name, desc, role, wf string) api.UserGroupForm {
		f := api.UserGroupForm{Name: name}
		if cmd.Flags().Changed("description") {
			f.Description = strPtr(desc)
		}
		if cmd.Flags().Changed("role") {
			r := api.UserGroupFormRole(role)
			f.Role = &r
		}
		if cmd.Flags().Changed("workflow-role") {
			wr := api.UserGroupFormWorkflowRole(wf)
			f.WorkflowRole = &wr
		}
		return f
	}
	var cn, cdesc, crole, cwf string
	create := &cobra.Command{
		Use: "create", Short: "Create a group",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := bindGroup(cmd, cn, cdesc, crole, cwf)
			return runWriteURL(cmd, "normatik groups create", "Group created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreateGroup(cmd.Context(), f)
			}, func(body []byte) string { return weburl.AdminGroup(responseID(body)) }, "id", "name")
		},
	}
	create.Flags().StringVar(&cn, "name", "", "group name (required)")
	create.Flags().StringVar(&cdesc, "description", "", "description")
	create.Flags().StringVar(&crole, "role", "", "user role assignment")
	create.Flags().StringVar(&cwf, "workflow-role", "", "workflow role assignment")
	_ = create.MarkFlagRequired("name")
	command.URLFlag(create)

	var un, udesc, urole, uwf string
	update := &cobra.Command{
		Use: "update <id>", Short: "Update a group", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := bindGroup(cmd, un, udesc, urole, uwf)
			return runWriteURL(cmd, "normatik groups update", "Group updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateGroup(cmd.Context(), id, f)
			}, func([]byte) string { return weburl.AdminGroup(id) }, "id", "name")
		},
	}
	update.Flags().StringVar(&un, "name", "", "group name (required)")
	update.Flags().StringVar(&udesc, "description", "", "description")
	update.Flags().StringVar(&urole, "role", "", "user role assignment")
	update.Flags().StringVar(&uwf, "workflow-role", "", "workflow role assignment")
	_ = update.MarkFlagRequired("name")
	command.URLFlag(update)

	activate := idWriteURL("activate <id>", "Activate a group", "normatik groups activate", "Group activated.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.ActivateGroup(ctx, id)
		}, weburl.AdminGroup)
	deactivate := idWriteURL("deactivate <id>", "Deactivate a group", "normatik groups deactivate", "Group deactivated.", "none",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeactivateGroup(ctx, id)
		}, weburl.AdminGroup)

	members := &cobra.Command{Use: "members", Short: "Group members (add, remove)", RunE: command.UnknownSub}
	var userID int64
	addM := &cobra.Command{
		Use: "add <groupId>", Short: "Add a member (--user-id)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			return runWriteURL(cmd, "normatik groups members add", "Member added.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.AddGroupMember(cmd.Context(), gid, api.AddMemberForm{UserId: userID})
			}, func([]byte) string { return weburl.AdminGroup(gid) })
		},
	}
	addM.Flags().Int64Var(&userID, "user-id", 0, "user id (required)")
	_ = addM.MarkFlagRequired("user-id")
	command.URLFlag(addM)
	remM := &cobra.Command{
		Use: "remove <groupId> <userId>", Short: "Remove a member", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, e1 := command.ParseID(args[0])
			uid, e2 := command.ParseID(args[1])
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
			body, apiErr := d.Client.RemoveGroupMember(cmd.Context(), gid, uid)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik groups members remove")
			}
			if command.PrintURL(d, cmd, weburl.AdminGroup(gid)) {
				return nil
			}
			writeResult(d, body, "Member removed.")
			return nil
		},
	}
	addSoftConfirm(remM)
	command.URLFlag(remM)
	members.AddCommand(addM, remM)
	addWriteCommands(c, create, update, activate, deactivate, members)
}

// ---- page-types ----

func addPageTypesWrites(c *cobra.Command) {
	create := formFileWriteURL[api.PageTypeForm]("create", "Create a page type (-f form.json)", "normatik page-types create", "Page type created.",
		func(d *command.Deps, ctx context.Context, f api.PageTypeForm) ([]byte, *client.APIError) {
			return d.Client.CreatePageType(ctx, f)
		}, func(body []byte) string { return weburl.PageType(responseID(body)) })
	update := idFormFileWriteURL[api.PageTypeForm]("update <id>", "Update a page type (-f form.json)", "normatik page-types update", "Page type updated.",
		func(d *command.Deps, ctx context.Context, id int64, f api.PageTypeForm) ([]byte, *client.APIError) {
			return d.Client.UpdatePageType(ctx, id, f)
		}, func(id int64, _ []byte) string { return weburl.PageType(id) })
	// Delete points at the bare list route (mapping table): the detail page
	// is gone after the delete.
	del := idWriteURL("delete <id>", "Delete a page type", "normatik page-types delete", "Page type deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeletePageType(ctx, id)
		}, func(int64) string { return weburl.PageTypes() })

	var newParent int64
	var keepWf bool
	move := &cobra.Command{
		Use: "move <id>", Short: "Move a page type (--new-parent-id)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.PageTypeMoveForm{}
			if cmd.Flags().Changed("new-parent-id") {
				f.NewParentId = i64Ptr(newParent)
			}
			if cmd.Flags().Changed("keep-workflow") {
				f.KeepWorkflowOnViaOwnFlag = boolPtr(keepWf)
			}
			return runWriteURL(cmd, "normatik page-types move", "Page type moved.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.MovePageType(cmd.Context(), id, f)
			}, func([]byte) string { return weburl.PageType(id) })
		},
	}
	move.Flags().Int64Var(&newParent, "new-parent-id", 0, "new parent page type id (empty = root)")
	move.Flags().BoolVar(&keepWf, "keep-workflow", false, "keep workflow via own flag")
	command.URLFlag(move)

	addWriteCommands(c, create, update, del, move)
}

// ---- domain-enums ----

func addDomainEnumsWrites(c *cobra.Command) {
	create := formFileWriteURL[api.DomainEnumForm]("create", "Create a domain enum (-f form.json with values[])", "normatik domain-enums create", "Domain enum created.",
		func(d *command.Deps, ctx context.Context, f api.DomainEnumForm) ([]byte, *client.APIError) {
			return d.Client.CreateDomainEnum(ctx, f)
		}, func(body []byte) string { return weburl.AdminDomainEnum(responseID(body)) })
	update := idFormFileWriteURL[api.DomainEnumForm]("update <id>", "Update a domain enum (-f form.json)", "normatik domain-enums update", "Domain enum updated.",
		func(d *command.Deps, ctx context.Context, id int64, f api.DomainEnumForm) ([]byte, *client.APIError) {
			return d.Client.UpdateDomainEnum(ctx, id, f)
		}, func(id int64, _ []byte) string { return weburl.AdminDomainEnum(id) })
	// Delete points at the bare list route (mapping table): the detail page
	// is gone after the delete.
	del := idWriteURL("delete <id>", "Delete a domain enum", "normatik domain-enums delete", "Domain enum deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeleteDomainEnum(ctx, id)
		}, func(int64) string { return weburl.AdminDomainEnums() })
	addWriteCommands(c, create, update, del)
}

// ---- workflow-roles ----

func addWorkflowRolesWrites(c *cobra.Command) {
	var role string
	mk := func(use, short, invocation, success string, fn func(*command.Deps, context.Context, int64, api.WorkflowRoleForm) ([]byte, *client.APIError)) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := command.ParseID(args[0])
			if perr != nil {
				return command.Handled(2)
			}
			f := api.WorkflowRoleForm{}
			// Backend semantics (WorkflowRoleForm.workflowRole is nullable — null means
			// remove the role, and Jackson treats an absent field as null): an empty
			// flag value must OMIT the field entirely, because the enum rejects "" with
			// 400 INVALID_REQUEST. So `--workflow-role ""` serializes {} → unassign.
			if cmd.Flags().Changed("workflow-role") && role != "" {
				wr := api.WorkflowRoleFormWorkflowRole(role)
				f.WorkflowRole = &wr
			}
			return runWriteURL(cmd, invocation, success, func(d *command.Deps) ([]byte, *client.APIError) { return fn(d, cmd.Context(), id, f) },
				func([]byte) string { return weburl.AdminWorkflowRoles() })
		}}
		cmd.Flags().StringVar(&role, "workflow-role", "", "workflow role (empty = unassign)")
		command.URLFlag(cmd)
		return cmd
	}
	setUser := mk("set-user <userId>", "Set the workflow role of a user", "normatik workflow-roles set-user", "Workflow role set.",
		func(d *command.Deps, ctx context.Context, id int64, f api.WorkflowRoleForm) ([]byte, *client.APIError) {
			return d.Client.SetUserWorkflowRole(ctx, id, f)
		})
	setGroup := mk("set-group <groupId>", "Set the workflow role of a group", "normatik workflow-roles set-group", "Workflow role set.",
		func(d *command.Deps, ctx context.Context, id int64, f api.WorkflowRoleForm) ([]byte, *client.APIError) {
			return d.Client.SetGroupWorkflowRole(ctx, id, f)
		})
	addWriteCommands(c, setUser, setGroup)
}

// ---- landing-settings ----

func addLandingSettingsWrites(c *cobra.Command) {
	var active bool
	var content string
	update := &cobra.Command{
		Use: "update", Short: "Update the landing settings (--active, --content)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.LandingSettingsForm{}
			if cmd.Flags().Changed("active") {
				f.Active = boolPtr(active)
			}
			if cmd.Flags().Changed("content") {
				f.Content = strPtr(content)
			}
			return runWriteURL(cmd, "normatik landing-settings update", "Landing settings updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateLandingSettings(cmd.Context(), f)
			}, func([]byte) string { return weburl.AdminLanding() })
		},
	}
	update.Flags().BoolVar(&active, "active", false, "landing page active")
	update.Flags().StringVar(&content, "content", "", "markdown content of the landing page")
	command.URLFlag(update)
	addWriteCommands(c, update)
}
