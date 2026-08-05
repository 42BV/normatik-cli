package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// annotateURLHelp appends a discoverability note to every command (group)
// whose subtree registers the --url flag, listing the direct children that
// support it. --url is a local flag by design (CR-besluit 2: unmapped
// commands must reject it as "unknown flag"), so without this note only each
// leaf's own --help would reveal the flag — an agent reading `normatik
// pages -h` would never learn it exists. The annotation is derived from the
// built tree, so newly wired commands appear automatically.
func annotateURLHelp(cmd *cobra.Command) bool {
	has := cmd.Flags().Lookup("url") != nil
	var withURL []string
	for _, c := range cmd.Commands() {
		if annotateURLHelp(c) {
			has = true
			withURL = append(withURL, c.Name())
		}
	}
	if len(withURL) > 0 {
		note := "Deep-links: --url prints only the frontend URL for the resource (wins over -o json and --quiet). Available on: " +
			strings.Join(withURL, ", ") + " — see each command's --help."
		if cmd.Long == "" {
			cmd.Long = cmd.Short + "\n\n" + note
		} else {
			cmd.Long += "\n\n" + note
		}
	}
	return has
}
