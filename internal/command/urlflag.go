package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// URLFlag registers --url on cmd: instead of the command's normal table/JSON
// output, print only the resolved frontend URL for the touched resource on
// stdout — the shape an agent pipes into `open $(...)` or embeds straight
// into a report (CR-besluit 1). Only commands with a mappable frontend route
// (see internal/weburl) call this; commands without one simply never
// register the flag, so cobra's own "unknown flag" rejects it
// self-documentingly (CR-besluit 2).
//
// Edge-semantics shared by every --url command (CR-besluit 5, C02) live in
// CheckURLDryRun and PrintURL below — call both from RunE:
//   - --url wins over -o json: PrintURL always prints the plain URL, never
//     wrapped in JSON, because it bypasses the Printer's JSON/table branch
//     entirely.
//   - --url survives --quiet: PrintURL never consults Printer.Quiet.
//   - --url + --dry-run (or --preview) is a conflict — those modes perform
//     no write, so there is no resource id to build a URL from.
//     CheckURLDryRun rejects them via the standard error envelope.
//   - Writes: perform the write first, then build the URL from the
//     response's id and call PrintURL — never resolve --url before the
//     write happened.
func URLFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("url", false, "print only the frontend URL for this resource (suppresses normal output; wins over -o json and --quiet)")
}

// CheckURLDryRun rejects --url combined with --dry-run or --preview through
// the standard error envelope (CR-besluit 5/6, C02): those modes never
// perform the write, so there is no resource id to build a URL from. Safe
// to call unconditionally right after Build(), even on commands that never
// register --dry-run or --preview — GetBool then just reports false.
func CheckURLDryRun(d *Deps, cmd *cobra.Command) error {
	url, _ := cmd.Flags().GetBool("url")
	if !url {
		return nil
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	preview, _ := cmd.Flags().GetBool("preview")
	if dryRun {
		d.Printer.Message("Error [USAGE]: --url cannot be combined with --dry-run (a dry-run performs no write, so there is no resource id to build a URL from)")
		return Handled(2)
	}
	if preview {
		d.Printer.Message("Error [USAGE]: --url cannot be combined with --preview (a preview performs no write, so there is no resource id to build a URL from)")
		return Handled(2)
	}
	return nil
}

// PrintURL implements the --url output contract. When --url was requested it
// prints d.BaseURL+path (the resolved frontend URL, e.g. weburl.Page(id)) to
// stdout and reports true, so the caller's RunE can return nil immediately
// with no further output. It reports false, printing nothing, when --url was
// not set — the caller then falls through to its normal Printer output.
// d.BaseURL is already guaranteed non-empty here: Build() fails with exit 78
// before a Deps is ever returned when no environment is configured (same
// precedent as `normatik base-url`, auth.go:405-409).
func PrintURL(d *Deps, cmd *cobra.Command, path string) bool {
	url, _ := cmd.Flags().GetBool("url")
	if !url {
		return false
	}
	fmt.Fprintln(d.Printer.Out, d.BaseURL+path)
	return true
}
