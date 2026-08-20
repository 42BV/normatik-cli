package cli

import (
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

// withWrites returns a registrar that builds the read command and then attaches
// the resource's write subcommands (F3).
func withWrites(build func() *cobra.Command, add func(*cobra.Command)) command.Registrar {
	return func() *cobra.Command {
		c := build()
		add(c)
		return c
	}
}

// init registers all resource commands so root stays constant.
func init() {
	command.Register(withWrites(newUsersCmd, addUsersWrites))
	command.Register(withWrites(newGroupsCmd, addGroupsWrites))
	command.Register(withWrites(newPageTypesCmd, addPageTypesWrites))
	command.Register(withWrites(newPropertyDescriptorsCmd, addPropertyDescriptorWrites))
	command.Register(withWrites(newWorkItemTypesCmd, addWorkItemTypesWrites))
	command.Register(withWrites(newWorkItemsCmd, addWorkItemsWrites))
	command.Register(withWrites(newDomainEnumsCmd, addDomainEnumsWrites))
	command.Register(withWrites(newWorkflowRolesCmd, addWorkflowRolesWrites))
	command.Register(withWrites(newLandingSettingsCmd, addLandingSettingsWrites))
	command.Register(withWrites(newTrashCmd, addTrashWrites))
	command.Register(withWrites(newArchiveCmd, addArchiveWrites))
	command.Register(newWorkflowCmd) // read-only
	command.Register(newAuditCmd)    // read-only
	command.Register(newContentCmd)  // content validate (dry-run)
}

func parent(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: command.UnknownSub}
}

// idArg parses a single positional <id> with a friendly usage error.
func idArg(cmd *cobra.Command, arg string) (int64, error) {
	id, err := command.ParseID(arg)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// runListURL is runList (readhelpers.go) with --url support: fn always
// executes — a failed lookup still renders the standard error envelope
// instead of silently printing a URL (same contract as `pages get`) — and
// when --url is set the resolved urlPath replaces the normal list render
// (CR-besluit 1). Callers register the flag themselves via command.URLFlag.
func runListURL(cmd *cobra.Command, invocation, urlPath string, fn func(*command.Deps) ([]byte, *client.APIError), fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if command.PrintURL(d, cmd, urlPath) {
		return nil
	}
	d.Printer.List(body, fields...)
	return nil
}

// runObjectURL is runObject (readhelpers.go) with --url support; see runListURL.
func runObjectURL(cmd *cobra.Command, invocation, urlPath string, fn func(*command.Deps) ([]byte, *client.APIError), fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if command.PrintURL(d, cmd, urlPath) {
		return nil
	}
	d.Printer.Raw(body, fields...)
	return nil
}

// ---- users ----

func newUsersCmd() *cobra.Command {
	c := parent("users", "Users (list, search, get, create, update, delete, reactivate, permanent-delete)")
	var status string
	var pg paging
	list := &cobra.Command{
		Use: "list", Short: "List users (--status ACTIVE|DELETED)",
		Example: "  normatik users list --status ACTIVE",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik users list", weburl.AdminUsers(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListUsers(cmd.Context(), status, pg.page, pg.size, pg.sort)
			})
		},
	}
	addPaging(list, &pg)
	list.Flags().StringVar(&status, "status", "", "filter by status: ACTIVE|DELETED")
	command.URLFlag(list)

	var spg paging
	search := &cobra.Command{
		Use: "search <query>", Short: "Search active users (admin)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListURL(cmd, "normatik users search", weburl.AdminUsers(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SearchUsers(cmd.Context(), args[0], spg.page, spg.size, spg.sort)
			})
		},
	}
	addPaging(search, &spg)
	command.URLFlag(search)

	var expand []string
	get := &cobra.Command{
		Use: "get <id>", Short: "Get a user (--expand groups)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObjectURL(cmd, "normatik users get", weburl.AdminUser(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetUser(cmd.Context(), id, expand)
			}, "id", "displayName", "email", "role", "workflowRole")
		},
	}
	get.Flags().StringSliceVar(&expand, "expand", nil, "sections: groups")
	command.URLFlag(get)
	c.AddCommand(list, search, get)
	return c
}

// ---- groups ----

