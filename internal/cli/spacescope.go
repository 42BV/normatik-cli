package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

// The CLI scope contract (CR spaces, "CLI-scopecontract", N25).
//
// Every space-bound page/workflow command checks the CURRENT module state
// (GET /public/v1/modules) before it does anything else:
//
//   - Spaces enabled: --space <key> is mandatory, also with one space, with an
//     ID-addressed page or parent, with --url and with --dry-run. A missing
//     flag is a usage error (exit 2) without result or mutation.
//   - Spaces disabled: the flag may be left out; the server then uses the
//     default space. Workflow roles are the exception: their API always needs a
//     concrete space, so the CLI translates the default into its explicit key.
//   - The module state could not be read: that is an error, never "disabled".
//   - A given --space is a scope, never a move: it must equal the real space of
//     every page or parent the command addresses, else the action is refused.
//
// The explicit move/restore contracts (pages move-to-space, trash/archive
// restore) keep their own source/target choice and are not scoped by --space.

// spaceScopeEnforced is the test seam of the middleware (same pattern as
// stdinIsTTY): the pre-existing command suites mock one endpoint and would not
// know about the module lookup. Only the scope suites switch it on.
var spaceScopeEnforced = true

const (
	spaceFlagName = "space"

	// ModuleSpaces is the name the Public API gives the Spaces module.
	moduleSpaces = "SPACES"

	spaceRequiredDetail = "--space is required while the Spaces module is enabled."
	spaceRequiredHint   = "Spaces is enabled. Specify --space <key>. Use normatik spaces list to find available spaces."
)

// scopeMode says what a scoped command does with --space.
type scopeMode int

const (
	// scopeApply passes the key to the API as ?space= (lists, search, tree, create, landing).
	scopeApply scopeMode = iota
	// scopeRole is scopeApply for workflow roles: the API always needs a concrete
	// key, so without a flag (module off) the default space's key is filled in.
	scopeRole
	// scopeVerify is for commands that address pages by ID: the flag must match
	// the real space of each addressed page.
	scopeVerify
	// scopeFlagOnly is for commands whose ID is not a page (image, attachment):
	// the flag is required with the module on, there is nothing to compare it to.
	scopeFlagOnly
)

// spaceScope declares, per command path, how --space applies and where the
// page IDs live that must match it.
type spaceScope struct {
	mode scopeMode
	// args are the positional indexes that hold a page ID.
	args []int
	// argsFrom, when > 0, marks every positional argument from index argsFrom-1 on as a page ID.
	argsFrom int
	// flags are the flags (int64, int64 list or string list) that hold page IDs.
	flags []string
}

func applySpace() spaceScope    { return spaceScope{mode: scopeApply} }
func roleSpace() spaceScope     { return spaceScope{mode: scopeRole} }
func flagOnlySpace() spaceScope { return spaceScope{mode: scopeFlagOnly} }
func pageArg() spaceScope       { return spaceScope{mode: scopeVerify, args: []int{0}} }
func pageFlag(names ...string) spaceScope {
	return spaceScope{mode: scopeVerify, flags: names}
}

