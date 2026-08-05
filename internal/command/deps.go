// Package command is the shared CLI layer: dependency bootstrap (client +
// printer), the single error-render path, exit-code-carrying handled errors,
// and a resource registry + verb-builders so adding a resource is a few lines.
package command

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/42BV/normatik-cli/internal/auth"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/config"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/spf13/cobra"
)

// profileList returns the known profile names, comma-joined and sorted.
func profileList(cfg *config.Config) string {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// HandledError signals that the error was already rendered; Main only maps it to
// an exit code (no double printing).
type HandledError struct{ Code int }

func (e *HandledError) Error() string { return fmt.Sprintf("exit %d", e.Code) }

// Handled wraps an exit code as an already-rendered error.
func Handled(code int) error { return &HandledError{Code: code} }

// Deps holds everything a command needs: an API client, an output printer,
// and the resolved base-URL (CanonicalSiteURL — no trailing slash, no legacy
// /api suffix) that --url commands concatenate a weburl path onto.
type Deps struct {
	Client  *client.Client
	Printer *render.Printer
	BaseURL string
}

// Resolve loads config and resolves the environment (base-URL + key) for the
// given command, honouring the --profile flag, env vars and stored profiles.
func Resolve(cmd *cobra.Command) (*config.Config, auth.Resolved, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, auth.Resolved{}, err
	}
	profile, _ := cmd.Flags().GetString("profile")
	resolved, err := auth.Resolve(cfg, profile)
	return cfg, resolved, err
}

// Build constructs the printer + API client. Output mode: --output flag >
// config.Output > table. Credentials via Resolve (profile/env/keychain).
func Build(cmd *cobra.Command) (*Deps, error) {
	cfg, res, err := Resolve(cmd)
	output, _ := cmd.Flags().GetString("output")
	if !cmd.Flags().Changed("output") && cfg != nil && cfg.Output != "" {
		output = cfg.Output
	}
	p := render.New(output)
	if fields, _ := cmd.Flags().GetStringSlice("fields"); len(fields) > 0 {
		p.Fields = fields
	}
	if q, _ := cmd.Flags().GetBool("quiet"); q {
		p.Quiet = true
	}
	if err != nil {
		p.Message("Error [CONFIG]: could not load config: %v", err)
		return nil, Handled(78)
	}
	if pf, _ := cmd.Flags().GetString("profile"); pf != "" {
		if _, ok := cfg.Profiles[pf]; !ok {
			p.Message("Error [USAGE]: unknown profile %q. Known: %s", pf, profileList(cfg))
			p.Message("  Try:  normatik auth list  ·  normatik login --profile %s --url ... --paste", pf)
			return nil, Handled(2)
		}
	}
	if res.BaseURL == "" {
		p.Message("Error [CONFIG]: no environment configured -- run: normatik login   (or pass --url, or set NORMATIK_BASE_URL)")
		return nil, Handled(78) // EX_CONFIG
	}
	if res.APIKey == "" {
		p.Message("Error [NO_API_KEY]: no API key found. Log in or set NORMATIK_API_KEY.")
		p.Message("  Try:  normatik login --url https://wiki.example/ --paste")
		p.Message("  or:   export NORMATIK_API_KEY=wiki_...")
		return nil, Handled(78) // EX_CONFIG
	}
	c, err := client.New(res.BaseURL, res.APIKey)
	if err != nil {
		p.Message("Error [CONFIG]: %v", err)
		return nil, Handled(78)
	}
	return &Deps{Client: c, Printer: p, BaseURL: res.BaseURL}, nil
}

// RenderError is the single error-render route for every command: structured
// ProblemDetail + hint + synthesized next-command, a malformed-response class,
// or a transport failure. It returns a Handled error carrying the exit code.
// invocation is the failed command path (e.g. "normatik pages list"), used to
// synthesize a runnable "Try:" suggestion — not the base-URL.
func RenderError(p *render.Printer, e *client.APIError, invocation string) error {
	switch {
	case e.Problem != nil:
		p.Problem(e.Problem, e.Problem.Suggestion(invocation))
		return Handled(e.Problem.ExitCode())
	case e.TooLarge != nil:
		p.Message("Error [RESPONSE_TOO_LARGE]: HTTP %d response exceeds the %d-byte safety limit.", e.Status, e.TooLarge.Limit)
		return Handled(65)
	case e.Malformed:
		p.Malformed(e.Status, e.Body)
		return Handled(65) // EX_DATAERR — server gaf geen ProblemDetail terug
	default:
		p.Message("Error [TRANSPORT]: %v", e.Error())
		p.Message("  Hint: is the backend running on the correct base-URL?")
		return Handled(1)
	}
}

// ParseID parses a positional <id> argument.
func ParseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

// UnknownSub is the RunE for parent commands (no own action): show help when
// called bare, or reject an unknown subcommand with did-you-mean suggestions.
func UnknownSub(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	if sugg := cmd.SuggestionsFor(args[0]); len(sugg) > 0 {
		msg += "\n\nDid you mean this?"
		for _, s := range sugg {
			msg += "\n\t" + s
		}
	}
	msg += fmt.Sprintf("\n\nRun '%s --help' for the available commands.", cmd.CommandPath())
	return errors.New(msg)
}
