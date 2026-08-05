package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// LeafCommandPaths builds the real cobra command tree (the same tree Main()
// executes against — including cobra's built-in `help`/`completion`, added the
// same way Execute() adds them) and returns the full space-joined path of every
// LEAF command (a command with no children — the only nodes that ever make an
// API call or run their own logic; pure routing parents like `pages` or `users`
// are excluded). Hidden commands (cobra's own `__complete`/`__completeNoDesc`
// shell-completion protocol commands) are excluded too: they are not
// discoverable, agent-facing commands.
//
// This exists so the e2e completeness guard (e2e's INFRA test) can diff the
// CLI's actual command surface against the READ_ONLY read/write matrices
// without parsing --help text (styled/rendered by fang, not a stable format)
// and without re-deriving the tree via source/AST scanning (most `Use:`
// strings are passed through builder helpers as parameters, not literals in a
// `cobra.Command{}` composite literal, so an AST walk over composite literals
// alone would silently miss most leaves).
func LeafCommandPaths() []string {
	cmds := LeafCommands()
	paths := make([]string, 0, len(cmds))
	for _, c := range cmds {
		paths = append(paths, c.Path)
	}
	return paths
}

// writeAnnotationKey marks a command (sub)tree as API-mutating. Every write
// registration site sets it via addWriteCommands/markWriteTree, so the e2e
// completeness guard can classify each leaf as read vs mutating offline and
// name the target READ_ONLY matrix — the RO2 contract (NORM-ogmlitne).
const writeAnnotationKey = "normatik-write"

// markWriteTree annotates cmd and its whole subtree as API-mutating.
func markWriteTree(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[writeAnnotationKey] = "true"
	for _, child := range cmd.Commands() {
		markWriteTree(child)
	}
}

// addWriteCommands marks every given command (subtree) as API-mutating and
// attaches it to parent — the single registration idiom for write commands.
func addWriteCommands(parent *cobra.Command, cmds ...*cobra.Command) {
	for _, c := range cmds {
		markWriteTree(c)
		parent.AddCommand(c)
	}
}

// LeafCommand is one visible leaf of the real command tree with its
// read/mutating classification (from the write annotation, inherited down the
// subtree it was registered under).
type LeafCommand struct {
	Path  string
	Write bool
}

// LeafCommands returns every visible leaf with its classification, sorted by
// path. Same tree semantics as documented above for LeafCommandPaths.
func LeafCommands() []LeafCommand {
	root := newRoot()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var leaves []LeafCommand
	for _, child := range root.Commands() {
		collectLeaves(child, nil, false, &leaves)
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].Path < leaves[j].Path })
	return leaves
}

// collectLeaves walks cmd's subtree, appending every visible, childless command
// with its write classification. segments accumulates the path from (but
// excluding) the root; parentWrite propagates a write annotation set on an
// ancestor (markWriteTree annotates recursively, but children attached AFTER
// marking would otherwise be missed).
func collectLeaves(cmd *cobra.Command, segments []string, parentWrite bool, leaves *[]LeafCommand) {
	if cmd.Hidden {
		return
	}
	write := parentWrite || cmd.Annotations[writeAnnotationKey] == "true"
	name := strings.Fields(cmd.Use)[0]
	path := append(append([]string{}, segments...), name)

	children := cmd.Commands()
	if len(children) == 0 {
		*leaves = append(*leaves, LeafCommand{Path: strings.Join(path, " "), Write: write})
		return
	}
	for _, child := range children {
		collectLeaves(child, path, write, leaves)
	}
}
