package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/problem"
	"github.com/spf13/cobra"
)

// Spaces administration (CR spaces, "ADMIN-beheer via Public API en CLI") and the
// module setting that switches the feature. These commands never take the
// scope flag --space: they name the space they work on as a <key> argument, so
// discovery, first-space creation and module management stay usable before and
// without the module.

var spaceFields = []string{"id", "key", "name", "initials", "color", "isDefault", "hidden", "accessMode"}

func init() {
	command.Register(newSpacesCmd)
	command.Register(newModulesCmd)
	command.Register(newAdminCmd)
}

// spaceNotFoundErr is the SPACE_NOT_FOUND answer of an unknown key, shaped like the
// server's own so it renders through the one error path.
func spaceNotFoundErr(key string) *client.APIError {
	key = strings.ToLower(strings.TrimSpace(key))
	return &client.APIError{Status: 404, Problem: &problem.Problem{
		Status:    404,
		Title:     "Space not found",
		ErrorCode: "SPACE_NOT_FOUND",
		Detail:    fmt.Sprintf("No space with key %q.", key),
		Hint:      "Use normatik spaces list to find available spaces.",
	}}
}

// spaceByKey resolves a lowercase key to its space. The administrator listing
// (all=true) also finds hidden and deleted spaces; a caller without that right
// (a space administrator) falls back to the spaces visible to them.
func spaceByKey(ctx context.Context, d *command.Deps, key string) (api.SpaceResult, *client.APIError) {
	sp, ok, apiErr := d.Client.ResolveSpace(ctx, key, true)
	if apiErr != nil && apiErr.Status == 403 {
		sp, ok, apiErr = d.Client.ResolveSpace(ctx, key, false)
	}
	if apiErr != nil {
		return api.SpaceResult{}, apiErr
	}
	if !ok {
		return api.SpaceResult{}, spaceNotFoundErr(key)
	}
	return sp, nil
}

// withSpace runs fn against the space named by args[0].
func withSpace(cmd *cobra.Command, invocation string, args []string, fn func(*command.Deps, api.SpaceResult) ([]byte, *client.APIError)) func(*command.Deps) ([]byte, *client.APIError) {
	return func(d *command.Deps) ([]byte, *client.APIError) {
		sp, apiErr := spaceByKey(cmd.Context(), d, args[0])
		if apiErr != nil {
			return nil, apiErr
		}
		return fn(d, sp)
	}
}

func newSpacesCmd() *cobra.Command {
	c := parent("spaces", "Spaces (list, get, create, update, access, hidden, default, delete, purge, landing, definitions)")

	var all bool
	list := &cobra.Command{
		Use:     "list",
		Short:   "List the spaces you can use (--all: every space incl. hidden and deleted, administrators)",
		Example: "  normatik spaces list\n  normatik spaces list --all -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, "normatik spaces list", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListSpaces(cmd.Context(), all)
			}, spaceFields...)
		},
	}
	list.Flags().BoolVar(&all, "all", false, "list every space including hidden and deleted ones (administrators)")

	get := &cobra.Command{
		Use: "get <key>", Short: "Get a space with its page counts", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObject(cmd, "normatik spaces get", withSpace(cmd, "normatik spaces get", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.GetSpace(cmd.Context(), sp.Id)
				}), append(spaceFields, "activePageCount", "trashCount", "archivedCount", "deletedAt")...)
		},
	}

	c.AddCommand(list, get)
	addWriteCommands(c, newSpacesCreateCmd(), newSpacesUpdateCmd(), newSpacesHiddenCmd(), newSpacesDefaultCmd(),
		newSpacesDeleteCmd(), newSpacesPurgeCmd())
	c.AddCommand(newSpacesAccessCmd(), newSpacesLandingCmd(), newSpacesDefinitionsCmd())
	return c
}

