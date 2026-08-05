package cli

import (
	"io"
	"os"

	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

// paging holds the shared list flags.
type paging struct {
	page, size int
	sort       []string
}

func addPaging(cmd *cobra.Command, pg *paging) {
	cmd.Flags().IntVar(&pg.page, "page", 1, "page number (one-based)")
	cmd.Flags().IntVar(&pg.size, "size", 10, "items per page")
	cmd.Flags().StringArrayVar(&pg.sort, "sort", nil, "sort expression, e.g. name,asc (repeatable; server-side whitelist)")
}

// runList builds deps, calls fn and renders the body as a list/table.
func runList(cmd *cobra.Command, invocation string, fn func(*command.Deps) ([]byte, *client.APIError), fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	d.Printer.List(body, fields...)
	return nil
}

// runObject builds deps, calls fn and renders the body as a single object.
func runObject(cmd *cobra.Command, invocation string, fn func(*command.Deps) ([]byte, *client.APIError), fields ...string) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	d.Printer.Raw(body, fields...)
	return nil
}

// writeBinary writes a conditional-GET download to --output-file or stdout,
// honouring a 304 not-modified result.
func writeBinary(cmd *cobra.Command, invocation, outFile string, fn func(*command.Deps) (io.ReadCloser, bool, *client.APIError)) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	body, notModified, apiErr := fn(d)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if notModified {
		d.Printer.Message("304 Not Modified — file is unchanged.")
		return nil
	}
	defer func() { _ = body.Close() }()
	if outFile != "" {
		written, werr := atomicWrite(outFile, body)
		if werr != nil {
			if limitErr := client.APIErrorFromRead(werr); limitErr != nil {
				return command.RenderError(d.Printer, limitErr, invocation)
			}
			d.Printer.Message("Error [WRITE]: could not write %q: %v", outFile, werr)
			return command.Handled(1)
		}
		d.Printer.Message("%d bytes written to %s", written, outFile)
		return nil
	}
	if _, werr := io.Copy(os.Stdout, body); werr != nil {
		if limitErr := client.APIErrorFromRead(werr); limitErr != nil {
			return command.RenderError(d.Printer, limitErr, invocation)
		}
		d.Printer.Message("Error [WRITE]: could not write download to stdout: %v", werr)
		return command.Handled(1)
	}
	return nil
}
