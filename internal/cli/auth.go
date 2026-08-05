package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/42BV/normatik-cli/internal/auth"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/config"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// loginMethod is the authentication method a login run uses: paste an existing
// API key, or mint a fresh one through the browser approval flow.
type loginMethod int

const (
	methodPaste loginMethod = iota
	methodBrowser
	maxStdinAPIKeyBytes = 8192
)

// profileName resolves the profile a login/logout targets: the --profile flag
// wins; without it the ACTIVE profile is used (so `auth use prod; logout` logs
// out prod, not literally "default"); a fresh install without an active profile
// falls back to "default".
func profileName(cmd *cobra.Command) string {
	p, _ := cmd.Flags().GetString("profile")
	if p != "" {
		return p
	}
	if cfg, err := config.Load(); err == nil && cfg.ActiveProfile != "" {
		return cfg.ActiveProfile
	}
	return "default"
}

func newLoginCmd() *cobra.Command {
	var url string
	var readOnly, noBrowser, browserFlag, pasteFlag, keyStdin bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to a Normatik environment (interactive wizard, or scripted via flags)",
		Long: "On a terminal, a bare `normatik login` is a wizard: it asks for the environment URL and then\n" +
			"shows a menu to pick how you authenticate — paste an existing API key, or create a key in\n" +
			"your browser. On a real terminal use the arrow keys (or 1/2) and Enter to choose, with the\n" +
			"browser flow preselected; non-interactively the browser flow is the scriptable default.\n\n" +
			"Browser flow: the CLI starts a login on the server, you approve it in the browser, and the CLI\n" +
			"receives a fresh API key (never shown on screen). Paste flow: you supply an existing key via a\n" +
			"hidden prompt, or via --key-stdin in a pipeline. Both paths validate via GET /api/public/v1/users/me\n" +
			"and store the site URL in config.toml + the key in\n" +
			"the OS keychain. With --no-browser (or a browserless environment: no TTY, SSH, CI) only the\n" +
			"approval URL is printed; polling continues either way.\n\n" +
			"Method precedence: an explicit method flag (--browser / --paste / --key-stdin) wins and skips\n" +
			"the menu (--key-stdin implies --paste); otherwise the interactive menu decides; non-interactively\n" +
			"(--no-input, no TTY, CI) there is no menu and the scriptable default is the browser flow.\n\n" +
			"Environment URL: without --url the CLI asks for it on a terminal (prefilled with the current\n" +
			"environment when a profile/NORMATIK_BASE_URL is known, so pressing enter confirms it). There is\n" +
			"no localhost default: non-interactively (--no-input, no TTY, CI) you must pass --url or set\n" +
			"NORMATIK_BASE_URL.",
		Example: "  normatik login\n" +
			"  normatik login --url https://wiki.example/ --browser --read-only\n" +
			"  normatik login --url https://wiki.example/ --no-browser\n" +
			"  normatik login --url https://wiki.example/ --paste\n" +
			"  printf '%s\\n' \"$KEY\" | normatik login --url https://wiki.example/ --key-stdin --no-input",
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			profile := profileName(cmd)

			// V3-wizard stap 1: bepaal de omgeving-URL — gedeeld door het browser-
			// en het paste-pad. Met --url wint de vlag; zonder --url wordt op een
			// TTY om de URL gevraagd (met de resolvbare base als prefill) en
			// non-interactief de resolvbare base gebruikt of een nette fout gegeven.
			site, err := resolveLoginURL(cmd, p, url)
			if err != nil {
				return err
			}

			// V3a-wizard stap 2: kies de authenticatie-methode. Voorrang:
			// expliciete methode-vlag (--browser/--paste/--key-stdin) > interactief
			// menu > non-interactieve default (browser, de scriptbare keuze).
			method, decided := methodFromFlags(keyStdin, pasteFlag, browserFlag)
			if !decided {
				if nonInteractive(cmd) {
					method = methodBrowser // scriptbaar: geen menu, geen prompt
				} else if m, merr := promptMethodChoice(p); merr != nil {
					// EOF/Ctrl-D of een TTY-leesfout bij het methode-menu:
					// behandel als annuleren i.p.v. stil de side-effecting
					// browser-flow te starten (die mint server-side een key).
					// Consistent met de URL-prompt en confirmProfileOverwrite,
					// die EOF ook als afbreken behandelen.
					p.Message("Login aborted — no login method chosen.")
					return command.Handled(2)
				} else {
					method = m
				}
			}

			if method == methodPaste {
				return runPasteLogin(cmd, p, profile, site, keyStdin)
			}
			return runBrowserLogin(cmd, p, profile, site, readOnly, noBrowser)
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "environment site URL, e.g. https://wiki.acme.com (/api is added by the transport)")
	cmd.Flags().BoolVar(&browserFlag, "browser", false, "force the browser flow (skip the interactive method menu)")
	cmd.Flags().BoolVar(&pasteFlag, "paste", false, "force pasting a key through a hidden terminal prompt (skip the method menu)")
	cmd.Flags().BoolVar(&keyStdin, "key-stdin", false, "read one API-key line from non-interactive stdin; implies --paste")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "browser flow: suggest a read-only key on the approval page")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "browser flow: do not open a browser, only print the approval URL")
	// --browser is the opposite of paste/key-stdin; reject contradictory combos
	// up front rather than silently picking one.
	cmd.MarkFlagsMutuallyExclusive("browser", "paste")
	cmd.MarkFlagsMutuallyExclusive("browser", "key-stdin")
	return cmd
}