func newSpacesCreateCmd() *cobra.Command {
	var key, name, color, initials, access string
	cmd := &cobra.Command{
		Use:   "create --key <key> --name <name> --access ALL_USERS|SELECTED",
		Short: "Create a space (the first one is allowed while Spaces is disabled)",
		Long: "Creates a space. The key is 2-20 ASCII letters or digits, is lowercased and never changes afterwards.\n" +
			"The first space of an empty installation can be created while the Spaces module is disabled; further spaces need the module.",
		Example: "  normatik spaces create --key home --name Home --access ALL_USERS\n" +
			"  normatik spaces create --key privacy --name Privacy --access SELECTED --color blue --initials PR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.SpaceForm{
				Key:              strings.ToLower(strings.TrimSpace(key)),
				Name:             name,
				AccessMode:       api.SpaceFormAccessMode(strings.ToUpper(access)),
				Color:            changedStr(cmd, "color", color),
				InitialsOverride: changedStr(cmd, "initials", initials),
			}
			return runWrite(cmd, "normatik spaces create", "Space created.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.CreateSpace(cmd.Context(), f)
			}, spaceFields...)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "space key: 2-20 ASCII letters or digits (lowercased, immutable)")
	cmd.Flags().StringVar(&name, "name", "", "space name")
	cmd.Flags().StringVar(&access, "access", "", "access mode: ALL_USERS|SELECTED")
	cmd.Flags().StringVar(&color, "color", "", "space colour (default: derived by the server)")
	cmd.Flags().StringVar(&initials, "initials", "", "initials override (default: derived from the name)")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("access")
	return cmd
}

func newSpacesUpdateCmd() *cobra.Command {
	var name, color, initials string
	cmd := &cobra.Command{
		Use:   "update <key>",
		Short: "Change name, colour and initials of a space (the key never changes)",
		Args:  cobra.ExactArgs(1),
		Example: "  normatik spaces update home --name \"Company wiki\"\n" +
			"  normatik spaces update home --color green --initials CW",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("color") && !cmd.Flags().Changed("initials") {
				return cmdUsageError(cmd, "nothing to update: pass --name, --color and/or --initials")
			}
			return runWrite(cmd, "normatik spaces update", "Space updated.", withSpace(cmd, "normatik spaces update", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return updateSpaceAppearance(cmd, d, sp.Id, name, color, initials)
				}), spaceFields...)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new space name")
	cmd.Flags().StringVar(&color, "color", "", "new space colour")
	cmd.Flags().StringVar(&initials, "initials", "", "new initials override")
	return cmd
}

func newSpacesHiddenCmd() *cobra.Command {
	var hidden bool
	cmd := &cobra.Command{
		Use:     "hidden <key> --hidden=true|false",
		Short:   "Hide or show a space (administrators)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik spaces hidden privacy --hidden=true\n  normatik spaces hidden privacy --hidden=false",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWrite(cmd, "normatik spaces hidden", "Space visibility updated.", withSpace(cmd, "normatik spaces hidden", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.SetSpaceHidden(cmd.Context(), sp.Id, hidden)
				}), spaceFields...)
		},
	}
	cmd.Flags().BoolVar(&hidden, "hidden", false, "true hides the space, false shows it")
	_ = cmd.MarkFlagRequired("hidden")
	return cmd
}

func newSpacesDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "default <key>",
		Short:   "Make a space the default space (administrators)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik spaces default home",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWrite(cmd, "normatik spaces default", "Default space updated.", withSpace(cmd, "normatik spaces default", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.MakeSpaceDefault(cmd.Context(), sp.Id)
				}), spaceFields...)
		},
	}
}

func newSpacesDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <key>",
		Short:   "Delete a space; trashed and archived pages are kept, and without them the space is removed for good (administrators)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik spaces delete privacy --confirm",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmSoft(cmd, d); e != nil {
				return e
			}
			return runWrite(cmd, "normatik spaces delete", "Space deleted.", withSpace(cmd, "normatik spaces delete", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.DeleteSpace(cmd.Context(), sp.Id)
				}))
		},
	}
	addSoftConfirm(cmd)
	return cmd
}

func newSpacesPurgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "purge <key>",
		Short:   "Remove a deleted space without pages PERMANENTLY; this happens by itself once its last page is gone (administrators, irreversible)",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik spaces purge privacy --confirm=privacy",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if e := confirmHard(cmd, d, strings.ToLower(args[0])); e != nil {
				return e
			}
			return runWrite(cmd, "normatik spaces purge", "Space permanently removed.", withSpace(cmd, "normatik spaces purge", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.PurgeSpace(cmd.Context(), sp.Id)
				}))
		},
	}
	addHardConfirm(cmd)
	return cmd
}

// ---- access ----