// spaceScopes is the complete table of scoped commands, keyed by full command
// path. TestSpaceScopeCoversEverySpaceBoundLeaf fails when a leaf under a
// space-bound group is in neither this table nor spaceScopeExempt.
var spaceScopes = map[string]spaceScope{
	"pages list":   applySpace(),
	"pages search": applySpace(),
	"pages tree":   applySpace(),
	// create: a root page lands in --space; a --parent must live in it.
	"pages create":          {mode: scopeApply, flags: []string{"parent"}},
	"pages property-values": {mode: scopeApply, flags: []string{"ids"}},
	// describe-properties without --page is page-type-only but still space-bound
	// (page types are available per space).
	"pages describe-properties":       pageFlag("page"),
	"pages get":                       pageArg(),
	"pages render":                    pageArg(),
	"pages update":                    pageArg(),
	"pages delete":                    pageArg(),
	"pages archive":                   pageArg(),
	"pages cascade-archive":           pageArg(),
	"pages cascade-trash":             pageArg(),
	"pages cascade-impact":            pageArg(),
	"pages move":                      {mode: scopeVerify, args: []int{0}, flags: []string{"parent"}},
	"pages sort-children":             pageArg(),
	"pages migrate-sort-children":     pageArg(),
	"pages migrate-sort-root":         {mode: scopeVerify, argsFrom: 1},
	"pages revisions list":            pageArg(),
	"pages revisions snapshot":        pageArg(),
	"pages revisions start":           pageArg(),
	"pages revisions transition":      pageArg(),
	"pages revisions restore":         pageArg(),
	"pages revisions discard":         pageArg(),
	"pages revisions migrate-publish": pageArg(),
	"pages images list":               pageArg(),
	"pages images upload":             pageArg(),
	"pages images download":           flagOnlySpace(),
	"pages images delete":             flagOnlySpace(),
	"pages attachments upload":        pageArg(),
	"pages attachments download":      flagOnlySpace(),
	"pages attachments delete":        flagOnlySpace(),

	"pages restriction create":              pageArg(),
	"pages restriction remove":              pageArg(),
	"pages restriction access add":          pageArg(),
	"pages restriction access update":       pageArg(),
	"pages restriction access remove":       pageArg(),
	"pages restriction group-access add":    pageArg(),
	"pages restriction group-access update": pageArg(),
	"pages restriction group-access remove": pageArg(),

	"attachments list": pageArg(),

	"work-items list":        pageFlag("page-id"),
	"work-items get":         pageFlag("page-id"),
	"work-items transitions": pageFlag("page-id"),
	"work-items create":      pageFlag("page-id"),
	"work-items edit":        pageFlag("page-id"),
	"work-items delete":      pageFlag("page-id"),
	"work-items comment":     pageFlag("page-id"),
	"work-items transition":  pageFlag("page-id"),

	"landing-settings get":    applySpace(),
	"landing-settings update": applySpace(),

	"workflow-roles get":       roleSpace(),
	"workflow-roles set-user":  roleSpace(),
	"workflow-roles set-group": roleSpace(),
}

// spaceBoundGroups are the top-level groups whose every leaf must be classified.
var spaceBoundGroups = []string{"pages", "attachments", "work-items", "landing-settings", "workflow-roles", "trash", "archive", "workflow"}

// spaceScopeExempt lists the leaves of a space-bound group that deliberately
// take no --space, each with its reason.
var spaceScopeExempt = map[string]string{
	"pages move-to-space": "explicit move contract: --target-space names the destination, the source is the page itself",

	"trash list":                     "administrator lifecycle: lists the trash of all spaces",
	"trash show":                     "administrator lifecycle by page ID",
	"trash restore":                  "administrator restore contract: --target-space only while the original space was deleted",
	"trash purge":                    "administrator lifecycle by page ID",
	"trash cascade-impact":           "administrator lifecycle by page ID",
	"trash cascade-restore":          "administrator restore contract: --target-space only while the original space was deleted",
	"trash cascade-permanent-delete": "administrator lifecycle by page ID",
	"archive list":                   "administrator lifecycle: lists the archive of all spaces",
	"archive show":                   "administrator lifecycle by page ID",
	"archive restore":                "administrator restore contract: --target-space only while the original space was deleted",
	"archive delete":                 "administrator lifecycle by page ID",
	"archive cascade-impact":         "administrator lifecycle by page ID",
	"archive cascade-unarchive":      "administrator restore contract: --target-space only while the original space was deleted",

	"workflow review":  "personal queue across spaces; the API has no space filter",
	"workflow publish": "personal queue across spaces; the API has no space filter",
	"workflow drafts":  "personal queue across spaces; the API has no space filter",
}