// methodFromFlags returns the login method fixed by an explicit flag, plus a
// bool that is true only when a flag actually pins the method. --browser forces
// the browser flow; --paste and --key-stdin force the paste flow. When nothing
// pins the method the choice is left to the interactive menu or the
// non-interactive default. Contradictory flags are rejected earlier by cobra.
func methodFromFlags(keyStdin, paste, browser bool) (loginMethod, bool) {
	switch {
	case browser:
		return methodBrowser, true
	case paste || keyStdin:
		return methodPaste, true
	default:
		return methodPaste, false
	}
}

// runPasteLogin is the shared paste dispatch used by --paste, --key-stdin and
// menu choice 1. Interactive input always uses the hidden terminal prompt;
// piped input is accepted only through the explicit, bounded --key-stdin path.
func runPasteLogin(cmd *cobra.Command, p *render.Printer, profile, site string, keyStdin bool) error {
	if keyStdin {
		if stdinIsTTY() {
			p.Message("Error [USAGE]: --key-stdin requires piped, non-interactive stdin.")
			return command.Handled(2)
		}
		key, err := readAPIKeyStdin(cmd.InOrStdin())
		if err != nil {
			p.Message("Error [USAGE]: could not read API key from stdin: %v", err)
			return command.Handled(2)
		}
		return finishLogin(cmd, p, profile, site, key, false)
	}

	// Menu option 1 or --paste: prompt for the key on a real TTY.
	if nonInteractive(cmd) {
		p.Message("Error [USAGE]: paste login needs an interactive terminal; pipe one key line to --key-stdin, or use NORMATIK_API_KEY for normal CI commands.")
		return command.Handled(2)
	}
	v, perr := promptSecretKey()
	if perr != nil {
		p.Message("Error [USAGE]: could not read the API key (%v); use --key-stdin with piped input, or NORMATIK_API_KEY for normal CI commands.", perr)
		return command.Handled(2)
	}
	key := strings.TrimSpace(v)
	if key == "" {
		p.Message("Error [USAGE]: API key must not be empty")
		return command.Handled(2)
	}
	return finishLogin(cmd, p, profile, site, key, false)
}

func readAPIKeyStdin(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxStdinAPIKeyBytes+1))
	if err != nil {
		return "", errors.New("stdin read failed")
	}
	if len(data) > maxStdinAPIKeyBytes {
		return "", fmt.Errorf("stdin exceeds %d bytes", maxStdinAPIKeyBytes)
	}
	lines := strings.Split(string(data), "\n")
	key := strings.TrimSpace(lines[0])
	if key == "" {
		return "", errors.New("stdin must contain a non-empty API key line")
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" {
			return "", errors.New("stdin must not contain a second non-empty line")
		}
	}
	return key, nil
}

