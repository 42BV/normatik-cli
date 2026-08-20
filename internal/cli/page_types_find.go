package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

// knownDataTypes is the sorted allowlist of PropertyDescriptor dataTypes, derived
// from the generated constants (there is no generated AllValues helper). It backs
// --data-type validation and the valid-values hint line.
var knownDataTypes = []string{
	string(api.PropertyDescriptorResultDataTypeCALCULATED),
	string(api.PropertyDescriptorResultDataTypeCONDITIONALENUM),
	string(api.PropertyDescriptorResultDataTypeDATE),
	string(api.PropertyDescriptorResultDataTypeDATETIME),
	string(api.PropertyDescriptorResultDataTypeENUM),
	string(api.PropertyDescriptorResultDataTypeMARKDOWN),
	string(api.PropertyDescriptorResultDataTypeNUMBER),
	string(api.PropertyDescriptorResultDataTypePAGEINCOMING),
	string(api.PropertyDescriptorResultDataTypePAGEOUTGOING),
	string(api.PropertyDescriptorResultDataTypePAGESELECTOR),
	string(api.PropertyDescriptorResultDataTypePAGETYPE),
	string(api.PropertyDescriptorResultDataTypePARENTPAGE),
	string(api.PropertyDescriptorResultDataTypePROPERTYCHAIN),
	string(api.PropertyDescriptorResultDataTypeTEXT),
	string(api.PropertyDescriptorResultDataTypeUSERLIST),
}

// pageTypePropertyMatch is one output row: a page type that owns a property
// matching the --data-type / --writable filters.
type pageTypePropertyMatch struct {
	PageType   string `json:"pageType"`
	PageTypeId int64  `json:"pageTypeId"`
	Property   string `json:"property"`
	DataType   string `json:"dataType"`
	Writable   bool   `json:"writable"`
}

// newPageTypesFindCmd discovers which page types own a property of a given
// dataType (and/or a writable property), aggregating client-side over the full
// page-types list — one detail call per page type.
func newPageTypesFindCmd() *cobra.Command {
	var dataType string
	var writable bool
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find page types that have a property of a given data type",
		Long: "Find page types whose property descriptors match a data type and/or writability.\n\n" +
			"This aggregates CLIENT-SIDE: it fetches the full page-types list and then issues one\n" +
			"detail call per page type, so it can be slow and issue many requests on large\n" +
			"installations. --data-type matches against the writable and read-only dataTypes;\n" +
			"--writable narrows the result to properties that can be set via `--property`.\n\n" +
			"List the pages of a matched type with: normatik pages list --page-type-id <id>",
		Example: "  normatik page-types find --data-type NUMBER\n" +
			"  normatik page-types find --data-type PAGE_OUTGOING --writable\n" +
			"  normatik page-types find --writable --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			dataTypeSet := cmd.Flags().Changed("data-type")
			var wanted api.PropertyDescriptorResultDataType
			if dataTypeSet {
				upper := strings.ToUpper(strings.TrimSpace(dataType))
				if !containsString(knownDataTypes, upper) {
					extra := []string{fmt.Sprintf("valid data types: %s", strings.Join(knownDataTypes, ", "))}
					if dym := closestName(upper, knownDataTypes); dym != "" {
						extra = append([]string{fmt.Sprintf("did you mean '%s'?", dym)}, extra...)
					}
					return renderPropError(d, invalidRequest(fmt.Sprintf("unknown data-type %q", upper), extra...), "normatik page-types find")
				}
				wanted = api.PropertyDescriptorResultDataType(upper)
			}

			body, apiErr := d.Client.ListPageTypes(cmd.Context())
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik page-types find")
			}
			// `find` maps to the same bare /page-types URL as `list`/`delete`
			// (mapping table); the --data-type/--writable client-side fan-out
			// below has nothing to do with that URL, so --url short-circuits
			// right after the same base validation `list` performs.
			if command.PrintURL(d, cmd, weburl.PageTypes()) {
				return nil
			}
			var pageTypes []api.PageTypeListResult
			if err := json.Unmarshal(body, &pageTypes); err != nil {
				d.Printer.Malformed(0, body)
				return command.Handled(65)
			}

			rows := make([]pageTypePropertyMatch, 0)
			for _, pt := range pageTypes {
				id := deref64(pt.Id)
				descriptors, dErr := fetchDescriptors(cmd.Context(), d.Client, id)
				if dErr != nil {
					return command.RenderError(d.Printer, dErr, "normatik page-types find")
				}
				for _, desc := range descriptors {
					dt := derefDataType(desc.DataType)
					if dataTypeSet && dt != wanted {
						continue
					}
					if writable && !writableDataType(dt) {
						continue
					}
					rows = append(rows, pageTypePropertyMatch{
						PageType:   derefStr(pt.Name),
						PageTypeId: id,
						Property:   derefStr(desc.Name),
						DataType:   string(dt),
						Writable:   writableDataType(dt),
					})
				}
			}
			out, _ := json.Marshal(rows)
			d.Printer.List(out, "pageType", "pageTypeId", "property", "dataType", "writable")
			return nil
		},
	}
	cmd.Flags().StringVar(&dataType, "data-type", "", "filter to properties of this dataType (case-insensitive)")
	cmd.Flags().BoolVar(&writable, "writable", false, "only writable properties")
	cmd.MarkFlagsOneRequired("data-type", "writable")
	command.URLFlag(cmd)
	return cmd
}

// containsString reports whether s is present in list.
func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