// applySpaceScope wires the scope contract into the command tree: every table
// entry gets its --space flag (unless it already has one) and a RunE that runs
// the scope check first. An entry for a command that does not exist is a
// programming error and panics at startup, so a stale table cannot go unnoticed.
func applySpaceScope(root *cobra.Command) {
	for path, spec := range spaceScopes {
		cmd := findCommand(root, strings.Fields(path))
		if cmd == nil || cmd.RunE == nil {
			panic("spaceScopes: no runnable command at path " + strconv.Quote(path))
		}
		if cmd.Flags().Lookup(spaceFlagName) == nil {
			cmd.Flags().String(spaceFlagName, "", spaceFlagUsage(spec))
		}
		cmd.RunE = scoped(spec, cmd.RunE)
	}
}

func spaceFlagUsage(spec spaceScope) string {
	if spec.mode == scopeVerify || len(spec.flags) > 0 {
		return "space key (required while the Spaces module is enabled; must match the space of the pages it addresses)"
	}
	return "space key (required while the Spaces module is enabled)"
}

func findCommand(root *cobra.Command, path []string) *cobra.Command {
	cur := root
	for _, seg := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == seg {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// scoped wraps RunE with the scope check.
func scoped(spec spaceScope, run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if spaceScopeEnforced {
			if err := resolveSpaceScope(cmd, args, spec); err != nil {
				return err
			}
		}
		return run(cmd, args)
	}
}

// spaceOf is the lowercase, trimmed --space value of cmd ("" when not given).
// Commands read their scope through it; the scope check normalises the flag
// (and fills in the default for workflow roles) before RunE runs.
func spaceOf(cmd *cobra.Command) string {
	s, _ := cmd.Flags().GetString(spaceFlagName)
	return strings.ToLower(strings.TrimSpace(s))
}

// resolveSpaceScope enforces the contract for one invocation. It renders its own
// errors and returns a Handled error; nil means the command may run.
func resolveSpaceScope(cmd *cobra.Command, args []string, spec spaceScope) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	invocation := cmd.CommandPath()
	enabled, err := spacesModuleEnabled(cmd.Context(), d, invocation)
	if err != nil {
		return err
	}
	space := spaceOf(cmd)
	if space == "" && enabled {
		d.Printer.LocalError("USAGE", spaceRequiredDetail, spaceRequiredHint, 2)
		return command.Handled(2)
	}
	if space != "" {
		// Normalise: the API takes the lowercase key, ISMS and isms are one space.
		_ = cmd.Flags().Set(spaceFlagName, space)
	}
	if space == "" && spec.mode == scopeRole {
		key, err := defaultSpaceKey(cmd.Context(), d, invocation)
		if err != nil {
			return err
		}
		_ = cmd.Flags().Set(spaceFlagName, key)
	}
	if space == "" {
		return nil // module off, default space: nothing to compare with
	}
	return verifyPageSpaces(cmd, args, spec, d, space)
}