// finishLogin is the shared tail of both login paths (paste + browser):
// canonicalize the site URL, validate the key via /api/public/v1/users/me, run the overwrite-confirm,
// store key + config transactionally and switch the active profile. The key is
// never printed. serverMinted is true only on the browser path, where the server
// already created a fresh API key that the CLI never shows: if this tail then
// fails, that key is orphaned and the user must revoke it in their Profile — the
// NORMATIK_API_KEY hint is meaningless there because the key value was never seen.
func finishLogin(cmd *cobra.Command, p *render.Printer, profile, url, key string, serverMinted bool) error {
	// revokeHint warns that a server-minted key is now dangling. Only emitted on
	// the browser path; on the paste path the user already holds the key so there
	// is nothing new to revoke.
	revokeHint := func() {
		if serverMinted {
			p.Message("  Note: a new API key was already created on the server but not stored. Revoke it in your Profile if this login did not complete.")
		}
	}

	base, err := auth.Detect(cmd.Context(), url, key, nil)
	if err != nil {
		p.Message("Error [LOGIN_FAILED]: %v", err)
		p.Message("  Hint: use an https:// environment URL and check whether the API key is valid.")
		revokeHint()
		return command.Handled(4)
	}

	// Een corrupte/onleesbare config.toml mag geen panic geven op de
	// overwrite-check hieronder: expliciet afbreken zoals auth use/list/remove.
	cfg, err := config.Load()
	if err != nil {
		p.Message("Error [CONFIG]: %v", err)
		revokeHint()
		return command.Handled(78)
	}
	// Bestaand profiel dat naar een ANDERE base-URL zou gaan wijzen:
	// op een TTY eerst bevestigen; non-interactief gaat door (scriptpad).
	if existing, ok := cfg.Profiles[profile]; ok && existing.BaseURL != "" && existing.BaseURL != base {
		label := fmt.Sprintf("Profile %q currently points to %s — overwrite with %s? [y/N]: ",
			profile, existing.BaseURL, base)
		if !confirmProfileOverwrite(cmd, label) {
			p.Message("Login aborted — profile %q keeps %s.", profile, existing.BaseURL)
			revokeHint()
			return command.Handled(2)
		}
	}

	// Transactioneel: schrijf eerst de key naar de keychain. Faalt dat
	// (bv. headless/CI zonder Secret Service), dan raken we config NIET aan
	// — geen half-ingelogd profiel.
	if err := auth.SetKey(profile, key); err != nil {
		p.Message("Error [KEYCHAIN]: could not store key: %v", err)
		if serverMinted {
			// The browser path never showed the key, so NORMATIK_API_KEY is
			// impossible to act on here — point at revocation instead.
			revokeHint()
		} else {
			p.Message("  Hint: on headless/CI use NORMATIK_API_KEY instead of login.")
		}
		return command.Handled(78)
	}
	cfg.SetProfile(profile, base)
	// Een succesvolle login maakt het zojuist ingelogde profiel actief,
	// zodat volgende commands zonder --profile dit profiel raken.
	cfg.ActiveProfile = profile
	if err := cfg.Save(); err != nil {
		_ = auth.DeleteKey(profile) // rollback de zojuist geschreven key
		p.Message("Error [CONFIG]: could not save config: %v", err)
		revokeHint()
		return command.Handled(78)
	}

	safeProfile := render.SafeLine(profile)
	safeBase := render.SafeLine(base)
	suffix := render.Styled(styleMuted.Render(fmt.Sprintf("profile %q · %s", safeProfile, safeBase)))
	if who := whoamiName(cmd, base, key); who != "" {
		success := render.Styled(styleSuccess.Render("✓ Logged in as " + render.SafeLine(who)))
		p.Message("%s  %s", success, suffix)
	} else {
		success := render.Styled(styleSuccess.Render("✓ Logged in"))
		p.Message("%s  %s", success, suffix)
	}
	p.Message("Active profile is now %q.", profile)
	return nil
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Remove the stored API key of a profile",
		Long:    "Removes only the keychain key of the profile; the profile (base-URL) stays in config.toml and remains visible in `auth list`.",
		Example: "  normatik logout\n  normatik logout --profile prod",
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			profile := profileName(cmd)
			if err := auth.DeleteKey(profile); err != nil {
				p.Message("Error [KEYCHAIN]: no stored key for profile %q (%v)", profile, err)
				return command.Handled(1)
			}
			p.Message("Logged out — key for profile %q removed.", profile)
			return nil
		},
	}
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage environment profiles (use, list, remove)",
		RunE:  command.UnknownSub,
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "use <profile>",
		Short:   "Set the active profile",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik auth use prod",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			cfg, err := config.Load()
			if err != nil {
				p.Message("Error [CONFIG]: %v", err)
				return command.Handled(78)
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				p.Message("Error [USAGE]: unknown profile %q. Known: %s", args[0], strings.Join(profileNames(cfg), ", "))
				return command.Handled(2)
			}
			cfg.ActiveProfile = args[0]
			if err := cfg.Save(); err != nil {
				p.Message("Error [CONFIG]: %v", err)
				return command.Handled(78)
			}
			p.Message("Active profile: %q", args[0])
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "remove <profile>",
		Short:   "Remove a profile (config entry + keychain key)",
		Long:    "Removes the profile from config.toml and its API key from the OS keychain. An absent keychain key is not an error. Removing the active profile clears the active-profile pointer.",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik auth remove staging",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			cfg, err := config.Load()
			if err != nil {
				p.Message("Error [CONFIG]: %v", err)
				return command.Handled(78)
			}
			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				p.Message("Error [USAGE]: unknown profile %q. Known: %s", name, strings.Join(profileNames(cfg), ", "))
				return command.Handled(2)
			}
			delete(cfg.Profiles, name)
			wasActive := cfg.ActiveProfile == name
			if wasActive {
				cfg.ActiveProfile = ""
			}
			if err := cfg.Save(); err != nil {
				p.Message("Error [CONFIG]: %v", err)
				return command.Handled(78)
			}
			if err := auth.DeleteKeyIfPresent(name); err != nil {
				p.Message("Warning: profile removed, but the keychain key could not be deleted: %v", err)
			}
			p.Message("Profile %q removed (config + keychain key).", name)
			if wasActive {
				p.Message("Warning: %q was the active profile — there is no active profile now.", name)
				p.Message("  Try:  normatik auth use <profile>  ·  normatik login")
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "Show the profiles (without API keys)",
		Example: "  normatik auth list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, _ := cmd.Flags().GetString("output")
			p := render.New(output)
			cfg, err := config.Load()
			if err != nil {
				p.Message("Error [CONFIG]: %v", err)
				return command.Handled(78)
			}
			p.Profiles(cfg.ActiveProfile, profilesView(cfg))
			return nil
		},
	})
	return cmd
}

func newBaseURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "base-url",
		Short:   "Print the active site URL (handy for building page-links)",
		Example: "  normatik base-url",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, res, err := command.Resolve(cmd)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error [CONFIG]:", err)
				return command.Handled(78)
			}
			// No implicit localhost fallback anymore: print a hint (not a blank
			// line) and fail with EX_CONFIG when no environment is configured.
			if res.BaseURL == "" {
				fmt.Fprintln(os.Stderr, "(no environment configured — run normatik login)")
				return command.Handled(78)
			}
			fmt.Fprintln(os.Stdout, res.BaseURL)
			return nil
		},
	}
}

// --- helpers ---

func profileNames(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Profiles))
	for k := range cfg.Profiles {
		out = append(out, k)
	}
	return out
}

// profilesView builds the `auth list` view: base-URL + keychain key presence
// per profile. Only presence is checked — key material never leaves this scope.
func profilesView(cfg *config.Config) map[string]render.ProfileInfo {
	out := make(map[string]render.ProfileInfo, len(cfg.Profiles))
	for k, v := range cfg.Profiles {
		_, err := auth.GetKey(k)
		out[k] = render.ProfileInfo{BaseURL: v.BaseURL, HasKey: err == nil}
	}
	return out
}

// confirmProfileOverwrite decides whether login may repoint an EXISTING profile
// to a different base-URL. On a TTY the user is asked; non-interactive runs
// (no TTY or --no-input) proceed WITHOUT a question — that is the script/agent
// path. This deliberately differs from the destructive --confirm commands
// (which fail closed non-interactively): login is idempotent and recoverable,
// and scripted re-logins must keep working. Package var so tests can pin the
// TTY answer path.
var confirmProfileOverwrite = func(cmd *cobra.Command, label string) bool {
	if nonInteractive(cmd) {
		return true
	}
	ans, err := promptLine(label)
	if err != nil {
		return false
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes"
}

func whoamiName(cmd *cobra.Command, base, key string) string {
	c, err := client.New(base, key)
	if err != nil {
		return ""
	}
	body, apiErr := c.Me(cmd.Context())
	if apiErr != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if s, ok := m["displayName"].(string); ok {
			return s
		}
	}
	return ""
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func promptLine(label string) (string, error) {
	if !isTTY(os.Stdin) {
		return "", errors.New("non-interactive")
	}
	fmt.Fprint(os.Stderr, label)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	return strings.TrimSpace(s), err
}

// promptSecret reads one line without echoing it to the terminal — used for the
// pasted API key so the secret never lands on screen or in shell scrollback. It
// errors when stdin is not a TTY (there is nothing to read securely).
func promptSecret(label string) (string, error) {
	if !isTTY(os.Stdin) {
		return "", errors.New("non-interactive")
	}
	fmt.Fprint(os.Stderr, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return strings.TrimSpace(string(b)), err
}

// promptSecretKey asks for an API key without echoing it. Package var so tests
// can drive the paste path deterministically without a real TTY (same seam
// pattern as promptEnvURL / openBrowser).
var promptSecretKey = func() (string, error) {
	return promptSecret("API key (wiki_...): ")
}

// menuChoiceReader reads one line of method-menu input. Package var so the menu
// loop (re-ask on invalid input) can be tested with scripted answers without a
// TTY.
var menuChoiceReader = func() (string, error) {
	return promptLine("Choose 1 or 2 [2]: ")
}

// promptMethodChoice shows the interactive method menu (V3a wizard step 2) and
// returns the chosen method. On a real terminal (stdin AND stdout a TTY) it is
// an arrow-key selector in the Normatik house style; otherwise — piped stdout,
// or when raw mode is unavailable — it falls back to the plain numbered line
// menu. Package var so the RunE dispatch can be tested by pinning the returned
// method (seam like promptEnvURL / openBrowser); the line menu is tested via
// runMethodMenu and the selector core via runInteractiveMenu with scripted input.
var promptMethodChoice = func(p *render.Printer) (loginMethod, error) {
	if stdinIsTTY() && stdoutIsTTY() {
		if m, err, ok := promptMethodChoiceInteractive(); ok {
			return m, err
		}
	}
	return runMethodMenu(p, menuChoiceReader)
}

// methodMenuItems are the two authentication choices, in the same order as the
// numbered line menu (index 0 = paste, 1 = browser) so digit quick-select and
// the fallback stay consistent. Browser is preselected: it is the default a bare
// login has always taken on Enter.
func methodMenuItems() []menuItem {
	return []menuItem{
		{title: "Paste an existing API key", hint: "paste your own Normatik API key into the terminal to authenticate"},
		{title: "Create a key in your browser", hint: "open the browser to create and authenticate using a new Normatik API key"},
	}
}

// promptMethodChoiceInteractive runs the arrow-key selector against the real
// terminal. It puts stdin in raw mode (returning ok=false if that fails so the
// caller falls back to the line menu), maps the chosen row to a loginMethod, and
// propagates a cancel (Esc/q/Ctrl-C) as an error the wizard treats as abort.
func promptMethodChoiceInteractive() (loginMethod, error, bool) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return methodBrowser, nil, false
	}
	defer func() { _ = term.Restore(fd, old) }()

	idx, err := runInteractiveMenu(os.Stdin, os.Stdout, "How do you want to log in?", methodMenuItems(), 1)
	if err != nil {
		return methodBrowser, err, true
	}
	if idx == 0 {
		return methodPaste, nil, true
	}
	return methodBrowser, nil, true
}