func newSpacesAccessCmd() *cobra.Command {
	c := parent("access", "Access of a space (get, set, impact)")

	get := &cobra.Command{
		Use: "get <key>", Short: "Get the access mode and grants of a space", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObject(cmd, "normatik spaces access get", withSpace(cmd, "normatik spaces access get", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.GetSpaceAccess(cmd.Context(), sp.Id)
				}))
		},
	}

	var mode, file string
	var users, groups []string
	var adminUsers, adminGroups []int64
	var confirmOwnLoss bool
	set := &cobra.Command{
		Use:   "set <key> --mode ALL_USERS|SELECTED",
		Short: "Save the access mode and the COMPLETE grant lists of a space in one step",
		Args:  cobra.ExactArgs(1),
		Long: "Replaces the access of a space: the mode plus every user and group grant and every space administrator.\n" +
			"Grants are <id>:<level> with level READ|READ_WRITE|ADMIN. Use -f to start from a JSON form (SpaceAccessForm); flags then override it.",
		Example: "  normatik spaces access set privacy --mode SELECTED --user 12:READ_WRITE --group 3:READ --admin-user 7\n" +
			"  normatik spaces access set home --mode ALL_USERS",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := api.SpaceAccessForm{
				AdministratorGroupIds: []int64{}, AdministratorUserIds: []int64{},
				Groups: []api.SpaceGroupGrantForm{}, Users: []api.SpaceUserGrantForm{},
			}
			if file != "" {
				loaded, err := loadForm[api.SpaceAccessForm](file)
				if err != nil {
					return cmdUsageError(cmd, fmt.Sprintf("could not read form: %v", err))
				}
				f = loaded
			}
			if cmd.Flags().Changed("mode") {
				f.AccessMode = api.SpaceAccessFormAccessMode(strings.ToUpper(mode))
			}
			if f.AccessMode == "" {
				return cmdUsageError(cmd, "--mode is required (ALL_USERS|SELECTED)")
			}
			if cmd.Flags().Changed("user") {
				f.Users = nil
				for _, u := range users {
					id, level, err := parseGrant(u)
					if err != nil {
						return cmdUsageError(cmd, "--user "+err.Error())
					}
					f.Users = append(f.Users, api.SpaceUserGrantForm{UserId: id, AccessLevel: api.SpaceUserGrantFormAccessLevel(level)})
				}
			}
			if cmd.Flags().Changed("group") {
				f.Groups = nil
				for _, g := range groups {
					id, level, err := parseGrant(g)
					if err != nil {
						return cmdUsageError(cmd, "--group "+err.Error())
					}
					f.Groups = append(f.Groups, api.SpaceGroupGrantForm{GroupId: id, AccessLevel: api.SpaceGroupGrantFormAccessLevel(level)})
				}
			}
			if cmd.Flags().Changed("admin-user") {
				f.AdministratorUserIds = adminUsers
			}
			if cmd.Flags().Changed("admin-group") {
				f.AdministratorGroupIds = adminGroups
			}
			if cmd.Flags().Changed("confirm-own-loss") {
				f.ConfirmOwnLoss = boolPtr(confirmOwnLoss)
			}
			return runWrite(cmd, "normatik spaces access set", "Space access saved.", withSpace(cmd, "normatik spaces access set", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.UpdateSpaceAccess(cmd.Context(), sp.Id, f)
				}))
		},
	}
	set.Flags().StringVar(&mode, "mode", "", "access mode: ALL_USERS|SELECTED")
	set.Flags().StringArrayVar(&users, "user", nil, "user grant <userId>:<READ|READ_WRITE|ADMIN> (repeatable)")
	set.Flags().StringArrayVar(&groups, "group", nil, "group grant <groupId>:<READ|READ_WRITE|ADMIN> (repeatable)")
	set.Flags().Int64SliceVar(&adminUsers, "admin-user", nil, "space administrator user ids (comma-separated or repeated)")
	set.Flags().Int64SliceVar(&adminGroups, "admin-group", nil, "space administrator group ids (comma-separated or repeated)")
	set.Flags().BoolVar(&confirmOwnLoss, "confirm-own-loss", false, "confirm a change that removes your own access")
	set.Flags().StringVarP(&file, "file", "f", "", "JSON form file (SpaceAccessForm) to start from")

	var impactMode string
	var impactUsers, impactGroups []int64
	impact := &cobra.Command{
		Use:   "impact <key> --mode ALL_USERS|SELECTED",
		Short: "Preview how many users gain or lose access if this configuration is saved",
		Args:  cobra.ExactArgs(1),
		Long:  "Pass every user and group that would hold a grant, administrators included.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObject(cmd, "normatik spaces access impact", withSpace(cmd, "normatik spaces access impact", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.PreviewSpaceAccessImpact(cmd.Context(), sp.Id, strings.ToUpper(impactMode), impactUsers, impactGroups)
				}))
		},
	}
	impact.Flags().StringVar(&impactMode, "mode", "", "access mode: ALL_USERS|SELECTED")
	impact.Flags().Int64SliceVar(&impactUsers, "user-id", nil, "ids of every user that would hold a grant (comma-separated or repeated)")
	impact.Flags().Int64SliceVar(&impactGroups, "group-id", nil, "ids of every group that would hold a grant (comma-separated or repeated)")
	_ = impact.MarkFlagRequired("mode")

	c.AddCommand(get, impact)
	addWriteCommands(c, set)
	return c
}