func newGroupsCmd() *cobra.Command {
	c := parent("groups", "Groups (list, search, get, create, update, activate, deactivate, members)")
	var status string
	var pg paging
	list := &cobra.Command{
		Use: "list", Short: "List groups (--status ACTIVE|INACTIVE)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik groups list", weburl.AdminGroups(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListGroups(cmd.Context(), status, pg.page, pg.size, pg.sort)
			})
		},
	}
	addPaging(list, &pg)
	list.Flags().StringVar(&status, "status", "", "filter by status: ACTIVE|INACTIVE")
	command.URLFlag(list)

	var spg paging
	search := &cobra.Command{
		Use: "search <query>", Short: "Search groups by name", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListURL(cmd, "normatik groups search", weburl.AdminGroups(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SearchGroups(cmd.Context(), args[0], spg.page, spg.size, spg.sort)
			})
		},
	}
	addPaging(search, &spg)
	command.URLFlag(search)

	get := &cobra.Command{
		Use: "get <id>", Short: "Get a group (with members)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObjectURL(cmd, "normatik groups get", weburl.AdminGroup(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetGroup(cmd.Context(), id)
			}, "id", "name", "status", "role", "workflowRole")
		},
	}
	command.URLFlag(get)
	c.AddCommand(list, search, get)
	return c
}

// ---- page-types ----

func newPageTypesCmd() *cobra.Command {
	c := parent("page-types", "Page types (list, get, find, available-descriptors, chain-link-options, create, update, delete, move)")
	list := &cobra.Command{
		Use: "list", Short: "List all page types",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik page-types list", weburl.PageTypes(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListPageTypes(cmd.Context())
			})
		},
	}
	command.URLFlag(list)
	var expand []string
	get := &cobra.Command{
		Use: "get <id>", Short: "Get a page type (--expand usage)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObjectURL(cmd, "normatik page-types get", weburl.PageType(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetPageType(cmd.Context(), id, expand)
			}, "id", "name", "abstractType")
		},
	}
	get.Flags().StringSliceVar(&expand, "expand", nil, "sections: usage")
	command.URLFlag(get)
	apd := &cobra.Command{
		Use: "available-descriptors <id>", Short: "Available property descriptors for display columns", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runListURL(cmd, "normatik page-types available-descriptors", weburl.PageType(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetAvailablePropertyDescriptors(cmd.Context(), id)
			})
		},
	}
	command.URLFlag(apd)
	var clpPageID int64
	clp := &cobra.Command{
		Use: "chain-link-options <id>", Short: "Chain-link options (--page-id required)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runListURL(cmd, "normatik page-types chain-link-options", weburl.PageType(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetChainLinkOptions(cmd.Context(), id, clpPageID)
			})
		},
	}
	clp.Flags().Int64Var(&clpPageID, "page-id", 0, "page id for the chain-link context (required)")
	_ = clp.MarkFlagRequired("page-id")
	command.URLFlag(clp)
	c.AddCommand(list, get, newPageTypesFindCmd(), apd, clp)
	return c
}

// ---- property-descriptors ----

func newPropertyDescriptorsCmd() *cobra.Command {
	c := parent("property-descriptors", "Property descriptors (get, create, update, delete, swap, sort, display-columns-sort)")
	get := &cobra.Command{
		Use: "get <id>", Short: "Get a property descriptor", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObject(cmd, "normatik property-descriptors get", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetPropertyDescriptor(cmd.Context(), id)
			}, "id", "name", "dataType")
		},
	}
	c.AddCommand(get)
	return c
}

// ---- work-item-types ----

func newWorkItemTypesCmd() *cobra.Command {
	c := parent("work-item-types", "Work item types (list, get, create, update, delete, transitions)")
	var pg paging
	list := &cobra.Command{
		Use: "list", Short: "List work item types",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik work-item-types list", weburl.AdminWorkItemTypes(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListWorkItemTypes(cmd.Context(), pg.page, pg.size, pg.sort)
			})
		},
	}
	addPaging(list, &pg)
	command.URLFlag(list)
	get := &cobra.Command{
		Use: "get <slug>", Short: "Get a work item type (by slug)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObjectURL(cmd, "normatik work-item-types get", weburl.AdminWorkItemType(args[0]), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetWorkItemType(cmd.Context(), args[0])
			}, "slug", "name")
		},
	}
	command.URLFlag(get)
	c.AddCommand(list, get)
	return c
}

// ---- work-items (scoped under --page) ----

