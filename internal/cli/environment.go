package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/auth"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/config"
	"github.com/42BV/normatik-cli/internal/httpx"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/spf13/cobra"
)

// ---- environment (name/banner, Google login, preparation readiness — admin only;
// plus the one-time environment-seed lifecycle, see addEnvironmentWrites) ----
//
// Three admin-secured sub-resources under one noun, each mapping onto its own
// public-API controller: settings (name + banner-alert toggle), google-login
// (status/disable, never returns clientId/clientSecret) and readiness
// (business-data counts + environment-seed status, used by the orchestrator
// preflight). All three are @RequireAdmin on the backend service, unlike the
// internal UI's environment-settings GET which every authenticated user can read.

func newEnvironmentCmd() *cobra.Command {
	c := parent("environment", "Environment name/banner, Google login, preparation readiness (admin) and the one-time environment-seed lifecycle")

	settings := parent("settings", "Environment name and banner-alert toggle (get, set)")
	settingsGet := &cobra.Command{
		Use: "get", Short: "Current environment name and banner-alert toggle",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObject(cmd, "normatik environment settings get", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetEnvironmentSettings(cmd.Context())
			})
		},
	}
	settings.AddCommand(settingsGet)
	c.AddCommand(settings)

	googleLogin := parent("google-login", "Google-login status (status, disable)")
	googleLoginStatus := &cobra.Command{
		Use: "status", Short: "Current Google-login status (never returns clientId/clientSecret)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObject(cmd, "normatik environment google-login status", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetGoogleLoginStatus(cmd.Context())
			})
		},
	}
	googleLogin.AddCommand(googleLoginStatus)
	c.AddCommand(googleLogin)

	readiness := &cobra.Command{
		Use: "readiness", Short: "Preparation-readiness overview: business-data counts + environment-seed status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObject(cmd, "normatik environment readiness", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetPreparationReadiness(cmd.Context())
			})
		},
	}
	c.AddCommand(readiness)

	seedStatus := &cobra.Command{
		Use:   "seed-status",
		Short: "Environment-seed transfer state: status, registered transfer identifiers, and whether an eligible replacement administrator exists (bootstrap account only)",
		Long: "Calls GET /environment-seed. Read-only — never mutates anything. Lets the preparation " +
			"orchestrator's self-removal preflight check its local verification report's run-ID/digest " +
			"against what is actually registered, and whether `environment seed-complete` would find an " +
			"eligible replacement administrator, before ever attempting that mutating call. Never reveals " +
			"which user, or any password state, is behind hasEligibleReplacementAdmin.",
		Example: "  normatik environment seed-status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObject(cmd, "normatik environment seed-status", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.GetEnvironmentSeedReadiness(cmd.Context())
			}, "status", "transferRunId", "transferReportDigest", "hasEligibleReplacementAdmin")
		},
	}
	c.AddCommand(seedStatus)

	return c
}

// addEnvironmentWrites attaches `settings set` and `google-login disable` onto
// the sub-parents newEnvironmentCmd already built (found by name — settings/
// google-login carry both a read and a write verb, unlike the write-only
// sub-parents elsewhere in this package that are built from scratch here).
func addEnvironmentWrites(c *cobra.Command) {
	var setName string
	var setAlertText string
	var setAlertEnabled bool
	set := &cobra.Command{
		Use: "set", Short: "Update the environment name and/or banner-alert toggle (--name, --alert-enabled)",
		Long: "Updates the environment settings singleton. This is a full replace: an omitted flag\n" +
			"resets that field (name to empty, the banner to disabled) rather than leaving it\n" +
			"untouched — pass both flags to change one without resetting the other.\n" +
			"--alert-text is an alias for --name (the backend has one field for both the About-page\n" +
			"name and the banner text); the two are mutually exclusive.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.EnvironmentSettingsForm{}
			if cmd.Flags().Changed("name") {
				f.Name = strPtr(setName)
			}
			if cmd.Flags().Changed("alert-text") {
				f.Name = strPtr(setAlertText)
			}
			if cmd.Flags().Changed("alert-enabled") {
				f.Enabled = boolPtr(setAlertEnabled)
			}
			return runWrite(cmd, "normatik environment settings set", "Environment settings updated.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.UpdateEnvironmentSettings(cmd.Context(), f)
			})
		},
	}
	set.Flags().StringVar(&setName, "name", "", "environment name (shown on the About page, and as the banner text when the alert is enabled)")
	set.Flags().StringVar(&setAlertText, "alert-text", "", "banner alert text (alias for --name; same underlying field, mutually exclusive with --name)")
	set.Flags().BoolVar(&setAlertEnabled, "alert-enabled", false, "show the environment name as a banner across the top of the app")
	set.MarkFlagsMutuallyExclusive("name", "alert-text")

	disable := &cobra.Command{
		Use: "disable", Short: "Disable Google login (idempotent; no matching enable on this route — use the admin UI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWrite(cmd, "normatik environment google-login disable", "Google login disabled.", func(d *command.Deps) ([]byte, *client.APIError) {
				return d.Client.DisableGoogleLogin(cmd.Context())
			})
		},
	}

	addWriteCommands(findChild(c, "settings"), set)
	addWriteCommands(findChild(c, "google-login"), disable)

	// One-time environment-seed lifecycle: bootstrap the temporary administrator
	// account on an empty target, register/revoke a strictly verified transfer,
	// and complete the seed (removing the bootstrap account and its API keys).
	// Every leaf mutates the target.
	addWriteCommands(c,
		newEnvironmentBootstrapCmd(),
		newEnvironmentTransferVerificationCmd(),
		newEnvironmentSeedCompleteCmd())
}

