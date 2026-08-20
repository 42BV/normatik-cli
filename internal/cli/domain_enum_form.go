package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/localfile"
	"github.com/spf13/cobra"
)

// decodeDomainEnumForm is the type-specific decode for DomainEnumForm.
// It does not go through loadForm[T]: sortOrder is a Go int32, so a missing
// field would become 0. autoSort fills 0..n-1 in file order and skips the
// presence check.
func decodeDomainEnumForm(data []byte, autoSort bool) (api.DomainEnumForm, error) {
	var raw struct {
		Name   string            `json:"name"`
		Values []json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return api.DomainEnumForm{}, err
	}
	form := api.DomainEnumForm{
		Name:   raw.Name,
		Values: make([]api.DomainEnumValueForm, 0, len(raw.Values)),
	}
	for i, rawVal := range raw.Values {
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(rawVal, &keys); err != nil {
			return api.DomainEnumForm{}, fmt.Errorf("values[%d]: %w", i, err)
		}
		var v api.DomainEnumValueForm
		if err := json.Unmarshal(rawVal, &v); err != nil {
			return api.DomainEnumForm{}, fmt.Errorf("values[%d]: %w", i, err)
		}
		sortRaw, hasSort := keys["sortOrder"]
		if autoSort {
			v.SortOrder = int32(i)
		} else if !hasSort || string(sortRaw) == "null" {
			return api.DomainEnumForm{}, invalidRequest(
				fmt.Sprintf("values[%d].sortOrder is required", i),
				"values[].sortOrder is required; 0 is the first position. Use --auto-sort-order on create.",
			)
		}
		form.Values = append(form.Values, v)
	}
	return form, nil
}

func runDomainEnumFormWrite(
	cmd *cobra.Command,
	file string,
	autoSort bool,
	invocation, success string,
	fn func(*command.Deps, api.DomainEnumForm) ([]byte, *client.APIError),
	urlPath func([]byte) string,
) error {
	d, err := command.Build(cmd)
	if err != nil {
		return err
	}
	data, rerr := localfile.ReadBounded(file, maxFormFileBytes)
	if rerr != nil {
		d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, rerr)
		return command.Handled(2)
	}
	form, derr := decodeDomainEnumForm(data, autoSort)
	if derr != nil {
		var pfe *propFlagError
		if errors.As(derr, &pfe) {
			return renderPropError(d, pfe, invocation)
		}
		d.Printer.Message("Error [FORM]: could not read -f %q: %v", file, derr)
		return command.Handled(2)
	}
	body, apiErr := fn(d, form)
	if apiErr != nil {
		return command.RenderError(d.Printer, apiErr, invocation)
	}
	if command.PrintURL(d, cmd, urlPath(body)) {
		return nil
	}
	writeResult(d, body, success)
	return nil
}
