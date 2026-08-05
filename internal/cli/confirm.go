package cli

import (
	"os"

	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// addSoftConfirm registers a boolean --confirm flag for reversible destructive
// ops (e.g. soft delete → trash).
func addSoftConfirm(cmd *cobra.Command) {
	cmd.Flags().Bool("confirm", false, "confirm this destructive action")
}

// addHardConfirm registers a string --confirm flag for irreversible ops; the
// value must match the resource identifier.
func addHardConfirm(cmd *cobra.Command) {
	cmd.Flags().String("confirm", "", "confirm with the exact id/slug (irreversible)")
}

// nonInteractive reports whether confirmation can never be supplied interactively
// for this run: either the global --no-input flag is set, or stdin is not a TTY
// (piped / CI / agent). In that case a missing --confirm must fail closed rather
// than appear to hang waiting for input.
// stdinIsTTY reports whether stdin is an interactive terminal. term.IsTerminal
// does a real terminal probe (ioctl), so a non-TTY character device such as
// /dev/null correctly counts as non-interactive. A plain os.ModeCharDevice test
// does NOT: /dev/null is a character device on both macOS and Linux, so
// `login < /dev/null` would falsely look interactive and try to prompt instead
// of failing closed. Package var so tests can simulate a TTY (a test process
// never has a real terminal on stdin).
var stdinIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// stdoutIsTTY reports whether stdout is an interactive terminal, using the same
// real terminal probe as stdinIsTTY (term.IsTerminal via ioctl). A plain
// os.ModeCharDevice test would count /dev/null as a terminal; browserCapable
// uses this so `login >/dev/null` is not mistaken for a browser-capable stdout.
// Package var so tests can simulate a terminal.
var stdoutIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func nonInteractive(cmd *cobra.Command) bool {
	if ni, _ := cmd.Flags().GetBool("no-input"); ni {
		return true
	}
	return !stdinIsTTY()
}

func confirmSoft(cmd *cobra.Command, d *command.Deps) error {
	if ok, _ := cmd.Flags().GetBool("confirm"); ok {
		return nil
	}
	if nonInteractive(cmd) {
		d.Printer.Message("Error [CONFIRM]: destructive action without interactive input (non-TTY/--no-input). Confirm explicitly with --confirm.")
	} else {
		d.Printer.Message("Error [CONFIRM]: this action is destructive; confirm with --confirm")
	}
	return command.Handled(2)
}

func confirmHard(cmd *cobra.Command, d *command.Deps, expected string) error {
	val, _ := cmd.Flags().GetString("confirm")
	if val == "" {
		if nonInteractive(cmd) {
			d.Printer.Message("Error [CONFIRM]: IRREVERSIBLE action without interactive input (non-TTY/--no-input). Confirm explicitly with --confirm=%q.", expected)
		} else {
			d.Printer.Message("Error [CONFIRM]: this action is IRREVERSIBLE; confirm with --confirm=%q", expected)
		}
		return command.Handled(2)
	}
	if val != expected {
		d.Printer.Message("Error [CONFIRM]: --confirm=%q does not match %q", val, expected)
		return command.Handled(2)
	}
	return nil
}