func newWorkItemsCmd() *cobra.Command {
	c := parent("work-items", "Work items of a page (list, get, transitions, create, edit, delete, comment, transition)")
	var pageID int64
	c.PersistentFlags().Int64Var(&pageID, "page-id", 0, "page id the work items belong to (required)")
	_ = c.MarkPersistentFlagRequired("page-id")
	var typ string
	list := &cobra.Command{
		Use: "list", Short: "List work items of a page (--type)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik work-items list", weburl.Page(pageID), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListWorkItems(cmd.Context(), pageID, typ)
			})
		},
	}
	list.Flags().StringVar(&typ, "type", "", "filter by work-item-type slug")
	command.URLFlag(list)
	get := &cobra.Command{
		Use: "get <workItemId>", Short: "Get a work item", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObjectURL(cmd, "normatik work-items get", weburl.Page(pageID), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetWorkItem(cmd.Context(), pageID, wid)
			})
		},
	}
	command.URLFlag(get)
	tr := &cobra.Command{
		Use: "transitions <workItemId>", Short: "Available transitions for a work item", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wid, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runListURL(cmd, "normatik work-items transitions", weburl.Page(pageID), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetWorkItemTransitions(cmd.Context(), pageID, wid)
			})
		},
	}
	command.URLFlag(tr)
	c.AddCommand(list, get, tr)
	return c
}

// ---- domain-enums ----

func newDomainEnumsCmd() *cobra.Command {
	c := parent("domain-enums", "Domain enums (list, get, usages, create, update, delete)")
	var expand []string
	list := &cobra.Command{
		Use:   "list",
		Short: "List domain enums (--expand values; JSON is a bare array)",
		Long:  "JSON output is a bare array, not a paged envelope. Same shape as page-types list.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik domain-enums list", weburl.AdminDomainEnums(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListDomainEnums(cmd.Context(), expand)
			})
		},
	}
	list.Flags().StringSliceVar(&expand, "expand", nil, "sections: values")
	command.URLFlag(list)
	get := &cobra.Command{
		Use: "get <id>", Short: "Get a domain enum (with its allowed values)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			// Own render (not runObject): the human output must include the nested
			// `values` array, which the generic scalar printer drops. JSON unchanged.
			d, derr := command.Build(cmd)
			if derr != nil {
				return derr
			}
			body, apiErr := d.Client.GetDomainEnum(cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik domain-enums get")
			}
			if command.PrintURL(d, cmd, weburl.AdminDomainEnum(id)) {
				return nil
			}
			d.Printer.DomainEnum(body)
			return nil
		},
	}
	command.URLFlag(get)
	usages := &cobra.Command{
		Use: "usages <id>", Short: "Usages of a domain enum", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return runObjectURL(cmd, "normatik domain-enums usages", weburl.AdminDomainEnum(id), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetDomainEnumUsages(cmd.Context(), id)
			})
		},
	}
	command.URLFlag(usages)
	c.AddCommand(list, get, usages)
	return c
}

// ---- workflow-roles ----

func newWorkflowRolesCmd() *cobra.Command {
	c := parent("workflow-roles", "Workflow roles composite (get, set-user, set-group)")
	var include string
	get := &cobra.Command{
		Use: "get", Short: "Get workflow roles (--include available|groups|users)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObjectURL(cmd, "normatik workflow-roles get", weburl.AdminWorkflowRoles(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetWorkflowRoles(cmd.Context(), include)
			})
		},
	}
	get.Flags().StringVar(&include, "include", "", "extra sections: available|groups|users")
	command.URLFlag(get)
	c.AddCommand(get)
	return c
}

// ---- workflow queues ----

func newWorkflowCmd() *cobra.Command {
	c := parent("workflow", "Workflow queues (review, publish, drafts)")
	var rpg paging
	review := &cobra.Command{
		Use: "review", Short: "Pages waiting for review",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik workflow review", weburl.WorkflowReview(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListReviewQueue(cmd.Context(), rpg.page, rpg.size, rpg.sort)
			})
		},
	}
	addPaging(review, &rpg)
	command.URLFlag(review)
	var ppg paging
	publish := &cobra.Command{
		Use: "publish", Short: "Pages waiting for publication",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik workflow publish", weburl.WorkflowPublish(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListPublishQueue(cmd.Context(), ppg.page, ppg.size, ppg.sort)
			})
		},
	}
	addPaging(publish, &ppg)
	command.URLFlag(publish)
	var dpg paging
	drafts := &cobra.Command{
		Use: "drafts", Short: "Draft pages (workflow-enabled, STORED, not in trash/archived)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik workflow drafts", weburl.WorkflowDrafts(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListDraftsQueue(cmd.Context(), dpg.page, dpg.size, dpg.sort)
			})
		},
	}
	addPaging(drafts, &dpg)
	command.URLFlag(drafts)
	c.AddCommand(review, publish, drafts)
	return c
}

// ---- landing-settings ----