// findChild returns the direct subcommand of c named name. Panics if absent —
// a programmer error (a write-registration function wired to the wrong noun),
// never a runtime condition.
func findChild(c *cobra.Command, name string) *cobra.Command {
	for _, child := range c.Commands() {
		if child.Name() == name {
			return child
		}
	}
	panic("cli: expected child command " + name + " under " + c.Name())
}

// readBootstrapCredentials reads the fixed bootstrap secret, the new
// administrator's email and its temporary password WITHOUT ever taking them
// from argv or an environment variable (N26): on a TTY, three hidden prompts
// (term.ReadPassword, same mechanism as promptSecret); off a TTY
// (piped/CI/orchestrator), three bounded, non-empty stdin lines in that exact
// order. Package var so tests can drive the value without a real terminal
// (same seam pattern as readNewUserPassword / promptSecretKey).
var readBootstrapCredentials = func() (secret, email, password string, err error) {
	if isTTY(os.Stdin) {
		if secret, err = promptSecret("Bootstrap secret: "); err != nil {
			return "", "", "", err
		}
		if email, err = promptSecret("New administrator email: "); err != nil {
			return "", "", "", err
		}
		if password, err = promptSecret("Temporary password: "); err != nil {
			return "", "", "", err
		}
	} else {
		r := bufio.NewReader(io.LimitReader(os.Stdin, 3*maxPasswordBytes))
		if secret, err = readBootstrapLine(r); err != nil {
			return "", "", "", err
		}
		if email, err = readBootstrapLine(r); err != nil {
			return "", "", "", err
		}
		if password, err = readBootstrapLine(r); err != nil {
			return "", "", "", err
		}
	}
	if secret == "" || email == "" || password == "" {
		return "", "", "", errors.New("secret, email and password must all be non-empty")
	}
	return secret, email, password, nil
}

// readBootstrapLine reads one line off a bounded reader, trimming the trailing
// newline; EOF on a line that already holds data is not an error (mirrors
// readNewUserPassword's bufio.ReadString('\n') handling).
func readBootstrapLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func newEnvironmentBootstrapCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "One-time bootstrap of the temporary administrator account (anonymous, empty target only)",
		Long: "Calls the anonymous POST /environment-seed/bootstrap on --url. Succeeds exactly once, only on " +
			"a target with no users or pages yet and the correct fixed bootstrap secret; repeated or " +
			"concurrent calls never create a second account. The secret, the new administrator's email and " +
			"its temporary password are read via a hidden prompt (or three bounded stdin lines, in that " +
			"order, when stdin is not a terminal) -- never via argv or an environment variable, and never " +
			"echoed back or included in any output. Bootstrap itself does not log in: use the normal " +
			"`normatik login` browser or paste flow afterwards.",
		Example: "  normatik environment bootstrap --url https://wiki.example/\n" +
			"  printf '%s\\n%s\\n%s\\n' \"$SECRET\" \"$EMAIL\" \"$PASSWORD\" | \\\n" +
			"    normatik environment bootstrap --url https://wiki.example/ --no-input",
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			site := normalizeLoginURL(strings.TrimSpace(url))
			if site == "" {
				p.Message("Error [USAGE]: --url is required.")
				return command.Handled(2)
			}
			if err := httpx.ValidateBaseURL(site); err != nil {
				p.Message("Error [USAGE]: %v", err)
				return command.Handled(2)
			}
			secret, email, password, rerr := readBootstrapCredentials()
			if rerr != nil {
				p.Message("Error [USAGE]: could not read the bootstrap secret/email/password: %v", rerr)
				return command.Handled(2)
			}
			result, apiErr := client.Bootstrap(cmd.Context(), site, secret, email, password)
			if apiErr != nil {
				return command.RenderError(p, apiErr, "normatik environment bootstrap")
			}
			body, merr := json.Marshal(result)
			if merr != nil {
				p.Message("Error [TRANSPORT]: could not encode the bootstrap result: %v", merr)
				return command.Handled(1)
			}
			p.Raw(body, "userId", "email")
			p.Message("Bootstrap account created. Log in with: normatik login --url %s", site)
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "target environment site URL (required)")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newEnvironmentTransferVerificationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer-verification",
		Short: "Register or revoke a strictly verified environment-seed transfer (bootstrap account only)",
		RunE:  command.UnknownSub,
	}
	cmd.AddCommand(newEnvironmentTransferVerificationRegisterCmd())
	cmd.AddCommand(newEnvironmentTransferVerificationRevokeCmd())
	return cmd
}