// parseGrant parses "<id>:<level>".
func parseGrant(v string) (int64, string, error) {
	idPart, level, ok := strings.Cut(v, ":")
	id, err := strconv.ParseInt(strings.TrimSpace(idPart), 10, 64)
	level = strings.ToUpper(strings.TrimSpace(level))
	if !ok || err != nil || id <= 0 || (level != "READ" && level != "READ_WRITE" && level != "ADMIN") {
		return 0, "", fmt.Errorf("%q must be <id>:<READ|READ_WRITE|ADMIN>", v)
	}
	return id, level, nil
}

// ---- landing ----

func newSpacesLandingCmd() *cobra.Command {
	c := parent("landing", "Landing page settings of a space by key (get, set)")
	get := &cobra.Command{
		Use: "get <key>", Short: "Get the landing page settings of a space, also of a hidden or deleted one", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObject(cmd, "normatik spaces landing get", withSpace(cmd, "normatik spaces landing get", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.GetSpaceLanding(cmd.Context(), sp.Id)
				}), "active", "content")
		},
	}
	var active bool
	var content string
	set := &cobra.Command{
		Use: "set <key>", Short: "Update the landing page settings of a space (--active, --content; an omitted flag keeps its current value)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("active") && !cmd.Flags().Changed("content") {
				return cmdUsageError(cmd, "nothing to update: pass --active and/or --content")
			}
			return runWrite(cmd, "normatik spaces landing set", "Landing settings updated.", withSpace(cmd, "normatik spaces landing set", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return updateLandingSettings(cmd, active, content,
						func() (client.SettingsSnapshot, *client.APIError) {
							return d.Client.GetSpaceLandingSnapshot(cmd.Context(), sp.Id)
						},
						func(form api.LandingSettingsForm, token string) ([]byte, *client.APIError) {
							return d.Client.UpdateSpaceLanding(cmd.Context(), sp.Id, form, token)
						})
				}), "active", "content")
		},
	}
	set.Flags().BoolVar(&active, "active", false, "landing page active (omitted: keep the current value)")
	set.Flags().StringVar(&content, "content", "", "markdown content of the landing page (omitted: keep the current content; '' clears it)")
	c.AddCommand(get)
	addWriteCommands(c, set)
	return c
}

// landingForm builds the complete LandingSettingsForm for a landing PUT, which
// replaces both fields (NORM-fhfsecoi). A passed flag always wins, so
// an empty --content clears the content; an omitted flag keeps the value from the
// current settings returned by read. A failed read is returned as is, so no PUT
// is sent.
func landingForm(cmd *cobra.Command, active bool, content string, read func() ([]byte, *client.APIError)) (api.LandingSettingsForm, *client.APIError) {
	body, apiErr := read()
	if apiErr != nil {
		return api.LandingSettingsForm{}, apiErr
	}
	var cur api.LandingSettingsResult
	if err := json.Unmarshal(body, &cur); err != nil {
		return api.LandingSettingsForm{}, &client.APIError{Malformed: true, Body: body}
	}
	f := api.LandingSettingsForm{Active: boolPtr(cur.Active), Content: cur.Content}
	if v := changedBool(cmd, "active", active); v != nil {
		f.Active = v
	}
	if v := changedStr(cmd, "content", content); v != nil {
		f.Content = v
	}
	return f, nil
}