func newLandingSettingsCmd() *cobra.Command {
	c := parent("landing-settings", "Landing page settings (get, update)")
	get := &cobra.Command{
		Use: "get", Short: "Current landing settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObjectURL(cmd, "normatik landing-settings get", weburl.AdminLanding(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetLandingSettings(cmd.Context())
			})
		},
	}
	command.URLFlag(get)
	c.AddCommand(get)
	return c
}

// ---- trash ----

func newTrashCmd() *cobra.Command {
	c := parent("trash", "Trash (list, show, restore, purge, cascade-*)")
	var pg paging
	list := &cobra.Command{
		Use: "list", Short: "List deleted pages (admin)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik trash list", weburl.AdminTrash(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListTrash(cmd.Context(), pg.page, pg.size, pg.sort)
			})
		},
	}
	addPaging(list, &pg)
	command.URLFlag(list)

	show := &cobra.Command{
		Use: "show <pageId>", Short: "Show a trashed page (admin read view)", Args: cobra.ExactArgs(1),
		Example: "  normatik trash show 42\n  open $(normatik trash show 42 --url)",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			d, berr := command.Build(cmd)
			if berr != nil {
				return berr
			}
			body, apiErr := d.Client.GetTrashedPage(cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik trash show")
			}
			if command.PrintURL(d, cmd, weburl.AdminTrashPage(id)) {
				return nil
			}
			// Nested PageResult would render as "…" under Raw; dedicated flatten.
			d.Printer.TrashedPageView(body)
			return nil
		},
	}
	command.URLFlag(show)
	c.AddCommand(list, show)
	c.AddCommand(newTrashCascadeImpactCmd())
	return c
}

// ---- archive ----

func newArchiveCmd() *cobra.Command {
	c := parent("archive", "Archive (list, show, restore, delete, cascade-*)")
	var pg paging
	list := &cobra.Command{
		Use: "list", Short: "List archived pages (admin)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Explicit columns: the archive reason is the whole point of the
			// overview, and deriveFields caps at 6 columns in map order, so it
			// must not be left to derivation.
			return runListURL(cmd, "normatik archive list", weburl.AdminArchive(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListArchive(cmd.Context(), pg.page, pg.size, pg.sort)
			}, "id", "name", "pageTypeName", "reason", "archivedAt")
		},
	}
	addPaging(list, &pg)
	command.URLFlag(list)

	show := &cobra.Command{
		Use: "show <pageId>", Short: "Show an archived page (admin read view)", Args: cobra.ExactArgs(1),
		Example: "  normatik archive show 42\n  open $(normatik archive show 42 --url)",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			d, berr := command.Build(cmd)
			if berr != nil {
				return berr
			}
			body, apiErr := d.Client.GetArchivedPage(cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik archive show")
			}
			if command.PrintURL(d, cmd, weburl.AdminArchivePage(id)) {
				return nil
			}
			// Nested PageResult would render as "…" under Raw; dedicated flatten.
			d.Printer.ArchivedPageView(body)
			return nil
		},
	}
	command.URLFlag(show)
	c.AddCommand(list, show)
	c.AddCommand(newArchiveCascadeImpactCmd())
	return c
}

// ---- audit ----

func newAuditCmd() *cobra.Command {
	c := parent("audit", "Audit log (search)")
	var f client.AuditFilters
	var pg paging
	search := &cobra.Command{
		Use: "search", Short: "Search audit-log entries (admin)",
		Example: "  normatik audit search --actor alice --from 2026-01-01\n  normatik audit search --entity-type PAGE --entity-id 36",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListURL(cmd, "normatik audit search", weburl.AdminAudit(), func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.SearchAuditLog(cmd.Context(), f, pg.page, pg.size, pg.sort)
			})
		},
	}
	addPaging(search, &pg)
	search.Flags().StringVar(&f.Search, "search", "", "free-text search term")
	search.Flags().StringVar(&f.ActionType, "action-type", "", "filter by action type")
	search.Flags().StringVar(&f.Actor, "actor", "", "filter by actor")
	search.Flags().StringVar(&f.EntityType, "entity-type", "", "filter by entity type")
	search.Flags().Int64Var(&f.EntityID, "entity-id", 0, "filter by entity id")
	search.Flags().StringVar(&f.From, "from", "", "from date (YYYY-MM-DD)")
	search.Flags().StringVar(&f.To, "to", "", "to date (YYYY-MM-DD)")
	search.Flags().StringSliceVar(&f.Include, "include", nil, "extra sections")
	command.URLFlag(search)
	c.AddCommand(search)
	return c
}