func newEnvironmentTransferVerificationRegisterCmd() *cobra.Command {
	var runID, digest string
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a strictly verified transfer (readback run-ID + report digest)",
		Long: "POST /public/v1/environment-seed/transfer-verification. Moves the environment-seed status to " +
			"TRANSFER_VERIFIED, recording the readback run-ID and report digest. Bootstrap-account only; " +
			"refuses once the seed is COMPLETED.",
		Example: "  normatik environment transfer-verification register --run-id 2026-09-26T12:00:00Z-a1b2c3 --digest 4f8c9e...e2",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := api.TransferVerificationForm{RunId: runID, ReportDigest: digest}
			return runWrite(cmd, "normatik environment transfer-verification register", "Transfer verification registered.",
				func(d *command.Deps) ([]byte, *client.APIError) {
					return d.Client.RegisterEnvironmentSeedTransferVerification(cmd.Context(), f)
				}, "status", "transferRunId", "transferReportDigest", "transferVerifiedAt")
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "readback run-ID of the verified transfer (required)")
	cmd.Flags().StringVar(&digest, "digest", "", "sha256 digest of the verification report (required)")
	_ = cmd.MarkFlagRequired("run-id")
	_ = cmd.MarkFlagRequired("digest")
	return cmd
}

func newEnvironmentTransferVerificationRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a registered transfer verification (bootstrap account only)",
		Long: "DELETE /public/v1/environment-seed/transfer-verification. Moves the status back to PENDING; " +
			"idempotent when already PENDING, refuses once COMPLETED. Used by an import reset after a " +
			"failed/injected import.",
		Example: "  normatik environment transfer-verification revoke",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWrite(cmd, "normatik environment transfer-verification revoke", "Transfer verification revoked.",
				func(d *command.Deps) ([]byte, *client.APIError) {
					return d.Client.RevokeEnvironmentSeedTransferVerification(cmd.Context())
				}, "status")
		},
	}
}

func newEnvironmentSeedCompleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed-complete",
		Short: "Complete the environment seed and remove the logged-in bootstrap account (irreversible)",
		Long: "Recognizes the logged-in account via GET /users/me, then calls POST /environment-seed/complete: " +
			"the server verifies the caller is the bootstrap account, the transfer is TRANSFER_VERIFIED and at " +
			"least one other ACTIVE user has a local password and effective ADMIN + PUBLISHER roles, before " +
			"completing the seed and permanently removing the calling account and all its API keys in one " +
			"transaction. Only after the server confirms success does the CLI remove the local profile (the " +
			"config.toml entry and the OS-keychain key) -- a rejected or failed call leaves the profile intact.",
		Example: "  normatik environment seed-complete --confirm=bootstrap@example.com",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			meBody, apiErr := d.Client.Me(cmd.Context())
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik environment seed-complete")
			}
			var me struct {
				Email string `json:"email"`
			}
			if json.Unmarshal(meBody, &me) != nil || me.Email == "" {
				d.Printer.Message("Error [TRANSPORT]: could not determine the logged-in account's email from GET /users/me.")
				return command.Handled(65)
			}
			if e := confirmHard(cmd, d, me.Email); e != nil {
				return e
			}
			body, apiErr := d.Client.CompleteEnvironmentSeed(cmd.Context())
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik environment seed-complete")
			}
			profile := profileName(cmd)
			clearLocalProfile(d, profile)
			d.Printer.Raw(body, "status", "completedAt")
			d.Printer.Message("Bootstrap account removed; local profile %q cleared. Log in as another administrator to continue.", profile)
			return nil
		},
	}
	addHardConfirm(cmd)
	return cmd
}

// clearLocalProfile removes profile's keychain key and config.toml entry AFTER
// a confirmed server-side success -- never before. Failures here are reported
// as warnings, not errors: the destructive server-side action already
// succeeded, so the process must not exit non-zero over stale local state.
func clearLocalProfile(d *command.Deps, profile string) {
	if err := auth.DeleteKeyIfPresent(profile); err != nil {
		d.Printer.Message("Warning: the bootstrap account was removed, but the local keychain key for profile %q could not be deleted: %v", profile, err)
	}
	cfg, err := config.Load()
	if err != nil {
		d.Printer.Message("Warning: the bootstrap account was removed, but the local config could not be read to clear profile %q: %v", profile, err)
		return
	}
	if _, ok := cfg.Profiles[profile]; !ok {
		return
	}
	delete(cfg.Profiles, profile)
	if cfg.ActiveProfile == profile {
		cfg.ActiveProfile = ""
	}
	if err := cfg.Save(); err != nil {
		d.Printer.Message("Warning: the bootstrap account was removed, but the local profile %q could not be cleared from config: %v", profile, err)
	}
}