// ---- definitions ----

var definitionKinds = []string{client.DefinitionPageTypes, client.DefinitionEnums, client.DefinitionWorkItemTypes}

func newSpacesDefinitionsCmd() *cobra.Command {
	c := parent("definitions", "The page types, enumerations and work item types a space can use (list)")
	var kind string
	list := &cobra.Command{
		Use:     "list <key> --kind page-types|enums|work-item-types",
		Short:   "List the definitions available in a space: the global ones and those local to it, with origin",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik spaces definitions list privacy --kind page-types",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validDefinitionKind(kind) {
				return cmdUsageError(cmd, "--kind must be one of "+strings.Join(definitionKinds, ", "))
			}
			return runList(cmd, "normatik spaces definitions list", withSpace(cmd, "normatik spaces definitions list", args,
				func(d *command.Deps, sp api.SpaceResult) ([]byte, *client.APIError) {
					return d.Client.ListSpaceDefinitions(cmd.Context(), sp.Id, kind)
				}), "id", "name", "local", "originSpace")
		},
	}
	list.Flags().StringVar(&kind, "kind", "", "definition kind: page-types|enums|work-item-types")
	_ = list.MarkFlagRequired("kind")
	c.AddCommand(list)
	return c
}

func validDefinitionKind(k string) bool {
	for _, v := range definitionKinds {
		if k == v {
			return true
		}
	}
	return false
}

// ---- modules ----

func newModulesCmd() *cobra.Command {
	c := parent("modules", "Application modules (list, enable, disable)")
	list := &cobra.Command{
		Use: "list", Short: "List the modules and whether each is enabled",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, "normatik modules list", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.ListModules(cmd.Context())
			}, "name", "enabled")
		},
	}
	mk := func(verb string, enabled bool, short, success string) *cobra.Command {
		return &cobra.Command{
			Use: verb + " <name>", Short: short, Args: cobra.ExactArgs(1),
			Example: "  normatik modules " + verb + " SPACES",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runWrite(cmd, "normatik modules "+verb, success, func(d *command.Deps) ([]byte, *client.APIError) {
					return d.Client.UpdateModule(cmd.Context(), strings.ToUpper(strings.TrimSpace(args[0])), enabled)
				}, "name", "enabled")
			},
		}
	}
	enable := mk("enable", true, "Enable a module (system administrator); Spaces needs a visible default space", "Module enabled.")
	disable := mk("disable", false, "Disable a module (system administrator); spaces, pages and rights are kept", "Module disabled.")
	c.AddCommand(list)
	addWriteCommands(c, enable, disable)
	return c
}

// ---- admin: bootstrap of the first system administrator ----

func newAdminCmd() *cobra.Command {
	c := parent("admin", "Account administration (sys-admin)")
	var email string
	sysAdmin := &cobra.Command{
		Use:   "sys-admin --email <address>",
		Short: "Make a user a system administrator (bootstrap when none exists)",
		Long: "Grants the system administrator role to the user with this e-mail address.\n" +
			"The server decides who may: a system administrator, or an administrator API key only while NO system administrator exists " +
			"(the first one, when the default administrator account is missing). After that a plain administrator key is refused with SYS_ADMIN_REQUIRED.",
		Example: "  normatik admin sys-admin --email beheer@example.org",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWrite(cmd, "normatik admin sys-admin", "System administrator role granted.", func(d *command.Deps) ([]byte, *client.APIError) {
				body, apiErr := d.Client.GetUserByEmail(cmd.Context(), strings.TrimSpace(email))
				if apiErr != nil {
					return nil, apiErr
				}
				var u struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal(body, &u); err != nil || u.ID == 0 {
					return nil, &client.APIError{Malformed: true, Body: body}
				}
				return d.Client.GrantUserSysAdmin(cmd.Context(), u.ID)
			}, "id", "displayName", "email", "role")
		},
	}
	sysAdmin.Flags().StringVar(&email, "email", "", "e-mail address of the user to make system administrator")
	_ = sysAdmin.MarkFlagRequired("email")
	addWriteCommands(c, sysAdmin)
	return c
}

// cmdUsageError renders a usage error (exit 2) in the standard envelope.
func cmdUsageError(cmd *cobra.Command, detail string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	d.Printer.LocalError("USAGE", detail, "", 2)
	return command.Handled(2)
}