// runMethodMenu prints the method menu and loops `read` until it yields a valid
// choice. An empty line picks the default (browser); invalid input re-asks with a
// hint. Split out from promptMethodChoice so tests can feed scripted answers
// without touching the seam the dispatch stubs.
func runMethodMenu(p *render.Printer, read func() (string, error)) (loginMethod, error) {
	p.Message("How do you want to log in?")
	p.Message("  1) Paste an existing API key")
	p.Message("  2) Create a key in your browser")
	for {
		ans, err := read()
		if err != nil {
			return methodBrowser, err
		}
		switch strings.TrimSpace(ans) {
		case "", "2":
			return methodBrowser, nil
		case "1":
			return methodPaste, nil
		default:
			p.Message("Please enter 1 or 2.")
		}
	}
}

// promptEnvURL asks for the environment URL in the Normatik house style: an
// orange ◆ marker, a bold label, and — when resolvable — the current URL shown
// dimmed as the default that Enter accepts. No example is shown (users know
// their own URL); an empty line returns def. Package var so tests can drive the
// interactive login path without a real TTY (same seam pattern as
// confirmProfileOverwrite / openBrowser).
var promptEnvURL = func(def string) (string, error) {
	var b strings.Builder
	b.WriteString(styleAccent.Render("◆") + " ")
	b.WriteString(styleStrong.Render("Environment URL"))
	if def != "" {
		b.WriteString(" " + styleMuted.Render("["+def+"]"))
	}
	b.WriteString(styleMuted.Render(":") + " ")
	v, err := promptLine(b.String())
	if err != nil {
		return "", err
	}
	if v == "" {
		return def, nil
	}
	return v, nil
}