// spacesModuleEnabled reads the Spaces module state. Any failure to learn it is
// an error: an unreadable state must never be treated as "module off".
func spacesModuleEnabled(ctx context.Context, d *command.Deps, invocation string) (bool, error) {
	body, apiErr := d.Client.ListModules(ctx)
	if apiErr != nil {
		return false, command.RenderError(d.Printer, apiErr, invocation)
	}
	// enabled must be an explicit boolean: a missing or null value is an unknown state, not "off".
	var modules []struct {
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(body, &modules); err != nil {
		return false, moduleStateUnknown(d)
	}
	for _, m := range modules {
		if strings.EqualFold(m.Name, moduleSpaces) {
			if m.Enabled == nil {
				return false, moduleStateUnknown(d)
			}
			return *m.Enabled, nil
		}
	}
	return false, moduleStateUnknown(d)
}

func moduleStateUnknown(d *command.Deps) error {
	d.Printer.LocalError("MODULE_STATE_UNKNOWN",
		"the state of the Spaces module could not be determined, so --space cannot be checked.",
		"Run normatik modules list to see the module state, then retry.", 65)
	return command.Handled(65)
}

// defaultSpaceKey finds the key of the default space (module off: the only
// space normal use can reach), for the commands whose API needs a concrete key.
func defaultSpaceKey(ctx context.Context, d *command.Deps, invocation string) (string, error) {
	body, apiErr := d.Client.ListSpaces(ctx, false)
	if apiErr != nil {
		return "", command.RenderError(d.Printer, apiErr, invocation)
	}
	var spaces []api.SpaceResult
	if err := json.Unmarshal(body, &spaces); err != nil {
		return "", moduleStateUnknown(d)
	}
	for _, s := range spaces {
		if s.IsDefault {
			return s.Key, nil
		}
	}
	d.Printer.LocalError("SPACE_NOT_FOUND",
		"there is no default space available to take the workflow role scope from.",
		"Specify --space <key>. Use normatik spaces list to find available spaces.", 6)
	return "", command.Handled(6)
}

// verifyEffectiveCreateParent checks the parent a page create really sends: after the -f
// form and the flags are merged, so a parentId that only lives in the form is scoped too.
// A --parent flag was already checked by the scope check.
func verifyEffectiveCreateParent(cmd *cobra.Command, d *command.Deps, form api.PageCreateForm) error {
	if !spaceScopeEnforced || form.ParentId == nil || cmd.Flags().Changed("parent") {
		return nil
	}
	space := spaceOf(cmd)
	if space == "" {
		return nil
	}
	return verifyIDSpaces(cmd, d, space, []int64{*form.ParentId})
}

// verifyPageSpaces refuses the command when a page or parent it addresses lives
// in another space than --space: the flag scopes, it never moves.
func verifyPageSpaces(cmd *cobra.Command, args []string, spec spaceScope, d *command.Deps, space string) error {
	return verifyIDSpaces(cmd, d, space, scopePageIDs(cmd, args, spec))
}

func verifyIDSpaces(cmd *cobra.Command, d *command.Deps, space string, ids []int64) error {
	for _, id := range ids {
		actual, apiErr := d.Client.GetPageSpaceKey(cmd.Context(), id)
		if apiErr != nil {
			return command.RenderError(d.Printer, apiErr, cmd.CommandPath())
		}
		if actual != space {
			d.Printer.LocalError("SPACE_MISMATCH",
				fmt.Sprintf("page %d is in space %q, not in %q.", id, actual, space),
				"--space only selects the scope of a command and never moves a page. Pass --space "+actual+", or use normatik pages move-to-space to move the page.", 2)
			return command.Handled(2)
		}
	}
	return nil
}

// scopePageIDs collects the distinct page IDs the invocation addresses.
// Arguments that do not parse are skipped: the command itself rejects them.
func scopePageIDs(cmd *cobra.Command, args []string, spec spaceScope) []int64 {
	var ids []int64
	seen := map[int64]bool{}
	add := func(id int64) {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, i := range spec.args {
		if i < len(args) {
			if id, err := command.ParseID(args[i]); err == nil {
				add(id)
			}
		}
	}
	if spec.argsFrom > 0 {
		for i := spec.argsFrom - 1; i < len(args); i++ {
			if id, err := command.ParseID(args[i]); err == nil {
				add(id)
			}
		}
	}
	for _, name := range spec.flags {
		f := cmd.Flags().Lookup(name)
		if f == nil || !f.Changed {
			continue
		}
		for _, id := range idsIn(f.Value.String()) {
			add(id)
		}
	}
	return ids
}

// idsIn extracts the integers of a flag value as pflag prints it: "12",
// "[12,15]" or "12,15".
func idsIn(v string) []int64 {
	var out []int64
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsDigit(r) }) {
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// createSpace is the ?space= of a page create: only a ROOT page takes it. A
// child lives in its parent's space, which the scope check already matched
// against --space.
func createSpace(cmd *cobra.Command, form api.PageCreateForm) string {
	if form.ParentId != nil {
		return ""
	}
	return spaceOf(cmd)
}
