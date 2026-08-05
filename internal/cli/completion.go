package cli

import "github.com/spf13/cobra"

// expandSections is the single source for the 6 page-composite expand keys —
// shared by `pages get --expand`, `pages render`, and flag completion.
var expandSections = []string{
	"jira-macros", "workflow", "attachments", "images", "work-items", "restriction",
}

// commonSortKeys is a static, conservative set of frequently whitelisted sort
// keys. Per-endpoint whitelists are authoritative server-side; dynamic, value-
// accurate completion is a phase-2 item (see NORM-degceooe "not in scope").
var commonSortKeys = []string{
	"name", "displayName", "email", "createdAt", "performedAt", "status",
}

// registerCompletions wires static value-completion for known flag values. It
// never makes network calls and never fails command execution: completion
// errors are swallowed (the worst case is no suggestions).
func registerCompletions(root *cobra.Command) {
	_ = root.RegisterFlagCompletionFunc("output", fixedValues("table", "json"))
	_ = root.RegisterFlagCompletionFunc("fields", noFileComp) // free-form; avoid file completion noise
	walkCommands(root, func(c *cobra.Command) {
		if c.Flags().Lookup("expand") != nil {
			_ = c.RegisterFlagCompletionFunc("expand", fixedValues(expandSections...))
		}
		if c.Flags().Lookup("sort") != nil {
			_ = c.RegisterFlagCompletionFunc("sort", fixedValues(commonSortKeys...))
		}
	})
}

func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}

func fixedValues(values ...string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

func noFileComp(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}