// resolveLoginURL determines the environment URL for a login run (V3 wizard
// step 1), shared by the browser and paste paths. Precedence: --url wins
// outright. Without it the base is resolved from --profile / NORMATIK_BASE_URL /
// active-profile (auth.Resolve, no localhost fallback). On an interactive TTY
// the user is prompted with that base prefilled (empty prefill when none is
// known) so they confirm or change it. Non-interactively (--no-input or no TTY)
// there is no prompt: a resolvable base is used as-is, and its absence is a
// clean CONFIG error demanding --url or NORMATIK_BASE_URL. Returns the
// canonical site URL; transports add the /api context path.
func resolveLoginURL(cmd *cobra.Command, p *render.Printer, urlFlag string) (string, error) {
	if s := strings.TrimSpace(urlFlag); s != "" {
		return s, nil
	}
	cfg, err := config.Load()
	if err != nil {
		p.Message("Error [CONFIG]: %v", err)
		return "", command.Handled(78)
	}
	profileFlag, _ := cmd.Flags().GetString("profile")
	resolved, resolveErr := auth.Resolve(cfg, profileFlag)
	if resolveErr != nil {
		p.Message("Error [CONFIG]: %v", resolveErr)
		return "", command.Handled(78)
	}
	base := resolved.BaseURL

	if nonInteractive(cmd) {
		if base != "" {
			return base, nil // scripted/CI: use the resolved base, never prompt
		}
		p.Message("Error [CONFIG]: no environment configured -- pass --url or set NORMATIK_BASE_URL (non-interactive: no prompt).")
		return "", command.Handled(78)
	}

	v, perr := promptEnvURL(base)
	if perr != nil {
		if base != "" {
			return base, nil // could not read the TTY: fall back to the resolved base
		}
		p.Message("Error [USAGE]: --url required (or run interactively in a terminal)")
		return "", command.Handled(2)
	}
	if v = strings.TrimSpace(v); v == "" {
		p.Message("Error [USAGE]: an environment URL is required — pass --url or enter one at the prompt.")
		return "", command.Handled(2)
	}
	return v, nil
}
