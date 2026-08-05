package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/localfile"
	"github.com/spf13/cobra"
)

// maxPasswordBytes bounds the non-interactive password read so a piped stdin
// cannot force unbounded buffering.
const maxPasswordBytes = 4096

// readNewUserPassword obtains a new password WITHOUT ever taking it from argv
// (NORMATIK-12, CWE-214): on a TTY it prompts twice without echo; when stdin is
// piped (CI) it reads a single bounded line. Package var so tests can drive the
// value without a real terminal (same seam pattern as promptSecretKey).
var readNewUserPassword = func() (string, error) {
	if isTTY(os.Stdin) {
		pw, err := promptSecret("New password: ")
		if err != nil {
			return "", err
		}
		confirm, err := promptSecret("Confirm password: ")
		if err != nil {
			return "", err
		}
		if pw != confirm {
			return "", errors.New("passwords do not match")
		}
		if pw == "" {
			return "", errors.New("password must not be empty")
		}
		return pw, nil
	}
	// Non-interactive (piped stdin): read one bounded line, still off argv.
	r := bufio.NewReader(io.LimitReader(os.Stdin, maxPasswordBytes))
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	pw := strings.TrimRight(line, "\r\n")
	if pw == "" {
		return "", errors.New("password must not be empty")
	}
	return pw, nil
}

// maxFormFileBytes bounds the -f form/markdown reads (NORMATIK-21): generous for JSON forms
// and page content, but prevents an oversized or special file from exhausting memory.
const maxFormFileBytes = 16 << 20

// loadForm reads a JSON file into a form type (the -f form.json route for
// complex forms with nested arrays / tagged unions). Reads no-follow, regular-file-only and
// size-bounded via localfile (NORMATIK-21).
func loadForm[T any](path string) (T, error) {
	var v T
	data, err := localfile.ReadBounded(path, maxFormFileBytes)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(data, &v)
	return v, err
}

// runWrite builds deps, calls a write fn and renders the result; an empty 2xx
// body (204) prints the success message instead of a blank object.
func runWrite(cmd *cobra.Command, invocation, successMsg string, fn func(*command.Deps) ([]byte, *client.APIError), fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if strings.TrimSpace(string(body)) == "" {
		d.Printer.Message("%s", successMsg)
		return nil
	}
	d.Printer.Raw(body, fields...)
	return nil
}

// pointer helpers for optional form fields.
func strPtr(s string) *string { return &s }
func i64Ptr(i int64) *int64   { return &i }
func boolPtr(b bool) *bool    { return &b }
func changedStr(cmd *cobra.Command, name, v string) *string {
	if cmd.Flags().Changed(name) {
		return strPtr(v)
	}
	return nil
}
func changedI64(cmd *cobra.Command, name string, v int64) *int64 {
	if cmd.Flags().Changed(name) {
		return i64Ptr(v)
	}
	return nil
}
func changedBool(cmd *cobra.Command, name string, v bool) *bool {
	if cmd.Flags().Changed(name) {
		return boolPtr(v)
	}
	return nil
}
